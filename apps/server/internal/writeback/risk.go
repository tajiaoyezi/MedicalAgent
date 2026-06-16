// Package writeback 是 AI 写回确认网关与 risk_type 分类器的唯一 owner（c05）。
// writeback_confirmations 记录与高风险角色裁决在本包服务端落地、不可前端篡改；
// c09 安全合规为引用式收口（仅消费本包判定与记录做统一验收/审计，不重复实现分类拦截器）。
// aimed（c04）等 message 级生产方下发前前置消费本包的 ClassifyRisk / CanConfirm。
package writeback

import (
	"regexp"
	"strings"

	"medoffice/server/internal/auth"
)

// 高风险信号词（诊疗/用药/医嘱/临床文书/患者个体信息，§19.2）。
var highRiskSignals = []string{
	"诊断", "诊疗", "治疗方案", "用药", "处方", "剂量", "mg", "医嘱", "手术方案", "化疗",
	"放疗", "病危", "抢救", "禁忌", "不良反应", "住院号", "身份证", "联系方式", "确诊",
	"建议服用", "建议使用", "推荐剂量", "用法用量",
}

// 词表外的启发式兜底：捕捉换一种措辞即可绕过 denylist 的用药/诊疗表述（如「每日两次、连服七天」「首剂加倍」「500mg 口服」）。
// 仅保留**低误报的强临床信号**——刻意不收英文给药缩写（qd/bid/prn 等会与普通英文词/复述的 PubMed 摘要碰撞）、
// 也不收「频次词+通用量词（片/支/袋）」组合（每天吃一片面包、一支队伍 等良性文本会误报）。
var highRiskPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\d+\s*(mg|µg|ug|mcg|iu|ml)\b`),                                                            // 数字 + 西药单位
	regexp.MustCompile(`口服|静脉滴注|静脉注射|静滴|肌肉注射|肌注|皮下注射|舌下含服|雾化吸入|灌肠|栓剂`),                                  // 给药途径
	regexp.MustCompile(`每[日天]\s*[0-9一二三四五六七八九十两]+\s*次|一日\s*[0-9一二三四五六七八九十两]+\s*次|每\s*\d+\s*小时|首剂加倍|连服\s*[0-9一二三四五六七八九十两]+|顿服`), // 用法用量
}

// ClassifyRisk 判定内容是否高风险，返回 riskType（命中类别摘要）与是否高风险。
// 这是 c05 的权威分类器：文档写回、AIMed 答案、知识库问答、医学翻译文书下发前均经此判定。
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
		return "", false
	}
	return strings.Join(hits, "/"), true
}

// CanConfirm：具备 highrisk:confirm 权限（c01 授予 doctor/reviewer）方可最终确认/下发高风险内容；
// 普通用户仅能生成草稿/提交审核（§19.2、§24.7）。
func CanConfirm(u auth.AuthUser) bool { return u.HasPermission("highrisk:confirm") }

// ConfirmedRole 取用户的高风险确认角色（doctor 优先于 reviewer），用于落 writeback_confirmations.confirmed_role。
// 仅在 CanConfirm(u) 为真时有意义；无授权角色返回 ""。
func ConfirmedRole(u auth.AuthUser) string {
	for _, r := range u.RoleSlugs {
		if r == "doctor" {
			return "doctor"
		}
	}
	for _, r := range u.RoleSlugs {
		if r == "reviewer" {
			return "reviewer"
		}
	}
	return ""
}
