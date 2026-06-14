import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth, requirePermission } from "../middleware/auth.js";
import { writeAudit } from "../services/audit.js";

export async function registerPortalRoutes(app: FastifyInstance) {
  app.get("/api/portal/branding", async (request) => {
    const user = requireAuth(request);
    const { rows } = await pool.query(
      `SELECT tenant_id, name, org_type, enabled_modules, branding, storage_quota_bytes
       FROM tenants WHERE tenant_id = $1`,
      [user.tenantId],
    );
    if (!rows.length) return { error: "租户不存在" };
    const tenant = rows[0];
    const userCount = await pool.query(
      `SELECT COUNT(*)::int AS count FROM users WHERE tenant_id = $1`,
      [user.tenantId],
    );
    return {
      tenantId: tenant.tenant_id,
      name: tenant.name,
      orgType: tenant.org_type,
      enabledModules: tenant.enabled_modules,
      branding: tenant.branding,
      userCount: userCount.rows[0].count,
      storageQuotaBytes: tenant.storage_quota_bytes,
      onlyofficeThemeNote:
        "ONLYOFFICE 编辑器原生 UI 不承诺跟随主题，仅外部宿主页面与面板入口适配主题。",
    };
  });

  app.put("/api/portal/branding", async (request, reply) => {
    const user = requireAuth(request);
    requirePermission(user, "admin:console");

    const body = request.body as { branding?: Record<string, unknown> };
    if (!body.branding) {
      return reply.status(400).send({ error: "缺少 branding" });
    }

    const client = await pool.connect();
    try {
      await client.query(
        `UPDATE tenants SET branding = $1::jsonb, updated_at = NOW() WHERE tenant_id = $2`,
        [JSON.stringify(body.branding), user.tenantId],
      );
      await writeAudit(client, {
        tenantId: user.tenantId,
        actorId: user.userId,
        actorRole: user.roleSlugs.join(","),
        actionType: "branding_update",
        targetType: "tenant",
        targetId: user.tenantId,
        result: "成功",
        metadata: { branding: body.branding },
      });
      return { ok: true };
    } finally {
      client.release();
    }
  });
}
