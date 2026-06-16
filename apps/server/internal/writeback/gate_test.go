package writeback

import "testing"

func TestGateDocumentWrite(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		canEdit    bool
		canCopy    bool
		high       bool
		canConfirm bool
		allowed    bool
		review     bool
		status     int
	}{
		{"apply 普通低风险放行", "apply", true, true, false, false, true, false, 0},
		{"apply 无编辑权限拒绝", "apply", false, true, false, false, false, false, 403},
		{"apply 高风险普通用户仅可提交审核", "apply", true, true, true, false, false, true, 403},
		{"apply 高风险授权角色放行", "apply", true, true, true, true, true, false, 0},
		{"copy 高风险普通用户可生成副本（草稿）", "copy", false, true, true, false, true, false, 0},
		{"copy 无复制权限拒绝", "copy", false, false, false, false, false, false, 403},
		{"submit_review 始终记录待审核", "submit_review", false, false, true, false, true, true, 0},
		{"未知动作拒绝", "weird", true, true, false, false, false, false, 400},
	}
	for _, c := range cases {
		d := GateDocumentWrite(c.action, c.canEdit, c.canCopy, c.high, c.canConfirm)
		if d.Allowed != c.allowed || d.Review != c.review || (!c.allowed && d.Status != c.status) {
			t.Errorf("%s: got %+v", c.name, d)
		}
	}
}
