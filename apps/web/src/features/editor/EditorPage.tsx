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
  const [aiCommand, setAiCommand] = useState("");

  useEffect(() => {
    if (!documentId) return;

    let cancelled = false;

    const onReady = (event: MessageEvent) => {
      if (event.data?.channel === "medoffice-bridge-ready") {
        const iframe = editorRef.current?.querySelector("iframe");
        if (iframe?.contentWindow) {
          bridgeRef.current?.setTarget(iframe.contentWindow);
        }
      }
    };

    const onAiPanel = (event: Event) => {
      const detail = (event as CustomEvent).detail as { command?: string };
      setAiCommand(detail?.command ?? "");
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

        if (editorRef.current && window.DocsAPI) {
          const { bridgeToken: _bt, revision: _rev, mimeType: _mime, ...ooConfig } =
            res.editorConfig;
          editorInstanceRef.current = new window.DocsAPI.DocEditor(
            editorRef.current.id,
            {
              ...ooConfig,
              events: {
                onDocumentReady: () => {
                  const iframe = editorRef.current?.querySelector("iframe");
                  if (iframe?.contentWindow) {
                    bridge.setTarget(iframe.contentWindow);
                  }
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
        <button
          className="btn btn-sm btn-primary"
          onClick={() => {
            setAiOpen(true);
            setAiCommand("polish");
          }}
        >
          医疗 AI 面板
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
        </div>
        {aiOpen && (
          <AIPanel
            bridge={bridgeRef.current}
            initialCommand={aiCommand}
            onClose={() => setAiOpen(false)}
          />
        )}
      </div>
    </div>
  );
}
