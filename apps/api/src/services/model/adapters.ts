// c03｜四类协议 Adapter（design D1）：协议适配到统一 capability 接口。
// openai_compat / local_gateway → OpenAI 兼容路径；anthropic_messages → /v1/messages（仅生成类）；
// third_party → 通用 /invoke 信封。上层只面向 capability，不感知此处差异。
import type {
  Capability,
  EmbedRequest,
  EmbedResponse,
  GenerationRequest,
  GenerationResponse,
  RerankRequest,
  RerankResponse,
} from "./capabilities.js";
import { GENERATION_CAPABILITIES } from "./capabilities.js";
import { ProviderError, providerFetch, type ProviderConnection } from "./http.js";

export interface ModelAdapter {
  protocol: string;
  supports(cap: Capability): boolean;
  generate(req: GenerationRequest, conn: ProviderConnection): Promise<GenerationResponse>;
  embed(req: EmbedRequest, conn: ProviderConnection): Promise<EmbedResponse>;
  rerank(req: RerankRequest, conn: ProviderConnection): Promise<RerankResponse>;
}

function authHeaders(conn: ProviderConnection): Record<string, string> {
  return conn.credential ? { authorization: `Bearer ${conn.credential}` } : {};
}

function notSupported(protocol: string, cap: Capability): never {
  throw new ProviderError(
    "unknown",
    `${protocol} 协议不支持 ${cap} 能力，请为该用途单独配置 provider`,
  );
}

// ——— OpenAI 兼容（公网 OpenAI-compatible 及多数本地网关 vLLM/Ollama/Xinference） ———
const openAICompatBase: Omit<ModelAdapter, "protocol"> = {
  supports() {
    return true; // 生成类 + embed + rerank 均支持（visual_parse 由 visual_parse_providers 承担）
  },
  async generate(req, conn) {
    const json = (await providerFetch(
      conn,
      "v1/chat/completions",
      { model: conn.model, messages: req.messages },
      authHeaders(conn),
    )) as { choices?: Array<{ message?: { content?: string } }> };
    const content = json.choices?.[0]?.message?.content;
    if (typeof content !== "string") {
      throw new ProviderError("unknown", `provider「${conn.name}」返回缺少 choices[].message.content`);
    }
    return { content, model: conn.model };
  },
  async embed(req, conn) {
    const json = (await providerFetch(
      conn,
      "v1/embeddings",
      { model: conn.model, input: req.input },
      authHeaders(conn),
    )) as { data?: Array<{ embedding?: number[] }> };
    const vectors = (json.data ?? []).map((d) => d.embedding ?? []);
    if (!vectors.length || !vectors[0].length) {
      throw new ProviderError("unknown", `provider「${conn.name}」返回缺少 embedding 向量`);
    }
    return { vectors, model: conn.model, dim: vectors[0].length };
  },
  async rerank(req, conn) {
    const json = (await providerFetch(
      conn,
      "v1/rerank",
      { model: conn.model, query: req.query, documents: req.documents },
      authHeaders(conn),
    )) as { results?: Array<{ index: number; relevance_score: number }>; scores?: number[] };
    let scores: number[];
    if (Array.isArray(json.scores)) {
      scores = json.scores;
    } else if (Array.isArray(json.results)) {
      scores = new Array(req.documents.length).fill(0);
      for (const r of json.results) scores[r.index] = r.relevance_score;
    } else {
      throw new ProviderError("unknown", `provider「${conn.name}」rerank 返回缺少 scores/results`);
    }
    return { scores, model: conn.model };
  },
};

const openAICompatAdapter: ModelAdapter = { protocol: "openai_compat", ...openAICompatBase };
const localGatewayAdapter: ModelAdapter = { protocol: "local_gateway", ...openAICompatBase };

// ——— Anthropic Messages：仅生成类（embed/rerank/视觉解析须单独配置 provider） ———
const anthropicMessagesAdapter: ModelAdapter = {
  protocol: "anthropic_messages",
  supports(cap) {
    return GENERATION_CAPABILITIES.includes(cap);
  },
  async generate(req, conn) {
    const system = req.messages
      .filter((m) => m.role === "system")
      .map((m) => m.content)
      .join("\n");
    const messages = req.messages
      .filter((m) => m.role !== "system")
      .map((m) => ({ role: m.role, content: m.content }));
    const headers: Record<string, string> = { "anthropic-version": "2023-06-01" };
    if (conn.credential) headers["x-api-key"] = conn.credential;
    const json = (await providerFetch(
      conn,
      "v1/messages",
      { model: conn.model, max_tokens: 1024, system: system || undefined, messages },
      headers,
    )) as { content?: Array<{ text?: string }> };
    const content = json.content?.map((c) => c.text ?? "").join("");
    if (typeof content !== "string" || !content) {
      throw new ProviderError("unknown", `provider「${conn.name}」返回缺少 content[].text`);
    }
    return { content, model: conn.model };
  },
  embed() {
    return notSupported("anthropic_messages", "embed");
  },
  rerank() {
    return notSupported("anthropic_messages", "rerank");
  },
};

// ——— 第三方模型服务：通用 /invoke 信封（capability 透传，由第三方服务自适配） ———
const thirdPartyAdapter: ModelAdapter = {
  protocol: "third_party",
  supports() {
    return true;
  },
  async generate(req, conn) {
    const json = (await providerFetch(
      conn,
      "invoke",
      { capability: "generate", model: conn.model, messages: req.messages },
      authHeaders(conn),
    )) as { content?: string };
    if (typeof json.content !== "string") {
      throw new ProviderError("unknown", `provider「${conn.name}」/invoke 返回缺少 content`);
    }
    return { content: json.content, model: conn.model };
  },
  async embed(req, conn) {
    const json = (await providerFetch(
      conn,
      "invoke",
      { capability: "embed", model: conn.model, input: req.input },
      authHeaders(conn),
    )) as { vectors?: number[][] };
    const vectors = json.vectors ?? [];
    if (!vectors.length || !vectors[0]?.length) {
      throw new ProviderError("unknown", `provider「${conn.name}」/invoke 返回缺少 vectors`);
    }
    return { vectors, model: conn.model, dim: vectors[0].length };
  },
  async rerank(req, conn) {
    const json = (await providerFetch(
      conn,
      "invoke",
      { capability: "rerank", model: conn.model, query: req.query, documents: req.documents },
      authHeaders(conn),
    )) as { scores?: number[] };
    if (!Array.isArray(json.scores)) {
      throw new ProviderError("unknown", `provider「${conn.name}」/invoke 返回缺少 scores`);
    }
    return { scores: json.scores, model: conn.model };
  },
};

const ADAPTERS: Record<string, ModelAdapter> = {
  openai_compat: openAICompatAdapter,
  local_gateway: localGatewayAdapter,
  anthropic_messages: anthropicMessagesAdapter,
  third_party: thirdPartyAdapter,
};

export function getAdapter(protocol: string): ModelAdapter {
  const a = ADAPTERS[protocol];
  if (!a) throw new ProviderError("unknown", `未知协议：${protocol}`);
  return a;
}

/** 配置层校验：protocol 是否可绑定该 capability（task 2.3 Anthropic 限制） */
export function protocolSupportsCapability(protocol: string, cap: Capability): boolean {
  return getAdapter(protocol).supports(cap);
}
