// Package aimed 实现 c04 AIMed 学术助手：六模式声明式 policy、会话/消息读写、发送状态机、
// 智能模式匹配、文件上传落库主闭环、答案生成与操作栏、引用与反馈。
package aimed

import "strings"

// Mode 六大模式枚举（mode 列取值）。
type Mode string

const (
	ModeGeneral         Mode = "general"          // 通用问答
	ModeDeepReading     Mode = "deep_reading"     // 深度文献伴读
	ModeTrendAnalysis   Mode = "trend_analysis"   // 科研态势分析
	ModeEvidenceTracing Mode = "evidence_tracing" // 循证证据溯源
	ModeReviewGen       Mode = "review_gen"       // 智能综述生成
	ModeWritingAssist   Mode = "writing_assist"   // 学术写作辅助
)

// Modes 六模式有序列表（UI Tab 顺序）。
var Modes = []Mode{ModeGeneral, ModeDeepReading, ModeTrendAnalysis, ModeEvidenceTracing, ModeReviewGen, ModeWritingAssist}

// Policy 声明式数据源约束（design D1 表 / PRD §8.2 §8.3）。非硬编码 if/else。
type Policy struct {
	Mode             Mode
	Label            string
	AllowPubmed      bool
	AllowUpload      bool
	AllowKB          bool // 医疗知识库：§8.2 仅 general=✓
	AllowCurrentDoc  bool
	UploadRequired   bool // 深度文献伴读强制上传
	ClearFilesOnEnter bool // 切入科研态势分析/循证证据溯源清空文件
	Placeholder      string
	Guidance         string // §8.11.2 智能模式匹配引导文案
}

var modePolicy = map[Mode]Policy{
	ModeGeneral: {
		Mode: ModeGeneral, Label: "通用问答",
		AllowPubmed: true, AllowUpload: true, AllowKB: true, AllowCurrentDoc: false,
		Placeholder: "向 AIMed 提问，或上传文档获取解答。",
		Guidance:    "",
	},
	ModeDeepReading: {
		Mode: ModeDeepReading, Label: "深度文献伴读",
		AllowPubmed: false, AllowUpload: true, AllowKB: false, AllowCurrentDoc: false,
		UploadRequired: true,
		Placeholder:    "请上传文献，开启专注的深度解读。",
		Guidance:       "仅基于该文献逐段深度解读，避免外部信息干扰",
	},
	ModeTrendAnalysis: {
		Mode: ModeTrendAnalysis, Label: "科研态势分析",
		AllowPubmed: true, AllowUpload: false, AllowKB: false, AllowCurrentDoc: false,
		ClearFilesOnEnter: true,
		Placeholder:       "请输入研究领域，如\"肺癌免疫治疗\"，分析其前沿趋势与热点。",
		Guidance:          "获取该领域发文趋势、热点与演化图谱",
	},
	ModeEvidenceTracing: {
		Mode: ModeEvidenceTracing, Label: "循证证据溯源",
		AllowPubmed: true, AllowUpload: false, AllowKB: false, AllowCurrentDoc: false,
		ClearFilesOnEnter: true,
		Placeholder:       "请输入一个临床结论或医学问题，我将为您追溯证据。",
		Guidance:          "检索并验证临床结论，获取高级别证据",
	},
	ModeReviewGen: {
		Mode: ModeReviewGen, Label: "智能综述生成",
		AllowPubmed: true, AllowUpload: true, AllowKB: false, AllowCurrentDoc: false,
		Placeholder: "请描述综述主题，如\"近 5 年 XX 治疗进展\"，或上传参考文献。",
		Guidance:    "基于文献自动生成结构化综述，支持溯源",
	},
	ModeWritingAssist: {
		Mode: ModeWritingAssist, Label: "学术写作辅助",
		AllowPubmed: true, AllowUpload: true, AllowKB: false, AllowCurrentDoc: true,
		Placeholder: "请粘贴需要润色 / 扩写的文本，或直接描述您的写作需求。",
		Guidance:    "获得段落生成、润色优化与引用补充支持",
	},
}

// IsMode 判定取值是否为合法六模式之一。
func IsMode(m string) bool {
	_, ok := modePolicy[Mode(m)]
	return ok
}

// GetPolicy 取某模式 policy（非法模式回退 general）。
func GetPolicy(m Mode) Policy {
	if p, ok := modePolicy[m]; ok {
		return p
	}
	return modePolicy[ModeGeneral]
}

// ShowPubmedTag：§8.3 深度文献伴读隐藏「数据源：PubMed」标签，其余模式显示。
func (p Policy) ShowPubmedTag() bool { return p.Mode != ModeDeepReading }

// —— 智能模式匹配（§8.11）——
// 优先级：深度文献伴读 > 循证证据溯源 > 科研态势分析 > 智能综述生成 > 学术写作辅助（general 不参与推荐）。
var matchPriority = []Mode{ModeDeepReading, ModeEvidenceTracing, ModeTrendAnalysis, ModeReviewGen, ModeWritingAssist}

// 关键词词典（§8.11.2）。一级规则匹配，命中即推荐对应模式。
var modeKeywords = map[Mode][]string{
	ModeDeepReading:     {"这篇", "该文献", "逐段", "深度解读", "精读", "这份文档", "上传的论文", "通读全文", "解读这篇"},
	ModeEvidenceTracing: {"rct", "meta 分析", "meta分析", "meta-analysis", "循证", "证据等级", "证据级别", "系统综述", "指南推荐", "临床结论", "高级别证据", "追溯证据"},
	ModeTrendAnalysis:   {"趋势", "热点", "前沿", "发文量", "发文趋势", "演化", "态势", "图谱", "研究方向", "近5年", "近 5 年", "领域进展"},
	ModeReviewGen:       {"综述", "写一篇综述", "文献综述", "结构化综述", "综述生成", "总结进展", "review"},
	ModeWritingAssist:   {"润色", "扩写", "改写", "段落生成", "写作", "摘要润色", "优化这段", "帮我写"},
}

// MatchModes 对输入文本做关键词匹配，返回命中模式（按 §8.11 固定优先级排序、去重）。
func MatchModes(text string) []Mode {
	lower := strings.ToLower(text)
	var hits []Mode
	for _, m := range matchPriority {
		for _, kw := range modeKeywords[m] {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hits = append(hits, m)
				break
			}
		}
	}
	return hits
}

// RecommendMode 取最高优先级推荐模式（无命中返回空）。
func RecommendMode(text string) (Mode, bool) {
	hits := MatchModes(text)
	if len(hits) == 0 {
		return "", false
	}
	return hits[0], true
}
