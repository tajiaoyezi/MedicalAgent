/**
 * c01 主线冒烟：登录 → 上传 → 权限 → 下载 → 删除
 * 运行：npm run migrate && npm run smoke:integration --workspace=apps/api
 */
import { config } from "../config.js";

const BASE = `http://localhost:${config.port}`;

async function main() {
  const loginRes = await fetch(`${BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: "admin", password: "admin123" }),
  });
  if (!loginRes.ok) throw new Error(`login failed: ${loginRes.status}`);
  const cookies = loginRes.headers.getSetCookie?.() ?? [];
  const cookieHeader = cookies.map((c) => c.split(";")[0]).join("; ");

  const me = await fetch(`${BASE}/api/me`, {
    headers: { Cookie: cookieHeader },
  });
  if (!me.ok) throw new Error("session failed");

  const form = new FormData();
  form.append("file", new Blob(["integration test"], { type: "text/plain" }), "smoke.txt");
  form.append("space", "my");

  const upload = await fetch(`${BASE}/api/documents/upload`, {
    method: "POST",
    headers: { Cookie: cookieHeader },
    body: form,
  });
  if (!upload.ok) throw new Error(`upload failed: ${upload.status}`);
  const { documentId, fileHash } = (await upload.json()) as {
    documentId: string;
    fileHash: string;
  };
  console.log("upload OK", documentId, fileHash);

  const dl = await fetch(`${BASE}/api/documents/${documentId}/download`, {
    headers: { Cookie: cookieHeader },
  });
  if (!dl.ok) throw new Error("download failed");
  console.log("download OK");

  const del = await fetch(`${BASE}/api/documents/${documentId}`, {
    method: "DELETE",
    headers: { Cookie: cookieHeader },
  });
  if (!del.ok) throw new Error("delete failed");
  console.log("delete OK");

  const audit = await fetch(`${BASE}/api/admin/audit-logs`, {
    headers: { Cookie: cookieHeader },
  });
  if (!audit.ok) throw new Error("audit failed");
  const logs = (await audit.json()) as { logs: unknown[] };
  console.log("audit entries:", logs.logs.length);

  console.log("Integration smoke passed");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
