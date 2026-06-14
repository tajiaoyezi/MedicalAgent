import { useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { PanelLeft, Search, ChevronRight, ChevronDown, User, Settings, LogOut } from "lucide-react";
import type { SessionUser } from "../../lib/api";
import { ComplianceBadge } from "../../components";
import { routeMeta } from "./nav";

export default function TopBar({
  user,
  onLogout,
  onToggleNav,
}: {
  user: SessionUser;
  onLogout: () => void;
  onToggleNav: () => void;
}) {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const meta = routeMeta(pathname);
  const [menu, setMenu] = useState(false);
  const initial = user.displayName?.[0] ?? "U";

  return (
    <header
      style={{
        height: 60,
        flexShrink: 0,
        background: "var(--color-surface)",
        borderBottom: "1px solid var(--color-border)",
        display: "flex",
        alignItems: "center",
        gap: 16,
        padding: "0 22px",
        zIndex: 20,
      }}
    >
      <button
        className="btn btn-secondary btn-sm"
        onClick={onToggleNav}
        style={{ width: 34, height: 34, padding: 0 }}
        title="折叠 / 展开导航"
      >
        <PanelLeft size={17} />
      </button>

      <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
        <span style={{ fontSize: 13, color: "var(--color-text-3)" }}>{meta.group}</span>
        <ChevronRight size={14} style={{ color: "var(--color-text-3)" }} />
        <span
          style={{
            fontSize: 15,
            fontWeight: 600,
            color: "var(--color-text)",
            whiteSpace: "nowrap",
          }}
        >
          {meta.label}
        </span>
      </div>

      <div style={{ flex: 1 }} />

      {/* search (visual) */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 9,
          height: 38,
          width: 260,
          padding: "0 13px",
          border: "1px solid var(--color-border)",
          borderRadius: 10,
          background: "var(--color-surface-2)",
          color: "var(--color-text-3)",
        }}
      >
        <Search size={16} />
        <span style={{ fontSize: 13 }}>搜索文档、文献、模板…</span>
      </div>

      <ComplianceBadge kind="redaction" label="脱敏门禁已启用" tone="success" />
      <ComplianceBadge kind="model-env" label="私有化模型" tone="primary" />

      <div style={{ width: 1, height: 26, background: "var(--color-border)" }} />

      {/* user menu */}
      <div style={{ position: "relative" }}>
        <div
          onClick={() => setMenu((v) => !v)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            cursor: "pointer",
            padding: "4px 6px",
            borderRadius: 9,
          }}
        >
          <div
            style={{
              width: 32,
              height: 32,
              borderRadius: 9,
              background: "linear-gradient(135deg,#1677ff,#0a5fe0)",
              color: "#fff",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 13,
              fontWeight: 700,
            }}
          >
            {initial}
          </div>
          <ChevronDown size={15} style={{ color: "var(--color-text-3)" }} />
        </div>
        {menu && (
          <>
            <div style={{ position: "fixed", inset: 0, zIndex: 40 }} onClick={() => setMenu(false)} />
            <div
              style={{
                position: "absolute",
                top: 46,
                right: 0,
                width: 208,
                background: "var(--color-surface)",
                border: "1px solid var(--color-border)",
                borderRadius: 12,
                boxShadow: "var(--shadow-lg)",
                padding: 6,
                zIndex: 50,
                animation: "fadeUp .15s ease both",
              }}
            >
              <div
                style={{
                  padding: "10px 12px",
                  borderBottom: "1px solid var(--color-divider)",
                  marginBottom: 4,
                }}
              >
                <div style={{ fontSize: 13.5, fontWeight: 600 }}>{user.displayName}</div>
                <div style={{ fontSize: 11.5, color: "var(--color-text-3)" }}>{user.username}</div>
              </div>
              <MenuItem icon={<User size={15} />} label="个人资料" onClick={() => setMenu(false)} />
              {user.isAdmin && (
                <MenuItem
                  icon={<Settings size={15} />}
                  label="管理后台"
                  onClick={() => {
                    setMenu(false);
                    navigate("/admin");
                  }}
                />
              )}
              <MenuItem
                icon={<LogOut size={15} />}
                label="退出登录"
                danger
                onClick={() => {
                  setMenu(false);
                  onLogout();
                }}
              />
            </div>
          </>
        )}
      </div>
    </header>
  );
}

function MenuItem({
  icon,
  label,
  danger,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  danger?: boolean;
  onClick: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className="menu-item"
      style={{
        display: "flex",
        alignItems: "center",
        gap: 9,
        padding: "9px 12px",
        borderRadius: 8,
        fontSize: 13.5,
        cursor: "pointer",
        color: danger ? "var(--color-danger)" : "var(--color-text)",
      }}
    >
      {icon}
      {label}
    </div>
  );
}
