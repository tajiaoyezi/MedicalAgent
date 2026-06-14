// c03｜连通性测试（主动）与健康检查（被动标记 + TTL）（design D3，tasks 5.x）
import type { PoolClient } from "pg";
import type { Capability } from "./capabilities.js";
import { isGenerationCapability } from "./capabilities.js";
import { getAdapter } from "./adapters.js";
import { providerFetch, ProviderError, type ProviderConnection } from "./http.js";
import {
  getModelConnection,
  getVisualConnection,
  recordHealth,
} from "./provider-store.js";

export interface ConnectivityResult {
  status: "up" | "down";
  latencyMs: number;
  error?: string;
}

/** 对生成类发最小 chat、对 Embedding/Rerank 发对应轻量探针；结果写 provider_health_checks（active）。 */
async function probeModel(
  conn: ProviderConnection,
  capability: Capability,
): Promise<void> {
  const adapter = getAdapter(conn.protocol ?? "");
  if (capability === "embed") {
    await adapter.embed({ input: ["ping"] }, conn);
  } else if (capability === "rerank") {
    await adapter.rerank({ query: "ping", documents: ["a", "b"] }, conn);
  } else if (isGenerationCapability(capability)) {
    await adapter.generate({ messages: [{ role: "user", content: "ping" }] }, conn);
  } else {
    throw new ProviderError("unknown", `不支持对 ${capability} 做连通性探针`);
  }
}

export async function testModelConnectivity(
  client: PoolClient,
  tenantId: string,
  providerId: string,
  capability: Capability,
): Promise<ConnectivityResult> {
  const conn = await getModelConnection(client, tenantId, providerId);
  if (!conn) return { status: "down", latencyMs: 0, error: "provider 不存在或不属于当前租户" };

  const start = Date.now();
  try {
    await probeModel(conn, capability);
    const latencyMs = Date.now() - start;
    await recordHealth(client, {
      tenantId,
      providerId,
      providerKind: "model",
      checkKind: "active",
      status: "up",
      latencyMs,
    });
    return { status: "up", latencyMs };
  } catch (e) {
    const latencyMs = Date.now() - start;
    const error = (e as Error).message;
    await recordHealth(client, {
      tenantId,
      providerId,
      providerKind: "model",
      checkKind: "active",
      status: "down",
      latencyMs,
      error,
    });
    return { status: "down", latencyMs, error };
  }
}

export async function testVisualConnectivity(
  client: PoolClient,
  tenantId: string,
  vpProviderId: string,
): Promise<ConnectivityResult> {
  const conn = await getVisualConnection(client, tenantId, vpProviderId);
  if (!conn) return { status: "down", latencyMs: 0, error: "视觉解析 provider 不存在或不属于当前租户" };

  const start = Date.now();
  try {
    // 轻量探针：仅验证可达 + 鉴权（不发送 PHI），mock/真实服务返回 200 即视为连通
    await providerFetch(
      conn,
      "parse",
      { probe: true },
      conn.credential ? { authorization: `Bearer ${conn.credential}` } : {},
    );
    const latencyMs = Date.now() - start;
    await recordHealth(client, {
      tenantId,
      providerId: vpProviderId,
      providerKind: "visual",
      checkKind: "active",
      status: "up",
      latencyMs,
    });
    return { status: "up", latencyMs };
  } catch (e) {
    const latencyMs = Date.now() - start;
    const error = (e as Error).message;
    await recordHealth(client, {
      tenantId,
      providerId: vpProviderId,
      providerKind: "visual",
      checkKind: "active",
      status: "down",
      latencyMs,
      error,
    });
    return { status: "down", latencyMs, error };
  }
}
