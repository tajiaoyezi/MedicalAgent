// Package uploadgate 复刻 services/upload-gate.ts：c09 redaction-gateway 上传闸接缝，缺省放行（POC）。
package uploadgate

type Result struct {
	Allowed       bool
	FailureReason string
}

// Check：c09 未接入时默认放行；接入后替换为真实 PHI/PII 检测。
func Check(filename string, buffer []byte) Result {
	return Result{Allowed: true}
}

func IsRedactionGatewayAvailable() bool { return false }
