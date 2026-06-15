import { test as setup, expect, type Page } from "@playwright/test";
import fs from "node:fs";

// 通过真实登录表单获取服务端会话 cookie，落盘为两份 storageState 供其余用例复用。
// admin（管理员，可见 /admin）、user（普通成员，无 highrisk:confirm / 管理后台）。
const AUTH_DIR = "e2e/.auth";

async function login(page: Page, username: string, password: string, file: string) {
  await page.goto("/");
  await page.locator('input[autocomplete="username"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.getByRole("button", { name: "登录" }).click();
  // 登录成功后进入门户外壳：侧栏出现 AIMed 学术助手 导航项
  await expect(page.getByRole("link", { name: "AIMed 学术助手" })).toBeVisible({
    timeout: 20_000,
  });
  await page.context().storageState({ path: file });
}

setup("authenticate as admin", async ({ page }) => {
  fs.mkdirSync(AUTH_DIR, { recursive: true });
  await login(page, "admin", "admin123", `${AUTH_DIR}/admin.json`);
});

setup("authenticate as user", async ({ page }) => {
  fs.mkdirSync(AUTH_DIR, { recursive: true });
  await login(page, "user", "user123", `${AUTH_DIR}/user.json`);
});
