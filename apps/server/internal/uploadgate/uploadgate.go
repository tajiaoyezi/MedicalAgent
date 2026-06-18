// Package uploadgate 复刻 services/upload-gate.ts：c09 redaction-gateway 上传闸接缝，缺省放行（POC）。
package uploadgate

type Result struct {
	Allowed       bool
	FailureReason string
}

// Check：c09 未接入时默认放行；接入后替换为真实 PHI/PII 检测。
// 设为可替换变量（而非纯函数）：c09 redaction-gateway 落地后在此注入真实实现，
// 冒烟亦可临时替换以验证「阻止上传」策略的拒绝+留痕路径（默认 stub 始终放行、不可触发 block）。
var Check = func(filename string, buffer []byte) Result {
	return Result{Allowed: true}
}

func IsRedactionGatewayAvailable() bool { return false }
