package knowledge

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/parsing"
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
// c06 是这批演示文档的唯一资产装载 owner（c09 §20.4 Demo 数据集仅引用、不重复装载）。本函数**经 c06 受控
// 导入管线 D3/D4 真实装载**（design 步骤2 / Open Questions「必须经 D3/D4 管线且标 authorized」）：对每库
// 调 PreviewImport（来源适配器→staging→授权闸门：upload 源经 classifyAuthorization 判 authorized）→
// ConfirmImport（授权闸门→入库：metaComplete 硬门禁 + applyImportGrants 公共库全角色 view）→
// HandleIndexReady（消费 c03 索引就绪终态：标 kb chunk + chunk_acl.kb_id + index_status=indexed + 刷新
// document_count/member_count）。授权三态、ACL 传播、计数刷新一律复用真实服务层，不在 seed 二次实现（无漂移）。
//
// 唯一的 fixture 化：演示内容已预切片直接落 document_chunks（无真实文件可解析），并在 ConfirmImport 入队的解析
// 作业上直接删除（pre-parsed fixture，避免无 worker/无 MinIO 实体对象时的悬挂解析/解析失败）。语义召回依赖真实
// embedding（由解析 worker 产出），演示 fixture 不含向量——keyword/hybrid 检索与问答正常，纯 semantic 模式
// 演示 chunk 向量分为 0（已知限制，不影响 2.3「可被检索问答」的默认 hybrid 路径）。
//
// 幂等：以 source_url 前缀 'demo://' 为哨兵，某库已有演示文档则跳过（重复 migrate 不增殖）。对每个租户补齐；
// 无 admin 的租户跳过（无可作 owner/导入人的平台管理员）。返回新增份数。
func SeedDemoDocuments(db *gorm.DB) (int, error) {
	var entries []demoDocEntry
	if err := json.Unmarshal(demoDocsSeedJSON, &entries); err != nil {
		return 0, fmt.Errorf("解析 demo_docs_seed.json: %w", err)
	}

	var tenantIDs []string
	if err := db.Raw(`SELECT tenant_id FROM tenants`).Scan(&tenantIDs).Error; err != nil {
		return 0, err
	}

	loaded := 0
	for _, tenantID := range tenantIDs {
		var adminID string
		db.Raw(`SELECT u.user_id FROM users u
			JOIN user_roles ur ON ur.user_id = u.user_id
			JOIN roles r ON r.role_id = ur.role_id
			WHERE u.tenant_id = ? AND r.slug = 'admin' LIMIT 1`, tenantID).Scan(&adminID)
		if adminID == "" {
			continue // 无平台管理员 → 无可作 owner/导入人的用户，跳过该租户
		}
		// 平台管理员身份（PreviewImport/ConfirmImport 的 CanUploadToKB 经 admin:console 判定可上传任意库）。
		admin := auth.AuthUser{UserID: adminID, TenantID: tenantID, RoleSlugs: []string{"admin"}, Permissions: []string{"admin:console", "kb:create"}}
		for _, e := range entries {
			n, err := seedOneDemoDoc(db, admin, e)
			if err != nil {
				return loaded, fmt.Errorf("装载演示文档 %q@%q: %w", e.Title, e.KBName, err)
			}
			loaded += n
		}
	}
	fmt.Printf("Seed demo documents: %d tenants, %d 份演示文档新增（已存在则跳过）\n", len(tenantIDs), loaded)
	return loaded, nil
}

// seedOneDemoDoc 经真实导入管线为单个预置库装载一份演示文档。返回新增份数（0=已存在跳过，1=新增）。
func seedOneDemoDoc(db *gorm.DB, admin auth.AuthUser, e demoDocEntry) (int, error) {
	var kbID string
	db.Raw(`SELECT kb_id FROM knowledge_bases WHERE tenant_id = ? AND name = ? AND is_seed = TRUE`, admin.TenantID, e.KBName).Scan(&kbID)
	if kbID == "" {
		return 0, nil // 预置库不存在（理论上 SeedKnowledgeBases 已补齐）→ 跳过
	}
	// 幂等哨兵要求「已完整装载」（非 staging + 已索引）：若上次 migrate 在 ConfirmImport/HandleIndexReady 前
	// 中断、残留半截 staging demo:// 行，本次重跑会重新装载一份完整演示文档（旧 staging 行 is_staging=TRUE，
	// 不计入检索/计数，无害），而非永久卡在未确认态。
	var done bool
	db.Raw(`SELECT EXISTS(SELECT 1 FROM kb_documents WHERE tenant_id = ? AND kb_id = ? AND source_url LIKE 'demo://%'
		AND is_staging = FALSE AND index_status = 'indexed')`, admin.TenantID, kbID).Scan(&done)
	if done {
		return 0, nil
	}

	docID := uuid.NewString()
	sourceURL := "demo://" + kbID + "/" + e.Title

	// 1) 落「已解析」产物（文档 + 版本 + chunk）。无真实文件可解析，故 fixture 直接预切片，
	//    chunk 初始 source_type='document'（待 HandleIndexReady 翻为 kb）、chunk_acl 含 entries（与真实管线产出同构）。
	err := db.Transaction(func(tx *gorm.DB) error {
		verID := uuid.NewString()
		// documents.name 带 .txt 扩展名以与真实上传件一致（§11.6 文档类型筛选经 fileExt 取扩展名）。
		if err := tx.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
			VALUES (?, ?, ?, ?, 'app', 'kb', 'text/plain')`, docID, admin.TenantID, admin.UserID, e.Title+".txt").Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			VALUES (?, ?, ?, 1, ?, ?, 'import', ?, 0)`, verID, docID, admin.TenantID, "demo-"+docID, admin.UserID, "demo/"+kbID+"/"+docID).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID).Error; err != nil {
			return err
		}
		for i, text := range e.Chunks {
			if err := tx.Exec(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, source_title, chunk_text, chunk_acl, section, paragraph_index, superseded)
				VALUES (?, ?, 1, 'document', ?, ?, '{"inheritedFrom":"document","entries":[]}'::jsonb, '演示资料', ?, FALSE)`,
				admin.TenantID, docID, e.Title, text, i).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	// 2) 经真实导入管线：来源适配器→staging→授权闸门（upload→authorized）。
	kbDocID, err := PreviewImport(db, admin, ImportRequest{
		KBID: kbID, SourceType: SrcUpload, SourceURL: sourceURL, Title: e.Title, DocumentID: docID,
	})
	if err != nil {
		return 0, fmt.Errorf("PreviewImport: %w", err)
	}
	// 3) 授权闸门→入库（metaComplete 硬门禁 + applyImportGrants 公共库全角色 view + 刷新 member_count + 入队解析）。
	if err := ConfirmImport(db, admin, kbDocID); err != nil {
		return 0, fmt.Errorf("ConfirmImport: %w", err)
	}
	// 4) 消费 c03 索引就绪终态（标 kb chunk + chunk_acl.kb_id + index_status=indexed + 刷新 document_count）。
	if err := HandleIndexReady(db, parsing.IndexReadyEvent{TenantID: admin.TenantID, DocumentID: docID, DocumentVersion: 1, ChunkCount: len(e.Chunks)}); err != nil {
		return 0, fmt.Errorf("HandleIndexReady: %w", err)
	}
	// 删除 ConfirmImport 入队的解析作业：fixture 已预切片，无真实文件需解析（避免 worker 拉取不存在的 MinIO 对象而失败）。
	db.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, docID)
	return 1, nil
}
