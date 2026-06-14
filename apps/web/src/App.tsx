import { useEffect, useState } from "react";
import {
  BrowserRouter,
  Navigate,
  Route,
  Routes,
} from "react-router-dom";
import { api, type SessionUser } from "./lib/api";
import { applyTheme, type ThemeId } from "./lib/theme";
import LoginPage from "./features/auth/LoginPage";
import PortalLayout from "./features/portal/PortalLayout";
import AimedPage from "./features/aimed/AimedPage";
import KnowledgePage from "./features/knowledge/KnowledgePage";
import DigitalStaffPage from "./features/digital-staff/DigitalStaffPage";
import TranslationPage from "./features/translation/TranslationPage";
import TemplatesPage from "./features/templates/TemplatesPage";
import DocumentsPage from "./features/documents/DocumentsPage";
import RecentTasksPage from "./features/recent/RecentTasksPage";
import AdminPage from "./features/admin/AdminPage";

function AppRoutes({
  user,
  onLogout,
  theme,
  onThemeChange,
}: {
  user: SessionUser;
  onLogout: () => void;
  theme: ThemeId;
  onThemeChange: (t: ThemeId) => void;
}) {
  return (
    <PortalLayout
      user={user}
      onLogout={onLogout}
      theme={theme}
      onThemeChange={onThemeChange}
    >
      <Routes>
        <Route path="/" element={<Navigate to="/aimed" replace />} />
        <Route path="/aimed" element={<AimedPage />} />
        <Route path="/knowledge" element={<KnowledgePage />} />
        <Route path="/digital-staff" element={<DigitalStaffPage />} />
        <Route path="/translation" element={<TranslationPage />} />
        <Route path="/templates" element={<TemplatesPage />} />
        <Route path="/documents" element={<DocumentsPage />} />
        <Route path="/recent" element={<RecentTasksPage />} />
        <Route
          path="/admin"
          element={
            user.isAdmin ? <AdminPage /> : <Navigate to="/aimed" replace />
          }
        />
        <Route path="*" element={<Navigate to="/aimed" replace />} />
      </Routes>
    </PortalLayout>
  );
}

export default function App() {
  const [user, setUser] = useState<SessionUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [theme, setTheme] = useState<ThemeId>("blue-white");

  // Inject default theme tokens on mount so pre-auth screens (login) are themed.
  useEffect(() => {
    applyTheme(theme);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    async function init() {
      try {
        const session = await api<{ authenticated: boolean; user?: SessionUser }>(
          "/api/auth/session",
        );
        if (session.authenticated && session.user) {
          setUser(session.user);
          const branding = await api<{
            branding: { default_theme?: string };
          }>("/api/portal/branding");
          if (branding.branding?.default_theme === "green-white") {
            setTheme("green-white");
            applyTheme("green-white", branding.branding);
          } else {
            applyTheme("blue-white", branding.branding);
          }
        }
      } finally {
        setLoading(false);
      }
    }
    init();
  }, []);

  const handleLogin = (u: SessionUser) => {
    setUser(u);
    applyTheme(theme);
  };

  const handleLogout = async () => {
    await api("/api/auth/logout", { method: "POST" });
    setUser(null);
  };

  const handleThemeChange = (t: ThemeId) => {
    setTheme(t);
    applyTheme(t);
  };

  if (loading) {
    return (
      <div style={{ padding: 48, textAlign: "center" }}>MedOffice AI 加载中…</div>
    );
  }

  if (!user) {
    return <LoginPage onLogin={handleLogin} />;
  }

  return (
    <BrowserRouter>
      <AppRoutes
        user={user}
        onLogout={handleLogout}
        theme={theme}
        onThemeChange={handleThemeChange}
      />
    </BrowserRouter>
  );
}
