// c03｜9 类模型能力（capability）统一内部接口定义（design D1）
// 上层仅依赖这些 capability 类型，不感知底层协议/厂商 SDK。

export type Capability =
  | "chat"
  | "summarize"
  | "translate"
  | "embed"
  | "rerank"
  | "visual_parse"
  | "term_extract"
  | "proofread"
  | "outline_gen";

/** 生成类能力（Anthropic Messages 协议仅可绑定这些） */
export const GENERATION_CAPABILITIES: Capability[] = [
  "chat",
  "summarize",
  "translate",
  "term_extract",
  "proofread",
  "outline_gen",
];

/** model_routes 可绑定的 8 类能力（visual_parse 单独配置于 visual_parse_providers） */
export const ROUTABLE_CAPABILITIES: Capability[] = [
  "chat",
  "summarize",
  "translate",
  "embed",
  "rerank",
  "term_extract",
  "proofread",
  "outline_gen",
];

/** 主验收闭环涉及能力：每条必须至少有一条私有化/离线路径（§24.9，task 8.4） */
export const MAIN_LOOP_CAPABILITIES: Capability[] = [
  "chat",
  "translate",
  "embed",
  "visual_parse",
];

export function isGenerationCapability(cap: Capability): boolean {
  return GENERATION_CAPABILITIES.includes(cap);
}

// ——— 各 capability 的请求/响应契约（POC 取最小可用集合） ———

export interface ChatMessage {
  role: "system" | "user" | "assistant";
  content: string;
}

/** chat/summarize/translate/term_extract/proofread/outline_gen 共用生成类入参 */
export interface GenerationRequest {
  messages: ChatMessage[];
  /** 翻译/术语等可携带的附加提示，POC 透传 */
  hint?: string;
}

export interface GenerationResponse {
  content: string;
  model: string;
}

export interface EmbedRequest {
  input: string[];
}

export interface EmbedResponse {
  vectors: number[][];
  model: string;
  dim: number;
}

export interface RerankRequest {
  query: string;
  documents: string[];
}

export interface RerankResponse {
  /** 与 documents 同序的相关性分数 */
  scores: number[];
  model: string;
}

/** capability → 入参类型映射（router 入口用 unknown，由 adapter 内部按 capability 收敛） */
export type CapabilityRequest =
  | GenerationRequest
  | EmbedRequest
  | RerankRequest;
