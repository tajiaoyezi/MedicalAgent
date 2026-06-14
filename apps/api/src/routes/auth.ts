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
    const body = request.body as {
      username?: string;
      password?: string;
      tenant?: string;
    };
    const username = body.username?.trim();
    const password = body.password ?? "";

    if (!username || !password) {
      return reply.status(400).send({ error: "请输入用户名与口令" });
    }

    const client = await pool.connect();
    try {
      // 解析登录所属租户：username 仅在 (tenant_id, username) 维度唯一，
      // 不限定租户会在多租户下命中错误租户的用户。
      let tenantId: string | undefined;
      if (body.tenant) {
        const tr = await client.query(
          "SELECT tenant_id FROM tenants WHERE name = $1 LIMIT 1",
          [body.tenant],
        );
        tenantId = tr.rows[0]?.tenant_id;
        if (!tenantId) {
          return reply.status(401).send({ error: "凭据无效" });
        }
      } else {
        const tr = await client.query("SELECT tenant_id FROM tenants");
        if (tr.rows.length === 1) {
          tenantId = tr.rows[0].tenant_id;
        } else if (tr.rows.length > 1) {
          return reply.status(400).send({ error: "存在多个租户，请指定租户" });
        }
      }

      const { rows } = await client.query(
        `SELECT user_id, tenant_id, password_hash, is_enabled FROM users
         WHERE username = $1 AND tenant_id = $2 LIMIT 1`,
        [username, tenantId ?? null],
      );

      if (!rows.length) {
        if (tenantId) {
          await writeAudit(client, {
            tenantId,
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
