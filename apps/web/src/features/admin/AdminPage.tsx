import { useEffect, useState } from "react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";
import { Tabs, Tag, EmptyState } from "../../components";

interface UserRow {
  user_id: string;
  username: string;
  display_name: string;
  dept_id: string | null;
  is_enabled: boolean;
  roles: string[];
}

interface AuditRow {
  action_type: string;
  actor_role: string;
  result: string;
  failure_reason: string | null;
  created_at: string;
  target_type: string | null;
  target_id: string | null;
}

type Tab = "users" | "audit" | "tenant" | "provider";

const TABS: { key: Tab; label: string }[] = [
  { key: "users", label: "用户与角色" },
  { key: "audit", label: "审计日志" },
  { key: "tenant", label: "租户视图" },
  { key: "provider", label: "模型 Provider" },
];

export default function AdminPage() {
  const [tab, setTab] = useState<Tab>("users");
  const [users, setUsers] = useState<UserRow[]>([]);
  const [logs, setLogs] = useState<AuditRow[]>([]);
  const [tenant, setTenant] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    if (tab === "users") {
      api<{ users: UserRow[] }>("/api/admin/users").then((r) => setUsers(r.users));
    } else if (tab === "audit") {
      api<{ logs: AuditRow[] }>("/api/admin/audit-logs").then((r) => setLogs(r.logs));
    } else if (tab === "tenant") {
      api<{ tenant: Record<string, unknown> }>("/api/admin/tenant").then((r) => setTenant(r.tenant));
    }
  }, [tab]);

  async function toggleUser(id: string, enabled: boolean) {
    await api(`/api/admin/users/${id}`, {
      method: "PATCH",
      body: JSON.stringify({ isEnabled: enabled }),
    });
    const res = await api<{ users: UserRow[] }>("/api/admin/users");
    setUsers(res.users);
  }

  return (
    <ModuleShell title="管理后台" breadcrumb="管理 · 管理后台">
      <div style={{ marginBottom: 16 }}>
        <Tabs tabs={TABS} active={tab} onChange={setTab} />
      </div>

      {tab === "users" && (
        <table className="tbl">
          <thead>
            <tr>
              <th>用户名</th>
              <th>显示名</th>
              <th>角色</th>
              <th>状态</th>
              <th style={{ width: 110 }}>操作</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.user_id}>
                <td>{u.username}</td>
                <td>{u.display_name}</td>
                <td>
                  <span style={{ display: "inline-flex", gap: 4, flexWrap: "wrap" }}>
                    {(u.roles ?? []).map((r) => (
                      <Tag key={r} tone="info">
                        {r}
                      </Tag>
                    ))}
                  </span>
                </td>
                <td>
                  <Tag tone={u.is_enabled ? "success" : "neutral"}>
                    {u.is_enabled ? "启用" : "禁用"}
                  </Tag>
                </td>
                <td>
                  <button
                    className="btn btn-sm btn-ghost"
                    onClick={() => toggleUser(u.user_id, !u.is_enabled)}
                  >
                    {u.is_enabled ? "禁用" : "启用"}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {tab === "audit" && (
        <table className="tbl">
          <thead>
            <tr>
              <th>时间</th>
              <th>操作</th>
              <th>角色</th>
              <th>结果</th>
            </tr>
          </thead>
          <tbody>
            {logs.map((l, i) => (
              <tr key={i}>
                <td style={{ color: "var(--color-text-3)", whiteSpace: "nowrap" }}>
                  {new Date(l.created_at).toLocaleString()}
                </td>
                <td>{l.action_type}</td>
                <td>{l.actor_role}</td>
                <td>
                  <Tag tone={l.result === "成功" || l.result === "success" ? "success" : "danger"}>
                    {l.result}
                    {l.failure_reason ? ` · ${l.failure_reason}` : ""}
                  </Tag>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {tab === "tenant" && tenant && (
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fill,minmax(220px,1fr))",
            gap: 12,
          }}
        >
          {Object.entries(tenant).map(([k, v]) => (
            <div
              key={k}
              style={{
                border: "1px solid var(--color-border)",
                borderRadius: 10,
                padding: "10px 13px",
                background: "var(--color-surface-2)",
              }}
            >
              <div style={{ fontSize: 11.5, color: "var(--color-text-3)", marginBottom: 3 }}>{k}</div>
              <div style={{ fontSize: 13.5, fontWeight: 600, wordBreak: "break-all" }}>
                {v === null || v === undefined ? "—" : String(v)}
              </div>
            </div>
          ))}
        </div>
      )}

      {tab === "provider" && (
        <EmptyState
          title="模型 Provider 配置 · 规划中"
          desc="公网 / 私有化双入口、优先级与 fallback 的 Provider 管理由 c03 model-and-parse 挂载，本期暂不提供。"
        />
      )}
    </ModuleShell>
  );
}
