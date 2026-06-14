import type { FastifyInstance } from "fastify";
import { pool } from "../db/pool.js";
import { requireAuth } from "../middleware/auth.js";

export async function registerHealthRoutes(app: FastifyInstance) {
  app.get("/health", async () => ({ status: "ok", service: "medoffice-api" }));

  app.get("/api/health", async () => {
    try {
      await pool.query("SELECT 1");
      return { status: "ok", database: "connected" };
    } catch {
      return { status: "degraded", database: "disconnected" };
    }
  });

  app.get("/api/me", async (request, reply) => {
    const user = requireAuth(request);
    return {
      userId: user.userId,
      tenantId: user.tenantId,
      username: user.username,
      displayName: user.displayName,
      roleSlugs: user.roleSlugs,
      permissions: user.permissions,
      isAdmin: user.permissions.includes("admin:console"),
    };
  });
}
