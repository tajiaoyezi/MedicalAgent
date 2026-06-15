import { test, expect } from "@playwright/test";
import { collectClientErrors, openRoute, snapshot } from "./helpers";

// 最近任务页（来源过滤 chips + 空态）。chromium 项目已注入管理员登录态，直接 goto 即可。

// 与 RecentTasksPage.tsx 中 SOURCES 常量逐字对齐（顺序、文案均以源码为准）。
const SOURCES = [
  "AIMed 学术助手",
  "医疗知识库问答",
  "医疗数字员工",
  "医学翻译",
  "在线文档 AI 操作",
  "模板生成文档",
] as const;

test.describe("最近任务（管理员视角）", () => {
  test("页面外壳与来源过滤 chips 渲染", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/recent");

    // 模块外壳标题（ModuleShell title="最近任务"，breadcrumb 同含「最近任务」，故限定主内容区匹配）。
    await expect(
      page.getByRole("heading", { name: "最近任务" }).first()
    ).toBeVisible();

    // 六个来源过滤 chip 均为按钮，逐一断言可见。
    for (const s of SOURCES) {
      await expect(page.getByRole("button", { name: s }).first()).toBeVisible();
    }

    await snapshot(page, "recent-tasks");
    expect(errors).toEqual([]);
  });

  test("空态文案可见（无任务时）", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/recent");

    // 等待列表加载完成：要么出现空态标题，要么出现任务来源标签（避免在请求未回前误判）。
    const emptyTitle = page.getByText("暂无最近任务");
    const taskTag = page.locator(".tag, [class*='tag']").first();
    await expect(emptyTitle.or(taskTag).first()).toBeVisible();

    // 在确为空态时，断言空态标题与说明文案均按源码逐字呈现。
    if (await emptyTitle.isVisible()) {
      await expect(emptyTitle).toBeVisible();
      await expect(
        page.getByText("在 AIMed、知识库、翻译等模块产生的任务会出现在这里。")
      ).toBeVisible();
    }

    expect(errors).toEqual([]);
  });

  test("点击不同来源过滤 chip 后页面不报错", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/recent");

    // 依次点选 / 取消若干 chip：触发按来源过滤的 /api/recent-tasks 请求，断言页面无客户端错误。
    const aimed = page.getByRole("button", { name: "AIMed 学术助手" }).first();
    const translate = page.getByRole("button", { name: "医学翻译" }).first();

    await aimed.click();
    await page.waitForLoadState("networkidle");
    // 选中态切换为主色按钮（btn-primary），chip 仍在。
    await expect(aimed).toHaveClass(/btn-primary/);

    await translate.click();
    await page.waitForLoadState("networkidle");
    await expect(translate).toHaveClass(/btn-primary/);

    // 取消第一个 chip，回到未选中态。
    await aimed.click();
    await page.waitForLoadState("networkidle");
    await expect(aimed).toHaveClass(/btn-secondary/);

    // 模块外壳与 chips 在多次过滤后仍正常渲染（页面未崩溃）。
    await expect(
      page.getByRole("heading", { name: "最近任务" }).first()
    ).toBeVisible();
    await expect(translate).toBeVisible();

    expect(errors).toEqual([]);
  });
});
