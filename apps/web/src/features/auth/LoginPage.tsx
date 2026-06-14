import { useState } from "react";
import { Plus, ShieldCheck, BookOpenCheck, Languages } from "lucide-react";
import { api, type SessionUser } from "../../lib/api";
import { Button, Field, Input, Disclaimer } from "../../components";

export default function LoginPage({
  onLogin,
}: {
  onLogin: (user: SessionUser) => void;
}) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin123");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await api<{ user: SessionUser; redirectTo: string }>("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({ username, password }),
      });
      onLogin(res.user);
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ height: "100vh", display: "flex", background: "var(--color-bg)" }}>
      {/* brand panel */}
      <div
        style={{
          flex: 1,
          background: "linear-gradient(150deg,#0d1f38,#0a5fe0)",
          color: "#fff",
          padding: "56px 60px",
          display: "flex",
          flexDirection: "column",
          justifyContent: "center",
          minWidth: 0,
        }}
        className="login-brand"
      >
        <div style={{ display: "flex", alignItems: "center", gap: 12, marginBottom: 28 }}>
          <div
            style={{
              width: 44,
              height: 44,
              borderRadius: 12,
              background: "rgba(255,255,255,.16)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            <Plus size={26} strokeWidth={2.6} />
          </div>
          <div>
            <div style={{ fontSize: 22, fontWeight: 700 }}>MedOffice AI</div>
            <div style={{ fontSize: 13, opacity: 0.8, letterSpacing: 2 }}>医疗智能办公空间</div>
          </div>
        </div>
        <div style={{ fontSize: 30, fontWeight: 700, lineHeight: 1.4, maxWidth: 460 }}>
          可溯源、可确认的
          <br />
          医疗智能办公平台
        </div>
        <div style={{ marginTop: 20, fontSize: 14, opacity: 0.85, lineHeight: 1.8, maxWidth: 440 }}>
          深度文献伴读 · 智能综述 · 医学翻译 · 知识库 RAG —— 公网调用前 PHI/PII 脱敏，离线优先、私有化可降级。
        </div>
        <div style={{ marginTop: 36, display: "flex", flexDirection: "column", gap: 14 }}>
          {[
            { icon: <BookOpenCheck size={18} />, t: "带引用溯源的医学问答与综述" },
            { icon: <Languages size={18} />, t: "医学翻译术语一致、译稿可写回" },
            { icon: <ShieldCheck size={18} />, t: "多租户隔离 · RBAC · 写回人工确认" },
          ].map((f) => (
            <div key={f.t} style={{ display: "flex", alignItems: "center", gap: 11, fontSize: 14, opacity: 0.92 }}>
              <span style={{ opacity: 0.9 }}>{f.icon}</span>
              {f.t}
            </div>
          ))}
        </div>
      </div>

      {/* login card */}
      <div
        style={{
          width: 480,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          padding: 32,
        }}
      >
        <form onSubmit={submit} style={{ width: 340 }}>
          <h1 style={{ margin: "0 0 6px", fontSize: 24, color: "var(--color-text)" }}>登录</h1>
          <p style={{ margin: "0 0 26px", fontSize: 13.5, color: "var(--color-text-3)" }}>
            欢迎回到 MedOffice AI · 演示登录
          </p>

          <Field label="用户名" style={{ marginBottom: 16 }}>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" />
          </Field>
          <Field label="口令" style={{ marginBottom: 16 }}>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
            />
          </Field>

          {error && (
            <p className="error" style={{ fontSize: 13, margin: "0 0 14px" }}>
              {error}
            </p>
          )}

          <Button type="submit" block loading={loading} style={{ height: 42 }}>
            {loading ? "登录中…" : "登录"}
          </Button>

          <p style={{ marginTop: 16, fontSize: 12, color: "var(--color-text-3)" }}>
            演示账号：admin / admin123，user / user123
          </p>
          <div style={{ marginTop: 22 }}>
            <Disclaimer text="本平台所有 AI 产出均为草稿 / 辅助建议，不构成诊断、处方或医嘱，需经医生确认。" />
          </div>
        </form>
      </div>
    </div>
  );
}
