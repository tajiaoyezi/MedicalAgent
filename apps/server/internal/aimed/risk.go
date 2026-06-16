package aimed

import (
	"medoffice/server/internal/auth"
	"medoffice/server/internal/writeback"
)

// risk_type 高风险判定与 writeback_confirmations 的唯一 owner 为 c05（internal/writeback）。
// c04 作为 message 级生产方，下发前前置消费 c05 的权威分类器与角色裁决（§19.2、design D1/D8）。

// ClassifyRisk 委托 c05 权威分类器判定答案是否高风险，返回 riskType（命中类别摘要）与是否高风险。
func ClassifyRisk(content string) (riskType string, high bool) {
	return writeback.ClassifyRisk(content)
}

// CanConfirmHighRisk：具备 highrisk:confirm 权限（c01 授予 doctor/reviewer）方可确认下发；
// 普通用户仅能生成草稿/提交审核（§24.7 第三项）。委托 c05。
func CanConfirmHighRisk(u auth.AuthUser) bool { return writeback.CanConfirm(u) }
