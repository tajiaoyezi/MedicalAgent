package aimed

import "testing"

// 修复 #1：denylist 之外、换措辞绕过的用药/诊疗表述必须判高风险（修复前漏判 → 高风险草稿直达普通用户）。
func TestClassifyRiskCatchesRewordedDosing(t *testing.T) {
	cases := []string{
		"每日两次，每次一片，连服七天",
		"首剂加倍，之后维持原量",
		"阿莫西林 500mg 口服 bid",
		"静脉滴注 0.9% 氯化钠 250ml",
		"每日3次，餐后服用",
	}
	for _, c := range cases {
		if _, high := ClassifyRisk(c); !high {
			t.Errorf("应判高风险但漏判: %q", c)
		}
	}
}

// denylist 关键词仍生效。
func TestClassifyRiskKeywordStillWorks(t *testing.T) {
	if _, high := ClassifyRisk("请遵医嘱调整用药剂量"); !high {
		t.Error("含 denylist 关键词应判高风险")
	}
}

// 良性科普/文献综述不应被误判高风险（避免保守化后全量误拦）。
func TestClassifyRiskBenignNotFlagged(t *testing.T) {
	cases := []string{
		"这篇综述总结了近五年肺癌免疫治疗的研究进展。",
		"RCT 与队列研究在证据等级上的差异主要体现在偏倚控制。",
		"该指南讨论了筛查策略与随访间隔的循证依据。",
	}
	for _, c := range cases {
		if rt, high := ClassifyRisk(c); high {
			t.Errorf("良性内容被误判高风险: %q (riskType=%s)", c, rt)
		}
	}
}
