import bcrypt from "bcryptjs";
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth, requirePermission } from "../middleware/auth.js";
import { revokedUserIds } from "../middleware/session-revoke.js";
import { writeAudit } from "../services/audit.js";

export async function registerAdminRoutes(app: FastifyInstance) {
  app.addHook("preHandler", async (request, reply) => {
    if (!request.url.startsWith("/api/admin")) return;
    const user = requireAuth(request);
    if (!user.permissions.includes("admin:console")) {
      return reply.status(403).send({ error: "无权限访问管理后台" });
    }
  });

  app.get("/api/admin/tenant", async (request) => {
    const user = requireAuth(request);
    const { rows } = await pool.query(
      `SELECT tenant_id, name, org_type, branding, enabled_modules, storage_quota_bytes
       FROM tenants WHERE tenant_id = $1`,
      [user.tenantId],
    );
    const userCount = await pool.query(
      "SELECT COUNT(*)::int AS count FROM users WHERE tenant_id = $1",
      [user.tenantId],
    );
    return {
      tenant: rows[0],
      userCount: userCount.rows[0].count,
      note: "POC 单租户演示，不提供新建或切换多租户",
    };
  });

  app.get("/api/admin/users", async (request) => {
    const user = requireAuth(request);
    const { rows } = await pool.query(
      `SELECT u.user_id, u.username, u.display_name, u.dept_id, u.is_enabled, u.created_at,
              ARRAY_AGG(r.slug) AS roles
       FROM users u
       LEFT JOIN user_roles ur ON ur.user_id = u.user_id
       LEFT JOIN roles r ON r.role_id = ur.role_id
       WHERE u.tenant_id = $1
       GROUP BY u.user_id`,
      [user.tenantId],
    );
    return { users: rows };
  });

  app.post("/api/admin/users", async (request, reply) => {
    const user = requireAuth(request);
    requirePermission(user, "user:manage");

    const body = request.body as {
      username?: string;
      password?: string;
      displayName?: string;
      deptId?: string;
      roleSlug?: string;
    };

    if (!body.username || !body.password || !body.displayName) {
      return reply.status(400).send({ error: "缺少必填字段" });
    }

    const client = await pool.connect();
    try {
      const hash = await bcrypt.hash(body.password, 10);
      const res = await client.query(
        `INSERT INTO users (tenant_id, username, password_hash, display_name, dept_id)
         VALUES ($1, $2, $3, $4, $5) RETURNING user_id`,
        [
          user.tenantId,
          body.username,
          hash,
          body.displayName,
          body.deptId ?? null,
        ],
      );
      const newUserId = res.rows[0].user_id;

      if (body.roleSlug) {
        const roleRes = await client.query(
          "SELECT role_id FROM roles WHERE tenant_id = $1 AND slug = $2",
          [user.tenantId, body.roleSlug],
        );
        if (roleRes.rows.length) {
          await client.query(
            "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)",
            [newUserId, roleRes.rows[0].role_id],
          );
        }
      }

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "user_create",
        targetType: "user",
        targetId: newUserId,
        result: "成功",
      });

      return { userId: newUserId };
    } finally {
      client.release();
    }
  });

  app.patch("/api/admin/users/:id", async (request, reply) => {
    const user = requireAuth(request);
    requirePermission(user, "user:manage");
    const { id } = request.params as { id: string };
    const body = request.body as {
      isEnabled?: boolean;
      deptId?: string;
      roleSlug?: string;
    };

    const client = await pool.connect();
    try {
      if (body.isEnabled !== undefined) {
        await client.query(
          "UPDATE users SET is_enabled = $1, updated_at = NOW() WHERE user_id = $2 AND tenant_id = $3",
          [body.isEnabled, id, user.tenantId],
        );
        if (!body.isEnabled) {
          revokedUserIds.add(id);
        }
      }
      if (body.deptId !== undefined) {
        await client.query(
          "UPDATE users SET dept_id = $1, updated_at = NOW() WHERE user_id = $2",
          [body.deptId, id],
        );
      }
      if (body.roleSlug) {
        const roleRes = await client.query(
          "SELECT role_id FROM roles WHERE tenant_id = $1 AND slug = $2",
          [user.tenantId, body.roleSlug],
        );
        if (roleRes.rows.length) {
          await client.query("DELETE FROM user_roles WHERE user_id = $1", [id]);
          await client.query(
            "INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)",
            [id, roleRes.rows[0].role_id],
          );
        }
      }

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "user_update",
        targetType: "user",
        targetId: id,
        result: "成功",
        metadata: body,
      });

      return { ok: true };
    } finally {
      client.release();
    }
  });

  app.get("/api/admin/audit-logs", async (request) => {
    const user = requireAuth(request);
    requirePermission(user, "audit:view");

    const { rows } = await pool.query(
      `SELECT audit_id, actor_id, actor_role, action_type, target_type, target_id,
              result, failure_reason, metadata, created_at
       FROM audit_logs WHERE tenant_id = $1
       ORDER BY created_at DESC LIMIT 200`,
      [user.tenantId],
    );
    return { logs: rows };
  });
}
