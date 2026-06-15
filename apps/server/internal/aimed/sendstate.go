package aimed

import "strings"

// 文件五态（§8.6.4）。
const (
	FileUploading = "上传中"
	FileParsing   = "解析中"
	FileParsed    = "解析成功"
	FileFailed    = "解析失败"
	FileDeleted   = "已删除"
)

// FileState 发送状态机所需的最小文件视图。
type FileState struct {
	Status string
}

// SendVerdict 发送按钮状态（§8.5）：后端单点计算，前端只渲染。
type SendVerdict struct {
	CanSend bool   `json:"canSend"`
	Reason  string `json:"reason"` // 置灰原因；CanSend=true 时为空
}

// hasValidText：非空且非纯空格（§8.5「纯空格视为空」）。
func hasValidText(text string) bool { return strings.TrimSpace(text) != "" }

// CanSend 按「模式 + 文件上传/解析状态 + 输入有效文本」三元组计算发送按钮状态（§8.5）。
// files 已剔除「已删除」之外的活动文件由调用方保证；此处对 FileDeleted 同样跳过以稳健。
func CanSend(mode Mode, text string, files []FileState) SendVerdict {
	var active []FileState
	anyParsing, anyParsed := false, false
	for _, f := range files {
		if f.Status == FileDeleted {
			continue
		}
		active = append(active, f)
		switch f.Status {
		case FileUploading, FileParsing:
			anyParsing = true
		case FileParsed:
			anyParsed = true
		}
	}
	validText := hasValidText(text)

	switch mode {
	case ModeDeepReading:
		// 必须上传文件；无文件无论是否有文本均置灰
		if len(active) == 0 {
			return SendVerdict{false, "请先上传文献后再发送"}
		}
		if anyParsing {
			return SendVerdict{false, "文件解析中，请稍候"}
		}
		if !anyParsed {
			return SendVerdict{false, "文件解析失败，可移除后重新上传"}
		}
		return SendVerdict{true, ""}

	case ModeTrendAnalysis, ModeEvidenceTracing:
		// 无文件上传入口，仅看有效文本
		if !validText {
			return SendVerdict{false, "请输入内容后再发送"}
		}
		return SendVerdict{true, ""}

	default: // general / review_gen / writing_assist
		if len(active) == 0 {
			if !validText {
				return SendVerdict{false, "请输入内容后再发送"}
			}
			return SendVerdict{true, ""}
		}
		if anyParsing {
			return SendVerdict{false, "文件解析中，请稍候"}
		}
		if !anyParsed {
			return SendVerdict{false, "文件解析失败，可移除后重新上传"}
		}
		return SendVerdict{true, ""}
	}
}
