import { test, expect } from "@playwright/test";
import type { APIRequestContext } from "@playwright/test";
import { collectClientErrors, snapshot } from "./helpers";

// 编辑器（/editor/:id）与预览（/preview/:id）是门户外壳之外的独立宿主路由。
// 文档类型决定落点：可编辑（docx 等）→ 编辑器；图片/PDF/OFD → 预览（EditorPage 自动改判）。
// 故按「类型」显式取文档，不依赖「第一个文档」（会被其它用例的上传打乱、导致非确定性）。
// 外部 ONLYOFFICE DS（:8080）放宽：只断宿主头部/配置接口/AI 面板（host 侧 UI），
// 不依赖 DS 真实渲染或 JWT 链路；不触发高风险写回确认（医疗红线）。

const SPACES = ["my", "team", "app"] as const;
const SEED_IMAGE = "smoke-parse.png"; // 种子图片文档 → 预览类宿主

/** 按确切名称跨空间查文档 id；取不到返回 null。 */
async function findDocByName(
  request: APIRequestContext,
  name: string,
): Promise<string | null> {
  for (const space of SPACES) {
    const res = await request.get(`/api/documents?space=${space}`);
    if (!res.ok()) continue;
    const body = (await res.json()) as {
      documents?: Array<{ document_id: string; name: string }>;
    };
    const hit = body.documents?.find((d) => d.name === name);
    if (hit) return hit.document_id;
  }
  return null;
}

/** 上传一个可编辑文档（.docx 按扩展名受理，/api/editor/open 返回编辑器配置而非改判预览），返回 id。 */
async function uploadEditableDoc(request: APIRequestContext): Promise<string> {
  const up = await request.post("/api/documents/upload", {
    multipart: {
      file: {
        name: "e2e-editor-probe.docx",
        mimeType:
          "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        buffer: Buffer.from("MedOffice E2E 编辑器探针\n", "utf-8"),
      },
      space: "my",
    },
  });
  expect(up.ok()).toBeTruthy();
  const id = (await up.json()).documentId as string;
  expect(id).toBeTruthy();
  return id;
}

test.describe("编辑器与预览（管理员）", () => {
  test("文档中心点「打开」既有文档进入独立宿主路由", async ({ page }) => {
    const errors = collectClientErrors(page);
    await page.goto("/documents");
    await expect(page.getByRole("heading", { name: "文档中心" })).toBeVisible();

    // 点种子图片行的「打开」→ navigate(/editor/:id)，图片随后被改判为 /preview/:id
    const seedRow = page
      .locator("table.tbl tbody tr")
      .filter({ hasText: SEED_IMAGE });
    await expect(seedRow).toHaveCount(1);
    await seedRow.getByRole("button", { name: "打开" }).click();

    await expect(page).toHaveURL(/\/(editor|preview)\/[^/]+$/, {
      timeout: 20_000,
    });
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });
    await snapshot(page, "07-open-from-documents");
    expect(errors).toEqual([]);
  });

  test("编辑器宿主 /editor/:id：配置接口 200 + 头部 + AI 面板入口", async ({
    page,
    request,
  }) => {
    const docId = await uploadEditableDoc(request);

    const configPromise = page.waitForResponse(
      (r) =>
        /\/api\/editor\/open\/[^/]+/.test(r.url()) &&
        r.request().method() === "GET",
      { timeout: 20_000 },
    );
    await page.goto(`/editor/${docId}`);
    const configResp = await configPromise;
    // docx → 编辑器模式，配置接口 200（不改判预览）
    expect(configResp.status()).toBe(200);

    await expect(page).toHaveURL(/\/editor\//);
    // 编辑器宿主头部「在线编辑 · 权限 …」+ AI 面板入口（均为 host 侧、独立于 DS iframe）
    await expect(
      page.locator("header").getByText("在线编辑", { exact: false }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "医疗 AI 面板" }),
    ).toBeVisible();

    await snapshot(page, "07-editor-host");
    // 宿主内嵌 ONLYOFFICE DS，合成 docx 会触发 DS 侧告警，故本用例不加 console error 守卫。
  });

  test("编辑器 AI 面板：润色/校对类只读入口存在（不触发写回确认）", async ({
    page,
    request,
  }) => {
    const docId = await uploadEditableDoc(request);

    await page.goto(`/editor/${docId}`);
    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });
    await expect(page).toHaveURL(/\/editor\//);

    // 打开右侧医疗 AI 面板（host 侧 React UI，独立于 DS 渲染）
    const panelBtn = page.getByRole("button", { name: "医疗 AI 面板" });
    await expect(panelBtn).toBeVisible();
    await panelBtn.click();

    // 面板标题与只读入口存在；不点击应用，避免触发高风险写回确认链路（医疗红线）。
    // 注意：面板标题是 <strong>（非 heading），且「医疗 AI 面板」文本同时是工具栏开关按钮；
    // 面板 aside 在 DOM 中后于工具栏按钮，故取 .last() 命中面板标题。
    await expect(page.getByText("医疗 AI 面板").last()).toBeVisible();
    await expect(
      page.getByRole("button", { name: "读取当前选区" }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /润色预览/ }),
    ).toBeVisible();
    await expect(page.getByRole("button", { name: "关闭" })).toBeVisible();

    await snapshot(page, "07-editor-ai-panel");
  });

  test("预览宿主 /preview/:id：配置接口 200 + 头部（图片类文档）", async ({
    page,
    request,
  }) => {
    const errors = collectClientErrors(page);
    const docId = await findDocByName(request, SEED_IMAGE);
    test.skip(!docId, "种子图片文档缺失，跳过");

    const previewPromise = page.waitForResponse(
      (r) =>
        /\/api\/preview\/[^/]+$/.test(r.url()) &&
        r.request().method() === "GET",
      { timeout: 20_000 },
    );
    await page.goto(`/preview/${docId}`);
    const previewResp = await previewPromise;
    expect(previewResp.status()).toBe(200);

    await expect(
      page.getByRole("link", { name: /文档中心/ }).first(),
    ).toBeVisible({ timeout: 20_000 });
    await snapshot(page, "07-preview-host");
    expect(errors).toEqual([]);
  });
});
