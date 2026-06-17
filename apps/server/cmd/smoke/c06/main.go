// c06 knowledge-admin 验收冒烟 — PR1 foundation（PG-only，无需 MinIO）。需 docker PG 已起且已 migrate（009）。
// 直接调 internal/knowledge 服务包 + 校验迁移 009 表结构、§11.2 预置 13 库、卡片 9 字段、确定性排序、
// 创建 RBAC（kb:create）、租户/可见性隔离、置顶·权重授权。导入/检索问答/ACL 由后续 PR 的冒烟扩展。
package main

import (
	"fmt"
	"log"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/knowledge"
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
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id = ? AND action_type IN ('kb_create','kb_ranking_update')
		AND target_id IN (SELECT kb_id::text FROM knowledge_bases WHERE tenant_id = ? AND name LIKE 'c06-smoke%')`, tenantID, tenantID)
	g.Exec(`DELETE FROM knowledge_bases WHERE tenant_id = ? AND name LIKE 'c06-smoke%'`, tenantID)
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

	cleanup(g, tenantID)
	fmt.Println("\n✅ c06 冒烟（PR1 foundation）全部通过")
}
