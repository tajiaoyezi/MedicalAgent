package routes

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/storage"
	"medoffice/server/internal/uploadgate"
)

type docWithPerm struct {
	docperm.DocumentRow
	EffectivePermission docperm.Level `json:"effectivePermission"`
}

type versionRow struct {
	VersionID       string    `gorm:"column:version_id" json:"version_id"`
	DocumentVersion int       `gorm:"column:document_version" json:"document_version"`
	FileHash        string    `gorm:"column:file_hash" json:"file_hash"`
	SavedBy         string    `gorm:"column:saved_by" json:"saved_by"`
	SavedAt         time.Time `gorm:"column:saved_at" json:"saved_at"`
	Source          string    `gorm:"column:source" json:"source"`
	SizeBytes       int64     `gorm:"column:size_bytes" json:"size_bytes"`
}

func roleCSV(u auth.AuthUser) *string { return audit.P(strings.Join(u.RoleSlugs, ",")) }

// getDoc 取单文档（按租户隔离）。found=false → 404 由调用方处理。
func getDoc(db *gorm.DB, id, tenantID string) (docperm.DocumentRow, bool, error) {
	var doc docperm.DocumentRow
	err := db.Raw(`SELECT * FROM documents WHERE document_id = ? AND tenant_id = ?`, id, tenantID).Scan(&doc).Error
	if err != nil {
		return doc, false, err
	}
	return doc, doc.DocumentID != "", nil
}

func emitUploadSuccess(tx *gorm.DB, tenantID, documentID, versionID string, payload map[string]any) error {
	mb := mustJSON(payload)
	return tx.Exec(
		`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
		 VALUES ('upload_success', ?, ?, ?, ?::jsonb)`,
		documentID, versionID, tenantID, mb,
	).Error
}

// RegisterDocuments 复刻 routes/documents.ts 全部 13 个端点。
func RegisterDocuments(r *gin.Engine, db *gorm.DB, store *storage.Storage) {
	r.GET("/api/documents", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		recycle := c.Query("recycle")
		space := c.Query("space")
		appSource := c.Query("appSource")

		sql := `SELECT * FROM documents WHERE tenant_id = ?`
		args := []any{user.TenantID}
		if recycle == "true" {
			sql += " AND is_deleted = TRUE"
		} else {
			sql += " AND is_deleted = FALSE"
			if space != "" {
				sql += " AND space = ?"
				args = append(args, space)
			}
			if appSource != "" {
				sql += " AND app_source = ?"
				args = append(args, appSource)
			}
		}
		sql += " ORDER BY updated_at DESC"

		var rows []docperm.DocumentRow
		if err := db.Raw(sql, args...).Scan(&rows).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		visible := []docWithPerm{}
		for _, row := range rows {
			level, err := docperm.Resolve(db, user, row)
			if err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			if level != docperm.None {
				visible = append(visible, docWithPerm{DocumentRow: row, EffectivePermission: level})
			}
		}
		c.JSON(http.StatusOK, gin.H{"documents": visible})
	})

	r.POST("/api/documents/upload", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20)
		fh, err := c.FormFile("file")
		if err != nil {
			httpx.Fail(c, 400, "缺少文件")
			return
		}
		f, err := fh.Open()
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		buffer, rerr := io.ReadAll(f)
		f.Close()
		if rerr != nil { // 含 MaxBytesReader 超限：拒绝而非存半截
			httpx.Fail(c, 400, "文件读取失败或超出大小限制")
			return
		}
		space := c.PostForm("space")
		if space == "" {
			space = "my"
		}
		appSource := c.PostForm("appSource")
		mimetype := fh.Header.Get("Content-Type")
		filename := fh.Filename

		gate := uploadgate.Check(filename, buffer)
		if !gate.Allowed {
			_ = audit.Write(db, audit.Entry{
				TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
				ActionType: "file_upload", Result: "失败",
				FailureReason: audit.P(orStr(gate.FailureReason, "上传被门禁阻止")),
			})
			httpx.Fail(c, 403, gate.FailureReason)
			return
		}

		documentID := uuid.NewString()
		versionID := uuid.NewString()
		fileHash := storage.ComputeFileHash(buffer)
		objectKey := storage.ObjectKeyForVersion(user.TenantID, documentID, versionID)
		if err := store.Put(c.Request.Context(), objectKey, buffer, mimetype); err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				documentID, user.TenantID, user.UserID, filename, space, nullIfEmpty(appSource), nullIfEmpty(mimetype),
			).Error; err != nil {
				return err
			}
			if err := tx.Exec(
				`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
				 VALUES (?, ?, ?, 1, ?, ?, 'import', ?, ?)`,
				versionID, documentID, user.TenantID, fileHash, user.UserID, objectKey, len(buffer),
			).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, documentID).Error; err != nil {
				return err
			}
			if err := emitUploadSuccess(tx, user.TenantID, documentID, versionID, map[string]any{"filename": filename, "source": "upload"}); err != nil {
				return err
			}
			return audit.Write(tx, audit.Entry{
				TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
				ActionType: "file_upload", TargetType: audit.P("document"), TargetID: audit.P(documentID), Result: "成功",
			})
		})
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"documentId": documentID, "versionId": versionID, "fileHash": fileHash})
	})

	r.POST("/api/documents/create", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			Name      string `json:"name"`
			Space     string `json:"space"`
			AppSource string `json:"appSource"`
			Source    string `json:"source"`
			Content   string `json:"content"`
		}
		_ = c.ShouldBindJSON(&body)
		if strings.TrimSpace(body.Name) == "" {
			httpx.Fail(c, 400, "缺少文档名称")
			return
		}
		space := body.Space
		if space == "" {
			space = "my"
		}
		versionSource := "import"
		if body.Source == "template" || body.Source == "ai_writeback" || body.Source == "import" {
			versionSource = body.Source
		}
		content := []byte(body.Content)

		documentID := uuid.NewString()
		versionID := uuid.NewString()
		fileHash := storage.ComputeFileHash(content)
		objectKey := storage.ObjectKeyForVersion(user.TenantID, documentID, versionID)
		if err := store.Put(c.Request.Context(), objectKey, content, "text/plain"); err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(
				`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
				 VALUES (?, ?, ?, ?, ?, ?, 'text/plain')`,
				documentID, user.TenantID, user.UserID, strings.TrimSpace(body.Name), space, nullIfEmpty(body.AppSource),
			).Error; err != nil {
				return err
			}
			if err := tx.Exec(
				`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
				 VALUES (?, ?, ?, 1, ?, ?, ?, ?, ?)`,
				versionID, documentID, user.TenantID, fileHash, user.UserID, versionSource, objectKey, len(content),
			).Error; err != nil {
				return err
			}
			if err := tx.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, documentID).Error; err != nil {
				return err
			}
			if err := emitUploadSuccess(tx, user.TenantID, documentID, versionID, map[string]any{"source": "server_create"}); err != nil {
				return err
			}
			return audit.Write(tx, audit.Entry{
				TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
				ActionType: "document_create", TargetType: audit.P("document"), TargetID: audit.P(documentID), Result: "成功",
			})
		})
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"documentId": documentID, "versionId": versionID, "fileHash": fileHash})
	})

	r.GET("/api/documents/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		doc, found, err := getDoc(db, id, user.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, err := docperm.Resolve(db, user, doc)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if level == docperm.None {
			httpx.Fail(c, 403, "无权限")
			return
		}
		var versions []versionRow
		_ = db.Raw(
			`SELECT version_id, document_version, file_hash, saved_by, saved_at, source, size_bytes
			 FROM document_versions WHERE document_id = ? ORDER BY saved_at DESC`, id,
		).Scan(&versions).Error
		c.JSON(http.StatusOK, gin.H{
			"document": docWithPerm{DocumentRow: doc, EffectivePermission: level},
			"versions": versions,
		})
	})

	r.PATCH("/api/documents/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		var body struct {
			Name        *string `json:"name"`
			IsFavorited *bool   `json:"isFavorited"`
		}
		_ = c.ShouldBindJSON(&body)
		doc, found, err := getDoc(db, id, user.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanEdit(level) {
			httpx.Fail(c, 403, "无权限")
			return
		}
		if body.Name != nil && *body.Name != "" {
			_ = db.Exec(`UPDATE documents SET name = ?, updated_at = NOW() WHERE document_id = ?`, *body.Name, id).Error
		}
		if body.IsFavorited != nil {
			_ = db.Exec(`UPDATE documents SET is_favorited = ?, updated_at = NOW() WHERE document_id = ?`, *body.IsFavorited, id).Error
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.DELETE("/api/documents/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		doc, found, err := getDoc(db, id, user.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanEdit(level) && level != docperm.Manage {
			httpx.Fail(c, 403, "无权限")
			return
		}
		_ = db.Exec(`UPDATE documents SET is_deleted = TRUE, updated_at = NOW() WHERE document_id = ?`, id).Error
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "file_delete", TargetType: audit.P("document"), TargetID: audit.P(id), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/api/documents/:id/restore", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanEdit(level) {
			httpx.Fail(c, 403, "无权限")
			return
		}
		_ = db.Exec(`UPDATE documents SET is_deleted = FALSE, updated_at = NOW() WHERE document_id = ?`, id).Error
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/api/documents/:id/download", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanDownload(level) {
			_ = audit.Write(db, audit.Entry{
				TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
				ActionType: "file_download", TargetType: audit.P("document"), TargetID: audit.P(id),
				Result: "失败", FailureReason: audit.P("权限不足"),
			})
			httpx.Fail(c, 403, "无下载权限")
			return
		}
		var objectKey string
		if doc.CurrentVersionID != nil {
			_ = db.Raw(`SELECT object_key FROM document_versions WHERE version_id = ?`, *doc.CurrentVersionID).Scan(&objectKey).Error
		}
		if objectKey == "" {
			// 无可用版本：不签空 key、不记伪成功审计（Node 此处会 500，这里更明确地 404）
			httpx.Fail(c, 404, "无可下载版本")
			return
		}
		url, err := store.PresignedURL(c.Request.Context(), objectKey, 300*time.Second)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "file_download", TargetType: audit.P("document"), TargetID: audit.P(id), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"url": url, "expiresIn": 300})
	})

	r.POST("/api/documents/:id/permissions", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		var body struct {
			PrincipalType   string `json:"principalType"`
			PrincipalID     string `json:"principalId"`
			PermissionLevel string `json:"permissionLevel"`
		}
		_ = c.ShouldBindJSON(&body)
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanManagePermissions(level) {
			httpx.Fail(c, 403, "无权限")
			return
		}
		validLevels := map[string]bool{"owner": true, "manage": true, "edit": true, "comment": true, "view": true, "none": true}
		validTypes := map[string]bool{"user": true, "role": true, "dept": true}
		if !validTypes[body.PrincipalType] || !validLevels[body.PermissionLevel] || body.PrincipalID == "" {
			httpx.Fail(c, 400, "principalType / permissionLevel / principalId 无效")
			return
		}
		var one int
		switch body.PrincipalType {
		case "user":
			_ = db.Raw(`SELECT 1 FROM users WHERE user_id::text = ? AND tenant_id = ?`, body.PrincipalID, user.TenantID).Scan(&one).Error
		case "role":
			_ = db.Raw(`SELECT 1 FROM roles WHERE slug = ? AND tenant_id = ?`, body.PrincipalID, user.TenantID).Scan(&one).Error
		default:
			_ = db.Raw(`SELECT 1 FROM users WHERE dept_id = ? AND tenant_id = ? LIMIT 1`, body.PrincipalID, user.TenantID).Scan(&one).Error
		}
		if one == 0 {
			httpx.Fail(c, 400, "principal 不存在或不属于当前租户")
			return
		}
		if err := db.Exec(
			`INSERT INTO document_permissions (tenant_id, document_id, principal_type, principal_id, permission_level)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT (document_id, principal_type, principal_id)
			 DO UPDATE SET permission_level = EXCLUDED.permission_level`,
			user.TenantID, id, body.PrincipalType, body.PrincipalID, body.PermissionLevel,
		).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "document_permission_change", TargetType: audit.P("document"), TargetID: audit.P(id), Result: "成功",
			Metadata: map[string]any{"principalType": body.PrincipalType, "principalId": body.PrincipalID, "permissionLevel": body.PermissionLevel},
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/api/documents/:id/share", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanShare(level) {
			httpx.Fail(c, 403, "无分享权限")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "分享占位（本期路由占位）"})
	})

	r.POST("/api/documents/:id/versions", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 50<<20)
		fh, err := c.FormFile("file")
		if err != nil {
			httpx.Fail(c, 400, "缺少文件")
			return
		}
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !docperm.CanEdit(level) {
			httpx.Fail(c, 403, "无权限")
			return
		}
		f, _ := fh.Open()
		buffer, rerr := io.ReadAll(f)
		f.Close()
		if rerr != nil {
			httpx.Fail(c, 400, "文件读取失败或超出大小限制")
			return
		}
		var nextVersion int
		_ = db.Raw(`SELECT COALESCE(MAX(document_version), 0) + 1 FROM document_versions WHERE document_id = ?`, id).Scan(&nextVersion).Error
		versionID := uuid.NewString()
		fileHash := storage.ComputeFileHash(buffer)
		objectKey := storage.ObjectKeyForVersion(user.TenantID, id, versionID)
		if err := store.Put(c.Request.Context(), objectKey, buffer, fh.Header.Get("Content-Type")); err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if err := db.Exec(
			`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			 VALUES (?, ?, ?, ?, ?, ?, 'user_edit', ?, ?)`,
			versionID, id, user.TenantID, nextVersion, fileHash, user.UserID, objectKey, len(buffer),
		).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = db.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, id).Error
		c.JSON(http.StatusOK, gin.H{"versionId": versionID, "documentVersion": nextVersion, "fileHash": fileHash})
	})

	// 合并 /actions/open 与 /actions/:action 为单一 :action 路由（避免 gin 同层 静态+通配 叶子冲突）。
	r.GET("/api/documents/:id/actions/:action", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		action := c.Param("action")

		if action == "open" {
			var doc docperm.DocumentRow
			_ = db.Raw(`SELECT * FROM documents WHERE document_id = ? AND tenant_id = ? AND is_deleted = FALSE`, id, user.TenantID).Scan(&doc).Error
			if doc.DocumentID == "" {
				httpx.Fail(c, 404, "文档不存在")
				return
			}
			level, _ := docperm.Resolve(db, user, doc)
			if level == docperm.None {
				httpx.Fail(c, 403, "无权限")
				return
			}
			c.JSON(http.StatusOK, gin.H{"redirect": "/editor/" + id, "documentId": id})
			return
		}

		allowed := map[string]bool{"aimed": true, "translate": true, "template": true, "knowledge": true}
		if !allowed[action] {
			httpx.Fail(c, 400, "未知操作")
			return
		}
		doc, found, _ := getDoc(db, id, user.TenantID)
		if !found {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if level == docperm.None {
			httpx.Fail(c, 403, "无权限")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"placeholder": true, "action": action, "documentId": id,
			"message": "操作 " + action + " 为路由占位，由后续 phase 实现",
		})
	})
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
