package aimed

import (
	"regexp"
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

// 词表外的启发式兜底：捕捉换一种措辞即可绕过 denylist 的用药/诊疗表述（如「每日两次、每次一片、连服七天」「首剂加倍」）。
// 每条都按低误报设计（强临床信号）；宁可对边缘内容保守误判多走人工确认，也不漏判高风险（医疗红线）。
var highRiskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\d+\s*(mg|µg|ug|mcg|iu|ml)\b`),                                                            // 数字 + 西药单位
	regexp.MustCompile(`(?i)\b(qd|bid|tid|qid|qn|qod|q\d+h|prn)\b`),                                                   // 临床给药频次缩写
	regexp.MustCompile(`口服|静脉滴注|静脉注射|静滴|肌肉注射|肌注|皮下注射|舌下含服|雾化吸入|灌肠|栓剂`),                                  // 给药途径
	regexp.MustCompile(`每[日天]\s*[0-9一二三四五六七八九十两]+\s*次|一日\s*[0-9一二三四五六七八九十两]+\s*次|每\s*\d+\s*小时|首剂加倍|连服\s*[0-9一二三四五六七八九十两]+|顿服`), // 用法用量
}

// 频次提示 + 药物单位的组合（任一单独均易误报，组合才判高风险，覆盖「每次一片」这类中文数量表述）。
var (
	highRiskFreqHint = regexp.MustCompile(`每日|每天|每次|一日|每隔|睡前|空腹|餐[前后中]`)
	highRiskDoseUnit = regexp.MustCompile(`[0-9一二三四五六七八九十两]+\s*(片|粒|丸|滴|袋|支|毫克|毫升)`)
)

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
		for _, re := range highRiskPatterns {
			if re.MatchString(content) {
				return "用药/诊疗表述", true
			}
		}
		if highRiskFreqHint.MatchString(content) && highRiskDoseUnit.MatchString(content) {
			return "用药/诊疗表述", true
		}
		return "", false
	}
	return strings.Join(hits, "/"), true
}

// CanConfirmHighRisk：具备 highrisk:confirm 权限（c01 授予 doctor/reviewer）方可确认下发；
// 普通用户仅能生成草稿/提交审核（§24.7 第三项）。
func CanConfirmHighRisk(u auth.AuthUser) bool { return u.HasPermission("highrisk:confirm") }
