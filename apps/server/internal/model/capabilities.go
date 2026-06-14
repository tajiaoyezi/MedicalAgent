// Package model 复刻 c03 模型 Provider 抽象层：能力契约、四协议 adapter、出网网关、provider 存取、fallback 路由、健康检查、脱敏接缝。
package model

type Capability string

const (
	CapChat        Capability = "chat"
	CapSummarize   Capability = "summarize"
	CapTranslate   Capability = "translate"
	CapEmbed       Capability = "embed"
	CapRerank      Capability = "rerank"
	CapVisualParse Capability = "visual_parse"
	CapTermExtract Capability = "term_extract"
	CapProofread   Capability = "proofread"
	CapOutlineGen  Capability = "outline_gen"
)

// GenerationCapabilities：生成类（Anthropic Messages 仅可绑定这些）。
var GenerationCapabilities = []Capability{CapChat, CapSummarize, CapTranslate, CapTermExtract, CapProofread, CapOutlineGen}

// RoutableCapabilities：model_routes 可绑定的 8 类（visual_parse 单独配置）。
var RoutableCapabilities = []Capability{CapChat, CapSummarize, CapTranslate, CapEmbed, CapRerank, CapTermExtract, CapProofread, CapOutlineGen}

// MainLoopCapabilities：主验收闭环能力，每条须至少一条私有化路径。
var MainLoopCapabilities = []Capability{CapChat, CapTranslate, CapEmbed, CapVisualParse}

func IsGenerationCapability(cap Capability) bool { return contains(GenerationCapabilities, cap) }
func IsRoutableCapability(cap Capability) bool   { return contains(RoutableCapabilities, cap) }
func isMainLoop(cap Capability) bool             { return contains(MainLoopCapabilities, cap) }

func contains(s []Capability, x Capability) bool {
	for _, v := range s {
		if v == x {
			return true
		}
	}
	return false
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type GenerationRequest struct {
	Messages []ChatMessage
	Hint     string
}

type GenerationResponse struct {
	Content string
	Model   string
}

type EmbedRequest struct {
	Input []string
}

type EmbedResponse struct {
	Vectors [][]float64
	Model   string
	Dim     int
}

type RerankRequest struct {
	Query     string
	Documents []string
}

type RerankResponse struct {
	Scores []float64
	Model  string
}
