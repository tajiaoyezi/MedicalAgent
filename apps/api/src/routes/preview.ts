import { createHash } from "node:crypto";
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";
import {
  resolveEffectivePermission,
  type DocumentRow,
} from "../services/document-permissions.js";
import { resolveEditorRoute } from "../services/editor-types.js";
import {
  createObjectStorage,
  objectKeyForVersion,
} from "../services/object-storage.js";
import { config } from "../config.js";

const storage = createObjectStorage();

function minimalPdfBuffer(title: string): Buffer {
  const text = `OFD Preview: ${title}`;
  const pdf = `%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Contents 4 0 R>>endobj
4 0 obj<</Length ${text.length + 20}>>stream
BT /F1 12 Tf 72 720 Td (${text}) Tj ET
endstream endobj
xref
0 5
trailer<</Size 5/Root 1 0 R>>
startxref
0
%%EOF`;
  return Buffer.from(pdf, "utf8");
}

export async function registerPreviewRoutes(app: FastifyInstance) {
  app.get("/api/preview/:documentId", async (request, reply) => {
    const user = requireAuth(request);
    const { documentId } = request.params as { documentId: string };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        `SELECT d.*, dv.version_id, dv.object_key, dv.file_hash
         FROM documents d
         JOIN document_versions dv ON d.current_version_id = dv.version_id
         WHERE d.document_id = $1 AND d.is_deleted = FALSE`,
        [documentId],
      );
      if (!docRes.rows.length) {
        return reply.status(404).send({ error: "文档不存在" });
      }

      const row = docRes.rows[0];
      const doc = row as DocumentRow & {
        version_id: string;
        object_key: string;
        file_hash: string;
        mime_type?: string;
      };

      if (doc.tenant_id !== user.tenantId) {
        return reply.status(403).send({ error: "无权限" });
      }

      const level = await resolveEffectivePermission(client, user, doc);
      if (level === "none") {
        return reply.status(403).send({ error: "无权限" });
      }

      const routeInfo = resolveEditorRoute(doc.name);

      if (routeInfo.route === "preview-ofd") {
        const cacheKey = createHash("sha256")
          .update(`${doc.tenant_id}:${doc.file_hash}`)
          .digest("hex");
        const cacheRes = await client.query(
          `SELECT target_object_key FROM editor_conversion_cache WHERE source_hash = $1`,
          [cacheKey],
        );

        let previewKey: string;
        if (cacheRes.rows.length) {
          previewKey = cacheRes.rows[0].target_object_key as string;
        } else {
          const pdfBuf = minimalPdfBuffer(doc.name);
          previewKey = objectKeyForVersion(
            doc.tenant_id,
            documentId,
            `ofd-preview-${cacheKey.slice(0, 8)}`,
          );
          await storage.put(previewKey, pdfBuf, "application/pdf");
          await client.query(
            `INSERT INTO editor_conversion_cache (source_hash, target_object_key, target_mime)
             VALUES ($1, $2, 'application/pdf')
             ON CONFLICT (source_hash) DO NOTHING`,
            [cacheKey, previewKey],
          );
        }

        const url = await storage.presignedUrl(previewKey, 300);
        return {
          previewType: "ofd",
          label: "只读预览（OFD 转 PDF）",
          readOnly: true,
          url,
          dsUrl: config.onlyoffice.dsUrl,
          aiEntries: ["aimed", "translation"],
        };
      }

      if (routeInfo.route === "preview-pdf") {
        const url = await storage.presignedUrl(doc.object_key, 300);
        return {
          previewType: "pdf",
          readOnly: true,
          url,
          dsUrl: config.onlyoffice.dsUrl,
          aiEntries: ["aimed", "translation"],
          currentPage: 1,
        };
      }

      if (routeInfo.route === "preview-image") {
        const url = await storage.presignedUrl(doc.object_key, 300);
        return {
          previewType: "image",
          url,
          fileHash: doc.file_hash,
          visualParse: true,
        };
      }

      return reply.status(400).send({ error: "不支持的预览类型" });
    } finally {
      client.release();
    }
  });

  app.get("/api/preview/:documentId/parse-status", async (request, reply) => {
    const user = requireAuth(request);
    const { documentId } = request.params as { documentId: string };

    const client = await pool.connect();
    try {
      const docRes = await client.query(
        `SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2`,
        [documentId, user.tenantId],
      );
      if (!docRes.rows.length) {
        return reply.status(404).send({ error: "文档不存在" });
      }

      // document_parse_jobs 由 c03 owner 建表（migration 005）；建表前优雅降级避免 42P01
      const reg = await client.query(
        `SELECT to_regclass('public.document_parse_jobs') AS t`,
      );
      if (!reg.rows[0]?.t) {
        return {
          status: "pending",
          message: "等待 c03 解析服务建表并消费 upload_success 事件后创建作业",
          jobs: [],
        };
      }

      // 1.5a：用 document_version + c03 状态词（pending/parsing/succeeded/failed），
      // JOIN document_visual_parse_results 取结构化结果；去除 stub 列 result/job_type/version_id
      const jobRes = await client.query(
        `SELECT j.job_id, j.status, j.substatus, j.failure_reason, j.document_version,
                j.index_ready_at, j.updated_at,
                r.confidence AS visual_confidence, r.failure_reason AS visual_failure_reason
         FROM document_parse_jobs j
         LEFT JOIN document_visual_parse_results r
           ON r.document_id = j.document_id AND r.document_version = j.document_version
         WHERE j.document_id = $1 AND j.tenant_id = $2
         ORDER BY j.created_at DESC LIMIT 1`,
        [documentId, user.tenantId],
      );

      if (!jobRes.rows.length) {
        return {
          status: "pending",
          message: "等待 c03 解析服务消费 upload_success 事件后创建作业",
          jobs: [],
        };
      }

      const row = jobRes.rows[0];
      return {
        status: row.status,
        substatus: row.substatus,
        documentVersion: row.document_version,
        failureReason: row.failure_reason,
        indexReadyAt: row.index_ready_at,
        updatedAt: row.updated_at,
        visual:
          row.visual_confidence !== null || row.visual_failure_reason !== null
            ? { confidence: row.visual_confidence, failureReason: row.visual_failure_reason }
            : null,
        jobs: jobRes.rows,
      };
    } finally {
      client.release();
    }
  });
}
