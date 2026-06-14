package routes

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/httpx"
)

// RegisterPortal 复刻 routes/portal.ts：GET/PUT /api/portal/branding。
func RegisterPortal(r *gin.Engine, db *gorm.DB) {
	r.GET("/api/portal/branding", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var t struct {
			TenantID          string         `gorm:"column:tenant_id"`
			Name              string         `gorm:"column:name"`
			OrgType           string         `gorm:"column:org_type"`
			EnabledModules    datatypes.JSON `gorm:"column:enabled_modules"`
			Branding          datatypes.JSON `gorm:"column:branding"`
			StorageQuotaBytes int64          `gorm:"column:storage_quota_bytes"`
		}
		if err := db.Raw(
			`SELECT tenant_id, name, org_type, enabled_modules, branding, storage_quota_bytes
			 FROM tenants WHERE tenant_id = ?`, user.TenantID,
		).Scan(&t).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		if t.TenantID == "" {
			c.JSON(http.StatusOK, gin.H{"error": "租户不存在"})
			return
		}
		var count int
		_ = db.Raw(`SELECT COUNT(*)::int FROM users WHERE tenant_id = ?`, user.TenantID).Scan(&count).Error
		c.JSON(http.StatusOK, gin.H{
			"tenantId":            t.TenantID,
			"name":                t.Name,
			"orgType":             t.OrgType,
			"enabledModules":      t.EnabledModules,
			"branding":            t.Branding,
			"userCount":           count,
			"storageQuotaBytes":   t.StorageQuotaBytes,
			"onlyofficeThemeNote": "ONLYOFFICE 编辑器原生 UI 不承诺跟随主题，仅外部宿主页面与面板入口适配主题。",
		})
	})

	r.PUT("/api/portal/branding", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		if !auth.RequirePerm(c, user, "admin:console") {
			return
		}
		var body struct {
			Branding map[string]any `json:"branding"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Branding == nil {
			httpx.Fail(c, 400, "缺少 branding")
			return
		}
		if err := db.Exec(
			`UPDATE tenants SET branding = ?::jsonb, updated_at = NOW() WHERE tenant_id = ?`,
			mustJSON(body.Branding), user.TenantID,
		).Error; err != nil {
			httpx.Fail(c, 500, "服务器错误")
			return
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID),
			ActorRole:  audit.P(strings.Join(user.RoleSlugs, ",")),
			ActionType: "branding_update", TargetType: audit.P("tenant"), TargetID: audit.P(user.TenantID),
			Result: "成功", Metadata: map[string]any{"branding": body.Branding},
		})
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
}
