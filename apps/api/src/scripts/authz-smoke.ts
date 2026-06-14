/**
 * c01 授权加固冒烟：证明 fix/c01-authz 的 4 个跨租户/越权缺陷已被拒绝。
 * 前置：docker compose up -d && npm run migrate && 启动 API（npm run dev -w apps/api）
 * 运行：npm run smoke:authz --workspace=apps/api
 *
 * 临时种入第 2 个租户 B + 用户 bob，断言：
 *  #4 登录按租户隔离（多租户下无 tenant 拒绝、跨租户同名拒绝、指定租户放行）
 *  #1 管理员跨租户 PATCH 用户被拒（404）
 *  #2 文档授权写入拒绝外租户/伪造 principal（400）
 *  #3 禁用后重新启用的用户可再次登录（revoke 集合已清除）
 * 结束后清理租户 B。
 */
import bcrypt from "bcryptjs";
import { pool } from "../db/pool.js";
import { config } from "../config.js";

const BASE = `http://localhost:${config.port}`;
const TENANT_B_NAME = "B医院-authz冒烟";
let pass = 0;

/** 按 FK 安全顺序清除某租户的全部依赖行 */
async function purgeTenant(tenantId: string) {
  await pool.query("DELETE FROM audit_logs WHERE tenant_id = $1", [tenantId]);
  await pool.query("DELETE FROM recent_tasks WHERE tenant_id = $1", [tenantId]);
  await pool.query("DELETE FROM document_events WHERE tenant_id = $1", [tenantId]);
  await pool.query("DELETE FROM documents WHERE tenant_id = $1", [tenantId]);
  await pool.query(
    "DELETE FROM user_roles WHERE user_id IN (SELECT user_id FROM users WHERE tenant_id = $1)",
    [tenantId],
  );
  await pool.query("DELETE FROM users WHERE tenant_id = $1", [tenantId]);
  await pool.query(
    "DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM roles WHERE tenant_id = $1)",
    [tenantId],
  );
  await pool.query("DELETE FROM roles WHERE tenant_id = $1", [tenantId]);
  await pool.query("DELETE FROM tenants WHERE tenant_id = $1", [tenantId]);
}
function ok(cond: boolean, msg: string) {
  if (!cond) throw new Error("断言失败: " + msg);
  pass++;
  console.log("  ✓", msg);
}

async function login(username: string, password: string, tenant?: string) {
  const res = await fetch(`${BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password, tenant }),
  });
  const cookies = res.headers.getSetCookie?.() ?? [];
  const cookie = cookies.map((c) => c.split(";")[0]).join("; ");
  return { status: res.status, cookie };
}

async function main() {
  // 解析租户 A（admin 所在）名称与租户 A 普通用户
  const a = await pool.query(
    `SELECT t.tenant_id, t.name FROM tenants t
     JOIN users u ON u.tenant_id = t.tenant_id WHERE u.username = 'admin' LIMIT 1`,
  );
  const tenantA = a.rows[0].tenant_id as string;
  const tenantAName = a.rows[0].name as string;
  const normalUser = await pool.query(
    "SELECT user_id, username FROM users WHERE tenant_id = $1 AND username = 'user' LIMIT 1",
    [tenantA],
  );

  // 先清除可能残留的同名测试租户（使脚本可重复运行）
  const stale = await pool.query("SELECT tenant_id FROM tenants WHERE name = $1", [TENANT_B_NAME]);
  for (const row of stale.rows) await purgeTenant(row.tenant_id as string);

  // 种入租户 B + 角色 + 用户 bob
  const tb = await pool.query(
    "INSERT INTO tenants (name) VALUES ($1) RETURNING tenant_id",
    [TENANT_B_NAME],
  );
  const tenantB = tb.rows[0].tenant_id as string;
  const rb = await pool.query(
    "INSERT INTO roles (tenant_id, name, slug) VALUES ($1, '普通用户', 'user') RETURNING role_id",
    [tenantB],
  );
  const hash = await bcrypt.hash("bob123", 10);
  const ub = await pool.query(
    "INSERT INTO users (tenant_id, username, password_hash, display_name) VALUES ($1, 'bob', $2, 'Bob') RETURNING user_id",
    [tenantB, hash],
  );
  const bobId = ub.rows[0].user_id as string;
  await pool.query("INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)", [
    bobId,
    rb.rows[0].role_id,
  ]);

  let docId: string | undefined;
  try {
    console.log("#4 登录租户隔离");
    ok((await login("admin", "admin123")).status === 400, "多租户下未指定租户的登录被拒(400)");
    const adminA = await login("admin", "admin123", tenantAName);
    ok(adminA.status === 200, "指定租户 A 的 admin 登录成功(200)");
    ok((await login("bob", "bob123", tenantAName)).status === 401, "bob 在租户 A 下登录被拒(401，跨租户同名隔离)");
    ok((await login("bob", "bob123", TENANT_B_NAME)).status === 200, "bob 在租户 B 下登录成功(200)");

    console.log("#1 管理员跨租户 PATCH 用户");
    const patchCross = await fetch(`${BASE}/api/admin/users/${bobId}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json", Cookie: adminA.cookie },
      body: JSON.stringify({ isEnabled: false }),
    });
    ok(patchCross.status === 404, "admin(租户A) 改租户B 的 bob 被拒(404)");

    console.log("#2 文档授权写入 principal 校验");
    const form = new FormData();
    form.append("file", new Blob(["authz"], { type: "text/plain" }), "authz.txt");
    form.append("space", "my");
    const up = await fetch(`${BASE}/api/documents/upload`, {
      method: "POST",
      headers: { Cookie: adminA.cookie },
      body: form,
    });
    ok(up.status === 200 || up.status === 201, "admin 上传测试文档成功");
    docId = ((await up.json()) as { documentId: string }).documentId;
    const foreignUser = await fetch(`${BASE}/api/documents/${docId}/permissions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Cookie: adminA.cookie },
      body: JSON.stringify({ principalType: "user", principalId: bobId, permissionLevel: "view" }),
    });
    ok(foreignUser.status === 400, "给外租户用户 bob 授权被拒(400)");
    const bogusRole = await fetch(`${BASE}/api/documents/${docId}/permissions`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Cookie: adminA.cookie },
      body: JSON.stringify({ principalType: "role", principalId: "不存在的角色", permissionLevel: "view" }),
    });
    ok(bogusRole.status === 400, "给不存在的角色授权被拒(400)");

    if (normalUser.rows.length) {
      console.log("#3 禁用后重新启用可再登录");
      const uid = normalUser.rows[0].user_id as string;
      await fetch(`${BASE}/api/admin/users/${uid}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", Cookie: adminA.cookie },
        body: JSON.stringify({ isEnabled: false }),
      });
      ok((await login("user", "user123", tenantAName)).status === 403, "禁用后 user 登录被拒(403)");
      await fetch(`${BASE}/api/admin/users/${uid}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json", Cookie: adminA.cookie },
        body: JSON.stringify({ isEnabled: true }),
      });
      ok((await login("user", "user123", tenantAName)).status === 200, "重新启用后 user 登录成功(200，revoke 已清)");
    }

    console.log(`\nauthz 冒烟通过：${pass} 条断言全部成立`);
  } finally {
    // 清理：测试文档（租户 A）+ 整个租户 B
    if (docId) {
      await pool.query("DELETE FROM document_events WHERE document_id = $1", [docId]);
      await pool.query("DELETE FROM documents WHERE document_id = $1", [docId]);
    }
    await purgeTenant(tenantB);
  }
}

main()
  .then(() => process.exit(0))
  .catch((e) => {
    console.error(e);
    process.exit(1);
  });
