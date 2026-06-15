import { test } from "@playwright/test";
import fs from "node:fs";

// 诊断爬虫（非断言）：以 admin 身份逐路由巡检，采集 console error / pageerror /
// 失败的 /api 请求 / 全页截图 / 正文文本样本，落 e2e/.artifacts/explore.json。
// 用于建立「地面真值」后再据此编写正式用例；可随时单独运行 `npx playwright test _explore`。
const ROUTES = [
  "/aimed",
  "/knowledge",
  "/digital-staff",
  "/translation",
  "/templates",
  "/documents",
  "/recent",
  "/admin",
];

interface RouteReport {
  route: string;
  consoleErrors: string[];
  pageErrors: string[];
  failedApi: string[];
  textSample: string;
}

test("crawl all portal routes (diagnostic)", async ({ page }) => {
  const report: RouteReport[] = [];
  fs.mkdirSync("e2e/.artifacts", { recursive: true });

  for (const route of ROUTES) {
    const consoleErrors: string[] = [];
    const pageErrors: string[] = [];
    const failedApi: string[] = [];

    const onConsole = (m: import("@playwright/test").ConsoleMessage) => {
      if (m.type() === "error") consoleErrors.push(m.text());
    };
    const onPageErr = (e: Error) => pageErrors.push(e.message);
    const onResp = (r: import("@playwright/test").Response) => {
      const u = r.url();
      if (u.includes("/api/") && r.status() >= 400) {
        failedApi.push(`${r.status()} ${r.request().method()} ${new URL(u).pathname}`);
      }
    };

    page.on("console", onConsole);
    page.on("pageerror", onPageErr);
    page.on("response", onResp);

    await page.goto(route, { waitUntil: "networkidle" }).catch(() => {});
    await page.waitForTimeout(900);

    const slug = route.replace(/\//g, "_") || "_root";
    await page.screenshot({ path: `e2e/.artifacts/explore${slug}.png`, fullPage: true });
    const textSample = (
      await page.locator("body").innerText().catch(() => "")
    ).slice(0, 2000);

    report.push({ route, consoleErrors, pageErrors, failedApi, textSample });

    page.off("console", onConsole);
    page.off("pageerror", onPageErr);
    page.off("response", onResp);
  }

  fs.writeFileSync("e2e/.artifacts/explore.json", JSON.stringify(report, null, 2));

  for (const r of report) {
    console.log(`\n### ${r.route}`);
    if (r.pageErrors.length) console.log("  pageErrors:", r.pageErrors);
    if (r.consoleErrors.length) console.log("  consoleErrors:", r.consoleErrors.slice(0, 6));
    if (r.failedApi.length) console.log("  failedApi:", r.failedApi);
    console.log("  text:", r.textSample.slice(0, 240).replace(/\s+/g, " "));
  }
});
