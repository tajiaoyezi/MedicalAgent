import { test, expect } from "@playwright/test";
import { collectClientErrors, openRoute, snapshot } from "./helpers";

// 5 个占位/规划页：各自渲染正确占位文案，且无客户端错误。
// 占位页正文由 ModuleShell 渲染（<h1> 标题 + 卡片内说明段落）。
// 注意：侧栏导航项也包含「AIMed 学术助手」等文案，故正文断言一律走 .first()/精确文案区分。

test.describe("占位/规划页", () => {
  test("/aimed 渲染 AIMed 学术助手占位文案", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/aimed");

    // 页面标题（ModuleShell <h1>）
    await expect(
      page.getByRole("heading", { name: "AIMed 学术助手", level: 1 }),
    ).toBeVisible();
    // 占位说明正文
    await expect(
      page.getByText("AIMed 学术助手占位页 — c04 aimed-rag-citation 将在此挂载六大模式。"),
    ).toBeVisible();
    await expect(page.getByText("登录后默认进入本模块（PRD §6.3）。")).toBeVisible();

    await snapshot(page, "placeholder-aimed");
    expect(errors).toEqual([]);
  });

  test("/knowledge 渲染医疗知识库占位文案", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/knowledge");

    await expect(
      page.getByRole("heading", { name: "医疗知识库", level: 1 }),
    ).toBeVisible();
    await expect(
      page.getByText("医疗知识库占位页 — c06 knowledge-admin 将挂载 13 类知识库。"),
    ).toBeVisible();

    await snapshot(page, "placeholder-knowledge");
    expect(errors).toEqual([]);
  });

  test("/translation 渲染医学翻译占位文案", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/translation");

    await expect(
      page.getByRole("heading", { name: "医学翻译", level: 1 }),
    ).toBeVisible();
    await expect(
      page.getByText("医学翻译占位页 — c07 medical-translation 将挂载文件级异步翻译。"),
    ).toBeVisible();

    await snapshot(page, "placeholder-translation");
    expect(errors).toEqual([]);
  });

  test("/templates 渲染医疗模板库占位文案", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/templates");

    await expect(
      page.getByRole("heading", { name: "医疗模板库", level: 1 }),
    ).toBeVisible();
    await expect(
      page.getByText("医疗模板库占位页 — c08 template-center 将挂载 200 个模板。"),
    ).toBeVisible();

    await snapshot(page, "placeholder-templates");
    expect(errors).toEqual([]);
  });

  test("/digital-staff 渲染医疗数字员工「规划中」文案", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/digital-staff");

    await expect(
      page.getByRole("heading", { name: "医疗数字员工", level: 1 }),
    ).toBeVisible();
    // 卡片内 <h2>规划中</h2>
    await expect(page.getByRole("heading", { name: "规划中", level: 2 })).toBeVisible();
    await expect(
      page.getByText(
        "医疗数字员工能力规划于 V1.1 / V1.2，V1.0 仅保留导航入口，不提供创建、运行、编排或执行历史。",
      ),
    ).toBeVisible();

    await snapshot(page, "placeholder-digital-staff");
    expect(errors).toEqual([]);
  });
});
