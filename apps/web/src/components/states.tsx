import type { ReactNode } from "react";
import { Inbox, AlertTriangle, Lock } from "lucide-react";

function Centered({
  icon,
  title,
  desc,
  action,
  tone = "var(--color-text-3)",
}: {
  icon: ReactNode;
  title: string;
  desc?: string;
  action?: ReactNode;
  tone?: string;
}) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        gap: 12,
        padding: "56px 24px",
        textAlign: "center",
        color: "var(--color-text-2)",
      }}
    >
      <div style={{ color: tone }}>{icon}</div>
      <div style={{ fontSize: 15, fontWeight: 600, color: "var(--color-text)" }}>
        {title}
      </div>
      {desc && (
        <div style={{ fontSize: 13, color: "var(--color-text-3)", maxWidth: 360 }}>
          {desc}
        </div>
      )}
      {action}
    </div>
  );
}

export function EmptyState({
  title = "暂无数据",
  desc,
  action,
}: {
  title?: string;
  desc?: string;
  action?: ReactNode;
}) {
  return <Centered icon={<Inbox size={40} strokeWidth={1.6} />} title={title} desc={desc} action={action} />;
}

export function ErrorState({
  title = "出错了",
  desc = "请稍后重试或联系管理员。",
  action,
}: {
  title?: string;
  desc?: string;
  action?: ReactNode;
}) {
  return (
    <Centered
      icon={<AlertTriangle size={40} strokeWidth={1.6} />}
      title={title}
      desc={desc}
      action={action}
      tone="var(--color-danger)"
    />
  );
}

export function NoPermission({
  title = "无访问权限",
  desc = "你没有查看该内容的权限，请联系管理员申请。",
}: {
  title?: string;
  desc?: string;
}) {
  return (
    <Centered
      icon={<Lock size={40} strokeWidth={1.6} />}
      title={title}
      desc={desc}
      tone="var(--color-warning)"
    />
  );
}

export function Skeleton({
  width = "100%",
  height = 14,
  style,
}: {
  width?: number | string;
  height?: number | string;
  style?: React.CSSProperties;
}) {
  return <div className="skeleton" style={{ width, height, ...style }} />;
}

export function SkeletonList({ rows = 4 }: { rows?: number }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 4 }}>
      {Array.from({ length: rows }).map((_, i) => (
        <Skeleton key={i} height={16} width={`${90 - i * 8}%`} />
      ))}
    </div>
  );
}
