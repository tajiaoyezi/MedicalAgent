import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "../../lib/api";
import {
  createBridge,
  loadOnlyofficeApi,
  type MedOfficeBridge,
} from "../../lib/bridge-sdk";
import AIPanel from "./AIPanel";

interface OpenResponse {
  mode: string;
  documentId: string;
  permission: string;
  dsUrl: string;
  editorConfig: Record<string, unknown>;
  bridgeToken: string;
  revision: string;
}

export default function EditorPage() {
  const { documentId } = useParams<{ documentId: string }>();
  const navigate = useNavigate();
  const editorRef = useRef<HTMLDivElement>(null);
  const bridgeRef = useRef<MedOfficeBridge | null>(null);
  const editorInstanceRef = useRef<{ destroyEditor?: () => void } | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [permission, setPermission] = useState("");
  const [aiOpen, setAiOpen] = useState(false);
  const [aiFocus, setAiFocus] = useState<"document" | "selection">("document");
  const [docType, setDocType] = useState("docx");

  useEffect(() => {
    if (!documentId) return;

    let cancelled = false;

    const onReady = (event: MessageEvent) => {
      if (event.data?.channel === "medoffice-bridge-ready") {
        // 关键修复：目标须是**插件 window**（嵌套 sandbox iframe，宿主无法 querySelector），
        // 取就绪包的 event.source（即插件 window）作为命令投递目标；DS 编辑器 iframe 不是插件、投不进。
        if (event.source) {
          bridgeRef.current?.setTarget(event.source as Window);
          if (import.meta.env.DEV) {
            (window as unknown as { __medbridgeReady?: boolean }).__medbridgeReady = true;
          }
        }
      }
    };

    const onAiPanel = () => {
      setAiFocus("document");
      setAiOpen(true);
    };

    window.addEventListener("message", onReady);
    window.addEventListener("medoffice:open-ai-panel", onAiPanel);

    async function boot() {
      try {
        const res = await api<OpenResponse>(`/api/editor/open/${documentId}`);
        if (cancelled) return;

        if (res.mode === "preview") {
          navigate(`/preview/${documentId}`, { replace: true });
          return;
        }

        setPermission(res.permission);
        await loadOnlyofficeApi(res.dsUrl);
        if (cancelled) return;

        const bridge = createBridge(res.bridgeToken, res.revision);
        bridgeRef.current = bridge;
        // DEV-only：暴露 bridge 供 e2e（12-onlyoffice-live）驱动真实写回（headless 无法在 DS 画布内做选区）。生产构建剥离。
        if (import.meta.env.DEV) {
          (window as unknown as { __medbridge?: MedOfficeBridge }).__medbridge = bridge;
        }

        if (editorRef.current && window.DocsAPI) {
          const { bridgeToken: _bt, revision: _rev, mimeType: _mime, ...ooConfig } =
            res.editorConfig;
          editorInstanceRef.current = new window.DocsAPI.DocEditor(
            editorRef.current.id,
            {
              ...ooConfig,
              events: {
                onDocumentReady: () => {
                  // 桥目标由插件就绪包的 event.source 设定（见 onReady），此处不再用 DS 编辑器 iframe（投不进嵌套插件）。
                  // 文档打开后默认展示医疗 AI 面板（§5.4/§14.6/§14.8，触发唯一 owner=c05）。
                  setAiFocus("document");
                  setAiOpen(true);
                  bridge
                    .getDocumentType()
                    .then((r) => setDocType(r.data?.type ?? "docx"))
                    .catch(() => setDocType("docx"));
                },
              },
            },
          );
        }

        setLoading(false);
      } catch (e) {
        if (!cancelled) {
          setError(e instanceof Error ? e.message : "打开失败");
          setLoading(false);
        }
      }
    }

    boot();

    return () => {
      cancelled = true;
      window.removeEventListener("message", onReady);
      window.removeEventListener("medoffice:open-ai-panel", onAiPanel);
      editorInstanceRef.current?.destroyEditor?.();
      editorInstanceRef.current = null;
    };
  }, [documentId, navigate]);

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh" }}>
      <header
        style={{
          display: "flex",
          alignItems: "center",
          gap: 12,
          padding: "10px 16px",
          borderBottom: "1px solid var(--color-border)",
          background: "var(--color-surface)",
        }}
      >
        <Link to="/documents" className="btn btn-sm btn-ghost">
          ← 文档中心
        </Link>
        <span style={{ fontSize: 13, color: "var(--color-text-2)" }}>
          在线编辑 · 权限 {permission}
        </span>
        <div style={{ flex: 1 }} />
        {/* 顶部自定义按钮「医疗空间」：打开面板并聚焦文档级 AI 功能区 */}
        <button
          className="btn btn-sm btn-primary"
          onClick={() => {
            setAiFocus("document");
            setAiOpen(true);
          }}
        >
          医疗空间
        </button>
      </header>

      <div style={{ flex: 1, display: "flex", minHeight: 0 }}>
        <div style={{ flex: 1, position: "relative" }}>
          {loading && (
            <div style={{ padding: 48, textAlign: "center" }}>编辑器加载中…</div>
          )}
          {error && (
            <div style={{ padding: 48, textAlign: "center", color: "var(--color-danger)" }}>
              {error}
            </div>
          )}
          <div
            id="onlyoffice-editor"
            ref={editorRef}
            style={{ width: "100%", height: "100%" }}
          />

          {/* 选区浮层入口（润色 / 翻译 / 解释 / 补引用）：选中文本后由此发起，读类就地、写类经确认网关 */}
          <div
            style={{
              position: "absolute",
              bottom: 16,
              left: 16,
              display: "flex",
              gap: 6,
              background: "var(--color-surface)",
              border: "1px solid var(--color-border)",
              borderRadius: 8,
              padding: 6,
              boxShadow: "var(--shadow-sm)",
            }}
          >
            <span style={{ fontSize: 11, color: "var(--color-text-3)", alignSelf: "center" }}>选区浮层</span>
            {["润色", "翻译", "解释", "补引用"].map((a) => (
              <button
                key={a}
                className="btn btn-sm btn-ghost"
                onClick={() => {
                  setAiFocus("selection");
                  setAiOpen(true);
                }}
              >
                {a}
              </button>
            ))}
          </div>
        </div>

        {/* 右侧固定图标「医疗 AI」入口 */}
        {!aiOpen && (
          <button
            title="医疗 AI"
            onClick={() => {
              setAiFocus("document");
              setAiOpen(true);
            }}
            style={{
              width: 44,
              borderLeft: "1px solid var(--color-border)",
              background: "var(--color-surface)",
              cursor: "pointer",
              fontSize: 18,
            }}
          >
            🩺
          </button>
        )}
        {aiOpen && (
          <AIPanel
            bridge={bridgeRef.current}
            docType={docType}
            initialFocus={aiFocus}
            onClose={() => {
              // 经 c02 面板控制类收起，且不改动文档内容（§9.3 关闭面板）
              void bridgeRef.current?.closeAIPanel();
              setAiOpen(false);
            }}
          />
        )}
      </div>
    </div>
  );
}
