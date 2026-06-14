import type { ReactNode } from "react";

export function Card({
  title,
  extra,
  children,
  padding = 18,
  style,
}: {
  title?: ReactNode;
  extra?: ReactNode;
  children: ReactNode;
  padding?: number;
  style?: React.CSSProperties;
}) {
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 14,
        boxShadow: "var(--shadow-sm)",
        ...style,
      }}
    >
      {(title || extra) && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "14px 18px",
            borderBottom: "1px solid var(--color-divider)",
          }}
        >
          <div style={{ fontSize: 14.5, fontWeight: 600 }}>{title}</div>
          {extra}
        </div>
      )}
      <div style={{ padding }}>{children}</div>
    </div>
  );
}
