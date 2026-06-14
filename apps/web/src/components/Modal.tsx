import type { ReactNode } from "react";
import { X } from "lucide-react";
import { Button } from "./Button";

export function Modal({
  open,
  title,
  children,
  onClose,
  footer,
  width = 480,
}: {
  open: boolean;
  title?: ReactNode;
  children: ReactNode;
  onClose: () => void;
  footer?: ReactNode;
  width?: number;
}) {
  if (!open) return null;
  return (
    <div className="modal-mask" onClick={onClose}>
      <div className="modal" style={{ maxWidth: width }} onClick={(e) => e.stopPropagation()}>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "15px 18px",
            borderBottom: "1px solid var(--color-divider)",
          }}
        >
          <div style={{ fontSize: 15, fontWeight: 600 }}>{title}</div>
          <X
            size={18}
            style={{ cursor: "pointer", color: "var(--color-text-3)" }}
            onClick={onClose}
          />
        </div>
        <div style={{ padding: 18, fontSize: 13.5, color: "var(--color-text-2)" }}>
          {children}
        </div>
        {footer !== undefined && (
          <div
            style={{
              display: "flex",
              justifyContent: "flex-end",
              gap: 10,
              padding: "12px 18px",
              borderTop: "1px solid var(--color-divider)",
            }}
          >
            {footer}
          </div>
        )}
      </div>
    </div>
  );
}

/** Convenience confirm modal */
export function ConfirmModal({
  open,
  title = "确认操作",
  message,
  confirmText = "确认",
  danger,
  onConfirm,
  onCancel,
}: {
  open: boolean;
  title?: string;
  message: ReactNode;
  confirmText?: string;
  danger?: boolean;
  onConfirm: () => void;
  onCancel: () => void;
}) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      footer={
        <>
          <Button variant="ghost" onClick={onCancel}>
            取消
          </Button>
          <Button variant={danger ? "danger" : "primary"} onClick={onConfirm}>
            {confirmText}
          </Button>
        </>
      }
    >
      {message}
    </Modal>
  );
}
