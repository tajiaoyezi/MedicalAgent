import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Upload, Download, Trash2, RotateCcw, FileText, LayoutGrid, List, ExternalLink } from "lucide-react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";
import { Tag, EmptyState, SkeletonList } from "../../components";

interface DocRow {
  document_id: string;
  name: string;
  space: string;
  app_source: string | null;
  is_favorited: boolean;
  effectivePermission: string;
  updated_at: string;
}

const SPACES = [
  { id: "my", label: "我的文档" },
  { id: "team", label: "团队文档" },
  { id: "app", label: "应用文档" },
] as const;

const PERM_TONE: Record<string, "primary" | "success" | "warning" | "neutral"> = {
  owner: "primary",
  manage: "primary",
  edit: "success",
  comment: "warning",
  view: "neutral",
};

export default function DocumentsPage() {
  const navigate = useNavigate();
  const [space, setSpace] = useState("my");
  const [recycle, setRecycle] = useState(false);
  const [view, setView] = useState<"table" | "card">("table");
  const [docs, setDocs] = useState<DocRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const q = recycle ? "?recycle=true" : `?space=${space}`;
      const res = await api<{ documents: DocRow[] }>(`/api/documents${q}`);
      setDocs(res.documents);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "加载失败");
    } finally {
      setLoading(false);
    }
  }, [space, recycle]);

  useEffect(() => {
    load();
  }, [load]);

  async function upload(file: File) {
    const fd = new FormData();
    fd.append("file", file);
    fd.append("space", space);
    try {
      await api("/api/documents/upload", { method: "POST", body: fd });
      setMessage("上传成功");
      load();
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "上传失败");
    }
  }

  async function download(id: string) {
    try {
      const res = await api<{ url: string }>(`/api/documents/${id}/download`);
      window.open(res.url, "_blank");
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "下载失败");
    }
  }

  async function deleteDoc(id: string) {
    if (!confirm("确认删除到回收站？")) return;
    await api(`/api/documents/${id}`, { method: "DELETE" });
    load();
  }

  function restore(id: string) {
    api(`/api/documents/${id}/restore`, { method: "POST" }).then(load);
  }

  function openDoc(id: string) {
    navigate(`/editor/${id}`);
  }

  const actions = (d: DocRow) =>
    recycle ? (
      <button className="btn btn-sm btn-secondary" onClick={() => restore(d.document_id)}>
        <RotateCcw size={14} /> 恢复
      </button>
    ) : (
      <>
        <button className="btn btn-sm btn-primary" onClick={() => openDoc(d.document_id)}>
          <ExternalLink size={14} /> 打开
        </button>
        <button className="btn btn-sm btn-ghost" onClick={() => download(d.document_id)}>
          <Download size={14} /> 下载
        </button>
        <button className="btn btn-sm btn-ghost" onClick={() => deleteDoc(d.document_id)}>
          <Trash2 size={14} /> 删除
        </button>
      </>
    );

  return (
    <ModuleShell
      title="文档中心"
      breadcrumb="文档与任务 · 文档中心"
      toolbar={
        <label className="btn btn-primary" style={{ cursor: "pointer" }}>
          <Upload size={15} /> 上传文件
          <input
            type="file"
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) upload(f);
            }}
          />
        </label>
      }
    >
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 16, flexWrap: "wrap" }}>
        {SPACES.map((s) => (
          <button
            key={s.id}
            className={`btn btn-sm ${!recycle && space === s.id ? "btn-primary" : "btn-secondary"}`}
            onClick={() => {
              setRecycle(false);
              setSpace(s.id);
            }}
          >
            {s.label}
          </button>
        ))}
        <button
          className={`btn btn-sm ${recycle ? "btn-primary" : "btn-secondary"}`}
          onClick={() => setRecycle(true)}
        >
          回收站
        </button>
        <div style={{ flex: 1 }} />
        <button
          className={`btn btn-sm ${view === "table" ? "btn-primary" : "btn-ghost"}`}
          onClick={() => setView("table")}
          title="表格视图"
          style={{ width: 34, padding: 0 }}
        >
          <List size={16} />
        </button>
        <button
          className={`btn btn-sm ${view === "card" ? "btn-primary" : "btn-ghost"}`}
          onClick={() => setView("card")}
          title="卡片视图"
          style={{ width: 34, padding: 0 }}
        >
          <LayoutGrid size={16} />
        </button>
      </div>

      {message && (
        <p style={{ fontSize: 13, color: "var(--color-text-2)", margin: "0 0 12px" }}>{message}</p>
      )}

      {loading ? (
        <SkeletonList rows={5} />
      ) : docs.length === 0 ? (
        <EmptyState title={recycle ? "回收站为空" : "暂无文档"} desc="上传 PDF / DOCX 即可在此管理与按权限协作。" />
      ) : view === "table" ? (
        <table className="tbl">
          <thead>
            <tr>
              <th>名称</th>
              <th>权限</th>
              <th style={{ width: 200 }}>操作</th>
            </tr>
          </thead>
          <tbody>
            {docs.map((d) => (
              <tr key={d.document_id}>
                <td>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                    <FileText size={16} style={{ color: "var(--color-text-3)" }} />
                    {d.name}
                  </span>
                </td>
                <td>
                  <Tag tone={PERM_TONE[d.effectivePermission] ?? "neutral"}>
                    {d.effectivePermission}
                  </Tag>
                </td>
                <td>
                  <span style={{ display: "inline-flex", gap: 6 }}>{actions(d)}</span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill,minmax(240px,1fr))",
            gap: 14,
          }}
        >
          {docs.map((d) => (
            <div
              key={d.document_id}
              style={{
                border: "1px solid var(--color-border)",
                borderRadius: 12,
                padding: 14,
                background: "var(--color-surface)",
              }}
            >
              <div style={{ display: "flex", alignItems: "center", gap: 9, marginBottom: 10 }}>
                <span
                  style={{
                    width: 34,
                    height: 34,
                    borderRadius: 9,
                    background: "var(--color-primary-softer)",
                    color: "var(--color-primary)",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                  }}
                >
                  <FileText size={18} />
                </span>
                <div
                  style={{
                    fontSize: 13.5,
                    fontWeight: 600,
                    minWidth: 0,
                    whiteSpace: "nowrap",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                  }}
                  title={d.name}
                >
                  {d.name}
                </div>
              </div>
              <div style={{ marginBottom: 12 }}>
                <Tag tone={PERM_TONE[d.effectivePermission] ?? "neutral"}>{d.effectivePermission}</Tag>
              </div>
              <div style={{ display: "flex", gap: 6 }}>{actions(d)}</div>
            </div>
          ))}
        </div>
      )}
    </ModuleShell>
  );
}
