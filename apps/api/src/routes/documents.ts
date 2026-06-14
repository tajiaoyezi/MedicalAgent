import { v4 as uuidv4 } from "uuid";
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth, requirePermission } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";
import {
  canDownload,
  canEdit,
  canManagePermissions,
  canShare,
  resolveEffectivePermission,
  type DocumentRow,
} from "../services/document-permissions.js";
import {
  computeFileHash,
  createObjectStorage,
  objectKeyForVersion,
} from "../services/object-storage.js";
import { checkUploadGate } from "../services/upload-gate.js";

const storage = createObjectStorage();

async function emitUploadSuccess(
  client: import("pg").PoolClient,
  tenantId: string,
  documentId: string,
  versionId: string,
  payload: Record<string, unknown> = {},
) {
  await client.query(
    `INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
     VALUES ('upload_success', $1, $2, $3, $4::jsonb)`,
    [documentId, versionId, tenantId, JSON.stringify(payload)],
  );
}

export async function registerDocumentRoutes(app: FastifyInstance) {
  app.get("/api/documents", async (request) => {
    const user = requireAuth(request);
    const query = request.query as {
      space?: string;
      appSource?: string;
      recycle?: string;
    };

    const client = await pool.connect();
    try {
      let sql = `SELECT * FROM documents WHERE tenant_id = $1`;
      const params: unknown[] = [user.tenantId];

      if (query.recycle === "true") {
        sql += " AND is_deleted = TRUE";
      } else {
        sql += " AND is_deleted = FALSE";
        if (query.space) {
          params.push(query.space);
          sql += ` AND space = $${params.length}`;
        }
        if (query.appSource) {
          params.push(query.appSource);
          sql += ` AND app_source = $${params.length}`;
        }
      }

      sql += " ORDER BY updated_at DESC";
      const { rows } = await client.query(sql, params);

      const visible = [];
      for (const row of rows as DocumentRow[]) {
        const level = await resolveEffectivePermission(client, user, row);
        if (level !== "none") {
          visible.push({
            ...row,
            effectivePermission: level,
          });
        }
      }
      return { documents: visible };
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/upload", async (request, reply) => {
    const user = requireAuth(request);
    const data = await request.file();
    if (!data) {
      return reply.status(400).send({ error: "缺少文件" });
    }

    const buffer = await data.toBuffer();
    const space =
      (data.fields.space as { value?: string } | undefined)?.value ?? "my";
    const appSource = (data.fields.appSource as { value?: string } | undefined)
      ?.value;

    const gate = checkUploadGate(data.filename, buffer);
    const client = await pool.connect();
    try {
      if (!gate.allowed) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: "file_upload",
          result: "失败",
          failureReason: gate.failureReason ?? "上传被门禁阻止",
        });
        return reply.status(403).send({ error: gate.failureReason });
      }

      const documentId = uuidv4();
      const versionId = uuidv4();
      const fileHash = computeFileHash(buffer);
      const objectKey = objectKeyForVersion(
        user.tenantId,
        documentId,
        versionId,
      );

      await storage.put(objectKey, buffer, data.mimetype);

      await client.query("BEGIN");
      await client.query(
        `INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
         VALUES ($1, $2, $3, $4, $5, $6, $7)`,
        [
          documentId,
          user.tenantId,
          user.userId,
          data.filename,
          space,
          appSource ?? null,
          data.mimetype,
        ],
      );
      await client.query(
        `INSERT INTO document_versions (
          version_id, document_id, tenant_id, document_version, file_hash,
          saved_by, source, object_key, size_bytes
        ) VALUES ($1, $2, $3, 1, $4, $5, 'import', $6, $7)`,
        [
          versionId,
          documentId,
          user.tenantId,
          fileHash,
          user.userId,
          objectKey,
          buffer.length,
        ],
      );
      await client.query(
        `UPDATE documents SET current_version_id = $1, updated_at = NOW() WHERE document_id = $2`,
        [versionId, documentId],
      );
      await emitUploadSuccess(client, user.tenantId, documentId, versionId, {
        filename: data.filename,
        source: "upload",
      });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "file_upload",
        targetType: "document",
        targetId: documentId,
        result: "成功",
      });
      await client.query("COMMIT");

      return { documentId, versionId, fileHash };
    } catch (e) {
      await client.query("ROLLBACK");
      throw e;
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/create", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as {
      name?: string;
      space?: string;
      appSource?: string;
      source?: string;
      content?: string;
    };

    if (!body.name?.trim()) {
      return reply.status(400).send({ error: "缺少文档名称" });
    }

    const space = body.space ?? "my";
    const versionSource =
      body.source === "template" ||
      body.source === "ai_writeback" ||
      body.source === "import"
        ? body.source
        : "import";

    const content = Buffer.from(body.content ?? "", "utf8");
    const client = await pool.connect();
    try {
      const documentId = uuidv4();
      const versionId = uuidv4();
      const fileHash = computeFileHash(content);
      const objectKey = objectKeyForVersion(
        user.tenantId,
        documentId,
        versionId,
      );

      await storage.put(objectKey, content, "text/plain");

      await client.query("BEGIN");
      await client.query(
        `INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
         VALUES ($1, $2, $3, $4, $5, $6, 'text/plain')`,
        [
          documentId,
          user.tenantId,
          user.userId,
          body.name.trim(),
          space,
          body.appSource ?? null,
        ],
      );
      await client.query(
        `INSERT INTO document_versions (
          version_id, document_id, tenant_id, document_version, file_hash,
          saved_by, source, object_key, size_bytes
        ) VALUES ($1, $2, $3, 1, $4, $5, $6, $7, $8)`,
        [
          versionId,
          documentId,
          user.tenantId,
          fileHash,
          user.userId,
          versionSource,
          objectKey,
          content.length,
        ],
      );
      await client.query(
        `UPDATE documents SET current_version_id = $1, updated_at = NOW() WHERE document_id = $2`,
        [versionId, documentId],
      );
      await emitUploadSuccess(client, user.tenantId, documentId, versionId, {
        source: "server_create",
      });
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "document_create",
        targetType: "document",
        targetId: documentId,
        result: "成功",
      });
      await client.query("COMMIT");

      return { documentId, versionId, fileHash };
    } catch (e) {
      await client.query("ROLLBACK");
      throw e;
    } finally {
      client.release();
    }
  });

  app.get("/api/documents/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) {
        return reply.status(404).send({ error: "文档不存在" });
      }
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (level === "none") {
        return reply.status(403).send({ error: "无权限" });
      }

      const versions = await client.query(
        `SELECT version_id, document_version, file_hash, saved_by, saved_at, source, size_bytes
         FROM document_versions WHERE document_id = $1 ORDER BY saved_at DESC`,
        [id],
      );

      return { document: { ...doc, effectivePermission: level }, versions: versions.rows };
    } finally {
      client.release();
    }
  });

  app.patch("/api/documents/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as { name?: string; isFavorited?: boolean };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canEdit(level)) return reply.status(403).send({ error: "无权限" });

      if (body.name) {
        await client.query(
          "UPDATE documents SET name = $1, updated_at = NOW() WHERE document_id = $2",
          [body.name, id],
        );
      }
      if (body.isFavorited !== undefined) {
        await client.query(
          "UPDATE documents SET is_favorited = $1, updated_at = NOW() WHERE document_id = $2",
          [body.isFavorited, id],
        );
      }
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.delete("/api/documents/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canEdit(level) && level !== "manage") {
        return reply.status(403).send({ error: "无权限" });
      }

      await client.query(
        "UPDATE documents SET is_deleted = TRUE, updated_at = NOW() WHERE document_id = $1",
        [id],
      );
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "file_delete",
        targetType: "document",
        targetId: id,
        result: "成功",
      });
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/:id/restore", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canEdit(level)) return reply.status(403).send({ error: "无权限" });

      await client.query(
        "UPDATE documents SET is_deleted = FALSE, updated_at = NOW() WHERE document_id = $1",
        [id],
      );
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.get("/api/documents/:id/download", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canDownload(level)) {
        await writeAudit(client, {
          tenantId: user.tenantId,
          actorId: user.userId,
          actorRole: user.roleSlugs.join(","),
          actionType: "file_download",
          targetType: "document",
          targetId: id,
          result: "失败",
          failureReason: "权限不足",
        });
        return reply.status(403).send({ error: "无下载权限" });
      }

      const verRes = await client.query(
        "SELECT object_key FROM document_versions WHERE version_id = $1",
        [doc.current_version_id!],
      );
      const objectKey = verRes.rows[0].object_key as string;
      const url = await storage.presignedUrl(objectKey, 300);

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "file_download",
        targetType: "document",
        targetId: id,
        result: "成功",
      });

      return { url, expiresIn: 300 };
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/:id/permissions", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as {
      principalType: string;
      principalId: string;
      permissionLevel: string;
    };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canManagePermissions(level)) {
        return reply.status(403).send({ error: "无权限" });
      }

      // 校验 principal 合法且属于当前租户，拒绝伪造/任意 principal_id（防越权授权）
      const VALID_LEVELS = ["owner", "manage", "edit", "comment", "view", "none"];
      if (
        !["user", "role", "dept"].includes(body.principalType) ||
        !VALID_LEVELS.includes(body.permissionLevel) ||
        !body.principalId
      ) {
        return reply.status(400).send({ error: "principalType / permissionLevel / principalId 无效" });
      }
      let principalOk = false;
      if (body.principalType === "user") {
        // ::text 比较避免非法 UUID 触发 pg 类型错误
        const r = await client.query(
          "SELECT 1 FROM users WHERE user_id::text = $1 AND tenant_id = $2",
          [body.principalId, user.tenantId],
        );
        principalOk = r.rows.length > 0;
      } else if (body.principalType === "role") {
        const r = await client.query(
          "SELECT 1 FROM roles WHERE slug = $1 AND tenant_id = $2",
          [body.principalId, user.tenantId],
        );
        principalOk = r.rows.length > 0;
      } else {
        const r = await client.query(
          "SELECT 1 FROM users WHERE dept_id = $1 AND tenant_id = $2 LIMIT 1",
          [body.principalId, user.tenantId],
        );
        principalOk = r.rows.length > 0;
      }
      if (!principalOk) {
        return reply.status(400).send({ error: "principal 不存在或不属于当前租户" });
      }

      await client.query(
        `INSERT INTO document_permissions (tenant_id, document_id, principal_type, principal_id, permission_level)
         VALUES ($1, $2, $3, $4, $5)
         ON CONFLICT (document_id, principal_type, principal_id)
         DO UPDATE SET permission_level = EXCLUDED.permission_level`,
        [
          user.tenantId,
          id,
          body.principalType,
          body.principalId,
          body.permissionLevel,
        ],
      );

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "document_permission_change",
        targetType: "document",
        targetId: id,
        result: "成功",
        metadata: body,
      });

      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/:id/share", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canShare(level)) {
        return reply.status(403).send({ error: "无分享权限" });
      }
      return { ok: true, message: "分享占位（本期路由占位）" };
    } finally {
      client.release();
    }
  });

  app.post("/api/documents/:id/versions", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const data = await request.file();
    if (!data) return reply.status(400).send({ error: "缺少文件" });

    const buffer = await data.toBuffer();
    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (!canEdit(level)) return reply.status(403).send({ error: "无权限" });

      const countRes = await client.query(
        "SELECT COALESCE(MAX(document_version), 0) + 1 AS next FROM document_versions WHERE document_id = $1",
        [id],
      );
      const nextVersion = countRes.rows[0].next as number;
      const versionId = uuidv4();
      const fileHash = computeFileHash(buffer);
      const objectKey = objectKeyForVersion(user.tenantId, id, versionId);

      await storage.put(objectKey, buffer, data.mimetype);

      await client.query(
        `INSERT INTO document_versions (
          version_id, document_id, tenant_id, document_version, file_hash,
          saved_by, source, object_key, size_bytes
        ) VALUES ($1, $2, $3, $4, $5, $6, 'user_edit', $7, $8)`,
        [
          versionId,
          id,
          user.tenantId,
          nextVersion,
          fileHash,
          user.userId,
          objectKey,
          buffer.length,
        ],
      );
      await client.query(
        "UPDATE documents SET current_version_id = $1, updated_at = NOW() WHERE document_id = $2",
        [versionId, id],
      );

      return { versionId, documentVersion: nextVersion, fileHash };
    } finally {
      client.release();
    }
  });

  // 打开文档 → 跳转编辑器/预览（c02）
  app.get("/api/documents/:id/actions/open", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2 AND is_deleted = FALSE",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (level === "none") return reply.status(403).send({ error: "无权限" });

      return {
        redirect: `/editor/${id}`,
        documentId: id,
      };
    } finally {
      client.release();
    }
  });

  // 占位路由：AIMed / 翻译 / 模板 / 知识库
  app.get("/api/documents/:id/actions/:action", async (request, reply) => {
    const user = requireAuth(request);
    const { id, action } = request.params as { id: string; action: string };
    const allowed = ["aimed", "translate", "template", "knowledge"];
    if (!allowed.includes(action)) {
      return reply.status(400).send({ error: "未知操作" });
    }

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
        [id, user.tenantId],
      );
      if (!docRes.rows.length) return reply.status(404).send({ error: "文档不存在" });
      const doc = docRes.rows[0] as DocumentRow;
      const level = await resolveEffectivePermission(client, user, doc);
      if (level === "none") return reply.status(403).send({ error: "无权限" });

      return {
        placeholder: true,
        action,
        documentId: id,
        message: `操作 ${action} 为路由占位，由后续 phase 实现`,
      };
    } finally {
      client.release();
    }
  });
}
