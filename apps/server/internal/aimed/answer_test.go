package aimed

import "testing"

// #11 防御：模型 prose 中超出有效引用序号的悬空/越界角标应被移除，范围内（含重复）保留。
func TestSanitizeMarkers(t *testing.T) {
	tests := []struct {
		name  string
		prose string
		max   int
		want  string
	}{
		{"范围内保留", "结论A[1]，结论B[2]。", 2, "结论A[1]，结论B[2]。"},
		{"越界移除", "显著获益[4]，另见[2]。", 2, "显著获益，另见[2]。"},
		{"重复保留", "见[1]，又见[1]。", 1, "见[1]，又见[1]。"},
		{"零角标移除", "占位[0] 文本。", 3, "占位 文本。"},
		{"max=0 全移除", "全部[1][2] 去掉。", 0, "全部 去掉。"},
		{"无角标不变", "纯文本无角标。", 3, "纯文本无角标。"},
	}
	for _, tt := range tests {
		if got := sanitizeMarkers(tt.prose, tt.max); got != tt.want {
			t.Errorf("%s: sanitizeMarkers(%q,%d)=%q want %q", tt.name, tt.prose, tt.max, got, tt.want)
		}
	}
}
