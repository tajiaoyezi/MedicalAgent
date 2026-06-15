import { expect, type Page } from "@playwright/test";

/**
 * 附加监听，收集本页 console.error 与未捕获异常；测试末尾应断言为空。
 * 用法：const errors = collectClientErrors(page); ... expect(errors).toEqual([]);
 */
export function collectClientErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() === "error") errors.push(`console.error: ${m.text()}`);
  });
  return errors;
}

/** 打开门户内某客户端路由并等待外壳渲染（侧栏 AIMed 导航项可见即认为已登录就绪）。 */
export async function openRoute(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await expect(page.getByRole("link", { name: "AIMed 学术助手" })).toBeVisible();
}

/** 截图留证到 e2e/.artifacts/<name>.png（全页）。 */
export async function snapshot(page: Page, name: string): Promise<void> {
  await page.screenshot({ path: `e2e/.artifacts/${name}.png`, fullPage: true });
}
