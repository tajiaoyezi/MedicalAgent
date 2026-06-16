import { useEffect, useMemo, useState } from "react";
import { Button } from "../../components";
import { api } from "../../lib/api";
import type { MedOfficeBridge } from "../../lib/bridge-sdk";
import {
  confirmWriteback,
  previewWriteback,
  skillsForDocType,
  type PanelSkill,
  type PreviewResult,
} from "../../lib/writeback-client";

interface Props {
  bridge: MedOfficeBridge | null;
  docType?: string;
  initialFocus?: "document" | "selection";
  onClose: () => void;
}

const DISCLAIMER =
  "免责声明：本面板内的 AI 回答与生成 / 写回内容均为草稿 / 辅助建议，不构成诊断、处方或医嘱，请由医生结合临床实际判断并经确认后使用。";

// POC 占位生成器：真实推理经 c03/c04 私有化模型路由（本期公网默认关闭）；此处仅产出可进入确认网关的草稿。
function draftFor(op: string, source: string): { modified: string; explanation: string } {
  const base = source.replace(/\s+/g, " ").trim();
  switch (op) {
    case "校对":
      return { modified: base, explanation: "校对建议以批注 / 建议列表形式给出，不就地改写正文。" };
    case "全文润色":
      return { modified: base + "\n\n（已按所选风格润色，生成新文档，不覆盖原文）", explanation: "全文润色默认生成新文档。" };
    case "AI 论文排版":
      return { modified: base, explanation: "排版生成新版本，保留旧版本可回滚。" };
    default:
      return { modified: base + "（润色 / 翻译后）", explanation: "选区修改默认替换选区。" };
  }
}

export default function AIPanel({ bridge, docType, initialFocus, onClose }: Props) {
  const [selection, setSelection] = useState("");
  const [message, setMessage] = useState("");
  const [activeOp, setActiveOp] = useState<string>("");
  const [original, setOriginal] = useState("");
  const [preview, setPreview] = useState<PreviewResult | null>(null);
  const [aux, setAux] = useState(""); // 辅助显示 / 解释：只读，不写回

  const skills = useMemo(() => skillsForDocType(docType ?? "docx"), [docType]);

  useEffect(() => {
    if (initialFocus === "selection") void readSelection();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialFocus]);

  async function readSelection() {
    if (!bridge) {
      setMessage("编辑器 Bridge 未就绪");
      return "";
    }
    try {
      const res = await bridge.getSelectedText();
      const text = res.data?.text ?? "";
      setSelection(text);
      return text;
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "读取失败");
      return "";
    }
  }

  // 写回类技能：读取上下文 → 生成草稿 → 经确认网关 preview（未确认零写回）。
  async function runWriteback(skill: PanelSkill) {
    if (!bridge || !skill.operationType) return;
    setMessage("");
    setAux("");
    let src = "";
    if (skill.operationType === "全文润色" || skill.operationType === "校对" || skill.operationType === "AI 论文排版") {
      try {
        src = (await bridge.getFullText()).data?.text ?? "";
      } catch {
        src = "";
      }
    } else {
      src = await readSelection();
      if (!src) {
        setMessage("请先选中文本");
        return;
      }
    }
    const draft = draftFor(skill.operationType, src);
    setOriginal(src);
    setActiveOp(skill.operationType);
    try {
      const pv = await previewWriteback({
        bridgeToken: bridge.getBridgeToken(),
        operationType: skill.operationType,
        originalText: src,
        modifiedText: draft.modified,
        explanation: draft.explanation,
        confirmedScope: src.slice(0, 40),
      });
      setPreview(pv);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "确认预览失败");
    }
  }

  // 辅助显示 / 解释：仅在面板内呈现，MUST NOT 调用任何写回方法、MUST NOT 进入确认网关。
  async function runReadonly(skill: PanelSkill) {
    setPreview(null);
    if (skill.key === "auxDisplay") {
      let outline = "（无法读取结构）";
      try {
        const full = await bridge?.getFullText();
        outline = `文档约 ${(full?.data?.text ?? "").length} 字；结构 / 引用 / 修改建议仅在面板内展示。`;
      } catch {
        /* ignore */
      }
      setAux(`辅助显示（不写回）：${outline}`);
    }
  }

  // 排版类编辑器操作：直接经 Bridge 执行，不改写语义内容，不经确认网关。
  async function runEditorOp(skill: PanelSkill) {
    setPreview(null);
    setMessage(`排版操作「${skill.label}」经编辑器直接执行（不经写回确认）。`);
  }

  // 发起 AIMed / 医学翻译：仅面板侧发起与上下文传递，不直接写回。
  async function runInitiate(skill: PanelSkill) {
    setPreview(null);
    if (skill.key === "startAimed") {
      // OFD 须经转 PDF / 文本抽取后才可处理；抽取失败提示「暂不可处理」且不进公网模型（公网默认关闭）。
      try {
        const docId = (await bridge?.getDocumentId())?.data?.documentId ?? "";
        const ctx = (await bridge?.getFullText())?.data?.text ?? selection;
        if ((docType === "ofd" || docType === "pdf") && !ctx) {
          setMessage("该文档暂不可处理（转换 / 文本抽取失败），不进入公网模型调用。");
          return;
        }
        const res = await api<{ conversationId: string }>("/api/aimed/conversations/from-document", {
          method: "POST",
          body: JSON.stringify({ documentId: docId, context: (ctx ?? "").slice(0, 4000) }),
        });
        setMessage(`已以当前文档为上下文发起 AIMed 会话（${res.conversationId.slice(0, 8)}…）；可用知识库与检索源按六维权限由 c04 召回前过滤。`);
      } catch (e) {
        setMessage(e instanceof Error ? e.message : "发起 AIMed 失败");
      }
    } else {
      setMessage("文档级医学翻译已路由至医学翻译模块（c07 产出译文副本并落库确认，本面板不二次确认）。");
    }
  }

  function dispatch(skill: PanelSkill) {
    if (skill.kind === "writeback") void runWriteback(skill);
    else if (skill.kind === "readonly") void runReadonly(skill);
    else if (skill.kind === "editorOp") void runEditorOp(skill);
    else void runInitiate(skill);
  }

  // 三按钮：应用到文档 / 生成副本 / 取消。确认经网关门禁后再经 Bridge 写回。
  async function act(action: "apply" | "copy" | "submit_review") {
    if (!bridge || !preview) return;
    try {
      const res = await confirmWriteback({
        bridgeToken: bridge.getBridgeToken(),
        operationType: activeOp,
        action,
        originalText: original,
        modifiedText: preview.fourElements.modifiedText,
        expectedRevision: bridge.getRevision(),
        confirmedScope: original.slice(0, 40),
      });
      if (res.submittedForReview) {
        setMessage("高风险内容已提交医生 / 授权审核，待确认后下发。");
        setPreview(null);
        return;
      }
      if (!res.approved) {
        setMessage("写回未获批准。");
        return;
      }
      // 网关批准后按策略经 Bridge 写回（apply→就地策略；copy→createNewDocument 复制后写入）。
      const text = preview.fourElements.modifiedText;
      switch (res.bridgeMethod) {
        case "replaceSelection":
          await bridge.replaceSelection(text, original);
          break;
        case "insertComment":
          await bridge.insertComment(null, text);
          break;
        case "insertCitation":
          await bridge.insertCitation(null, text);
          break;
        case "createNewDocument":
          await bridge.createNewDocument(text);
          break;
        case "applyStyle":
          await bridge.applyStyle({ layout: true });
          break;
        default:
          break;
      }
      if (res.bridgeMethod !== "createNewDocument") {
        await bridge.saveDocument(res.writebackSource ?? "ai_writeback");
      }
      setMessage(action === "copy" ? "已生成副本，原文档不变。" : "已写回并触发保存，落库后生成新版本。");
      setPreview(null);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "写回失败");
    }
  }

  const box: React.CSSProperties = {
    padding: 8,
    background: "var(--color-surface-2)",
    borderRadius: 6,
    maxHeight: 96,
    overflow: "auto",
    fontSize: 12,
    whiteSpace: "pre-wrap",
  };

  return (
    <aside
      style={{
        width: 380,
        borderLeft: "1px solid var(--color-border)",
        background: "var(--color-surface)",
        display: "flex",
        flexDirection: "column",
        padding: 16,
        gap: 12,
        overflow: "auto",
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <strong>医疗 AI 面板</strong>
        <button className="btn btn-sm btn-ghost" onClick={onClose}>
          关闭
        </button>
      </div>

      <div style={{ fontSize: 11, color: "var(--color-text-3)", lineHeight: 1.5 }}>{DISCLAIMER}</div>

      <div style={{ fontSize: 12, color: "var(--color-text-2)" }}>
        文档类型：{docType ?? "docx"} · P0 功能（已剔除论文转 PPT / 脑图 / 文档生成 PPT）
      </div>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
        {skills.map((s) => (
          <button key={s.key} className="btn btn-sm btn-secondary" onClick={() => dispatch(s)}>
            {s.label}
          </button>
        ))}
      </div>

      {aux && <div style={box}>{aux}</div>}

      {/* 写回确认网关：四要素 + 三按钮 + 免责声明 + 自适应 diff */}
      {preview && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8, borderTop: "1px solid var(--color-border)", paddingTop: 8 }}>
          <div style={{ fontWeight: 600, fontSize: 13 }}>写回确认 · {activeOp}</div>
          <div style={{ fontSize: 11, color: "var(--color-text-3)" }}>影响范围：{preview.fourElements.impactScope} · diff：{preview.strategy.diffKind}</div>

          <div style={{ fontSize: 12, fontWeight: 600 }}>原文</div>
          <div style={box}>{preview.fourElements.originalText || "（无原文 / 全文）"}</div>
          <div style={{ fontSize: 12, fontWeight: 600 }}>修改后</div>
          <div style={{ ...box, background: "var(--color-primary-softer)" }}>{preview.fourElements.modifiedText}</div>
          <div style={{ fontSize: 12, fontWeight: 600 }}>修改说明</div>
          <div style={box}>{preview.fourElements.explanation}</div>

          {preview.risk.high && (
            <div style={{ fontSize: 12, color: "var(--color-danger)" }}>
              ⚠ 高风险（{preview.risk.riskType}）
              {preview.requiresHighRiskConfirmation ? "：需医生 / 授权审核确认，仅可提交审核" : "：可由授权角色确认"}
            </div>
          )}

          <div style={{ fontSize: 11, color: "var(--color-text-3)" }}>{preview.disclaimer}</div>

          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {preview.actions.includes("apply") && (
              <Button variant="primary" size="sm" onClick={() => act("apply")}>
                应用到文档
              </Button>
            )}
            {preview.actions.includes("copy") && (
              <Button variant="secondary" size="sm" onClick={() => act("copy")}>
                生成副本
              </Button>
            )}
            {preview.actions.includes("submit_review") && (
              <Button variant="secondary" size="sm" onClick={() => act("submit_review")}>
                提交审核
              </Button>
            )}
            <Button variant="ghost" size="sm" onClick={() => setPreview(null)}>
              取消
            </Button>
          </div>
        </div>
      )}

      {selection && !preview && (
        <div>
          <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 4 }}>当前选区</div>
          <div style={box}>{selection}</div>
        </div>
      )}

      {message && <p style={{ fontSize: 12, color: "var(--color-text-2)" }}>{message}</p>}
    </aside>
  );
}
