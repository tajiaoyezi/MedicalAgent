import { test, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { collectClientErrors, snapshot } from "./helpers";

// 编辑器（/editor/:id）与预览（/preview/:id）为门户外壳之外的独立路由。
// 需要一个真实文档 id：用 request 夹具按空间顺序拉 /api/documents 取首个文档。
// 外部依赖（ONLYOFFICE DS iframe）放宽：只断言宿主容器/头部/AI 面板/配置接口，
// 不依赖 DS 真实渲染或 JWT 链路；不触发高风险写回确认。

const SPACES = ["my", "team", "app"] as const;

/** 跨「我的/团队/应用」空间取第一个可用文档 id；取不到返回 null。 */
async function firstDocumentId(
  request: APIRequestContext,
): Promise<string | null> {
  for (const space of SPACES) {
    const res = await request.get(`/api/documents?space=${space}`);
    if (!res.ok()) continue;
    const body = (await res.json()) as {
      documents?: Array<{ document_id: string }>;
    };
    const id = body.documents?.[0]?.document_id;
    if (id) return id;
  }
  return null;
}

test.describe("编辑器与预览（管理员）", () => {
  test("文档中心点「打开」可进入编辑器或预览宿主路由", async ({
    page,
    request,
  }) => {
    const errors = collectClientErrors(page);
    const docId = await firstDocumentId(request);
    test.skip(!docId, "种子库无可用文档，跳过");

    await page.goto("/documents");
    // 文档中心壳渲染
    await expect(
      page.getByRole("heading", { name: "文档中心" }),
    ).toBeVisible();

    // 点击首行「打开」→ 触发 navigate(/editor/:id)，随后 open 接口可能改判为 preview
    const openBtn = page.getByRole("button", { name: /打开/ }).first();
    await expect(openBtn).toBeVisible();
    await openBtn.click();

    // 离开门户 documents 路由，进入独立宿主（editor 或 preview）
    await expect(page).toHaveURL(/\/(editor|preview)\/[^/]+$/, {
      timeout: 20_000,
    });

    // 两类宿主头部都含「← 文档中心」返回链接
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });

    await snapshot(page, "07-open-from-documents");
    expect(errors).toEqual([]);
  });

  test("编辑器页 /editor/:id 加载宿主：配置接口 200 + 头部 + AI 面板入口", async ({
    page,
    request,
  }) => {
    const errors = collectClientErrors(page);
    const docId = await firstDocumentId(request);
    test.skip(!docId, "种子库无可用文档，跳过");

    // 监听编辑器/预览配置接口（open 可能内部改判为 preview 路由）
    const configPromise = page.waitForResponse(
      (r) =>
        /\/api\/(editor\/open|preview)\/[^/]+/.test(r.url()) &&
        r.request().method() === "GET",
      { timeout: 20_000 },
    );

    await page.goto(`/editor/${docId}`);
    const configResp = await configPromise;
    // 宿主配置接口应成功返回（200）
    expect(configResp.status()).toBe(200);

    // 落点可能是编辑器或被改判到预览；两者均有「← 文档中心」头部链接
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });

    const url = page.url();
    if (/\/editor\//.test(url)) {
      // 编辑器宿主：头部「在线编辑 · 权限」文案 + 宿主容器 + AI 面板入口按钮
      await expect(
        page.locator("header").getByText("在线编辑", { exact: false }),
      ).toBeVisible();
      await expect(page.locator("#onlyoffice-editor")).toBeAttached();
      await expect(
        page.getByRole("button", { name: "医疗 AI 面板" }),
      ).toBeVisible();
    } else {
      // 被改判为预览：预览头部链接可见即可（不强依赖 DS 渲染）
      await expect(
        page.getByRole("link", { name: /文档中心/ }).first(),
      ).toBeVisible();
    }

    await snapshot(page, "07-editor-host");
    expect(errors).toEqual([]);
  });

  test("编辑器 AI 面板：润色/校对类入口存在（不触发写回确认）", async ({
    page,
    request,
  }) => {
    const errors = collectClientErrors(page);
    const docId = await firstDocumentId(request);
    test.skip(!docId, "种子库无可用文档，跳过");

    // 等宿主配置接口返回（editor/open 可能内部改判为 preview），避免在改判完成前过早读 URL
    const settle = page
      .waitForResponse(
        (r) =>
          /\/api\/(editor\/open|preview)\/[^/]+/.test(r.url()) &&
          r.request().method() === "GET",
        { timeout: 20_000 },
      )
      .catch(() => null);
    await page.goto(`/editor/${docId}`);
    await settle;

    // 仅在落点为编辑器时验证 AI 面板；被改判为预览则跳过该断言
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });
    // 给 editor→preview 改判留出一次导航刷新后再判定，规避竞态误判
    await page.waitForLoadState("networkidle").catch(() => {});
    test.skip(
      !/\/editor\//.test(page.url()),
      "首个种子文档为预览类（如图片），编辑器改判为预览、无 AI 面板；如需覆盖 AI 面板交互请预置可编辑文档",
    );

    // 打开右侧医疗 AI 面板
    const panelBtn = page.getByRole("button", { name: "医疗 AI 面板" });
    await expect(panelBtn).toBeVisible();
    await panelBtn.click();

    // 面板标题与只读入口存在（读取选区 / 润色预览占位），不点击应用避免高风险写回
    await expect(
      page.getByRole("heading", { name: "医疗 AI 面板" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "读取当前选区" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /润色预览/ }),
    ).toBeVisible();

    await snapshot(page, "07-editor-ai-panel");
    expect(errors).toEqual([]);
  });

  test("预览页 /preview/:id 加载预览宿主：配置接口 200 + 头部", async ({
    page,
    request,
  }) => {
    const errors = collectClientErrors(page);
    const docId = await firstDocumentId(request);
    test.skip(!docId, "种子库无可用文档，跳过");

    // 预览宿主拉取 /api/preview/:id 配置
    const previewPromise = page.waitForResponse(
      (r) =>
        /\/api\/preview\/[^/]+$/.test(r.url()) &&
        r.request().method() === "GET",
      { timeout: 20_000 },
    );

    await page.goto(`/preview/${docId}`);
    const previewResp = await previewPromise;
    expect(previewResp.status()).toBe(200);

    // 预览宿主头部含返回文档中心链接（loading 结束后渲染）
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });

    await snapshot(page, "07-preview-host");
    expect(errors).toEqual([]);
  });
});
