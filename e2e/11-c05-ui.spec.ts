import {
  test,
  expect,
  request as playwrightRequest,
  type APIRequestContext,
  type Page,
} from "@playwright/test";
import { collectClientErrors, openRoute } from "./helpers";

// c05 前端交互流 e2e（页面级，不依赖 ONLYOFFICE DS）：
// ① 最近任务页按来源差异——查看 / 继续追问边界、前 10 字截断 + 悬浮全标题、删除流；
// ② 编辑器宿主三类入口（右侧固定图标 / 顶部医疗空间 / 选区浮层）与面板内容（免责声明 + docx P0 功能 + 发起入口）。
// 均为 host 侧 React UI，DS 不在 docker 起也可渲染（c05 写回实际落盘的 API 门禁由 10-api-writeback-recent 覆盖）。

const RUN = Math.random().toString(36).slice(2, 8); // 每次运行唯一后缀，避免残留任务串扰

const TASKS = {
  conv: { source: "AIMed 学术助手", refType: "conversation", title: `C5UIconv-${RUN}` },
  docai: { source: "在线文档 AI 操作", refType: "writeback_confirmation", title: `C5UIdocai-${RUN}` },
  staff: { source: "医疗数字员工", refType: "", title: `C5UIstaff-${RUN}` },
  longt: {
    source: "医学翻译",
    refType: "translation_job",
    title: `C5UI长标题这是用于测试前十字截断与悬浮全标题的较长标题-${RUN}`,
  },
} as const;

// 定位某条最近任务行（信息区 title 属性=完整标题，唯一），返回其行容器（父元素）。
function rowByTitle(page: Page, fullTitle: string) {
  return page.locator(`[title="${fullTitle}"]`).locator("..");
}

test.describe("c05 最近任务页：来源差异与列表项操作", () => {
  const created: string[] = [];

  test.beforeAll(async () => {
    const ctx = await playwrightRequest.newContext({
      baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
      storageState: "e2e/.auth/admin.json",
    });
    try {
      for (const t of Object.values(TASKS)) {
        const resp = await ctx.post("/api/recent-tasks", {
          data: { source: t.source, title: t.title, refType: t.refType, refId: t.refType ? crypto.randomUUID() : "" },
        });
        expect(resp.ok()).toBeTruthy();
        created.push((await resp.json()).taskId as string);
      }
    } finally {
      await ctx.dispose();
    }
  });

  test.afterAll(async () => {
    const ctx = await playwrightRequest.newContext({
      baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
      storageState: "e2e/.auth/admin.json",
    });
    try {
      for (const id of created) {
        await ctx.delete(`/api/recent-tasks/${id}`, { data: { deleteLinkedDocument: false } });
      }
    } finally {
      await ctx.dispose();
    }
  });

  test("会话来源（AIMed）行：查看 + 继续追问 均可用", async ({ page }) => {
    const errors = collectClientErrors(page);
    await openRoute(page, "/recent");
    const row = rowByTitle(page, TASKS.conv.title);
    await expect(row).toBeVisible();
    await expect(row.getByRole("button", { name: "查看" })).toBeVisible();
    await expect(row.getByRole("button", { name: "继续追问" })).toBeVisible();
    expect(errors).toEqual([]);
  });

  test("非会话来源（在线文档 AI）行：可查看但无继续追问", async ({ page }) => {
    await openRoute(page, "/recent");
    const row = rowByTitle(page, TASKS.docai.title);
    await expect(row).toBeVisible();
    await expect(row.getByRole("button", { name: "查看" })).toBeVisible();
    await expect(row.getByRole("button", { name: "继续追问" })).toHaveCount(0);
  });

  test("数字员工占位行：不可恢复（无查看 / 无继续追问），仍可重命名 / 删除", async ({ page }) => {
    await openRoute(page, "/recent");
    const row = rowByTitle(page, TASKS.staff.title);
    await expect(row).toBeVisible();
    await expect(row.getByRole("button", { name: "查看" })).toHaveCount(0);
    await expect(row.getByRole("button", { name: "继续追问" })).toHaveCount(0);
    await expect(row.getByRole("button", { name: "重命名" })).toBeVisible();
    await expect(row.getByRole("button", { name: "删除" })).toBeVisible();
  });

  test("标题前 10 字截断 + 悬浮展示完整标题", async ({ page }) => {
    await openRoute(page, "/recent");
    const info = page.locator(`[title="${TASKS.longt.title}"]`);
    await expect(info).toBeVisible();
    // 悬浮全标题：title 属性=完整标题
    await expect(info).toHaveAttribute("title", TASKS.longt.title);
    // 首页展示截断到前 10 字（短于完整标题、且为其前缀）
    const shown = (await info.innerText()).trim().split("\n")[0].trim();
    expect(shown.length).toBeLessThanOrEqual(10);
    expect(TASKS.longt.title.startsWith(shown)).toBeTruthy();
    expect(shown).not.toBe(TASKS.longt.title);
  });

  test("点「查看」经恢复分发器回源（弹恢复来源提示）", async ({ page }) => {
    await openRoute(page, "/recent");
    let dialogMsg = "";
    page.on("dialog", (d) => {
      dialogMsg = d.message();
      void d.accept();
    });
    await rowByTitle(page, TASKS.conv.title).getByRole("button", { name: "查看" }).click();
    await expect.poll(() => dialogMsg).toContain("恢复来源");
    expect(dialogMsg).toContain("AIMed 学术助手");
  });

  test("删除二次确认后行从列表消失", async ({ page }) => {
    await openRoute(page, "/recent");
    // confirm 取消（dismiss）→ 仅软删任务、不删关联文档
    page.on("dialog", (d) => void d.dismiss());
    const row = rowByTitle(page, TASKS.staff.title);
    await expect(row).toBeVisible();
    await row.getByRole("button", { name: "删除" }).click();
    await expect(page.locator(`[title="${TASKS.staff.title}"]`)).toHaveCount(0);
  });
});

test.describe("c05 编辑器宿主：医疗 AI 面板三类入口", () => {
  async function uploadDocx(request: APIRequestContext): Promise<string> {
    const up = await request.post("/api/documents/upload", {
      multipart: {
        file: {
          name: "e2e-c05-panel.docx",
          mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
          buffer: Buffer.from("MedOffice c05 面板入口探针\n", "utf-8"),
        },
        space: "my",
      },
    });
    expect(up.ok()).toBeTruthy();
    return (await up.json()).documentId as string;
  }

  test("三类入口齐备：顶部医疗空间 + 右侧固定图标 + 选区浮层四动作", async ({ page, request }) => {
    const docId = await uploadDocx(request);
    await page.goto(`/editor/${docId}`);
    await expect(page.getByRole("link", { name: /文档中心/ }).first()).toBeVisible({ timeout: 20_000 });

    // 顶部自定义按钮「医疗空间」
    await expect(page.getByRole("button", { name: "医疗空间" })).toBeVisible();
    // 右侧固定图标「医疗 AI」（面板未展开时可见；DS 未起→不会自动展开面板）
    await expect(page.getByTitle("医疗 AI")).toBeVisible();
    // 选区浮层四动作
    await expect(page.getByText("选区浮层")).toBeVisible();
    for (const a of ["润色", "翻译", "解释", "补引用"]) {
      await expect(page.getByRole("button", { name: a, exact: true })).toBeVisible();
    }
  });

  test("打开面板：免责声明 + docx P0 功能集 + 发起入口；关闭后图标复现", async ({ page, request }) => {
    const docId = await uploadDocx(request);
    await page.goto(`/editor/${docId}`);
    await expect(page.getByRole("link", { name: /文档中心/ }).first()).toBeVisible({ timeout: 20_000 });

    await page.getByRole("button", { name: "医疗空间" }).click();

    // 面板顶部 §19.3 免责声明
    await expect(page.getByText(/免责声明/).first()).toBeVisible();
    // docx P0 写回类 + 只读 + 发起入口（剔除 §22.2 项不应出现）
    for (const s of ["全文润色", "选区润色", "校对", "AI 论文排版", "插入标注", "辅助显示", "从当前文档发起 AIMed", "从当前文档发起医学翻译"]) {
      await expect(page.getByRole("button", { name: s })).toBeVisible();
    }
    await expect(page.getByRole("button", { name: /论文转\s*PPT|文档脑图|生成\s*PPT/ })).toHaveCount(0);

    // 关闭面板 → 右侧固定图标复现
    await page.getByRole("button", { name: "关闭" }).click();
    await expect(page.getByTitle("医疗 AI")).toBeVisible();
  });
});
