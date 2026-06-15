import { test, expect, type Locator } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import {
  collectClientErrors,
  ensureSeedImage,
  openRoute,
  SEED_IMAGE_NAME,
  snapshot,
} from "./helpers";

/**
 * 删除当前列表里所有同名行，逐行删到 0，得到干净基线。
 * 上传不按文件名去重，多次运行会累积同名文档；故断言「恰好 1 行」前需先清干净。
 * 调用前须已注册 `page.on("dialog", (d) => d.accept())` 处理原生删除确认。
 */
async function deleteAllByName(table: Locator, name: string): Promise<void> {
  // 关键：先等表体加载完成再计数——表格在 /api/documents 返回后才渲染，
  // openRoute 只等门户外壳；过早 count() 会得到 0、漏删导致同名行累积。
  await expect(table).toBeVisible();
  const rows = table.locator("tbody tr").filter({ hasText: name });
  for (let guard = 0; guard < 50; guard++) {
    const c = await rows.count();
    if (c === 0) break;
    await rows.first().getByRole("button", { name: "删除" }).click();
    await expect(rows).toHaveCount(c - 1);
  }
}

// 文档中心（管理员视角）：tab 切换、列表渲染、上传、删除、打开跳转。
// 源真值：DocumentsPage.tsx —— 四个 tab 为 <button>（我的文档/团队文档/应用文档/回收站，
// 选中态 btn-primary）；上传为 <label class=btn-primary> 含「上传文件」+ 隐藏 input[type=file]；
// 既有图片文档由 ensureSeedImage 自建（owner，操作 打开/下载/删除）；删除用原生 window.confirm
// 「确认删除到回收站？」；「打开」navigate 到 /editor/:id（EditorPage 可能 redirect 到 /preview/:id）。

const ROUTE = "/documents";
const ARTIFACT_DIR = "e2e/.artifacts";
// 固定文件名（脚本环境禁用 Math.random）；先删后传或容忍已存在。
const UPLOAD_NAME = "e2e-upload.txt";

test.describe("文档中心", () => {
  test("四个 tab 可切换且列表渲染既有文档", async ({ page, request }) => {
    const errors = collectClientErrors(page);
    await ensureSeedImage(request); // 确保存在一张既有图片文档（套件自给自足，不依赖 smoke 脚本）
    await openRoute(page, ROUTE);

    // 关键渲染：面包屑、标题、上传入口、表头。
    await expect(page.getByText("文档与任务 · 文档中心")).toBeVisible();
    // 上传入口是 <label class=btn-primary>（含隐藏 input[type=file]），非 <button>，故按文本断言
    await expect(page.getByText("上传文件")).toBeVisible();
    const table = page.locator("table.tbl");
    await expect(table.locator("thead").getByText("名称")).toBeVisible();
    await expect(table.locator("thead").getByText("权限")).toBeVisible();
    await expect(table.locator("thead").getByText("操作")).toBeVisible();

    // 既有图片文档：owner 权限 + 操作含 打开/下载/删除。
    const seedRow = table.locator("tbody tr").filter({ hasText: SEED_IMAGE_NAME });
    await expect(seedRow).toHaveCount(1);
    await expect(seedRow.getByText("owner")).toBeVisible();
    await expect(seedRow.getByRole("button", { name: "打开" })).toBeVisible();
    await expect(seedRow.getByRole("button", { name: "下载" })).toBeVisible();
    await expect(seedRow.getByRole("button", { name: "删除" })).toBeVisible();
    await snapshot(page, "documents-my");

    // 四个 tab 逐个切换，断言其变为选中态（btn-primary）。
    for (const label of ["团队文档", "应用文档", "回收站", "我的文档"]) {
      const tab = page.getByRole("button", { name: label, exact: true });
      await tab.click();
      await expect(tab).toHaveClass(/btn-primary/);
    }
    await snapshot(page, "documents-recycle-then-my");

    expect(errors).toEqual([]);
  });

  test("上传文件后出现在我的文档列表", async ({ page }) => {
    const errors = collectClientErrors(page);

    // 在 e2e/.artifacts 写一个临时小文件用于上传。
    fs.mkdirSync(ARTIFACT_DIR, { recursive: true });
    const filePath = path.join(ARTIFACT_DIR, UPLOAD_NAME);
    fs.writeFileSync(filePath, "MedOffice E2E 上传探针文本。\n", "utf-8");

    page.on("dialog", (d) => d.accept());
    await openRoute(page, ROUTE);
    const table = page.locator("table.tbl");

    // 清掉历史遗留的同名行，建立干净基线（上传不去重名，会累积）。
    await deleteAllByName(table, UPLOAD_NAME);

    // 触发上传：直接对隐藏的 input[type=file] setInputFiles。
    await page.locator('input[type="file"]').setInputFiles(filePath);
    await expect(page.getByText("上传成功")).toBeVisible();

    // 干净基线下上传 1 个，应恰好 1 行。
    await expect(
      table.locator("tbody tr").filter({ hasText: UPLOAD_NAME }),
    ).toHaveCount(1);
    await snapshot(page, "documents-after-upload");

    expect(errors).toEqual([]);
  });

  test("删除自己上传的文件后从列表消失", async ({ page }) => {
    const errors = collectClientErrors(page);
    page.on("dialog", (d) => d.accept());

    // 先确保探针文件存在（独立用例，不依赖上一条执行顺序）。
    fs.mkdirSync(ARTIFACT_DIR, { recursive: true });
    const filePath = path.join(ARTIFACT_DIR, UPLOAD_NAME);
    fs.writeFileSync(filePath, "MedOffice E2E 删除探针文本。\n", "utf-8");

    await openRoute(page, ROUTE);
    const table = page.locator("table.tbl");

    // 干净基线：清掉历史遗留，再上传恰好 1 个作为待删项。
    await deleteAllByName(table, UPLOAD_NAME);
    await page.locator('input[type="file"]').setInputFiles(filePath);
    await expect(page.getByText("上传成功")).toBeVisible();
    await expect(
      table.locator("tbody tr").filter({ hasText: UPLOAD_NAME }),
    ).toHaveCount(1);

    // 点删除（原生 confirm 已自动 accept），断言它从「我的文档」消失。
    await table
      .locator("tbody tr")
      .filter({ hasText: UPLOAD_NAME })
      .first()
      .getByRole("button", { name: "删除" })
      .click();
    await expect(
      table.locator("tbody tr").filter({ hasText: UPLOAD_NAME }),
    ).toHaveCount(0);
    await snapshot(page, "documents-after-delete");

    // 进一步证据：删除后应进入回收站。注意多次运行会累积同名软删条目，故断言「至少 1 条」。
    await page.getByRole("button", { name: "回收站", exact: true }).click();
    await expect(
      page.locator("table.tbl tbody tr").filter({ hasText: UPLOAD_NAME }).first(),
    ).toBeVisible();

    expect(errors).toEqual([]);
  });

  test("点既有文档「打开」跳转到编辑器或预览路由", async ({ page, request }) => {
    const errors = collectClientErrors(page);
    await ensureSeedImage(request); // 确保存在一张既有图片文档
    await openRoute(page, ROUTE);

    const seedRow = page
      .locator("table.tbl tbody tr")
      .filter({ hasText: SEED_IMAGE_NAME });
    await expect(seedRow).toHaveCount(1);
    await seedRow.getByRole("button", { name: "打开" }).click();

    // EditorPage 对图片类会 navigate 到 /preview/:id；故 url 命中 /editor/ 或 /preview/ 其一。
    await expect(page).toHaveURL(/\/(editor|preview)\//);
    await snapshot(page, "documents-open-editor");

    // 编辑器/预览页是 ONLYOFFICE 宿主，可能因 DS 未就绪产生 console error，故此处不加错误守卫。
  });
});
