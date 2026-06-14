package model

import (
	"time"

	"gorm.io/gorm"
)

type ConnectivityResult struct {
	Status    string `json:"status"`
	LatencyMs int    `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

func probeModel(conn ProviderConnection, capability Capability) error {
	a, err := getAdapter(conn.Protocol)
	if err != nil {
		return err
	}
	switch {
	case capability == CapEmbed:
		_, e := a.Embed(EmbedRequest{Input: []string{"ping"}}, conn)
		return e
	case capability == CapRerank:
		_, e := a.Rerank(RerankRequest{Query: "ping", Documents: []string{"a", "b"}}, conn)
		return e
	case IsGenerationCapability(capability):
		_, e := a.Generate(GenerationRequest{Messages: []ChatMessage{{Role: "user", Content: "ping"}}}, conn)
		return e
	default:
		return newProviderError(ErrUnknown, "不支持对 "+string(capability)+" 做连通性探针")
	}
}

func TestModelConnectivity(db *gorm.DB, tenantID, providerID string, capability Capability) (ConnectivityResult, error) {
	conn, err := GetModelConnection(db, tenantID, providerID)
	if err != nil {
		return ConnectivityResult{}, err
	}
	if conn == nil {
		return ConnectivityResult{Status: "down", LatencyMs: 0, Error: "provider 不存在或不属于当前租户"}, nil
	}
	start := time.Now()
	perr := probeModel(*conn, capability)
	latency := int(time.Since(start).Milliseconds())
	if perr == nil {
		_ = RecordHealth(db, HealthRecord{TenantID: tenantID, ProviderID: providerID, ProviderKind: "model", CheckKind: "active", Status: "up", LatencyMs: &latency})
		return ConnectivityResult{Status: "up", LatencyMs: latency}, nil
	}
	_ = RecordHealth(db, HealthRecord{TenantID: tenantID, ProviderID: providerID, ProviderKind: "model", CheckKind: "active", Status: "down", LatencyMs: &latency, Error: perr.Error()})
	return ConnectivityResult{Status: "down", LatencyMs: latency, Error: perr.Error()}, nil
}

func TestVisualConnectivity(db *gorm.DB, tenantID, vpProviderID string) (ConnectivityResult, error) {
	conn, err := GetVisualConnection(db, tenantID, vpProviderID)
	if err != nil {
		return ConnectivityResult{}, err
	}
	if conn == nil {
		return ConnectivityResult{Status: "down", LatencyMs: 0, Error: "视觉解析 provider 不存在或不属于当前租户"}, nil
	}
	start := time.Now()
	_, perr := ProviderFetch(*conn, "parse", map[string]any{"probe": true}, authHeaders(*conn))
	latency := int(time.Since(start).Milliseconds())
	if perr == nil {
		_ = RecordHealth(db, HealthRecord{TenantID: tenantID, ProviderID: vpProviderID, ProviderKind: "visual", CheckKind: "active", Status: "up", LatencyMs: &latency})
		return ConnectivityResult{Status: "up", LatencyMs: latency}, nil
	}
	_ = RecordHealth(db, HealthRecord{TenantID: tenantID, ProviderID: vpProviderID, ProviderKind: "visual", CheckKind: "active", Status: "down", LatencyMs: &latency, Error: perr.Error()})
	return ConnectivityResult{Status: "down", LatencyMs: latency, Error: perr.Error()}, nil
}
