import { useState } from "react";
import { api, type SessionUser } from "../../lib/api";

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
      const res = await api<{
        user: SessionUser;
        redirectTo: string;
      }>("/api/auth/login", {
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
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "var(--color-bg)",
      }}
    >
      <form className="card" onSubmit={submit} style={{ width: 360 }}>
        <h1 style={{ margin: "0 0 8px", color: "var(--color-primary)" }}>
          MedOffice AI
        </h1>
        <p className="muted" style={{ marginBottom: 24 }}>
          医疗智能办公空间 · 演示登录
        </p>
        <label style={{ display: "block", marginBottom: 8 }}>用户名</label>
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          style={{ width: "100%", marginBottom: 16 }}
        />
        <label style={{ display: "block", marginBottom: 8 }}>口令</label>
        <input
          type="password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          style={{ width: "100%", marginBottom: 16 }}
        />
        {error && <p className="error">{error}</p>}
        <button className="primary" type="submit" disabled={loading} style={{ width: "100%" }}>
          {loading ? "登录中…" : "登录"}
        </button>
        <p className="muted" style={{ marginTop: 16, fontSize: 12 }}>
          演示账号：admin / admin123，user / user123
        </p>
      </form>
    </div>
  );
}
