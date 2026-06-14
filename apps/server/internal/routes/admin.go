package routes

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
)

// RegisterAdmin 复刻 routes/admin.ts：/api/admin/* 全组先过 admin:console 守卫，部分路由再要 user:manage/audit:view。
func RegisterAdmin(r *gin.Engine, db *gorm.DB) {
	admin := r.Group("/api/admin", auth.AdminConsoleGuard())

	admin.GET("/tenant", func(c *gin.Context) {
		user := auth.CurrentUser(c)
		var t struct {
			TenantID          string         `gorm:"column:tenant_id" json:"tenant_id"`
			Name              string         `gorm:"column:name" json:"name"`
			OrgType           string         `gorm:"column:org_type" json:"org_type"`
			Branding          datatypes.JSON `gorm:"column:branding" json:"branding"`
			EnabledModules    datatypes.JSON `gorm:"column:enabled_modules" json:"enabled_modules"`
			StorageQuotaBytes int64          `gorm:"column:storage_quota_bytes" json:"storage_quota_bytes"`
		}
		_ = db.Raw(`SELECT tenant_id, name, org_type, branding, enabled_modules, storage_quota_bytes FROM tenants WHERE tenant_id = ?`, user.TenantID).Scan(&t).Error
		var count int
		_ = db.Raw(`SELECT COUNT(*)::int FROM users WHERE tenant_id = ?`, user.TenantID).Scan(&count).Error
		c.JSON(http.StatusOK, gin.H{"tenant": t, "userCount": count, "note": "POC 单租户演示，不提供新建或切换多租户"})
	})

	admin.GET("/users", func(c *gin.Context) {
		user := auth.CurrentUser(c)
		var rows []struct {
			UserID      string         `gorm:"column:user_id" json:"user_id"`
			Username    string         `gorm:"column:username" json:"username"`
			DisplayName string         `gorm:"column:display_name" json:"display_name"`
			DeptID      *string        `gorm:"column:dept_id" json:"dept_id"`
			IsEnabled   bool           `gorm:"column:is_enabled" json:"is_enabled"`
			CreatedAt   time.Time      `gorm:"column:created_at" json:"created_at"`
			Roles       datatypes.JSON `gorm:"column:roles" json:"roles"`
		}
		_ = db.Raw(
			`SELECT u.user_id, u.username, u.display_name, u.dept_id, u.is_enabled, u.created_at,
			        COALESCE(JSON_AGG(r.slug) FILTER (WHERE r.slug IS NOT NULL), '[]'::json) AS roles
			 FROM users u
			 LEFT JOIN user_roles ur ON ur.user_id = u.user_id
			 LEFT JOIN roles r ON r.role_id = ur.role_id
			 WHERE u.tenant_id = ?
			 GROUP BY u.user_id`, user.TenantID,
		).Scan(&rows).Error
		c.JSON(http.StatusOK, gin.H{"users": rows})
	})

	admin.POST("/users", func(c *gin.Context) {
		user := auth.CurrentUser(c)
		if !auth.RequirePerm(c, user, "user:manage") {
			return
		}
		var body struct {
			Username    string `json:"username"`
			Password    string `json:"password"`
			DisplayName string `json:"displayName"`
			DeptID      string `json:"deptId"`
			RoleSlug    string `json:"roleSlug"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Username == "" || body.Password == "" || body.DisplayName == "" {
			httpx.Fail(c, 400, "缺少必填字段")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 10)
		if err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		var newUserID string
		if err := db.Raw(
			`INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
			 VALUES (?, ?, ?, ?, ?) RETURNING user_id`,
			user.TenantID, body.Username, string(hash), body.DisplayName, nullIfEmpty(body.DeptID),
		).Scan(&newUserID).Error; err != nil || newUserID == "" {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if body.RoleSlug != "" {
			var roleID string
			_ = db.Raw(`SELECT role_id FROM roles WHERE tenant_id = ? AND slug = ?`, user.TenantID, body.RoleSlug).Scan(&roleID).Error
			if roleID != "" {
				_ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, newUserID, roleID).Error
			}
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "user_create", TargetType: audit.P("user"), TargetID: audit.P(newUserID), Result: "成功",
		})
		c.JSON(http.StatusOK, gin.H{"userId": newUserID})
	})

	admin.PATCH("/users/:id", func(c *gin.Context) {
		user := auth.CurrentUser(c)
		if !auth.RequirePerm(c, user, "user:manage") {
			return
		}
		id := c.Param("id")
		var body struct {
			IsEnabled *bool   `json:"isEnabled"`
			DeptID    *string `json:"deptId"`
			RoleSlug  string  `json:"roleSlug"`
		}
		_ = c.ShouldBindJSON(&body)

		// 跨租户隔离：目标用户须属当前管理员租户，否则 404（防 IDOR）。
		var targetTenant string
		_ = db.Raw(`SELECT tenant_id FROM users WHERE user_id = ?`, id).Scan(&targetTenant).Error
		if targetTenant == "" || targetTenant != user.TenantID {
			httpx.Fail(c, 404, "用户不存在")
			return
		}

		meta := map[string]any{}
		if body.IsEnabled != nil {
			_ = db.Exec(`UPDATE users SET is_enabled = ?, updated_at = NOW() WHERE user_id = ? AND tenant_id = ?`, *body.IsEnabled, id, user.TenantID).Error
			if *body.IsEnabled {
				auth.Unrevoke(id)
			} else {
				auth.Revoke(id)
			}
			meta["isEnabled"] = *body.IsEnabled
		}
		if body.DeptID != nil {
			_ = db.Exec(`UPDATE users SET dept_id = ?, updated_at = NOW() WHERE user_id = ? AND tenant_id = ?`, *body.DeptID, id, user.TenantID).Error
			meta["deptId"] = *body.DeptID
		}
		if body.RoleSlug != "" {
			var roleID string
			_ = db.Raw(`SELECT role_id FROM roles WHERE tenant_id = ? AND slug = ?`, user.TenantID, body.RoleSlug).Scan(&roleID).Error
			if roleID != "" {
				_ = db.Exec(`DELETE FROM user_roles WHERE user_id = ?`, id).Error
				_ = db.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, id, roleID).Error
			}
			meta["roleSlug"] = body.RoleSlug
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
			ActionType: "user_update", TargetType: audit.P("user"), TargetID: audit.P(id), Result: "成功", Metadata: meta,
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	admin.GET("/audit-logs", func(c *gin.Context) {
		user := auth.CurrentUser(c)
		if !auth.RequirePerm(c, user, "audit:view") {
			return
		}
		var logs []struct {
			AuditID       string         `gorm:"column:audit_id" json:"audit_id"`
			ActorID       *string        `gorm:"column:actor_id" json:"actor_id"`
			ActorRole     *string        `gorm:"column:actor_role" json:"actor_role"`
			ActionType    string         `gorm:"column:action_type" json:"action_type"`
			TargetType    *string        `gorm:"column:target_type" json:"target_type"`
			TargetID      *string        `gorm:"column:target_id" json:"target_id"`
			Result        string         `gorm:"column:result" json:"result"`
			FailureReason *string        `gorm:"column:failure_reason" json:"failure_reason"`
			Metadata      datatypes.JSON `gorm:"column:metadata" json:"metadata"`
			CreatedAt     time.Time      `gorm:"column:created_at" json:"created_at"`
		}
		_ = db.Raw(
			`SELECT audit_id, actor_id, actor_role, action_type, target_type, target_id, result, failure_reason, metadata, created_at
			 FROM audit_logs WHERE tenant_id = ? ORDER BY created_at DESC LIMIT 200`, user.TenantID,
		).Scan(&logs).Error
		c.JSON(http.StatusOK, gin.H{"logs": logs})
	})
}
