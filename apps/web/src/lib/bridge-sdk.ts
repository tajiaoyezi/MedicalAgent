const BASE_ALLOWED_ORIGINS = new Set([
  "http://localhost:5173",
  "http://127.0.0.1:5173",
  // 桥插件由 ONLYOFFICE DS 经 host.docker.internal:5173 加载，其回包 event.origin 为该值（dev 真实 DS）。
  "http://host.docker.internal:5173",
]);

const extraOrigins = new Set<string>();

export function registerBridgeOrigins(...origins: string[]) {
  for (const o of origins) {
    if (o) extraOrigins.add(o);
  }
}

function isAllowedOrigin(origin: string): boolean {
  if (BASE_ALLOWED_ORIGINS.has(origin)) return true;
  if (extraOrigins.has(origin)) return true;
  if (typeof window !== "undefined" && origin === window.location.origin) {
    return true;
  }
  return false;
}

let seq = 0;

function nextId() {
  seq += 1;
  return `bridge-${seq}-${Date.now()}`;
}

export interface BridgeResponse<T = unknown> {
  ok: boolean;
  data?: T;
  docKey?: string;
  revision?: string;
  error?: string;
}

interface AuthorizeResult {
  permitted?: boolean;
  revision?: string;
  saveIntentId?: string;
}

export class MedOfficeBridge {
  private bridgeToken: string;
  private revision: string;
  private iframe: Window | null = null;

  constructor(bridgeToken: string, revision: string) {
    this.bridgeToken = bridgeToken;
    this.revision = revision;
  }

  setTarget(win: Window | null) {
    this.iframe = win;
  }

  updateRevision(revision: string) {
    this.revision = revision;
  }

  getRevision() {
    return this.revision;
  }

  getBridgeToken() {
    return this.bridgeToken;
  }

  private async authorize(
    method: string,
    extra: Record<string, unknown> = {},
  ): Promise<AuthorizeResult> {
    const res = await fetch("/api/bridge/authorize", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        bridgeToken: this.bridgeToken,
        method,
        expectedRevision: this.revision,
        ...extra,
      }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error((err as { error?: string }).error ?? "Bridge 授权失败");
    }
    return res.json() as Promise<AuthorizeResult>;
  }

  private async armWritebackSave(saveIntentId: string) {
    const res = await fetch("/api/bridge/arm-writeback-save", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        bridgeToken: this.bridgeToken,
        saveIntentId,
      }),
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error((err as { error?: string }).error ?? "写回保存意图激活失败");
    }
  }

  private callPlugin<T>(
    method: string,
    params?: Record<string, unknown>,
  ): Promise<BridgeResponse<T>> {
    return new Promise((resolve, reject) => {
      if (!this.iframe) {
        reject(new Error("编辑器未就绪"));
        return;
      }
      const requestId = nextId();
      const handler = (event: MessageEvent) => {
        if (!isAllowedOrigin(event.origin)) return;
        const data = event.data;
        if (
          !data ||
          data.channel !== "medoffice-bridge" ||
          data.requestId !== requestId
        ) {
          return;
        }
        window.removeEventListener("message", handler);
        if (data.ok) {
          if (data.revision) this.revision = data.revision;
          resolve(data as BridgeResponse<T>);
        } else {
          reject(new Error(data.error ?? "Bridge 调用失败"));
        }
      };
      window.addEventListener("message", handler);
      this.iframe.postMessage(
        {
          channel: "medoffice-bridge-host",
          requestId,
          method,
          params,
          revision: this.revision,
        },
        "*",
      );
      setTimeout(() => {
        window.removeEventListener("message", handler);
        reject(new Error("Bridge 调用超时"));
      }, 15000);
    });
  }

  async invoke<T>(method: string, params?: Record<string, unknown>) {
    const extra: Record<string, unknown> = {};
    if (method === "saveDocument" && params?.writebackSource) {
      extra.writebackSource = params.writebackSource;
    }
    const auth = await this.authorize(method, extra);
    if (auth.revision) {
      this.revision = auth.revision;
    }
    return { auth, result: await this.callPlugin<T>(method, params) };
  }

  async getSelectedText() {
    const { result } = await this.invoke<{ text: string; range: unknown; page: number }>(
      "getSelectedText",
    );
    return result;
  }

  async getFullText() {
    const { result } = await this.invoke<{ text: string; outline: unknown[] }>(
      "getFullText",
    );
    return result;
  }

  async replaceSelection(text: string, originalText?: string) {
    const { result } = await this.invoke("replaceSelection", {
      text,
      originalText,
    });
    return result;
  }

  async saveDocument(writebackSource?: string) {
    const source = writebackSource ?? "ai_writeback";
    const { auth, result } = await this.invoke<{ saveTriggered?: boolean }>(
      "saveDocument",
      { writebackSource: source },
    );
    if (auth.saveIntentId) {
      await this.armWritebackSave(auth.saveIntentId);
    }
    return result;
  }

  async getDocumentType() {
    const { result } = await this.invoke<{ type: string }>("getDocumentType");
    return result;
  }

  async getDocumentId() {
    const { result } = await this.invoke<{ documentId: string }>("getDocumentId");
    return result;
  }

  async getDocumentTitle() {
    const { result } = await this.invoke<{ title: string }>("getDocumentTitle");
    return result;
  }

  async insertComment(range: unknown, comment: string) {
    const { result } = await this.invoke("insertComment", { range, comment });
    return result;
  }

  async insertCitation(position: unknown, citation: unknown) {
    const { result } = await this.invoke("insertCitation", { position, citation });
    return result;
  }

  async createNewDocument(content: string, templateId?: string) {
    const { result } = await this.invoke<{ documentId?: string }>(
      "createNewDocument",
      { content, templateId },
    );
    return result;
  }

  async applyStyle(style: Record<string, unknown>) {
    const { result } = await this.invoke("applyStyle", style);
    return result;
  }

  async openAIPanel(command: string, payload?: Record<string, unknown>) {
    const auth = await this.authorize("openAIPanel");
    if (auth.revision) this.revision = auth.revision;
    window.dispatchEvent(
      new CustomEvent("medoffice:open-ai-panel", {
        detail: { command, payload },
      }),
    );
  }

  async closeAIPanel() {
    try {
      await this.authorize("closeAIPanel");
    } catch {
      // 关闭面板属纯收起，授权失败不应阻断 UI 收起
    }
  }

  async getConfirmPreview(originalText: string, modifiedText: string) {
    const res = await fetch("/api/bridge/confirm-preview", {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        originalText,
        modifiedText,
        impactScope: "selection",
      }),
    });
    return res.json();
  }
}

export function createBridge(bridgeToken: string, revision: string) {
  return new MedOfficeBridge(bridgeToken, revision);
}

declare global {
  interface Window {
    DocsAPI?: {
      DocEditor: new (
        id: string,
        config: Record<string, unknown>,
      ) => { destroyEditor?: () => void };
    };
  }
}

export function loadOnlyofficeApi(dsUrl: string): Promise<void> {
  try {
    registerBridgeOrigins(new URL(dsUrl).origin);
  } catch {
    // ignore invalid ds url
  }
  return new Promise((resolve, reject) => {
    if (window.DocsAPI) {
      resolve();
      return;
    }
    const script = document.createElement("script");
    script.src = `${dsUrl}/web-apps/apps/api/documents/api.js`;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("无法加载 ONLYOFFICE API"));
    document.head.appendChild(script);
  });
}
