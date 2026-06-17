package routes

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/knowledge"
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
	default:
		return http.StatusInternalServerError, "服务器错误"
	}
}

// RegisterKnowledge 挂载 c06 知识库管理路由（PR1：首页卡片/排序/创建/置顶·权重）。
// 检索问答、导入管线、ACL 等端点由后续 PR 在本文件扩展。
func RegisterKnowledge(r *gin.Engine, db *gorm.DB) {
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
}
