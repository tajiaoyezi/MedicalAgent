import bcrypt from "bcryptjs";
import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { writeAudit } from "../services/audit.js";
import {
  getSessionUser,
  loadUserById,
  type SessionData,
} from "../middleware/auth.js";

export async function registerAuthRoutes(app: FastifyInstance) {
  app.post("/api/auth/login", async (request, reply) => {
    const body = request.body as { username?: string; password?: string };
    const username = body.username?.trim();
    const password = body.password ?? "";

    if (!username || !password) {
      return reply.status(400).send({ error: "请输入用户名与口令" });
    }

    const client = await pool.connect();
    try {
      const { rows } = await client.query(
        `SELECT user_id, tenant_id, password_hash, is_enabled FROM users
         WHERE username = $1 LIMIT 1`,
        [username],
      );

      if (!rows.length) {
        const tenantRow = await client.query(
          "SELECT tenant_id FROM tenants LIMIT 1",
        );
        const tid = tenantRow.rows[0]?.tenant_id;
        if (tid) {
          await writeAudit(client, {
            tenantId: tid,
            actionType: "login",
            result: "失败",
            failureReason: "用户不存在",
            metadata: { username },
          });
        }
        return reply.status(401).send({ error: "凭据无效" });
      }

      const row = rows[0];
      if (!row.is_enabled) {
        await writeAudit(client, {
          tenantId: row.tenant_id,
          actorId: row.user_id,
          actionType: "login",
          result: "失败",
          failureReason: "账号已禁用",
        });
        return reply.status(403).send({ error: "账号不可用" });
      }

      const ok = await bcrypt.compare(password, row.password_hash);
      if (!ok) {
        await writeAudit(client, {
          tenantId: row.tenant_id,
          actorId: row.user_id,
          actionType: "login",
          result: "失败",
          failureReason: "口令错误",
        });
        return reply.status(401).send({ error: "凭据无效" });
      }

      const user = await loadUserById(client, row.user_id);
      if (!user) {
        return reply.status(401).send({ error: "凭据无效" });
      }

      const session = request.session as SessionData;
      session.user = user;

      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "login",
        result: "成功",
      });

      return {
        user: {
          userId: user.userId,
          username: user.username,
          displayName: user.displayName,
          roleSlugs: user.roleSlugs,
          isAdmin: user.permissions.includes("admin:console"),
        },
        redirectTo: "/aimed",
      };
    } finally {
      client.release();
    }
  });

  app.post("/api/auth/logout", async (request) => {
    await request.session.destroy();
    return { ok: true };
  });

  app.get("/api/auth/session", async (request) => {
    const user = getSessionUser(request);
    if (!user) return { authenticated: false };
    return {
      authenticated: true,
      user: {
        userId: user.userId,
        username: user.username,
        displayName: user.displayName,
        roleSlugs: user.roleSlugs,
        isAdmin: user.permissions.includes("admin:console"),
      },
    };
  });
}
