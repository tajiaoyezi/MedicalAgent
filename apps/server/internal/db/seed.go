package db

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Seed 复刻 migrate.ts seedIfNeeded：值逐字对齐（demo 租户 + 10 权限 + 5 角色 + grants + admin/user 两用户）。
func Seed(ctx context.Context, conn *pgx.Conn) error {
	var existing string
	if err := conn.QueryRow(ctx, `SELECT tenant_id FROM tenants LIMIT 1`).Scan(&existing); err == nil {
		fmt.Println("Seed data already exists, skipping")
		return nil
	}

	enabledModules, _ := json.Marshal([]string{"aimed", "knowledge", "translation", "templates", "documents", "admin"})
	branding, _ := json.Marshal(map[string]any{
		"logo_url":         nil,
		"primary_color":    "#1677ff",
		"secondary_color":  "#69b1ff",
		"login_background": nil,
		"nav_style":        "default",
		"button_radius":    "6px",
		"font_size":        "14px",
		"default_theme":    "blue-white",
	})

	var tenantID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO tenants (name, org_type, enabled_modules, branding)
		 VALUES ($1, $2, $3::jsonb, $4::jsonb) RETURNING tenant_id`,
		"MedOffice 演示医院", "hospital", string(enabledModules), string(branding),
	).Scan(&tenantID); err != nil {
		return err
	}

	perms := []struct{ name, desc string }{
		{"document:read", "读取文档"},
		{"document:write", "写入文档"},
		{"document:share", "分享文档"},
		{"user:manage", "用户管理"},
		{"audit:view", "查看审计"},
		{"admin:console", "管理后台"},
		{"highrisk:confirm", "高风险确认"},
		{"template:manage", "模板管理"},
		{"kb:create", "创建知识库"},
		{"model:manage", "模型与评测管理"},
	}
	permIDs := map[string]string{}
	for _, p := range perms {
		var id string
		if err := conn.QueryRow(ctx,
			`INSERT INTO permissions (name, description) VALUES ($1, $2)
			 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description
			 RETURNING permission_id`,
			p.name, p.desc,
		).Scan(&id); err != nil {
			return err
		}
		permIDs[p.name] = id
	}

	roleDefs := []struct {
		slug, name string
		perms      []string
	}{
		{"admin", "管理员", []string{"document:read", "document:write", "document:share", "user:manage", "audit:view", "admin:console", "template:manage", "kb:create", "model:manage"}},
		{"user", "普通用户", []string{"document:read", "document:write"}},
		{"dept", "科室", []string{"document:read", "document:write"}},
		{"doctor", "医生", []string{"document:read", "document:write", "highrisk:confirm"}},
		{"reviewer", "授权审核", []string{"document:read", "document:write", "highrisk:confirm"}},
	}
	roleIDs := map[string]string{}
	for _, r := range roleDefs {
		var id string
		if err := conn.QueryRow(ctx,
			`INSERT INTO roles (tenant_id, name, slug) VALUES ($1, $2, $3) RETURNING role_id`,
			tenantID, r.name, r.slug,
		).Scan(&id); err != nil {
			return err
		}
		roleIDs[r.slug] = id
		for _, perm := range r.perms {
			if _, err := conn.Exec(ctx,
				`INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				id, permIDs[perm],
			); err != nil {
				return err
			}
		}
	}

	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin123"), 10)
	if err != nil {
		return err
	}
	userHash, err := bcrypt.GenerateFromPassword([]byte("user123"), 10)
	if err != nil {
		return err
	}

	var adminID, userID string
	if err := conn.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
		 VALUES ($1, 'admin', $2, '演示管理员', 'dept-demo') RETURNING user_id`,
		tenantID, string(adminHash),
	).Scan(&adminID); err != nil {
		return err
	}
	if err := conn.QueryRow(ctx,
		`INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
		 VALUES ($1, 'user', $2, '演示用户', 'dept-demo') RETURNING user_id`,
		tenantID, string(userHash),
	).Scan(&userID); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, adminID, roleIDs["admin"]); err != nil {
		return err
	}
	if _, err := conn.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleIDs["user"]); err != nil {
		return err
	}

	fmt.Println("Seed data inserted (admin/admin123, user/user123)")
	return nil
}
