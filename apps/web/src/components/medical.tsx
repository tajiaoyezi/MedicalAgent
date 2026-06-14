import { useState, type ReactNode } from "react";
import { ShieldCheck, AlertTriangle, Info, FileText, Server, WifiOff } from "lucide-react";
import { Button } from "./Button";

/** 引用来源（由调用方 c04/c06 注入；本组件仅渲染） */
export interface CitationSource {
  title?: string;
  journal?: string;
  year?: string | number;
  pubmedId?: string;
  doi?: string;
  locator?: string; // 如「第 12 页 · 第 3 段」
}

/** 引用角标 [n] + 来源弹层 — 纯呈现，不自行检索/定位 */
export function CitationChip({ index, source }: { index: number; source?: CitationSource }) {
  const [open, setOpen] = useState(false);
  return (
    <span style={{ position: "relative", display: "inline-block" }}>
      <sup
        onClick={() => setOpen((v) => !v)}
        style={{
          cursor: "pointer",
          color: "var(--color-primary)",
          background: "var(--color-primary-softer)",
          borderRadius: 5,
          padding: "1px 5px",
          fontSize: 10.5,
          fontWeight: 700,
          marginLeft: 2,
        }}
      >
        [{index}]
      </sup>
      {open && source && (
        <div
          style={{
            position: "absolute",
            top: 20,
            left: 0,
            zIndex: 60,
            width: 280,
            background: "var(--color-surface)",
            border: "1px solid var(--color-border)",
            borderRadius: 12,
            boxShadow: "var(--shadow-lg)",
            padding: 12,
            animation: "fadeUp .15s ease both",
          }}
        >
          <div style={{ fontSize: 13, fontWeight: 600, color: "var(--color-text)" }}>
            {source.title ?? "未命名来源"}
          </div>
          <div style={{ fontSize: 11.5, color: "var(--color-text-3)", marginTop: 4, lineHeight: 1.6 }}>
            {[source.journal, source.year].filter(Boolean).join(" · ")}
            {source.pubmedId && <div>PMID: {source.pubmedId}</div>}
            {source.doi && <div>DOI: {source.doi}</div>}
            {source.locator && <div>{source.locator}</div>}
          </div>
        </div>
      )}
    </span>
  );
}

/** 高风险提示条 */
export function RiskBanner({ children }: { children: ReactNode }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 9,
        padding: "10px 13px",
        borderRadius: 10,
        background: "var(--color-danger-soft)",
        color: "var(--color-danger)",
        fontSize: 12.5,
        fontWeight: 600,
      }}
    >
      <AlertTriangle size={16} />
      {children}
    </div>
  );
}

/** 医疗免责声明 */
export function Disclaimer({
  text = "本内容由 AI 生成，仅为草稿 / 辅助建议，不构成诊断、处方或医嘱，需由医生（或授权审核角色）确认后使用。",
}: {
  text?: string;
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: 7,
        fontSize: 11.5,
        color: "var(--color-text-3)",
        lineHeight: 1.6,
      }}
    >
      <Info size={13} style={{ flexShrink: 0, marginTop: 2 }} />
      <span>{text}</span>
    </div>
  );
}

/** 合规徽标 —— 状态由 c09/c03 注入，本组件仅渲染 */
type ComplianceKind = "redaction" | "model-env" | "offline";
export function ComplianceBadge({
  kind,
  label,
  tone = "success",
}: {
  kind: ComplianceKind;
  label: string;
  tone?: "success" | "primary" | "warning";
}) {
  const fg =
    tone === "primary"
      ? "var(--color-primary)"
      : tone === "warning"
        ? "var(--color-warning)"
        : "var(--color-success)";
  const bg =
    tone === "primary"
      ? "var(--color-primary-softer)"
      : tone === "warning"
        ? "var(--color-warning-soft)"
        : "var(--color-success-soft)";
  const Icon = kind === "model-env" ? Server : kind === "offline" ? WifiOff : ShieldCheck;
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        height: 34,
        padding: "0 12px",
        borderRadius: 9,
        background: bg,
        color: fg,
        fontSize: 12.5,
        fontWeight: 600,
        whiteSpace: "nowrap",
      }}
    >
      <Icon size={15} />
      {label}
    </span>
  );
}

/** 写回确认卡呈现骨架 —— 真实 diff 与确认链路由 c05 接入；本组件仅按 props 渲染 */
export function ConfirmWritebackCard({
  original,
  revised,
  explanation,
  impact,
  onApply,
  onCopy,
  onCancel,
}: {
  original: ReactNode;
  revised: ReactNode;
  explanation: ReactNode;
  impact: ReactNode;
  onApply?: () => void;
  onCopy?: () => void;
  onCancel?: () => void;
}) {
  const row = (label: string, body: ReactNode, accent?: string) => (
    <div style={{ marginBottom: 12 }}>
      <div style={{ fontSize: 11, fontWeight: 700, letterSpacing: 0.5, color: "var(--color-text-3)", marginBottom: 4 }}>
        {label}
      </div>
      <div
        style={{
          fontSize: 13,
          color: "var(--color-text)",
          background: "var(--color-surface-2)",
          border: `1px solid ${accent ?? "var(--color-border)"}`,
          borderRadius: 9,
          padding: "9px 11px",
          lineHeight: 1.6,
        }}
      >
        {body}
      </div>
    </div>
  );
  return (
    <div
      style={{
        background: "var(--color-surface)",
        border: "1px solid var(--color-border)",
        borderRadius: 14,
        boxShadow: "var(--shadow-md)",
        padding: 16,
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 12 }}>
        <FileText size={16} style={{ color: "var(--color-primary)" }} />
        <div style={{ fontSize: 14, fontWeight: 600 }}>写回确认</div>
      </div>
      {row("原文", original)}
      {row("修改后", revised, "var(--color-primary)")}
      {row("修改说明", explanation)}
      {row("影响范围", impact)}
      <div style={{ display: "flex", gap: 8, marginTop: 14 }}>
        <Button size="sm" onClick={onApply}>
          应用到文档
        </Button>
        <Button size="sm" variant="secondary" onClick={onCopy}>
          生成副本
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>
          取消
        </Button>
      </div>
      <div style={{ marginTop: 12 }}>
        <Disclaimer />
      </div>
    </div>
  );
}
