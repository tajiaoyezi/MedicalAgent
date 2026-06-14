/**
 * c02 编辑器授权与写回溯源冒烟 — 多数用例可离线运行
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
  await testCallbackJwtRequired();
  console.log(`editor-authz smoke passed (${pass} assertions)`);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
