// c03｜model_providers / model_routes / visual_parse_providers / provider_health_checks 存取
// 含凭据加密/掩码、用途绑定校验（Anthropic 限制）、按 capability 解析 fallback 链（跳过 down provider）。
import type { PoolClient } from "pg";
import type { Capability } from "./capabilities.js";
import { MAIN_LOOP_CAPABILITIES, ROUTABLE_CAPABILITIES } from "./capabilities.js";
import { protocolSupportsCapability } from "./adapters.js";
import { config } from "../../config.js";
import {
  decryptCredential,
  encryptCredential,
  maskCredential,
} from "./crypto.js";
import type {
  DeploymentKind,
  NetworkPolicy,
  ProviderConnection,
} from "./http.js";

export interface ModelProviderInput {
  name: string;
  protocol: string;
  deploymentKind: DeploymentKind;
  baseUrl: string;
  credential?: string | null;
  model: string;
  timeoutMs?: number;
  maxRetries?: number;
  networkPolicy?: NetworkPolicy;
  enabled?: boolean;
  defaultPriority?: number;
}

interface ModelProviderRow {
  provider_id: string;
  tenant_id: string;
  name: string;
  protocol: string;
  deployment_kind: DeploymentKind;
  base_url: string;
  credential_cipher: string | null;
  model: string;
  timeout_ms: number;
  max_retries: number;
  network_policy: NetworkPolicy;
  enabled: boolean;
  default_priority: number;
}

interface VisualProviderRow {
  vp_provider_id: string;
  tenant_id: string;
  name: string;
  backend_kind: string;
  deployment_kind: DeploymentKind;
  base_url: string;
  credential_cipher: string | null;
  model: string | null;
  timeout_ms: number;
  network_policy: NetworkPolicy;
  enabled: boolean;
  default_priority: number;
}

function modelRowToConnection(row: ModelProviderRow): ProviderConnection {
  return {
    providerId: row.provider_id,
    kind: "model",
    name: row.name,
    protocol: row.protocol,
    deploymentKind: row.deployment_kind,
    baseUrl: row.base_url,
    credential: decryptCredential(row.credential_cipher),
    model: row.model,
    timeoutMs: row.timeout_ms,
    maxRetries: row.max_retries,
    networkPolicy: row.network_policy,
  };
}

function visualRowToConnection(row: VisualProviderRow): ProviderConnection {
  return {
    providerId: row.vp_provider_id,
    kind: "visual",
    name: row.name,
    backendKind: row.backend_kind,
    deploymentKind: row.deployment_kind,
    baseUrl: row.base_url,
    credential: decryptCredential(row.credential_cipher),
    model: row.model ?? "",
    timeoutMs: row.timeout_ms,
    maxRetries: 0,
    networkPolicy: row.network_policy,
  };
}

/** 掩码 DTO（task 8.3：凭据不明文返回前端） */
export function maskModelProvider(row: ModelProviderRow) {
  return {
    providerId: row.provider_id,
    name: row.name,
    protocol: row.protocol,
    deploymentKind: row.deployment_kind,
    baseUrl: row.base_url,
    credentialMasked: maskCredential(row.credential_cipher),
    model: row.model,
    timeoutMs: row.timeout_ms,
    maxRetries: row.max_retries,
    networkPolicy: row.network_policy,
    enabled: row.enabled,
    defaultPriority: row.default_priority,
  };
}

export function maskVisualProvider(row: VisualProviderRow) {
  return {
    vpProviderId: row.vp_provider_id,
    name: row.name,
    backendKind: row.backend_kind,
    deploymentKind: row.deployment_kind,
    baseUrl: row.base_url,
    credentialMasked: maskCredential(row.credential_cipher),
    model: row.model,
    networkPolicy: row.network_policy,
    enabled: row.enabled,
    defaultPriority: row.default_priority,
  };
}

// ——— model_providers CRUD ———

export async function listModelProviders(client: PoolClient, tenantId: string) {
  const { rows } = await client.query(
    `SELECT * FROM model_providers WHERE tenant_id = $1 ORDER BY deployment_kind, default_priority, name`,
    [tenantId],
  );
  return (rows as ModelProviderRow[]).map(maskModelProvider);
}

export async function getModelProviderRow(
  client: PoolClient,
  tenantId: string,
  providerId: string,
): Promise<ModelProviderRow | null> {
  const { rows } = await client.query(
    `SELECT * FROM model_providers WHERE provider_id = $1 AND tenant_id = $2`,
    [providerId, tenantId],
  );
  return (rows[0] as ModelProviderRow) ?? null;
}

/** 单 provider → 已解密 ProviderConnection（供连通性测试/视觉解析使用，不回前端）。 */
export async function getModelConnection(
  client: PoolClient,
  tenantId: string,
  providerId: string,
): Promise<ProviderConnection | null> {
  const row = await getModelProviderRow(client, tenantId, providerId);
  return row ? modelRowToConnection(row) : null;
}

export async function getVisualConnection(
  client: PoolClient,
  tenantId: string,
  vpProviderId: string,
): Promise<ProviderConnection | null> {
  const { rows } = await client.query(
    `SELECT * FROM visual_parse_providers WHERE vp_provider_id = $1 AND tenant_id = $2`,
    [vpProviderId, tenantId],
  );
  return rows[0] ? visualRowToConnection(rows[0] as VisualProviderRow) : null;
}

export async function createModelProvider(
  client: PoolClient,
  tenantId: string,
  input: ModelProviderInput,
): Promise<string> {
  const { rows } = await client.query(
    `INSERT INTO model_providers (
       tenant_id, name, protocol, deployment_kind, base_url, credential_cipher,
       model, timeout_ms, max_retries, network_policy, enabled, default_priority
     ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
     RETURNING provider_id`,
    [
      tenantId,
      input.name,
      input.protocol,
      input.deploymentKind,
      input.baseUrl,
      encryptCredential(input.credential),
      input.model,
      input.timeoutMs ?? 30000,
      input.maxRetries ?? 1,
      input.deploymentKind === "private"
        ? input.networkPolicy ?? "intranet_only"
        : null,
      input.enabled ?? false,
      input.defaultPriority ?? 100,
    ],
  );
  return rows[0].provider_id as string;
}

export async function updateModelProvider(
  client: PoolClient,
  tenantId: string,
  providerId: string,
  patch: Partial<ModelProviderInput>,
): Promise<boolean> {
  const fields: string[] = [];
  const params: unknown[] = [];
  const set = (col: string, val: unknown) => {
    params.push(val);
    fields.push(`${col} = $${params.length}`);
  };
  if (patch.name !== undefined) set("name", patch.name);
  if (patch.baseUrl !== undefined) set("base_url", patch.baseUrl);
  if (patch.credential !== undefined) set("credential_cipher", encryptCredential(patch.credential));
  if (patch.model !== undefined) set("model", patch.model);
  if (patch.timeoutMs !== undefined) set("timeout_ms", patch.timeoutMs);
  if (patch.maxRetries !== undefined) set("max_retries", patch.maxRetries);
  if (patch.networkPolicy !== undefined) set("network_policy", patch.networkPolicy);
  if (patch.enabled !== undefined) set("enabled", patch.enabled);
  if (patch.defaultPriority !== undefined) set("default_priority", patch.defaultPriority);
  if (!fields.length) return false;
  set("updated_at", new Date());
  params.push(providerId, tenantId);
  const res = await client.query(
    `UPDATE model_providers SET ${fields.join(", ")}
     WHERE provider_id = $${params.length - 1} AND tenant_id = $${params.length}`,
    params,
  );
  return (res.rowCount ?? 0) > 0;
}

export async function deleteModelProvider(
  client: PoolClient,
  tenantId: string,
  providerId: string,
): Promise<boolean> {
  const res = await client.query(
    `DELETE FROM model_providers WHERE provider_id = $1 AND tenant_id = $2`,
    [providerId, tenantId],
  );
  return (res.rowCount ?? 0) > 0;
}

// ——— model_routes（用途绑定） ———

export class RouteBindError extends Error {}

export async function bindRoute(
  client: PoolClient,
  tenantId: string,
  capability: Capability,
  providerId: string,
  priority: number,
): Promise<void> {
  if (!ROUTABLE_CAPABILITIES.includes(capability)) {
    throw new RouteBindError(
      `${capability} 不可经 model_routes 绑定（视觉解析须配置于 visual_parse_providers）`,
    );
  }
  const provider = await getModelProviderRow(client, tenantId, providerId);
  if (!provider) throw new RouteBindError("provider 不存在或不属于当前租户");
  // task 2.3：Anthropic 协议仅可绑定生成类能力
  if (!protocolSupportsCapability(provider.protocol, capability)) {
    throw new RouteBindError(
      `${provider.protocol} 协议仅用于生成类能力，Embedding/Rerank/视觉解析须单独配置 provider`,
    );
  }
  await client.query(
    `INSERT INTO model_routes (tenant_id, capability, provider_id, priority, enabled)
     VALUES ($1,$2,$3,$4,TRUE)
     ON CONFLICT (tenant_id, capability, provider_id)
     DO UPDATE SET priority = EXCLUDED.priority, enabled = TRUE`,
    [tenantId, capability, providerId, priority],
  );
}

export async function listRoutes(client: PoolClient, tenantId: string) {
  const { rows } = await client.query(
    `SELECT r.route_id, r.capability, r.provider_id, r.priority, r.enabled, p.name AS provider_name,
            p.deployment_kind, p.protocol, p.enabled AS provider_enabled
     FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
     WHERE r.tenant_id = $1 ORDER BY r.capability, r.priority`,
    [tenantId],
  );
  return rows;
}

export async function unbindRoute(
  client: PoolClient,
  tenantId: string,
  routeId: string,
): Promise<boolean> {
  const res = await client.query(
    `DELETE FROM model_routes WHERE route_id = $1 AND tenant_id = $2`,
    [routeId, tenantId],
  );
  return (res.rowCount ?? 0) > 0;
}

// ——— visual_parse_providers CRUD ———

export interface VisualProviderInput {
  name: string;
  backendKind: string;
  deploymentKind: DeploymentKind;
  baseUrl: string;
  credential?: string | null;
  model?: string | null;
  timeoutMs?: number;
  networkPolicy?: NetworkPolicy;
  enabled?: boolean;
  defaultPriority?: number;
}

export async function listVisualProviders(client: PoolClient, tenantId: string) {
  const { rows } = await client.query(
    `SELECT * FROM visual_parse_providers WHERE tenant_id = $1 ORDER BY deployment_kind, default_priority, name`,
    [tenantId],
  );
  return (rows as VisualProviderRow[]).map(maskVisualProvider);
}

export async function createVisualProvider(
  client: PoolClient,
  tenantId: string,
  input: VisualProviderInput,
): Promise<string> {
  const { rows } = await client.query(
    `INSERT INTO visual_parse_providers (
       tenant_id, name, backend_kind, deployment_kind, base_url, credential_cipher,
       model, timeout_ms, network_policy, enabled, default_priority
     ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
     RETURNING vp_provider_id`,
    [
      tenantId,
      input.name,
      input.backendKind,
      input.deploymentKind,
      input.baseUrl,
      encryptCredential(input.credential),
      input.model ?? null,
      input.timeoutMs ?? 60000,
      input.deploymentKind === "private"
        ? input.networkPolicy ?? "intranet_only"
        : null,
      input.enabled ?? false,
      input.defaultPriority ?? 100,
    ],
  );
  return rows[0].vp_provider_id as string;
}

export async function deleteVisualProvider(
  client: PoolClient,
  tenantId: string,
  vpProviderId: string,
): Promise<boolean> {
  const res = await client.query(
    `DELETE FROM visual_parse_providers WHERE vp_provider_id = $1 AND tenant_id = $2`,
    [vpProviderId, tenantId],
  );
  return (res.rowCount ?? 0) > 0;
}

// ——— 健康检查 ———

export interface HealthRecord {
  tenantId: string;
  providerId: string;
  providerKind: "model" | "visual";
  checkKind: "active" | "passive";
  status: "up" | "down";
  latencyMs?: number | null;
  error?: string | null;
  ttlSeconds?: number;
}

export async function recordHealth(client: PoolClient, rec: HealthRecord): Promise<void> {
  await client.query(
    `INSERT INTO provider_health_checks
       (tenant_id, provider_id, provider_kind, check_kind, status, latency_ms, error, ttl_seconds)
     VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
    [
      rec.tenantId,
      rec.providerId,
      rec.providerKind,
      rec.checkKind,
      rec.status,
      rec.latencyMs ?? null,
      rec.error ?? null,
      rec.ttlSeconds ?? config.model.healthTtlSeconds,
    ],
  );
}

/** provider 当前是否应被路由跳过：最近一次健康记录为 down 且仍在 TTL 内 → 跳过。 */
async function isCurrentlyDown(client: PoolClient, providerId: string): Promise<boolean> {
  const { rows } = await client.query(
    `SELECT status, ttl_seconds, checked_at,
            EXTRACT(EPOCH FROM (NOW() - checked_at)) AS age_sec
     FROM provider_health_checks
     WHERE provider_id = $1 ORDER BY checked_at DESC LIMIT 1`,
    [providerId],
  );
  if (!rows.length) return false;
  const r = rows[0];
  return r.status === "down" && Number(r.age_sec) < Number(r.ttl_seconds);
}

// ——— fallback 链解析 ———

/** 某 capability 的 fallback 链：enabled provider 按 priority 升序，跳过健康 down（TTL 内）的 provider。 */
export async function resolveModelChain(
  client: PoolClient,
  tenantId: string,
  capability: Capability,
): Promise<ProviderConnection[]> {
  const { rows } = await client.query(
    `SELECT p.*, r.priority AS route_priority
     FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
     WHERE r.tenant_id = $1 AND r.capability = $2 AND r.enabled = TRUE AND p.enabled = TRUE
     ORDER BY r.priority ASC, p.default_priority ASC`,
    [tenantId, capability],
  );
  const out: ProviderConnection[] = [];
  for (const row of rows as ModelProviderRow[]) {
    if (await isCurrentlyDown(client, row.provider_id)) continue;
    out.push(modelRowToConnection(row));
  }
  return out;
}

export async function resolveVisualChain(
  client: PoolClient,
  tenantId: string,
): Promise<ProviderConnection[]> {
  const { rows } = await client.query(
    `SELECT * FROM visual_parse_providers
     WHERE tenant_id = $1 AND enabled = TRUE
     ORDER BY default_priority ASC, name ASC`,
    [tenantId],
  );
  const out: ProviderConnection[] = [];
  for (const row of rows as VisualProviderRow[]) {
    if (await isCurrentlyDown(client, row.vp_provider_id)) continue;
    out.push(visualRowToConnection(row));
  }
  return out;
}

// ——— 配置层校验（task 8.4）：每个被依赖能力至少一条 enabled provider；主闭环能力至少一条私有化路径 ———

export interface CapabilityCoverage {
  capability: Capability;
  bound: boolean;
  hasPrivate: boolean;
  isMainLoop: boolean;
  canGoLive: boolean;
}

export async function validateCapabilityCoverage(
  client: PoolClient,
  tenantId: string,
): Promise<CapabilityCoverage[]> {
  const out: CapabilityCoverage[] = [];
  for (const cap of ROUTABLE_CAPABILITIES) {
    const { rows } = await client.query(
      `SELECT p.deployment_kind
       FROM model_routes r JOIN model_providers p ON p.provider_id = r.provider_id
       WHERE r.tenant_id = $1 AND r.capability = $2 AND r.enabled = TRUE AND p.enabled = TRUE`,
      [tenantId, cap],
    );
    const bound = rows.length > 0;
    const hasPrivate = rows.some((r) => r.deployment_kind === "private");
    const isMainLoop = MAIN_LOOP_CAPABILITIES.includes(cap);
    out.push({ capability: cap, bound, hasPrivate, isMainLoop, canGoLive: bound && (!isMainLoop || hasPrivate) });
  }
  // visual_parse（单独配置于 visual_parse_providers，属主闭环、须有私有化路径）
  const { rows: vrows } = await client.query(
    `SELECT deployment_kind FROM visual_parse_providers WHERE tenant_id = $1 AND enabled = TRUE`,
    [tenantId],
  );
  const vbound = vrows.length > 0;
  const vpriv = vrows.some((r) => r.deployment_kind === "private");
  out.push({
    capability: "visual_parse",
    bound: vbound,
    hasPrivate: vpriv,
    isMainLoop: true,
    canGoLive: vbound && vpriv,
  });
  return out;
}

export { modelRowToConnection, visualRowToConnection };
