// c03｜用途路由 + 优先级 fallback + 公网前置脱敏门控 + 出网网关编排（design D2/D4/D6）
// 唯一对上层暴露的调用入口：invokeChat/invokeTranslate/.../invokeEmbed/invokeRerank。
// 上层不感知协议/provider/公网私有化；fallback、脱敏门禁、切换均落 audit_logs。
import type { PoolClient } from "pg";
import { writeAudit } from "../audit.js";
import type {
  Capability,
  EmbedRequest,
  EmbedResponse,
  GenerationRequest,
  GenerationResponse,
  RerankRequest,
  RerankResponse,
} from "./capabilities.js";
import { getAdapter, type ModelAdapter } from "./adapters.js";
import { isFallbackable, ProviderError, type ProviderConnection } from "./http.js";
import { redactionGateway } from "./redaction-gateway.js";
import { recordHealth, resolveModelChain } from "./provider-store.js";

export interface InvokeContext {
  tenantId: string;
  actorId?: string | null;
  actorRole?: string | null;
}

export class CapabilityUnavailableError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "CapabilityUnavailableError";
  }
}

interface ChainCall<T> {
  capability: Capability;
  /** 公网外发前交 c09 redaction-gateway 判定的原文 */
  outboundText: string;
  run(adapter: ModelAdapter, conn: ProviderConnection): Promise<T>;
}

async function runChain<T>(
  client: PoolClient,
  ctx: InvokeContext,
  call: ChainCall<T>,
): Promise<T> {
  const chain = await resolveModelChain(client, ctx.tenantId, call.capability);
  if (!chain.length) {
    await writeAudit(client, {
      tenantId: ctx.tenantId,
      actorId: ctx.actorId,
      actorRole: ctx.actorRole,
      actionType: "model_route_missing",
      targetType: "capability",
      targetId: call.capability,
      result: "失败",
      failureReason: "该用途未配置可用模型",
    });
    throw new CapabilityUnavailableError(`该用途未配置可用模型：${call.capability}`);
  }

  for (let i = 0; i < chain.length; i++) {
    const conn = chain[i];
    const next = chain[i + 1] ?? null;

    // D6：公网 provider 必须先过 c09 脱敏门禁；未通过则跳过公网，尝试下一（私有化）provider
    if (conn.deploymentKind === "public") {
      const verdict = await redactionGateway.evaluate({
        tenantId: ctx.tenantId,
        text: call.outboundText,
      });
      if (!verdict.available || !verdict.passed) {
        await writeAudit(client, {
          tenantId: ctx.tenantId,
          actorId: ctx.actorId,
          actorRole: ctx.actorRole,
          actionType: "model_redaction_block",
          targetType: "capability",
          targetId: call.capability,
          result: "失败",
          failureReason: verdict.reason,
          metadata: {
            provider: conn.name,
            providerId: conn.providerId,
            deploymentKind: "public",
            switchTo: next?.name ?? null,
            confidence: verdict.confidence,
          },
        });
        continue; // 跳过公网，绝不在未脱敏情况下外发
      }
      // c09 落地后：verdict.redactedText 即脱敏后内容，应替换 payload 再外发（本期不可达）
    }

    const adapter = getAdapter(conn.protocol ?? "");
    let lastError: ProviderError | null = null;
    for (let attempt = 0; attempt <= conn.maxRetries; attempt++) {
      try {
        const result = await call.run(adapter, conn);
        await writeAudit(client, {
          tenantId: ctx.tenantId,
          actorId: ctx.actorId,
          actorRole: ctx.actorRole,
          actionType: "model_invoke",
          targetType: "capability",
          targetId: call.capability,
          result: "成功",
          metadata: {
            provider: conn.name,
            providerId: conn.providerId,
            deploymentKind: conn.deploymentKind,
            attempt,
          },
        });
        return result;
      } catch (e) {
        const err =
          e instanceof ProviderError ? e : new ProviderError("unknown", (e as Error).message);
        lastError = err;
        if (!isFallbackable(err.errorClass)) {
          // 不可 fallback（鉴权错/超上限/内容安全）→ 直接终止链路上抛
          await writeAudit(client, {
            tenantId: ctx.tenantId,
            actorId: ctx.actorId,
            actorRole: ctx.actorRole,
            actionType: "model_invoke",
            targetType: "capability",
            targetId: call.capability,
            result: "失败",
            failureReason: err.message,
            metadata: { provider: conn.name, providerId: conn.providerId, errorClass: err.errorClass },
          });
          throw err;
        }
        // 可 fallback：继续该 provider 的重试，耗尽后切换
      }
    }

    // 该 provider 重试耗尽 → 被动标记 down + 记录 fallback 切换（四要素：provider/失败原因/切换目标/时间戳）
    await recordHealth(client, {
      tenantId: ctx.tenantId,
      providerId: conn.providerId,
      providerKind: "model",
      checkKind: "passive",
      status: "down",
      error: lastError?.message ?? "调用失败",
    });
    await writeAudit(client, {
      tenantId: ctx.tenantId,
      actorId: ctx.actorId,
      actorRole: ctx.actorRole,
      actionType: "model_fallback",
      targetType: "capability",
      targetId: call.capability,
      result: "失败",
      failureReason: lastError?.message ?? "调用失败",
      metadata: {
        fromProvider: conn.name,
        fromProviderId: conn.providerId,
        errorClass: lastError?.errorClass ?? "unknown",
        toProvider: next?.name ?? null,
        toProviderId: next?.providerId ?? null,
        capability: call.capability,
      },
    });
  }

  // 全链路失败/被跳过 → 明确不可用错误（绝不静默成功）
  await writeAudit(client, {
    tenantId: ctx.tenantId,
    actorId: ctx.actorId,
    actorRole: ctx.actorRole,
    actionType: "model_unavailable",
    targetType: "capability",
    targetId: call.capability,
    result: "失败",
    failureReason: "该用途所有 provider 依次失败或被脱敏门禁拒绝",
  });
  throw new CapabilityUnavailableError(
    `该用途所有 provider 均不可用：${call.capability}`,
  );
}

// ——— 对上层暴露的 capability 调用入口 ———

export function invokeGeneration(
  client: PoolClient,
  capability: Capability,
  req: GenerationRequest,
  ctx: InvokeContext,
): Promise<GenerationResponse> {
  return runChain(client, ctx, {
    capability,
    outboundText: req.messages.map((m) => m.content).join("\n"),
    run: (adapter, conn) => adapter.generate(req, conn),
  });
}

export function invokeEmbed(
  client: PoolClient,
  req: EmbedRequest,
  ctx: InvokeContext,
): Promise<EmbedResponse> {
  return runChain(client, ctx, {
    capability: "embed",
    outboundText: req.input.join("\n"),
    run: (adapter, conn) => adapter.embed(req, conn),
  });
}

export function invokeRerank(
  client: PoolClient,
  req: RerankRequest,
  ctx: InvokeContext,
): Promise<RerankResponse> {
  return runChain(client, ctx, {
    capability: "rerank",
    outboundText: [req.query, ...req.documents].join("\n"),
    run: (adapter, conn) => adapter.rerank(req, conn),
  });
}
