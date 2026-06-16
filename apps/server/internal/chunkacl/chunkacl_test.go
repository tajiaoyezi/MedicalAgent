package chunkacl

import (
	"testing"

	"medoffice/server/internal/auth"
)

func mkUser(id string, roles ...string) auth.AuthUser {
	return auth.AuthUser{UserID: id, RoleSlugs: roles}
}

// 修复 #2 的判定核心：定位侧与检索侧共用同一套 chunk_acl 解释，保证「检索拦的、定位也拦」。
func TestAllows(t *testing.T) {
	tests := []struct {
		name string
		acl  map[string]any
		user auth.AuthUser
		want bool
	}{
		{"空 ACL 继承文档级放行", map[string]any{}, mkUser("a"), true},
		{"deny_users 命中拒绝", map[string]any{"deny_users": []any{"a"}}, mkUser("a"), false},
		{"deny_users 未命中放行", map[string]any{"deny_users": []any{"b"}}, mkUser("a"), true},
		{"allow_users 非空且不含拒绝", map[string]any{"allow_users": []any{"b"}}, mkUser("a"), false},
		{"allow_users 含放行", map[string]any{"allow_users": []any{"a"}}, mkUser("a"), true},
		{"allow_roles 无交集拒绝", map[string]any{"allow_roles": []any{"doctor"}}, mkUser("a", "user"), false},
		{"allow_roles 有交集放行", map[string]any{"allow_roles": []any{"doctor"}}, mkUser("a", "doctor"), true},
	}
	for _, tt := range tests {
		if got := Allows(tt.acl, tt.user); got != tt.want {
			t.Errorf("%s: Allows=%v want %v", tt.name, got, tt.want)
		}
	}
}
