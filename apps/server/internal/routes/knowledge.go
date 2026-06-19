package routes

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/knowledge"
	"medoffice/server/internal/pubmed"
	"medoffice/server/internal/rag"
	"medoffice/server/internal/storage"
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
	case errors.Is(err, knowledge.ErrSourceOffline):
		return http.StatusServiceUnavailable, "公网不可用，URL/白名单来源不可用，请改用「批量上传已下载的授权文件」完成入库"
	default:
		return http.StatusInternalServerError, "服务器错误"
	}
}

// RegisterKnowledge 挂载 c06 知识库管理 + 受控导入 + 全局搜索 + 检索问答路由。
// 搜索（/api/kb-search）与问答（/api/kb-qa/*）复用 c04 rag.Engine / aimed.Service 内核（KB 作数据源 + kb_id 选择）。
func RegisterKnowledge(r *gin.Engine, db *gorm.DB, store *storage.Storage, aimedSvc *aimed.Service, ragEng *rag.Engine, pub *pubmed.Service) {
	adapter := knowledge.NewSourceAdapter(pub) // 受控公网来源适配器（PubMed/PMC 取数 + URL 离线降级守卫）
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
			if errors.Is(err, knowledge.ErrForbidden) {
				_ = audit.Write(db, audit.Entry{
					TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
					ActionType: "kb_ranking_update", TargetType: audit.P("knowledge_base"), TargetID: audit.P(c.Param("id")),
					Result: "失败", FailureReason: audit.P("无权配置排序"),
				})
			}
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

	// 知识库级 ACL 授予（§19.1，仅平台管理员或库管理员）：把 (principal, level) 应用到该库当前正式文档的
	// document_permissions（KB 级 ACL = 文档级授权聚合），授予后刷新 member_count。
	r.POST("/api/kb/:id/grant", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			PrincipalType string `json:"principalType"`
			PrincipalID   string `json:"principalId"`
			Level         string `json:"level"`
		}
		_ = c.ShouldBindJSON(&body)
		if err := knowledge.GrantKB(db, user, c.Param("id"), body.PrincipalType, body.PrincipalID, body.Level); err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
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

	// PubMed/PMC 来源适配器（4.6/4.7）：经 c04 pubmed-data-service 按 kind(pubmed/pmc/doi)+id 取结构化文献
	// （公网可用→在线真实拉取；否则→离线缓存降级），授权三态初值取自 c04 标记；authorized 自动入库+索引。
	r.POST("/api/kb/:id/import/pubmed", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			Kind string `json:"kind"` // pubmed / pmc / doi
			ID   string `json:"id"`   // pmid / pmcid / doi
		}
		_ = c.ShouldBindJSON(&body)
		if body.ID == "" {
			httpx.Fail(c, http.StatusBadRequest, "缺少文献标识")
			return
		}
		if body.Kind == "" {
			body.Kind = "pubmed"
		}
		kbDocID, err := adapter.ImportFromPubMed(db, user, c.Param("id"), body.Kind, body.ID)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"kbDocumentId": kbDocID})
	})

	// URL/白名单来源适配器（4.4/4.8）：URL 抓取依赖公网；公网不可用→该来源置不可用并引导改用批量上传授权文件（503）。
	r.POST("/api/kb/:id/import/url", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			SourceURL       string `json:"sourceUrl"`
			AdminAuthorized bool   `json:"adminAuthorized"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.SourceURL == "" {
			httpx.Fail(c, http.StatusBadRequest, "缺少来源 URL")
			return
		}
		kbDocID, err := adapter.ImportFromURL(db, user, c.Param("id"), body.SourceURL, body.AdminAuthorized)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"kbDocumentId": kbDocID})
	})

	// 本地/批量上传入口（4.3 批量逐项落库 + 5.2a 持久化前消费 c09 上传闸）：multipart 多文件，每份先过上传闸
	// （命中阻止策略→拒绝入库+留痕、不落盘），放行则落盘 + 经导入管线落一条 staging 预览记录（N 份→N 条）。
	r.POST("/api/kb/:id/upload", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 200<<20)
		form, err := c.MultipartForm()
		if err != nil {
			httpx.Fail(c, 400, "上传解析失败或超出大小限制")
			return
		}
		fhs := form.File["files"]
		if len(fhs) == 0 {
			httpx.Fail(c, 400, "缺少文件")
			return
		}
		files := make([]knowledge.KBUploadFile, 0, len(fhs))
		for _, fh := range fhs {
			f, oerr := fh.Open()
			if oerr != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			buffer, rerr := io.ReadAll(f)
			f.Close()
			if rerr != nil {
				httpx.Fail(c, 400, "文件读取失败")
				return
			}
			files = append(files, knowledge.KBUploadFile{Filename: fh.Filename, MimeType: fh.Header.Get("Content-Type"), Buffer: buffer})
		}
		results, err := knowledge.KBUpload(c.Request.Context(), db, store, user, c.Param("id"), files)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
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

	// 管理员在权限范围内查看问答日志（9.3，§11.5）：平台管理员见全租户、库管理员见自管库相关问答；
	// 普通用户（不管理任何库）被拒。每条含用户/所选 kb_id/查询/时间与对应引用来源。
	r.GET("/api/kb-qa/logs", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		limit := 0
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}
		logs, err := knowledge.ListQALogs(db, user, c.Query("kbId"), limit)
		if err != nil {
			code, msg := kbStatus(err)
			httpx.Fail(c, code, msg)
			return
		}
		c.JSON(http.StatusOK, gin.H{"logs": logs})
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
