import type { ReactNode } from "react";

type Tone = "primary" | "success" | "warning" | "danger" | "info" | "neutral";

const TONE: Record<Tone, { fg: string; bg: string }> = {
  primary: { fg: "var(--color-primary)", bg: "var(--color-primary-softer)" },
  success: { fg: "var(--color-success)", bg: "var(--color-success-soft)" },
  warning: { fg: "var(--color-warning)", bg: "var(--color-warning-soft)" },
  danger: { fg: "var(--color-danger)", bg: "var(--color-danger-soft)" },
  info: { fg: "var(--color-info)", bg: "var(--color-info-soft)" },
  neutral: { fg: "var(--color-text-3)", bg: "var(--color-surface-3)" },
};

export function Tag({
  tone = "neutral",
  icon,
  children,
}: {
  tone?: Tone;
  icon?: ReactNode;
  children: ReactNode;
}) {
  const c = TONE[tone];
  return (
    <span className="tag" style={{ color: c.fg, background: c.bg }}>
      {icon}
      {children}
    </span>
  );
}
