-- c03-model-and-parse｜模型 Provider 抽象层配置（PRD §16.4 / §18 命名）
-- owner=c03：model_providers / model_routes / provider_health_checks / visual_parse_providers
-- 仅新增表，无破坏性变更；敏感凭据以密文列存储（应用层 AES-256-GCM 加密、读时掩码）

-- 模型 Provider 连接级配置（公网/私有化双入口同表，以 deployment_kind 区分两套字段语义）
CREATE TABLE IF NOT EXISTS model_providers (
  provider_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  name TEXT NOT NULL,
  -- 四类协议（§16.4）
  protocol TEXT NOT NULL CHECK (
    protocol IN ('openai_compat', 'anthropic_messages', 'local_gateway', 'third_party')
  ),
  -- 公网 / 私有化
  deployment_kind TEXT NOT NULL CHECK (deployment_kind IN ('public', 'private')),
  base_url TEXT NOT NULL,
  -- api_key | token 统一以密文列存储；NULL 表示未配置凭据
  credential_cipher TEXT,
  model TEXT NOT NULL,
  timeout_ms INTEGER NOT NULL DEFAULT 30000,
  max_retries INTEGER NOT NULL DEFAULT 1,
  -- 私有化网络访问策略（公网 provider 置 NULL）：allow_all / intranet_only / deny_egress
  network_policy TEXT CHECK (
    network_policy IS NULL OR network_policy IN ('allow_all', 'intranet_only', 'deny_egress')
  ),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  default_priority INTEGER NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_model_providers_tenant
  ON model_providers (tenant_id, deployment_kind, enabled);

-- 用途绑定（capability ↔ provider 多对多，按 priority 排成 fallback 链）
-- visual_parse 不在此表绑定：视觉解析 provider 单独配置于 visual_parse_providers（§16.4 单独配置约束）
CREATE TABLE IF NOT EXISTS model_routes (
  route_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  capability TEXT NOT NULL CHECK (
    capability IN (
      'chat', 'summarize', 'translate', 'embed', 'rerank',
      'term_extract', 'proofread', 'outline_gen'
    )
  ),
  provider_id UUID NOT NULL REFERENCES model_providers(provider_id) ON DELETE CASCADE,
  priority INTEGER NOT NULL DEFAULT 100,
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tenant_id, capability, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_model_routes_cap
  ON model_routes (tenant_id, capability, enabled, priority);

-- 连通性测试（主动）与健康检查（被动标记）结果；provider_kind 覆盖模型与视觉两类 provider，故不设 FK
CREATE TABLE IF NOT EXISTS provider_health_checks (
  check_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  provider_id UUID NOT NULL,
  provider_kind TEXT NOT NULL CHECK (provider_kind IN ('model', 'visual')),
  check_kind TEXT NOT NULL CHECK (check_kind IN ('active', 'passive')),
  status TEXT NOT NULL CHECK (status IN ('up', 'down')),
  latency_ms INTEGER,
  error TEXT,
  ttl_seconds INTEGER NOT NULL DEFAULT 60,
  checked_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_provider_health_latest
  ON provider_health_checks (provider_id, checked_at DESC);

-- 文档视觉解析 provider（公网/私有化双配置；backend 对上层透明）
CREATE TABLE IF NOT EXISTS visual_parse_providers (
  vp_provider_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(tenant_id),
  name TEXT NOT NULL,
  backend_kind TEXT NOT NULL CHECK (
    backend_kind IN ('ocr', 'multimodal_llm', 'layout', 'table', 'third_party_api', 'private_service')
  ),
  deployment_kind TEXT NOT NULL CHECK (deployment_kind IN ('public', 'private')),
  base_url TEXT NOT NULL,
  credential_cipher TEXT,
  model TEXT,
  timeout_ms INTEGER NOT NULL DEFAULT 60000,
  network_policy TEXT CHECK (
    network_policy IS NULL OR network_policy IN ('allow_all', 'intranet_only', 'deny_egress')
  ),
  enabled BOOLEAN NOT NULL DEFAULT FALSE,
  default_priority INTEGER NOT NULL DEFAULT 100,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_visual_parse_providers_tenant
  ON visual_parse_providers (tenant_id, deployment_kind, enabled, default_priority);

-- 新增「模型与评测管理」权限点（§17.7），并授予所有租户的 admin 角色（幂等，适配既有库）
INSERT INTO permissions (name, description)
VALUES ('model:manage', '模型与评测管理')
ON CONFLICT (name) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.role_id, p.permission_id
FROM roles r
CROSS JOIN permissions p
WHERE r.slug = 'admin' AND p.name = 'model:manage'
ON CONFLICT DO NOTHING;
