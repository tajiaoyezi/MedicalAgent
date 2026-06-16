// Package chunkacl 解释 document_chunks.chunk_acl 物理列（owner=c03，下游仅消费）。
// 由 rag 检索过滤与 citation 点击定位共用同一套判定，避免两端逻辑漂移（曾出现「检索拦、定位放」的越权缺口）。
package chunkacl

import (
	"encoding/json"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
)

// Allows 判定当前用户对某 chunk 是否放行：
//   - {} 或缺省 → 继承文档级（放行）
//   - {"deny_users":[...]} 命中当前用户 → 拒绝
//   - {"allow_users":[...]} 非空且不含当前用户 → 拒绝（严于文档级）
//   - {"allow_roles":[...]} 非空且与用户角色无交集 → 拒绝
func Allows(acl map[string]any, user auth.AuthUser) bool {
	if len(acl) == 0 {
		return true
	}
	if deny, ok := acl["deny_users"].([]any); ok {
		for _, v := range deny {
			if s, _ := v.(string); s == user.UserID {
				return false
			}
		}
	}
	if allow, ok := acl["allow_users"].([]any); ok && len(allow) > 0 {
		found := false
		for _, v := range allow {
			if s, _ := v.(string); s == user.UserID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if allowRoles, ok := acl["allow_roles"].([]any); ok && len(allowRoles) > 0 {
		found := false
		for _, v := range allowRoles {
			s, _ := v.(string)
			for _, rs := range user.RoleSlugs {
				if rs == s {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Load 读取某 chunk 的 chunk_acl（按 document_chunks.id 主键 + 租户隔离）。
// 查不到（chunk 已被 supersede/删除）或解析失败时返回空 map（由 Allows 判为放行，定位侧仍受文档级 docperm 兜底）。
func Load(db *gorm.DB, tenantID, chunkID string) (map[string]any, error) {
	var raw string
	err := db.Raw(
		`SELECT chunk_acl::text FROM document_chunks WHERE id = ? AND tenant_id = ?`,
		chunkID, tenantID,
	).Scan(&raw).Error
	acl := map[string]any{}
	if err != nil {
		return acl, err
	}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &acl)
	}
	return acl, nil
}
