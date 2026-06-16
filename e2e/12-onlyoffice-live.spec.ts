import { test, expect, type APIRequestContext } from "@playwright/test";
import { readFileSync } from "node:fs";

// c05 task 15.1 真实接入测试（真实 ONLYOFFICE DS），**DS 依赖、默认跳过**（不在 DS-less 的常规套件中）。
// 开启：① docker compose up -d onlyoffice（DS :8080）；② dev:web --host 0.0.0.0 且 .env 设
// ONLYOFFICE_PLUGIN_URL=http://host.docker.internal:5173/onlyoffice-plugin/；③ DS_LIVE=1 npx playwright test 12-onlyoffice-live。
//
// 本测试验证「面板经 c02 真实 ONLYOFFICE 完成挂载 + 真实 docx 渲染 + c02 编辑器/下载/回调链路 + c05 文档打开默认展示面板」。
//
// 已知边界（不在本测试断言、留作 c02 后续）：MedOffice 桥插件 `window.parent.postMessage` 无法穿透
// ONLYOFFICE 嵌套的插件 iframe 到达宿主（host↔plugin 往返失效），故「选区→replaceSelection→saveDocument→
// 保存回调落版本」的程序化端到端无法在真实 DS 上经桥驱动。c02 保存回调→`document_versions` 机制本身由
// smoke:onlyoffice / smoke:editor-authz 以模拟回调覆盖；本测试已实证真实 DS 会向 `/api/editor/callback` 真实回调。

const DOCX_MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";

async function uploadFixtureDocx(request: APIRequestContext): Promise<string> {
  const buffer = readFileSync("e2e/fixtures/minimal.docx");
  const up = await request.post("/api/documents/upload", {
    multipart: { file: { name: "e2e-c05-live.docx", mimeType: DOCX_MIME, buffer }, space: "my" },
  });
  expect(up.ok()).toBeTruthy();
  return (await up.json()).documentId as string;
}

test.describe("c05 真实 ONLYOFFICE 集成（15.1，DS_LIVE）", () => {
  test("真实 DS 渲染合法 docx + c02 挂载 + c05 文档打开默认展示面板", async ({ page, request }) => {
    test.skip(!process.env.DS_LIVE, "需 onlyoffice DS 容器 + dev:web --host；以 DS_LIVE=1 开启");
    test.setTimeout(180_000);

    // 捕获 DS→宿主的编辑器事件协议（onDocumentReady/onPluginsReady 等），用于判定真实 DS 已加载并打开文档。
    await page.addInitScript(() => {
      (window as unknown as { __dsEvents: string[] }).__dsEvents = [];
      window.addEventListener("message", (e) => {
        // DS 经 postMessage 下发的编辑器事件为 JSON 字符串（{"event":"onDocumentReady",...}）；亦兼容对象形态。
        let ev: unknown;
        const d = e.data;
        if (typeof d === "string") {
          try {
            ev = (JSON.parse(d) as { event?: unknown }).event;
          } catch {
            ev = undefined;
          }
        } else if (d && typeof d === "object") {
          ev = (d as { event?: unknown }).event;
        }
        if (typeof ev === "string") {
          (window as unknown as { __dsEvents: string[] }).__dsEvents.push(ev);
        }
      });
    });

    const documentId = await uploadFixtureDocx(request);
    await page.goto(`/editor/${documentId}`);

    // 1) 真实 DS 打开了合法 docx：等 onDocumentReady 事件抵达宿主（DS 完成渲染 + 经 c02 真实下载链路取文件）。
    await page.waitForFunction(
      () => (window as unknown as { __dsEvents?: string[] }).__dsEvents?.includes("onDocumentReady") === true,
      null,
      { timeout: 120_000 },
    );

    // 2) 真实 DS 已加载桥插件（onPluginsReady 在 onDocumentReady 之后抵达 → DS 拉取 host.docker.internal:5173 的 c02 桥插件成功）。
    await page.waitForFunction(
      () => (window as unknown as { __dsEvents?: string[] }).__dsEvents?.includes("onPluginsReady") === true,
      null,
      { timeout: 60_000 },
    );

    // 3) c05「文档打开后默认展示医疗 AI 面板」在真实 DS 上由 onDocumentReady 触发：面板自动展开，§19.3 免责声明 + docx P0 功能可见。
    await expect(page.getByText(/免责声明/).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole("button", { name: "全文润色" })).toBeVisible();
  });
});
