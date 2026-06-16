package writeback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"gorm.io/gorm"
)

// ContentHash 计算内容哈希（before/after_content_hash）。空串返回空，便于「无原文」的全文生成场景。
func ContentHash(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// RecordInput 落 writeback_confirmations 一条记录（§19.2 全字段 + doc_ai §6.6 恢复载体）。
type RecordInput struct {
	TenantID      string
	SubjectType   string // document | message | translation_job
	SubjectID     string
	ConfirmedBy   string
	ConfirmedRole string // doctor | reviewer | ""（非高风险确认）
	ConfirmedScope string
	RiskType      string // 高风险命中类别；非高风险为 ""
	BeforeHash    string
	AfterHash     string
	Action        string // apply | copy | submit_review | dispatch
	OperationType string // doc_ai 操作类型；message/translation_job 下发可为 ""
	OutputVersionID string // 写回所落 document_versions；确认时若未知可为 ""
	// 审计字段
	ActorRole string
	AuditAction string         // audit_logs.action_type
	AuditMeta   map[string]any // audit_logs.metadata 附加
}

// Record 在单事务内写 audit_logs（取 audit_id）+ writeback_confirmations（以 audit_log_id 关联），返回 confirmation_id。
// 确认动作进 audit_logs（不进 document_events——文档写回事件由 c02 经保存回调唯一产生 ai_writeback）。
func Record(db *gorm.DB, in RecordInput) (string, error) {
	var confirmationID string
	err := db.Transaction(func(tx *gorm.DB) error {
		meta := in.AuditMeta
		if meta == nil {
			meta = map[string]any{}
		}
		meta["subjectType"] = in.SubjectType
		meta["subjectId"] = in.SubjectID
		meta["confirmationAction"] = in.Action
		if in.RiskType != "" {
			meta["riskType"] = in.RiskType
		}
		mb, _ := json.Marshal(meta)
		auditAction := in.AuditAction
		if auditAction == "" {
			auditAction = "writeback_confirm"
		}
		var auditID string
		if err := tx.Raw(
			`INSERT INTO audit_logs (tenant_id, actor_id, actor_role, action_type, target_type, target_id, result, metadata)
			 VALUES (?, ?, ?, ?, ?, ?, '成功', ?::jsonb) RETURNING audit_id`,
			in.TenantID, in.ConfirmedBy, nilIfEmpty(in.ActorRole), auditAction, in.SubjectType, in.SubjectID, string(mb),
		).Scan(&auditID).Error; err != nil {
			return err
		}
		if err := tx.Raw(
			`INSERT INTO writeback_confirmations
			   (tenant_id, subject_type, subject_id, confirmed_by, confirmed_role, confirmed_scope,
			    risk_type, before_content_hash, after_content_hash, confirmation_action,
			    audit_log_id, operation_type, output_version_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 RETURNING confirmation_id`,
			in.TenantID, in.SubjectType, in.SubjectID, in.ConfirmedBy, nilIfEmpty(in.ConfirmedRole), nilIfEmpty(in.ConfirmedScope),
			nilIfEmpty(in.RiskType), nilIfEmpty(in.BeforeHash), nilIfEmpty(in.AfterHash), in.Action,
			auditID, nilIfEmpty(in.OperationType), nilIfEmpty(in.OutputVersionID),
		).Scan(&confirmationID).Error; err != nil {
			return err
		}
		return nil
	})
	return confirmationID, err
}

// SetOutputVersion 在写回经 c02 保存回调落版本后，回填 output_version_id（确认时版本尚未生成的异步补登）。
func SetOutputVersion(db *gorm.DB, tenantID, confirmationID, versionID string) error {
	return db.Exec(
		`UPDATE writeback_confirmations SET output_version_id = ? WHERE confirmation_id = ? AND tenant_id = ?`,
		versionID, confirmationID, tenantID,
	).Error
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
