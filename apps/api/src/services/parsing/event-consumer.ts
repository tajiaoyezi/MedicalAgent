// c03｜document_events 纯消费方（design D7/spec）：消费 §10.6 全部 6 类触发源 → 异步创建解析作业。
// c03 在 document_events 上不产生任何 event_type；作业生命周期仅落 audit_logs。
import type { PoolClient } from "pg";
import { pool } from "../../db/pool.js";
import { writeAudit } from "../audit.js";
import { runParseJob, type ParseJob } from "./pipeline.js";

const CONSUMER = "c03-parse";

/** 取未消费的 document_events，逐条为对应 (document_id, document_version) 创建 pending 作业并标记已消费。 */
export async function dispatchPendingEvents(client: PoolClient): Promise<number> {
  const { rows } = await client.query(
    `SELECT e.event_id, e.event_type, e.document_id, e.tenant_id, dv.document_version
     FROM document_events e
     JOIN document_versions dv ON dv.version_id = e.version_id
     LEFT JOIN document_event_consumptions c
       ON c.event_id = e.event_id AND c.consumer = $1
     WHERE c.event_id IS NULL
     ORDER BY e.occurred_at ASC
     LIMIT 200`,
    [CONSUMER],
  );

  let created = 0;
  for (const ev of rows) {
    const jobRes = await client.query(
      `INSERT INTO document_parse_jobs (tenant_id, document_id, document_version, status, triggered_by)
       VALUES ($1, $2, $3, 'pending', $4)
       RETURNING job_id`,
      [ev.tenant_id, ev.document_id, ev.document_version, ev.event_type],
    );
    await client.query(
      `INSERT INTO document_event_consumptions (event_id, consumer) VALUES ($1, $2)
       ON CONFLICT DO NOTHING`,
      [ev.event_id, CONSUMER],
    );
    await writeAudit(client, {
      tenantId: ev.tenant_id,
      actionType: "parse_job",
      targetType: "document",
      targetId: ev.document_id,
      result: "成功",
      metadata: {
        jobId: jobRes.rows[0].job_id,
        documentVersion: ev.document_version,
        status: "pending",
        triggeredBy: ev.event_type,
      },
    });
    created++;
  }
  return created;
}

/** 取 pending 作业并依次执行。runParseJob 内部已捕获异常并落 failed，不会中断循环。 */
export async function runPendingJobs(client: PoolClient, limit = 20): Promise<number> {
  const { rows } = await client.query(
    `SELECT job_id, tenant_id, document_id, document_version
     FROM document_parse_jobs WHERE status = 'pending'
     ORDER BY created_at ASC LIMIT $1`,
    [limit],
  );
  for (const row of rows) {
    const job: ParseJob = {
      jobId: row.job_id,
      tenantId: row.tenant_id,
      documentId: row.document_id,
      documentVersion: row.document_version,
    };
    await runParseJob(client, job);
  }
  return rows.length;
}

/** 一轮 tick：消费事件 + 执行 pending 作业（供后台轮询与冒烟脚本调用）。 */
export async function parseTick(): Promise<{ dispatched: number; ran: number }> {
  const client = await pool.connect();
  try {
    const dispatched = await dispatchPendingEvents(client);
    const ran = await runPendingJobs(client);
    return { dispatched, ran };
  } finally {
    client.release();
  }
}

/** 失败作业重试（task 7.5）：置回 pending，下一轮 tick 重新执行；动作落审计。 */
export async function retryJob(
  client: PoolClient,
  tenantId: string,
  jobId: string,
  actorId: string,
  actorRole: string,
): Promise<boolean> {
  const res = await client.query(
    `UPDATE document_parse_jobs
     SET status = 'pending', substatus = NULL, failure_reason = NULL,
         triggered_by = 'manual_retry', actor_id = $3, started_at = NULL, completed_at = NULL,
         updated_at = NOW()
     WHERE job_id = $1 AND tenant_id = $2 AND status = 'failed'
     RETURNING document_id`,
    [jobId, tenantId, actorId],
  );
  if (!res.rowCount) return false;
  await writeAudit(client, {
    tenantId,
    actorId,
    actorRole,
    actionType: "parse_job_retry",
    targetType: "document",
    targetId: res.rows[0].document_id,
    result: "成功",
    metadata: { jobId },
  });
  return true;
}

let timer: NodeJS.Timeout | null = null;

/** 启动后台轮询（index.ts 调用）。间隔 <=0 关闭。 */
export function startParseWorker(intervalMs: number): void {
  if (timer || intervalMs <= 0) return;
  let running = false;
  timer = setInterval(async () => {
    if (running) return;
    running = true;
    try {
      await parseTick();
    } catch (e) {
      console.error("[parse-worker] tick 失败:", (e as Error).message);
    } finally {
      running = false;
    }
  }, intervalMs);
  if (typeof timer.unref === "function") timer.unref();
}
