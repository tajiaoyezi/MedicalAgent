package routes

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/citation"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/storage"
)

// RegisterAIMed 装配 c04 AIMed 学术助手端点（RAG 检索 / 引用溯源 / 答案落地）。
func RegisterAIMed(r *gin.Engine, db *gorm.DB, store *storage.Storage, svc *aimed.Service) {
	g := r.Group("/api/aimed")
	g.Use(auth.RequireAuth())

	// 六模式元数据（label/占位文案/引导文案/数据源约束）
	g.GET("/modes", func(c *gin.Context) {
		out := []gin.H{}
		for _, m := range aimed.Modes {
			p := aimed.GetPolicy(m)
			out = append(out, gin.H{
				"mode": string(m), "label": p.Label, "placeholder": p.Placeholder, "guidance": p.Guidance,
				"allowPubmed": p.AllowPubmed, "allowUpload": p.AllowUpload, "allowKb": p.AllowKB, "allowCurrentDoc": p.AllowCurrentDoc,
				"uploadRequired": p.UploadRequired, "clearFilesOnEnter": p.ClearFilesOnEnter, "showPubmedTag": p.ShowPubmedTag(),
			})
		}
		c.JSON(http.StatusOK, gin.H{"modes": out})
	})

	// 智能模式匹配（发送时触发；仅高亮推荐，不强制切换）
	g.POST("/match", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		_ = u
		var body struct {
			Mode string `json:"mode"`
			Text string `json:"text"`
		}
		_ = c.ShouldBindJSON(&body)
		res := aimed.Evaluate(aimed.Mode(body.Mode), body.Text)
		c.JSON(http.StatusOK, res)
	})

	// 建会话
	g.POST("/conversations", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Module string `json:"module"`
			Mode   string `json:"mode"`
			Title  string `json:"title"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Mode != "" && body.Module != aimed.ModuleKBQA && !aimed.IsMode(body.Mode) {
			httpx.Fail(c, 400, "无效的模式")
			return
		}
		id, err := aimed.CreateConversation(db, u.TenantID, u.UserID, body.Module, body.Mode, body.Title)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		conv, _ := aimed.GetConversation(db, u.TenantID, u.UserID, id)
		c.JSON(http.StatusOK, gin.H{"conversationId": id, "conversation": conv})
	})

	// 从在线文档发起会话（c05 面板侧 Bridge 传入已组装上下文，取数归 c05，建会话归 c04）
	g.POST("/conversations/from-document", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			DocumentID string `json:"documentId"`
			Context    string `json:"context"` // c05 已组装的全文/选区/结构
			Mode       string `json:"mode"`
		}
		_ = c.ShouldBindJSON(&body)
		mode := body.Mode
		if mode == "" {
			mode = string(aimed.ModeWritingAssist)
		}
		id, err := aimed.CreateConversation(db, u.TenantID, u.UserID, aimed.ModuleAimed, mode, "文档会话")
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if strings.TrimSpace(body.Context) != "" {
			_, _ = aimed.AddMessage(db, u.TenantID, u.UserID, id, "user", "[文档上下文]\n"+body.Context, mode, nil, map[string]any{"contextFromDocument": body.DocumentID})
		}
		c.JSON(http.StatusOK, gin.H{"conversationId": id, "currentDocId": body.DocumentID})
	})

	// 列会话
	g.GET("/conversations", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		rows, err := aimed.ListConversations(db, u.TenantID, u.UserID, c.Query("module"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"conversations": rows})
	})

	// 取会话 + 消息
	g.GET("/conversations/:id", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		msgs, _ := aimed.ListMessages(db, u.TenantID, conv.ConversationID)
		p := aimed.GetPolicy(aimed.Mode(conv.Mode))
		c.JSON(http.StatusOK, gin.H{"conversation": conv, "messages": msgs, "files": conv.Files(), "placeholder": p.Placeholder, "showPubmedTag": p.ShowPubmedTag()})
	})

	// 切换模式（§8.3）
	g.POST("/conversations/:id/mode", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Mode string `json:"mode"`
		}
		_ = c.ShouldBindJSON(&body)
		if !aimed.IsMode(body.Mode) {
			httpx.Fail(c, 400, "无效的模式")
			return
		}
		conv, err := aimed.SwitchMode(db, u.TenantID, u.UserID, c.Param("id"), aimed.Mode(body.Mode))
		if err != nil {
			failConv(c, err)
			return
		}
		p := aimed.GetPolicy(aimed.Mode(body.Mode))
		c.JSON(http.StatusOK, gin.H{"conversation": conv, "placeholder": p.Placeholder, "showPubmedTag": p.ShowPubmedTag(), "files": conv.Files()})
	})

	// 发送按钮状态机（§8.5）
	g.POST("/conversations/:id/send-state", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		var body struct {
			Mode string `json:"mode"`
			Text string `json:"text"`
		}
		_ = c.ShouldBindJSON(&body)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		mode := aimed.Mode(body.Mode)
		if body.Mode == "" {
			mode = aimed.Mode(conv.Mode)
		}
		var files []aimed.FileState
		for _, f := range conv.Files() {
			files = append(files, aimed.FileState{Status: f.Status})
		}
		c.JSON(http.StatusOK, aimed.CanSend(mode, body.Text, files))
	})

	// 上传文件（§8.6 约束 + PHI 门禁 + 落库主闭环）
	g.POST("/conversations/:id/files", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (aimed.MaxFileBytes)+(1<<20))
		fh, ferr := c.FormFile("file")
		if ferr != nil {
			httpx.Fail(c, 400, "缺少文件")
			return
		}
		f, _ := fh.Open()
		buffer, rerr := io.ReadAll(f)
		f.Close()
		if rerr != nil {
			httpx.Fail(c, 400, aimed.MsgFileTooLarge)
			return
		}
		if ok, reason := aimed.CheckUploadConstraints(conv, fh.Filename, fh.Size, buffer); !ok {
			httpx.Fail(c, 400, reason)
			return
		}
		uf, ierr := aimed.IngestFile(db, store, u, fh.Filename, fh.Header.Get("Content-Type"), buffer, "本地文件")
		if ierr != nil {
			var ue *aimed.UploadError
			if errors.As(ierr, &ue) {
				httpx.Fail(c, 403, ue.Reason)
				return
			}
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		files := append(conv.Files(), uf)
		_ = aimed.SetUploadedFiles(db, u.TenantID, u.UserID, conv.ConversationID, files)
		c.JSON(http.StatusOK, gin.H{"file": uf, "files": files})
	})

	// 删除会话文件（标记已删除）
	g.DELETE("/conversations/:id/files/:fileId", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		fileID := c.Param("fileId")
		files := conv.Files()
		found := false
		for i := range files {
			if files[i].FileID == fileID {
				files[i].Status = aimed.FileDeleted
				found = true
			}
		}
		if !found {
			httpx.Fail(c, 404, "文件不存在")
			return
		}
		_ = aimed.SetUploadedFiles(db, u.TenantID, u.UserID, conv.ConversationID, files)
		c.JSON(http.StatusOK, gin.H{"ok": true, "files": files})
	})

	// 提问（主问答闭环）
	g.POST("/conversations/:id/ask", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		var body struct {
			Query        string `json:"query"`
			CurrentDocID string `json:"currentDocId"`
		}
		_ = c.ShouldBindJSON(&body)
		if strings.TrimSpace(body.Query) == "" {
			httpx.Fail(c, 400, "请输入内容后再发送")
			return
		}
		res, aerr := svc.Answer(db, aimed.AnswerRequest{User: u, Conversation: conv, Query: body.Query, CurrentDocID: body.CurrentDocID})
		if aerr != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// 取消息引用（先校验 message 归当前用户所有，避免同租户跨用户读他人引用元数据）
	g.GET("/messages/:id/citations", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		if _, err := aimed.GetMessage(db, u.TenantID, u.UserID, c.Param("id")); err != nil {
			httpx.Fail(c, 404, "消息不存在")
			return
		}
		cites, err := citation.ListByMessage(db, u.TenantID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"citations": cites})
	})

	// 点击引用定位（先取引用再校验其所属 message 归当前用户；非本人/不存在统一降级为「已删除」，不泄露存在性）
	g.POST("/citations/:id/locate", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		cit, err := citation.Get(db, u.TenantID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if cit != nil {
			if _, merr := aimed.GetMessage(db, u.TenantID, u.UserID, cit.MessageID); merr != nil {
				cit = nil // 存在但非本人 → 视同不存在，避免泄露
			}
		}
		res, lerr := citation.LocateLoaded(db, u, cit)
		if lerr != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// 反馈：赞/踩（§8.10.5）
	g.POST("/messages/:id/feedback", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		msgID := c.Param("id")
		var body struct {
			Rating  string `json:"rating"` // 赞 / 踩
			Reason  string `json:"reason"`
			Comment string `json:"comment"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Rating != "赞" && body.Rating != "踩" {
			httpx.Fail(c, 400, "rating 必须为 赞 或 踩")
			return
		}
		// §8.10.5：reason 为「踩」专属维度——踩必取 7 项之一，赞不携带 reason（清空避免污染统计）。
		reason := body.Reason
		if body.Rating == "踩" {
			if !aimed.IsValidFeedbackReason(reason) {
				httpx.Fail(c, 400, "无效的反馈原因")
				return
			}
		} else {
			reason = ""
		}
		if _, err := aimed.GetMessage(db, u.TenantID, u.UserID, msgID); err != nil {
			httpx.Fail(c, 404, "消息不存在")
			return
		}
		if err := aimed.WriteFeedback(db, u.TenantID, u.UserID, "message", msgID, body.Rating, reason, body.Comment); err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "reasons": aimed.FeedbackReasons})
	})

	// 删除消息（二次确认在前端；此处软删 + 同步）
	g.DELETE("/messages/:id", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		ok, err := aimed.DeleteMessage(db, u.TenantID, u.UserID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "消息不存在")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// 重新生成（保留旧版本：旧消息不删，新答案以 parent 链追加）
	g.POST("/messages/:id/regenerate", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		old, err := aimed.GetMessage(db, u.TenantID, u.UserID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 404, "消息不存在")
			return
		}
		conv, cerr := aimed.GetConversation(db, u.TenantID, u.UserID, old.ConversationID)
		if cerr != nil {
			failConv(c, cerr)
			return
		}
		// 以该答案的原始用户输入重新生成。复用原 user 消息为新答案的 parent：
		// 同一提问的多个答案版本挂在同一 parent 下（§8 重新生成版本链），不再重复插入一条 user 消息。
		query := old.Content
		req := aimed.AnswerRequest{User: u, Conversation: conv, Query: query}
		if old.ParentMessageID != nil {
			if um, e := aimed.GetMessage(db, u.TenantID, u.UserID, *old.ParentMessageID); e == nil {
				query = um.Content
				req.Query = query
				req.ExistingUserMessageID = *old.ParentMessageID
			}
		}
		res, aerr := svc.Answer(db, req)
		if aerr != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"regenerated": res, "keptOldMessageId": old.MessageID})
	})

	// 生成在线 Word
	g.POST("/messages/:id/generate-word", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		msg, err := aimed.GetMessage(db, u.TenantID, u.UserID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 404, "消息不存在")
			return
		}
		conv, cerr := aimed.GetConversation(db, u.TenantID, u.UserID, msg.ConversationID)
		if cerr != nil {
			failConv(c, cerr)
			return
		}
		res, gerr := svc.GenerateWord(db, store, u, conv, msg.MessageID)
		if gerr != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// 保存为（范围 × 格式）
	g.POST("/conversations/:id/save-as", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		conv, err := loadConv(db, u, c.Param("id"))
		if err != nil {
			failConv(c, err)
			return
		}
		var body struct {
			Scope     string `json:"scope"`
			Format    string `json:"format"`
			MessageID string `json:"messageId"`
		}
		_ = c.ShouldBindJSON(&body)
		res, serr := svc.SaveAs(db, store, u, conv, body.Scope, body.Format, body.MessageID)
		if serr != nil {
			httpx.Fail(c, 400, serr.Error())
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// 翻译/保存后翻译入口分流（§8.12 / §13.2）
	g.POST("/conversations/:id/translate", func(c *gin.Context) {
		u := auth.CurrentUser(c)
		_ = u
		var body struct {
			Target string `json:"target"` // selection / short / whole / full
		}
		_ = c.ShouldBindJSON(&body)
		if body.Target == "selection" || body.Target == "short" {
			c.JSON(http.StatusOK, gin.H{"route": "aimed_inline", "mode": string(aimed.ModeWritingAssist), "message": "选区/短文本翻译走 AIMed 学术写作辅助内联处理"})
			return
		}
		// 整篇/全文 → 分流 c07 建 translation_job（建任务服务由 c07 提供，本期未落地）
		c.JSON(http.StatusOK, gin.H{"route": "c07_translation", "message": "整篇/全文翻译分流至医学翻译模块（c07 建 translation_job）", "deferred": true})
	})
}

// loadConv 取会话（404/500 由 failConv 处理）。
func loadConv(db *gorm.DB, u auth.AuthUser, id string) (*aimed.Conversation, error) {
	return aimed.GetConversation(db, u.TenantID, u.UserID, id)
}

func failConv(c *gin.Context, err error) {
	if errors.Is(err, aimed.ErrNotFound) {
		httpx.Fail(c, 404, "会话不存在")
		return
	}
	httpx.Fail(c, 500, "服务器错误")
}
