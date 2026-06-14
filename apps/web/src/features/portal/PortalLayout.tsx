import { NavLink } from "react-router-dom";
import type { SessionUser } from "../../lib/api";
import type { ThemeId } from "../../lib/theme";

const NAV_ITEMS = [
  { to: "/aimed", label: "AIMed 学术助手" },
  { to: "/knowledge", label: "医疗知识库" },
  { to: "/digital-staff", label: "医疗数字员工" },
  { to: "/translation", label: "医学翻译" },
  { to: "/templates", label: "医疗模板库" },
  { to: "/documents", label: "文档中心" },
  { to: "/recent", label: "最近任务" },
] as const;

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
  return (
    <div style={{ display: "flex", minHeight: "100vh" }}>
      <nav
        style={{
          width: 220,
          background: "var(--color-nav-bg)",
          color: "var(--color-nav-text)",
          padding: "16px 0",
          flexShrink: 0,
        }}
      >
        <div style={{ padding: "0 16px 16px", fontWeight: 600 }}>
          MedOffice AI
        </div>
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            style={({ isActive }) => ({
              display: "block",
              padding: "10px 16px",
              color: isActive ? "#fff" : "rgba(255,255,255,0.75)",
              background: isActive ? "rgba(255,255,255,0.12)" : "transparent",
              textDecoration: "none",
            })}
          >
            {item.label}
          </NavLink>
        ))}
        {user.isAdmin && (
          <NavLink
            to="/admin"
            style={({ isActive }) => ({
              display: "block",
              padding: "10px 16px",
              color: isActive ? "#fff" : "rgba(255,255,255,0.75)",
              background: isActive ? "rgba(255,255,255,0.12)" : "transparent",
              textDecoration: "none",
            })}
          >
            管理后台
          </NavLink>
        )}
        <div style={{ padding: "16px", borderTop: "1px solid rgba(255,255,255,0.15)", marginTop: 16 }}>
          <div style={{ fontSize: 12, opacity: 0.8 }}>{user.displayName}</div>
          <select
            value={theme}
            onChange={(e) => onThemeChange(e.target.value as ThemeId)}
            style={{ width: "100%", marginTop: 8 }}
          >
            <option value="blue-white">蓝白主题</option>
            <option value="green-white">绿白主题</option>
          </select>
          <button
            className="ghost"
            onClick={onLogout}
            style={{ width: "100%", marginTop: 8, color: "#fff", borderColor: "rgba(255,255,255,0.3)" }}
          >
            退出
          </button>
        </div>
      </nav>
      <main style={{ flex: 1, padding: 24, minWidth: 0 }}>
        {children}
      </main>
    </div>
  );
}
