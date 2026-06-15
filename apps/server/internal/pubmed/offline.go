package pubmed

import (
	"sort"
	"strings"
)

// OfflineProvider 读离线 PubMed 缓存/预置演示数据（部署预置 / c06 导入沉淀）。
// 本期内置一份小型演示语料，覆盖验收测试集主题（§20.4），保证断网闭环（§24.1）。
type OfflineProvider struct {
	corpus []RetrievedSource
}

// NewOfflineProvider 用内置演示语料构造（也可由部署注入更大缓存）。
func NewOfflineProvider() *OfflineProvider {
	return &OfflineProvider{corpus: demoCorpus()}
}

// NewOfflineProviderWith 用自定义语料构造（测试 / 部署预置）。
func NewOfflineProviderWith(corpus []RetrievedSource) *OfflineProvider {
	return &OfflineProvider{corpus: corpus}
}

func (p *OfflineProvider) Name() string { return "offline" }

// Search 关键词重叠打分召回（离线无向量服务，纯词面匹配）。
func (p *OfflineProvider) Search(query string, limit int) ([]RetrievedSource, error) {
	terms := tokenize(query)
	type scored struct {
		src   RetrievedSource
		score int
	}
	var hits []scored
	for _, s := range p.corpus {
		hay := strings.ToLower(s.Title + " " + s.Abstract + " " + s.Journal)
		sc := 0
		for _, t := range terms {
			if strings.Contains(hay, t) {
				sc++
			}
		}
		if sc > 0 {
			s.AuthStatus = AuthAuthorized // 离线缓存内容视为已授权来源
			hits = append(hits, scored{s, sc})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if limit <= 0 {
		limit = 10
	}
	out := make([]RetrievedSource, 0, limit)
	for i, h := range hits {
		if i >= limit {
			break
		}
		out = append(out, h.src)
	}
	return out, nil
}

func (p *OfflineProvider) FetchDetail(id string) (*RetrievedSource, error) {
	for _, s := range p.corpus {
		if s.PubmedID == id || s.DOI == id || s.URL == id {
			cp := s
			return &cp, nil
		}
	}
	return nil, nil
}

func tokenize(q string) []string {
	lower := strings.ToLower(q)
	repl := strings.NewReplacer(",", " ", "，", " ", "。", " ", "、", " ", "/", " ", "\n", " ")
	lower = repl.Replace(lower)
	var out []string
	for _, w := range strings.Fields(lower) {
		if len(w) >= 2 {
			out = append(out, w)
		}
	}
	// 中文无空格分词：补充按 2-gram 粗切，提升离线中文召回
	for _, run := range chineseBigrams(q) {
		out = append(out, run)
	}
	return out
}

func chineseBigrams(q string) []string {
	runes := []rune(q)
	var out []string
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x4e00 && a <= 0x9fff && b >= 0x4e00 && b <= 0x9fff {
			out = append(out, strings.ToLower(string([]rune{a, b})))
		}
	}
	return out
}

// demoCorpus 内置离线 PubMed 演示语料（验收/冒烟用）。
func demoCorpus() []RetrievedSource {
	return []RetrievedSource{
		{SourceType: "pubmed", PubmedID: "34567890", DOI: "10.1000/lung-immuno-2021", Title: "Immune checkpoint inhibitors in non-small cell lung cancer: a meta-analysis", Journal: "Journal of Clinical Oncology", Year: 2021, URL: "https://pubmed.ncbi.nlm.nih.gov/34567890/", Abstract: "肺癌 免疫治疗 PD-1/PD-L1 检查点抑制剂 在非小细胞肺癌 中显著改善总生存期。本 meta 分析纳入随机对照试验 RCT，证据等级高。lung cancer immunotherapy checkpoint inhibitor improves survival."},
		{SourceType: "pubmed", PubmedID: "33456789", DOI: "10.1000/diabetes-sglt2-2020", Title: "SGLT2 inhibitors and cardiovascular outcomes in type 2 diabetes", Journal: "NEJM", Year: 2020, URL: "https://pubmed.ncbi.nlm.nih.gov/33456789/", Abstract: "糖尿病 SGLT2 抑制剂 降低心血管事件风险。2 型糖尿病 患者 心血管 结局 改善。diabetes cardiovascular outcome randomized controlled trial."},
		{SourceType: "pubmed", PubmedID: "35678901", DOI: "10.1000/htn-guideline-2022", Title: "2022 hypertension management guideline: evidence-based recommendations", Journal: "Hypertension", Year: 2022, URL: "https://pubmed.ncbi.nlm.nih.gov/35678901/", Abstract: "高血压 指南 循证 推荐 临床结论 证据等级 血压 管理。hypertension guideline evidence based clinical recommendation blood pressure."},
		{SourceType: "pubmed", PubmedID: "36789012", DOI: "10.1000/covid-vaccine-2022", Title: "Efficacy and safety of mRNA COVID-19 vaccines: a systematic review", Journal: "Lancet", Year: 2022, URL: "https://pubmed.ncbi.nlm.nih.gov/36789012/", Abstract: "新冠 疫苗 mRNA 有效性 安全性 系统综述 免疫 抗体。covid vaccine efficacy safety systematic review immune antibody virus."},
		{SourceType: "pubmed", PubmedID: "32345678", DOI: "10.1000/breast-her2-2019", Title: "HER2-targeted therapy in breast cancer: a decade of progress", Journal: "Nature Reviews Cancer", Year: 2019, URL: "https://pubmed.ncbi.nlm.nih.gov/32345678/", Abstract: "乳腺癌 HER2 靶向治疗 肿瘤 进展 综述。breast cancer HER2 targeted therapy tumor progress review trend hotspot."},
		{SourceType: "pubmed", PubmedID: "37890123", DOI: "10.1000/sepsis-fluid-2023", Title: "Fluid resuscitation strategies in sepsis: an updated meta-analysis", Journal: "Critical Care Medicine", Year: 2023, URL: "https://pubmed.ncbi.nlm.nih.gov/37890123/", Abstract: "脓毒症 感染 液体复苏 meta 分析 重症 临床。sepsis infection fluid resuscitation meta-analysis critical care randomized."},
		{SourceType: "pubmed", PubmedID: "31234567", DOI: "10.1000/ai-imaging-2018", Title: "Deep learning for medical imaging diagnosis: trends and hotspots", Journal: "Radiology", Year: 2018, URL: "https://pubmed.ncbi.nlm.nih.gov/31234567/", Abstract: "人工智能 医学影像 深度学习 诊断 趋势 热点 前沿 发文趋势。artificial intelligence medical imaging deep learning diagnosis trend hotspot frontier."},
		{SourceType: "pubmed", PubmedID: "38901234", DOI: "10.1000/liver-nash-2023", Title: "Pharmacotherapy for NASH: evidence from recent clinical trials", Journal: "Gastroenterology", Year: 2023, URL: "https://pubmed.ncbi.nlm.nih.gov/38901234/", Abstract: "肝 脂肪性肝炎 NASH 药物治疗 临床试验 证据。liver NASH pharmacotherapy clinical trial evidence randomized controlled."},
	}
}
