import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";
import {
  resolveEffectivePermission,
  type DocumentRow,
} from "../services/document-permissions.js";
import { buildDocumentKey, buildEditorConfig } from "../services/editor-config.js";
import {
  createEditorSession,
  getSessionByCallbackToken,
  getSessionByOpenToken,
  touchEditorSession,
} from "../services/editor-sessions.js";
import { resolveEditorRoute } from "../services/editor-types.js";
import { verifyOnlyofficeToken } from "../services/onlyoffice-jwt.js";
import {
  processSaveCallback,
  type CallbackBody,
} from "../services/callback-processor.js";
import { getCallbackMetrics } from "../services/editor-metrics.js";
import { createObjectStorage } from "../services/object-storage.js";
import { config } from "../config.js";

const storage = createObjectStorage();

export async function registerEditorRoutes(app: FastifyInstance) {
  app.get("/api/editor/open/:documentId", async (request, reply) => {
    const user = requireAuth(request);
    const { documentId } = request.params as { documentId: string };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        `SELECT d.*, dv.version_id AS cv_id, dv.file_hash AS cv_hash
         FROM documents d
         LEFT JOIN document_versions dv ON d.current_version_id = dv.version_id
         WHERE d.document_id = $1`,
        [documentId],
      );
      if (!docRes.rows.length) {
        return reply.status(404).send({ error: "文档不存在" });
      }

      const doc = docRes.rows[0] as DocumentRow & {
        cv_id?: string;
        cv_hash?: string;
        mime_type?: string;
        is_deleted: boolean;
      };

      if (doc.tenant_id !== user.tenantId) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: "open",
          targetType: "document",
          targetId: documentId,
          result: "失败",
          failureReason: "跨租户访问",
        });
        return reply.status(403).send({ error: "无权限" });
      }

      if (doc.is_deleted) {
        return reply.status(404).send({ error: "文档不存在" });
      }

      const level = await resolveEffectivePermission(client, user, doc);
      if (level === "none") {
        return reply.status(403).send({ error: "无权限" });
      }

      const routeInfo = resolveEditorRoute(doc.name);
      if (routeInfo.route === "unsupported") {
        return reply.status(400).send({ error: "不支持的文件类型" });
      }

      if (
        routeInfo.route === "preview-pdf" ||
        routeInfo.route === "preview-image" ||
        routeInfo.route === "preview-ofd"
      ) {
        return {
          mode: "preview",
          previewType: routeInfo.route.replace("preview-", ""),
          documentId,
          permission: level,
        };
      }

      if (!doc.cv_id || !doc.cv_hash) {
        return reply.status(400).send({ error: "文档无可用版本" });
      }

      const documentKey = buildDocumentKey(documentId, doc.cv_id);
      const session = createEditorSession({
        documentId,
        documentKey,
        tenantId: user.tenantId,
        userId: user.userId,
        versionId: doc.cv_id,
        revision: doc.cv_hash,
      });
      touchEditorSession(session);

      const editorConfig = buildEditorConfig({
        session,
        filename: doc.name,
        documentType: routeInfo.documentType!,
        permission: level,
        user: { userId: user.userId, displayName: user.displayName },
      });

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "open",
        targetType: "document",
        targetId: documentId,
        result: "成功",
      });

      return {
        mode: "editor",
        documentId,
        permission: level,
        dsUrl: config.onlyoffice.dsUrl,
        editorConfig,
        bridgeToken: session.bridgeToken,
        revision: session.revision,
      };
    } finally {
      client.release();
    }
  });

  app.get("/api/editor/download/:openToken", async (request, reply) => {
    const { openToken } = request.params as { openToken: string };
    const session = getSessionByOpenToken(openToken);
    if (!session) {
      return reply.status(403).send({ error: "下载链接无效或已过期" });
    }
    touchEditorSession(session);

    const client = await pool.connect();
    try {
      const verRes = await client.query(
        `SELECT object_key, dv.tenant_id
         FROM document_versions dv
         WHERE dv.version_id = $1 AND dv.document_id = $2`,
        [session.versionId, session.documentId],
      );
      if (!verRes.rows.length) {
        return reply.status(404).send({ error: "版本不存在" });
      }
      if (verRes.rows[0].tenant_id !== session.tenantId) {
        return reply.status(403).send({ error: "无权限" });
      }

      const objectKey = verRes.rows[0].object_key as string;
      const buffer = await storage.get(objectKey);
      const docRes = await client.query(
        `SELECT name, mime_type FROM documents WHERE document_id = $1`,
        [session.documentId],
      );
      const mime =
        (docRes.rows[0]?.mime_type as string) ?? "application/octet-stream";
      const name = (docRes.rows[0]?.name as string) ?? "document";

      reply.header("Content-Type", mime);
      reply.header(
        "Content-Disposition",
        `attachment; filename="${encodeURIComponent(name)}"`,
      );
      return reply.send(buffer);
    } finally {
      client.release();
    }
  });

  app.post("/api/editor/callback", async (request, reply) => {
    const query = request.query as { token?: string };
    const callbackToken = query.token;
    if (!callbackToken) {
      return reply.status(403).send({ error: 1 });
    }

    let body = request.body as CallbackBody;
    if (config.onlyoffice.jwtEnabled) {
      if (!body.token) {
        return reply.status(403).send({ error: 1 });
      }
      const verified = verifyOnlyofficeToken(body.token);
      if (!verified) {
        return reply.status(403).send({ error: 1 });
      }
      body = { ...body, ...verified } as CallbackBody;
    }

    const session = getSessionByCallbackToken(callbackToken);
    if (!session) {
      return reply.status(403).send({ error: 1 });
    }

    const client = await pool.connect();
    try {
      const result = await processSaveCallback(
        client,
        session,
        body,
        session.userId,
        "editor",
      );
      return reply.send(result);
    } finally {
      client.release();
    }
  });

  app.get("/api/editor/metrics", async (request) => {
    requireAuth(request);
    return getCallbackMetrics();
  });
}
