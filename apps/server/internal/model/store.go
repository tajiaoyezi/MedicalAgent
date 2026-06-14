package model

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/cryptox"
)

// 包级状态：凭据编解码器与健康 TTL（复刻 Node 模块级 config/crypto 单例）。由 Init 在启动时设置。
var (
	codec            *cryptox.Codec
	healthTTLSeconds = 60
)

// Init 初始化凭据密钥与健康 TTL。server 与 c03 smoke 启动时各调用一次。
func Init(credentialSecret string, healthTTL int) {
	codec = cryptox.New(credentialSecret)
	if healthTTL > 0 {
		healthTTLSeconds = healthTTL
	}
}

type modelProviderRow struct {
	ProviderID       string  `gorm:"column:provider_id"`
	TenantID         string  `gorm:"column:tenant_id"`
	Name             string  `gorm:"column:name"`
	Protocol         string  `gorm:"column:protocol"`
	DeploymentKind   string  `gorm:"column:deployment_kind"`
	BaseURL          string  `gorm:"column:base_url"`
	CredentialCipher *string `gorm:"column:credential_cipher"`
	Model            string  `gorm:"column:model"`
	TimeoutMs        int     `gorm:"column:timeout_ms"`
	MaxRetries       int     `gorm:"column:max_retries"`
	NetworkPolicy    *string `gorm:"column:network_policy"`
	Enabled          bool    `gorm:"column:enabled"`
	DefaultPriority  int     `gorm:"column:default_priority"`
}

type visualProviderRow struct {
	VPProviderID     string  `gorm:"column:vp_provider_id"`
	TenantID         string  `gorm:"column:tenant_id"`
	Name             string  `gorm:"column:name"`
	BackendKind      string  `gorm:"column:backend_kind"`
	DeploymentKind   string  `gorm:"column:deployment_kind"`
	BaseURL          string  `gorm:"column:base_url"`
	CredentialCipher *string `gorm:"column:credential_cipher"`
	Model            *string `gorm:"column:model"`
	TimeoutMs        int     `gorm:"column:timeout_ms"`
	NetworkPolicy    *string `gorm:"column:network_policy"`
	Enabled          bool    `gorm:"column:enabled"`
	DefaultPriority  int     `gorm:"column:default_priority"`
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func modelRowToConnection(r modelProviderRow) ProviderConnection {
	return ProviderConnection{
		ProviderID: r.ProviderID, Kind: "model", Name: r.Name, Protocol: r.Protocol,
		DeploymentKind: DeploymentKind(r.DeploymentKind), BaseURL: r.BaseURL,
		Credential: codec.Decrypt(deref(r.CredentialCipher)), Model: r.Model,
		TimeoutMs: r.TimeoutMs, MaxRetries: r.MaxRetries, NetworkPolicy: deref(r.NetworkPolicy),
	}
}

func visualRowToConnection(r visualProviderRow) ProviderConnection {
	return ProviderConnection{
		ProviderID: r.VPProviderID, Kind: "visual", Name: r.Name, BackendKind: r.BackendKind,
		DeploymentKind: DeploymentKind(r.DeploymentKind), BaseURL: r.BaseURL,
		Credential: codec.Decrypt(deref(r.CredentialCipher)), Model: deref(r.Model),
		TimeoutMs: r.TimeoutMs, MaxRetries: 0, NetworkPolicy: deref(r.NetworkPolicy),
	}
}

func maskModelProvider(r modelProviderRow) map[string]any {
	return map[string]any{
		"providerId": r.ProviderID, "name": r.Name, "protocol": r.Protocol,
		"deploymentKind": r.DeploymentKind, "baseUrl": r.BaseURL,
		"credentialMasked": codec.Mask(deref(r.CredentialCipher)), "model": r.Model,
		"timeoutMs": r.TimeoutMs, "maxRetries": r.MaxRetries, "networkPolicy": r.NetworkPolicy,
		"enabled": r.Enabled, "defaultPriority": r.DefaultPriority,
	}
}

func maskVisualProvider(r visualProviderRow) map[string]any {
	return map[string]any{
		"vpProviderId": r.VPProviderID, "name": r.Name, "backendKind": r.BackendKind,
		"deploymentKind": r.DeploymentKind, "baseUrl": r.BaseURL,
		"credentialMasked": codec.Mask(deref(r.CredentialCipher)), "model": r.Model,
		"networkPolicy": r.NetworkPolicy, "enabled": r.Enabled, "defaultPriority": r.DefaultPriority,
	}
}

// ——— model_providers CRUD ———

type ModelProviderInput struct {
	Name            string
	Protocol        string
	DeploymentKind  string
	BaseURL         string
	Credential      string // "" = null
	Model           string
	TimeoutMs       *int
	MaxRetries      *int
	NetworkPolicy   string // "" = 默认（private→intranet_only / public→null）
	Enabled         bool
	DefaultPriority *int
}

func intOr(p *int, def int) int {
	if p != nil {
		return *p
	}
	return def
}

func cipherOf(plain string) any {
	if plain == "" {
		return nil
	}
	return codec.Encrypt(plain)
}

func policyOf(deploymentKind, networkPolicy string) any {
	if deploymentKind != "private" {
		return nil
	}
	if networkPolicy == "" {
		return "intranet_only"
	}
	return networkPolicy
}

func ListModelProviders(db *gorm.DB, tenantID string) ([]map[string]any, error) {
	var rows []modelProviderRow
	if err := db.Raw(`SELECT * FROM model_providers WHERE tenant_id = ? ORDER BY deployment_kind, default_priority, name`, tenantID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, maskModelProvider(r))
	}
	return out, nil
}

func GetModelProviderRow(db *gorm.DB, tenantID, providerID string) (*modelProviderRow, error) {
	var r modelProviderRow
	if err := db.Raw(`SELECT * FROM model_providers WHERE provider_id = ? AND tenant_id = ?`, providerID, tenantID).Scan(&r).Error; err != nil {
		return nil, err
	}
	if r.ProviderID == "" {
		return nil, nil
	}
	return &r, nil
}

func GetModelConnection(db *gorm.DB, tenantID, providerID string) (*ProviderConnection, error) {
	r, err := GetModelProviderRow(db, tenantID, providerID)
	if err != nil || r == nil {
		return nil, err
	}
	c := modelRowToConnection(*r)
	return &c, nil
}

func GetVisualConnection(db *gorm.DB, tenantID, vpProviderID string) (*ProviderConnection, error) {
	var r visualProviderRow
	if err := db.Raw(`SELECT * FROM visual_parse_providers WHERE vp_provider_id = ? AND tenant_id = ?`, vpProviderID, tenantID).Scan(&r).Error; err != nil {
		return nil, err
	}
	if r.VPProviderID == "" {
		return nil, nil
	}
	c := visualRowToConnection(r)
	return &c, nil
}

func CreateModelProvider(db *gorm.DB, tenantID string, in ModelProviderInput) (string, error) {
	var id string
	err := db.Raw(
		`INSERT INTO model_providers (tenant_id, name, protocol, deployment_kind, base_url, credential_cipher,
		   model, timeout_ms, max_retries, network_policy, enabled, default_priority)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?) RETURNING provider_id`,
		tenantID, in.Name, in.Protocol, in.DeploymentKind, in.BaseURL, cipherOf(in.Credential),
		in.Model, intOr(in.TimeoutMs, 30000), intOr(in.MaxRetries, 1), policyOf(in.DeploymentKind, in.NetworkPolicy),
		in.Enabled, intOr(in.DefaultPriority, 100),
	).Scan(&id).Error
	return id, err
}

type ModelProviderPatch struct {
	Name            *string
	BaseURL         *string
	Credential      *string
	Model           *string
	TimeoutMs       *int
	MaxRetries      *int
	NetworkPolicy   *string
	Enabled         *bool
	DefaultPriority *int
}

func UpdateModelProvider(db *gorm.DB, tenantID, providerID string, patch ModelProviderPatch) (bool, error) {
	updates := map[string]any{}
	if patch.Name != nil {
		updates["name"] = *patch.Name
	}
	if patch.BaseURL != nil {
		updates["base_url"] = *patch.BaseURL
	}
	if patch.Credential != nil {
		updates["credential_cipher"] = cipherOf(*patch.Credential)
	}
	if patch.Model != nil {
		updates["model"] = *patch.Model
	}
	if patch.TimeoutMs != nil {
		updates["timeout_ms"] = *patch.TimeoutMs
	}
	if patch.MaxRetries != nil {
		updates["max_retries"] = *patch.MaxRetries
	}
	if patch.NetworkPolicy != nil {
		if *patch.NetworkPolicy == "" {
			updates["network_policy"] = nil
		} else {
			updates["network_policy"] = *patch.NetworkPolicy
		}
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	if patch.DefaultPriority != nil {
		updates["default_priority"] = *patch.DefaultPriority
	}
	if len(updates) == 0 {
		return false, nil
	}
	updates["updated_at"] = time.Now()
	res := db.Table("model_providers").
		Where("provider_id = ? AND tenant_id = ?", providerID, tenantID).
		Updates(updates)
	return res.RowsAffected > 0, res.Error
}

func DeleteModelProvider(db *gorm.DB, tenantID, providerID string) (bool, error) {
	res := db.Exec(`DELETE FROM model_providers WHERE provider_id = ? AND tenant_id = ?`, providerID, tenantID)
	return res.RowsAffected > 0, res.Error
}

// ——— model_routes ———

type RouteBindError struct{ Msg string }

func (e *RouteBindError) Error() string { return e.Msg }

func BindRoute(db *gorm.DB, tenantID string, capability Capability, providerID string, priority int) error {
	if !IsRoutableCapability(capability) {
		return &RouteBindError{fmt.Sprintf("%s 不可经 model_routes 绑定（视觉解析须配置于 visual_parse_providers）", string(capability))}
	}
	row, err := GetModelProviderRow(db, tenantID, providerID)
	if err != nil {
		return err
	}
	if row == nil {
		return &RouteBindError{"provider 不存在或不属于当前租户"}
	}
	if !ProtocolSupportsCapability(row.Protocol, capability) {
		return &RouteBindError{fmt.Sprintf("%s 协议仅用于生成类能力，Embedding/Rerank/视觉解析须单独配置 provider", row.Protocol)}
	}
	return db.Exec(
		`INSERT INTO model_routes (tenant_id, capability, provider_id, priority, enabled)
		 VALUES (?,?,?,?,TRUE)
		 ON CONFLICT (tenant_id, capability, provider_id)
		 DO UPDATE SET priority = EXCLUDED.priority, enabled = TRUE`,
		tenantID, capability, providerID, priority,
	).Error
}

func ListRoutes(db *gorm.DB, tenantID string) ([]map[string]any, error) {
	var rows []struct {
		RouteID         string `gorm:"column:route_id" json:"route_id"`
		Capability      string `gorm:"column:capability" json:"capability"`
		ProviderID      string `gorm:"column:provider_id" json:"provider_id"`
		Priority        int    `gorm:"column:priority" json:"priority"`
		Enabled         bool   `gorm:"column:enabled" json:"enabled"`
		ProviderName    string `gorm:"column:provider_name" json:"provider_name"`
		DeploymentKind  string `gorm:"column:deployment_kind" json:"deployment_kind"`
		Protocol        string `gorm:"column:protocol" json:"protocol"`
		ProviderEnabled bool   `gorm:"column:provider_enabled" json:"provider_enabled"`
	}
	if err := db.Raw(
		`SELECT r.route_id, r.capability, r.provider_id, r.priority, r.enabled, p.name AS provider_name,
		        p.deployment_kind, p.protocol, p.enabled AS provider_enabled
		 FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
		 WHERE r.tenant_id = ? ORDER BY r.capability, r.priority`, tenantID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"route_id": r.RouteID, "capability": r.Capability, "provider_id": r.ProviderID,
			"priority": r.Priority, "enabled": r.Enabled, "provider_name": r.ProviderName,
			"deployment_kind": r.DeploymentKind, "protocol": r.Protocol, "provider_enabled": r.ProviderEnabled,
		})
	}
	return out, nil
}

func UnbindRoute(db *gorm.DB, tenantID, routeID string) (bool, error) {
	res := db.Exec(`DELETE FROM model_routes WHERE route_id = ? AND tenant_id = ?`, routeID, tenantID)
	return res.RowsAffected > 0, res.Error
}

// ——— visual_parse_providers CRUD ———

type VisualProviderInput struct {
	Name            string
	BackendKind     string
	DeploymentKind  string
	BaseURL         string
	Credential      string
	Model           string // "" = null
	TimeoutMs       *int
	NetworkPolicy   string
	Enabled         bool
	DefaultPriority *int
}

func ListVisualProviders(db *gorm.DB, tenantID string) ([]map[string]any, error) {
	var rows []visualProviderRow
	if err := db.Raw(`SELECT * FROM visual_parse_providers WHERE tenant_id = ? ORDER BY deployment_kind, default_priority, name`, tenantID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, maskVisualProvider(r))
	}
	return out, nil
}

func CreateVisualProvider(db *gorm.DB, tenantID string, in VisualProviderInput) (string, error) {
	var model any
	if in.Model != "" {
		model = in.Model
	}
	var id string
	err := db.Raw(
		`INSERT INTO visual_parse_providers (tenant_id, name, backend_kind, deployment_kind, base_url, credential_cipher,
		   model, timeout_ms, network_policy, enabled, default_priority)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?) RETURNING vp_provider_id`,
		tenantID, in.Name, in.BackendKind, in.DeploymentKind, in.BaseURL, cipherOf(in.Credential),
		model, intOr(in.TimeoutMs, 60000), policyOf(in.DeploymentKind, in.NetworkPolicy),
		in.Enabled, intOr(in.DefaultPriority, 100),
	).Scan(&id).Error
	return id, err
}

func DeleteVisualProvider(db *gorm.DB, tenantID, vpProviderID string) (bool, error) {
	res := db.Exec(`DELETE FROM visual_parse_providers WHERE vp_provider_id = ? AND tenant_id = ?`, vpProviderID, tenantID)
	return res.RowsAffected > 0, res.Error
}

// ——— 健康检查 + fallback 链 ———

type HealthRecord struct {
	TenantID     string
	ProviderID   string
	ProviderKind string // model | visual
	CheckKind    string // active | passive
	Status       string // up | down
	LatencyMs    *int
	Error        string
	TTLSeconds   *int
}

func RecordHealth(db *gorm.DB, rec HealthRecord) error {
	ttl := healthTTLSeconds
	if rec.TTLSeconds != nil {
		ttl = *rec.TTLSeconds
	}
	var latency any
	if rec.LatencyMs != nil {
		latency = *rec.LatencyMs
	}
	var errStr any
	if rec.Error != "" {
		errStr = rec.Error
	}
	return db.Exec(
		`INSERT INTO provider_health_checks (tenant_id, provider_id, provider_kind, check_kind, status, latency_ms, error, ttl_seconds)
		 VALUES (?,?,?,?,?,?,?,?)`,
		rec.TenantID, rec.ProviderID, rec.ProviderKind, rec.CheckKind, rec.Status, latency, errStr, ttl,
	).Error
}

func isCurrentlyDown(db *gorm.DB, providerID string) (bool, error) {
	var row struct {
		Status     string  `gorm:"column:status"`
		TTLSeconds int     `gorm:"column:ttl_seconds"`
		AgeSec     float64 `gorm:"column:age_sec"`
	}
	err := db.Raw(
		`SELECT status, ttl_seconds, EXTRACT(EPOCH FROM (NOW() - checked_at)) AS age_sec
		 FROM provider_health_checks WHERE provider_id = ? ORDER BY checked_at DESC LIMIT 1`, providerID,
	).Scan(&row).Error
	if err != nil {
		return false, err
	}
	if row.Status == "" {
		return false, nil
	}
	return row.Status == "down" && row.AgeSec < float64(row.TTLSeconds), nil
}

func ResolveModelChain(db *gorm.DB, tenantID string, capability Capability) ([]ProviderConnection, error) {
	var rows []modelProviderRow
	if err := db.Raw(
		`SELECT p.* FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
		 WHERE r.tenant_id = ? AND r.capability = ? AND r.enabled = TRUE AND p.enabled = TRUE
		 ORDER BY r.priority ASC, p.default_priority ASC`, tenantID, capability,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderConnection, 0, len(rows))
	for _, r := range rows {
		down, err := isCurrentlyDown(db, r.ProviderID)
		if err != nil {
			return nil, err
		}
		if down {
			continue
		}
		out = append(out, modelRowToConnection(r))
	}
	return out, nil
}

func ResolveVisualChain(db *gorm.DB, tenantID string) ([]ProviderConnection, error) {
	var rows []visualProviderRow
	if err := db.Raw(
		`SELECT * FROM visual_parse_providers WHERE tenant_id = ? AND enabled = TRUE
		 ORDER BY default_priority ASC, name ASC`, tenantID,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]ProviderConnection, 0, len(rows))
	for _, r := range rows {
		down, err := isCurrentlyDown(db, r.VPProviderID)
		if err != nil {
			return nil, err
		}
		if down {
			continue
		}
		out = append(out, visualRowToConnection(r))
	}
	return out, nil
}

// ——— 配置覆盖校验 ———

type CapabilityCoverage struct {
	Capability Capability `json:"capability"`
	Bound      bool       `json:"bound"`
	HasPrivate bool       `json:"hasPrivate"`
	IsMainLoop bool       `json:"isMainLoop"`
	CanGoLive  bool       `json:"canGoLive"`
}

func ValidateCapabilityCoverage(db *gorm.DB, tenantID string) ([]CapabilityCoverage, error) {
	out := []CapabilityCoverage{}
	for _, cap := range RoutableCapabilities {
		var kinds []string
		if err := db.Raw(
			`SELECT p.deployment_kind FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
			 WHERE r.tenant_id = ? AND r.capability = ? AND r.enabled = TRUE AND p.enabled = TRUE`, tenantID, cap,
		).Scan(&kinds).Error; err != nil {
			return nil, err
		}
		bound := len(kinds) > 0
		hasPrivate := hasPrivateKind(kinds)
		isML := isMainLoop(cap)
		out = append(out, CapabilityCoverage{cap, bound, hasPrivate, isML, bound && (!isML || hasPrivate)})
	}
	var vkinds []string
	if err := db.Raw(`SELECT deployment_kind FROM visual_parse_providers WHERE tenant_id = ? AND enabled = TRUE`, tenantID).Scan(&vkinds).Error; err != nil {
		return nil, err
	}
	vbound := len(vkinds) > 0
	vpriv := hasPrivateKind(vkinds)
	out = append(out, CapabilityCoverage{CapVisualParse, vbound, vpriv, true, vbound && vpriv})
	return out, nil
}

func hasPrivateKind(kinds []string) bool {
	for _, k := range kinds {
		if k == "private" {
			return true
		}
	}
	return false
}
