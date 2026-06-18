package db

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
)

//go:embed demo_docs_seed.json
var demoDocsSeedJSON []byte

// demoDocEntry 是某预置库的一份演示文档配置（demo_docs_seed.json 一项）。
type demoDocEntry struct {
	KBName string   `json:"kb_name"`
	Title  string   `json:"title"`
	Chunks []string `json:"chunks"`
}

// SeedDemoDocuments 为每个 §11.2 预置库装载 ≥1 份授权/开放来源的演示文档（c06 tasks 2.3）。
//
// c06 是这批演示文档的唯一资产装载 owner（c09 §20.4 Demo 数据集仅引用、不重复装载）。本函数物化
// c06 受控导入管线（D3/D4）的「终态」作为种子 fixture：来源 upload→authorization_status=authorized
// （等同 classifyAuthorization 的 upload 分支）、is_staging=false（确认入库）、index_status=indexed +
// chunk_acl.kb_id 注入（等同消费 c03「索引就绪」事件的终态）、对全租户角色授予 view（等同 applyImportGrants
// 的 seed 公共库分支），并刷新 document_count/member_count/updated_at。
//
// 不入 document_parse_jobs：演示内容已预切片，作为「已解析/已索引」fixture 落库，避免无 worker / 无 MinIO
// 实体对象时的悬挂解析与解析失败。导入管线本身的运行期路径由 c06 冒烟 [8]-[11] 独立验证。
//
// 幂等：以 source_url 前缀 'demo://' 为哨兵，某库已有演示文档则跳过（重复 migrate 不增殖）。与演示账号 seed
// 分离、对每租户补齐；生产环境同样需要可演示的预置库内容，故不受 NodeEnv 跳过约束（内容为开放/授权示例）。
func SeedDemoDocuments(ctx context.Context, conn *pgx.Conn) error {
	var entries []demoDocEntry
	if err := json.Unmarshal(demoDocsSeedJSON, &entries); err != nil {
		return fmt.Errorf("解析 demo_docs_seed.json: %w", err)
	}

	var tenantIDs []string
	rows, err := conn.Query(ctx, `SELECT tenant_id FROM tenants`)
	if err != nil {
		return err
	}
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

	loaded := 0
	for _, tenantID := range tenantIDs {
		var adminID string
		if err := conn.QueryRow(ctx,
			`SELECT u.user_id FROM users u
			 JOIN user_roles ur ON ur.user_id = u.user_id
			 JOIN roles r ON r.role_id = ur.role_id
			 WHERE u.tenant_id = $1 AND r.slug = 'admin' LIMIT 1`, tenantID,
		).Scan(&adminID); err != nil {
			continue // 该租户无 admin（无可作 owner/导入人的用户）→ 跳过演示装载
		}
		for _, e := range entries {
			n, err := seedOneDemoDoc(ctx, conn, tenantID, adminID, e)
			if err != nil {
				return fmt.Errorf("装载演示文档 %q@%q: %w", e.Title, e.KBName, err)
			}
			loaded += n
		}
	}
	fmt.Printf("Seed demo documents: %d tenants, %d 份演示文档新增（已存在则跳过）\n", len(tenantIDs), loaded)
	return nil
}

// seedOneDemoDoc 为单个预置库装载一份演示文档（终态）。返回新增份数（0=已存在跳过，1=新增）。
func seedOneDemoDoc(ctx context.Context, conn *pgx.Conn, tenantID, adminID string, e demoDocEntry) (int, error) {
	var kbID string
	if err := conn.QueryRow(ctx,
		`SELECT kb_id FROM knowledge_bases WHERE tenant_id = $1 AND name = $2 AND is_seed = TRUE`,
		tenantID, e.KBName,
	).Scan(&kbID); err != nil {
		return 0, nil // 预置库不存在（理论上 SeedKnowledgeBases 已补齐）→ 跳过
	}

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM kb_documents WHERE tenant_id = $1 AND kb_id = $2 AND source_url LIKE 'demo://%')`,
		tenantID, kbID,
	).Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		return 0, nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var docID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO documents (tenant_id, owner_id, name, space, app_source, mime_type)
		 VALUES ($1, $2, $3, 'app', 'kb', 'text/plain') RETURNING document_id`,
		tenantID, adminID, e.Title,
	).Scan(&docID); err != nil {
		return 0, err
	}
	var verID string
	if err := tx.QueryRow(ctx,
		`INSERT INTO document_versions (document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
		 VALUES ($1, $2, 1, $3, $4, 'import', $5, 0) RETURNING version_id`,
		docID, tenantID, "demo-"+docID, adminID, "demo/"+kbID+"/"+docID,
	).Scan(&verID); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE documents SET current_version_id = $1 WHERE document_id = $2`, verID, docID); err != nil {
		return 0, err
	}

	aclJSON := fmt.Sprintf(`{"kb_id":%q,"inheritedFrom":"document"}`, kbID)
	for i, text := range e.Chunks {
		if _, err := tx.Exec(ctx,
			`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, source_title, chunk_text, chunk_acl, section, paragraph_index, superseded)
			 VALUES ($1, $2, 1, 'kb', $3, $4, $5::jsonb, '演示资料', $6, FALSE)`,
			tenantID, docID, e.Title, text, aclJSON, i,
		); err != nil {
			return 0, err
		}
	}

	if _, err := tx.Exec(ctx,
		`INSERT INTO kb_documents (tenant_id, kb_id, document_id, source_url, source_type, imported_by, copyright_status,
		   source_version, parse_status, index_status, authorization_status, is_staging, title)
		 VALUES ($1, $2, $3, $4, 'upload', $5, 'open_or_licensed', 'v1', 'parsed', 'indexed', 'authorized', FALSE, $6)`,
		tenantID, kbID, docID, "demo://"+kbID+"/"+e.Title, adminID, e.Title,
	); err != nil {
		return 0, err
	}

	// 预置（公共）库：授予本租户全部角色 view，使演示文档对全租户可检索问答（等同 applyImportGrants seed 分支）。
	if _, err := tx.Exec(ctx,
		`INSERT INTO document_permissions (tenant_id, document_id, principal_type, principal_id, permission_level)
		 SELECT $1, $2, 'role', r.slug, 'view' FROM roles r WHERE r.tenant_id = $1
		 ON CONFLICT (document_id, principal_type, principal_id) DO NOTHING`,
		tenantID, docID,
	); err != nil {
		return 0, err
	}

	// 刷新物化计数与更新时间（仅计 indexed 且非 staging；member_count = owner ∪ 授权解析到的用户去重）。
	if _, err := tx.Exec(ctx,
		`UPDATE knowledge_bases SET
		   document_count = (SELECT COUNT(*) FROM kb_documents kd WHERE kd.kb_id = $1 AND kd.index_status = 'indexed' AND kd.is_staging = FALSE),
		   member_count = (SELECT COUNT(DISTINCT uid) FROM (
		     SELECT d.owner_id::text AS uid FROM kb_documents kbd JOIN documents d ON d.document_id = kbd.document_id
		       WHERE kbd.kb_id = $1 AND kbd.is_staging = FALSE
		     UNION
		     SELECT u.user_id::text FROM users u WHERE u.tenant_id = $2 AND EXISTS (
		       SELECT 1 FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		       WHERE kbd.kb_id = $1 AND kbd.is_staging = FALSE AND dp.permission_level <> 'none' AND (
		         (dp.principal_type='user' AND dp.principal_id = u.user_id::text)
		         OR (dp.principal_type='role' AND dp.principal_id IN (SELECT r.slug FROM roles r JOIN user_roles ur ON ur.role_id = r.role_id WHERE ur.user_id = u.user_id))
		         OR (dp.principal_type='dept' AND dp.principal_id = u.dept_id)))
		   ) m),
		   updated_at = NOW()
		 WHERE kb_id = $1 AND tenant_id = $2`,
		kbID, tenantID,
	); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return 1, nil
}
