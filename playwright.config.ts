import { defineConfig, devices } from "@playwright/test";

// E2E 套件针对「已在本机运行」的前后端：
//   前端 vite dev :5173（/api 代理到 Go 后端 :3001），依赖 docker compose（PG/MinIO/ONLYOFFICE）。
//   先 `npm run dev:api` 与 `npm run dev:web`，再 `npm run test:e2e`。
// 单租户共享种子数据 + 服务端 memstore 会话，故 workers=1 串行执行避免互相干扰。
export default defineConfig({
  testDir: "./e2e",
  // 测试运行产物（trace/test-results）单独目录；与人工截图/日志的 e2e/.artifacts 分开，
  // 避免 Playwright 每次运行清空 outputDir 时连带删除截图与日志。
  outputDir: "./e2e/.test-results",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 45_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never", outputFolder: "e2e/.report" }]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    actionTimeout: 12_000,
    navigationTimeout: 20_000,
    locale: "zh-CN",
  },
  projects: [
    { name: "setup", testMatch: /auth\.setup\.ts/ },
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], storageState: "e2e/.auth/admin.json" },
      dependencies: ["setup"],
    },
  ],
});
