package routes

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/storage"
)

// RegisterEditor 复刻 routes/editor.ts：open / download / callback / metrics。
func RegisterEditor(r *gin.Engine, db *gorm.DB, store *storage.Storage, svc *editor.Service) {
	r.GET("/api/editor/open/:documentId", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		documentID := c.Param("documentId")
		var row struct {
			DocumentID string  `gorm:"column:document_id"`
			TenantID   string  `gorm:"column:tenant_id"`
			OwnerID    string  `gorm:"column:owner_id"`
			Name       string  `gorm:"column:name"`
			Space      string  `gorm:"column:space"`
			IsDeleted  bool    `gorm:"column:is_deleted"`
			CvID       *string `gorm:"column:cv_id"`
			CvHash     *string `gorm:"column:cv_hash"`
		}
		_ = db.Raw(
			`SELECT d.*, dv.version_id AS cv_id, dv.file_hash AS cv_hash
			 FROM documents d LEFT JOIN document_versions dv ON d.current_version_id = dv.version_id
			 WHERE d.document_id = ?`, documentID,
		).Scan(&row).Error
		if row.DocumentID == "" {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		if row.TenantID != user.TenantID {
			_ = audit.Write(db, audit.Entry{
				TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
				ActionType: "open", TargetType: audit.P("document"), TargetID: audit.P(documentID),
				Result: "失败", FailureReason: audit.P("跨租户访问"),
			})
			httpx.Fail(c, 403, "无权限")
			return
		}
		if row.IsDeleted {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		doc := docperm.DocumentRow{DocumentID: row.DocumentID, TenantID: row.TenantID, OwnerID: row.OwnerID, Space: row.Space}
		level, _ := docperm.Resolve(db, user, doc)
		if level == docperm.None {
			httpx.Fail(c, 403, "无权限")
			return
		}
		info := editor.ResolveEditorRoute(row.Name)
		if info.Route == editor.RouteUnsupported {
			httpx.Fail(c, 400, "不支持的文件类型")
			return
		}
		if info.Route == editor.RoutePreviewPDF || info.Route == editor.RoutePreviewImage || info.Route == editor.RoutePreviewOFD {
			c.JSON(http.StatusOK, gin.H{
				"mode":        "preview",
				"previewType": strings.TrimPrefix(string(info.Route), "preview-"),
				"documentId":  documentID,
				"permission":  level,
			})
			return
		}
		if row.CvID == nil || *row.CvID == "" || row.CvHash == nil || *row.CvHash == "" {
			httpx.Fail(c, 400, "文档无可用版本")
			return
		}
		documentKey := editor.BuildDocumentKey(documentID, *row.CvID)
		session := svc.Sessions.Create(editor.CreateInput{
			DocumentID: documentID, DocumentKey: documentKey, TenantID: user.TenantID,
			UserID: user.UserID, VersionID: *row.CvID, Revision: *row.CvHash,
		})
		svc.Sessions.Touch(session)
		editorConfig := editor.BuildEditorConfig(svc.Config(), svc.JWT, editor.BuildConfigInput{
			Session: session, Filename: row.Name, DocumentType: info.DocumentType,
			Permission: level, UserID: user.UserID, DisplayName: user.DisplayName,
		})
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "open", TargetType: audit.P("document"), TargetID: audit.P(documentID), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{
			"mode": "editor", "documentId": documentID, "permission": level,
			"dsUrl": svc.Config().DSURL, "editorConfig": editorConfig,
			"bridgeToken": session.BridgeToken, "revision": session.Revision,
		})
	})

	r.GET("/api/editor/download/:openToken", func(c *gin.Context) {
		session := svc.Sessions.GetByOpenToken(c.Param("openToken"))
		if session == nil {
			httpx.Fail(c, 403, "下载链接无效或已过期")
			return
		}
		svc.Sessions.Touch(session)
		_, versionID, _ := svc.Sessions.Snapshot(session) // 锁内读，避免与并发回调写竞争
		var ver struct {
			ObjectKey string `gorm:"column:object_key"`
			TenantID  string `gorm:"column:tenant_id"`
		}
		_ = db.Raw(`SELECT object_key, tenant_id FROM document_versions WHERE version_id = ? AND document_id = ?`, versionID, session.DocumentID).Scan(&ver).Error
		if ver.ObjectKey == "" {
			httpx.Fail(c, 404, "版本不存在")
			return
		}
		if ver.TenantID != session.TenantID {
			httpx.Fail(c, 403, "无权限")
			return
		}
		buffer, err := store.Get(c.Request.Context(), ver.ObjectKey)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		var d struct {
			Name     string  `gorm:"column:name"`
			MimeType *string `gorm:"column:mime_type"`
		}
		_ = db.Raw(`SELECT name, mime_type FROM documents WHERE document_id = ?`, session.DocumentID).Scan(&d).Error
		mime := "application/octet-stream"
		if d.MimeType != nil && *d.MimeType != "" {
			mime = *d.MimeType
		}
		name := d.Name
		if name == "" {
			name = "document"
		}
		c.Header("Content-Disposition", `attachment; filename="`+url.QueryEscape(name)+`"`)
		c.Data(http.StatusOK, mime, buffer)
	})

	r.POST("/api/editor/callback", func(c *gin.Context) {
		callbackToken := c.Query("token")
		if callbackToken == "" {
			c.JSON(http.StatusForbidden, gin.H{"error": 1})
			return
		}
		raw, _ := io.ReadAll(c.Request.Body)
		body, ok := editor.ParseCallback(raw, svc.JWT)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": 1})
			return
		}
		session := svc.Sessions.GetByCallbackToken(callbackToken)
		if session == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": 1})
			return
		}
		n := svc.ProcessSaveCallback(c.Request.Context(), db, session, body, session.UserID, "editor")
		c.JSON(http.StatusOK, gin.H{"error": n})
	})

	r.GET("/api/editor/metrics", func(c *gin.Context) {
		if _, ok := auth.Require(c); !ok {
			return
		}
		c.JSON(http.StatusOK, svc.Metrics.Snapshot())
	})
}
