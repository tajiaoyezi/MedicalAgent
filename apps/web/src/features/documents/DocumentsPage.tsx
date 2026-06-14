import { useCallback, useEffect, useState } from "react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";

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

export default function DocumentsPage() {
  const [space, setSpace] = useState("my");
  const [recycle, setRecycle] = useState(false);
  const [docs, setDocs] = useState<DocRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [message, setMessage] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const q = recycle
        ? "?recycle=true"
        : `?space=${space}${space === "app" ? "" : ""}`;
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

  return (
    <ModuleShell
      title="文档中心"
      breadcrumb="文档中心"
      toolbar={
        <label className="primary" style={{ padding: "8px 16px", cursor: "pointer" }}>
          上传文件
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
      <div style={{ display: "flex", gap: 8, marginBottom: 16, flexWrap: "wrap" }}>
        {SPACES.map((s) => (
          <button
            key={s.id}
            className={!recycle && space === s.id ? "primary" : "ghost"}
            onClick={() => {
              setRecycle(false);
              setSpace(s.id);
            }}
          >
            {s.label}
          </button>
        ))}
        <button
          className={recycle ? "primary" : "ghost"}
          onClick={() => setRecycle(true)}
        >
          回收站
        </button>
      </div>
      {message && <p>{message}</p>}
      {loading ? (
        <p>加载中…</p>
      ) : docs.length === 0 ? (
        <p className="muted">暂无文档</p>
      ) : (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ textAlign: "left", borderBottom: "1px solid #eee" }}>
              <th>名称</th>
              <th>权限</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {docs.map((d) => (
              <tr key={d.document_id} style={{ borderBottom: "1px solid #f0f0f0" }}>
                <td style={{ padding: 8 }}>{d.name}</td>
                <td>{d.effectivePermission}</td>
                <td style={{ padding: 8 }}>
                  {!recycle && (
                    <>
                      <button className="ghost" onClick={() => download(d.document_id)}>
                        下载
                      </button>
                      <button className="ghost" onClick={() => deleteDoc(d.document_id)}>
                        删除
                      </button>
                    </>
                  )}
                  {recycle && (
                    <button
                      className="ghost"
                      onClick={() =>
                        api(`/api/documents/${d.document_id}/restore`, {
                          method: "POST",
                        }).then(load)
                      }
                    >
                      恢复
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </ModuleShell>
  );
}
