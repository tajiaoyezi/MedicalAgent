// Package audit 复刻 services/audit.ts 的 writeAudit：写 audit_logs（仅审计动作进此表，不进 document_events）。
package audit

import (
	"encoding/json"

	"gorm.io/gorm"
)

// Entry 等价 AuditEntry。可空字段用 *string（nil → NULL）。
type Entry struct {
	TenantID      string
	ActorID       *string
	ActorRole     *string
	ActionType    string
	TargetType    *string
	TargetID      *string
	Result        string // "成功" | "失败"
	FailureReason *string
	Metadata      map[string]any
}

// P 便捷构造 *string。
func P(s string) *string { return &s }

// Write 插入一条审计。db 可为根连接或事务（gorm tx 同为 *gorm.DB），与 Node writeAudit(client) 一致。
func Write(db *gorm.DB, e Entry) error {
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return db.Exec(
		`INSERT INTO audit_logs (tenant_id, actor_id, actor_role, action_type, target_type, target_id, result, failure_reason, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb)`,
		e.TenantID, e.ActorID, e.ActorRole, e.ActionType, e.TargetType, e.TargetID, e.Result, e.FailureReason, string(mb),
	).Error
}
