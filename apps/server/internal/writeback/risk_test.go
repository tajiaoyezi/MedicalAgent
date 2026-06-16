package writeback

import (
	"testing"

	"medoffice/server/internal/auth"
)

// c05 权威分类器：换措辞绕过的用药/诊疗表述必须判高风险（医疗红线，漏判 → 高风险草稿直达普通用户）。
func TestClassifyRiskCatchesRewordedDosing(t *testing.T) {
	cases := []string{
		"每日两次，每次一片，连服七天",
		"首剂加倍，之后维持原量",
		"阿莫西林 500mg 口服 bid",
		"静脉滴注 0.9% 氯化钠 250ml",
		"请遵医嘱调整用药剂量",
	}
	for _, c := range cases {
		if _, high := ClassifyRisk(c); !high {
			t.Errorf("应判高风险但漏判: %q", c)
		}
	}
}

// 良性科普/文献综述不应被误判高风险（含英文缩写碰撞、中文频次词+通用量词碰撞回归用例）。
func TestClassifyRiskBenignNotFlagged(t *testing.T) {
	cases := []string{
		"这篇综述总结了近五年肺癌免疫治疗的研究进展。",
		"He placed a bid on the auction and the prn function returned a value.",
		"每天吃一片面包当早餐有助于补充能量。",
	}
	for _, c := range cases {
		if rt, high := ClassifyRisk(c); high {
			t.Errorf("良性内容被误判高风险: %q (riskType=%s)", c, rt)
		}
	}
}

// 高风险确认角色裁决：highrisk:confirm 决定可否确认；ConfirmedRole 取 doctor 优先于 reviewer。
func TestConfirmRoleResolution(t *testing.T) {
	normal := auth.AuthUser{Permissions: []string{"document:read"}}
	if CanConfirm(normal) {
		t.Error("普通用户不应可确认高风险")
	}
	doctor := auth.AuthUser{RoleSlugs: []string{"doctor"}, Permissions: []string{"highrisk:confirm"}}
	if !CanConfirm(doctor) || ConfirmedRole(doctor) != "doctor" {
		t.Error("doctor 应可确认且角色取 doctor")
	}
	reviewer := auth.AuthUser{RoleSlugs: []string{"reviewer"}, Permissions: []string{"highrisk:confirm"}}
	if ConfirmedRole(reviewer) != "reviewer" {
		t.Error("reviewer 角色应取 reviewer")
	}
	both := auth.AuthUser{RoleSlugs: []string{"reviewer", "doctor"}, Permissions: []string{"highrisk:confirm"}}
	if ConfirmedRole(both) != "doctor" {
		t.Error("同时具备时 doctor 优先")
	}
}
