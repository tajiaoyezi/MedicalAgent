import { v4 as uuidv4 } from "uuid";
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";
import {
  canEdit,
  resolveEffectivePermission,
  type DocumentRow,
} from "../services/document-permissions.js";

const SOURCE_VALUES = [
  "AIMed 学术助手",
  "医疗知识库问答",
  "医疗数字员工",
  "医学翻译",
  "在线文档 AI 操作",
  "模板生成文档",
] as const;

function groupByTime(updatedAt: Date): string {
  const now = new Date();
  const diff = now.getTime() - updatedAt.getTime();
  const day = 86400000;
  if (diff < day) return "today";
  if (diff < 7 * day) return "7d";
  if (diff < 30 * day) return "30d";
  if (diff < 365 * day) return "1y";
  return "all";
}

export async function registerRecentTasksRoutes(app: FastifyInstance) {
  app.get("/api/recent-tasks", async (request) => {
    const user = requireAuth(request);
    const query = request.query as { sources?: string };
    const sourceFilter = query.sources
      ? query.sources.split(",").filter(Boolean)
      : null;

    const { rows } = await pool.query(
      `SELECT task_id, source, title, ref_type, ref_id, updated_at, deleted_at
       FROM recent_tasks
       WHERE tenant_id = $1 AND user_id = $2 AND deleted_at IS NULL
       ORDER BY updated_at DESC`,
      [user.tenantId, user.userId],
    );

    const tasks = rows
      .filter((r) => {
        if (!sourceFilter?.length) return true;
        return sourceFilter.includes(r.source as string);
      })
      .map((r) => ({
        taskId: r.task_id,
        source: r.source,
        title: r.title,
        titlePreview:
          (r.title as string).length > 10
            ? (r.title as string).slice(0, 10)
            : r.title,
        refType: r.ref_type,
        refId: r.ref_id,
        updatedAt: r.updated_at,
        timeGroup: groupByTime(new Date(r.updated_at as string)),
      }));

    return { tasks };
  });

  app.post("/api/recent-tasks", async (request, reply) => {
    const user = requireAuth(request);
    const body = request.body as {
      source?: string;
      title?: string;
      refType?: string;
      refId?: string;
    };

    if (!body.source || !body.title) {
      return reply.status(400).send({ error: "缺少 source 或 title" });
    }
    if (!SOURCE_VALUES.includes(body.source as typeof SOURCE_VALUES[number])) {
      return reply.status(400).send({ error: "无效的 source" });
    }

    const taskId = uuidv4();
    await pool.query(
      `INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, ref_type, ref_id, updated_at)
       VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
       ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
       DO UPDATE SET title = EXCLUDED.title, updated_at = NOW(), deleted_at = NULL`,
      [
        taskId,
        user.tenantId,
        user.userId,
        body.source,
        body.title,
        body.refType ?? null,
        body.refId ?? null,
      ],
    );

    return { taskId };
  });

  app.patch("/api/recent-tasks/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as { title?: string };

    if (!body.title?.trim()) {
      return reply.status(400).send({ error: "缺少标题" });
    }

    await pool.query(
      `UPDATE recent_tasks SET title = $1, updated_at = NOW()
       WHERE task_id = $2 AND tenant_id = $3 AND user_id = $4`,
      [body.title.trim(), id, user.tenantId, user.userId],
    );
    return { ok: true };
  });

  app.delete("/api/recent-tasks/:id", async (request, reply) => {
    const user = requireAuth(request);
    const { id } = request.params as { id: string };
    const body = request.body as { deleteLinkedDocument?: boolean };

    const client = await pool.connect();
    try {
      const taskRes = await client.query(
        `SELECT * FROM recent_tasks WHERE task_id = $1 AND tenant_id = $2 AND user_id = $3`,
        [id, user.tenantId, user.userId],
      );
      if (!taskRes.rows.length) {
        return reply.status(404).send({ error: "任务不存在" });
      }
      const task = taskRes.rows[0];

      if (
        body.deleteLinkedDocument &&
        task.ref_type === "document" &&
        task.ref_id
      ) {
        const docRes = await client.query(
          "SELECT * FROM documents WHERE document_id = $1 AND tenant_id = $2",
          [task.ref_id, user.tenantId],
        );
        if (docRes.rows.length) {
          const doc = docRes.rows[0] as DocumentRow;
          const level = await resolveEffectivePermission(client, user, doc);
          if (!canEdit(level) && level !== "manage") {
            return reply.status(403).send({ error: "无删除关联文档权限" });
          }
          await client.query(
            "UPDATE documents SET is_deleted = TRUE WHERE document_id = $1",
            [task.ref_id],
          );
          await writeAudit(client, {
            tenantId: user.tenantId,
            actorId: user.userId,
            actorRole: user.roleSlugs.join(","),
            actionType: "file_delete",
            targetType: "document",
            targetId: task.ref_id as string,
            result: "成功",
            metadata: { fromRecentTask: id },
          });
        }
      }

      await client.query(
        "UPDATE recent_tasks SET deleted_at = NOW() WHERE task_id = $1",
        [id],
      );
      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.post("/api/recent-tasks/batch-delete", async (request) => {
    const user = requireAuth(request);
    const body = request.body as {
      taskIds?: string[];
      deleteLinkedDocument?: boolean;
    };
    const ids = body.taskIds ?? [];

    for (const id of ids) {
      await pool.query(
        "UPDATE recent_tasks SET deleted_at = NOW() WHERE task_id = $1 AND user_id = $2",
        [id, user.userId],
      );
    }
    return { ok: true, deleted: ids.length };
  });
}
