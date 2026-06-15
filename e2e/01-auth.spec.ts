import { test, expect } from "@playwright/test";
import { collectClientErrors, snapshot } from "./helpers";

// 01 认证与 RBAC：登录页渲染、错误口令、正确登录跳转，以及成员/管理员的导航与路由守卫差异。

// ──────────────────────────────────────────────────────────────────────────
// (A) 未登录：清空 storageState
// ──────────────────────────────────────────────────────────────────────────
test.describe("未登录访问与登录流程", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("访问根路径显示登录页（标题与演示账号提示）", async ({ page }) => {
    const errors = collectClientErrors(page);
    await page.goto("/");

    // 关键渲染断言：登录标题 + 演示账号提示
    await expect(page.getByRole("heading", { name: "登录" })).toBeVisible();
    await expect(
      page.getByText("演示账号：admin / admin123，user / user123"),
    ).toBeVisible();

    await snapshot(page, "auth-login-page");
    // 无客户端错误守卫
    expect(errors).toEqual([]);
  });

  test("错误口令提交后显示错误文案并停留登录页", async ({ page }) => {
    await page.goto("/");
    await page.locator('input[autocomplete="username"]').fill("admin");
    await page.locator('input[type="password"]').fill("wrong");
    await page.getByRole("button", { name: "登录" }).click();

    // 出现错误提示（后端返回的鉴权失败文案，断言错误段落可见即可）
    await expect(page.locator("p.error")).toBeVisible();
    // 仍停留登录页：登录标题与表单仍在
    await expect(page.getByRole("heading", { name: "登录" })).toBeVisible();
    await expect(page.getByRole("button", { name: "登录" })).toBeVisible();
  });

  test("正确 admin/admin123 登录后进入门户并落到 /aimed", async ({ page }) => {
    const errors = collectClientErrors(page);
    await page.goto("/");
    await page.locator('input[autocomplete="username"]').fill("admin");
    await page.locator('input[type="password"]').fill("admin123");
    await page.getByRole("button", { name: "登录" }).click();

    // 登录后落到 /aimed，侧栏出现 AIMed 学术助手导航项
    await expect(page).toHaveURL(/\/aimed$/);
    await expect(
      page.getByRole("link", { name: "AIMed 学术助手" }),
    ).toBeVisible();

    await snapshot(page, "auth-admin-logged-in");
    expect(errors).toEqual([]);
  });
});

// ──────────────────────────────────────────────────────────────────────────
// (B) 普通成员：storageState=user
// ──────────────────────────────────────────────────────────────────────────
test.describe("普通成员视角的 RBAC", () => {
  test.use({ storageState: "e2e/.auth/user.json" });

  test("成员侧栏不出现「管理后台」导航项", async ({ page }) => {
    const errors = collectClientErrors(page);
    await page.goto("/aimed");

    // 已登录就绪：AIMed 学术助手可见
    await expect(
      page.getByRole("link", { name: "AIMed 学术助手" }),
    ).toBeVisible();
    // 成员无管理后台导航项
    await expect(
      page.getByRole("link", { name: "管理后台" }),
    ).toHaveCount(0);

    await snapshot(page, "auth-member-sidebar");
    expect(errors).toEqual([]);
  });

  test("成员直接访问 /admin 被重定向回 /aimed", async ({ page }) => {
    await page.goto("/admin");

    // 不落在 /admin，AIMed 占位文案可见
    await expect(page).not.toHaveURL(/\/admin/);
    await expect(
      page.getByText(
        "AIMed 学术助手占位页 — c04 aimed-rag-citation 将在此挂载六大模式。",
      ),
    ).toBeVisible();
  });
});

// ──────────────────────────────────────────────────────────────────────────
// (C) 管理员：默认 storageState（chromium 项目已配置为管理员登录态）
// ──────────────────────────────────────────────────────────────────────────
test.describe("管理员视角的 RBAC 与退出登录", () => {
  test("管理员侧栏出现「管理后台」导航项", async ({ page }) => {
    const errors = collectClientErrors(page);
    await page.goto("/aimed");

    await expect(
      page.getByRole("link", { name: "AIMed 学术助手" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: "管理后台" }),
    ).toBeVisible();

    await snapshot(page, "auth-admin-sidebar");
    expect(errors).toEqual([]);
  });

});

// ──────────────────────────────────────────────────────────────────────────
// (D) 退出登录：用独立「未登录」storageState 现场登录再登出。
//     登出会销毁服务端会话；若复用默认 admin storageState（被 02-09 共享的同一服务端
//     会话），登出会连带让后续所有 storageState 用例失去登录态。故此处用独立会话隔离。
// ──────────────────────────────────────────────────────────────────────────
test.describe("退出登录（独立会话，避免污染共享 admin 会话）", () => {
  test.use({ storageState: { cookies: [], origins: [] } });

  test("通过 TopBar 用户菜单退出登录后回到登录页", async ({ page }) => {
    // 现场登录（独立会话）
    await page.goto("/");
    await page.locator('input[autocomplete="username"]').fill("admin");
    await page.locator('input[type="password"]').fill("admin123");
    await page.getByRole("button", { name: "登录" }).click();
    await expect(
      page.getByRole("link", { name: "AIMed 学术助手" }),
    ).toBeVisible();

    // 打开 TopBar 右侧用户菜单（点击 header 内的头像触发区）
    await page.locator("header").locator("div[style*='cursor: pointer']").first().click();
    // 点击「退出登录」菜单项
    await page.getByText("退出登录", { exact: true }).click();

    // 回到登录页：登录表单可见
    await expect(page.getByRole("heading", { name: "登录" })).toBeVisible();
    await expect(page.getByRole("button", { name: "登录" })).toBeVisible();
  });
});
