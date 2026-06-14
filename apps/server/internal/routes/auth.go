package routes

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
)

// nullIfEmpty 让空租户 ID 以 NULL（而非非法 UUID 空串）参与比较，复刻 Node 的 `tenantId ?? null`。
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RegisterAuth 复刻 routes/auth.ts：/api/auth/login、/logout、/session。
func RegisterAuth(r *gin.Engine, db *gorm.DB) {
	r.POST("/api/auth/login", func(c *gin.Context) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Tenant   string `json:"tenant"`
		}
		_ = c.ShouldBindJSON(&body)
		username := strings.TrimSpace(body.Username)
		password := body.Password
		if username == "" || password == "" {
			httpx.Fail(c, 400, "请输入用户名与口令")
			return
		}

		// 解析登录所属租户（username 仅在 (tenant_id, username) 维度唯一）。
		var tenantID string
		if body.Tenant != "" {
			var tid string
			if err := db.Raw(`SELECT tenant_id FROM tenants WHERE name = ? LIMIT 1`, body.Tenant).Scan(&tid).Error; err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			if tid == "" {
				httpx.Fail(c, 401, "凭据无效")
				return
			}
			tenantID = tid
		} else {
			var ids []string
			if err := db.Raw(`SELECT tenant_id FROM tenants`).Scan(&ids).Error; err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			if len(ids) == 1 {
				tenantID = ids[0]
			} else if len(ids) > 1 {
				httpx.Fail(c, 400, "存在多个租户，请指定租户")
				return
			}
		}

		var row struct {
			UserID       string `gorm:"column:user_id"`
			TenantID     string `gorm:"column:tenant_id"`
			PasswordHash string `gorm:"column:password_hash"`
			IsEnabled    bool   `gorm:"column:is_enabled"`
		}
		if err := db.Raw(
			`SELECT user_id, tenant_id, password_hash, is_enabled FROM users
			 WHERE username = ? AND tenant_id = ? LIMIT 1`,
			username, nullIfEmpty(tenantID),
		).Scan(&row).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}

		if row.UserID == "" {
			if tenantID != "" {
				_ = audit.Write(db, audit.Entry{
					TenantID: tenantID, ActionType: "login", Result: "失败",
					FailureReason: audit.P("用户不存在"), Metadata: map[string]any{"username": username},
				})
			}
			httpx.Fail(c, 401, "凭据无效")
			return
		}
		if !row.IsEnabled {
			_ = audit.Write(db, audit.Entry{
				TenantID: row.TenantID, ActorID: audit.P(row.UserID),
				ActionType: "login", Result: "失败", FailureReason: audit.P("账号已禁用"),
			})
			httpx.Fail(c, 403, "账号不可用")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)) != nil {
			_ = audit.Write(db, audit.Entry{
				TenantID: row.TenantID, ActorID: audit.P(row.UserID),
				ActionType: "login", Result: "失败", FailureReason: audit.P("口令错误"),
			})
			httpx.Fail(c, 401, "凭据无效")
			return
		}

		user, err := auth.LoadUserByID(db, row.UserID)
		if err != nil || user == nil {
			httpx.Fail(c, 401, "凭据无效")
			return
		}
		if err := auth.SetSessionUser(c, *user); err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID),
			ActorRole: audit.P(strings.Join(user.RoleSlugs, ",")), ActionType: "login", Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{
			"user": gin.H{
				"userId": user.UserID, "username": user.Username, "displayName": user.DisplayName,
				"roleSlugs": user.RoleSlugs, "isAdmin": user.IsAdmin(),
			},
			"redirectTo": "/aimed",
		})
	})

	r.POST("/api/auth/logout", func(c *gin.Context) {
		_ = auth.ClearSession(c)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/api/auth/session", func(c *gin.Context) {
		u, ok := auth.SessionUser(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"authenticated": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"authenticated": true,
			"user": gin.H{
				"userId": u.UserID, "username": u.Username, "displayName": u.DisplayName,
				"roleSlugs": u.RoleSlugs, "isAdmin": u.IsAdmin(),
			},
		})
	})
}
