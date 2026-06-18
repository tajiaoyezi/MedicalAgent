package knowledge

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/storage"
	"medoffice/server/internal/uploadgate"
)

// BatchItemResult 批量导入逐项结果（4.3：每份文档独立入库记录/解析状态/索引状态）。
type BatchItemResult struct {
	Title        string `json:"title"`
	KBDocumentID string `json:"kbDocumentId,omitempty"`
	Error        string `json:"error,omitempty"`
}

// BatchImport 批量逐项预览导入（4.3）：对每个 ImportRequest 独立调 PreviewImport，逐项落一条 staging
// kb_documents 记录（各自独立 authorization/parse/index 状态），单项失败不影响其余项。返回 N 条结果（N=len(items)）。
// 入库前确认（ConfirmImport）与单份一致，仍走人工预览确认链路。
func BatchImport(db *gorm.DB, u auth.AuthUser, items []ImportRequest) []BatchItemResult {
	out := make([]BatchItemResult, 0, len(items))
	for _, it := range items {
		r := BatchItemResult{Title: it.Title}
		kbDocID, err := PreviewImport(db, u, it)
		if err != nil {
			r.Error = err.Error()
		} else {
			r.KBDocumentID = kbDocID
		}
		out = append(out, r)
	}
	return out
}

// screenUpload 知识库本地/批量上传入口的 c09 上传闸消费（5.2a，与出网/向量化前门禁为两个独立执行点）：
// 在内容持久化入 kb_documents/向量化前先经 c09 redaction-gateway 上传闸做 PHI/PII 识别；策略=阻止上传且命中
// 时拒绝入库并写 result=失败、failure_reason 非空的 audit_logs（脱敏命中由 c09 写 privacy_redaction_events，
// 本能力不写）。redaction-gateway owner=c09，c06 仅前置消费 uploadgate.Check 接缝。
// 注意：c09 未接入前 Check 为恒放行 stub（IsRedactionGatewayAvailable()=false），本期上传闸不实际拦截任何内容
// （与公网默认关闭 posture 一致）；c09 落地后于启动期注入真实检测实现，届时阻止策略才生效。
// 返回是否放行；不放行即拒绝该文件入库。
func screenUpload(db *gorm.DB, u auth.AuthUser, kbID, filename string, buffer []byte) (bool, string) {
	g := uploadgate.Check(filename, buffer)
	if g.Allowed {
		return true, ""
	}
	reason := g.FailureReason
	if reason == "" {
		reason = "上传内容命中 PHI/PII，按「阻止上传」策略拒绝入库"
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_upload_blocked", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID),
		Result: "失败", FailureReason: audit.P(reason),
		Metadata: map[string]any{"filename": filename, "gate": "c09_upload_gate"},
	})
	return false, reason
}

// KBUploadFile 一份待上传文件（本地/批量上传入口）。
type KBUploadFile struct {
	Filename string
	MimeType string
	Buffer   []byte
}

// KBUploadResult 单份上传结果：被上传闸阻止、或落 staging 预览记录。
type KBUploadResult struct {
	Filename     string `json:"filename"`
	KBDocumentID string `json:"kbDocumentId,omitempty"`
	Blocked      bool   `json:"blocked,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Error        string `json:"error,omitempty"`
}

// KBUpload 知识库本地/批量上传入口（4.3 + 5.2a）：对每份文件先经 c09 上传闸（screenUpload）——
// 命中「阻止上传」则拒绝入库并留痕、不落任何记录；放行则落盘 + 建 c01 documents/version，再经导入管线
// PreviewImport 落一条 staging 预览记录（逐项独立、N 份→N 条）。确认入库仍由 ConfirmImport 人工确认。
func KBUpload(ctx context.Context, db *gorm.DB, store *storage.Storage, u auth.AuthUser, kbID string, files []KBUploadFile) ([]KBUploadResult, error) {
	// 上传入口权限分级（与单份导入一致：平台管理员/库创建人/库管理员可上传到自管库）。
	can, err := CanUploadToKB(db, u, kbID)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrForbidden
	}
	out := make([]KBUploadResult, 0, len(files))
	for _, f := range files {
		// 5.2a：持久化/向量化前先过 c09 上传闸；命中阻止策略 → 拒绝入库 + 留痕，不落盘不建记录。
		if ok, reason := screenUpload(db, u, kbID, f.Filename, f.Buffer); !ok {
			out = append(out, KBUploadResult{Filename: f.Filename, Blocked: true, Reason: reason})
			continue
		}
		docID := uuid.NewString()
		verID := uuid.NewString()
		objectKey := storage.ObjectKeyForVersion(u.TenantID, docID, verID)
		if err := store.Put(ctx, objectKey, f.Buffer, f.MimeType); err != nil {
			out = append(out, KBUploadResult{Filename: f.Filename, Error: "存储失败"})
			continue
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
				VALUES (?, ?, ?, ?, 'app', 'kb', ?)`, docID, u.TenantID, u.UserID, f.Filename, nullIfBlank(f.MimeType)).Error; err != nil {
				return err
			}
			if err := tx.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
				VALUES (?, ?, ?, 1, ?, ?, 'import', ?, ?)`, verID, docID, u.TenantID, storage.ComputeFileHash(f.Buffer), u.UserID, objectKey, len(f.Buffer)).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID).Error
		}); err != nil {
			// 事务回滚后补偿删除已落盘对象，避免 MinIO 孤儿对象（与 c01/c02/c04「Put 在事务外、事务失败必补偿删除」同口径）。
			_ = store.Delete(ctx, objectKey)
			out = append(out, KBUploadResult{Filename: f.Filename, Error: "落库失败"})
			continue
		}
		// 逐项经导入管线落 staging 预览记录（4.3 N 份→N 条；确认入库由 ConfirmImport 人工确认）。
		kbDocID, err := PreviewImport(db, u, ImportRequest{
			KBID: kbID, SourceType: SrcUpload, SourceURL: "upload://" + f.Filename, Title: f.Filename, DocumentID: docID,
		})
		if err != nil {
			out = append(out, KBUploadResult{Filename: f.Filename, Error: err.Error()})
			continue
		}
		out = append(out, KBUploadResult{Filename: f.Filename, KBDocumentID: kbDocID})
	}
	return out, nil
}

func nullIfBlank(s string) any {
	if s == "" {
		return nil
	}
	return s
}
