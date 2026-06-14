/**
 * c02 ONLYOFFICE 连通性自检 — 需 docker-compose up（含 onlyoffice）后运行
 */
import { config } from "../config.js";
import {
  buildDocumentKey,
  buildEditorConfig,
} from "../services/editor-config.js";
import {
  clearAllEditorSessions,
  createEditorSession,
  getSessionByOpenToken,
} from "../services/editor-sessions.js";
import { signOnlyofficePayload, verifyOnlyofficeToken } from "../services/onlyoffice-jwt.js";
import { resolveEditorRoute } from "../services/editor-types.js";
import { getCallbackMetrics } from "../services/editor-metrics.js";

async function checkDsHealth() {
  const url = `${config.onlyoffice.dsUrl}/healthcheck`;
  const res = await fetch(url).catch(() => null);
  if (!res?.ok) {
    console.warn("  ONLYOFFICE DS 未就绪（", url, "）— 跳过 DS 在线检查");
    return false;
  }
  console.log("  ONLYOFFICE DS healthcheck OK");
  return true;
}

async function checkJwtRoundtrip() {
  const payload = { test: true, document: { key: "smoke-key" } };
  const token = signOnlyofficePayload(payload);
  if (!token) {
    console.log("  JWT 未启用，跳过签名检查");
    return;
  }
  const verified = verifyOnlyofficeToken(token);
  if (!verified) throw new Error("JWT 验签失败");
  console.log("  JWT 签名/验签 OK");
}

async function checkEditorConfig() {
  clearAllEditorSessions();
  const session = createEditorSession({
    documentId: "00000000-0000-0000-0000-000000000001",
    documentKey: buildDocumentKey(
      "00000000-0000-0000-0000-000000000001",
      "00000000-0000-0000-0000-000000000002",
    ),
    tenantId: "00000000-0000-0000-0000-000000000099",
    userId: "00000000-0000-0000-0000-000000000003",
    versionId: "00000000-0000-0000-0000-000000000002",
    revision: "abc123",
  });

  const cfg = buildEditorConfig({
    session,
    filename: "smoke.docx",
    documentType: "word",
    permission: "edit",
    user: { userId: session.userId, displayName: "Smoke User" },
  }) as {
    documentType?: string;
    document?: { key?: string };
    token?: string;
  };

  if (!cfg.documentType) {
    throw new Error("编辑器配置缺少顶层 documentType");
  }
  if (!cfg.document?.key) {
    throw new Error("JWT 包装后顶层必须保留 document");
  }
  if (config.onlyoffice.jwtEnabled && !cfg.token) {
    throw new Error("JWT 启用时配置顶层必须包含 token");
  }

  const route = resolveEditorRoute("report.xlsx");
  if (route.documentType !== "cell") throw new Error("xlsx 路由错误");

  const bad = resolveEditorRoute("archive.zip");
  if (bad.route !== "unsupported") throw new Error("zip 应被拒绝");

  const found = getSessionByOpenToken(session.openToken);
  if (!found) throw new Error("open_token 会话丢失");

  console.log("  编辑器配置签发 OK");
  console.log("  文件类型路由 OK");
}

async function checkApiEditorMetrics() {
  const base = `http://localhost:${config.port}`;
  try {
    const res = await fetch(`${base}/api/health`);
    const health = await res.json();
    console.log("  API health:", health.status);
  } catch {
    console.warn("  API 未运行，跳过 /api/health 检查");
  }
  const metrics = getCallbackMetrics();
  console.log("  回调指标:", metrics);
}

async function smoke() {
  console.log("ONLYOFFICE bridge smoke...");
  await checkJwtRoundtrip();
  await checkEditorConfig();
  await checkApiEditorMetrics();
  await checkDsHealth();
  console.log("ONLYOFFICE smoke passed（离线降级：DS 未启动时仍可通过配置/JWT/路由自检）");
}

smoke().catch((e) => {
  console.error(e);
  process.exit(1);
});
