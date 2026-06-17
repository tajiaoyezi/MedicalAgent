package db

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

//go:embed kb_seed.json
var kbSeedJSON []byte

// kbSeedEntry 是版本化 seed 配置（kb_seed.json）的一项，对应 §11.2 的 13 个预置知识库之一。
// 清单与简介存在配置文件，不在代码中硬编码清单字符串（D1）。
type kbSeedEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	DataSource  string `json:"data_source"`
}

// SeedKnowledgeBases 幂等写入 §11.2 的 13 个 is_seed 知识库（D1）。
//
// 与 Seed 不同：Seed 在「已有租户」时整体跳过（避免重灌演示账号），而 13 库种子是平台级数据，
// 需在每次 migrate 时对每个租户幂等补齐（ON CONFLICT(tenant_id,name) DO NOTHING），
// 使既有库（已 seed 过账号但尚无知识库）也能补上 13 库，且重复执行不产生重复卡片。
// created_by 取该租户 admin 用户（无则 NULL，卡片创建人回退「系统预置」）。
func SeedKnowledgeBases(ctx context.Context, conn *pgx.Conn) error {
	var seeds []kbSeedEntry
	if err := json.Unmarshal(kbSeedJSON, &seeds); err != nil {
		return fmt.Errorf("解析 kb_seed.json: %w", err)
	}
	if len(seeds) != 13 {
		return fmt.Errorf("kb_seed.json 必须恰好 13 个知识库（§11.2），当前 %d", len(seeds))
	}

	rows, err := conn.Query(ctx, `SELECT tenant_id FROM tenants`)
	if err != nil {
		return err
	}
	var tenantIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		tenantIDs = append(tenantIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	inserted := 0
	for _, tenantID := range tenantIDs {
		// 该租户 admin 作为 created_by（仅取展示用；seed 库归平台预置）。
		var adminID *string
		var got string
		if err := conn.QueryRow(ctx,
			`SELECT u.user_id FROM users u
			 JOIN user_roles ur ON ur.user_id = u.user_id
			 JOIN roles r ON r.role_id = ur.role_id
			 WHERE u.tenant_id = $1 AND r.slug = 'admin' LIMIT 1`, tenantID,
		).Scan(&got); err == nil {
			adminID = &got
		}

		for _, s := range seeds {
			tag, err := conn.Exec(ctx,
				`INSERT INTO knowledge_bases (tenant_id, name, description, created_by, is_seed, data_source)
				 VALUES ($1, $2, $3, $4, TRUE, $5)
				 ON CONFLICT (tenant_id, name) DO NOTHING`,
				tenantID, s.Name, s.Description, adminID, s.DataSource,
			)
			if err != nil {
				return fmt.Errorf("seed 知识库 %q: %w", s.Name, err)
			}
			inserted += int(tag.RowsAffected())
		}
	}
	fmt.Printf("Seed knowledge bases: %d tenants, %d 库新增（已存在则跳过）\n", len(tenantIDs), inserted)
	return nil
}
