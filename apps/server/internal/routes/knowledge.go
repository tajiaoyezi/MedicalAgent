package routes

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/knowledge"
	"medoffice/server/internal/rag"
)

// kbStatus 把知识库服务层语义错误映射为 HTTP 状态码与中文文案。
func kbStatus(err error) (int, string) {
	switch {
	case errors.Is(err, knowledge.ErrForbidden):
		return http.StatusForbidden, "无权限"
	case errors.Is(err, knowledge.ErrNotFound):
		return http.StatusNotFound, "知识库不存在"
	case errors.Is(err, knowledge.ErrConflict):
		return http.StatusConflict, "同名知识库已存在"
	case errors.Is(err, knowledge.ErrInvalidInput):
		return http.StatusBadRequest, "参数不合法"
	case errors.Is(err, knowledge.ErrRejectedSource):
		return http.StatusForbidden, "来源被红线禁止（未授权商业库/镜像站/下载链接）"
	case errors.Is(err, knowledge.ErrNotAuthorized):
		return http.StatusForbidden, "来源未授权，仅可临时预览、不可写入正式公共库"
	case errors.Is(err, knowledge.ErrMissingMeta):
		return http.StatusBadRequest, "缺少必录元数据字段，无法完成入库"
	case errors.Is(err, knowledge.ErrRedactionBlock):
		return http.StatusForbidden, "脱敏门禁拦截"
	default:
		return http.StatusInternalServerError, "服务器错误"
	}
}

// RegisterKnowledge 挂载 c06 知识库管理 + 受控导入 + 全局搜索 + 检索问答路由。
// 搜索（/api/kb-search）与问答（/api/kb-qa/*）复用 c04 rag.Engine / aimed.Service 内核（KB 作数据源 + kb_id 选择）。
func RegisterKnowledge(r *gin.Engine, db *gorm.DB, aimedSvc *aimed.Service, ragEng *rag.Engine) {
	// 知识库列表（已按确定性多级排序）；canCreate 决定前端是否展示「创建知识库」管理入口（§11.4 终端用户隔离）。
	r.GET("/api/kb", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		cards, err := knowledge.ListVisible(db, user)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"knowledgeBases": cards, "canCreate": user.HasPermission("kb:create")})
	})

	// 单库卡片（租户隔离 + 可见性校验；不可见返回 404 不泄露存在性）。
	r.GET("/api/kb/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		card, err := knowledge.Get(db, user, c.Param("id"))
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, card)
	})

	// 创建空知识库（kb:create 锚定 c01 auth-rbac；普通用户无入口、调用被拒）。
	r.POST("/api/kb", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			DataSource  string `json:"dataSource"`
		}
		_ = c.ShouldBindJSON(&body)
		kbID, err := knowledge.Create(db, user, body.Name, body.Description, body.DataSource)
		if err != nil {
			code, msg := kbStatus(err)
			// 越权创建尝试留痕（spec「普通用户绕过 UI 直接调用被拒绝」配套审计）。
			if errors.Is(err, knowledge.ErrForbidden) {
				_ = audit.Write(db, audit.Entry{
					TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
					ActionType: "kb_create", TargetType: audit.P("knowledge_base"),
					Result: "失败", FailureReason: audit.P("无 kb:create 权限点"),
				})
			}
			httpx.Fail(c, code, msg)
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "kb_create", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID),
			Result: "成功", Metadata: map[string]any{"name": body.Name},
		})
		c.JSON(http.StatusOK, gin.H{"kbId": kbID})
	})

	// 配置置顶/手动权重（仅平台管理员或库创建人；PR3 以 per-kb 管理级 ACL 收口库管理员判定）。
	r.PATCH("/api/kb/:id/ranking", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			IsPinned     *bool `json:"isPinned"`
			ManualWeight *int  `json:"manualWeight"`
			ClearWeight  bool  `json:"clearWeight"`
		}
		_ = c.ShouldBindJSON(&body)
		if err := knowledge.SetRanking(db, user, c.Param("id"), body.IsPinned, body.ManualWeight, body.ClearWeight); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "kb_ranking_update", TargetType: audit.P("knowledge_base"), TargetID: audit.P(c.Param("id")),
			Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// ── 受控导入管线（四段式 + 三态授权状态机）──
	// 预览（来源适配器 → staging）：经授权闸门定状态、落 staging 行（不进正式索引）。
	r.POST("/api/kb/:id/import", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			SourceType      string `json:"sourceType"`
			SourceURL       string `json:"sourceUrl"`
			Title           string `json:"title"`
			AdminAuthorized bool   `json:"adminAuthorized"`
			DocumentID      string `json:"documentId"`
			PublicNetwork   bool   `json:"publicNetwork"`
		}
		_ = c.ShouldBindJSON(&body)
		kbDocID, err := knowledge.PreviewImport(db, user, knowledge.ImportRequest{
			KBID: c.Param("id"), SourceType: body.SourceType, SourceURL: body.SourceURL, Title: body.Title,
			AdminAuthorized: body.AdminAuthorized, DocumentID: body.DocumentID, PublicNetwork: body.PublicNetwork,
		})
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"kbDocumentId": kbDocID})
	})

	// 列出某库导入记录（含 staging 与正式，解析/索引状态可追踪）。
	r.GET("/api/kb/:id/documents", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		if _, err := knowledge.Get(db, user, c.Param("id")); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		var rows []struct {
			KBDocumentID        string `gorm:"column:kb_document_id" json:"kbDocumentId"`
			Title               string `gorm:"column:title" json:"title"`
			SourceType          string `gorm:"column:source_type" json:"sourceType"`
			SourceURL           string `gorm:"column:source_url" json:"sourceUrl"`
			AuthorizationStatus string `gorm:"column:authorization_status" json:"authorizationStatus"`
			IsStaging           bool   `gorm:"column:is_staging" json:"isStaging"`
			ParseStatus         string `gorm:"column:parse_status" json:"parseStatus"`
			IndexStatus         string `gorm:"column:index_status" json:"indexStatus"`
		}
		_ = db.Raw(
			`SELECT kb_document_id, title, source_type, source_url, authorization_status, is_staging, parse_status, index_status
			 FROM kb_documents WHERE tenant_id = ? AND kb_id = ? ORDER BY imported_at DESC`,
			user.TenantID, c.Param("id"),
		).Scan(&rows).Error
		c.JSON(http.StatusOK, gin.H{"documents": rows})
	})

	// 入库前预览确认（人工确认链路）：仅 authorized + 必录字段非空可入正式库。
	r.POST("/api/kb-documents/:kbDocId/confirm", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		if err := knowledge.ConfirmImport(db, user, c.Param("kbDocId")); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 取消预览：丢弃 staging 资料，不落正式库、不建索引。
	r.POST("/api/kb-documents/:kbDocId/cancel", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		if err := knowledge.CancelImport(db, user, c.Param("kbDocId")); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 管理员触发重建索引：产生 manual_reindex document_events（c03 消费重解析），收尾走同一索引就绪路径。
	r.POST("/api/kb-documents/:kbDocId/reindex", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		if err := knowledge.Reindex(db, user, c.Param("kbDocId")); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// ── 全局搜索（§11.6）：三模式（keyword/semantic/hybrid）+ 多维筛选（知识库/文档类型/来源/更新时间），
	// 复用 c04 rag.Retrieve（权限六维过滤召回前），kb_id 数据源选择按可见集合裁剪 ──
	r.POST("/api/kb-search", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			KBIds        []string `json:"kbIds"`
			Query        string   `json:"query"`
			Mode         string   `json:"mode"`
			DocType      string   `json:"docType"`
			Source       string   `json:"source"`
			UpdatedAfter string   `json:"updatedAfter"`
		}
		_ = c.ShouldBindJSON(&body)
		var f knowledge.SearchFilters
		f.DocType = body.DocType
		f.Source = body.Source
		if body.UpdatedAfter != "" {
			if t, err := time.Parse(time.RFC3339, body.UpdatedAfter); err == nil {
				f.UpdatedAfter = &t
			}
		}
		res, err := knowledge.KBSearch(db, ragEng, user, body.KBIds, body.Query, body.Mode, f)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// ── 知识库问答（§11.7）：/api/kb-qa/* 独立树，复用 c04 aimed.Answer（检索→rerank→生成带引用答案→
	// 高风险经 c05 message 级确认前置→§19.3 免责声明/草稿/无召回不臆造），KB 作数据源 + kb_id 选择 ──
	r.POST("/api/kb-qa/conversations", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			Title string `json:"title"`
		}
		_ = c.ShouldBindJSON(&body)
		convID, err := knowledge.StartKBQA(db, user, body.Title)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"conversationId": convID})
	})

	r.POST("/api/kb-qa/conversations/:id/ask", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			KBIds []string `json:"kbIds"`
			Query string   `json:"query"`
		}
		_ = c.ShouldBindJSON(&body)
		res, err := knowledge.AskKB(db, aimedSvc, user, c.Param("id"), body.KBIds, body.Query)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// 会话恢复回源（§6.6 由 c05 最近任务恢复编排消费）：取 kb_qa 会话 + 消息。
	r.GET("/api/kb-qa/conversations/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		conv, err := aimed.GetConversation(db, user.TenantID, user.UserID, c.Param("id"))
		if err != nil || conv.Module != aimed.ModuleKBQA {
			httpx.Fail(c, 404, "会话不存在")
			return
		}
		msgs, _ := aimed.ListMessages(db, user.TenantID, conv.ConversationID)
		c.JSON(http.StatusOK, gin.H{"conversation": conv, "messages": msgs})
	})
}
