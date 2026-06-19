// c06 knowledge-admin 验收冒烟 — PR1 foundation（PG-only，无需 MinIO）。需 docker PG 已起且已 migrate（009）。
// 直接调 internal/knowledge 服务包 + 校验迁移 009 表结构、§11.2 预置 13 库、卡片 9 字段、确定性排序、
// 创建 RBAC（kb:create）、租户/可见性隔离、置顶·权重授权。导入/检索问答/ACL 由后续 PR 的冒烟扩展。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/knowledge"
	"medoffice/server/internal/model"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/pubmed"
	"medoffice/server/internal/rag"
	"medoffice/server/internal/uploadgate"
)

func okAssert(cond bool, msg string) {
	if !cond {
		log.Fatalf("断言失败: %s", msg)
	}
	fmt.Println("  ✓", msg)
}

func tableExists(g *gorm.DB, table string) bool {
	var n int
	g.Raw(`SELECT COUNT(*)::int FROM information_schema.tables WHERE table_name = ?`, table).Scan(&n)
	return n > 0
}

func hasColumn(g *gorm.DB, table, col string) bool {
	var n int
	g.Raw(`SELECT COUNT(*)::int FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, table, col).Scan(&n)
	return n > 0
}

// §11.2 预置 13 库权威清单（与 knowledge-base spec「默认展示 13 个知识库卡片」Scenario 一字一致）。
var expected13 = []string{
	"医院制度与流程知识库", "临床指南与专家共识知识库", "药品说明书与用药安全知识库",
	"医疗质量与质控规范知识库", "感染防控知识库", "护理规范知识库",
	"医学文献与 PubMed 精选知识库", "科研项目与论文写作知识库", "医学检查检验知识库",
	"临床路径与病例资料知识库", "患者宣教知识库", "医保与病案编码知识库", "行政办公与会议资料知识库",
}

func cleanup(g *gorm.DB, tenantID string) {
	// 导入/重建/脱敏审计 + 测试文档/chunk/事件 + kb_documents（FK 级联随 knowledge_bases 删除）。
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id = ? AND action_type LIKE 'kb_%'`, tenantID)
	var docs []string
	g.Raw(`SELECT document_id FROM documents WHERE tenant_id = ? AND name LIKE 'c06-smoke%'`, tenantID).Scan(&docs)
	for _, id := range docs {
		// 先删消费记账（c03 worker 消费 manual_reindex 等事件时写入），再删 document_events，避免 FK 违约。
		g.Exec(`DELETE FROM document_event_consumptions WHERE event_id IN (SELECT event_id FROM document_events WHERE document_id = ?)`, id)
		g.Exec(`DELETE FROM document_events WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id = ?)`, id)
		g.Exec(`DELETE FROM document_chunks WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM kb_documents WHERE document_id = ?`, id)
		g.Exec(`UPDATE documents SET current_version_id = NULL WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM document_versions WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM documents WHERE document_id = ?`, id)
	}
	// 经来源适配器导入（PubMed/PMC，[20]）的 c01 文档名为文献标题（非 c06-smoke 前缀），上面的名前缀循环漏掉，
	// 故按 c06-smoke KB 关联的 kb_documents.document_id 兜底清理其 c01 文档/chunk/版本（须在删 KB 与其 kb_documents 之前）。
	var adapterDocs []string
	g.Raw(`SELECT DISTINCT kbd.document_id FROM kb_documents kbd JOIN knowledge_bases kb ON kb.kb_id = kbd.kb_id
		WHERE kb.tenant_id = ? AND kb.name LIKE 'c06-smoke%' AND kbd.document_id IS NOT NULL`, tenantID).Scan(&adapterDocs)
	for _, id := range adapterDocs {
		g.Exec(`DELETE FROM document_event_consumptions WHERE event_id IN (SELECT event_id FROM document_events WHERE document_id = ?)`, id)
		g.Exec(`DELETE FROM document_events WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id = ?)`, id)
		g.Exec(`DELETE FROM document_chunks WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM kb_documents WHERE document_id = ?`, id)
		g.Exec(`UPDATE documents SET current_version_id = NULL WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM document_versions WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM documents WHERE document_id = ?`, id)
	}
	g.Exec(`DELETE FROM kb_documents WHERE tenant_id = ? AND kb_id IN (SELECT kb_id FROM knowledge_bases WHERE tenant_id = ? AND name LIKE 'c06-smoke%')`, tenantID, tenantID)
	g.Exec(`DELETE FROM source_whitelist_rules WHERE source_identifier LIKE 'c06-smoke%'`)
	g.Exec(`DELETE FROM knowledge_bases WHERE tenant_id = ? AND name LIKE 'c06-smoke%'`, tenantID)
	// kb_qa 测试会话 + 消息 + 引用 + 最近任务 + agent 追踪（best-effort）。
	var convs []string
	g.Raw(`SELECT conversation_id FROM conversations WHERE tenant_id = ? AND title LIKE 'c06-smoke%'`, tenantID).Scan(&convs)
	for _, cid := range convs {
		g.Exec(`DELETE FROM citations WHERE message_id IN (SELECT message_id FROM messages WHERE conversation_id = ?)`, cid)
		g.Exec(`DELETE FROM recent_tasks WHERE ref_type = 'conversation' AND ref_id = ?`, cid)
		g.Exec(`DELETE FROM agent_steps WHERE run_id IN (SELECT run_id FROM agent_runs WHERE conversation_id = ?)`, cid)
		g.Exec(`DELETE FROM agent_runs WHERE conversation_id = ?`, cid)
		g.Exec(`DELETE FROM messages WHERE conversation_id = ?`, cid)
		g.Exec(`DELETE FROM conversations WHERE conversation_id = ?`, cid)
	}
	// 自愈物化计数：document_count 与 member_count 均从真值重算（修复测试残留漂移）。
	// member_count = owner ∪ 授权解析到的用户去重（与 refreshMemberCount 同口径）——预置库装载演示文档后
	// 应反映其授权成员（不再恒 0），使冒烟对历史污染稳健、且两项计数与真值一致。
	g.Exec(`UPDATE knowledge_bases SET
		document_count = (SELECT COUNT(*) FROM kb_documents kd WHERE kd.kb_id = knowledge_bases.kb_id AND kd.index_status='indexed' AND kd.is_staging=FALSE),
		member_count = (SELECT COUNT(DISTINCT uid) FROM (
			SELECT d.owner_id::text AS uid FROM kb_documents kbd JOIN documents d ON d.document_id = kbd.document_id
			  WHERE kbd.kb_id = knowledge_bases.kb_id AND kbd.is_staging = FALSE
			UNION
			SELECT u.user_id::text FROM users u WHERE u.tenant_id = knowledge_bases.tenant_id AND EXISTS (
			  SELECT 1 FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
			  WHERE kbd.kb_id = knowledge_bases.kb_id AND kbd.is_staging = FALSE AND dp.permission_level <> 'none' AND (
			    (dp.principal_type='user' AND dp.principal_id = u.user_id::text)
			    OR (dp.principal_type='role' AND dp.principal_id IN (SELECT r.slug FROM roles r JOIN user_roles ur ON ur.role_id = r.role_id WHERE ur.user_id = u.user_id))
			    OR (dp.principal_type='dept' AND dp.principal_id = u.dept_id)))
		) m)
		WHERE tenant_id = ?`, tenantID)
}

func main() {
	cfg := config.Load()
	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	var tenantID, adminID, userID string
	g.Raw(`SELECT tenant_id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	if tenantID == "" {
		log.Fatal("无租户，请先 migrate")
	}
	g.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'admin'`, tenantID).Scan(&adminID)
	g.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'user'`, tenantID).Scan(&userID)
	if adminID == "" || userID == "" {
		log.Fatal("无 admin/user 用户，请先 migrate seed")
	}
	cleanup(g, tenantID)

	admin := auth.AuthUser{UserID: adminID, TenantID: tenantID, RoleSlugs: []string{"admin"}, Permissions: []string{"kb:create", "admin:console"}}
	normal := auth.AuthUser{UserID: userID, TenantID: tenantID, RoleSlugs: []string{"user"}, Permissions: []string{"document:read"}}

	// ---- [1] 迁移 009：c06 新建三表 + 不重建他人 owner 的表 ----
	fmt.Println("\n[1] 迁移 009：knowledge_bases / kb_documents / source_whitelist_rules（owner=c06）")
	okAssert(tableExists(g, "knowledge_bases"), "knowledge_bases 表存在（owner=c06）")
	okAssert(tableExists(g, "kb_documents"), "kb_documents 表存在（owner=c06）")
	okAssert(tableExists(g, "source_whitelist_rules"), "source_whitelist_rules 表存在（owner=c06）")
	for _, col := range []string{"kb_id", "tenant_id", "name", "description", "created_by", "is_seed",
		"is_pinned", "manual_weight", "data_source", "member_count", "document_count", "created_at", "updated_at"} {
		okAssert(hasColumn(g, "knowledge_bases", col), "knowledge_bases 含字段："+col)
	}
	// §11.5.1 导入 10 必录字段槽位齐备
	for _, col := range []string{"source_url", "source_type", "imported_by", "imported_at", "copyright_status",
		"source_version", "parse_status", "index_status", "whitelist_rule_id", "authorized_by"} {
		okAssert(hasColumn(g, "kb_documents", col), "kb_documents 含 §11.5.1 必录字段："+col)
	}
	okAssert(hasColumn(g, "kb_documents", "tenant_id") && hasColumn(g, "kb_documents", "kb_id"), "kb_documents 带 tenant_id/kb_id 外键列")
	// 不重建 owner 归属他人的表：chunk_acl 列名与 c03 一致、不并存「单一 acl」
	okAssert(hasColumn(g, "document_chunks", "chunk_acl"), "chunk_acl 列由 c03 所建、列名一致（c06 仅写值）")
	okAssert(!hasColumn(g, "document_chunks", "acl"), "无「单一 acl」与 chunk_acl 并存指同一列")

	// ---- [2] 预置 13 库（§11.2）----
	fmt.Println("\n[2] 预置 13 个医疗知识库（§11.2）")
	var seedCount int
	g.Raw(`SELECT COUNT(*)::int FROM knowledge_bases WHERE tenant_id = ? AND is_seed = TRUE`, tenantID).Scan(&seedCount)
	okAssert(seedCount == 13, fmt.Sprintf("恰好 13 个 is_seed 预置库（实际 %d）", seedCount))
	for _, name := range expected13 {
		var n int
		g.Raw(`SELECT COUNT(*)::int FROM knowledge_bases WHERE tenant_id = ? AND is_seed = TRUE AND name = ?`, tenantID, name).Scan(&n)
		okAssert(n == 1, "§11.2 预置库存在："+name)
	}
	// 幂等：seed 经多次 migrate 仍无同名重复（ON CONFLICT(tenant_id,name) DO NOTHING）
	var dupName int
	g.Raw(`SELECT COUNT(*)::int FROM (
		SELECT name FROM knowledge_bases WHERE tenant_id = ? AND is_seed = TRUE GROUP BY name HAVING COUNT(*) > 1
	) d`, tenantID).Scan(&dupName)
	okAssert(dupName == 0, "预置库无同名重复（ON CONFLICT 幂等，多次 migrate 不增殖）")

	// ---- [3] 卡片 9 字段 ----
	fmt.Println("\n[3] 知识库卡片 9 字段（§11.2）")
	cards, err := knowledge.ListVisible(g, normal)
	okAssert(err == nil && len(cards) >= 13, "普通用户可见列表含 13 预置库")
	c0 := cards[0]
	okAssert(c0.KBID != "" && c0.Name != "" && c0.DataSource != "" && c0.UpdatedAt != "", "卡片含 名称/ID/数据源/更新时间")
	okAssert(c0.CreatedByName != "", "卡片含创建人（seed 库回退「系统预置」或 admin 显示名）")
	okAssert(c0.DocumentCount >= 1 && c0.MemberCount >= 1, "预置库装载演示文档后 文档数量/成员人数 物化计数>0（2.3 演示资料 + ACL 刷新；空库计数初值 0 见 [18]）")

	// ---- [4] 创建 RBAC（kb:create 锚定 c01 auth-rbac）----
	fmt.Println("\n[4] 创建知识库 RBAC（kb:create）")
	kbA, err := knowledge.Create(g, admin, "c06-smoke-A", "测试库A", "测试")
	okAssert(err == nil && kbA != "", "持 kb:create 的 admin 创建空库成功")
	_, err = knowledge.Create(g, normal, "c06-smoke-X", "越权", "测试")
	okAssert(err == knowledge.ErrForbidden, "无 kb:create 的普通用户创建被拒（ErrForbidden）")
	_, err = knowledge.Create(g, admin, "c06-smoke-A", "重名", "测试")
	okAssert(err == knowledge.ErrConflict, "同租户同名创建被拒（ErrConflict）")
	_, err = knowledge.Create(g, admin, "   ", "空名", "测试")
	okAssert(err == knowledge.ErrInvalidInput, "空名创建被拒（ErrInvalidInput）")

	// ---- [5] 确定性多级排序（§11.3 / D2）----
	fmt.Println("\n[5] 确定性多级排序：置顶 → 权重降序(NULLS LAST) → 更新时间 → 创建时间")
	kbB, _ := knowledge.Create(g, admin, "c06-smoke-B", "权重10", "测试")
	kbC, _ := knowledge.Create(g, admin, "c06-smoke-C", "权重5", "测试")
	w10, w5 := 10, 5
	okAssert(knowledge.SetRanking(g, admin, kbB, nil, &w10, false) == nil, "设 B 手动权重=10")
	okAssert(knowledge.SetRanking(g, admin, kbC, nil, &w5, false) == nil, "设 C 手动权重=5")
	pinned := true
	okAssert(knowledge.SetRanking(g, admin, kbA, &pinned, nil, false) == nil, "设 A 置顶")
	all, _ := knowledge.ListVisible(g, admin)
	posOf := func(id string) int {
		for i, c := range all {
			if c.KBID == id {
				return i
			}
		}
		return -1
	}
	pA, pB, pC := posOf(kbA), posOf(kbB), posOf(kbC)
	okAssert(pA == 0, "置顶库 A 排在最前")
	okAssert(pB < pC, "非置顶按权重降序：B(10) 在 C(5) 之前")
	// 一个带权重的库排在所有 NULL 权重 seed 库之前（NULLS LAST）
	var firstSeedPos = -1
	for i, c := range all {
		if c.IsSeed {
			firstSeedPos = i
			break
		}
	}
	okAssert(firstSeedPos > pC, "有权重的非置顶库排在无权重 seed 库之前（manual_weight NULLS LAST）")
	// 清空权重回退时间排序
	okAssert(knowledge.SetRanking(g, admin, kbB, nil, nil, true) == nil, "清空 B 权重")

	// ---- [6] 租户/可见性隔离（§11.4）----
	fmt.Println("\n[6] 终端用户可见性隔离：普通用户仅见预设库 ∪ 自建库，不见他人私有库")
	nList, _ := knowledge.ListVisible(g, normal)
	hasPrivate := false
	for _, c := range nList {
		if c.KBID == kbA || c.KBID == kbB || c.KBID == kbC {
			hasPrivate = true
		}
	}
	okAssert(!hasPrivate, "普通用户列表不含 admin 创建的私有库（c06-smoke-A/B/C）")
	_, err = knowledge.Get(g, normal, kbA)
	okAssert(err == knowledge.ErrNotFound, "普通用户取无权私有库 → ErrNotFound（不泄露存在性）")
	// 跨租户：构造异租户用户取本租户库 → 不存在
	_, err = knowledge.Get(g, auth.AuthUser{UserID: userID, TenantID: "00000000-0000-0000-0000-000000000000"}, kbA)
	okAssert(err == knowledge.ErrNotFound, "跨租户取库 → ErrNotFound（tenant_id 隔离）")

	// ---- [7] 置顶/权重授权（仅平台管理员或库创建人）----
	fmt.Println("\n[7] 排序/置顶配置授权")
	okAssert(knowledge.SetRanking(g, normal, kbA, &pinned, nil, false) == knowledge.ErrForbidden, "普通用户配置他人库排序被拒")
	okAssert(knowledge.SetRanking(g, admin, "00000000-0000-0000-0000-000000000000", &pinned, nil, false) == knowledge.ErrNotFound, "不存在的库配置 → ErrNotFound")

	// ================= PR2：受控导入管线（任务组 4·5）=================
	importKB, _ := knowledge.Create(g, admin, "c06-smoke-import", "导入测试库", "测试")
	var seedKB string
	g.Raw(`SELECT kb_id FROM knowledge_bases WHERE tenant_id = ? AND is_seed = TRUE LIMIT 1`, tenantID).Scan(&seedKB)

	authStatusOf := func(kbDocID string) (string, *string, *string) {
		var r struct {
			S    string  `gorm:"column:authorization_status"`
			Rule *string `gorm:"column:whitelist_rule_id"`
			By   *string `gorm:"column:authorized_by"`
		}
		g.Raw(`SELECT authorization_status, whitelist_rule_id, authorized_by FROM kb_documents WHERE kb_document_id = ?`, kbDocID).Scan(&r)
		return r.S, r.Rule, r.By
	}

	// ---- [8] 受控导入授权状态机（D4：来源类型默认门 + 白名单/管理员授权放行 + 三态）----
	fmt.Println("\n[8] 受控导入授权状态机（D4 三态）")
	g.Exec(`INSERT INTO source_whitelist_rules (source_identifier, is_allowed, authorization_note, scope) VALUES ('c06-smoke-allow.example.org', TRUE, '测试白名单', 'platform')`)
	dUp, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcUpload, SourceURL: "file://c06-smoke.docx", Title: "上传件"})
	s1, _, _ := authStatusOf(dUp)
	okAssert(s1 == knowledge.AuthAuthorized, "上传来源 → authorized")
	dPm, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcPubMed, SourceURL: "pmc:PMC123", Title: "PMC 文献", C04AuthHint: knowledge.AuthAuthorized})
	s2, _, _ := authStatusOf(dPm)
	okAssert(s2 == knowledge.AuthAuthorized, "PubMed/PMC（消费 c04 authorized 标记）→ authorized")
	dWl, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://c06-smoke-allow.example.org/guideline", Title: "白名单官网"})
	s3, rule3, _ := authStatusOf(dWl)
	okAssert(s3 == knowledge.AuthAuthorized && rule3 != nil && *rule3 != "", "命中白名单 URL → authorized 且写 whitelist_rule_id")
	dUnknown, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://unknown-c06.example.net/x", Title: "未知来源"})
	s4, _, _ := authStatusOf(dUnknown)
	okAssert(s4 == knowledge.AuthPreviewOnly, "未命中白名单且无管理员授权 URL → preview_only（仅临时预览）")
	dAdmin, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://unknown-c06.example.net/y", Title: "管理员授权", AdminAuthorized: true})
	s5, _, by5 := authStatusOf(dAdmin)
	okAssert(s5 == knowledge.AuthAuthorized && by5 != nil && *by5 != "", "未命中白名单但管理员显式授权 → authorized 且写 authorized_by")
	_, errRej := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://www.cnki.net/article/x", Title: "商业库"})
	okAssert(errRej == knowledge.ErrRejectedSource, "未授权商业库/镜像站 → rejected 红线阻断（不落 staging）")
	// 安全加固：白名单按规范化主机名精确/子域匹配，堵无锚点子串旁路。
	dConfuse, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://evil-c06-smoke-allow.example.org/x", Title: "混淆前缀"})
	sc, _, _ := authStatusOf(dConfuse)
	okAssert(sc == knowledge.AuthPreviewOnly, "混淆前缀 evil-<白名单域> 不命中白名单（无锚点子串旁路已堵）→ preview_only")
	dEvilSuffix, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://c06-smoke-allow.example.org.attacker.net/x", Title: "尾部拼接"})
	se, _, _ := authStatusOf(dEvilSuffix)
	okAssert(se == knowledge.AuthPreviewOnly, "尾部拼接 <白名单域>.attacker.net 不命中 → preview_only")
	dSub, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://docs.c06-smoke-allow.example.org/x", Title: "合法子域"})
	ssv, subRule, _ := authStatusOf(dSub)
	okAssert(ssv == knowledge.AuthAuthorized && subRule != nil, "白名单域真实子域 docs.<白名单域> → authorized")
	_, errMirror := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://mirror.cnki.net/x", Title: "镜像子域"})
	okAssert(errMirror == knowledge.ErrRejectedSource, "商业库镜像子域 mirror.cnki.net → rejected（子域红线）")

	// ---- [9] 上传入口权限分级（CanUploadToKB）----
	fmt.Println("\n[9] 上传入口权限分级")
	canAdmin, _ := knowledge.CanUploadToKB(g, admin, seedKB)
	okAssert(canAdmin, "平台管理员可上传到任意库（含预置库）")
	canNormal, _ := knowledge.CanUploadToKB(g, normal, seedKB)
	okAssert(!canNormal, "普通用户不得写入公共/预置库")
	_, errUp := knowledge.PreviewImport(g, normal, knowledge.ImportRequest{KBID: seedKB, SourceType: knowledge.SrcUpload, Title: "越权"})
	okAssert(errUp == knowledge.ErrForbidden, "普通用户对公共库发起导入被拒（ErrForbidden）")

	// ---- [10] staging 物理隔离 + 入库前预览确认 + 必录字段硬门禁 ----
	fmt.Println("\n[10] staging 隔离 + 入库确认 + §11.5.1 必录字段")
	var stagingFlag bool
	g.Raw(`SELECT is_staging FROM kb_documents WHERE kb_document_id = ?`, dUnknown).Scan(&stagingFlag)
	okAssert(stagingFlag, "preview_only 资料 is_staging=true（与正式库物理隔离、不进正式索引）")
	okAssert(knowledge.ConfirmImport(g, admin, dUnknown) == knowledge.ErrNotAuthorized, "preview_only 不可确认入正式库（ErrNotAuthorized）")
	okAssert(knowledge.ConfirmImport(g, admin, dPm) == nil, "authorized + 必录字段齐 → 确认入库成功")
	g.Raw(`SELECT is_staging FROM kb_documents WHERE kb_document_id = ?`, dPm).Scan(&stagingFlag)
	okAssert(!stagingFlag, "确认后 is_staging=false（入正式库）")
	// 必录字段硬门禁：人为清空 source_type 后确认被阻断
	g.Exec(`UPDATE kb_documents SET is_staging = TRUE, source_type = '' WHERE kb_document_id = ?`, dWl)
	okAssert(knowledge.ConfirmImport(g, admin, dWl) == knowledge.ErrMissingMeta, "缺非空硬门禁字段（source_type）阻断入库（ErrMissingMeta）")
	// 取消预览不落库
	okAssert(knowledge.CancelImport(g, admin, dUp) == nil, "取消预览删除 staging 行")
	var cntUp int
	g.Raw(`SELECT COUNT(*)::int FROM kb_documents WHERE kb_document_id = ?`, dUp).Scan(&cntUp)
	okAssert(cntUp == 0, "取消后无残留")

	// ---- [11] 消费 c03 索引就绪事件：置 indexed + 标记 kb chunk + 刷新 document_count（5.4a）----
	fmt.Println("\n[11] 消费 c03 索引就绪事件（唯一触发源）")
	docID, _ := makeKBDoc(g, tenantID, adminID, importKB)
	// 事件到达前 MUST NOT 自行置 indexed
	var preIdx string
	g.Raw(`SELECT index_status FROM kb_documents WHERE document_id = ?`, docID).Scan(&preIdx)
	okAssert(preIdx == "pending", "索引就绪事件到达前 index_status 仍为 pending")
	_ = knowledge.HandleIndexReady(g, parsing.IndexReadyEvent{TenantID: tenantID, DocumentID: docID, DocumentVersion: 1, ChunkCount: 2})
	var kbStatus string
	g.Raw(`SELECT index_status FROM kb_documents WHERE document_id = ?`, docID).Scan(&kbStatus)
	okAssert(kbStatus == "indexed", "消费事件后 kb_documents.index_status=indexed")
	var kbChunkCnt int
	g.Raw(`SELECT COUNT(*)::int FROM document_chunks WHERE document_id = ? AND source_type = 'kb' AND chunk_acl->>'kb_id' = ?`, docID, importKB).Scan(&kbChunkCnt)
	okAssert(kbChunkCnt == 2, "chunk 被标记 source_type=kb 且 chunk_acl.kb_id 注入（c06 仅写值不改结构）")
	var docCount int
	g.Raw(`SELECT document_count FROM knowledge_bases WHERE kb_id = ?`, importKB).Scan(&docCount)
	okAssert(docCount >= 1, "知识库 document_count 随索引就绪事件增量刷新（仅计 indexed 且非 staging）")

	// ---- [12] 管理员触发重建索引产生 manual_reindex 事件（5.4）----
	fmt.Println("\n[12] 重建索引 → manual_reindex document_events（c03 消费）")
	var kbDocOfDoc string
	g.Raw(`SELECT kb_document_id FROM kb_documents WHERE document_id = ? LIMIT 1`, docID).Scan(&kbDocOfDoc)
	okAssert(knowledge.Reindex(g, admin, kbDocOfDoc) == nil, "管理员触发重建索引成功")
	var reEvt int
	g.Raw(`SELECT COUNT(*)::int FROM document_events WHERE document_id = ? AND event_type = 'manual_reindex'`, docID).Scan(&reEvt)
	okAssert(reEvt == 1, "产生一条 event_type=manual_reindex 的 document_events（携 §10.6 契约字段，由 c03 消费）")
	var afterReindex string
	g.Raw(`SELECT index_status FROM kb_documents WHERE kb_document_id = ?`, kbDocOfDoc).Scan(&afterReindex)
	okAssert(afterReindex == "pending", "重建索引后 index_status 回退 pending（收尾走同一索引就绪路径）")

	// ---- [13] 公网导入前脱敏门禁默认拒绝（5.1/5.2，本期公网关闭）----
	fmt.Println("\n[13] 公网导入前 PHI/PII 脱敏门禁（c09 未接入 → 默认拒绝/降级）")
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id = ? AND action_type = 'kb_import_redaction_block'`, tenantID)
	_, _ = knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: importKB, SourceType: knowledge.SrcURL, SourceURL: "https://c06-smoke-allow.example.org/phi", Title: "含PHI", PublicNetwork: true})
	var redactBlock int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id = ? AND action_type = 'kb_import_redaction_block'`, tenantID).Scan(&redactBlock)
	okAssert(redactBlock >= 1, "需公网时先过 c09 门禁，默认拒绝→降级私有化/离线并留痕（不放行公网）")

	// ================= PR3 wave A：知识库问答接 c04（组 6 QA + 组 7 溯源 + 9.4）=================
	fmt.Println("\n[14] 知识库问答（§11.7 接 c04 RAG/Answer + kb_id 选择 + 溯源 + 最近任务）")
	model.Init(cfg.Model.CredentialSecret, cfg.Model.HealthTTLSeconds)
	qaSvc := aimed.NewService(rag.NewEngine(pubmed.NewService(nil, nil, false)))
	// 复刻 server 启动期全量装载：使 migrate 装载的预置库演示资料（[18] 2.3）等已持久化 indexed 文档可被检索。
	if _, err := rag.Default().IndexAllReady(g); err != nil {
		log.Fatalf("rag 全量装载: %v", err)
	}

	convID, err := knowledge.StartKBQA(g, admin, "c06-smoke-qa 术后护理")
	okAssert(err == nil && convID != "", "创建知识库问答会话")
	var convRow struct {
		Module      string `gorm:"column:module"`
		Source      string `gorm:"column:source"`
		AllowKB     bool   `gorm:"column:allow_kb"`
		AllowPubmed bool   `gorm:"column:allow_pubmed"`
	}
	g.Raw(`SELECT module, source, allow_kb, allow_pubmed FROM conversations WHERE conversation_id = ?`, convID).Scan(&convRow)
	okAssert(convRow.Module == "kb_qa" && convRow.Source == "医疗知识库问答", "会话 module=kb_qa、source=医疗知识库问答（非 AIMed，c05 可按 module 识别恢复）")
	okAssert(convRow.AllowKB && !convRow.AllowPubmed, "kb_qa 数据源仅知识库（allow_kb=true、allow_pubmed=false）")

	// importKB（来自 [11]）已有 2 条已索引 kb chunk；问答 scope 到 importKB → 命中 + 引用溯源到该库 chunk
	res, err := knowledge.AskKB(g, qaSvc, admin, convID, []string{importKB}, "术后康复护理要点")
	okAssert(err == nil && res != nil, "AskKB 返回答案")
	okAssert(res.Draft && res.Disclaimer != "", "答案默认草稿 + 携 §19.3 医疗免责声明")
	okAssert(len(res.Citations) >= 1, "答案带引用（复用 c04 citations 溯源）")
	kbCited := false
	for _, ct := range res.Citations {
		if ct.KBID == importKB && ct.ChunkID != "" {
			kbCited = true
		}
	}
	okAssert(kbCited, "引用溯源到 知识库(kb_id)+chunk 级定位（§11.8）")

	var rtCount int
	g.Raw(`SELECT COUNT(*)::int FROM recent_tasks WHERE tenant_id=? AND user_id=? AND source='医疗知识库问答' AND ref_type='conversation' AND ref_id=? AND deleted_at IS NULL`, tenantID, adminID, convID).Scan(&rtCount)
	okAssert(rtCount == 1, "问答写入最近任务（source=医疗知识库问答/ref_type=conversation/ref_id=会话，c05 可按 ref_id 恢复）")

	// kb_id 数据源选择：scope 到无资料的空库 → 无召回不臆造
	emptyKB, _ := knowledge.Create(g, admin, "c06-smoke-qa-empty", "空库", "测试")
	res2, err := knowledge.AskKB(g, qaSvc, admin, convID, []string{emptyKB}, "术后康复护理要点")
	okAssert(err == nil && res2.NoResults, "选定无资料库（kb_id 数据源选择生效）→ 无召回不臆造（NoResults）")

	// 高风险前置：高风险问题 + 无 highrisk:confirm 角色 → 需 doctor/reviewer 确认后方可下发
	res3, err := knowledge.AskKB(g, qaSvc, admin, convID, []string{importKB}, "这个药每天吃几次、用法用量与剂量是多少")
	okAssert(err == nil && res3.HighRisk && res3.RequiresConfirmation, "高风险 kb_qa 答案下发前进 c05 message 级确认（普通用户/无授权角色不可直接下发）")

	// per-user 会话隔离 + module 隔离
	_, err = knowledge.AskKB(g, qaSvc, normal, convID, []string{importKB}, "x")
	okAssert(err == knowledge.ErrNotFound, "他人 kb_qa 会话不可访问（per-user 隔离）")
	aimedConv, _ := aimed.CreateConversation(g, tenantID, adminID, aimed.ModuleAimed, "general", "c06-smoke-aimed")
	_, err = knowledge.AskKB(g, qaSvc, admin, aimedConv, []string{importKB}, "x")
	okAssert(err == knowledge.ErrInvalidInput, "AIMed 会话不可走 kb_qa 入口（module 隔离）")

	// ================= PR3 wave B：全局搜索（§11.6 三模式 + 多维筛选）=================
	fmt.Println("\n[15] 全局搜索（§11.6 三模式 + 多维筛选 + kb_id 选择 + 可见性）")
	searchEng := rag.NewEngine(pubmed.NewService(nil, nil, false))
	for _, mode := range []string{"keyword", "semantic", "hybrid"} {
		sr, serr := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", mode, knowledge.SearchFilters{})
		okAssert(serr == nil && sr.Total >= 1 && sr.Mode == mode, "搜索模式 "+mode+" 返回命中（mode 透传，importKB 已索引 chunk）")
	}
	// 非法 mode → 归一为 hybrid
	srDefault, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "bogus", knowledge.SearchFilters{})
	okAssert(srDefault.Mode == "hybrid", "非法检索模式归一为 hybrid")
	// 多维筛选：文档类型维（txt 命中 / pdf 不命中），取自 documents 文件类型字段
	srTxt, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "keyword", knowledge.SearchFilters{DocType: "txt"})
	okAssert(srTxt.Total >= 1, "按文档类型=txt 筛选命中文件类型字段")
	srPdf, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "keyword", knowledge.SearchFilters{DocType: "pdf"})
	okAssert(srPdf.Total == 0, "按文档类型=pdf 筛选（无 pdf 资料）→ 0 命中")
	// 来源维（upload 命中 / pubmed 不命中），取自 kb_documents.source_type —— 与文档类型用不同承载字段
	srUp, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "keyword", knowledge.SearchFilters{Source: "upload"})
	okAssert(srUp.Total >= 1, "按来源=upload 筛选命中来源类型字段（与文档类型维不混同）")
	srPm, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "keyword", knowledge.SearchFilters{Source: "pubmed"})
	okAssert(srPm.Total == 0, "按来源=pubmed 筛选（无 pubmed 资料）→ 0 命中")
	// kb_id 数据源选择 + 可见性裁剪
	srEmpty, _ := knowledge.KBSearch(g, searchEng, admin, []string{emptyKB}, "smoke chunk", "hybrid", knowledge.SearchFilters{})
	okAssert(srEmpty.Total == 0, "scope 到空库 → 0 命中（kb_id 数据源选择生效）")
	_, errV := knowledge.KBSearch(g, searchEng, normal, []string{importKB}, "smoke chunk", "hybrid", knowledge.SearchFilters{})
	okAssert(errV == knowledge.ErrForbidden, "普通用户搜索无权私有库被拒（可见性裁剪）")
	// 溯源元数据随命中返回（kb_id + chunk_id）
	srSrc, _ := knowledge.KBSearch(g, searchEng, admin, []string{importKB}, "smoke chunk", "hybrid", knowledge.SearchFilters{})
	hasKBMeta := false
	for _, h := range srSrc.Hits {
		if h.KBID == importKB && h.ChunkID != "" {
			hasKBMeta = true
		}
	}
	okAssert(hasKBMeta, "搜索命中携带溯源元数据（kb_id + chunk_id，§11.8）")

	// ================= PR3 wave C：知识库级 ACL（组 8）+ member_count（3.2）=================
	fmt.Println("\n[16] 知识库级 ACL（document_permissions 聚合）+ member_count（§11.4/§11.9/§19.1）")
	mkDocChunks := func(name string) string {
		docID := uuid.NewString()
		verID := uuid.NewString()
		g.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source) VALUES (?,?,?,?,'app','kb')`, docID, tenantID, adminID, name)
		g.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			VALUES (?,?,?,1,?,?,'import','c06-smoke/key',0)`, verID, docID, tenantID, uuid.NewString(), adminID)
		g.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID)
		for i := 0; i < 2; i++ {
			g.Exec(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, chunk_text, chunk_acl, superseded)
				VALUES (?,?,1,'document',?,'{"inheritedFrom":"document"}'::jsonb,FALSE)`, tenantID, docID, fmt.Sprintf("acl chunk %d", i))
		}
		return docID
	}
	importToKB := func(kbID, docID, title string) {
		kbDocID, _ := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: kbID, SourceType: knowledge.SrcUpload, SourceURL: "file://" + title, Title: title, DocumentID: docID})
		_ = knowledge.ConfirmImport(g, admin, kbDocID)
		_ = knowledge.HandleIndexReady(g, parsing.IndexReadyEvent{TenantID: tenantID, DocumentID: docID, DocumentVersion: 1, ChunkCount: 2})
	}

	// (a) 导入 seed 公共库 → 全角色 view（8.4），普通用户经角色授权可检索问答
	docSeed := mkDocChunks("c06-smoke-acl-seed.txt")
	importToKB(seedKB, docSeed, "c06-smoke-acl-seed.txt")
	var roleGrant int
	g.Raw(`SELECT COUNT(*)::int FROM document_permissions WHERE document_id=? AND principal_type='role' AND principal_id='user' AND permission_level='view'`, docSeed).Scan(&roleGrant)
	okAssert(roleGrant == 1, "seed 库导入 → 文档级 document_permissions 授予角色 view（8.4 物化 document_acl）")
	var seedMembers int
	g.Raw(`SELECT member_count FROM knowledge_bases WHERE kb_id=?`, seedKB).Scan(&seedMembers)
	okAssert(seedMembers == 2, "member_count = 授权用户去重计数（admin owner + user 经角色授权 = 2，3.2）")
	convN, _ := knowledge.StartKBQA(g, normal, "c06-smoke-acl-qa")
	resN, errN := knowledge.AskKB(g, qaSvc, normal, convN, []string{seedKB}, "acl chunk")
	seedHit := false
	if errN == nil && resN != nil {
		for _, ct := range resN.Citations {
			if ct.KBID == seedKB {
				seedHit = true
			}
		}
	}
	okAssert(seedHit, "普通用户经 seed 库角色授权可问答检索到其文档（8.6 读取侧六维过滤放行授权内容）")

	// (b) 私有库默认隔离 → 授权后可见可检索（8.2/8.3）
	privKB, _ := knowledge.Create(g, admin, "c06-smoke-acl-priv", "私有库", "测试")
	docPriv := mkDocChunks("c06-smoke-acl-priv.txt")
	importToKB(privKB, docPriv, "c06-smoke-acl-priv.txt")
	vlist, _ := knowledge.ListVisible(g, normal)
	inList := func(list []knowledge.Card, id string) bool {
		for _, c := range list {
			if c.KBID == id {
				return true
			}
		}
		return false
	}
	okAssert(!inList(vlist, privKB), "私有库默认对普通用户不可见（§11.4）")
	_, e := knowledge.Get(g, normal, privKB)
	okAssert(e == knowledge.ErrNotFound, "普通用户取无权私有库 → ErrNotFound")
	_, e = knowledge.AskKB(g, qaSvc, normal, convN, []string{privKB}, "acl chunk")
	okAssert(e == knowledge.ErrForbidden, "普通用户问答无权私有库被拒（数据源裁剪）")
	// 授予
	okAssert(knowledge.GrantKB(g, admin, privKB, "user", userID, "view") == nil, "库管理员授予普通用户 view")
	nList2, _ := knowledge.ListVisible(g, normal)
	okAssert(inList(nList2, privKB), "授权后私有库对普通用户可见（§11.4 被授权私有库）")
	resP, errP := knowledge.AskKB(g, qaSvc, normal, convN, []string{privKB}, "acl chunk")
	privHit := false
	if errP == nil && resP != nil {
		for _, ct := range resP.Citations {
			if ct.KBID == privKB {
				privHit = true
			}
		}
	}
	okAssert(privHit, "授权后普通用户可问答检索到私有库文档（document_acl 聚合放行）")
	var privMembers int
	g.Raw(`SELECT member_count FROM knowledge_bases WHERE kb_id=?`, privKB).Scan(&privMembers)
	okAssert(privMembers == 2, "私有库授权后 member_count=2（admin owner + user 授权）")

	// (c) 管理授权（8.3）：普通用户不可授予/管理他人库
	okAssert(knowledge.GrantKB(g, normal, privKB, "user", adminID, "view") == knowledge.ErrForbidden, "普通用户对他人库授予被拒（非库管理员）")
	canA, _ := knowledge.CanManageKB(g, admin, privKB)
	canN, _ := knowledge.CanManageKB(g, normal, privKB)
	okAssert(canA && !canN, "库管理判定：平台管理员/创建人可、仅持 view 的普通用户不可")
	// 8.3：普通用户越权授予被拒须写 audit_logs(失败)（绕过 UI 直接调用留痕）
	var denyAudit int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND actor_id=? AND action_type='kb_acl_grant' AND target_id=? AND result='失败'`, tenantID, userID, privKB).Scan(&denyAudit)
	okAssert(denyAudit >= 1, "普通用户越权授予被拒并写 audit_logs(失败)（8.3 绕过 UI 调用留痕）")
	// 库管理员（per-kb manage 授予）可上传到自管库（kb-import「库管理员上传自管库」）
	canUpView, _ := knowledge.CanUploadToKB(g, normal, privKB)
	okAssert(!canUpView, "仅持 view 的普通用户不可上传到该库")
	_ = knowledge.GrantKB(g, admin, privKB, "user", userID, "manage")
	canUpMgr, _ := knowledge.CanUploadToKB(g, normal, privKB)
	okAssert(canUpMgr, "授予 per-kb manage 后为库管理员、可上传到自管库")

	// ================= wave D：审计、问答日志与管理员查看（组 9）=================
	fmt.Println("\n[17] 审计 / 问答日志 / 管理员查看（组 9：9.1 导入审计 + 9.2 检索问答审计 + 9.3 管理员查看）")
	// 9.1 导入与授权行为审计留痕：含来源/授权确认人/白名单规则 ID + 红线阻断同样留痕。
	var impConfirm, impRej, impAuthBy, impRule int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_confirm' AND metadata->>'sourceType' <> ''`, tenantID).Scan(&impConfirm)
	okAssert(impConfirm >= 1, "9.1 入库确认写 audit_logs 且含来源(sourceType)")
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_rejected'`, tenantID).Scan(&impRej)
	okAssert(impRej >= 1, "9.1 被红线阻断的导入同样留痕（kb_import_rejected）")
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_preview' AND metadata->>'authorizedBy' IS NOT NULL AND metadata->>'authorizedBy' <> ''`, tenantID).Scan(&impAuthBy)
	okAssert(impAuthBy >= 1, "9.1 管理员授权导入留痕授权确认人(authorizedBy)")
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_preview' AND metadata->>'whitelistRuleId' <> ''`, tenantID).Scan(&impRule)
	okAssert(impRule >= 1, "9.1 白名单导入留痕白名单规则 ID(whitelistRuleId)")
	var rbFull int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_redaction_block' AND actor_id IS NOT NULL AND target_id IS NOT NULL AND metadata->>'sourceType' <> ''`, tenantID).Scan(&rbFull)
	okAssert(rbFull >= 1, "9.1 脱敏阻断留痕字段齐全（操作人/kb_id/来源，与 kb_import_rejected 同字段集）")

	// 9.2 检索与问答行为审计 + 生成问答日志（用户/tenant_id/所选 kb_id/查询/返回引用/时间）。
	var qaAudit, qaWithCite, searchAudit int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_qa' AND metadata->>'query' <> '' AND metadata->'kbIds' IS NOT NULL`, tenantID).Scan(&qaAudit)
	okAssert(qaAudit >= 1, "9.2 问答写入 audit_logs（含用户/tenant_id/所选 kb_id/查询/时间）")
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_qa' AND (metadata->>'citationCount')::int >= 1`, tenantID).Scan(&qaWithCite)
	okAssert(qaWithCite >= 1, "9.2 问答日志记录返回引用数（citationCount≥1）")
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_search' AND metadata->>'query' <> ''`, tenantID).Scan(&searchAudit)
	okAssert(searchAudit >= 1, "9.2 检索行为写入 audit_logs（含查询/模式/所选 kb_id/命中数）")

	// 9.3 管理员在权限范围内查看问答日志 + 对应引用来源。
	adminLogs, errAL := knowledge.ListQALogs(g, admin, "", 0)
	okAssert(errAL == nil && len(adminLogs) >= 1, "9.3 平台管理员查看全租户问答日志")
	logFields, logHasCite := false, false
	for _, lg := range adminLogs {
		if lg.Query != "" && lg.UserID != "" && lg.OccurredAt != "" {
			logFields = true
		}
		if len(lg.Citations) >= 1 {
			logHasCite = true
		}
	}
	okAssert(logFields, "9.3 问答日志条目含 用户/查询/时间")
	okAssert(logHasCite, "9.3 问答日志展示对应引用来源（citations 按 message_id 关联）")

	// 权限范围裁剪：库管理员仅见自管库相关问答（normal 已在 [16] 获 privKB manage 授予）。
	mgrLogs, errMgr := knowledge.ListQALogs(g, normal, "", 0)
	okAssert(errMgr == nil && len(mgrLogs) >= 1, "9.3 库管理员可查看自管库相关问答日志")
	mgrScoped := true
	for _, lg := range mgrLogs {
		if !containsStr(lg.KBIDs, privKB) {
			mgrScoped = false
		}
	}
	okAssert(mgrScoped, "9.3 库管理员问答日志严格裁剪到自管库（不含仅 seedKB 的问答）")

	// 非管理者（无任何库管理权）查看被拒。
	stranger := auth.AuthUser{UserID: uuid.NewString(), TenantID: tenantID, RoleSlugs: []string{"user"}, Permissions: []string{"document:read"}}
	_, errStranger := knowledge.ListQALogs(g, stranger, "", 0)
	okAssert(errStranger == knowledge.ErrForbidden, "9.3 不管理任何库的普通用户查看问答日志被拒（管理类视图隔离）")

	// kbFilter：库管理员越界按非自管库过滤被拒；平台管理员按某库过滤仅返回该库问答。
	_, errFilter := knowledge.ListQALogs(g, normal, seedKB, 0)
	okAssert(errFilter == knowledge.ErrForbidden, "9.3 库管理员按非自管库(seedKB)过滤问答日志被拒（越界裁剪）")
	filtLogs, _ := knowledge.ListQALogs(g, admin, importKB, 0)
	filtScoped := len(filtLogs) >= 1
	for _, lg := range filtLogs {
		if !containsStr(lg.KBIDs, importKB) {
			filtScoped = false
		}
	}
	okAssert(filtScoped, "9.3 按 importKB 过滤 → 仅返回含该库的问答日志")

	// 跨库越权裁剪（修复对抗审查 high）：构造一条横跨「自管 KB ∪ 非自管 KB」的问答，
	// 验证库管理员视图把非自管库的 kb_id 与引用来源裁剪掉（防跨 KB ACL 边界的源数据/存在性泄露）。
	xMgr, _ := knowledge.Create(g, admin, "c06-smoke-x-mgr", "自管库", "测试")
	xOther, _ := knowledge.Create(g, admin, "c06-smoke-x-other", "非自管库", "测试")
	importToKB(xMgr, mkDocChunks("c06-smoke-x-mgr.txt"), "c06-smoke-x-mgr.txt")
	importToKB(xOther, mkDocChunks("c06-smoke-x-other.txt"), "c06-smoke-x-other.txt")
	_ = knowledge.GrantKB(g, admin, xMgr, "user", userID, "manage") // normal 管 xMgr、不管 xOther
	convX, _ := knowledge.StartKBQA(g, admin, "c06-smoke-x-cross")
	_, _ = knowledge.AskKB(g, qaSvc, admin, convX, []string{xMgr, xOther}, "acl chunk") // admin 跨库问答
	// 平台管理员视角：该跨库日志含两库且引用确含非自管库 xOther（确认泄露面真实存在，裁剪测试才有意义）
	axLogs, _ := knowledge.ListQALogs(g, admin, "", 0)
	var adminCross *knowledge.QALogEntry
	for i := range axLogs {
		if axLogs[i].ConversationID == convX {
			adminCross = &axLogs[i]
		}
	}
	okAssert(adminCross != nil && containsStr(adminCross.KBIDs, xMgr) && containsStr(adminCross.KBIDs, xOther), "9.3 平台管理员见跨库问答日志完整 kb 集（xMgr + xOther）")
	adminOtherCite := false
	if adminCross != nil {
		for _, ct := range adminCross.Citations {
			if ct.KBID == xOther {
				adminOtherCite = true
			}
		}
	}
	okAssert(adminOtherCite, "9.3 平台管理员视图含 xOther 引用（确认跨库引用泄露面真实存在）")
	// 库管理员（仅管 xMgr）视角：同一日志的 kb 集与引用被裁剪到自管库，xOther 不泄露
	nxLogs, _ := knowledge.ListQALogs(g, normal, xMgr, 0)
	var nxCross *knowledge.QALogEntry
	for i := range nxLogs {
		if nxLogs[i].ConversationID == convX {
			nxCross = &nxLogs[i]
		}
	}
	okAssert(nxCross != nil, "9.3 库管理员可见命中其自管库(xMgr)的跨库问答日志")
	if nxCross != nil {
		okAssert(containsStr(nxCross.KBIDs, xMgr) && !containsStr(nxCross.KBIDs, xOther), "9.3 库管理员视图 kb 集裁剪到自管库（不回显非自管 xOther 存在性）")
		nxLeak := false
		for _, ct := range nxCross.Citations {
			if ct.KBID == xOther {
				nxLeak = true
			}
		}
		okAssert(!nxLeak, "9.3 库管理员视图引用来源裁剪掉非自管库 xOther（防跨库源数据泄露）")
		okAssert(nxCross.CitationCount == len(nxCross.Citations), "9.3 裁剪后 citationCount 与实际返回引用数对齐（不泄露被裁引用存在性）")
	}

	// ================= 组 2：演示资料库（2.3）+ 预置空库完整能力（2.4）=================
	fmt.Println("\n[18] 演示资料库（2.3 每库 ≥1 授权演示文档）+ 预置空库完整能力（2.4）")
	// 2.3：13 预置库各有 ≥1 份 authorized+indexed+非 staging 演示文档（demo:// 哨兵；经真实导入管线
	// PreviewImport→ConfirmImport→HandleIndexReady 装载，授权/ACL/计数均由真实服务层产出、无 seed 二次实现）
	var seedDemoKBs []string
	g.Raw(`SELECT DISTINCT kb.kb_id FROM knowledge_bases kb JOIN kb_documents kd ON kd.kb_id=kb.kb_id
		WHERE kb.tenant_id=? AND kb.is_seed=TRUE AND kd.source_url LIKE 'demo://%'
		  AND kd.authorization_status='authorized' AND kd.index_status='indexed' AND kd.is_staging=FALSE
		ORDER BY kb.kb_id`, tenantID).Scan(&seedDemoKBs)
	okAssert(len(seedDemoKBs) == 13, fmt.Sprintf("13 个预置库各有 ≥1 份 authorized+indexed 演示文档（实际 %d；c06 唯一资产装载 owner，经真实 D3/D4 管线装载）", len(seedDemoKBs)))
	// 「可被检索问答」逐库验证（非抽样）：普通用户（经全角色 view 授权）对每个预置库均能检索到演示文档
	retrievableKBs := 0
	for _, kbID := range seedDemoKBs {
		sr, e := knowledge.KBSearch(g, searchEng, normal, []string{kbID}, "演示", "keyword", knowledge.SearchFilters{})
		if e == nil && sr.Total >= 1 {
			retrievableKBs++
		}
	}
	okAssert(retrievableKBs == 13, fmt.Sprintf("13 个预置库演示文档**每库**均可被普通用户检索到（实际 %d/13，非抽样）", retrievableKBs))
	demoSeedKB := seedDemoKBs[0]
	// 文档类型筛选维对演示库有效（演示文档 name 带 .txt 扩展名 → fileExt=txt，§11.6 筛选维不被削弱）
	dTxt, _ := knowledge.KBSearch(g, searchEng, normal, []string{demoSeedKB}, "演示", "keyword", knowledge.SearchFilters{DocType: "txt"})
	okAssert(dTxt.Total >= 1, "演示库按文档类型=txt 筛选命中（演示文档带扩展名、与真实上传件一致）")
	// 问答可溯源到该库
	demoConv, _ := knowledge.StartKBQA(g, normal, "c06-smoke-demo-qa")
	demoQA, dqerr := knowledge.AskKB(g, qaSvc, normal, demoConv, []string{demoSeedKB}, "演示")
	demoCited := false
	if dqerr == nil && demoQA != nil {
		for _, ct := range demoQA.Citations {
			if ct.KBID == demoSeedKB {
				demoCited = true
			}
		}
	}
	okAssert(demoCited, "普通用户对预置库演示文档问答可溯源到该库（kb_id 引用，§11.8）")

	// 2.4：预置/新建空库仍具备完整能力（上传/导入/检索/问答/溯源/权限过滤入口不被禁用）
	emptyCapKB, _ := knowledge.Create(g, admin, "c06-smoke-empty-cap", "空库能力", "测试")
	emptyCard, ecErr := knowledge.Get(g, admin, emptyCapKB)
	okAssert(ecErr == nil && emptyCard != nil && emptyCard.DocumentCount == 0 && emptyCard.MemberCount == 0, "空库可打开、文档/成员计数初值为 0（空库 vs 演示库计数对照）")
	emptyDoc, eiErr := knowledge.PreviewImport(g, admin, knowledge.ImportRequest{KBID: emptyCapKB, SourceType: knowledge.SrcUpload, SourceURL: "file://c06-smoke-empty.txt", Title: "空库导入件"})
	okAssert(eiErr == nil && emptyDoc != "", "空库上传/导入入口就绪（可发起预览导入）")
	okAssert(knowledge.CancelImport(g, admin, emptyDoc) == nil, "空库预览可取消（不落正式库、不建索引）")
	emptySearch, esErr := knowledge.KBSearch(g, searchEng, admin, []string{emptyCapKB}, "演示", "hybrid", knowledge.SearchFilters{})
	okAssert(esErr == nil && emptySearch.Total == 0, "空库检索入口可用（无资料返回 0 命中而非禁用/报错）")
	emptyQAConv, _ := knowledge.StartKBQA(g, admin, "c06-smoke-empty-qa")
	emptyQA, eqErr := knowledge.AskKB(g, qaSvc, admin, emptyQAConv, []string{emptyCapKB}, "演示")
	okAssert(eqErr == nil && emptyQA.NoResults, "空库问答入口可用（无召回不臆造 NoResults 而非禁用）")
	// 权限过滤/管理入口对空库不被禁用（2.4 列举的「权限过滤入口」）
	canMgrEmpty, _ := knowledge.CanManageKB(g, admin, emptyCapKB)
	okAssert(canMgrEmpty, "空库权限管理入口就绪（创建人 CanManageKB=true）")
	okAssert(knowledge.GrantKB(g, admin, emptyCapKB, "role", "user", "view") == nil, "空库 ACL 授予入口可用（GrantKB 不因空库被禁用）")

	// ================= 组 4/5：本地批量上传（4.3）+ c09 上传闸阻止策略（5.2a）=================
	fmt.Println("\n[19] 本地/批量上传逐项落库（4.3）+ c09 上传闸「阻止上传」策略（5.2a）")
	upKB, _ := knowledge.Create(g, admin, "c06-smoke-upload", "上传测试库", "测试")
	// 4.3：批量逐项落库 —— N 份文档经导入管线各落一条独立 staging 入库记录
	const nBatch = 3
	items := make([]knowledge.ImportRequest, 0, nBatch)
	for i := 0; i < nBatch; i++ {
		dID := mkDocChunks(fmt.Sprintf("c06-smoke-batch-%d.txt", i))
		items = append(items, knowledge.ImportRequest{KBID: upKB, SourceType: knowledge.SrcUpload, SourceURL: "upload://batch", Title: fmt.Sprintf("批量件%d", i), DocumentID: dID})
	}
	batchRes := knowledge.BatchImport(g, admin, items)
	okAssert(len(batchRes) == nBatch, fmt.Sprintf("批量导入返回 N 条结果（N=%d）", nBatch))
	created := 0
	for _, r := range batchRes {
		if r.KBDocumentID != "" && r.Error == "" {
			created++
		}
	}
	okAssert(created == nBatch, fmt.Sprintf("一次批量 %d 份生成 %d 条独立入库记录（4.3 逐项落库 N→N）", nBatch, created))
	var stagingCnt int
	g.Raw(`SELECT COUNT(*)::int FROM kb_documents WHERE tenant_id=? AND kb_id=? AND is_staging=TRUE`, tenantID, upKB).Scan(&stagingCnt)
	okAssert(stagingCnt == nBatch, fmt.Sprintf("批量上传逐项落 N 条 staging kb_documents（各自独立 parse/index 状态，实际 %d）", stagingCnt))

	// 5.2a：c09 上传闸「阻止上传」策略 —— 命中 PHI 逐份拒绝入库 + 写 audit_logs(失败)，门禁在持久化前（与出网门禁两个独立执行点）。
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id=? AND action_type='kb_upload_blocked'`, tenantID)
	orig := uploadgate.Check
	defer func() { uploadgate.Check = orig }() // defer 还原（即便中途 panic 也复位 stub，避免污染）
	uploadgate.Check = func(filename string, buffer []byte) uploadgate.Result {
		return uploadgate.Result{Allowed: false, FailureReason: "命中身份证号/手机号（演示阻止策略）"}
	}
	blockRes, _ := knowledge.KBUpload(context.Background(), g, nil, admin, upKB, []knowledge.KBUploadFile{
		{Filename: "phi-1.txt", MimeType: "text/plain", Buffer: []byte("张三 身份证 110101...")},
		{Filename: "phi-2.txt", MimeType: "text/plain", Buffer: []byte("手机号 138...")},
	})
	uploadgate.Check = orig // 还原 stub（默认放行）；defer 兜底 panic 路径
	allBlocked := len(blockRes) == 2
	for _, r := range blockRes {
		if !r.Blocked {
			allBlocked = false
		}
	}
	okAssert(allBlocked, "含 PHI 上传在「阻止上传」策略下逐份被拒（不落盘、不建记录；store 未被触碰）")
	var blockAudit int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_upload_blocked' AND result='失败' AND failure_reason <> ''`, tenantID).Scan(&blockAudit)
	okAssert(blockAudit == 2, fmt.Sprintf("阻止上传逐份写 audit_logs(result=失败、failure_reason 非空)（实际 %d，c09 写 privacy_redaction_events，c06 仅留审计）", blockAudit))
	var afterBlock int
	g.Raw(`SELECT COUNT(*)::int FROM kb_documents WHERE tenant_id=? AND kb_id=?`, tenantID, upKB).Scan(&afterBlock)
	okAssert(afterBlock == nBatch, "被阻止的上传未新增 kb_documents（门禁在持久化前拦截，仍为批量的 N 条）")

	// ---- [20] PubMed/PMC 来源适配器（4.6）+ 离线降级（4.7）+ URL 离线降级引导上传（4.8）----
	fmt.Println("\n[20] PubMed/PMC 来源适配器（4.6）+ 离线降级（4.7）+ URL 离线降级引导上传（4.8）")
	// 公网关闭 + 仅离线缓存 provider（断网 posture）：ImportByID 经 useOnline()=false 走离线 FetchDetail。
	offlinePub := pubmed.NewService(nil, pubmed.NewOfflineProvider(), false)
	okAssert(!offlinePub.PublicEnabled(), "公网关闭 + 无 online provider → PublicEnabled()=false（4.8 判网降级前提）")
	adapter := knowledge.NewSourceAdapter(offlinePub)
	pmKB, _ := knowledge.Create(g, admin, "c06-smoke-pubmed", "PubMed 导入测试库", "PubMed")

	// 4.6/4.7：离线缓存取一篇 PubMed 文献（PMID 34567890，离线语料含）→ c04 标 authorized → 仅落 staging 预览（不自动确认）。
	pmDocID, perr := adapter.ImportFromPubMed(g, admin, pmKB, "pubmed", "34567890")
	okAssert(perr == nil && pmDocID != "", "离线缓存完成一篇 PubMed 文献预览导入（4.7 断网降级：useOnline=false→离线 FetchDetail）")
	var pmRow struct {
		SourceType       string `gorm:"column:source_type"`
		SourceIdentifier string `gorm:"column:source_identifier"`
		AuthStatus       string `gorm:"column:authorization_status"`
		IsStaging        bool   `gorm:"column:is_staging"`
	}
	g.Raw(`SELECT source_type, source_identifier, authorization_status, is_staging
		FROM kb_documents WHERE tenant_id=? AND kb_document_id=?`, tenantID, pmDocID).Scan(&pmRow)
	okAssert(pmRow.SourceType == "pubmed", "导入记录来源类型=pubmed（4.6）")
	okAssert(pmRow.SourceIdentifier == "34567890", fmt.Sprintf("导入记录含 pubmed_id 来源标识=34567890（4.6，实际 %q）", pmRow.SourceIdentifier))
	okAssert(pmRow.AuthStatus == "authorized", "授权三态初值=authorized（取自 c04 RetrievedSource.AuthStatus、不从零重建，不与 c04 漂移）")
	// 修 HIGH：预览阶段仅落 staging、不自动确认、不物化任何 c01 文档（确认前不落正式可检索内容）。
	okAssert(pmRow.IsStaging, "PubMed 导入仅落 staging、未自动确认（入库前人工预览确认 MUST）")
	var preMaterialized int
	g.Raw(`SELECT COUNT(*)::int FROM kb_documents WHERE kb_document_id=? AND document_id IS NULL`, pmDocID).Scan(&preMaterialized)
	okAssert(preMaterialized == 1, "预览阶段 document_id IS NULL —— 未物化 c01 文档/chunk（D3 物理隔离：确认前不落正式内容）")

	// 人工确认入库（ConfirmImport）→ 此时才物化 c01 文档 + chunk 并消费索引就绪。
	okAssert(knowledge.ConfirmImport(g, admin, pmDocID) == nil, "人工确认入库 ConfirmImport 成功（入库前预览确认链路）")
	var pmRow2 struct {
		DocumentID  string `gorm:"column:document_id"`
		IsStaging   bool   `gorm:"column:is_staging"`
		IndexStatus string `gorm:"column:index_status"`
	}
	g.Raw(`SELECT document_id, is_staging, index_status FROM kb_documents WHERE tenant_id=? AND kb_document_id=?`, tenantID, pmDocID).Scan(&pmRow2)
	okAssert(!pmRow2.IsStaging && pmRow2.DocumentID != "" && pmRow2.IndexStatus == "indexed",
		"确认后物化 c01 文档 + 入正式库 + 索引就绪（is_staging=false、document_id 非空、index_status=indexed）")
	// §16.3 chunk 来源元数据齐备（pubmed 路径：pubmed_id/doi/journal/year）。
	var chunkMeta struct {
		PubmedID string `gorm:"column:pubmed_id"`
		DOI      string `gorm:"column:doi"`
		Journal  string `gorm:"column:journal"`
		Year     int    `gorm:"column:year"`
	}
	g.Raw(`SELECT pubmed_id, doi, journal, year FROM document_chunks WHERE document_id=? AND superseded=FALSE LIMIT 1`, pmRow2.DocumentID).Scan(&chunkMeta)
	okAssert(chunkMeta.PubmedID == "34567890" && chunkMeta.DOI != "" && chunkMeta.Journal != "" && chunkMeta.Year == 2021,
		fmt.Sprintf("文献 chunk §16.3 来源元数据齐备（pubmed_id=%s doi=%s journal=%s year=%d）", chunkMeta.PubmedID, chunkMeta.DOI, chunkMeta.Journal, chunkMeta.Year))
	// 确认入库的文献可被检索（离线导入闭环可演示）。
	searchEng2 := rag.NewEngine(pubmed.NewService(nil, nil, false))
	srPmImp, _ := knowledge.KBSearch(g, searchEng2, admin, []string{pmKB}, "肺癌 免疫治疗", "hybrid", knowledge.SearchFilters{})
	okAssert(srPmImp.Total >= 1, "确认入库的 PubMed 文献可被检索（4.7 离线导入闭环可演示）")
	// 幂等闸：对已确认行二次 ConfirmImport 为 no-op，不退化 index_status、不新增解析作业（物化来源无真实文件、重解析必失败）。
	var jobsBefore int
	g.Raw(`SELECT COUNT(*)::int FROM document_parse_jobs WHERE document_id=?`, pmRow2.DocumentID).Scan(&jobsBefore)
	okAssert(knowledge.ConfirmImport(g, admin, pmDocID) == nil, "二次 ConfirmImport 返回 nil（已确认行幂等 no-op）")
	var idxAfter string
	var jobsAfter int
	g.Raw(`SELECT index_status FROM kb_documents WHERE kb_document_id=?`, pmDocID).Scan(&idxAfter)
	g.Raw(`SELECT COUNT(*)::int FROM document_parse_jobs WHERE document_id=?`, pmRow2.DocumentID).Scan(&jobsAfter)
	okAssert(idxAfter == "indexed" && jobsAfter == jobsBefore, "二次确认未退化 index_status=indexed、未新增解析作业（修 low 二次确认退化）")

	// 4.6 no-drift + 修 HIGH（取消无残留）：c04 标 preview_only（DOI 非 10. 前缀）→ 仅临时预览、不可确认、取消无残留。
	prevDocID, prerr := adapter.ImportFromPubMed(g, admin, pmKB, "doi", "not-a-doi-c06smoke")
	okAssert(prerr == nil && prevDocID != "", "c04 标 preview_only 的 DOI 导入落 staging（不报错）")
	var prevRow struct {
		AuthStatus string `gorm:"column:authorization_status"`
		IsStaging  bool   `gorm:"column:is_staging"`
		DocID      *string `gorm:"column:document_id"`
	}
	g.Raw(`SELECT authorization_status, is_staging, document_id FROM kb_documents WHERE tenant_id=? AND kb_document_id=?`, tenantID, prevDocID).Scan(&prevRow)
	okAssert(prevRow.AuthStatus == "preview_only" && prevRow.IsStaging && prevRow.DocID == nil,
		"c04 preview_only 标记被忠实消费（c06 不重新判为 authorized）→ 仅临时预览、未物化 c01 内容")
	okAssert(errors.Is(knowledge.ConfirmImport(g, admin, prevDocID), knowledge.ErrNotAuthorized),
		"preview_only 不可确认入正式库（仅临时预览，需管理员补授权）")
	// 取消预览 → staging 记录删除、且预览阶段未建任何 c01 文档 → 无残留（修 HIGH，对应 4.9「取消后无残留」）。
	okAssert(knowledge.CancelImport(g, admin, prevDocID) == nil, "取消预览成功")
	var prevGone int
	g.Raw(`SELECT COUNT(*)::int FROM kb_documents WHERE kb_document_id=?`, prevDocID).Scan(&prevGone)
	okAssert(prevGone == 0, "取消后 staging 记录删除、预览阶段从未物化 c01 文档/chunk → 无残留（修 HIGH 4.9）")

	// 4.8：公网不可用 → URL/白名单来源置不可用（ErrSourceOffline）+ 留痕，引导改用「批量上传已下载授权文件」。
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_source_offline'`, tenantID)
	_, offErr := adapter.ImportFromURL(g, admin, pmKB, knowledge.SrcURL, "https://c06-smoke-allow.example.org/guideline", true)
	okAssert(errors.Is(offErr, knowledge.ErrSourceOffline), "公网不可用时 URL 导入返回 ErrSourceOffline（4.8 该来源置不可用）")
	var offlineAudit int
	g.Raw(`SELECT COUNT(*)::int FROM audit_logs WHERE tenant_id=? AND action_type='kb_import_source_offline' AND result='失败'`, tenantID).Scan(&offlineAudit)
	okAssert(offlineAudit == 1, "URL 离线降级留痕 kb_import_source_offline（result=失败，引导改用批量上传）")
	// 等效入库闭环不中断：改用上传已下载授权文件（经同一导入管线）完成等效入库。
	fbDoc := mkDocChunks("c06-smoke-fallback.txt")
	fbRes := knowledge.BatchImport(g, admin, []knowledge.ImportRequest{
		{KBID: pmKB, SourceType: knowledge.SrcUpload, SourceURL: "upload://downloaded-authorized.pdf", Title: "已下载授权文件", DocumentID: fbDoc},
	})
	okAssert(len(fbRes) == 1 && fbRes[0].KBDocumentID != "" && fbRes[0].Error == "",
		"改用上传已下载授权文件完成等效入库（4.8 离线闭环不中断）")

	cleanup(g, tenantID)
	fmt.Println("\n✅ c06 冒烟（PR1 + PR2 + PR3wA 问答 + PR3wB 搜索 + PR3wC ACL + waveD 审计/问答日志 + 演示库/空库 + 批量上传/上传闸 + PubMed/URL 来源适配器）全部通过")
}

// containsStr 判断字符串切片是否含目标值（smoke 本地小工具）。
func containsStr(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}

// makeKBDoc 造一个属某 KB 的正式文档：documents(app_source=kb)+version+2 chunk（source_type=document，待索引就绪标 kb）
// + kb_documents（is_staging=false、authorized、必录字段齐、document_id 关联、index_status=pending）。返回 (documentID, kbDocumentID)。
func makeKBDoc(g *gorm.DB, tenantID, ownerID, kbID string) (string, string) {
	docID := uuid.NewString()
	verID := uuid.NewString()
	g.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source) VALUES (?,?,?,?,'app','kb')`, docID, tenantID, ownerID, "c06-smoke-kbdoc.txt")
	g.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
		VALUES (?,?,?,1,?,?,'import','c06-smoke/key',0)`, verID, docID, tenantID, uuid.NewString(), ownerID)
	g.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID)
	for i := 0; i < 2; i++ {
		g.Exec(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, chunk_text, chunk_acl, superseded)
			VALUES (?,?,1,'document',?,'{"inheritedFrom":"document"}'::jsonb,FALSE)`, tenantID, docID, fmt.Sprintf("c06 smoke chunk %d", i))
	}
	kbDocID := uuid.NewString()
	g.Exec(`INSERT INTO kb_documents (kb_document_id, tenant_id, kb_id, document_id, source_url, source_type, imported_by,
		copyright_status, source_version, parse_status, index_status, authorization_status, is_staging, title)
		VALUES (?,?,?,?,?,'upload',?,'licensed','v1','parsed','pending','authorized',FALSE,'c06-smoke-kbdoc')`,
		kbDocID, tenantID, kbID, docID, "file://c06-smoke-kbdoc.txt", ownerID)
	return docID, kbDocID
}
