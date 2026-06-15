package aimed

import (
	"strings"

	"medoffice/server/internal/auth"
)

// 高风险信号词（诊疗/用药/医嘱/临床文书/患者个体信息，§19.2）。
// 注：risk_type 高风险判定与 writeback_confirmations 的唯一 owner 为 c05 服务端；
// 本期 c05 未落地，此处仅为 message 级生产方的前置消费 seam（轻量判定），c05 落地后由其权威分类器替换。
var highRiskSignals = []string{
	"诊断", "诊疗", "治疗方案", "用药", "处方", "剂量", "mg", "医嘱", "手术方案", "化疗",
	"放疗", "病危", "抢救", "禁忌", "不良反应", "住院号", "身份证", "联系方式", "确诊",
	"建议服用", "建议使用", "推荐剂量", "用法用量",
}

// ClassifyRisk 判定答案是否高风险，返回 riskType（命中类别摘要）与是否高风险。
func ClassifyRisk(content string) (riskType string, high bool) {
	lower := strings.ToLower(content)
	var hits []string
	for _, kw := range highRiskSignals {
		if strings.Contains(lower, strings.ToLower(kw)) {
			hits = append(hits, kw)
			if len(hits) >= 3 {
				break
			}
		}
	}
	if len(hits) == 0 {
		return "", false
	}
	return strings.Join(hits, "/"), true
}

// CanConfirmHighRisk：具备 highrisk:confirm 权限（c01 授予 doctor/reviewer）方可确认下发；
// 普通用户仅能生成草稿/提交审核（§24.7 第三项）。
func CanConfirmHighRisk(u auth.AuthUser) bool { return u.HasPermission("highrisk:confirm") }
