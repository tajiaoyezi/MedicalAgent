package routes

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/model"
	"medoffice/server/internal/parsing"
)

var (
	mmProtocols    = map[string]bool{"openai_compat": true, "anthropic_messages": true, "local_gateway": true, "third_party": true}
	mmDeployKinds  = map[string]bool{"public": true, "private": true}
	mmBackendKinds = map[string]bool{"ocr": true, "multimodal_llm": true, "layout": true, "table": true, "third_party_api": true, "private_service": true}
)

// RegisterAdminModels 复刻 routes/admin-models.ts：/api/admin/models/* 仅 model:manage 可访问；越权落审计。
func RegisterAdminModels(r *gin.Engine, db *gorm.DB) {
	g := r.Group("/api/admin/models")
	g.Use(func(c *gin.Context) {
		u, ok := auth.SessionUser(c)
		if !ok {
			httpx.Fail(c, 401, "未登录")
			return
		}
		if !u.HasPermission("model:manage") {
			_ = audit.Write(db, audit.Entry{
				TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: audit.P(strings.Join(u.RoleSlugs, ",")),
				ActionType: "model_config_access_denied", TargetType: audit.P("model_config"), Result: "失败", FailureReason: audit.P("无模型与评测管理权限"),
			})
			httpx.Fail(c, 403, "无模型与评测管理权限")
			return
		}
		c.Next()
	})
	cur := func(c *gin.Context) auth.AuthUser { u, _ := auth.SessionUser(c); return u }
	roleCSVm := func(u auth.AuthUser) *string { return audit.P(strings.Join(u.RoleSlugs, ",")) }

	// ——— model providers ———
	g.GET("/providers", func(c *gin.Context) {
		u := cur(c)
		list, err := model.ListModelProviders(db, u.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"providers": list})
	})

	g.POST("/providers", func(c *gin.Context) {
		u := cur(c)
		var body struct {
			Name            string `json:"name"`
			Protocol        string `json:"protocol"`
			DeploymentKind  string `json:"deploymentKind"`
			BaseURL         string `json:"baseUrl"`
			Credential      string `json:"credential"`
			Model           string `json:"model"`
			TimeoutMs       *int   `json:"timeoutMs"`
			MaxRetries      *int   `json:"maxRetries"`
			NetworkPolicy   string `json:"networkPolicy"`
			Enabled         bool   `json:"enabled"`
			DefaultPriority *int   `json:"defaultPriority"`
		}
		_ = c.ShouldBindJSON(&body)
		if !mmProtocols[body.Protocol] || !mmDeployKinds[body.DeploymentKind] {
			httpx.Fail(c, 400, "protocol / deploymentKind 非法")
			return
		}
		if body.Name == "" || body.BaseURL == "" || body.Model == "" {
			httpx.Fail(c, 400, "缺少 name / baseUrl / model")
			return
		}
		id, err := model.CreateModelProvider(db, u.TenantID, model.ModelProviderInput{
			Name: body.Name, Protocol: body.Protocol, DeploymentKind: body.DeploymentKind, BaseURL: body.BaseURL,
			Credential: body.Credential, Model: body.Model, TimeoutMs: body.TimeoutMs, MaxRetries: body.MaxRetries,
			NetworkPolicy: body.NetworkPolicy, Enabled: body.Enabled, DefaultPriority: body.DefaultPriority,
		})
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "model_provider_create", TargetType: audit.P("model_provider"), TargetID: audit.P(id), Result: "成功",
			Metadata: map[string]any{"name": body.Name, "protocol": body.Protocol, "deploymentKind": body.DeploymentKind},
		})
		c.JSON(http.StatusOK, gin.H{"providerId": id})
	})

	g.PATCH("/providers/:id", func(c *gin.Context) {
		u := cur(c)
		id := c.Param("id")
		var body struct {
			Name            *string `json:"name"`
			BaseURL         *string `json:"baseUrl"`
			Credential      *string `json:"credential"`
			Model           *string `json:"model"`
			TimeoutMs       *int    `json:"timeoutMs"`
			MaxRetries      *int    `json:"maxRetries"`
			NetworkPolicy   *string `json:"networkPolicy"`
			Enabled         *bool   `json:"enabled"`
			DefaultPriority *int    `json:"defaultPriority"`
		}
		_ = c.ShouldBindJSON(&body)
		ok, err := model.UpdateModelProvider(db, u.TenantID, id, model.ModelProviderPatch(body))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "provider 不存在")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "model_provider_update", TargetType: audit.P("model_provider"), TargetID: audit.P(id), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	g.DELETE("/providers/:id", func(c *gin.Context) {
		u := cur(c)
		id := c.Param("id")
		ok, err := model.DeleteModelProvider(db, u.TenantID, id)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "provider 不存在")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "model_provider_delete", TargetType: audit.P("model_provider"), TargetID: audit.P(id), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	g.POST("/providers/:id/test", func(c *gin.Context) {
		u := cur(c)
		id := c.Param("id")
		var body struct {
			Capability string `json:"capability"`
		}
		_ = c.ShouldBindJSON(&body)
		capability := model.Capability(body.Capability)
		if capability == "" {
			capability = model.CapChat
		}
		res, err := model.TestModelConnectivity(db, u.TenantID, id, capability)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		resp := gin.H{"capability": capability, "status": res.Status, "latencyMs": res.LatencyMs}
		if res.Error != "" {
			resp["error"] = res.Error
		}
		c.JSON(http.StatusOK, resp)
	})

	// ——— routes ———
	g.GET("/routes", func(c *gin.Context) {
		u := cur(c)
		list, err := model.ListRoutes(db, u.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"routes": list})
	})

	g.POST("/routes", func(c *gin.Context) {
		u := cur(c)
		var body struct {
			Capability string `json:"capability"`
			ProviderID string `json:"providerId"`
			Priority   *int   `json:"priority"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Capability == "" || !model.IsRoutableCapability(model.Capability(body.Capability)) {
			httpx.Fail(c, 400, "capability 非法（视觉解析须配置于 visual-providers）")
			return
		}
		if body.ProviderID == "" {
			httpx.Fail(c, 400, "缺少 providerId")
			return
		}
		priority := 100
		if body.Priority != nil {
			priority = *body.Priority
		}
		if err := model.BindRoute(db, u.TenantID, model.Capability(body.Capability), body.ProviderID, priority); err != nil {
			var rbe *model.RouteBindError
			if errors.As(err, &rbe) {
				httpx.Fail(c, 400, rbe.Msg)
				return
			}
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "model_route_bind", TargetType: audit.P("model_route"), TargetID: audit.P(body.Capability + ":" + body.ProviderID), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	g.DELETE("/routes/:routeId", func(c *gin.Context) {
		u := cur(c)
		ok, err := model.UnbindRoute(db, u.TenantID, c.Param("routeId"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "route 不存在")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// ——— visual parse providers ———
	g.GET("/visual-providers", func(c *gin.Context) {
		u := cur(c)
		list, err := model.ListVisualProviders(db, u.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, gin.H{"providers": list})
	})

	g.POST("/visual-providers", func(c *gin.Context) {
		u := cur(c)
		var body struct {
			Name            string `json:"name"`
			BackendKind     string `json:"backendKind"`
			DeploymentKind  string `json:"deploymentKind"`
			BaseURL         string `json:"baseUrl"`
			Credential      string `json:"credential"`
			Model           string `json:"model"`
			TimeoutMs       *int   `json:"timeoutMs"`
			NetworkPolicy   string `json:"networkPolicy"`
			Enabled         bool   `json:"enabled"`
			DefaultPriority *int   `json:"defaultPriority"`
		}
		_ = c.ShouldBindJSON(&body)
		if !mmBackendKinds[body.BackendKind] || !mmDeployKinds[body.DeploymentKind] {
			httpx.Fail(c, 400, "backendKind / deploymentKind 非法")
			return
		}
		if body.Name == "" || body.BaseURL == "" {
			httpx.Fail(c, 400, "缺少 name / baseUrl")
			return
		}
		id, err := model.CreateVisualProvider(db, u.TenantID, model.VisualProviderInput{
			Name: body.Name, BackendKind: body.BackendKind, DeploymentKind: body.DeploymentKind, BaseURL: body.BaseURL,
			Credential: body.Credential, Model: body.Model, TimeoutMs: body.TimeoutMs, NetworkPolicy: body.NetworkPolicy,
			Enabled: body.Enabled, DefaultPriority: body.DefaultPriority,
		})
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "visual_provider_create", TargetType: audit.P("visual_parse_provider"), TargetID: audit.P(id), Result: "成功",
			Metadata: map[string]any{"name": body.Name, "backendKind": body.BackendKind, "deploymentKind": body.DeploymentKind},
		})
		c.JSON(http.StatusOK, gin.H{"vpProviderId": id})
	})

	g.DELETE("/visual-providers/:id", func(c *gin.Context) {
		u := cur(c)
		id := c.Param("id")
		ok, err := model.DeleteVisualProvider(db, u.TenantID, id)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "视觉解析 provider 不存在")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSVm(u),
			ActionType: "visual_provider_delete", TargetType: audit.P("visual_parse_provider"), TargetID: audit.P(id), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	g.POST("/visual-providers/:id/test", func(c *gin.Context) {
		u := cur(c)
		res, err := model.TestVisualConnectivity(db, u.TenantID, c.Param("id"))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// ——— 失败解析作业重试 ———
	g.POST("/parse-jobs/:jobId/retry", func(c *gin.Context) {
		u := cur(c)
		ok, err := parsing.RetryJob(db, u.TenantID, c.Param("jobId"), u.UserID, strings.Join(u.RoleSlugs, ","))
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if !ok {
			httpx.Fail(c, 404, "失败作业不存在或不属于当前租户")
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "已重置为 pending，下一轮解析将重新执行"})
	})

	// ——— 配置覆盖校验 ———
	g.GET("/coverage", func(c *gin.Context) {
		u := cur(c)
		coverage, err := model.ValidateCapabilityCoverage(db, u.TenantID)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		allCanGoLive := true
		mainLoopHasPrivate := true
		for _, cv := range coverage {
			if !cv.CanGoLive {
				allCanGoLive = false
			}
			if cv.IsMainLoop && !cv.HasPrivate {
				mainLoopHasPrivate = false
			}
		}
		c.JSON(http.StatusOK, gin.H{"coverage": coverage, "allCanGoLive": allCanGoLive, "mainLoopHasPrivate": mainLoopHasPrivate})
	})
}
