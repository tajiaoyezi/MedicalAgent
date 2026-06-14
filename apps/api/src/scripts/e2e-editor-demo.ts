/**
 * c02 端到端演示脚本（API 层）— 登录 → 打开配置 → Bridge 授权 → 指标
 */
import { config } from "../config.js";

const API = `http://localhost:${config.port}`;

async function login(): Promise<string> {
  const res = await fetch(`${API}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username: "admin", password: "admin123" }),
  });
  if (!res.ok) throw new Error("登录失败");
  const cookie = res.headers.get("set-cookie");
  if (!cookie) throw new Error("无会话 cookie");
  return cookie.split(";")[0];
}

async function demo() {
  console.log("c02 E2E demo (API)...");
  const cookie = await login();
  const headers = { Cookie: cookie };

  const docsRes = await fetch(`${API}/api/documents?space=my`, { headers });
  const docs = (await docsRes.json()) as { documents: { document_id: string; name: string }[] };

  const docx = docs.documents.find((d) => d.name.endsWith(".docx"));
  if (!docx) {
    console.log("  无 docx 文档，跳过打开演示（请先上传样例 docx）");
    return;
  }

  const openRes = await fetch(`${API}/api/editor/open/${docx.document_id}`, { headers });
  const open = await openRes.json();
  console.log("  打开模式:", open.mode, "权限:", open.permission);

  if (open.bridgeToken) {
    const authRes = await fetch(`${API}/api/bridge/authorize`, {
      method: "POST",
      headers: { ...headers, "Content-Type": "application/json" },
      body: JSON.stringify({
        bridgeToken: open.bridgeToken,
        method: "getSelectedText",
      }),
    });
    const auth = await authRes.json();
    console.log("  Bridge 授权:", auth.permitted ? "通过" : "拒绝");
  }

  const metricsRes = await fetch(`${API}/api/editor/metrics`, { headers });
  console.log("  保存回调指标:", await metricsRes.json());

  console.log("E2E demo 完成");
}

demo().catch((e) => {
  console.error(e);
  process.exit(1);
});
