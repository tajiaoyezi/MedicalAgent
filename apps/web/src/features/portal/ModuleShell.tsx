import type { ReactNode } from "react";

export default function ModuleShell({
  title,
  breadcrumb,
  children,
  toolbar,
  note = "ONLYOFFICE 编辑器原生 UI 不承诺跟随主题，仅外部宿主页面与面板入口适配主题。",
}: {
  title: string;
  breadcrumb?: string;
  children: ReactNode;
  toolbar?: ReactNode;
  note?: string;
}) {
  return (
    <div style={{ padding: 24, maxWidth: 1400, margin: "0 auto" }}>
      <div
        style={{
          display: "flex",
          alignItems: "flex-end",
          justifyContent: "space-between",
          marginBottom: 16,
          gap: 12,
        }}
      >
        <div>
          {breadcrumb && (
            <div style={{ fontSize: 12, color: "var(--color-text-3)" }}>{breadcrumb}</div>
          )}
          <h1 style={{ margin: "4px 0 0", fontSize: 22, color: "var(--color-text)" }}>{title}</h1>
        </div>
        {toolbar}
      </div>
      <div
        style={{
          background: "var(--color-surface)",
          border: "1px solid var(--color-border)",
          borderRadius: 14,
          boxShadow: "var(--shadow-sm)",
          padding: 20,
        }}
      >
        {children}
      </div>
      {note && (
        <p style={{ marginTop: 12, fontSize: 12, color: "var(--color-text-3)" }}>{note}</p>
      )}
    </div>
  );
}
