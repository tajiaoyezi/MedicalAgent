// Package auth 复刻 middleware/auth.ts + session-revoke.ts：会话内 AuthUser 快照、被吊销用户集合、
// RequireAuth/RequirePermission/RevokeGuard 中间件、LoadUserByID。
package auth

import (
	"database/sql"
	"encoding/gob"
	"strings"
	"sync"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/httpx"
)

// AuthUser 等价 services/document-permissions.ts 的 AuthUser（会话快照）。
type AuthUser struct {
	UserID      string
	TenantID    string
	Username    string
	DisplayName string
	DeptID      string
	RoleSlugs   []string
	Permissions []string
}

func (u AuthUser) HasPermission(p string) bool {
	for _, x := range u.Permissions {
		if x == p {
			return true
		}
	}
	return false
}

func (u AuthUser) IsAdmin() bool { return u.HasPermission("admin:console") }

func init() { gob.Register(AuthUser{}) }

const (
	sessionUserKey = "user"
	ctxUserKey     = "authUser"
)

// —— 被吊销用户集合（复刻 middleware/session-revoke.ts 的 in-memory Set）——
var (
	revokedMu sync.RWMutex
	revoked   = map[string]struct{}{}
)

func Revoke(id string)   { revokedMu.Lock(); revoked[id] = struct{}{}; revokedMu.Unlock() }
func Unrevoke(id string) { revokedMu.Lock(); delete(revoked, id); revokedMu.Unlock() }
func IsRevoked(id string) bool {
	revokedMu.RLock()
	_, ok := revoked[id]
	revokedMu.RUnlock()
	return ok
}

// —— 会话读写 ——

func SetSessionUser(c *gin.Context, u AuthUser) error {
	s := sessions.Default(c)
	s.Set(sessionUserKey, u)
	return s.Save()
}

func ClearSession(c *gin.Context) error {
	s := sessions.Default(c)
	s.Clear()
	return s.Save()
}

// SessionUser 返回当前会话用户（被吊销视为未登录）。
func SessionUser(c *gin.Context) (AuthUser, bool) {
	s := sessions.Default(c)
	v := s.Get(sessionUserKey)
	if v == nil {
		return AuthUser{}, false
	}
	u, ok := v.(AuthUser)
	if !ok {
		return AuthUser{}, false
	}
	if IsRevoked(u.UserID) {
		return AuthUser{}, false
	}
	return u, true
}

// CurrentUser 取 RequireAuth 注入的当前用户。
func CurrentUser(c *gin.Context) AuthUser { return c.MustGet(ctxUserKey).(AuthUser) }

// RequireAuth 等价 requireAuth：未登录 → 401「未登录」。
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		u, ok := SessionUser(c)
		if !ok {
			httpx.Fail(c, 401, "未登录")
			return
		}
		c.Set(ctxUserKey, u)
		c.Next()
	}
}

// RequirePermission 等价 requirePermission：缺权限 → 403「无权限」。须在 RequireAuth 之后。
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := CurrentUser(c)
		if !u.HasPermission(perm) {
			httpx.Fail(c, 403, "无权限")
			return
		}
		c.Next()
	}
}

// RevokeGuard 复刻 index.ts 全局 preHandler：被吊销用户清会话，且对 /api/*（除 login/health）返回 401「会话已失效」。
func RevokeGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		s := sessions.Default(c)
		v := s.Get(sessionUserKey)
		if u, ok := v.(AuthUser); ok && IsRevoked(u.UserID) {
			s.Clear()
			_ = s.Save()
			p := c.Request.URL.Path
			if strings.HasPrefix(p, "/api/") &&
				!strings.HasPrefix(p, "/api/auth/login") &&
				!strings.HasPrefix(p, "/api/health") &&
				p != "/health" {
				httpx.Fail(c, 401, "会话已失效")
				return
			}
		}
		c.Next()
	}
}

// LoadUserByID 复刻 loadUserById：禁用/不存在用户返回 nil。
func LoadUserByID(db *gorm.DB, userID string) (*AuthUser, error) {
	var row struct {
		UserID      string
		TenantID    string
		Username    string
		DisplayName string
		DeptID      sql.NullString
		IsEnabled   bool
	}
	if err := db.Raw(
		`SELECT user_id, tenant_id, username, display_name, dept_id, is_enabled FROM users WHERE user_id = ?`,
		userID,
	).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.UserID == "" || !row.IsEnabled {
		return nil, nil
	}
	var slugs []string
	if err := db.Raw(
		`SELECT r.slug FROM roles r JOIN user_roles ur ON ur.role_id = r.role_id WHERE ur.user_id = ?`,
		userID,
	).Scan(&slugs).Error; err != nil {
		return nil, err
	}
	var perms []string
	if err := db.Raw(
		`SELECT DISTINCT p.name FROM permissions p
		 JOIN role_permissions rp ON rp.permission_id = p.permission_id
		 JOIN user_roles ur ON ur.role_id = rp.role_id
		 WHERE ur.user_id = ?`,
		userID,
	).Scan(&perms).Error; err != nil {
		return nil, err
	}
	return &AuthUser{
		UserID:      row.UserID,
		TenantID:    row.TenantID,
		Username:    row.Username,
		DisplayName: row.DisplayName,
		DeptID:      row.DeptID.String,
		RoleSlugs:   slugs,
		Permissions: perms,
	}, nil
}
