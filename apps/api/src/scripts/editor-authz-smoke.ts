/**
 * c02 编辑器授权与写回溯源冒烟 — 多数用例可离线运行
 * parse-status 端点用例需 DB 可达；API 在线时真打 /api/preview/:id/parse-status
 */
import { config } from "../config.js";
import { buildDocumentKey, buildEditorConfig } from "../services/editor-config.js";
import {
  armWritebackSaveIntent,
  clearAllEditorSessions,
  createEditorSession,
  createSaveIntent,
  peekPendingWritebackSave,
  confirmPendingWritebackSave,
} from "../services/editor-sessions.js";
import { assertDsDownloadUrl } from "../services/callback-processor.js";
import { wrapOnlyofficeConfig } from "../services/onlyoffice-jwt.js";
import { pool } from "../db/pool.js";

let pass = 0;

function ok(cond: boolean, msg: string) {
  if (!cond) throw new Error("断言失败: " + msg);
  pass++;
  console.log("  ✓", msg);
}

function testCommentOnlyConfig() {
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
    filename: "note.docx",
    documentType: "word",
    permission: "comment",
    user: { userId: session.userId, displayName: "Comment User" },
  }) as {
    editorConfig?: { mode?: string; callbackUrl?: string };
    document?: { permissions?: { edit?: boolean; comment?: boolean } };
  };

  ok(cfg.editorConfig?.mode === "edit", "comment-only mode 为 edit");
  ok(cfg.document?.permissions?.edit === false, "comment-only permissions.edit=false");
  ok(cfg.document?.permissions?.comment === true, "comment-only permissions.comment=true");
  ok(!!cfg.editorConfig?.callbackUrl, "comment-only 有 callbackUrl");
}

function testWritebackPeekStatus() {
  clearAllEditorSessions();
  const session = createEditorSession({
    documentId: "00000000-0000-0000-0000-000000000001",
    documentKey: "key1",
    tenantId: "00000000-0000-0000-0000-000000000099",
    userId: "00000000-0000-0000-0000-000000000003",
    versionId: "00000000-0000-0000-0000-000000000002",
    revision: "rev1",
  });

  const intentId = createSaveIntent(session, "ai_writeback");
  ok(!peekPendingWritebackSave(session, 6), "未 arm 时 status=6 不消费");

  ok(armWritebackSaveIntent(session, intentId), "arm 写回意图成功");
  ok(!peekPendingWritebackSave(session, 2), "armed 后 status=2 仍为 user_edit");
  ok(
    peekPendingWritebackSave(session, 6) === "ai_writeback",
    "armed 后 status=6 可 peek ai_writeback",
  );

  confirmPendingWritebackSave(session);
  ok(!peekPendingWritebackSave(session, 6), "confirm 后不再 peek");
}

function testJwtConfigShape() {
  const wrapped = wrapOnlyofficeConfig({
    documentType: "word",
    document: { key: "k" },
    editorConfig: { mode: "edit" },
  }) as { documentType?: string; token?: string; document?: unknown };
  ok(!!wrapped.documentType, "JWT 包装保留 documentType");
  ok(!!wrapped.document, "JWT 包装保留 document");
  if (config.onlyoffice.jwtEnabled) {
    ok(!!wrapped.token, "JWT 启用时含 token");
  }
}

function testDsUrlAllowlist() {
  const dsOrigin = new URL(config.onlyoffice.dsUrl);
  const allowed = `${dsOrigin.protocol}//${dsOrigin.host}/cache/files/docx`;
  assertDsDownloadUrl(allowed);
  ok(true, "DS 同源下载 URL 通过");

  let rejected = false;
  try {
    assertDsDownloadUrl("http://evil.example.com/steal");
  } catch {
    rejected = true;
  }
  ok(rejected, "非 DS 主机 URL 被拒绝");
}


async function testParseStatusEndpointWhenTableMissing() {
  const client = await pool.connect();
  try {
    const reg = await client.query(
      `SELECT to_regclass('public.document_parse_jobs') AS t`,
    );
    if (reg.rows[0]?.t) {
      console.warn(
        "  ⚠ document_parse_jobs 已存在（c03 已落地），跳过表缺失 parse-status 端点断言",
      );
      return;
    }
  } finally {
    client.release();
  }

  const base = `http://localhost:${config.port}`;
  let cookie: string;
  try {
    const tenantRes = await pool.query(
      `SELECT t.name FROM tenants t
       JOIN users u ON u.tenant_id = t.tenant_id
       WHERE u.username = 'admin' LIMIT 1`,
    );
    if (!tenantRes.rows.length) {
      console.warn("  ⚠ 无 admin 种子用户，跳过 parse-status 端点检查");
      return;
    }
    const tenantName = tenantRes.rows[0].name as string;
    const loginRes = await fetch(`${base}/api/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        username: "admin",
        password: "admin123",
        tenant: tenantName,
      }),
    });
    if (!loginRes.ok) {
      throw new Error(`login ${loginRes.status}`);
    }
    const cookies = loginRes.headers.getSetCookie?.() ?? [];
    cookie = cookies.map((c) => c.split(";")[0]).join("; ");
  } catch {
    console.warn("  ⚠ API 未运行，跳过 parse-status 端点在线检查");
    return;
  }

  const docRes = await pool.query(
    `SELECT d.document_id FROM documents d
     JOIN users u ON u.tenant_id = d.tenant_id
     WHERE u.username = 'admin' AND d.is_deleted = FALSE
     LIMIT 1`,
  );

  let documentId: string;
  if (docRes.rows.length) {
    documentId = docRes.rows[0].document_id as string;
  } else {
    const form = new FormData();
    form.append(
      "file",
      new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], {
        type: "image/png",
      }),
      "smoke-parse.png",
    );
    form.append("space", "my");
    const up = await fetch(`${base}/api/documents/upload`, {
      method: "POST",
      headers: { Cookie: cookie },
      body: form,
    });
    if (!up.ok) {
      throw new Error(`upload failed: ${up.status}`);
    }
    const uploaded = (await up.json()) as { documentId: string };
    documentId = uploaded.documentId;
    ok(!!documentId, "无种子文档时上传 smoke-parse.png 供 parse-status 用例");
  }

  const res = await fetch(`${base}/api/preview/${documentId}/parse-status`, {
    headers: { Cookie: cookie },
  });
  ok(res.status === 200, "表缺失时 GET /parse-status 返回 200");
  const body = (await res.json()) as {
    status?: string;
    jobs?: unknown[];
    message?: string;
  };
  ok(body.status === "pending", "表缺失时 parse-status JSON status=pending");
  ok(Array.isArray(body.jobs) && body.jobs.length === 0, "表缺失时 parse-status jobs=[]");
  ok(!!body.message, "表缺失时 parse-status 含说明 message");
}

async function testCallbackJwtRequired() {
  const base = `http://localhost:${config.port}`;
  try {
    const res = await fetch(`${base}/api/editor/callback?token=fake`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status: 2, url: "http://evil.test/x" }),
    });
    if (config.onlyoffice.jwtEnabled) {
      ok(res.status === 403, "JWT 开启时无 body.token 回调返回 403");
    }
  } catch {
    console.warn("  ⚠ API 未运行，跳过回调 JWT 在线检查");
  }
}

async function main() {
  console.log("c02 editor-authz smoke...");
  testCommentOnlyConfig();
  testWritebackPeekStatus();
  testJwtConfigShape();
  testDsUrlAllowlist();
  await testParseStatusEndpointWhenTableMissing();
  await testCallbackJwtRequired();
  console.log(`editor-authz smoke passed (${pass} assertions)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
