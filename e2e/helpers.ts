import { expect, type Page, type APIRequestContext } from "@playwright/test";

/** E2E 自建的图片文档名（预览类）。不依赖 smoke 脚本落的 smoke-parse.png，套件自给自足。 */
export const SEED_IMAGE_NAME = "e2e-seed-image.png";

// 1x1 透明 PNG（合法图片，/api/editor/open 判为 preview、/api/preview 返回 200）。
const PNG_1x1 = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC",
  "base64",
);

const DOC_SPACES = ["my", "team", "app"] as const;

/**
 * 确保存在一张可预览的图片文档（找不到就上传），返回其 document_id。
 * 让依赖「既有图片文档」的用例自给自足，不依赖 db seed 之外的 smoke 脚本。
 */
export async function ensureSeedImage(request: APIRequestContext): Promise<string> {
  for (const space of DOC_SPACES) {
    const res = await request.get(`/api/documents?space=${space}`);
    if (!res.ok()) continue;
    const body = (await res.json()) as {
      documents?: Array<{ document_id: string; name: string }>;
    };
    const hit = body.documents?.find((d) => d.name === SEED_IMAGE_NAME);
    if (hit) return hit.document_id;
  }
  const up = await request.post("/api/documents/upload", {
    multipart: {
      file: { name: SEED_IMAGE_NAME, mimeType: "image/png", buffer: PNG_1x1 },
      space: "my",
    },
  });
  expect(up.ok()).toBeTruthy();
  return (await up.json()).documentId as string;
}

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
