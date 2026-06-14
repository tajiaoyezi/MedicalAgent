// Package server 装配 gin 引擎：中间件顺序复刻 index.ts（cors → cookie/session → revoke-guard），再挂路由。
package server

import (
	"net/http"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/memstore"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/config"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/model"
	"medoffice/server/internal/routes"
	"medoffice/server/internal/storage"
)

type Deps struct {
	Config  config.Config
	DB      *gorm.DB
	Storage *storage.Storage
}

// New 构造引擎。中间件顺序复刻 index.ts，再挂各 register*Routes。
func New(d Deps) *gin.Engine {
	model.Init(d.Config.Model.CredentialSecret, d.Config.Model.HealthTTLSeconds) // 凭据密钥 + 健康 TTL
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.MaxMultipartMemory = 50 << 20 // 与 @fastify/multipart fileSize 50MB 对齐
	r.Use(gin.Logger())
	r.Use(httpx.Recovery())
	r.Use(httpx.CORS(d.Config.WebOrigin))

	store := memstore.NewStore([]byte(d.Config.SessionSecret))
	store.Options(sessions.Options{
		Path:     "/",
		MaxAge:   86400, // 24h（Node @fastify/session maxAge 86400000ms）
		HttpOnly: true,
		Secure:   d.Config.IsProd(),
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions("medoffice_sid", store))
	r.Use(auth.RevokeGuard())

	editorSvc := editor.NewService(d.Config.OnlyOffice, d.Storage)

	routes.RegisterHealth(r, d.DB)
	routes.RegisterAuth(r, d.DB)
	routes.RegisterPortal(r, d.DB)
	routes.RegisterDocuments(r, d.DB, d.Storage)
	routes.RegisterRecentTasks(r, d.DB)
	routes.RegisterAdmin(r, d.DB)
	routes.RegisterEditor(r, d.DB, d.Storage, editorSvc)
	routes.RegisterBridge(r, d.DB, editorSvc)
	routes.RegisterPreview(r, d.DB, d.Storage, d.Config.OnlyOffice)
	routes.RegisterAdminModels(r, d.DB)
	return r
}
