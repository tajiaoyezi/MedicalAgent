// c03｜provider 出网网关 + HTTP 客户端（design D4 network_policy；D2 错误分类）
// 私有化 provider 标记 deny_egress / intranet_only 时，出站请求经此网关在传输层拦截公网域名。

export type ErrorClass =
  | "timeout"
  | "http_5xx"
  | "rate_limit"
  | "health_down"
  | "missing_key"
  | "network_blocked"
  | "auth_error"
  | "input_too_large"
  | "content_safety"
  | "unknown";

/** 可 fallback（切换下一 provider）的错误类别；其余直接上抛终止链路（D2） */
export const FALLBACKABLE_ERRORS: ErrorClass[] = [
  "timeout",
  "http_5xx",
  "rate_limit",
  "health_down",
  "missing_key",
  "network_blocked",
];

export function isFallbackable(cls: ErrorClass): boolean {
  return FALLBACKABLE_ERRORS.includes(cls);
}

export class ProviderError extends Error {
  errorClass: ErrorClass;
  constructor(errorClass: ErrorClass, message: string) {
    super(message);
    this.name = "ProviderError";
    this.errorClass = errorClass;
  }
}

export type DeploymentKind = "public" | "private";
export type NetworkPolicy = "allow_all" | "intranet_only" | "deny_egress" | null;

export interface ProviderConnection {
  providerId: string;
  kind: "model" | "visual";
  name: string;
  protocol?: string;
  backendKind?: string;
  deploymentKind: DeploymentKind;
  baseUrl: string;
  /** 已解密凭据（仅用于实际外发，绝不回前端） */
  credential: string;
  model: string;
  timeoutMs: number;
  maxRetries: number;
  networkPolicy: NetworkPolicy;
}

const PRIVATE_HOST_RE =
  /^(localhost|127\.|10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.|169\.254\.|host\.docker\.internal$|.*\.local$|.*\.internal$|.*\.svc$|.*\.svc\.cluster\.local$)/i;

export function isPrivateHost(hostname: string): boolean {
  const h = hostname.toLowerCase();
  if (h === "host.docker.internal") return true;
  return PRIVATE_HOST_RE.test(h);
}

/** D4：对私有化 provider 强制执行 network_policy；命中「禁止出网」即便误配公网域名也拦截。 */
export function enforceNetworkPolicy(conn: ProviderConnection, url: URL): void {
  if (conn.deploymentKind !== "private") return;
  const policy = conn.networkPolicy ?? "intranet_only";
  if (policy === "allow_all") return;
  // deny_egress / intranet_only：仅允许内网/回环目标
  if (!isPrivateHost(url.hostname)) {
    throw new ProviderError(
      "network_blocked",
      `私有化 provider「${conn.name}」network_policy=${policy} 禁止出网，但目标 ${url.hostname} 为公网域名，出网网关已拦截`,
    );
  }
}

/** 统一出站请求：网络策略校验 + 超时 + 状态码→错误类别映射。不含重试（重试在 router 内按 provider 策略执行）。 */
export async function providerFetch(
  conn: ProviderConnection,
  path: string,
  body: unknown,
  headers: Record<string, string> = {},
): Promise<unknown> {
  let url: URL;
  try {
    url = new URL(path, conn.baseUrl.endsWith("/") ? conn.baseUrl : `${conn.baseUrl}/`);
  } catch {
    throw new ProviderError("unknown", `provider「${conn.name}」base_url 非法：${conn.baseUrl}`);
  }
  enforceNetworkPolicy(conn, url);

  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), conn.timeoutMs);
  let res: Response;
  try {
    res = await fetch(url, {
      method: "POST",
      headers: { "content-type": "application/json", ...headers },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
  } catch (e) {
    if ((e as Error).name === "AbortError") {
      throw new ProviderError("timeout", `provider「${conn.name}」请求超时（${conn.timeoutMs}ms）`);
    }
    // 连接错误（DNS/拒绝连接）按可 fallback 处理
    throw new ProviderError("http_5xx", `provider「${conn.name}」连接失败：${(e as Error).message}`);
  } finally {
    clearTimeout(timer);
  }

  if (res.status === 401 || res.status === 403) {
    throw new ProviderError("auth_error", `provider「${conn.name}」鉴权失败（HTTP ${res.status}）`);
  }
  if (res.status === 429) {
    throw new ProviderError("rate_limit", `provider「${conn.name}」被限流（HTTP 429）`);
  }
  if (res.status >= 500) {
    throw new ProviderError("http_5xx", `provider「${conn.name}」服务端错误（HTTP ${res.status}）`);
  }
  if (res.status >= 400) {
    const text = await res.text().catch(() => "");
    throw new ProviderError("unknown", `provider「${conn.name}」请求错误（HTTP ${res.status}）${text.slice(0, 200)}`);
  }
  return res.json();
}
