package aimed

import "strings"

// 固定文案（§8.11.4 / §8.11.5 原文）。
const (
	RefusalText      = "抱歉，AIMed 专注于医学科研问题，暂不支持该类提问。如需文献 / 科研相关帮助，请切换至对应模式。"
	CompoundHintText = "识别到您可能需要组合使用多个功能。建议分步操作"
	CompoundRetain   = "上传的文件和对话内容在兼容模式间会自动保留"
)

// 医学/科研信号词（general 模式无关问题判定的轻量启发式；二级 LLM 分类为 design D2 兜底，本期规则优先）。
var medicalSignals = []string{
	"医", "病", "症", "药", "诊", "疗", "治疗", "临床", "患者", "癌", "肿瘤", "细胞", "基因", "蛋白",
	"手术", "影像", "ct", "mri", "护理", "处方", "医嘱", "指南", "证据", "文献", "研究", "试验",
	"综述", "pubmed", "随机对照", "meta", "队列", "病例", "感染", "病毒", "抗体", "免疫", "心",
	"肝", "肺", "肾", "血", "糖尿病", "高血压", "诊断", "预后", "剂量", "疫苗",
}

func looksMedical(text string) bool {
	lower := strings.ToLower(text)
	for _, kw := range medicalSignals {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// MatchResult 智能模式匹配结论（§8.11）。仅影响 UI 提示，不改变实际发送模式与数据源。
type MatchResult struct {
	Recommended      Mode     `json:"recommended"`      // "" 表示无推荐
	RecommendedLabel string   `json:"recommendedLabel"` // 推荐模式中文名
	Guidance         string   `json:"guidance"`         // §8.11.2 引导文案
	Highlight        bool     `json:"highlight"`        // 推荐 ≠ 当前才高亮
	Compound         bool     `json:"compound"`         // 命中 ≥2 模式
	CompoundHint     string   `json:"compoundHint"`
	CompoundSteps    []string `json:"compoundSteps"`
	Refusal          string   `json:"refusal"` // 非空=通用问答拒答无关问题
}

// Evaluate 计算模式匹配结论。current=当前选定模式；text=用户输入。
func Evaluate(current Mode, text string) MatchResult {
	hits := MatchModes(text)
	res := MatchResult{}

	// 通用问答非医学/科研问题 → 固定拒答（§8.11.5）。命中任一专业模式关键词即视为医学/科研意图。
	if current == ModeGeneral && len(hits) == 0 && !looksMedical(text) {
		res.Refusal = RefusalText
		return res
	}

	if len(hits) > 0 {
		rec := hits[0]
		p := GetPolicy(rec)
		res.Recommended = rec
		res.RecommendedLabel = p.Label
		res.Guidance = p.Guidance
		res.Highlight = rec != current // 推荐≠当前才高亮，且 MUST NOT 自动切换
	}

	// 复合任务：命中 ≥2 模式 → 分步建议，不自动切多模式（§8.11.4）。
	if len(hits) >= 2 {
		res.Compound = true
		res.CompoundHint = CompoundHintText
		a, b := GetPolicy(hits[0]).Label, GetPolicy(hits[1]).Label
		res.CompoundSteps = []string{
			"① 先切换到「" + a + "」完成该部分",
			"② 再切换到「" + b + "」继续后续步骤",
			CompoundRetain,
		}
	}
	return res
}
