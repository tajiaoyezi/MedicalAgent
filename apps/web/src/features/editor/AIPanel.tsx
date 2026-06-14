import { useState } from "react";
import { Button } from "../../components";
import type { MedOfficeBridge } from "../../lib/bridge-sdk";

interface Props {
  bridge: MedOfficeBridge | null;
  initialCommand?: string;
  onClose: () => void;
}

export default function AIPanel({ bridge, initialCommand, onClose }: Props) {
  const [selection, setSelection] = useState("");
  const [preview, setPreview] = useState("");
  const [message, setMessage] = useState("");
  const [confirming, setConfirming] = useState(false);

  async function readSelection() {
    if (!bridge) {
      setMessage("编辑器 Bridge 未就绪");
      return;
    }
    try {
      const res = await bridge.getSelectedText();
      setSelection(res.data?.text ?? "");
      setMessage("已读取选区");
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "读取失败");
    }
  }

  async function simulatePolish() {
    if (!selection) {
      setMessage("请先读取选区");
      return;
    }
    const modified = selection.replace(/\s+/g, " ").trim() + "（润色预览）";
    setPreview(modified);
    setConfirming(true);
    if (bridge) {
      await bridge.getConfirmPreview(selection, modified);
    }
  }

  async function applyWriteback() {
    if (!bridge || !preview) return;
    try {
      await bridge.replaceSelection(preview, selection);
      await bridge.saveDocument("ai_writeback");
      setMessage("已触发保存，落库后将生成 ai_writeback 新版本");
      setConfirming(false);
    } catch (e) {
      setMessage(e instanceof Error ? e.message : "写回失败");
    }
  }

  return (
    <aside
      style={{
        width: 360,
        borderLeft: "1px solid var(--color-border)",
        background: "var(--color-surface)",
        display: "flex",
        flexDirection: "column",
        padding: 16,
        gap: 12,
      }}
    >
      <div style={{ display: "flex", justifyContent: "space-between" }}>
        <strong>医疗 AI 面板</strong>
        <button className="btn btn-sm btn-ghost" onClick={onClose}>
          关闭
        </button>
      </div>

      {initialCommand && (
        <p style={{ fontSize: 12, color: "var(--color-text-3)" }}>
          预置命令：{initialCommand}
        </p>
      )}

      <Button variant="secondary" size="sm" onClick={readSelection}>
        读取当前选区
      </Button>

      <Button variant="secondary" size="sm" onClick={simulatePolish}>
        润色预览（占位）
      </Button>

      {selection && (
        <div style={{ fontSize: 12 }}>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>原文</div>
          <div
            style={{
              padding: 8,
              background: "var(--color-surface-2)",
              borderRadius: 6,
              maxHeight: 80,
              overflow: "auto",
            }}
          >
            {selection}
          </div>
        </div>
      )}

      {confirming && preview && (
        <div style={{ fontSize: 12 }}>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>修改后（预览，未落盘）</div>
          <div
            style={{
              padding: 8,
              background: "var(--color-primary-softer)",
              borderRadius: 6,
              maxHeight: 80,
              overflow: "auto",
            }}
          >
            {preview}
          </div>
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <Button variant="primary" size="sm" onClick={applyWriteback}>
              应用到文档
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setConfirming(false);
                setPreview("");
              }}
            >
              取消
            </Button>
          </div>
        </div>
      )}

      {message && (
        <p style={{ fontSize: 12, color: "var(--color-text-2)" }}>{message}</p>
      )}
    </aside>
  );
}
