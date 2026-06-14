package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/httpx"
)

// RegisterBridge 复刻 routes/bridge.ts：authorize / arm-writeback-save / confirm-preview。
func RegisterBridge(r *gin.Engine, db *gorm.DB, svc *editor.Service) {
	bridgeAudit := func(user auth.AuthUser, method, targetID, reason string) {
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "bridge:" + method, TargetType: audit.P("document"), TargetID: audit.P(targetID),
			Result: "失败", FailureReason: audit.P(reason),
		})
	}

	r.POST("/api/bridge/authorize", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			BridgeToken      string         `json:"bridgeToken"`
			Method           string         `json:"method"`
			ExpectedRevision string         `json:"expectedRevision"`
			WritebackSource  string         `json:"writebackSource"`
			Payload          map[string]any `json:"payload"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.BridgeToken == "" || body.Method == "" {
			httpx.Fail(c, 400, "缺少 bridgeToken 或 method")
			return
		}
		session := svc.Sessions.GetByBridgeToken(body.BridgeToken)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效或过期 token", "permitted": false})
			return
		}
		if session.TenantID != user.TenantID || session.UserID != user.UserID {
			bridgeAudit(user, body.Method, session.DocumentID, "跨租户或会话不匹配")
			c.JSON(http.StatusForbidden, gin.H{"error": "跨租户调用被拒绝", "permitted": false})
			return
		}
		category, known := editor.CategorizeBridgeMethod(body.Method)
		if !known {
			httpx.Fail(c, 400, "未知 Bridge 方法")
			return
		}
		var doc docperm.DocumentRow
		_ = db.Raw(`SELECT * FROM documents WHERE document_id = ?`, session.DocumentID).Scan(&doc).Error
		if doc.DocumentID == "" || doc.IsDeleted {
			bridgeAudit(user, body.Method, session.DocumentID, "文档不存在")
			c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在", "permitted": false})
			return
		}
		if doc.TenantID != user.TenantID {
			bridgeAudit(user, body.Method, session.DocumentID, "跨租户")
			c.JSON(http.StatusForbidden, gin.H{"error": "跨租户调用被拒绝", "permitted": false})
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		if !editor.IsBridgeMethodAllowed(category, level) {
			bridgeAudit(user, body.Method, session.DocumentID, "权限不足")
			c.JSON(http.StatusForbidden, gin.H{"error": "越权操作被拒绝", "permitted": false})
			return
		}
		if editor.RequiresRevisionCheck(body.Method) {
			if body.ExpectedRevision == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "写回类方法必须提供 expectedRevision", "permitted": false})
				return
			}
			if body.ExpectedRevision != session.Revision {
				bridgeAudit(user, body.Method, session.DocumentID, "文档已更新，请重新读取上下文")
				c.JSON(http.StatusConflict, gin.H{"error": "文档已更新，请重新读取上下文", "permitted": false, "staleRevision": true})
				return
			}
		}
		svc.Sessions.Touch(session)
		var saveIntentID string
		if body.Method == "saveDocument" && body.WritebackSource != "" {
			saveIntentID = svc.Sessions.CreateSaveIntent(session, body.WritebackSource)
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "bridge:" + body.Method, TargetType: audit.P("document"), TargetID: audit.P(session.DocumentID), Result: "成功",
		})
		resp := gin.H{"permitted": true, "revision": session.Revision, "documentId": session.DocumentID, "docKey": session.DocumentKey}
		if saveIntentID != "" {
			resp["saveIntentId"] = saveIntentID
		}
		if editor.IsTextExportMethod(body.Method) {
			resp["redactionGatewayAnchor"] = gin.H{
				"anchorType": "c09_redaction_gateway", "exportKind": "document_text",
				"documentId": session.DocumentID, "versionId": session.VersionID,
			}
			resp["isOriginalTextExport"] = true
		}
		c.JSON(http.StatusOK, resp)
	})

	r.POST("/api/bridge/arm-writeback-save", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			BridgeToken  string `json:"bridgeToken"`
			SaveIntentID string `json:"saveIntentId"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.BridgeToken == "" || body.SaveIntentID == "" {
			httpx.Fail(c, 400, "缺少 bridgeToken 或 saveIntentId")
			return
		}
		session := svc.Sessions.GetByBridgeToken(body.BridgeToken)
		if session == nil {
			httpx.Fail(c, 401, "无效或过期 token")
			return
		}
		if session.TenantID != user.TenantID || session.UserID != user.UserID {
			httpx.Fail(c, 403, "跨租户调用被拒绝")
			return
		}
		if !svc.Sessions.ArmWritebackSaveIntent(session, body.SaveIntentID) {
			httpx.Fail(c, 400, "写回保存意图无效或已过期")
			return
		}
		svc.Sessions.Touch(session)
		c.JSON(http.StatusOK, gin.H{"armed": true})
	})

	r.POST("/api/bridge/confirm-preview", func(c *gin.Context) {
		if _, ok := auth.Require(c); !ok {
			return
		}
		var body struct {
			OriginalText string `json:"originalText"`
			ModifiedText string `json:"modifiedText"`
			ImpactScope  string `json:"impactScope"`
			Explanation  string `json:"explanation"`
		}
		_ = c.ShouldBindJSON(&body)
		impactScope := body.ImpactScope
		if impactScope == "" {
			impactScope = "selection"
		}
		c.JSON(http.StatusOK, gin.H{
			"preview": gin.H{
				"originalText": body.OriginalText, "modifiedText": body.ModifiedText,
				"impactScope": impactScope, "explanation": body.Explanation,
			},
			"actions": []string{"apply", "copy", "cancel"},
		})
	})
}
