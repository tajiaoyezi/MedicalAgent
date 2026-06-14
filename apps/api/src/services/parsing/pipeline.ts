// c03｜文本/扫描文档解析入库流水线（design D7）
// 状态机：pending → detecting →(visual_parsing?)→ chunking → embedding → indexing_handoff → succeeded|failed
// 内部子状态对外归并为 parsing；成功终态 succeeded。chunk/embedding 在最终事务内一次写入，
// 任何前置失败均不残留半成品 chunk（task 7.4）。作业生命周期仅写 audit_logs，绝不写 document_events。
import type { PoolClient } from "pg";
import { writeAudit } from "../audit.js";
import { createObjectStorage } from "../object-storage.js";
import { extensionOf } from "../editor-types.js";
import { invokeEmbed } from "../model/router.js";
import {
  chunkFromVisual,
  chunkPlainText,
  type TextSegment,
} from "./chunker.js";
import { emitIndexReady } from "./events.js";
import {
  runVisualParse,
  VisualParseFailedError,
  VisualProviderMissingError,
} from "./visual-parse.js";

const storage = createObjectStorage();

export interface ParseJob {
  jobId: string;
  tenantId: string;
  documentId: string;
  documentVersion: number;
}

const TEXT_EXT = new Set(["txt", "md", "markdown", "html", "htm", "csv", "json", "log"]);

type ParsePath = "direct_text" | "visual";

function detectParsePath(filename: string, mime: string | null): ParsePath {
  if (mime && mime.startsWith("text/")) return "direct_text";
  if (TEXT_EXT.has(extensionOf(filename))) return "direct_text";
  // 图片/PDF/OFD/Office 等统一走视觉解析（扫描/复杂/版式抽取）；可抽取文本的 Office 原生抽取库属后续集成
  return "visual";
}

async function setJob(
  client: PoolClient,
  jobId: string,
  fields: Record<string, unknown>,
): Promise<void> {
  const cols: string[] = [];
  const params: unknown[] = [];
  for (const [k, v] of Object.entries(fields)) {
    params.push(v);
    cols.push(`${k} = $${params.length}`);
  }
  params.push(jobId);
  await client.query(
    `UPDATE document_parse_jobs SET ${cols.join(", ")}, updated_at = NOW() WHERE job_id = $${params.length}`,
    params,
  );
}

async function failJob(
  client: PoolClient,
  job: ParseJob,
  reason: string,
): Promise<void> {
  await setJob(client, job.jobId, {
    status: "failed",
    substatus: null,
    failure_reason: reason,
    completed_at: new Date(),
  });
  await writeAudit(client, {
    tenantId: job.tenantId,
    actionType: "parse_job",
    targetType: "document",
    targetId: job.documentId,
    result: "失败",
    failureReason: reason,
    metadata: { jobId: job.jobId, documentVersion: job.documentVersion, status: "failed" },
  });
}

/** 运行单个解析作业。内部捕获所有异常并落 failed，不向外抛（供 worker 循环安全调用）。 */
export async function runParseJob(client: PoolClient, job: ParseJob): Promise<"succeeded" | "failed"> {
  try {
    await setJob(client, job.jobId, {
      status: "parsing",
      substatus: "detecting",
      started_at: new Date(),
    });

    // 定位该版本的对象与文档元数据
    const verRes = await client.query(
      `SELECT dv.object_key, d.name, d.mime_type
       FROM document_versions dv JOIN documents d ON d.document_id = dv.document_id
       WHERE dv.document_id = $1 AND dv.document_version = $2 AND dv.tenant_id = $3`,
      [job.documentId, job.documentVersion, job.tenantId],
    );
    if (!verRes.rows.length) {
      await failJob(client, job, "文档版本不存在");
      return "failed";
    }
    const { object_key: objectKey, name: filename, mime_type: mime } = verRes.rows[0] as {
      object_key: string;
      name: string;
      mime_type: string | null;
    };

    // detect → 选择路径
    const path = detectParsePath(filename, mime);
    let segments: TextSegment[];
    let sourceType: string;

    if (path === "direct_text") {
      const buf = await storage.get(objectKey);
      segments = chunkPlainText(buf.toString("utf8"));
      sourceType = "document";
    } else {
      await setJob(client, job.jobId, { substatus: "visual_parsing" });
      const visual = await runVisualParse(client, {
        tenantId: job.tenantId,
        documentId: job.documentId,
        documentVersion: job.documentVersion,
        objectKey,
        filename,
        mime: mime ?? "application/octet-stream",
        jobId: job.jobId,
      });
      segments = chunkFromVisual(visual);
      sourceType = "document";
    }

    await setJob(client, job.jobId, { substatus: "chunking" });
    if (!segments.length) {
      await failJob(client, job, "未抽取到可切分文本");
      return "failed";
    }

    // chunk_acl：默认继承来源文档级 ACL（不放宽）；c06 后续可写入更严范围
    const aclRes = await client.query(
      `SELECT principal_type, principal_id, permission_level
       FROM document_permissions WHERE document_id = $1`,
      [job.documentId],
    );
    const chunkAcl = JSON.stringify({
      inheritedFrom: "document",
      entries: aclRes.rows,
    });

    // embedding（经 Embed capability 走路由/降级）
    await setJob(client, job.jobId, { substatus: "embedding" });
    const embedRes = await invokeEmbed(
      client,
      { input: segments.map((s) => s.text) },
      { tenantId: job.tenantId },
    );
    if (embedRes.vectors.length !== segments.length) {
      await failJob(client, job, "embedding 数量与 chunk 数量不一致");
      return "failed";
    }

    // 写库（最终事务，一次写入；失败整体回滚不残留半成品）
    await client.query("BEGIN");
    try {
      // 重解析幂等：旧 chunk 标记 superseded 保留版本可溯源（task 7.11）
      await client.query(
        `UPDATE document_chunks SET superseded = TRUE
         WHERE document_id = $1 AND superseded = FALSE`,
        [job.documentId],
      );
      for (let i = 0; i < segments.length; i++) {
        const seg = segments[i];
        const chunkRes = await client.query(
          `INSERT INTO document_chunks
             (tenant_id, document_id, document_version, source_type, source_title,
              source_url, pubmed_id, doi, journal, year, section, page, paragraph_index,
              chunk_text, chunk_acl, superseded)
           VALUES ($1,$2,$3,$4,$5,NULL,NULL,NULL,NULL,NULL,$6,$7,$8,$9,$10::jsonb,FALSE)
           RETURNING id`,
          [
            job.tenantId,
            job.documentId,
            job.documentVersion,
            sourceType,
            filename,
            seg.section,
            seg.page,
            seg.paragraphIndex,
            seg.text,
            chunkAcl,
          ],
        );
        const chunkId = chunkRes.rows[0].id as string;
        const vec = embedRes.vectors[i];
        await client.query(
          `INSERT INTO embeddings (chunk_id, vector, model, dim)
           VALUES ($1, $2::jsonb, $3, $4)`,
          [chunkId, JSON.stringify(vec), embedRes.model, embedRes.dim],
        );
      }
      await setJob(client, job.jobId, {
        substatus: "indexing_handoff",
        status: "succeeded",
        index_ready_at: new Date(),
        completed_at: new Date(),
        failure_reason: null,
      });
      await client.query("COMMIT");
    } catch (e) {
      await client.query("ROLLBACK");
      throw e;
    }

    await writeAudit(client, {
      tenantId: job.tenantId,
      actionType: "parse_job",
      targetType: "document",
      targetId: job.documentId,
      result: "成功",
      metadata: {
        jobId: job.jobId,
        documentVersion: job.documentVersion,
        status: "succeeded",
        path,
        chunkCount: segments.length,
      },
    });

    // indexing_handoff：发出「索引就绪」事件（c04 构建检索索引、c06 知识库收尾），c03 不构建索引
    emitIndexReady({
      tenantId: job.tenantId,
      documentId: job.documentId,
      documentVersion: job.documentVersion,
      jobId: job.jobId,
      chunkCount: segments.length,
    });
    return "succeeded";
  } catch (e) {
    const reason =
      e instanceof VisualProviderMissingError || e instanceof VisualParseFailedError
        ? e.message
        : `解析异常：${(e as Error).message}`;
    try {
      await failJob(client, job, reason);
    } catch {
      // 失败落库本身异常时不再抛出，避免 worker 循环中断
    }
    return "failed";
  }
}
