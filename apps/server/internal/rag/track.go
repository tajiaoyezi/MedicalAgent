package rag

import (
	"encoding/json"

	"gorm.io/gorm"
)

// startRun 建一条 agent_run（一次 RAG 编排=一条 run）。失败返回空串（埋点不阻断主流程）。
func startRun(db *gorm.DB, tenantID, userID, convID, messageID string) string {
	var runID string
	err := db.Raw(
		`INSERT INTO agent_runs (tenant_id, user_id, conversation_id, message_id, status)
		 VALUES (?, ?, ?, ?, 'running') RETURNING run_id`,
		tenantID, userID, nilIfEmpty(convID), nilIfEmpty(messageID),
	).Scan(&runID).Error
	if err != nil {
		return ""
	}
	return runID
}

// recordStep 记一条 agent_step（每个管线节点一条），含输入/输出摘要与 metrics。
func recordStep(db *gorm.DB, tenantID, runID, name, in, out string, metrics map[string]any) {
	if runID == "" {
		return
	}
	mb := "{}"
	if metrics != nil {
		if b, err := json.Marshal(metrics); err == nil {
			mb = string(b)
		}
	}
	_ = db.Exec(
		`INSERT INTO agent_steps (run_id, tenant_id, step_name, input_summary, output_summary, metrics)
		 VALUES (?, ?, ?, ?, ?, ?::jsonb)`,
		runID, tenantID, name, truncate(in, 500), truncate(out, 500), mb,
	).Error
}

// RecordStep 对外暴露的步骤埋点（供 aimed 答案生成阶段记录模式识别准确率等指标，§20.3）。
func RecordStep(db *gorm.DB, tenantID, runID, name, in, out string, metrics map[string]any) {
	recordStep(db, tenantID, runID, name, in, out, metrics)
}

func endRun(db *gorm.DB, runID, status string) {
	if runID == "" {
		return
	}
	_ = db.Exec(`UPDATE agent_runs SET status = ?, ended_at = NOW() WHERE run_id = ?`, status, runID).Error
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
