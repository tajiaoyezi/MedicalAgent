// c05 ai-panel-recent-tasks 验收冒烟（PG-only，无需 MinIO）。需 docker PG 已起且已 migrate（008）。
// 直接调 internal/writeback 服务包 + recent_tasks 投递 SQL + writeback_confirmations 回源，逐条断言。
package main

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/config"
	"medoffice/server/internal/db"
	"medoffice/server/internal/writeback"
)

func okAssert(cond bool, msg string) {
	if !cond {
		log.Fatalf("断言失败: %s", msg)
	}
	fmt.Println("  ✓", msg)
}

func hasColumn(g *gorm.DB, table, col string) bool {
	var n int
	g.Raw(`SELECT COUNT(*)::int FROM information_schema.columns WHERE table_name = ? AND column_name = ?`, table, col).Scan(&n)
	return n > 0
}

func tableExists(g *gorm.DB, table string) bool {
	var n int
	g.Raw(`SELECT COUNT(*)::int FROM information_schema.tables WHERE table_name = ?`, table).Scan(&n)
	return n > 0
}

func makeDoc(g *gorm.DB, tenantID, ownerID, name string) (string, string) {
	docID := uuid.NewString()
	verID := uuid.NewString()
	g.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space) VALUES (?,?,?,?,'my')`, docID, tenantID, ownerID, name)
	g.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
		VALUES (?,?,?,1,?,?,'import','c05-smoke/key',0)`, verID, docID, tenantID, uuid.NewString(), ownerID)
	g.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID)
	return docID, verID
}

func cleanup(g *gorm.DB, tenantID string) {
	var docs []string
	g.Raw(`SELECT document_id FROM documents WHERE tenant_id = ? AND name LIKE 'c05-smoke%'`, tenantID).Scan(&docs)
	for _, id := range docs {
		g.Exec(`DELETE FROM writeback_confirmations WHERE subject_id = ?`, id)
		g.Exec(`DELETE FROM recent_tasks WHERE related_document_id = ?`, id)
		g.Exec(`UPDATE documents SET current_version_id = NULL WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM document_versions WHERE document_id = ?`, id)
		g.Exec(`DELETE FROM documents WHERE document_id = ?`, id)
	}
	g.Exec(`DELETE FROM writeback_confirmations WHERE tenant_id = ? AND audit_log_id IN (SELECT audit_id FROM audit_logs WHERE tenant_id = ? AND action_type LIKE 'writeback%')`, tenantID, tenantID)
	g.Exec(`DELETE FROM recent_tasks WHERE tenant_id = ? AND source = '在线文档 AI 操作'`, tenantID)
	g.Exec(`DELETE FROM audit_logs WHERE tenant_id = ? AND action_type IN ('writeback_confirm','writeback_dispatch_confirm','recent_task_upsert')`, tenantID)
}

func main() {
	cfg := config.Load()
	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db.Open: %v", err)
	}
	var tenantID, adminID string
	g.Raw(`SELECT tenant_id FROM tenants ORDER BY created_at LIMIT 1`).Scan(&tenantID)
	if tenantID == "" {
		log.Fatal("无租户，请先 migrate")
	}
	g.Raw(`SELECT user_id FROM users WHERE tenant_id = ? AND username = 'admin'`, tenantID).Scan(&adminID)
	if adminID == "" {
		log.Fatal("无 admin 用户")
	}
	cleanup(g, tenantID)

	// ---- [1] 迁移 008：writeback_confirmations 全字段 + recent_tasks 补列 ----
	fmt.Println("\n[1] 迁移 008：writeback_confirmations 表（§19.2 + doc_ai 恢复载体）/ recent_tasks 补列")
	okAssert(tableExists(g, "writeback_confirmations"), "writeback_confirmations 表存在（owner=c05）")
	for _, col := range []string{"confirmation_id", "subject_type", "subject_id", "confirmed_by", "confirmed_role",
		"confirmed_at", "confirmed_scope", "risk_type", "before_content_hash", "after_content_hash",
		"confirmation_action", "audit_log_id", "operation_type", "output_version_id"} {
		okAssert(hasColumn(g, "writeback_confirmations", col), "writeback_confirmations 含字段："+col)
	}
	for _, col := range []string{"title_preview", "status", "created_at", "related_document_id"} {
		okAssert(hasColumn(g, "recent_tasks", col), "recent_tasks 补列："+col)
	}

	// ---- [2] 权威 risk_type 分类器（owner=c05）----
	fmt.Println("\n[2] risk_type 权威分类器（高风险判定 + 角色裁决）")
	if _, high := writeback.ClassifyRisk("每日两次，每次一片，连服七天"); !high {
		log.Fatal("换措辞用药表述应判高风险")
	}
	fmt.Println("  ✓ 换措辞用药/诊疗表述判高风险（denylist 外启发式兜底）")
	if _, high := writeback.ClassifyRisk("这篇综述总结了肺癌免疫治疗研究进展"); high {
		log.Fatal("良性综述误判高风险")
	}
	fmt.Println("  ✓ 良性综述不误判")
	doctor := auth.AuthUser{UserID: adminID, TenantID: tenantID, RoleSlugs: []string{"doctor"}, Permissions: []string{"highrisk:confirm"}}
	normal := auth.AuthUser{UserID: adminID, TenantID: tenantID, RoleSlugs: []string{"member"}, Permissions: []string{"document:read"}}
	okAssert(writeback.CanConfirm(doctor) && !writeback.CanConfirm(normal), "highrisk:confirm 决定可否确认（doctor 可、普通不可）")
	okAssert(writeback.ConfirmedRole(doctor) == "doctor", "confirmed_role 取 doctor")

	// ---- [3] 默认写回策略矩阵（D3 / §9.6）----
	fmt.Println("\n[3] 默认写回策略矩阵 → c02 写回方法映射")
	type sc struct{ op, method, source, scope string }
	for _, s := range []sc{
		{writeback.OpSpanPolish, "replaceSelection", "ai_writeback", "selection"},
		{writeback.OpFullPolish, "createNewDocument", "ai_writeback", "document"},
		{writeback.OpProofread, "insertComment", "ai_writeback", "positions"},
		{writeback.OpCitation, "insertCitation", "ai_writeback", "positions"},
		{writeback.OpLayout, "applyStyle", "ai_writeback", "document"},
	} {
		st, ok := writeback.StrategyFor(s.op)
		okAssert(ok && st.BridgeMethod == s.method && st.WritebackSource == s.source && st.ImpactScope == s.scope,
			fmt.Sprintf("%s → %s / source=%s / scope=%s", s.op, s.method, s.source, s.scope))
	}
	okAssert(writeback.StrategyForCopy(writeback.OpSpanPolish).BridgeMethod == "createNewDocument", "生成副本路径走 createNewDocument 复制原文档")

	// ---- [4] 写回门禁裁决（共享 GateDocumentWrite）----
	fmt.Println("\n[4] 写回门禁裁决（权限 + 高风险角色）")
	okAssert(writeback.GateDocumentWrite("apply", true, true, false, false).Allowed, "低风险有编辑权限 apply 放行")
	okAssert(!writeback.GateDocumentWrite("apply", false, true, false, false).Allowed, "无编辑权限 apply 拒绝（仅可副本/查看）")
	d := writeback.GateDocumentWrite("apply", true, true, true, false)
	okAssert(!d.Allowed && d.Review, "高风险普通用户 apply 仅可提交审核")
	okAssert(writeback.GateDocumentWrite("apply", true, true, true, true).Allowed, "高风险授权角色 apply 放行")
	okAssert(writeback.GateDocumentWrite("copy", false, true, true, false).Allowed, "高风险普通用户可生成副本（草稿）")

	// ---- [5] document 写回确认记录（§19.2 全字段 + audit 关联 + doc_ai 恢复载体）----
	fmt.Println("\n[5] document 写回确认记录 writeback_confirmations + audit_logs 关联")
	docID, verID := makeDoc(g, tenantID, adminID, "c05-smoke-病例.docx")
	confID, err := writeback.Record(g, writeback.RecordInput{
		TenantID: tenantID, SubjectType: "document", SubjectID: docID, ConfirmedBy: adminID,
		ConfirmedRole: "doctor", ConfirmedScope: "第 2 段诊疗结论", RiskType: "用药/诊疗表述",
		BeforeHash: writeback.ContentHash("原文"), AfterHash: writeback.ContentHash("修改后"),
		Action: "apply", OperationType: writeback.OpFullPolish, OutputVersionID: verID,
		ActorRole: "doctor", AuditAction: "writeback_confirm",
	})
	okAssert(err == nil && confID != "", "落 document 确认记录")
	var rec struct {
		SubjectType   string  `gorm:"column:subject_type"`
		ConfirmedRole *string `gorm:"column:confirmed_role"`
		Action        string  `gorm:"column:confirmation_action"`
		OpType        *string `gorm:"column:operation_type"`
		OutVer        *string `gorm:"column:output_version_id"`
		Before        *string `gorm:"column:before_content_hash"`
		After         *string `gorm:"column:after_content_hash"`
		AuditID       *string `gorm:"column:audit_log_id"`
	}
	g.Raw(`SELECT subject_type, confirmed_role, confirmation_action, operation_type, output_version_id, before_content_hash, after_content_hash, audit_log_id
		FROM writeback_confirmations WHERE confirmation_id = ?`, confID).Scan(&rec)
	okAssert(rec.SubjectType == "document" && rec.ConfirmedRole != nil && *rec.ConfirmedRole == "doctor", "subject_type=document、confirmed_role=doctor")
	okAssert(rec.OpType != nil && *rec.OpType == writeback.OpFullPolish && rec.OutVer != nil && *rec.OutVer == verID, "operation_type/output_version_id 承载 §6.6 doc_ai 恢复内容")
	okAssert(rec.Before != nil && rec.After != nil && *rec.Before != *rec.After, "before/after_content_hash 分别对应写回前后内容")
	okAssert(rec.AuditID != nil && *rec.AuditID != "", "audit_log_id 关联 audit_logs")
	var auditAction string
	g.Raw(`SELECT action_type FROM audit_logs WHERE audit_id = ?`, *rec.AuditID).Scan(&auditAction)
	okAssert(auditAction == "writeback_confirm", "确认动作写入 audit_logs（action_type=writeback_confirm）")

	// ---- [6] message / translation_job 多态 subject 下发前确认（task 4.7）----
	fmt.Println("\n[6] message / translation_job 多态 subject 确认（三类产生方复用同表同链路）")
	msgID := uuid.NewString()
	mConf, _ := writeback.Record(g, writeback.RecordInput{
		TenantID: tenantID, SubjectType: "message", SubjectID: msgID, ConfirmedBy: adminID,
		ConfirmedRole: "reviewer", RiskType: "用药", Action: "dispatch", ActorRole: "reviewer", AuditAction: "writeback_dispatch_confirm",
	})
	jobID := uuid.NewString()
	jConf, _ := writeback.Record(g, writeback.RecordInput{
		TenantID: tenantID, SubjectType: "translation_job", SubjectID: jobID, ConfirmedBy: adminID,
		ConfirmedRole: "doctor", RiskType: "诊疗", Action: "dispatch", ActorRole: "doctor", AuditAction: "writeback_dispatch_confirm",
	})
	okAssert(mConf != "" && jConf != "", "message / translation_job 下发确认均落同一 writeback_confirmations 表")
	var subjCount int
	g.Raw(`SELECT COUNT(DISTINCT subject_type)::int FROM writeback_confirmations WHERE tenant_id = ? AND subject_id IN (?, ?, ?)`,
		tenantID, docID, msgID, jobID).Scan(&subjCount)
	okAssert(subjCount == 3, "document/message/translation_job 三类多态 subject 各有确认记录")

	// 普通用户提交审核（submit_review，confirmed_role 为空）
	srConf, _ := writeback.Record(g, writeback.RecordInput{
		TenantID: tenantID, SubjectType: "message", SubjectID: uuid.NewString(), ConfirmedBy: adminID,
		RiskType: "医嘱", Action: "submit_review", ActorRole: "member", AuditAction: "writeback_dispatch_confirm",
	})
	var srRole *string
	g.Raw(`SELECT confirmed_role FROM writeback_confirmations WHERE confirmation_id = ?`, srConf).Scan(&srRole)
	okAssert(srRole == nil, "普通用户提交审核记录 confirmed_role 为空（未授权角色确认）")

	// ---- [7] CHECK 约束守恒 ----
	fmt.Println("\n[7] writeback_confirmations CHECK 约束守恒")
	e1 := g.Exec(`INSERT INTO writeback_confirmations (tenant_id, subject_type, subject_id, confirmed_by, confirmation_action) VALUES (?, 'chunk', ?, ?, 'apply')`, tenantID, uuid.NewString(), adminID).Error
	okAssert(e1 != nil, "subject_type 枚举拒绝非法值（仅 document/message/translation_job）")
	e2 := g.Exec(`INSERT INTO writeback_confirmations (tenant_id, subject_type, subject_id, confirmed_by, confirmed_role, confirmation_action) VALUES (?, 'document', ?, ?, 'nurse', 'apply')`, tenantID, uuid.NewString(), adminID).Error
	okAssert(e2 != nil, "confirmed_role 枚举拒绝非 doctor/reviewer")
	e3 := g.Exec(`INSERT INTO writeback_confirmations (tenant_id, subject_type, subject_id, confirmed_by, confirmation_action, operation_type) VALUES (?, 'document', ?, ?, 'apply', '转PPT')`, tenantID, uuid.NewString(), adminID).Error
	okAssert(e3 != nil, "operation_type 枚举拒绝 V1.1 项（转PPT 等）")

	// ---- [8] doc_ai 最近任务 upsert + 恢复回源 ----
	fmt.Println("\n[8] 在线文档 AI 操作写入最近任务（ref_type=writeback_confirmation 幂等）+ 恢复回源")
	upsertDocAi := func(conf, title, preview, docID string) {
		g.Exec(`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, title_preview, status, ref_type, ref_id, related_document_id, updated_at)
			VALUES (?, ?, ?, '在线文档 AI 操作', ?, ?, 'confirmed', 'writeback_confirmation', ?, ?, NOW())
			ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
			DO UPDATE SET title = EXCLUDED.title, title_preview = EXCLUDED.title_preview, updated_at = NOW(), deleted_at = NULL`,
			uuid.NewString(), tenantID, adminID, title, preview, conf, docID)
	}
	upsertDocAi(confID, "全文润色 · c05-smoke-病例.docx", "全文润色 · c05-s", docID)
	upsertDocAi(confID, "全文润色 · c05-smoke-病例.docx", "全文润色 · c05-s", docID) // 重复投递
	var rtCount int
	g.Raw(`SELECT COUNT(*)::int FROM recent_tasks WHERE tenant_id = ? AND user_id = ? AND ref_type = 'writeback_confirmation' AND ref_id = ? AND deleted_at IS NULL`,
		tenantID, adminID, confID).Scan(&rtCount)
	okAssert(rtCount == 1, "同一 writeback_ref 重复投递幂等（不产生重复条目）")
	// 不同操作（不同 confirmation）各自独立成条
	confID2, _ := writeback.Record(g, writeback.RecordInput{TenantID: tenantID, SubjectType: "document", SubjectID: docID, ConfirmedBy: adminID, Action: "apply", OperationType: writeback.OpSpanPolish})
	upsertDocAi(confID2, "选区润色 · c05-smoke-病例.docx", "选区润色 · c05-s", docID)
	var docTasks int
	g.Raw(`SELECT COUNT(*)::int FROM recent_tasks WHERE tenant_id = ? AND related_document_id = ? AND deleted_at IS NULL`, tenantID, docID).Scan(&docTasks)
	okAssert(docTasks == 2, "同一文档多次不同操作按各自 writeback_ref 独立成条（不折叠）")
	// 恢复回源：仅凭 ref_type=writeback_confirmation 回 writeback_confirmations 取非哈希字段
	var restore struct {
		SubjectID string  `gorm:"column:subject_id"`
		OpType    *string `gorm:"column:operation_type"`
		OutVer    *string `gorm:"column:output_version_id"`
		Scope     *string `gorm:"column:confirmed_scope"`
	}
	g.Raw(`SELECT subject_id, operation_type, output_version_id, confirmed_scope FROM writeback_confirmations WHERE confirmation_id = ? AND subject_type = 'document'`, confID).Scan(&restore)
	okAssert(restore.SubjectID == docID && restore.OpType != nil && restore.OutVer != nil && restore.Scope != nil,
		"doc_ai 恢复由非哈希字段还原 document_id/操作类型/输出结果/选区")

	cleanup(g, tenantID)
	fmt.Println("\n✅ c05 冒烟全部通过")
}
