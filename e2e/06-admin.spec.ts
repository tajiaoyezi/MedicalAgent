import { test, expect } from "@playwright/test";
import { collectClientErrors, openRoute, snapshot } from "./helpers";

// 管理后台 /admin —— 仅管理员可见（chromium 默认 storageState=管理员登录态）。
// 四个 tab：用户与角色 / 审计日志 / 租户视图 / 模型 Provider。

test.describe("管理后台 · 页面外壳与四个 tab", () => {
  test("页面渲染、四个 tab 可切换且各自加载不报错", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    // ① 模块外壳标题
    await expect(page.getByRole("heading", { name: "管理后台" })).toBeVisible();

    const tabNames = ["用户与角色", "审计日志", "租户视图", "模型 Provider"];

    // 默认进入「用户与角色」tab，列表已加载
    await expect(page.getByRole("button", { name: "用户与角色" })).toBeVisible();
    await expect(page.locator("table.tbl")).toBeVisible();
    await snapshot(page, "admin-users");

    // 依次点击其余 tab，每个都应可见且无异常
    for (const name of tabNames) {
      await page.getByRole("button", { name }).click();
      await expect(page.getByRole("button", { name })).toBeVisible();
    }

    // ② 无客户端错误守卫
    expect(errors).toEqual([]);
  });
});

test.describe("管理后台 · 用户与角色", () => {
  test("用户列表含 admin/user 两行（显示名、角色、状态、操作）", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    // 默认即「用户与角色」tab
    const table = page.locator("table.tbl");
    await expect(table).toBeVisible();

    // 表头列
    await expect(table.getByRole("columnheader", { name: "用户名" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "显示名" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "角色" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "状态" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "操作" })).toBeVisible();

    // 演示管理员 admin 行（"admin" 同时出现在用户名列与角色 Tag，取首个即可，避免 strict 命中多个）
    const adminRow = table.locator("tr", { hasText: "演示管理员" });
    await expect(adminRow).toBeVisible();
    await expect(adminRow.getByText("admin", { exact: true }).first()).toBeVisible();

    // 演示用户 user 行
    const userRow = table.locator("tr", { hasText: "演示用户" });
    await expect(userRow).toBeVisible();
    await expect(userRow.getByText("user", { exact: true }).first()).toBeVisible();

    expect(errors).toEqual([]);
  });

  test("可对 user 行禁用再启用，状态随之切换并恢复原状", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    const table = page.locator("table.tbl");
    const userRow = table.locator("tr", { hasText: "演示用户" });
    await expect(userRow).toBeVisible();

    // 操作列按钮（btn-ghost），与状态 Tag 区分：按钮位于最后一列
    const actionBtn = userRow.locator("button.btn-ghost");
    await expect(actionBtn).toBeVisible();

    // 记录初始按钮文案：启用态显示「禁用」、禁用态显示「启用」
    const initialLabel = (await actionBtn.textContent())?.trim();
    expect(["启用", "禁用"]).toContain(initialLabel);

    // 点击切换 —— toggleUser 直接发 PATCH 后重拉列表（无确认弹窗）
    const patchPromise = page.waitForResponse(
      (r) =>
        /\/api\/admin\/users\/[^/]+$/.test(r.url()) && r.request().method() === "PATCH",
    );
    await actionBtn.click();
    const patchRes = await patchPromise;
    expect(patchRes.ok()).toBeTruthy();

    // 切换后按钮文案应反转
    const toggledLabel = initialLabel === "禁用" ? "启用" : "禁用";
    await expect(userRow.locator("button.btn-ghost")).toHaveText(toggledLabel);

    // 恢复原状：再点一次切回
    const restorePromise = page.waitForResponse(
      (r) =>
        /\/api\/admin\/users\/[^/]+$/.test(r.url()) && r.request().method() === "PATCH",
    );
    await userRow.locator("button.btn-ghost").click();
    const restoreRes = await restorePromise;
    expect(restoreRes.ok()).toBeTruthy();
    await expect(userRow.locator("button.btn-ghost")).toHaveText(initialLabel!);

    expect(errors).toEqual([]);
  });
});

test.describe("管理后台 · 审计日志", () => {
  test("切到审计日志 tab，渲染日志表或空态、不报错", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    await page.getByRole("button", { name: "审计日志" }).click();
    await expect(page.getByRole("button", { name: "审计日志" })).toHaveClass(/tab-active/);

    // 审计表表头
    const table = page.locator("table.tbl");
    await expect(table).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "时间" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "操作" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "角色" })).toBeVisible();
    await expect(table.getByRole("columnheader", { name: "结果" })).toBeVisible();

    // 已发生过登录等操作，表体通常有条目（不强制非空，仅断言表存在不报错）
    await snapshot(page, "admin-audit");

    expect(errors).toEqual([]);
  });
});

test.describe("管理后台 · 租户视图", () => {
  test("切到租户视图 tab，渲染租户信息卡片、不报错", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    await page.getByRole("button", { name: "租户视图" }).click();
    await expect(page.getByRole("button", { name: "租户视图" })).toHaveClass(/tab-active/);

    // 租户信息以卡片网格渲染，等待至少一张卡片（含 key/value）出现
    await expect(page.locator(".tbl")).toHaveCount(0);
    const card = page.locator('div[style*="break-all"]').first();
    await expect(card).toBeVisible();
    await snapshot(page, "admin-tenant");

    expect(errors).toEqual([]);
  });
});

test.describe("管理后台 · 模型 Provider", () => {
  test("切到模型 Provider tab，渲染规划中空态、不报错", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/admin");

    await page.getByRole("button", { name: "模型 Provider" }).click();
    await expect(page.getByRole("button", { name: "模型 Provider" })).toHaveClass(
      /tab-active/,
    );

    // 本期 Provider 管理由 c03 挂载，当前为「规划中」空态文案
    await expect(page.getByText("模型 Provider 配置 · 规划中")).toBeVisible();
    await snapshot(page, "admin-provider");

    expect(errors).toEqual([]);
  });
});

test.describe("管理后台 · 普通成员不可访问", () => {
  test.use({ storageState: "e2e/.auth/user.json" });

  test("普通成员访问 /admin 被重定向到 /aimed，不渲染管理后台", async ({ page }) => {
    await openRoute(page, "/admin");

    // 非管理员被 <Navigate to="/aimed"> 重定向，管理后台标题不应出现
    await expect(page).toHaveURL(/\/aimed$/);
    await expect(page.getByRole("heading", { name: "管理后台" })).toHaveCount(0);
  });
});
