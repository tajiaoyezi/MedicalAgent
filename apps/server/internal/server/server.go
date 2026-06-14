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
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/routes"
	"medoffice/server/internal/storage"
)

type Deps struct {
	Config  config.Config
	DB      *gorm.DB
	Storage *storage.Storage
}

// New 构造引擎。PR0 仅挂 health；后续 PR 在此追加路由注册。
func New(d Deps) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
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

	routes.RegisterHealth(r, d.DB)
	return r
}
