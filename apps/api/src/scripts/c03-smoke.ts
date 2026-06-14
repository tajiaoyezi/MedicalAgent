// c03 端到端验收冒烟（§24.9 本期可验证半段）。需本地 docker（PostgreSQL + MinIO）已起且已 migrate。
// 内置 mock 模型/视觉服务代表「私有化/本地」endpoint；公网路径本期默认拒绝（c09 未接入），故不验证公网放行。
// 运行：npm run smoke:c03 --workspace=apps/api
import { createServer, type Server } from "node:http";
import { v4 as uuidv4 } from "uuid";
import { pool } from "../db/pool.js";
import {
  createModelProvider,
  createVisualProvider,
  bindRoute,
  RouteBindError,
} from "../services/model/provider-store.js";
import {
  invokeEmbed,
  invokeGeneration,
  invokeRerank,
  CapabilityUnavailableError,
} from "../services/model/router.js";
import { testModelConnectivity } from "../services/model/health.js";
import { parseTick } from "../services/parsing/event-consumer.js";
import { createObjectStorage, objectKeyForVersion } from "../services/object-storage.js";

const PORT = 4733;
const MOCK = `http://127.0.0.1:${PORT}`;
const storage = createObjectStorage();

function ok(cond: boolean, msg: string) {
  if (!cond) throw new Error("断言失败: " + msg);
  console.log("  ✓", msg);
}

async function readBody(req: import("node:http").IncomingMessage): Promise<Record<string, unknown>> {
  const chunks: Buffer[] = [];
  for await (const c of req) chunks.push(c as Buffer);
  if (!chunks.length) return {};
  try {
    return JSON.parse(Buffer.concat(chunks).toString("utf8"));
  } catch {
    return {};
  }
}

function startMock(): Promise<Server> {
  const server = createServer(async (req, res) => {
    const url = req.url ?? "";
    const body = await readBody(req);
    const send = (code: number, obj: unknown) => {
      res.writeHead(code, { "content-type": "application/json" });
      res.end(JSON.stringify(obj));
    };
    if (url.startsWith("/fail")) return send(500, { error: "mock failure" });

    if (url.endsWith("/v1/chat/completions")) {
      return send(200, { choices: [{ message: { content: "[mock-chat] ok" } }] });
    }
    if (url.endsWith("/v1/messages")) {
      return send(200, { content: [{ text: "[mock-anthropic] ok" }] });
    }
    if (url.endsWith("/v1/embeddings")) {
      const input = (body.input as string[]) ?? [];
      return send(200, { model: "mock-embed", data: input.map(() => ({ embedding: [0.1, 0.2, 0.3] })) });
    }
    if (url.endsWith("/v1/rerank")) {
      const docs = (body.documents as string[]) ?? [];
      return send(200, { scores: docs.map((_, i) => 1 - i * 0.1) });
    }
    if (url.endsWith("/invoke")) {
      if (body.capability === "embed") {
        const input = (body.input as string[]) ?? [];
        return send(200, { vectors: input.map(() => [0.4, 0.5, 0.6]) });
      }
      return send(200, { content: "[mock-thirdparty] ok" });
    }
    if (url.endsWith("/parse")) {
      return send(200, {
        confidence: 0.95,
        pages: [
          {
            page: 1,
            confidence: 0.95,
            paragraphs: [
              { text: "扫描件标题段落", heading_level: 1 },
              { text: "扫描件正文第一段，含可溯源页码。" },
            ],
            tables: [{ rows: 2, cols: 2 }],
            images: [{ bbox: [0, 0, 10, 10] }],
          },
        ],
      });
    }
    return send(404, { error: "not found" });
  });
  return new Promise((resolve) => server.listen(PORT, "127.0.0.1", () => resolve(server)));
}

async function cleanup(client: import("pg").PoolClient, tenantId: string) {
  // 删除上轮 smoke 残留（按 name / 文档名前缀）
  const docs = await client.query(
    `SELECT document_id FROM documents WHERE tenant_id = $1 AND name LIKE 'c03-smoke%'`,
    [tenantId],
  );
  for (const d of docs.rows) {
    const id = d.document_id as string;
    await client.query(
      `DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM document_chunks WHERE document_id = $1)`,
      [id],
    );
    await client.query(`DELETE FROM document_chunks WHERE document_id = $1`, [id]);
    await client.query(`DELETE FROM document_visual_parse_results WHERE document_id = $1`, [id]);
    await client.query(
      `DELETE FROM document_event_consumptions WHERE event_id IN (SELECT event_id FROM document_events WHERE document_id = $1)`,
      [id],
    );
    await client.query(`DELETE FROM document_parse_jobs WHERE document_id = $1`, [id]);
    await client.query(`DELETE FROM document_events WHERE document_id = $1`, [id]);
    await client.query(`UPDATE documents SET current_version_id = NULL WHERE document_id = $1`, [id]);
    await client.query(`DELETE FROM document_versions WHERE document_id = $1`, [id]);
    await client.query(`DELETE FROM documents WHERE document_id = $1`, [id]);
  }
  await client.query(
    `DELETE FROM model_routes WHERE tenant_id = $1 AND provider_id IN
       (SELECT provider_id FROM model_providers WHERE name LIKE 'c03-smoke%')`,
    [tenantId],
  );
  await client.query(`DELETE FROM model_providers WHERE tenant_id = $1 AND name LIKE 'c03-smoke%'`, [tenantId]);
  await client.query(`DELETE FROM visual_parse_providers WHERE tenant_id = $1 AND name LIKE 'c03-smoke%'`, [tenantId]);
}

async function makeDocument(
  client: import("pg").PoolClient,
  tenantId: string,
  ownerId: string,
  name: string,
  mime: string,
  content: Buffer,
): Promise<string> {
  const documentId = uuidv4();
  const versionId = uuidv4();
  const objectKey = objectKeyForVersion(tenantId, documentId, versionId);
  await storage.put(objectKey, content, mime);
  await client.query(
    `INSERT INTO documents (document_id, tenant_id, owner_id, name, space, mime_type, current_version_id)
     VALUES ($1,$2,$3,$4,'my',$5,NULL)`,
    [documentId, tenantId, ownerId, name, mime],
  );
  await client.query(
    `INSERT INTO document_versions
       (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
     VALUES ($1,$2,$3,1,$4,$5,'import',$6,$7)`,
    [versionId, documentId, tenantId, uuidv4().replace(/-/g, ""), ownerId, objectKey, content.length],
  );
  await client.query(`UPDATE documents SET current_version_id = $1 WHERE document_id = $2`, [versionId, documentId]);
  await client.query(
    `INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
     VALUES ('upload_success', $1, $2, $3, '{"source":"c03-smoke"}'::jsonb)`,
    [documentId, versionId, tenantId],
  );
  return documentId;
}

async function auditCount(
  client: import("pg").PoolClient,
  tenantId: string,
  actionType: string,
  since: Date,
): Promise<number> {
  const { rows } = await client.query(
    `SELECT COUNT(*)::int AS n FROM audit_logs
     WHERE tenant_id = $1 AND action_type = $2 AND created_at >= $3`,
    [tenantId, actionType, since],
  );
  return rows[0].n as number;
}

async function main() {
  const mock = await startMock();
  const client = await pool.connect();
  const since = new Date();
  try {
    const t = await client.query("SELECT tenant_id FROM tenants ORDER BY created_at LIMIT 1");
    if (!t.rows.length) throw new Error("无租户，请先 npm run migrate");
    const tenantId = t.rows[0].tenant_id as string;
    const u = await client.query("SELECT user_id FROM users WHERE tenant_id = $1 AND username = 'admin'", [tenantId]);
    const adminId = u.rows[0].user_id as string;
    const ctx = { tenantId, actorId: adminId, actorRole: "admin" };

    await cleanup(client, tenantId);
    console.log("\n[1] 私有化 provider 主动连通性测试（5.x）");
    const privChat = await createModelProvider(client, tenantId, {
      name: "c03-smoke 私有chat",
      protocol: "openai_compat",
      deploymentKind: "private",
      baseUrl: `${MOCK}/ok`,
      credential: "sk-mock-private",
      model: "mock-llm",
      networkPolicy: "intranet_only",
      enabled: true,
      defaultPriority: 1,
    });
    const conn = await testModelConnectivity(client, tenantId, privChat, "chat");
    ok(conn.status === "up", `连通性测试成功（latency=${conn.latencyMs}ms）`);
    const failProvId = await createModelProvider(client, tenantId, {
      name: "c03-smoke 不可达",
      protocol: "openai_compat",
      deploymentKind: "private",
      baseUrl: `${MOCK}/fail`,
      model: "x",
      networkPolicy: "intranet_only",
      enabled: true,
      maxRetries: 0,
      defaultPriority: 5,
    });
    const connFail = await testModelConnectivity(client, tenantId, failProvId, "chat");
    ok(connFail.status === "down", "不可达 provider 连通性测试记为失败、不进入可用路由");

    console.log("\n[2] AIMed/翻译私有化路径 + 审计（9.1 / 9.2）");
    await bindRoute(client, tenantId, "chat", privChat, 1);
    await bindRoute(client, tenantId, "translate", privChat, 1);
    const chatRes = await invokeGeneration(client, "chat", { messages: [{ role: "user", content: "hi" }] }, ctx);
    ok(chatRes.content.includes("mock-chat"), "AIMed(chat) 经私有化 provider 调用成功");
    const trRes = await invokeGeneration(client, "translate", { messages: [{ role: "user", content: "hello" }] }, ctx);
    ok(trRes.content.includes("mock-chat"), "医学翻译(translate) 经私有化 provider 调用成功");
    ok((await auditCount(client, tenantId, "model_invoke", since)) >= 2, "model_invoke 成功审计已落库");

    console.log("\n[3] fallback 四要素审计（3.3 / 9.4）");
    // 用一个未做过连通性测试的失败 provider（避免被预先标记 down 而在链中被跳过），
    // 使其在调用时真实失败 → 触发 invoke 期 fallback 审计
    const fbFailId = await createModelProvider(client, tenantId, {
      name: "c03-smoke fallback失败源",
      protocol: "openai_compat",
      deploymentKind: "private",
      baseUrl: `${MOCK}/fail`,
      model: "x",
      networkPolicy: "intranet_only",
      enabled: true,
      maxRetries: 0,
      defaultPriority: 5,
    });
    await bindRoute(client, tenantId, "summarize", fbFailId, 1); // 优先级1：调用失败
    await bindRoute(client, tenantId, "summarize", privChat, 2); // 优先级2：私有化兜底
    const sumRes = await invokeGeneration(client, "summarize", { messages: [{ role: "user", content: "long" }] }, ctx);
    ok(sumRes.content.includes("mock-chat"), "高优先级失败后自动 fallback 到下一 provider 成功");
    const fb = await client.query(
      `SELECT metadata, failure_reason, created_at FROM audit_logs
       WHERE tenant_id = $1 AND action_type = 'model_fallback' AND created_at >= $2
       ORDER BY created_at DESC LIMIT 1`,
      [tenantId, since],
    );
    ok(fb.rows.length > 0, "fallback 审计记录存在");
    const meta = fb.rows[0].metadata as Record<string, unknown>;
    ok(
      Boolean(meta.fromProvider) && // provider
        Boolean(fb.rows[0].failure_reason) && // 失败原因（audit_logs 列）
        meta.toProvider !== undefined && // 切换目标
        Boolean(fb.rows[0].created_at), // 时间戳
      "fallback 四要素齐全（provider / 失败原因 / 切换目标 / 时间戳 created_at）",
    );

    console.log("\n[4] 用途未绑定时拒绝调用（3.2）");
    let rejected = false;
    try {
      await invokeRerank(client, { query: "q", documents: ["a"] }, ctx);
    } catch (e) {
      rejected = e instanceof CapabilityUnavailableError;
    }
    ok(rejected, "Rerank 未配置可用 provider 时返回明确不可用错误");

    console.log("\n[5] 公网默认拒绝并切私有化（D6 本期保守降级）");
    const pubProof = await createModelProvider(client, tenantId, {
      name: "c03-smoke 公网proof",
      protocol: "openai_compat",
      deploymentKind: "public",
      baseUrl: `${MOCK}/ok`,
      model: "pub",
      enabled: true,
    });
    await bindRoute(client, tenantId, "proofread", pubProof, 1);
    await bindRoute(client, tenantId, "proofread", privChat, 2);
    const proofRes = await invokeGeneration(client, "proofread", { messages: [{ role: "user", content: "x" }] }, ctx);
    ok(proofRes.content.includes("mock-chat"), "公网被脱敏门禁跳过、改走私有化成功");
    ok((await auditCount(client, tenantId, "model_redaction_block", since)) >= 1, "公网默认拒绝落 model_redaction_block 审计");

    console.log("\n[6] Anthropic 协议不可绑定 Embedding/Rerank（2.3）");
    const anthProv = await createModelProvider(client, tenantId, {
      name: "c03-smoke anthropic",
      protocol: "anthropic_messages",
      deploymentKind: "private",
      baseUrl: `${MOCK}/ok`,
      model: "claude-x",
      networkPolicy: "intranet_only",
      enabled: true,
    });
    let bindRejected = false;
    try {
      await bindRoute(client, tenantId, "embed", anthProv, 1);
    } catch (e) {
      bindRejected = e instanceof RouteBindError;
    }
    ok(bindRejected, "Anthropic 绑定 Embedding 被配置层拒绝");

    console.log("\n[7] 禁用公网时主闭环经私有化/离线完成：文本 + 扫描两条解析链路（9.3 / 7.x）");
    const privEmbed = await createModelProvider(client, tenantId, {
      name: "c03-smoke 私有embed",
      protocol: "openai_compat",
      deploymentKind: "private",
      baseUrl: `${MOCK}/ok`,
      model: "mock-embed",
      networkPolicy: "intranet_only",
      enabled: true,
    });
    await bindRoute(client, tenantId, "embed", privEmbed, 1);
    const privVisual = await createVisualProvider(client, tenantId, {
      name: "c03-smoke 私有视觉",
      backendKind: "private_service",
      deploymentKind: "private",
      baseUrl: `${MOCK}/ok`,
      networkPolicy: "intranet_only",
      enabled: true,
    });

    const textDoc = await makeDocument(
      client,
      tenantId,
      adminId,
      "c03-smoke-note.md",
      "text/markdown",
      Buffer.from("# 标题一\n\n第一段正文。\n\n第二段正文，用于切分。", "utf8"),
    );
    const scanDoc = await makeDocument(
      client,
      tenantId,
      adminId,
      "c03-smoke-scan.png",
      "image/png",
      Buffer.from("fake-png-bytes-for-poc"),
    );
    // 释放 client 占用，避免 worker 与本 client 抢占；parseTick 用独立连接
    const tick = await parseTick();
    ok(tick.dispatched >= 2, `事件消费创建作业（dispatched=${tick.dispatched}）`);

    const textJob = await client.query(
      `SELECT status, index_ready_at FROM document_parse_jobs WHERE document_id = $1`,
      [textDoc],
    );
    ok(textJob.rows[0]?.status === "succeeded", "文本型文档解析作业 succeeded");
    ok(textJob.rows[0]?.index_ready_at !== null, "文本作业发出『索引就绪』(index_ready_at) ");
    const textChunks = await client.query(
      `SELECT c.id, c.chunk_acl, e.chunk_id, e.dim FROM document_chunks c
       JOIN embeddings e ON e.chunk_id = c.id
       WHERE c.document_id = $1 AND c.superseded = FALSE`,
      [textDoc],
    );
    ok(textChunks.rows.length >= 2, "文本切分写入 document_chunks 且 embeddings 经 chunk_id 外键回连");
    ok(textChunks.rows.every((r) => r.chunk_acl !== null), "每个 chunk 携带 chunk_acl（默认继承文档级）");

    const scanJob = await client.query(`SELECT status FROM document_parse_jobs WHERE document_id = $1`, [scanDoc]);
    ok(scanJob.rows[0]?.status === "succeeded", "扫描件经私有化视觉解析 succeeded");
    const visRes = await client.query(
      `SELECT confidence FROM document_visual_parse_results WHERE document_id = $1 AND failure_reason IS NULL`,
      [scanDoc],
    );
    ok(visRes.rows.length >= 1, "扫描件结构化结果写入 document_visual_parse_results");

    console.log("\n[8] 1.5a：parse-status 查询无 42703 且返回真实状态");
    const ps = await client.query(
      `SELECT j.job_id, j.status, j.substatus, j.document_version, r.confidence
       FROM document_parse_jobs j
       LEFT JOIN document_visual_parse_results r
         ON r.document_id = j.document_id AND r.document_version = j.document_version
       WHERE j.document_id = $1 AND j.tenant_id = $2
       ORDER BY j.created_at DESC LIMIT 1`,
      [textDoc, tenantId],
    );
    ok(ps.rows[0]?.status === "succeeded", "parse-status SELECT 返回真实状态 succeeded（非 pending、无列错误）");

    console.log("\n[9] 结构校验：embeddings 无 tenant_id 列 / document_chunks 有 chunk_acl 列（1.7）");
    const embCols = await client.query(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'embeddings'`,
    );
    ok(!embCols.rows.some((r) => r.column_name === "tenant_id"), "embeddings 表无独立 tenant_id 列");
    const chunkCols = await client.query(
      `SELECT column_name FROM information_schema.columns WHERE table_name = 'document_chunks'`,
    );
    ok(chunkCols.rows.some((r) => r.column_name === "chunk_acl"), "document_chunks 含 chunk_acl 物理列");

    await cleanup(client, tenantId);
    console.log("\n✅ c03 冒烟全部通过");
  } finally {
    client.release();
    mock.close();
    await pool.end();
  }
}

main().catch((err) => {
  console.error("\n❌ c03 冒烟失败:", err.message);
  process.exit(1);
});
