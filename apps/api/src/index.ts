import Fastify from "fastify";
import cors from "@fastify/cors";
import cookie from "@fastify/cookie";
import session from "@fastify/session";
import multipart from "@fastify/multipart";
import { config } from "./config.js";
import { registerHealthRoutes } from "./routes/health.js";
import { registerAuthRoutes } from "./routes/auth.js";
import { registerPortalRoutes } from "./routes/portal.js";
import { registerDocumentRoutes } from "./routes/documents.js";
import { registerAdminRoutes } from "./routes/admin.js";
import { registerRecentTasksRoutes } from "./routes/recent-tasks.js";
import { registerEditorRoutes } from "./routes/editor.js";
import { registerBridgeRoutes } from "./routes/bridge.js";
import { registerPreviewRoutes } from "./routes/preview.js";
import { revokedUserIds } from "./middleware/session-revoke.js";
import { getSessionUser } from "./middleware/auth.js";

const app = Fastify({ logger: true });

await app.register(cors, {
  origin: config.webOrigin,
  credentials: true,
});

await app.register(cookie);
await app.register(session, {
  secret: config.sessionSecret,
  cookie: {
    secure: config.nodeEnv === "production",
    httpOnly: true,
    maxAge: 86400000,
    sameSite: "lax",
  },
});

await app.register(multipart, { limits: { fileSize: 50 * 1024 * 1024 } });

app.addHook("preHandler", async (request, reply) => {
  const user = getSessionUser(request);
  if (user && revokedUserIds.has(user.userId)) {
    await request.session.destroy();
    if (
      request.url.startsWith("/api/") &&
      !request.url.startsWith("/api/auth/login") &&
      !request.url.startsWith("/api/health") &&
      request.url !== "/health"
    ) {
      return reply.status(401).send({ error: "会话已失效" });
    }
  }
});

await registerHealthRoutes(app);
await registerAuthRoutes(app);
await registerPortalRoutes(app);
await registerDocumentRoutes(app);
await registerAdminRoutes(app);
await registerRecentTasksRoutes(app);
await registerEditorRoutes(app);
await registerBridgeRoutes(app);
await registerPreviewRoutes(app);

app.setErrorHandler((error, _request, reply) => {
  const statusCode = (error as { statusCode?: number }).statusCode ?? 500;
  reply.status(statusCode).send({
    error: (error as Error).message ?? "服务器错误",
  });
});

try {
  await app.listen({ port: config.port, host: config.host });
  console.log(`API listening on http://${config.host}:${config.port}`);
} catch (err) {
  app.log.error(err);
  process.exit(1);
}
