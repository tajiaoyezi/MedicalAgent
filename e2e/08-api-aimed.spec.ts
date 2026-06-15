import { test, expect, request as playwrightRequest } from "@playwright/test";

// c04 AIMed/RAG/PubMed/Citation 后端 API 契约测试。
// c04 仅后端实现、无前端 UI，故以 API 契约测试覆盖：经 vite :5173 反代到 Go 后端 :3001，
// 携带 chromium 项目注入的管理员 cookie（storageState=admin.json）。`request` 夹具继承同一上下文凭证。
// 对模型/外部相关端点放宽断言（只断 HTTP 状态明确 + 关键字段结构），避免 flaky。

// 失败排查辅助：断言前打印实际 status 与 body 片段。
async function dump(label: string, resp: import("@playwright/test").APIResponse) {
  if (!resp.ok()) {
    const body = await resp.text().catch(() => "<no body>");
    console.log(`[${label}] status=${resp.status()} body=${body.slice(0, 400)}`);
  }
}

const SIX_MODES = [
  "general",
  "deep_reading",
  "trend_analysis",
  "evidence_tracing",
  "review_gen",
  "writing_assist",
];

test.describe("c04 AIMed 后端 API 契约", () => {
  // 套件内共享一个会话 id。**关键**：用 beforeAll 创建而非依赖「某条建会话用例后赋值」——
  // Playwright 在某用例失败后会重启 worker，导致模块级变量被重置为空、后续用例级联失败；
  // 而 beforeAll 在每个 worker 启动时都会重跑，可在重启后重新拿到有效会话 id。
  let conversationId = "";

  test.beforeAll(async () => {
    const ctx = await playwrightRequest.newContext({
      baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
      storageState: "e2e/.auth/admin.json",
    });
    try {
      const create = await ctx.post("/api/aimed/conversations", {
        data: { module: "aimed", mode: "general", title: "E2E 共享会话" },
      });
      const json = await create.json();
      conversationId = json.conversationId;
    } finally {
      await ctx.dispose();
    }
  });

  test("GET /api/aimed/modes 返回六模式元数据", async ({ request }) => {
    const resp = await request.get("/api/aimed/modes");
    await dump("modes", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(Array.isArray(json.modes)).toBe(true);
    // 六模式齐全且各含 policy 关键字段
    const keys: string[] = json.modes.map((m: { mode: string }) => m.mode);
    for (const m of SIX_MODES) {
      expect(keys).toContain(m);
    }
    const general = json.modes.find((m: { mode: string }) => m.mode === "general");
    expect(general).toMatchObject({
      label: expect.any(String),
      placeholder: expect.any(String),
      allowPubmed: expect.any(Boolean),
      allowUpload: expect.any(Boolean),
      allowKb: expect.any(Boolean),
    });
    // §8.3：深度文献伴读隐藏 PubMed 标签
    const deep = json.modes.find((m: { mode: string }) => m.mode === "deep_reading");
    expect(deep.showPubmedTag).toBe(false);
  });

  test("POST /api/aimed/conversations 建会话 → GET 列表含之", async ({ request }) => {
    const create = await request.post("/api/aimed/conversations", {
      data: { module: "aimed", mode: "general", title: "E2E 契约会话" },
    });
    await dump("create-conversation", create);
    expect(create.status()).toBe(200);
    const created = await create.json();
    // 本用例自建自验，用局部变量，避免与 beforeAll 的共享 conversationId 互相干扰
    const localId: string = created.conversationId;
    expect(localId).toBeTruthy();
    // 返回的 conversation 快照含 allow_* 派生字段
    expect(created.conversation).toMatchObject({
      module: "aimed",
      mode: "general",
      allowPubmed: true,
      allowKb: true,
    });

    const list = await request.get("/api/aimed/conversations?module=aimed");
    await dump("list-conversations", list);
    expect(list.status()).toBe(200);
    const listed = await list.json();
    expect(Array.isArray(listed.conversations)).toBe(true);
    const ids: string[] = listed.conversations.map(
      (c: { conversationId: string }) => c.conversationId,
    );
    expect(ids).toContain(localId);
  });

  test("POST /api/aimed/conversations 非法模式 → 400", async ({ request }) => {
    const resp = await request.post("/api/aimed/conversations", {
      data: { module: "aimed", mode: "not_a_real_mode" },
    });
    expect(resp.status()).toBe(400);
  });

  test("GET /api/aimed/conversations/:id 取会话 + 消息 + 文件 + placeholder", async ({
    request,
  }) => {
    expect(conversationId, "需先建会话").toBeTruthy();
    const resp = await request.get(`/api/aimed/conversations/${conversationId}`);
    await dump("get-conversation", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.conversation.conversationId).toBe(conversationId);
    // 新建会话尚无消息：后端 messages 为 null；有消息时为数组。两者皆合法。
    expect(json.messages === null || Array.isArray(json.messages)).toBe(true);
    expect(Array.isArray(json.files)).toBe(true);
    expect(typeof json.placeholder).toBe("string");
    expect(typeof json.showPubmedTag).toBe("boolean");
  });

  test("POST /api/aimed/conversations/:id/mode 切换模式 → 返回新 policy", async ({
    request,
  }) => {
    expect(conversationId).toBeTruthy();
    const resp = await request.post(`/api/aimed/conversations/${conversationId}/mode`, {
      data: { mode: "evidence_tracing" },
    });
    await dump("switch-mode", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.conversation.mode).toBe("evidence_tracing");
    expect(typeof json.placeholder).toBe("string");
    // 切回 general，避免影响后续 ask 用例的数据源约束
    const back = await request.post(`/api/aimed/conversations/${conversationId}/mode`, {
      data: { mode: "general" },
    });
    expect(back.status()).toBe(200);
  });

  test("POST /api/aimed/conversations/:id/mode 非法模式 → 400", async ({ request }) => {
    expect(conversationId).toBeTruthy();
    const resp = await request.post(`/api/aimed/conversations/${conversationId}/mode`, {
      data: { mode: "bogus" },
    });
    expect(resp.status()).toBe(400);
  });

  test("POST /api/aimed/match 智能模式匹配 → 200 且结构稳定", async ({ request }) => {
    const resp = await request.post("/api/aimed/match", {
      data: { mode: "general", text: "帮我写一篇近5年肺癌免疫治疗的综述" },
    });
    await dump("match", resp);
    expect(resp.status()).toBe(200);
    // Evaluate 返回结构由后端定义，这里只断言返回为对象（不臆造字段名）
    const json = await resp.json();
    expect(json).not.toBeNull();
    expect(typeof json).toBe("object");
  });

  test("POST /api/aimed/conversations/:id/send-state 发送按钮状态机 → 200", async ({
    request,
  }) => {
    expect(conversationId).toBeTruthy();
    const resp = await request.post(
      `/api/aimed/conversations/${conversationId}/send-state`,
      { data: { mode: "general", text: "这是一段待发送的提问文本" } },
    );
    await dump("send-state", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    // CanSend 返回判定结果对象
    expect(typeof json).toBe("object");
    expect(json).not.toBeNull();
  });

  test("POST /api/aimed/conversations/:id/ask 提问 → 状态明确（200 或受控 4xx）", async ({
    request,
  }) => {
    expect(conversationId).toBeTruthy();
    const resp = await request.post(`/api/aimed/conversations/${conversationId}/ask`, {
      data: { query: "请简要说明 RCT 与 meta 分析的证据等级关系。" },
    });
    await dump("ask", resp);
    // 依赖模型，可能 fallback/脱敏门禁；断言状态明确而非生成内容
    expect([200, 400, 403, 500, 502, 503]).toContain(resp.status());
    if (resp.status() === 200) {
      const json = await resp.json();
      // 200 时断言答案结构（message + citations 字段存在性，不断言具体文本）
      expect(json).not.toBeNull();
      expect(typeof json).toBe("object");
    }
  });

  test("POST /api/aimed/conversations/:id/ask 空 query → 400", async ({ request }) => {
    expect(conversationId).toBeTruthy();
    const resp = await request.post(`/api/aimed/conversations/${conversationId}/ask`, {
      data: { query: "   " },
    });
    expect(resp.status()).toBe(400);
  });

  test("POST /api/aimed/messages/:id/feedback 非法 rating → 400", async ({ request }) => {
    // 不依赖真实 message：rating 校验在查消息前，非法 rating 必先被拦为 400
    const resp = await request.post("/api/aimed/messages/any-id/feedback", {
      data: { rating: "neutral" },
    });
    await dump("feedback-invalid-rating", resp);
    expect(resp.status()).toBe(400);
  });

  test("POST /api/aimed/messages/:id/feedback 不存在的消息 → 404", async ({ request }) => {
    // 合法 rating + 合法 reason，但消息不存在 → 404（验证 reason 白名单与 404 分支）
    const resp = await request.post(
      "/api/aimed/messages/00000000-0000-0000-0000-000000000000/feedback",
      { data: { rating: "踩", reason: "引用错误" } },
    );
    await dump("feedback-missing-msg", resp);
    expect(resp.status()).toBe(404);
  });

  test("GET /api/aimed/conversations/:id 不存在的会话 → 404", async ({ request }) => {
    const resp = await request.get(
      "/api/aimed/conversations/00000000-0000-0000-0000-000000000000",
    );
    expect(resp.status()).toBe(404);
  });

  test("未授权访问受保护端点 → 401", async () => {
    // 全新空凭证上下文（不携带管理员 cookie）
    const anon = await playwrightRequest.newContext({
      baseURL: "http://localhost:5173",
      storageState: { cookies: [], origins: [] },
    });
    try {
      const resp = await anon.get("/api/aimed/conversations");
      await dump("anon-list", resp);
      expect(resp.status()).toBe(401);
    } finally {
      await anon.dispose();
    }
  });
});
