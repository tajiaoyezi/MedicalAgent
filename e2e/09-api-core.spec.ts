import { test, expect } from "@playwright/test";

// 核心 API 契约：经 :5173 的 vite 代理打到 Go 后端 :3001。
// 用 request 夹具直接对真实后端断言响应字段（不 mock）。
// chromium 项目已注入管理员 storageState，故 request 夹具默认携带 admin cookie。

test.describe("核心 API 契约（经 :5173 代理）", () => {
  test("GET /api/health 返回 status=ok 且 database=connected", async ({
    request,
  }) => {
    const res = await request.get("/api/health");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBe("ok");
    expect(body.database).toBe("connected");
  });

  test("GET /api/auth/session（携管理员 cookie）authenticated=true 且为 admin", async ({
    request,
  }) => {
    const res = await request.get("/api/auth/session");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.authenticated).toBe(true);
    expect(body.user.username).toBe("admin");
    expect(body.user.isAdmin).toBe(true);
  });

  test("GET /api/me（携管理员 cookie）返回 username=admin、isAdmin=true", async ({
    request,
  }) => {
    const res = await request.get("/api/me");
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.username).toBe("admin");
    expect(body.isAdmin).toBe(true);
  });

  test("未登录 GET /api/auth/session → authenticated=false；受保护 /api/me → 401", async ({
    playwright,
    baseURL,
  }) => {
    // 独立 request context + 空 storageState，避免污染全局 admin 登录态。
    const anon = await playwright.request.newContext({
      baseURL,
      storageState: { cookies: [], origins: [] },
    });
    try {
      const sessionRes = await anon.get("/api/auth/session");
      expect(sessionRes.status()).toBe(200);
      const session = await sessionRes.json();
      expect(session.authenticated).toBe(false);

      // 受保护端点在未登录时应被 RequireAuth 拦下（401）。
      const meRes = await anon.get("/api/me");
      expect(meRes.status()).toBe(401);
    } finally {
      await anon.dispose();
    }
  });
});
