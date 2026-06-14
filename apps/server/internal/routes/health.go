package routes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
)

// RegisterHealth 复刻 routes/health.ts：/health、/api/health、/api/me。
func RegisterHealth(r *gin.Engine, gdb *gorm.DB) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "medoffice-api"})
	})

	r.GET("/api/health", func(c *gin.Context) {
		if sqlDB, err := gdb.DB(); err == nil {
			if err := sqlDB.Ping(); err == nil {
				c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "connected"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"status": "degraded", "database": "disconnected"})
	})

	r.GET("/api/me", auth.RequireAuth(), func(c *gin.Context) {
		u := auth.CurrentUser(c)
		c.JSON(http.StatusOK, gin.H{
			"userId":      u.UserID,
			"tenantId":    u.TenantID,
			"username":    u.Username,
			"displayName": u.DisplayName,
			"roleSlugs":   u.RoleSlugs,
			"permissions": u.Permissions,
			"isAdmin":     u.IsAdmin(),
		})
	})
}
