import { test, expect } from "@playwright/test";
import { collectClientErrors, openRoute, snapshot } from "./helpers";

// 门户外壳（管理员默认态）：侧栏分组/导航、面包屑、合规徽标、规划中入口、主题切换、折叠展开。
test.describe("门户外壳（管理员）", () => {
  test("侧栏三组及各导航项文案齐全", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    const sidebar = page.locator("aside");

    // 三个分组标题
    await expect(sidebar.getByText("工作空间", { exact: true })).toBeVisible();
    await expect(sidebar.getByText("文档与任务", { exact: true })).toBeVisible();
    await expect(sidebar.getByText("管理", { exact: true })).toBeVisible();

    // 各导航项文案（可导航的用 link role；规划中的「医疗数字员工」单独断言）
    await expect(page.getByRole("link", { name: "AIMed 学术助手" })).toBeVisible();
    await expect(page.getByRole("link", { name: "医疗知识库" })).toBeVisible();
    await expect(page.getByRole("link", { name: "医学翻译" })).toBeVisible();
    await expect(page.getByRole("link", { name: "医疗模板库" })).toBeVisible();
    await expect(page.getByRole("link", { name: "文档中心" })).toBeVisible();
    await expect(page.getByRole("link", { name: "最近任务" })).toBeVisible();
    await expect(page.getByRole("link", { name: "管理后台" })).toBeVisible();
    await expect(sidebar.getByText("医疗数字员工", { exact: true })).toBeVisible();

    await snapshot(page, "02-portal-shell");
    expect(errors).toEqual([]);
  });

  test("导航点击后 URL 与顶部面包屑标题随之更新", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    const header = page.locator("header");

    // 文档中心 → /documents，面包屑「文档与任务 > 文档中心」
    await page.getByRole("link", { name: "文档中心" }).click();
    await expect(page).toHaveURL(/\/documents$/);
    await expect(header.getByText("文档与任务", { exact: true })).toBeVisible();
    await expect(header.getByText("文档中心", { exact: true })).toBeVisible();

    // 最近任务 → /recent，面包屑「文档与任务 > 最近任务」
    await page.getByRole("link", { name: "最近任务" }).click();
    await expect(page).toHaveURL(/\/recent$/);
    await expect(header.getByText("文档与任务", { exact: true })).toBeVisible();
    await expect(header.getByText("最近任务", { exact: true })).toBeVisible();

    // 医学翻译 → /translation，面包屑「工作空间 > 医学翻译」
    await page.getByRole("link", { name: "医学翻译" }).click();
    await expect(page).toHaveURL(/\/translation$/);
    await expect(header.getByText("工作空间", { exact: true })).toBeVisible();
    await expect(header.getByText("医学翻译", { exact: true })).toBeVisible();

    expect(errors).toEqual([]);
  });

  test("顶部合规徽标可见", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    const header = page.locator("header");
    await expect(header.getByText("脱敏门禁已启用", { exact: true })).toBeVisible();
    await expect(header.getByText("私有化模型", { exact: true })).toBeVisible();

    expect(errors).toEqual([]);
  });

  test("「医疗数字员工」带规划中标签且不可导航", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    const sidebar = page.locator("aside");
    // 规划中标签存在
    await expect(sidebar.getByText("规划中", { exact: true })).toBeVisible();

    // 它是 div，非 link：不应存在同名 link role
    await expect(page.getByRole("link", { name: "医疗数字员工" })).toHaveCount(0);

    // 点击后仍停留在原路由，不发生导航
    await page.getByText("医疗数字员工", { exact: true }).click();
    await expect(page).toHaveURL(/\/aimed$/);

    expect(errors).toEqual([]);
  });

  test("主题切换：点击底部主题色块后页面仍正常", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    // 侧栏底部 3 个主题色块（title 来自 THEME_LABELS）
    await page.locator("aside").getByTitle("人文绿").click();
    await page.locator("aside").getByTitle("科技深色").click();
    await page.locator("aside").getByTitle("临床蓝").click();

    // 切换后外壳仍渲染正常
    await expect(page.getByRole("link", { name: "AIMed 学术助手" })).toBeVisible();

    expect(errors).toEqual([]);
  });

  test("折叠/展开导航：折叠后侧栏导航文案隐藏", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    const sidebar = page.locator("aside");
    const navLabel = sidebar.getByText("AIMed 学术助手", { exact: true });
    await expect(navLabel).toBeVisible();

    // 点 TopBar 折叠按钮
    const toggle = page.locator("header").getByTitle("折叠 / 展开导航");
    await toggle.click();
    // 折叠后导航文案隐藏（!collapsed 才渲染 label span）
    await expect(navLabel).toHaveCount(0);

    // 再点一次展开，文案恢复
    await toggle.click();
    await expect(sidebar.getByText("AIMed 学术助手", { exact: true })).toBeVisible();

    expect(errors).toEqual([]);
  });
});
