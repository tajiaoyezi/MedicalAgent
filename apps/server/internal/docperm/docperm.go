// Package docperm 复刻 services/document-permissions.ts：权限级联 owner/manage/edit/comment/view/none 与各 can* 判定。
package docperm

import (
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
)

type Level string

const (
	None    Level = "none"
	View    Level = "view"
	Comment Level = "comment"
	Edit    Level = "edit"
	Manage  Level = "manage"
	Owner   Level = "owner"
)

var order = map[Level]int{None: 0, View: 1, Comment: 2, Edit: 3, Manage: 4, Owner: 5}

func Max(levels []Level) Level {
	best := None
	for _, l := range levels {
		if order[l] > order[best] {
			best = l
		}
	}
	return best
}

func CanDownload(l Level) bool          { return l == Owner || l == Manage || l == Edit || l == View }
func CanEdit(l Level) bool              { return l == Owner || l == Manage || l == Edit }
func CanManagePermissions(l Level) bool { return l == Owner || l == Manage }
func CanShare(l Level) bool             { return l == Owner || l == Manage }
func CanComment(l Level) bool           { return l == Owner || l == Manage || l == Edit || l == Comment }
func CanCopy(l Level) bool              { return CanComment(l) } // §10.4

// DocumentRow 覆盖 documents 表全部列（SELECT * 扫描 + 列表输出）。
type DocumentRow struct {
	DocumentID       string    `gorm:"column:document_id" json:"document_id"`
	TenantID         string    `gorm:"column:tenant_id" json:"tenant_id"`
	OwnerID          string    `gorm:"column:owner_id" json:"owner_id"`
	Name             string    `gorm:"column:name" json:"name"`
	Space            string    `gorm:"column:space" json:"space"`
	AppSource        *string   `gorm:"column:app_source" json:"app_source"`
	MimeType         *string   `gorm:"column:mime_type" json:"mime_type"`
	IsDeleted        bool      `gorm:"column:is_deleted" json:"is_deleted"`
	IsFavorited      bool      `gorm:"column:is_favorited" json:"is_favorited"`
	CurrentVersionID *string   `gorm:"column:current_version_id" json:"current_version_id"`
	CreatedAt        time.Time `gorm:"column:created_at" json:"created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at" json:"updated_at"`
}

// Resolve 复刻 resolveEffectivePermission：跨租户→none；owner/用户/角色/科室授权取最大级。
func Resolve(db *gorm.DB, user auth.AuthUser, doc DocumentRow) (Level, error) {
	if doc.TenantID != user.TenantID {
		return None, nil
	}
	var levels []Level
	if doc.OwnerID == user.UserID {
		levels = append(levels, Owner)
	}

	var rows []struct {
		PrincipalType   string `gorm:"column:principal_type"`
		PrincipalID     string `gorm:"column:principal_id"`
		PermissionLevel string `gorm:"column:permission_level"`
	}
	if err := db.Raw(
		`SELECT principal_type, principal_id, permission_level
		 FROM document_permissions WHERE document_id = ? AND tenant_id = ?`,
		doc.DocumentID, user.TenantID,
	).Scan(&rows).Error; err != nil {
		return None, err
	}
	for _, r := range rows {
		lvl := Level(r.PermissionLevel)
		if r.PrincipalType == "user" && r.PrincipalID == user.UserID {
			levels = append(levels, lvl)
		}
		if r.PrincipalType == "role" && contains(user.RoleSlugs, r.PrincipalID) {
			levels = append(levels, lvl)
		}
		if r.PrincipalType == "dept" && user.DeptID != "" && r.PrincipalID == user.DeptID {
			levels = append(levels, lvl)
		}
	}
	if len(levels) == 0 && doc.Space == "my" && doc.OwnerID == user.UserID {
		levels = append(levels, Owner)
	}
	if len(levels) == 0 {
		return None, nil
	}
	return Max(levels), nil
}

func contains(s []string, x string) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}
