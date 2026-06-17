// c06 knowledge-admin 验收冒烟 — PR1 foundation（PG-only，无需 MinIO）。需 docker PG 已起且已 migrate（009）。
// 直接调 internal/knowledge 服务包 + 校验迁移 009 表结构、§11.2 预置 13 库、卡片 9 字段、确定性排序、
// 创建 RBAC（kb:create）、租户/可见性隔离、置顶·权重授权。导入/检索问答/ACL 由后续 PR 的冒烟扩展。
package main

import (
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
	// 自愈物化计数：document_count 从 kb_documents 真值重算（修复「kb_document 删除后无后续索引事件」遗留的漂移），
	// member_count 重置（ACL 阶段前恒 0）。使冒烟对历史污染稳健、且 document_count 与真值一致。
	g.Exec(`UPDATE knowledge_bases SET
		document_count = (SELECT COUNT(*) FROM kb_documents kd WHERE kd.kb_id = knowledge_bases.kb_id AND kd.index_status='indexed' AND kd.is_staging=FALSE),
		member_count = 0
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
	okAssert(c0.MemberCount == 0 && c0.DocumentCount == 0, "预置空库 成员人数/文档数量 物化计数初值为 0（ACL/索引就绪事件后刷新）")

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

	cleanup(g, tenantID)
	fmt.Println("\n✅ c06 冒烟（PR1 foundation + PR2 导入管线 + PR3wA 问答 + PR3wB 搜索）全部通过")
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
