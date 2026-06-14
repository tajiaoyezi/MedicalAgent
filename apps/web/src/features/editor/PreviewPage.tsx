import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api } from "../../lib/api";
import { loadOnlyofficeApi } from "../../lib/bridge-sdk";
import { Button, Tag } from "../../components";

interface PreviewData {
  previewType: string;
  label?: string;
  readOnly?: boolean;
  url?: string;
  dsUrl?: string;
  aiEntries?: string[];
  currentPage?: number;
  visualParse?: boolean;
}

interface ParseStatus {
  status: string;
  message?: string;
  result?: unknown;
  updatedAt?: string;
}

export default function PreviewPage() {
  const { documentId } = useParams<{ documentId: string }>();
  const [data, setData] = useState<PreviewData | null>(null);
  const [parseStatus, setParseStatus] = useState<ParseStatus | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  const loadParse = useCallback(async () => {
    if (!documentId) return;
    const res = await api<ParseStatus>(
      `/api/preview/${documentId}/parse-status`,
    );
    setParseStatus(res);
  }, [documentId]);

  useEffect(() => {
    if (!documentId) return;
    async function load() {
      try {
        const res = await api<PreviewData>(`/api/preview/${documentId}`);
        setData(res);
        if (res.previewType === "image") {
          await loadParse();
        }
        if (res.previewType === "pdf" && res.dsUrl) {
          await loadOnlyofficeApi(res.dsUrl);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "预览失败");
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [documentId, loadParse]);

  if (loading) {
    return <div style={{ padding: 48 }}>预览加载中…</div>;
  }

  if (error) {
    return (
      <div style={{ padding: 48 }}>
        <p style={{ color: "var(--color-danger)" }}>{error}</p>
        <Link to="/documents">返回文档中心</Link>
      </div>
    );
  }

  if (!data) return null;

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
        {data.label && <Tag tone="warning">{data.label}</Tag>}
        {data.readOnly && <Tag tone="neutral">只读</Tag>}
        <div style={{ flex: 1 }} />
        {data.aiEntries?.includes("aimed") && (
          <Button variant="secondary" size="sm">
            发起 AIMed
          </Button>
        )}
        {data.aiEntries?.includes("translation") && (
          <Button variant="secondary" size="sm">
            发起医学翻译
          </Button>
        )}
      </header>

      <div style={{ flex: 1, overflow: "auto", padding: 16 }}>
        {data.previewType === "image" && data.url && (
          <div>
            <img
              src={data.url}
              alt="预览"
              style={{ maxWidth: "100%", borderRadius: 8 }}
            />
            {data.visualParse && (
              <div style={{ marginTop: 16 }}>
                <Button variant="primary" size="sm" onClick={loadParse}>
                  视觉解析
                </Button>
                {parseStatus && (
                  <p style={{ marginTop: 8, fontSize: 13 }}>
                    状态：{parseStatus.status}
                    {parseStatus.message && ` — ${parseStatus.message}`}
                  </p>
                )}
              </div>
            )}
          </div>
        )}

        {(data.previewType === "pdf" || data.previewType === "ofd") && data.url && (
          <iframe
            src={data.url}
            title="PDF 预览"
            style={{ width: "100%", height: "80vh", border: "none" }}
          />
        )}

        {data.previewType === "pdf" && (
          <p style={{ fontSize: 12, color: "var(--color-text-3)", marginTop: 8 }}>
            当前页：{data.currentPage ?? 1}（供溯源定位，误差 ≤ 1 页）
          </p>
        )}
      </div>
    </div>
  );
}
