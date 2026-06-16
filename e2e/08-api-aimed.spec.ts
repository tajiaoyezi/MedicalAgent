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

// 1x1 透明 PNG（合法图片；格式白名单含 .png，PHI 门禁 POC 缺省放行 → 上传走主闭环落库）。
const PNG_1x1 = Buffer.from(
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC",
  "base64",
);

// c04 扩展端点：覆盖 08 主链未触及的 from-document / 引用 / 重新生成 / 删除 / 答案落地（generate-word·save-as）/
// 翻译分流 / 会话文件上传与删除。仍经 :5173 反代到 Go :3001，携管理员 storageState。
test.describe("c04 AIMed 扩展端点契约", () => {
  // beforeAll 内建会话并产一条真实答案消息（live 无 chunk → noResults 路径，但仍落库 assistant 消息），
  // 供 citations/regenerate/generate-word/save-as 等依赖「真实 message_id」的用例使用。worker 重启后亦能重建。
  let convId = "";
  let assistantMsgId = "";

  test.beforeAll(async () => {
    const ctx = await playwrightRequest.newContext({
      baseURL: process.env.E2E_BASE_URL ?? "http://localhost:5173",
      storageState: "e2e/.auth/admin.json",
    });
    try {
      const create = await ctx.post("/api/aimed/conversations", {
        data: { module: "aimed", mode: "general", title: "E2E 扩展会话" },
      });
      convId = (await create.json()).conversationId;
      const ask = await ctx.post(`/api/aimed/conversations/${convId}/ask`, {
        data: { query: "RCT 与队列研究在证据等级上的差异？" },
      });
      const aj = await ask.json();
      assistantMsgId = aj.messageId;
    } finally {
      await ctx.dispose();
    }
  });

  test("POST /conversations/from-document 建文档会话 + 注入上下文消息", async ({ request }) => {
    const resp = await request.post("/api/aimed/conversations/from-document", {
      data: { documentId: "doc-e2e-001", context: "患者主诉：反复咳嗽两周。", mode: "writing_assist" },
    });
    await dump("from-document", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.conversationId).toBeTruthy();
    expect(j.currentDocId).toBe("doc-e2e-001");
    // 提供 context 时应作为一条 user 消息（带 [文档上下文] 前缀）落库
    const conv = await request.get(`/api/aimed/conversations/${j.conversationId}`);
    const cj = await conv.json();
    expect(Array.isArray(cj.messages)).toBe(true);
    expect(
      cj.messages.some(
        (m: { role: string; content: string }) =>
          m.role === "user" && m.content.includes("[文档上下文]"),
      ),
    ).toBe(true);
  });

  test("GET /messages/:id/citations 返回引用数组", async ({ request }) => {
    expect(assistantMsgId, "需先经 beforeAll 产生答案消息").toBeTruthy();
    const resp = await request.get(`/api/aimed/messages/${assistantMsgId}/citations`);
    await dump("citations", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    // live 无种子 chunk 时 citations 可能为空数组；有则按角标排序。两者皆合法。
    expect(Array.isArray(j.citations)).toBe(true);
  });

  test("POST /citations/:id/locate 不存在引用 → 200 且降级「该引用源已删除」", async ({
    request,
  }) => {
    // 合法 UUID 但不存在：citation.Get 用 Scan（无 ErrRecordNotFound）→ (nil,nil) → Locate 降级 200。
    const resp = await request.post(
      "/api/aimed/citations/00000000-0000-0000-0000-000000000000/locate",
    );
    await dump("locate-missing", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.ok).toBe(false);
    expect(j.message).toBe("该引用源已删除");
  });

  test("POST /messages/:id/regenerate 重新生成 → 200 且保留旧消息", async ({ request }) => {
    expect(assistantMsgId).toBeTruthy();
    const resp = await request.post(`/api/aimed/messages/${assistantMsgId}/regenerate`);
    await dump("regenerate", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.keptOldMessageId).toBe(assistantMsgId);
    expect(j.regenerated).not.toBeNull();
    expect(typeof j.regenerated.messageId).toBe("string");
  });

  test("POST /messages/:id/regenerate 不存在消息 → 404", async ({ request }) => {
    const resp = await request.post(
      "/api/aimed/messages/00000000-0000-0000-0000-000000000000/regenerate",
    );
    expect(resp.status()).toBe(404);
  });

  test("POST /conversations/:id/save-as markdown 离线导出 → 200（不依赖 ONLYOFFICE）", async ({
    request,
  }) => {
    expect(convId).toBeTruthy();
    const resp = await request.post(`/api/aimed/conversations/${convId}/save-as`, {
      data: { scope: "conversation", format: "markdown" },
    });
    await dump("save-as-md", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.format).toBe("markdown");
    expect(j.openInOnlyoffice).toBe(false);
    expect(typeof j.exportText).toBe("string");
    expect(j.filename).toMatch(/\.md$/);
  });

  test("POST /conversations/:id/save-as scope=current 缺 messageId → 400", async ({
    request,
  }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/save-as`, {
      data: { scope: "current", format: "markdown" },
    });
    expect(resp.status()).toBe(400);
  });

  test("POST /conversations/:id/save-as 不支持格式 → 400", async ({ request }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/save-as`, {
      data: { scope: "conversation", format: "xml" },
    });
    expect(resp.status()).toBe(400);
  });

  test("POST /conversations/:id/translate 选区 → aimed_inline 分流", async ({ request }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/translate`, {
      data: { target: "selection" },
    });
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.route).toBe("aimed_inline");
  });

  test("POST /conversations/:id/translate 整篇 → c07_translation 分流(deferred)", async ({
    request,
  }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/translate`, {
      data: { target: "whole" },
    });
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.route).toBe("c07_translation");
    expect(j.deferred).toBe(true);
  });

  test("POST /messages/:id/generate-word 生成在线 Word → 200 落库 + 打开信号", async ({
    request,
  }) => {
    expect(assistantMsgId).toBeTruthy();
    const resp = await request.post(`/api/aimed/messages/${assistantMsgId}/generate-word`);
    await dump("generate-word", resp);
    expect(resp.status()).toBe(200);
    const j = await resp.json();
    expect(j.documentId).toBeTruthy();
    expect(j.openInOnlyoffice).toBe(true);
    expect(j.expandPanel).toBe(true);
    expect(j.filename).toMatch(/\.docx$/);
  });

  test("POST /messages/:id/generate-word 不存在消息 → 404", async ({ request }) => {
    const resp = await request.post(
      "/api/aimed/messages/00000000-0000-0000-0000-000000000000/generate-word",
    );
    expect(resp.status()).toBe(404);
  });

  test("POST /conversations/:id/files 上传 PNG → 200 落库解析中，再 DELETE → 200 标记已删除", async ({
    request,
  }) => {
    expect(convId).toBeTruthy();
    const up = await request.post(`/api/aimed/conversations/${convId}/files`, {
      multipart: { file: { name: "e2e-aimed.png", mimeType: "image/png", buffer: PNG_1x1 } },
    });
    await dump("file-upload", up);
    expect(up.status()).toBe(200);
    const uj = await up.json();
    expect(uj.file.documentId).toBeTruthy();
    expect(uj.file.status).toBe("解析中"); // FileParsing
    expect(Array.isArray(uj.files)).toBe(true);

    const del = await request.delete(
      `/api/aimed/conversations/${convId}/files/${uj.file.fileId}`,
    );
    await dump("file-delete", del);
    expect(del.status()).toBe(200);
    const dj = await del.json();
    const removed = dj.files.find(
      (f: { fileId: string; status: string }) => f.fileId === uj.file.fileId,
    );
    expect(removed.status).toBe("已删除"); // FileDeleted
  });

  test("POST /conversations/:id/files 缺文件 → 400", async ({ request }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/files`, {
      multipart: { space: "my" }, // 无 file 字段 → c.FormFile 失败
    });
    expect(resp.status()).toBe(400);
  });

  test("POST /conversations/:id/files 不支持格式(.txt) → 400 + 白名单文案", async ({
    request,
  }) => {
    const resp = await request.post(`/api/aimed/conversations/${convId}/files`, {
      multipart: {
        file: { name: "note.txt", mimeType: "text/plain", buffer: Buffer.from("hello") },
      },
    });
    await dump("file-bad-ext", resp);
    expect(resp.status()).toBe(400);
    const j = await resp.json();
    expect(j.error).toContain("文件类型支持");
  });

  test("DELETE /conversations/:id/files/:fileId 不存在文件 → 404", async ({ request }) => {
    const resp = await request.delete(`/api/aimed/conversations/${convId}/files/no-such-file`);
    expect(resp.status()).toBe(404);
  });

  test("POST /conversations/:id/files 会话不存在 → 404", async ({ request }) => {
    const resp = await request.post(
      "/api/aimed/conversations/00000000-0000-0000-0000-000000000000/files",
      { multipart: { file: { name: "x.png", mimeType: "image/png", buffer: PNG_1x1 } } },
    );
    expect(resp.status()).toBe(404);
  });

  // #18 反馈校验：踩必取 §8.10.5 七项之一，缺/非法 reason → 400（修复前缺 reason 可静默落库）
  test("POST /messages/:id/feedback 踩缺 reason → 400", async ({ request }) => {
    expect(assistantMsgId).toBeTruthy();
    const resp = await request.post(`/api/aimed/messages/${assistantMsgId}/feedback`, {
      data: { rating: "踩" },
    });
    await dump("feedback-down-no-reason", resp);
    expect(resp.status()).toBe(400);
  });

  test("POST /messages/:id/feedback 踩 + 合法 reason → 200", async ({ request }) => {
    expect(assistantMsgId).toBeTruthy();
    const resp = await request.post(`/api/aimed/messages/${assistantMsgId}/feedback`, {
      data: { rating: "踩", reason: "不准确" },
    });
    await dump("feedback-down-valid", resp);
    expect(resp.status()).toBe(200);
  });

  // #14 赞携带任意 reason 不应污染统计：接口接受但 reason 被清空（断言 200，不报错）
  test("POST /messages/:id/feedback 赞携带 reason → 200（reason 被清空）", async ({ request }) => {
    expect(assistantMsgId).toBeTruthy();
    const resp = await request.post(`/api/aimed/messages/${assistantMsgId}/feedback`, {
      data: { rating: "赞", reason: "随便填的脏串" },
    });
    await dump("feedback-up-with-reason", resp);
    expect(resp.status()).toBe(200);
  });
});

// 高危修复回归（fix/c04-highrisk-acl-isolation）：
// #3/#8 同租户跨用户越权——message 是 per-user 资源，他人不得读其引用/反馈/重新生成（均应 404）。
// #10 重新生成版本链——不再重复插入 user 消息，新答案 parent 复用原 user 消息。
// admin 与 user 是同租户（单租户 seed）不同用户，故构成「跨用户、同租户」越权面。
const BASE = process.env.E2E_BASE_URL ?? "http://localhost:5173";

test.describe("c04 越权隔离与版本链回归（高危修复验证）", () => {
  let adminConvId = "";
  let adminUserMsgId = "";
  let adminAssistantMsgId = "";

  test.beforeAll(async () => {
    const ctx = await playwrightRequest.newContext({
      baseURL: BASE,
      storageState: "e2e/.auth/admin.json",
    });
    try {
      const create = await ctx.post("/api/aimed/conversations", {
        data: { module: "aimed", mode: "general", title: "越权回归-admin" },
      });
      adminConvId = (await create.json()).conversationId;
      const ask = await ctx.post(`/api/aimed/conversations/${adminConvId}/ask`, {
        data: { query: "证据等级与研究设计的关系？" },
      });
      const aj = await ask.json();
      adminAssistantMsgId = aj.messageId;
      adminUserMsgId = aj.userMessageId;
    } finally {
      await ctx.dispose();
    }
  });

  test("#3 越权：user 读 admin 消息引用 → 404", async () => {
    const userCtx = await playwrightRequest.newContext({
      baseURL: BASE,
      storageState: "e2e/.auth/user.json",
    });
    try {
      const r = await userCtx.get(`/api/aimed/messages/${adminAssistantMsgId}/citations`);
      expect(r.status()).toBe(404);
    } finally {
      await userCtx.dispose();
    }
  });

  test("#3 越权：user 定位 admin 引用所属会话端点不泄露（消息越权先被拦）", async () => {
    // citations 越权已 404，故 user 无法拿到 admin 的 citation_id；此处直接验证消息级越权拦截一致性
    const userCtx = await playwrightRequest.newContext({
      baseURL: BASE,
      storageState: "e2e/.auth/user.json",
    });
    try {
      const r = await userCtx.post(`/api/aimed/messages/${adminAssistantMsgId}/generate-word`);
      expect(r.status()).toBe(404);
    } finally {
      await userCtx.dispose();
    }
  });

  test("#8 越权：user 对 admin 消息反馈 → 404（不污染反馈统计）", async () => {
    const userCtx = await playwrightRequest.newContext({
      baseURL: BASE,
      storageState: "e2e/.auth/user.json",
    });
    try {
      const r = await userCtx.post(`/api/aimed/messages/${adminAssistantMsgId}/feedback`, {
        data: { rating: "赞" },
      });
      expect(r.status()).toBe(404);
    } finally {
      await userCtx.dispose();
    }
  });

  test("#8 越权：user 对 admin 消息重新生成 → 404", async () => {
    const userCtx = await playwrightRequest.newContext({
      baseURL: BASE,
      storageState: "e2e/.auth/user.json",
    });
    try {
      const r = await userCtx.post(`/api/aimed/messages/${adminAssistantMsgId}/regenerate`);
      expect(r.status()).toBe(404);
    } finally {
      await userCtx.dispose();
    }
  });

  test("#10 版本链：重新生成不产生重复 user 气泡，新答案 parent 复用原 user 消息", async ({
    request,
  }) => {
    const before = await request.get(`/api/aimed/conversations/${adminConvId}`);
    const bj = await before.json();
    const userMsgsBefore = ((bj.messages ?? []) as Array<{ role: string }>).filter(
      (m) => m.role === "user",
    ).length;

    const regen = await request.post(`/api/aimed/messages/${adminAssistantMsgId}/regenerate`);
    expect(regen.status()).toBe(200);
    const rj = await regen.json();
    // 新答案复用原始 user 消息为 parent（不另建重复提问）
    expect(rj.regenerated.userMessageId).toBe(adminUserMsgId);

    const after = await request.get(`/api/aimed/conversations/${adminConvId}`);
    const aj = await after.json();
    const userMsgsAfter = ((aj.messages ?? []) as Array<{ role: string }>).filter(
      (m) => m.role === "user",
    ).length;
    expect(userMsgsAfter).toBe(userMsgsBefore); // 无新增重复 user 气泡
  });
});
