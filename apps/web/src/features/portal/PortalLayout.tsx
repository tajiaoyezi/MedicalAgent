import { useState } from "react";
import { NavLink, useLocation } from "react-router-dom";
import { LogOut, Plus } from "lucide-react";
import type { SessionUser } from "../../lib/api";
import { THEME_LABELS, type ThemeId } from "../../lib/theme";
import { NAV_GROUPS } from "./nav";
import TopBar from "./TopBar";

const THEME_DOTS: { id: ThemeId; color: string }[] = [
  { id: "blue-white", color: "#1677ff" },
  { id: "green-white", color: "#13a065" },
  { id: "dark", color: "#0a0f1c" },
];

export default function PortalLayout({
  children,
  user,
  onLogout,
  theme,
  onThemeChange,
}: {
  children: React.ReactNode;
  user: SessionUser;
  onLogout: () => void;
  theme: ThemeId;
  onThemeChange: (t: ThemeId) => void;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const { pathname } = useLocation();
  const sidebarW = collapsed ? 72 : 232;
  const initial = user.displayName?.[0] ?? "U";

  return (
    <div style={{ height: "100vh", width: "100%", display: "flex", overflow: "hidden" }}>
      {/* SIDEBAR */}
      <aside
        style={{
          width: sidebarW,
          flexShrink: 0,
          background: "var(--color-nav-bg)",
          display: "flex",
          flexDirection: "column",
          transition: "width .18s ease",
          overflow: "hidden",
        }}
      >
        {/* brand */}
        <div
          style={{
            height: 60,
            display: "flex",
            alignItems: "center",
            gap: 11,
            padding: "0 16px",
            flexShrink: 0,
            borderBottom: "1px solid var(--color-nav-border)",
          }}
        >
          <div
            style={{
              width: 34,
              height: 34,
              borderRadius: 9,
              background: "linear-gradient(135deg,#1677ff,#0a5fe0)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
              boxShadow: "0 4px 12px rgba(22,119,255,.4)",
            }}
          >
            <Plus size={20} color="#fff" strokeWidth={2.6} />
          </div>
          {!collapsed && (
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 15, fontWeight: 700, color: "#fff", lineHeight: 1.2 }}>
                MedOffice AI
              </div>
              <div style={{ fontSize: 10.5, color: "var(--color-nav-text)", letterSpacing: 1.2 }}>
                医疗智能办公空间
              </div>
            </div>
          )}
        </div>

        {/* nav */}
        <div style={{ flex: 1, overflowY: "auto", overflowX: "hidden", padding: "12px 0" }}>
          {NAV_GROUPS.map((group) => {
            const items = group.items.filter((i) => !i.adminOnly || user.isAdmin);
            if (!items.length) return null;
            return (
              <div key={group.label}>
                {!collapsed && (
                  <div
                    style={{
                      fontSize: 10.5,
                      fontWeight: 700,
                      letterSpacing: 1,
                      color: "var(--color-nav-text)",
                      opacity: 0.7,
                      padding: "12px 24px 6px",
                      textTransform: "uppercase",
                    }}
                  >
                    {group.label}
                  </div>
                )}
                {items.map((item) => {
                  const Icon = item.icon;
                  const active = pathname.startsWith(item.to);
                  const planned = item.planned;
                  const content = (
                    <>
                      <Icon size={19} strokeWidth={1.8} style={{ flexShrink: 0 }} />
                      {!collapsed && (
                        <span style={{ flex: 1, whiteSpace: "nowrap" }}>{item.label}</span>
                      )}
                      {!collapsed && planned && (
                        <span
                          style={{
                            fontSize: 10,
                            fontWeight: 600,
                            padding: "1px 6px",
                            borderRadius: 6,
                            background: "var(--color-nav-hover-bg)",
                            color: "var(--color-nav-text)",
                          }}
                        >
                          规划中
                        </span>
                      )}
                    </>
                  );
                  const baseStyle: React.CSSProperties = {
                    display: "flex",
                    alignItems: "center",
                    gap: 12,
                    padding: "10px 12px",
                    margin: "2px 12px",
                    borderRadius: 10,
                    fontSize: 13.5,
                    fontWeight: active ? 600 : 500,
                    color: active
                      ? "var(--color-nav-active-text)"
                      : planned
                        ? "var(--color-text-disabled)"
                        : "var(--color-nav-text)",
                    background: active ? "var(--color-nav-active-bg)" : undefined,
                    boxShadow: active ? "inset 3px 0 0 var(--color-primary)" : "none",
                    cursor: planned ? "not-allowed" : "pointer",
                    opacity: planned ? 0.7 : 1,
                    justifyContent: collapsed ? "center" : "flex-start",
                    textDecoration: "none",
                  };
                  if (planned) {
                    return (
                      <div key={item.to} style={baseStyle} title={`${item.label}（规划中）`}>
                        {content}
                      </div>
                    );
                  }
                  return (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      className="nav-item"
                      style={baseStyle}
                      title={item.label}
                    >
                      {content}
                    </NavLink>
                  );
                })}
              </div>
            );
          })}
        </div>

        {/* footer: theme dots + user + logout */}
        <div
          style={{
            flexShrink: 0,
            borderTop: "1px solid var(--color-nav-border)",
            padding: 12,
          }}
        >
          <div style={{ display: "flex", gap: 6 }}>
            {THEME_DOTS.map((d) => (
              <span
                key={d.id}
                onClick={() => onThemeChange(d.id)}
                title={THEME_LABELS[d.id]}
                style={{
                  height: 26,
                  flex: collapsed ? "none" : 1,
                  width: collapsed ? 16 : undefined,
                  borderRadius: 7,
                  background: d.color,
                  cursor: "pointer",
                  border: theme === d.id ? "2px solid #fff" : "2px solid transparent",
                  boxShadow:
                    theme === d.id
                      ? "0 0 0 2px var(--color-primary)"
                      : "inset 0 0 0 1px rgba(255,255,255,.18)",
                }}
              />
            ))}
          </div>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              padding: "9px 6px",
              marginTop: 8,
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
                flexShrink: 0,
              }}
            >
              {initial}
            </div>
            {!collapsed && (
              <div style={{ minWidth: 0, flex: 1 }}>
                <div
                  style={{
                    fontSize: 13,
                    fontWeight: 600,
                    color: "#fff",
                    whiteSpace: "nowrap",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                  }}
                >
                  {user.displayName}
                </div>
                <div style={{ fontSize: 11, color: "var(--color-nav-text)" }}>
                  {user.isAdmin ? "管理员" : "成员"}
                </div>
              </div>
            )}
            <span title="退出登录" onClick={onLogout} style={{ display: "flex", flexShrink: 0, cursor: "pointer" }}>
              <LogOut size={17} style={{ color: "var(--color-nav-text)" }} />
            </span>
          </div>
        </div>
      </aside>

      {/* MAIN COLUMN */}
      <div
        style={{
          flex: 1,
          minWidth: 0,
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          background: "var(--color-bg)",
        }}
      >
        <TopBar
          user={user}
          onLogout={onLogout}
          onToggleNav={() => setCollapsed((v) => !v)}
        />
        <div style={{ flex: 1, overflowY: "auto", overflowX: "hidden" }}>{children}</div>
      </div>
    </div>
  );
}
