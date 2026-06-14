package model

// 复刻 redaction-gateway.ts：c09 PHI/PII 脱敏门禁接缝，唯一 owner=c09。本期默认拒绝（公网保守降级）。
type RedactionInput struct {
	TenantID string
	Text     string
}

type RedactionVerdict struct {
	Available    bool
	Passed       bool
	Confidence   float64
	RedactedText string
	Reason       string
}

type RedactionGateway interface {
	Evaluate(in RedactionInput) RedactionVerdict
}

type defaultRedaction struct{}

func (defaultRedaction) Evaluate(RedactionInput) RedactionVerdict {
	return RedactionVerdict{
		Available:  false,
		Passed:     false,
		Confidence: 0,
		Reason:     "c09 redaction-gateway 未接入（phase 9 落地）；本期公网调用按识别服务不可用处理、默认拒绝/降级私有化",
	}
}

// redactionGateway：本期默认实现。c09 落地时替换此变量，router/visual 调用点不变。
var redactionGateway RedactionGateway = defaultRedaction{}

// EvaluateRedaction 供 parsing 视觉解析在公网前置门控调用（model 包外唯一脱敏入口）。
func EvaluateRedaction(in RedactionInput) RedactionVerdict { return redactionGateway.Evaluate(in) }
