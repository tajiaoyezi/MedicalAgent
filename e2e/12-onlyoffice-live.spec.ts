import { test, expect, type APIRequestContext } from "@playwright/test";
import { readFileSync } from "node:fs";

// c05 task 15.1 真实接入测试（真实 ONLYOFFICE DS 全链路写回），**DS 依赖、默认跳过**。
// 开启：① docker compose up -d onlyoffice（DS :8080）；② dev:web --host 0.0.0.0 且 .env 设
// ONLYOFFICE_PLUGIN_URL=http://host.docker.internal:5173/onlyoffice-plugin/；③ DS_LIVE=1 npx playwright test 12-onlyoffice-live。
//
// 验证：面板经 c02 真实桥完成挂载（修复后 plugin→host 走 window.top、host 取 event.source=插件 window 回发命令）→
// 读取真实正文 → replaceSelection 写回（PasteText 光标插入）→ saveDocument（forcesave）→ 保存回调落 ai_writeback 版本。
// headless 无法在 DS 画布内做选区，故经 DEV-only window.__medbridge 驱动桥（生产构建剥离 import.meta.env.DEV 分支）。

const DOCX_MIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document";

async function uploadFixtureDocx(request: APIRequestContext): Promise<string> {
  const buffer = readFileSync("e2e/fixtures/minimal.docx");
  const up = await request.post("/api/documents/upload", {
    multipart: { file: { name: "e2e-c05-live.docx", mimeType: DOCX_MIME, buffer }, space: "my" },
  });
  expect(up.ok()).toBeTruthy();
  return (await up.json()).documentId as string;
}

test.describe("c05 真实 ONLYOFFICE 写回链路（15.1，DS_LIVE）", () => {
  test("挂载→读取→replaceSelection→saveDocument→保存回调落 ai_writeback 版本", async ({ page, request }) => {
    test.skip(!process.env.DS_LIVE, "需 onlyoffice DS 容器 + dev:web --host；以 DS_LIVE=1 开启");
    test.setTimeout(180_000);

    const documentId = await uploadFixtureDocx(request);
    await page.goto(`/editor/${documentId}`);

    // 1) 真实 DS 桥就绪：插件 init 经 window.top 发 medoffice-bridge-ready，宿主 onReady 取 event.source=插件 window 作目标。
    //    __medbridgeReady 置真即证明「plugin→host 通道修复」（此前嵌套 iframe 致此信号永不到达）。
    await page.waitForFunction(() => (window as unknown as { __medbridgeReady?: boolean }).__medbridgeReady === true, null, {
      timeout: 120_000,
    });

    // 2) c05「文档打开默认展示面板」在真实 DS 上触发：§19.3 免责声明 + docx P0 功能可见。
    await expect(page.getByText(/免责声明/).first()).toBeVisible({ timeout: 20_000 });
    await expect(page.getByRole("button", { name: "全文润色" })).toBeVisible();

    // 3) 经真实插件读取（验证 host→plugin 命令 + plugin→host 回包双向往返均工作）。
    //    用 getDocumentType（取自 Asc.plugin.info、不依赖文档内容渲染时机）作稳定的往返证据。
    const docType = await page.evaluate(async () => {
      const b = (window as unknown as { __medbridge?: { getDocumentType: () => Promise<{ data?: { type?: string } }> } }).__medbridge!;
      const r = await b.getDocumentType();
      return r.data?.type ?? "";
    });
    expect(docType).toBe("word");

    // 4) 经真实插件写回（replaceSelection→PasteText 光标插入）。
    await page.evaluate(async () => {
      const b = (window as unknown as { __medbridge?: { replaceSelection: (t: string, o?: string) => Promise<unknown> } }).__medbridge!;
      await b.replaceSelection("【AI 润色】真实写回插入的新正文。", "");
    });
    // 等改动经协同同步到 DS（否则随后 forcesave 见不到改动、error=4 不落版本）。
    await page.waitForTimeout(4000);
    // saveDocument → 宿主 arm 写回意图 → 后端触发 DS forcesave(status=6)，落 ai_writeback 版本。
    await page.evaluate(async () => {
      const b = (window as unknown as { __medbridge?: { saveDocument: (s?: string) => Promise<unknown> } }).__medbridge!;
      await b.saveDocument("ai_writeback");
    });

    // 5) 保存回调异步落库：轮询出现 source=ai_writeback 的新版本（写回可回滚链路在真实 DS 落地）。
    await expect
      .poll(
        async () => {
          const resp = await request.get(`/api/documents/${documentId}`);
          if (!resp.ok()) return false;
          const json = (await resp.json()) as { versions?: Array<{ source: string }> };
          return (json.versions ?? []).some((v) => v.source === "ai_writeback");
        },
        { timeout: 90_000, intervals: [2000, 3000, 5000] },
      )
      .toBe(true);
  });
});
