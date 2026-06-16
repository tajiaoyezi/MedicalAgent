// c05 写回确认网关前端客户端 + 医疗 AI 面板 P0 功能目录。
// 网关是 AI 改文档的唯一收口：技能产出 → preview（四要素/策略/风险/权限）→ 用户确认 → confirm（服务端门禁 + 落记录）→ 经 Bridge 写回。

import { api } from "./api";

// AI 操作类型（与后端 writeback.OperationType / writeback_confirmations.operation_type 一致）。
export const OP = {
  fullPolish: "全文润色",
  spanPolish: "选区润色",
  proofread: "校对",
  spanTranslate: "选区翻译",
  citation: "补引用",
  annotation: "插入标注",
  layout: "AI 论文排版",
} as const;

export type OperationType = (typeof OP)[keyof typeof OP];

export interface WritebackStrategy {
  operationType: string;
  bridgeMethod: string;
  writebackSource: string;
  impactScope: string;
  diffKind: string;
}

export interface PreviewResult {
  fourElements: {
    originalText: string;
    modifiedText: string;
    explanation: string;
    impactScope: string;
  };
  strategy: WritebackStrategy;
  risk: { riskType: string; high: boolean };
  requiresHighRiskConfirmation: boolean;
  permission: { canApply: boolean; canCopy: boolean };
  disclaimer: string;
  actions: string[]; // apply | copy | submit_review | cancel
}

export interface ConfirmResult {
  approved: boolean;
  submittedForReview?: boolean;
  confirmationId?: string;
  action?: string;
  bridgeMethod?: string;
  writebackSource?: string;
}

export function previewWriteback(input: {
  bridgeToken: string;
  operationType: string;
  originalText: string;
  modifiedText: string;
  explanation: string;
  confirmedScope?: string;
}): Promise<PreviewResult> {
  return api<PreviewResult>("/api/writeback/preview", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function confirmWriteback(input: {
  bridgeToken: string;
  operationType: string;
  action: "apply" | "copy" | "submit_review";
  originalText: string;
  modifiedText: string;
  expectedRevision: string;
  confirmedScope?: string;
}): Promise<ConfirmResult> {
  return api<ConfirmResult>("/api/writeback/confirm", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function dispatchConfirm(input: {
  subjectType: "message" | "translation_job";
  subjectId: string;
  content: string;
  action: "dispatch" | "submit_review";
}): Promise<ConfirmResult & { highRisk?: boolean }> {
  return api("/api/writeback/dispatch-confirm", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

// ── 面板 P0 功能目录（按文档类型，已剔除 §22.2 V1.1 项：论文转 PPT / AI 文档脑图 / 文档生成 PPT）──
export interface PanelSkill {
  key: string;
  label: string;
  operationType?: OperationType; // 经写回确认网关的语义写回类
  kind: "writeback" | "readonly" | "editorOp" | "initiate";
}

// docx：全套 P0（写回类经确认网关，排版类直执行不经网关）。
export const DOCX_SKILLS: PanelSkill[] = [
  { key: "fullPolish", label: "全文润色", operationType: OP.fullPolish, kind: "writeback" },
  { key: "spanPolish", label: "选区润色", operationType: OP.spanPolish, kind: "writeback" },
  { key: "proofread", label: "校对", operationType: OP.proofread, kind: "writeback" },
  { key: "layout", label: "AI 论文排版", operationType: OP.layout, kind: "writeback" },
  { key: "annotation", label: "插入标注", operationType: OP.annotation, kind: "writeback" },
  { key: "auxDisplay", label: "辅助显示", kind: "readonly" },
  { key: "outline", label: "目录 / 更新目录 / 目录级别", kind: "editorOp" },
  { key: "paging", label: "分页", kind: "editorOp" },
  { key: "headerFooter", label: "页眉页脚", kind: "editorOp" },
  { key: "paragraph", label: "段落", kind: "editorOp" },
  { key: "startAimed", label: "从当前文档发起 AIMed", kind: "initiate" },
  { key: "startTranslate", label: "从当前文档发起医学翻译", kind: "initiate" },
];

// pdf / ofd：仅 P0 子集（医学翻译 / AIMed / 批注·预览），无 docx 专属全文润色/排版/目录写回。
export const PDF_OFD_SKILLS: PanelSkill[] = [
  { key: "startAimed", label: "AIMed 学术助手", kind: "initiate" },
  { key: "startTranslate", label: "医学翻译", kind: "initiate" },
  { key: "annotation", label: "批注", operationType: OP.annotation, kind: "writeback" },
  { key: "auxDisplay", label: "预览 / 辅助显示", kind: "readonly" },
];

export function skillsForDocType(docType: string): PanelSkill[] {
  if (docType === "pdf" || docType === "ofd") return PDF_OFD_SKILLS;
  return DOCX_SKILLS;
}
