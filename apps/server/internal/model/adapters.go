package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Adapter 复刻 adapters.ts：协议适配到统一 capability 接口。
type Adapter interface {
	Protocol() string
	Supports(cap Capability) bool
	Generate(req GenerationRequest, conn ProviderConnection) (GenerationResponse, error)
	Embed(req EmbedRequest, conn ProviderConnection) (EmbedResponse, error)
	Rerank(req RerankRequest, conn ProviderConnection) (RerankResponse, error)
}

func notSupported(protocol string, cap Capability) error {
	return newProviderError(ErrUnknown, fmt.Sprintf("%s 协议不支持 %s 能力，请为该用途单独配置 provider", protocol, string(cap)))
}

// ——— OpenAI 兼容（openai_compat / local_gateway） ———
type openAICompatAdapter struct{ protocol string }

func (a openAICompatAdapter) Protocol() string         { return a.protocol }
func (a openAICompatAdapter) Supports(Capability) bool { return true }

func (a openAICompatAdapter) Generate(req GenerationRequest, conn ProviderConnection) (GenerationResponse, error) {
	rb, err := ProviderFetch(conn, "v1/chat/completions", map[string]any{"model": conn.Model, "messages": req.Messages}, authHeaders(conn))
	if err != nil {
		return GenerationResponse{}, err
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content *string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	_ = json.Unmarshal(rb, &out)
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == nil {
		return GenerationResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」返回缺少 choices[].message.content", conn.Name))
	}
	return GenerationResponse{Content: *out.Choices[0].Message.Content, Model: conn.Model}, nil
}

func (a openAICompatAdapter) Embed(req EmbedRequest, conn ProviderConnection) (EmbedResponse, error) {
	rb, err := ProviderFetch(conn, "v1/embeddings", map[string]any{"model": conn.Model, "input": req.Input}, authHeaders(conn))
	if err != nil {
		return EmbedResponse{}, err
	}
	var out struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rb, &out)
	vectors := make([][]float64, 0, len(out.Data))
	for _, d := range out.Data {
		vectors = append(vectors, d.Embedding)
	}
	if len(vectors) == 0 || len(vectors[0]) == 0 {
		return EmbedResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」返回缺少 embedding 向量", conn.Name))
	}
	return EmbedResponse{Vectors: vectors, Model: conn.Model, Dim: len(vectors[0])}, nil
}

func (a openAICompatAdapter) Rerank(req RerankRequest, conn ProviderConnection) (RerankResponse, error) {
	rb, err := ProviderFetch(conn, "v1/rerank", map[string]any{"model": conn.Model, "query": req.Query, "documents": req.Documents}, authHeaders(conn))
	if err != nil {
		return RerankResponse{}, err
	}
	var out struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
		Scores []float64 `json:"scores"`
	}
	_ = json.Unmarshal(rb, &out)
	var scores []float64
	switch {
	case out.Scores != nil:
		scores = out.Scores
	case out.Results != nil:
		scores = make([]float64, len(req.Documents))
		for _, r := range out.Results {
			if r.Index >= 0 && r.Index < len(scores) {
				scores[r.Index] = r.RelevanceScore
			}
		}
	default:
		return RerankResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」rerank 返回缺少 scores/results", conn.Name))
	}
	return RerankResponse{Scores: scores, Model: conn.Model}, nil
}

// ——— Anthropic Messages：仅生成类 ———
type anthropicAdapter struct{}

func (anthropicAdapter) Protocol() string             { return "anthropic_messages" }
func (anthropicAdapter) Supports(cap Capability) bool { return IsGenerationCapability(cap) }

func (anthropicAdapter) Generate(req GenerationRequest, conn ProviderConnection) (GenerationResponse, error) {
	var system []string
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		if m.Role == "system" {
			system = append(system, m.Content)
		} else {
			messages = append(messages, map[string]any{"role": m.Role, "content": m.Content})
		}
	}
	headers := map[string]string{"anthropic-version": "2023-06-01"}
	if conn.Credential != "" {
		headers["x-api-key"] = conn.Credential
	}
	body := map[string]any{"model": conn.Model, "max_tokens": 1024, "messages": messages}
	if sys := strings.Join(system, "\n"); sys != "" {
		body["system"] = sys
	}
	rb, err := ProviderFetch(conn, "v1/messages", body, headers)
	if err != nil {
		return GenerationResponse{}, err
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(rb, &out)
	var sb strings.Builder
	for _, c := range out.Content {
		sb.WriteString(c.Text)
	}
	if sb.Len() == 0 {
		return GenerationResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」返回缺少 content[].text", conn.Name))
	}
	return GenerationResponse{Content: sb.String(), Model: conn.Model}, nil
}

func (anthropicAdapter) Embed(EmbedRequest, ProviderConnection) (EmbedResponse, error) {
	return EmbedResponse{}, notSupported("anthropic_messages", CapEmbed)
}

func (anthropicAdapter) Rerank(RerankRequest, ProviderConnection) (RerankResponse, error) {
	return RerankResponse{}, notSupported("anthropic_messages", CapRerank)
}

// ——— 第三方 /invoke 信封 ———
type thirdPartyAdapter struct{}

func (thirdPartyAdapter) Protocol() string         { return "third_party" }
func (thirdPartyAdapter) Supports(Capability) bool { return true }

func (thirdPartyAdapter) Generate(req GenerationRequest, conn ProviderConnection) (GenerationResponse, error) {
	rb, err := ProviderFetch(conn, "invoke", map[string]any{"capability": "generate", "model": conn.Model, "messages": req.Messages}, authHeaders(conn))
	if err != nil {
		return GenerationResponse{}, err
	}
	var out struct {
		Content *string `json:"content"`
	}
	_ = json.Unmarshal(rb, &out)
	if out.Content == nil {
		return GenerationResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」/invoke 返回缺少 content", conn.Name))
	}
	return GenerationResponse{Content: *out.Content, Model: conn.Model}, nil
}

func (thirdPartyAdapter) Embed(req EmbedRequest, conn ProviderConnection) (EmbedResponse, error) {
	rb, err := ProviderFetch(conn, "invoke", map[string]any{"capability": "embed", "model": conn.Model, "input": req.Input}, authHeaders(conn))
	if err != nil {
		return EmbedResponse{}, err
	}
	var out struct {
		Vectors [][]float64 `json:"vectors"`
	}
	_ = json.Unmarshal(rb, &out)
	if len(out.Vectors) == 0 || len(out.Vectors[0]) == 0 {
		return EmbedResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」/invoke 返回缺少 vectors", conn.Name))
	}
	return EmbedResponse{Vectors: out.Vectors, Model: conn.Model, Dim: len(out.Vectors[0])}, nil
}

func (thirdPartyAdapter) Rerank(req RerankRequest, conn ProviderConnection) (RerankResponse, error) {
	rb, err := ProviderFetch(conn, "invoke", map[string]any{"capability": "rerank", "model": conn.Model, "query": req.Query, "documents": req.Documents}, authHeaders(conn))
	if err != nil {
		return RerankResponse{}, err
	}
	var out struct {
		Scores []float64 `json:"scores"`
	}
	_ = json.Unmarshal(rb, &out)
	if out.Scores == nil {
		return RerankResponse{}, newProviderError(ErrUnknown, fmt.Sprintf("provider「%s」/invoke 返回缺少 scores", conn.Name))
	}
	return RerankResponse{Scores: out.Scores, Model: conn.Model}, nil
}

func getAdapter(protocol string) (Adapter, error) {
	switch protocol {
	case "openai_compat":
		return openAICompatAdapter{protocol: "openai_compat"}, nil
	case "local_gateway":
		return openAICompatAdapter{protocol: "local_gateway"}, nil
	case "anthropic_messages":
		return anthropicAdapter{}, nil
	case "third_party":
		return thirdPartyAdapter{}, nil
	default:
		return nil, newProviderError(ErrUnknown, "未知协议："+protocol)
	}
}

// ProtocolSupportsCapability：配置层校验 protocol 是否可绑定该 capability（Anthropic 限制）。
func ProtocolSupportsCapability(protocol string, cap Capability) bool {
	a, err := getAdapter(protocol)
	if err != nil {
		return false
	}
	return a.Supports(cap)
}
