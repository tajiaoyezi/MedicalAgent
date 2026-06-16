import {
  test,
  expect,
  request as playwrightRequest,
  type APIRequestContext,
} from "@playwright/test";

// c05 ai-panel-recent-tasks 后端 API 契约测试（经 vite :5173 反代到 Go :3001，admin cookie）。
// 覆盖：写回确认网关 preview/confirm（经真实编辑器会话取 bridgeToken）、message/translation_job 下发前高风险确认、
// 最近任务六类来源聚合 / 恢复分发 / 继续追问边界 / 删除。admin 不具 highrisk:confirm（普通用户口径）。

async function dump(label: string, resp: import("@playwright/test").APIResponse) {
  if (!resp.ok()) {
    const body = await resp.text().catch(() => "<no body>");
    console.log(`[${label}] status=${resp.status()} body=${body.slice(0, 400)}`);
  }
}

const HIGH_RISK = "推荐剂量 500mg 口服，每日两次，连服七天";
const BENIGN = "这段文字描述了研究方法与样本来源，语句通顺。";

// 上传一个可编辑 docx，返回 document_id。
async function uploadDocx(request: APIRequestContext): Promise<string> {
  const up = await request.post("/api/documents/upload", {
    multipart: {
      file: {
        name: "e2e-c05-writeback.docx",
        mimeType:
          "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        buffer: Buffer.from("MedOffice c05 写回探针\n", "utf-8"),
      },
      space: "my",
    },
  });
  expect(up.ok()).toBeTruthy();
  return (await up.json()).documentId as string;
}

// 打开编辑器会话取 bridgeToken + revision（写回网关入参）。
async function openEditor(
  request: APIRequestContext,
  documentId: string,
): Promise<{ bridgeToken: string; revision: string }> {
  const res = await request.get(`/api/editor/open/${documentId}`);
  await dump("editor-open", res);
  expect(res.status()).toBe(200);
  const json = await res.json();
  expect(json.bridgeToken).toBeTruthy();
  return { bridgeToken: json.bridgeToken, revision: json.revision };
}

test.describe("c05 写回确认网关 API 契约", () => {
  test("preview：四要素 + 默认策略 + 风险 + 免责声明 + 操作按钮", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/preview", {
      data: {
        bridgeToken,
        operationType: "选区润色",
        originalText: "原始选区文本",
        modifiedText: BENIGN,
        explanation: "润色为更学术表述",
        confirmedScope: "第 1 段",
      },
    });
    await dump("preview", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.fourElements).toMatchObject({
      originalText: "原始选区文本",
      impactScope: "selection",
    });
    expect(json.strategy.bridgeMethod).toBe("replaceSelection");
    expect(json.strategy.writebackSource).toBe("ai_writeback");
    expect(typeof json.disclaimer).toBe("string");
    expect(json.disclaimer.length).toBeGreaterThan(0);
    expect(json.actions).toContain("apply");
    expect(json.actions).toContain("cancel");
  });

  test("preview：未知操作类型 400", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/preview", {
      data: { bridgeToken, operationType: "转PPT", originalText: "x", modifiedText: "y" },
    });
    expect(resp.status()).toBe(400);
  });

  test("confirm：低风险 apply 放行并返回写回方法 + 落 doc_ai 最近任务", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken, revision } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/confirm", {
      data: {
        bridgeToken,
        operationType: "选区润色",
        action: "apply",
        originalText: "原始",
        modifiedText: BENIGN,
        expectedRevision: revision,
        confirmedScope: "第 1 段",
      },
    });
    await dump("confirm-apply", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.approved).toBe(true);
    expect(json.bridgeMethod).toBe("replaceSelection");
    expect(json.confirmationId).toBeTruthy();

    // doc_ai 最近任务（source=在线文档 AI 操作、ref_type=writeback_confirmation）已落
    const rt = await request.get("/api/recent-tasks?sources=" + encodeURIComponent("在线文档 AI 操作"));
    const tasks = (await rt.json()).tasks as Array<{ source: string; refType: string; canContinue: boolean }>;
    const hit = tasks.find((t) => t.refType === "writeback_confirmation");
    expect(hit).toBeTruthy();
    expect(hit!.source).toBe("在线文档 AI 操作");
    expect(hit!.canContinue).toBe(false); // 非会话来源不提供继续追问
  });

  test("confirm：高风险 apply 普通用户被拦，仅可提交审核", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken, revision } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/confirm", {
      data: {
        bridgeToken,
        operationType: "选区润色",
        action: "apply",
        originalText: "原始",
        modifiedText: HIGH_RISK,
        expectedRevision: revision,
      },
    });
    expect(resp.status()).toBe(403);
    const json = await resp.json();
    expect(json.requiresHighRiskConfirmation).toBe(true);
  });

  test("confirm：高风险 submit_review 记录待审核", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken, revision } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/confirm", {
      data: {
        bridgeToken,
        operationType: "选区润色",
        action: "submit_review",
        originalText: "原始",
        modifiedText: HIGH_RISK,
        expectedRevision: revision,
      },
    });
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.submittedForReview).toBe(true);
    expect(json.confirmationId).toBeTruthy();
  });

  test("confirm：apply 过期 revision 触发 409 冲突", async ({ request }) => {
    const docId = await uploadDocx(request);
    const { bridgeToken } = await openEditor(request, docId);
    const resp = await request.post("/api/writeback/confirm", {
      data: {
        bridgeToken,
        operationType: "选区润色",
        action: "apply",
        originalText: "原始",
        modifiedText: BENIGN,
        expectedRevision: "stale-revision-xyz",
      },
    });
    expect(resp.status()).toBe(409);
    const json = await resp.json();
    expect(json.staleRevision).toBe(true);
  });
});

test.describe("c05 message/translation_job 下发前高风险确认", () => {
  test("translation_job 高风险 + 普通用户 submit_review", async ({ request }) => {
    const resp = await request.post("/api/writeback/dispatch-confirm", {
      data: {
        subjectType: "translation_job",
        subjectId: crypto.randomUUID(),
        content: HIGH_RISK,
        action: "submit_review",
      },
    });
    await dump("dispatch-submit", resp);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.submittedForReview).toBe(true);
  });

  test("translation_job 高风险 + 普通用户 dispatch 被拦 403", async ({ request }) => {
    const resp = await request.post("/api/writeback/dispatch-confirm", {
      data: {
        subjectType: "translation_job",
        subjectId: crypto.randomUUID(),
        content: HIGH_RISK,
        action: "dispatch",
      },
    });
    expect(resp.status()).toBe(403);
    expect((await resp.json()).requiresHighRiskConfirmation).toBe(true);
  });

  test("translation_job 低风险直接放行（不进确认链路）", async ({ request }) => {
    const resp = await request.post("/api/writeback/dispatch-confirm", {
      data: {
        subjectType: "translation_job",
        subjectId: crypto.randomUUID(),
        content: BENIGN,
        action: "dispatch",
      },
    });
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.approved).toBe(true);
    expect(json.highRisk).toBe(false);
  });

  test("message 主体须本人归属：不存在/非本人 message → 404", async ({ request }) => {
    const resp = await request.post("/api/writeback/dispatch-confirm", {
      data: {
        subjectType: "message",
        subjectId: crypto.randomUUID(),
        content: HIGH_RISK,
        action: "submit_review",
      },
    });
    expect(resp.status()).toBe(404);
  });

  test("非法 subjectType → 400", async ({ request }) => {
    const resp = await request.post("/api/writeback/dispatch-confirm", {
      data: { subjectType: "chunk", subjectId: crypto.randomUUID(), content: "x", action: "dispatch" },
    });
    expect(resp.status()).toBe(400);
  });
});

test.describe("c05 最近任务六类来源聚合 / 恢复分发 / 删除", () => {
  // 在独立上下文里准备多来源任务，避免污染共享会话。
  const created: string[] = [];

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

  async function seed(request: APIRequestContext, source: string, refType: string): Promise<string> {
    const resp = await request.post("/api/recent-tasks", {
      data: { source, title: `c05-e2e ${source}`, refType, refId: crypto.randomUUID() },
    });
    expect(resp.status()).toBe(200);
    const id = (await resp.json()).taskId as string;
    created.push(id);
    return id;
  }

  test("六类来源汇聚 + restorable / canContinue 边界", async ({ request }) => {
    const convId = await seed(request, "AIMed 学术助手", "conversation");
    const transId = await seed(request, "医学翻译", "translation_job");
    const tmplId = await seed(request, "模板生成文档", "document");
    const staffId = await seed(request, "医疗数字员工", "");

    const resp = await request.get("/api/recent-tasks");
    expect(resp.status()).toBe(200);
    const tasks = (await resp.json()).tasks as Array<{
      taskId: string;
      source: string;
      restorable: boolean;
      canContinue: boolean;
    }>;
    const byId = (id: string) => tasks.find((t) => t.taskId === id)!;

    // 继续追问仅会话类（conversation）；数字员工占位不可恢复
    expect(byId(convId).canContinue).toBe(true);
    expect(byId(transId).canContinue).toBe(false);
    expect(byId(tmplId).canContinue).toBe(false);
    expect(byId(staffId).restorable).toBe(false);
    // source 均为中文规范值
    for (const t of tasks) {
      expect(t.source).not.toMatch(/^(aimed|kb_qa|doc_ai|translation|template)$/);
    }
  });

  test("恢复分发器仅凭 ref_type 判定回源", async ({ request }) => {
    const convId = await seed(request, "医疗知识库问答", "conversation");
    const transId = await seed(request, "医学翻译", "translation_job");
    const tmplId = await seed(request, "模板生成文档", "document");

    const conv = await (await request.get(`/api/recent-tasks/${convId}/restore`)).json();
    expect(conv.refType).toBe("conversation");
    expect(conv.action).toBe("open_kb_qa"); // 知识库问答会话
    expect(conv.conversationId).toBeTruthy();

    const trans = await (await request.get(`/api/recent-tasks/${transId}/restore`)).json();
    expect(trans.refType).toBe("translation_job");
    expect(trans.action).toBe("open_translation");

    const tmpl = await (await request.get(`/api/recent-tasks/${tmplId}/restore`)).json();
    expect(tmpl.refType).toBe("document");
    expect(tmpl.action).toBe("open_document");
  });

  test("数字员工恢复 → 规划中、不可恢复", async ({ request }) => {
    const staffId = await seed(request, "医疗数字员工", "");
    const resp = await request.get(`/api/recent-tasks/${staffId}/restore`);
    expect(resp.status()).toBe(200);
    const json = await resp.json();
    expect(json.restorable).toBe(false);
    expect(json.planned).toBe(true);
  });
});
