import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";
import {
  resolveEffectivePermission,
  type DocumentRow,
} from "../services/document-permissions.js";
import {
  categorizeBridgeMethod,
  isBridgeMethodAllowed,
  isTextExportMethod,
  requiresRevisionCheck,
} from "../services/bridge-security.js";
import {
  armWritebackSaveIntent,
  createSaveIntent,
  getSessionByBridgeToken,
  touchEditorSession,
} from "../services/editor-sessions.js";

export async function registerBridgeRoutes(app: FastifyInstance) {
  app.post("/api/bridge/authorize", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as {
      bridgeToken?: string;
      method?: string;
      expectedRevision?: string;
      writebackSource?: string;
      payload?: Record<string, unknown>;
    };

    const { bridgeToken, method, expectedRevision, writebackSource } = body;
    if (!bridgeToken || !method) {
      return reply.status(400).send({ error: "缺少 bridgeToken 或 method" });
    }

    const session = getSessionByBridgeToken(bridgeToken);
    if (!session) {
      return reply.status(401).send({ error: "无效或过期 token", permitted: false });
    }

    if (session.tenantId !== user.tenantId || session.userId !== user.userId) {
      const client = await pool.connect();
      try {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: `bridge:${method}`,
          targetType: "document",
          targetId: session.documentId,
          result: "失败",
          failureReason: "跨租户或会话不匹配",
        });
      } finally {
        client.release();
      }
      return reply.status(403).send({ error: "跨租户调用被拒绝", permitted: false });
    }

    const category = categorizeBridgeMethod(method);
    if (!category) {
      return reply.status(400).send({ error: "未知 Bridge 方法" });
    }

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        `SELECT * FROM documents WHERE document_id = $1`,
        [session.documentId],
      );
      if (!docRes.rows.length || docRes.rows[0].is_deleted) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: `bridge:${method}`,
          targetType: "document",
          targetId: session.documentId,
          result: "失败",
          failureReason: "文档不存在",
        });
        return reply.status(404).send({ error: "文档不存在", permitted: false });
      }

      const doc = docRes.rows[0] as DocumentRow;
      if (doc.tenant_id !== user.tenantId) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: `bridge:${method}`,
          targetType: "document",
          targetId: session.documentId,
          result: "失败",
          failureReason: "跨租户",
        });
        return reply.status(403).send({ error: "跨租户调用被拒绝", permitted: false });
      }

      const level = await resolveEffectivePermission(client, user, doc);
      if (!isBridgeMethodAllowed(category, level)) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: `bridge:${method}`,
          targetType: "document",
          targetId: session.documentId,
          result: "失败",
          failureReason: "权限不足",
        });
        return reply.status(403).send({ error: "越权操作被拒绝", permitted: false });
      }

      if (requiresRevisionCheck(method)) {
        if (!expectedRevision) {
          return reply.status(400).send({
            error: "写回类方法必须提供 expectedRevision",
            permitted: false,
          });
        }
        if (expectedRevision !== session.revision) {
          await writeAudit(client, {
            tenantId: user.tenantId,
            actorId: user.userId,
            actorRole: user.roleSlugs.join(","),
            actionType: `bridge:${method}`,
            targetType: "document",
            targetId: session.documentId,
            result: "失败",
            failureReason: "文档已更新，请重新读取上下文",
          });
          return reply.status(409).send({
            error: "文档已更新，请重新读取上下文",
            permitted: false,
            staleRevision: true,
          });
        }
      }

      touchEditorSession(session);

      let saveIntentId: string | undefined;
      if (method === "saveDocument" && writebackSource) {
        saveIntentId = createSaveIntent(session, writebackSource);
      }

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: `bridge:${method}`,
        targetType: "document",
        targetId: session.documentId,
        result: "成功",
      });

      const response: Record<string, unknown> = {
        permitted: true,
        revision: session.revision,
        documentId: session.documentId,
        docKey: session.documentKey,
      };

      if (saveIntentId) {
        response.saveIntentId = saveIntentId;
      }

      if (isTextExportMethod(method)) {
        response.redactionGatewayAnchor = {
          anchorType: "c09_redaction_gateway",
          exportKind: "document_text",
          documentId: session.documentId,
          versionId: session.versionId,
        };
        response.isOriginalTextExport = true;
      }

      return response;
    } finally {
      client.release();
    }
  });

  app.post("/api/bridge/arm-writeback-save", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as {
      bridgeToken?: string;
      saveIntentId?: string;
    };

    const { bridgeToken, saveIntentId } = body;
    if (!bridgeToken || !saveIntentId) {
      return reply.status(400).send({ error: "缺少 bridgeToken 或 saveIntentId" });
    }

    const session = getSessionByBridgeToken(bridgeToken);
    if (!session) {
      return reply.status(401).send({ error: "无效或过期 token" });
    }

    if (session.tenantId !== user.tenantId || session.userId !== user.userId) {
      return reply.status(403).send({ error: "跨租户调用被拒绝" });
    }

    const armed = armWritebackSaveIntent(session, saveIntentId);
    if (!armed) {
      return reply.status(400).send({ error: "写回保存意图无效或已过期" });
    }

    touchEditorSession(session);
    return { armed: true };
  });

  app.post("/api/bridge/confirm-preview", async (request) => {
    requireAuth(request);
    const body = request.body as {
      originalText?: string;
      modifiedText?: string;
      impactScope?: string;
      explanation?: string;
    };
    return {
      preview: {
        originalText: body.originalText ?? "",
        modifiedText: body.modifiedText ?? "",
        impactScope: body.impactScope ?? "selection",
        explanation: body.explanation ?? "",
      },
      actions: ["apply", "copy", "cancel"],
    };
  });
}
