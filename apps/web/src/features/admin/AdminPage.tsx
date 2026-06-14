import { useEffect, useState } from "react";
import ModuleShell from "../portal/ModuleShell";
import { api } from "../../lib/api";

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

export default function AdminPage() {
  const [tab, setTab] = useState<"users" | "audit" | "tenant">("users");
  const [users, setUsers] = useState<UserRow[]>([]);
  const [logs, setLogs] = useState<AuditRow[]>([]);
  const [tenant, setTenant] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    if (tab === "users") {
      api<{ users: UserRow[] }>("/api/admin/users").then((r) => setUsers(r.users));
    } else if (tab === "audit") {
      api<{ logs: AuditRow[] }>("/api/admin/audit-logs").then((r) =>
        setLogs(r.logs),
      );
    } else {
      api<{ tenant: Record<string, unknown> }>("/api/admin/tenant").then((r) =>
        setTenant(r.tenant),
      );
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
    <ModuleShell title="管理后台" breadcrumb="管理后台">
      <p className="muted" style={{ marginBottom: 16 }}>
        本期仅提供用户与角色管理、系统审计日志；模型/知识库/模板/翻译等配置由后续 phase 挂载。
      </p>
      <div style={{ display: "flex", gap: 8, marginBottom: 16 }}>
        {(["users", "audit", "tenant"] as const).map((t) => (
          <button
            key={t}
            className={tab === t ? "primary" : "ghost"}
            onClick={() => setTab(t)}
          >
            {t === "users" ? "用户与角色" : t === "audit" ? "审计日志" : "租户视图"}
          </button>
        ))}
      </div>
      {tab === "users" && (
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr>
              <th>用户名</th>
              <th>显示名</th>
              <th>角色</th>
              <th>状态</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.user_id}>
                <td>{u.username}</td>
                <td>{u.display_name}</td>
                <td>{(u.roles ?? []).join(", ")}</td>
                <td>{u.is_enabled ? "启用" : "禁用"}</td>
                <td>
                  <button
                    className="ghost"
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
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
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
                <td>{new Date(l.created_at).toLocaleString()}</td>
                <td>{l.action_type}</td>
                <td>{l.actor_role}</td>
                <td>
                  {l.result}
                  {l.failure_reason ? ` (${l.failure_reason})` : ""}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {tab === "tenant" && tenant && (
        <pre style={{ fontSize: 13, overflow: "auto" }}>
          {JSON.stringify(tenant, null, 2)}
        </pre>
      )}
    </ModuleShell>
  );
}
