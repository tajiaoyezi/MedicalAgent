package aimed

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/storage"
	"medoffice/server/internal/uploadgate"
)

// §8.6.3 上传限制。
const (
	MaxFilesPerConversation = 10
	MaxFileBytes            = 100 * 1024 * 1024 // 100MB
)

// §8.6.2 支持格式白名单。OFD 仅转换/预览/解析/问答/翻译，不支持在线编辑（编辑能力由 c02 控制，本期落库不受影响）。
var allowedExts = map[string]bool{
	".pdf": true, ".ofd": true, ".doc": true, ".docx": true, ".xlsx": true,
	".xls": true, ".ppt": true, ".pptx": true, ".png": true, ".jpg": true,
}

// §8.6.5 异常提示文案（与 PRD 逐字一致）。
const (
	MsgFormatUnsupported = "文件类型支持：pdf / ofd / doc / docx / xlsx / xls / ppt / pptx / png / jpg"
	MsgFileTooLarge      = "所选文件中存在超过 100MB 的文件，已自动去除"
	MsgTooManyFiles      = "一次最多上传 10 个文件"
	MsgParseFailed       = "文件解析失败，可移除后重新上传"
	MsgFileDeleted       = "该文件已删除，无法继续作为上下文使用"
	MsgEncrypted         = "暂不支持加密文件，请解除加密后重试"
	MsgPHIBlocked        = "文件包含敏感信息（PHI/PII），按策略已阻止上传"
)

// CheckUploadConstraints 校验单文件可否加入会话（数量/大小/格式/加密）。
func CheckUploadConstraints(conv *Conversation, filename string, size int64, buffer []byte) (ok bool, reason string) {
	if activeFileCount(conv) >= MaxFilesPerConversation {
		return false, MsgTooManyFiles
	}
	if size > MaxFileBytes {
		return false, MsgFileTooLarge
	}
	if !allowedExts[ext(filename)] {
		return false, MsgFormatUnsupported
	}
	if isEncrypted(filename, buffer) {
		return false, MsgEncrypted
	}
	return true, ""
}

func activeFileCount(conv *Conversation) int {
	n := 0
	for _, f := range conv.Files() {
		if f.Status != FileDeleted {
			n++
		}
	}
	return n
}

// IngestFile 经 c09 上传时 PHI 门禁后，调 c01 文档中心落库（documents/document_versions）+ upload_success 事件，
// 返回会话级文件清单项（状态=解析中，含真实 document_id）。c03 异步消费 upload_success 完成解析→chunk→索引就绪。
func IngestFile(db *gorm.DB, store *storage.Storage, user auth.AuthUser, filename, mimetype string, buffer []byte, fileSource string) (UploadedFile, error) {
	// §19.4 上传时 PHI/PII 识别门禁（owner=c09，本能力仅消费判定）
	if g := uploadgate.Check(filename, buffer); !g.Allowed {
		_ = audit.Write(db, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: audit.P(strings.Join(user.RoleSlugs, ",")),
			ActionType: "aimed_file_upload", Result: "失败", FailureReason: audit.P(MsgPHIBlocked),
		})
		return UploadedFile{}, &UploadError{Reason: MsgPHIBlocked}
	}

	documentID := uuid.NewString()
	versionID := uuid.NewString()
	fileHash := storage.ComputeFileHash(buffer)
	objectKey := storage.ObjectKeyForVersion(user.TenantID, documentID, versionID)
	if err := store.Put(context.Background(), objectKey, buffer, mimetype); err != nil {
		return UploadedFile{}, err
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
			 VALUES (?, ?, ?, ?, 'app', 'aimed', ?)`,
			documentID, user.TenantID, user.UserID, filename, nullIfEmptyMime(mimetype),
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			 VALUES (?, ?, ?, 1, ?, ?, 'import', ?, ?)`,
			versionID, documentID, user.TenantID, fileHash, user.UserID, objectKey, len(buffer),
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, documentID).Error; err != nil {
			return err
		}
		// upload_success 事件唯一产生方=c01 文档中心入口（本能力复用同一入库事件，c03 消费）
		if err := tx.Exec(
			`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
			 VALUES ('upload_success', ?, ?, ?, ?::jsonb)`,
			documentID, versionID, user.TenantID, `{"source":"aimed_upload"}`,
		).Error; err != nil {
			return err
		}
		return audit.Write(tx, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: audit.P(strings.Join(user.RoleSlugs, ",")),
			ActionType: "aimed_file_upload", TargetType: audit.P("document"), TargetID: audit.P(documentID), Result: "成功",
			Metadata: map[string]any{"filename": filename, "source": fileSource},
		})
	})
	if err != nil {
		// 事务回滚后补偿删除已写入对象，避免 MinIO 孤儿对象（与 editor 写回同一口径）
		_ = store.Delete(context.Background(), objectKey)
		return UploadedFile{}, err
	}

	return UploadedFile{
		FileID: documentID, Name: filename, Type: ext(filename), Size: int64(len(buffer)),
		Status: FileParsing, Source: fileSource, UploadedAt: time.Now().Format(time.RFC3339), DocumentID: documentID,
	}, nil
}

// UploadError 业务错误（携带前端可展示原因）。
type UploadError struct{ Reason string }

func (e *UploadError) Error() string { return e.Reason }

// isEncrypted 轻量加密检测：PDF /Encrypt 标记 或 MS-Office 加密容器（CFBF/ECMA-376 magic）。
func isEncrypted(filename string, buffer []byte) bool {
	if bytes.Contains(buffer, []byte("/Encrypt")) {
		return true
	}
	// ECMA-376 加密包以 OLE 复合文档 magic D0CF11E0 开头（加密的 docx/xlsx/pptx）
	if len(buffer) >= 8 && bytes.HasPrefix(buffer, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		low := strings.ToLower(filename)
		if strings.HasSuffix(low, ".docx") || strings.HasSuffix(low, ".xlsx") || strings.HasSuffix(low, ".pptx") {
			return true
		}
	}
	return false
}

func ext(filename string) string {
	i := strings.LastIndexByte(filename, '.')
	if i < 0 {
		return ""
	}
	return strings.ToLower(filename[i:])
}

func nullIfEmptyMime(m string) any {
	if m == "" {
		return nil
	}
	return m
}
