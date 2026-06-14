// c03｜管理后台「模型与评测管理」配置入口（§17.7 配置部分；tasks 8.x）
// 仅 model:manage 权限角色可读写；租户隔离；凭据掩码返回；写操作落审计。不含评测项（Evals 属 V1.1）。
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";
import type { Capability } from "../services/model/capabilities.js";
import { ROUTABLE_CAPABILITIES } from "../services/model/capabilities.js";
import {
  RouteBindError,
  bindRoute,
  createModelProvider,
  createVisualProvider,
  deleteModelProvider,
  deleteVisualProvider,
  listModelProviders,
  listRoutes,
  listVisualProviders,
  unbindRoute,
  updateModelProvider,
  validateCapabilityCoverage,
} from "../services/model/provider-store.js";
import {
  testModelConnectivity,
  testVisualConnectivity,
} from "../services/model/health.js";
import { retryJob } from "../services/parsing/event-consumer.js";

const PROTOCOLS = new Set(["openai_compat", "anthropic_messages", "local_gateway", "third_party"]);
const DEPLOY_KINDS = new Set(["public", "private"]);
const BACKEND_KINDS = new Set([
  "ocr",
  "multimodal_llm",
  "layout",
  "table",
  "third_party_api",
  "private_service",
]);

export async function registerAdminModelRoutes(app: FastifyInstance) {
  // 仅 model:manage 可访问；越权访问/修改落审计（task 8.2）
  app.addHook("preHandler", async (request, reply) => {
    if (!request.url.startsWith("/api/admin/models")) return;
    const user = requireAuth(request);
    if (!user.permissions.includes("model:manage")) {
      const client = await pool.connect();
      try {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: "model_config_access_denied",
          targetType: "model_config",
          result: "失败",
          failureReason: "无模型与评测管理权限",
        });
      } finally {
        client.release();
      }
      return reply.status(403).send({ error: "无模型与评测管理权限" });
    }
  });

  // ——— model providers ———
  app.get("/api/admin/models/providers", async (request) => {
    const user = requireAuth(request);
    const client = await pool.connect();
    try {
      return { providers: await listModelProviders(client, user.tenantId) };
    } finally {
      client.release();
    }
  });

  app.post("/api/admin/models/providers", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as Record<string, unknown>;
    if (!PROTOCOLS.has(String(body.protocol)) || !DEPLOY_KINDS.has(String(body.deploymentKind))) {
      return reply.status(400).send({ error: "protocol / deploymentKind 非法" });
    }
    if (!body.name || !body.baseUrl || !body.model) {
      return reply.status(400).send({ error: "缺少 name / baseUrl / model" });
    }
    const client = await pool.connect();
    try {
      const providerId = await createModelProvider(client, user.tenantId, {
        name: String(body.name),
        protocol: String(body.protocol),
        deploymentKind: body.deploymentKind as "public" | "private",
        baseUrl: String(body.baseUrl),
        credential: body.credential ? String(body.credential) : null,
        model: String(body.model),
        timeoutMs: body.timeoutMs ? Number(body.timeoutMs) : undefined,
        maxRetries: body.maxRetries !== undefined ? Number(body.maxRetries) : undefined,
        networkPolicy: (body.networkPolicy as never) ?? null,
        enabled: Boolean(body.enabled),
        defaultPriority: body.defaultPriority !== undefined ? Number(body.defaultPriority) : undefined,
      });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "model_provider_create",
        targetType: "model_provider",
        targetId: providerId,
        result: "成功",
        metadata: { name: body.name, protocol: body.protocol, deploymentKind: body.deploymentKind },
      });
      return { providerId };
    } finally {
      client.release();
    }
  });

  app.patch("/api/admin/models/providers/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as Record<string, unknown>;
    const client = await pool.connect();
    try {
      const ok = await updateModelProvider(client, user.tenantId, id, {
        name: body.name !== undefined ? String(body.name) : undefined,
        baseUrl: body.baseUrl !== undefined ? String(body.baseUrl) : undefined,
        credential: body.credential !== undefined ? String(body.credential) : undefined,
        model: body.model !== undefined ? String(body.model) : undefined,
        timeoutMs: body.timeoutMs !== undefined ? Number(body.timeoutMs) : undefined,
        maxRetries: body.maxRetries !== undefined ? Number(body.maxRetries) : undefined,
        networkPolicy: body.networkPolicy as never,
        enabled: body.enabled !== undefined ? Boolean(body.enabled) : undefined,
        defaultPriority: body.defaultPriority !== undefined ? Number(body.defaultPriority) : undefined,
      });
      if (!ok) return reply.status(404).send({ error: "provider 不存在" });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "model_provider_update",
        targetType: "model_provider",
        targetId: id,
        result: "成功",
      });
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.delete("/api/admin/models/providers/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const ok = await deleteModelProvider(client, user.tenantId, id);
      if (!ok) return reply.status(404).send({ error: "provider 不存在" });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "model_provider_delete",
        targetType: "model_provider",
        targetId: id,
        result: "成功",
      });
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.post("/api/admin/models/providers/:id/test", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as { capability?: string };
    const capability = (body.capability ?? "chat") as Capability;
    const client = await pool.connect();
    try {
      const result = await testModelConnectivity(client, user.tenantId, id, capability);
      return { capability, ...result };
    } finally {
      client.release();
    }
  });

  // ——— routes（用途绑定） ———
  app.get("/api/admin/models/routes", async (request) => {
    const user = requireAuth(request);
    const client = await pool.connect();
    try {
      return { routes: await listRoutes(client, user.tenantId) };
    } finally {
      client.release();
    }
  });

  app.post("/api/admin/models/routes", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as { capability?: string; providerId?: string; priority?: number };
    if (!body.capability || !ROUTABLE_CAPABILITIES.includes(body.capability as Capability)) {
      return reply.status(400).send({ error: "capability 非法（视觉解析须配置于 visual-providers）" });
    }
    if (!body.providerId) return reply.status(400).send({ error: "缺少 providerId" });
    const client = await pool.connect();
    try {
      await bindRoute(
        client,
        user.tenantId,
        body.capability as Capability,
        body.providerId,
        body.priority ?? 100,
      );
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "model_route_bind",
        targetType: "model_route",
        targetId: `${body.capability}:${body.providerId}`,
        result: "成功",
      });
      return { ok: true };
    } catch (e) {
      if (e instanceof RouteBindError) {
        return reply.status(400).send({ error: e.message });
      }
      throw e;
    } finally {
      client.release();
    }
  });

  app.delete("/api/admin/models/routes/:routeId", async (request, reply) => {
    const user = requireAuth(request);
    const { routeId } = request.params as { routeId: string };
    const client = await pool.connect();
    try {
      const ok = await unbindRoute(client, user.tenantId, routeId);
      if (!ok) return reply.status(404).send({ error: "route 不存在" });
      return { ok: true };
    } finally {
      client.release();
    }
  });

  // ——— visual parse providers ———
  app.get("/api/admin/models/visual-providers", async (request) => {
    const user = requireAuth(request);
    const client = await pool.connect();
    try {
      return { providers: await listVisualProviders(client, user.tenantId) };
    } finally {
      client.release();
    }
  });

  app.post("/api/admin/models/visual-providers", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as Record<string, unknown>;
    if (!BACKEND_KINDS.has(String(body.backendKind)) || !DEPLOY_KINDS.has(String(body.deploymentKind))) {
      return reply.status(400).send({ error: "backendKind / deploymentKind 非法" });
    }
    if (!body.name || !body.baseUrl) {
      return reply.status(400).send({ error: "缺少 name / baseUrl" });
    }
    const client = await pool.connect();
    try {
      const vpProviderId = await createVisualProvider(client, user.tenantId, {
        name: String(body.name),
        backendKind: String(body.backendKind),
        deploymentKind: body.deploymentKind as "public" | "private",
        baseUrl: String(body.baseUrl),
        credential: body.credential ? String(body.credential) : null,
        model: body.model ? String(body.model) : null,
        timeoutMs: body.timeoutMs ? Number(body.timeoutMs) : undefined,
        networkPolicy: (body.networkPolicy as never) ?? null,
        enabled: Boolean(body.enabled),
        defaultPriority: body.defaultPriority !== undefined ? Number(body.defaultPriority) : undefined,
      });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "visual_provider_create",
        targetType: "visual_parse_provider",
        targetId: vpProviderId,
        result: "成功",
        metadata: { name: body.name, backendKind: body.backendKind, deploymentKind: body.deploymentKind },
      });
      return { vpProviderId };
    } finally {
      client.release();
    }
  });

  app.delete("/api/admin/models/visual-providers/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const ok = await deleteVisualProvider(client, user.tenantId, id);
      if (!ok) return reply.status(404).send({ error: "视觉解析 provider 不存在" });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "visual_provider_delete",
        targetType: "visual_parse_provider",
        targetId: id,
        result: "成功",
      });
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.post("/api/admin/models/visual-providers/:id/test", async (request) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      return await testVisualConnectivity(client, user.tenantId, id);
    } finally {
      client.release();
    }
  });

  // ——— 失败解析作业重试（task 7.5；管理员触发，落审计） ———
  app.post("/api/admin/models/parse-jobs/:jobId/retry", async (request, reply) => {
    const user = requireAuth(request);
    const { jobId } = request.params as { jobId: string };
    const client = await pool.connect();
    try {
      const ok = await retryJob(
        client,
        user.tenantId,
        jobId,
        user.userId,
        user.roleSlugs.join(","),
      );
      if (!ok) return reply.status(404).send({ error: "失败作业不存在或不属于当前租户" });
      return { ok: true, message: "已重置为 pending，下一轮解析将重新执行" };
    } finally {
      client.release();
    }
  });

  // ——— 配置覆盖校验（task 8.4） ———
  app.get("/api/admin/models/coverage", async (request) => {
    const user = requireAuth(request);
    const client = await pool.connect();
    try {
      const coverage = await validateCapabilityCoverage(client, user.tenantId);
      return {
        coverage,
        allCanGoLive: coverage.every((c) => c.canGoLive),
        mainLoopHasPrivate: coverage.filter((c) => c.isMainLoop).every((c) => c.hasPrivate),
      };
    } finally {
      client.release();
    }
  });
}
