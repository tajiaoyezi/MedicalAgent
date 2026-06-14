package parsing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/model"
	"medoffice/server/internal/storage"
)

// ParseJob：单个解析作业的标识（消费方扫描 + 流水线执行）。
type ParseJob struct {
	JobID           string `gorm:"column:job_id"`
	TenantID        string `gorm:"column:tenant_id"`
	DocumentID      string `gorm:"column:document_id"`
	DocumentVersion int    `gorm:"column:document_version"`
}

// Engine 持有解析所需的对象存储；模型/视觉经各自的包函数。
type Engine struct {
	store *storage.Storage
}

func NewEngine(store *storage.Storage) *Engine { return &Engine{store: store} }

var textExt = map[string]bool{"txt": true, "md": true, "markdown": true, "html": true, "htm": true, "csv": true, "json": true, "log": true}

func extensionOf(filename string) string {
	dot := strings.LastIndex(filename, ".")
	if dot < 0 {
		return ""
	}
	return strings.ToLower(filename[dot+1:])
}

func detectParsePath(filename string, mime *string) string {
	if mime != nil && strings.HasPrefix(*mime, "text/") {
		return "direct_text"
	}
	if textExt[extensionOf(filename)] {
		return "direct_text"
	}
	return "visual"
}

func setJob(db *gorm.DB, jobID string, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	return db.Table("document_parse_jobs").Where("job_id = ?", jobID).Updates(fields).Error
}

func (e *Engine) failJob(db *gorm.DB, job ParseJob, reason string) {
	_ = setJob(db, job.JobID, map[string]any{"status": "failed", "substatus": nil, "failure_reason": reason, "completed_at": time.Now()})
	_ = audit.Write(db, audit.Entry{
		TenantID: job.TenantID, ActionType: "parse_job", TargetType: audit.P("document"), TargetID: audit.P(job.DocumentID),
		Result: "失败", FailureReason: audit.P(reason),
		Metadata: map[string]any{"jobId": job.JobID, "documentVersion": job.DocumentVersion, "status": "failed"},
	})
}

// RunParseJob 运行单作业；内部捕获所有异常并落 failed，不向外抛（供 worker 循环安全调用）。
// 顶层 recover 兜底意外 panic（如 nil 解引用），使单作业失败不中断整轮 tick。
func (e *Engine) RunParseJob(db *gorm.DB, job ParseJob) (result string) {
	defer func() {
		if r := recover(); r != nil {
			func() {
				defer func() { _ = recover() }()
				e.failJob(db, job, fmt.Sprintf("解析异常(panic)：%v", r))
			}()
			result = "failed"
		}
	}()
	res, err := e.runInner(db, job)
	if err != nil {
		reason := "解析异常：" + err.Error()
		var vpm *VisualProviderMissingError
		var vpf *VisualParseFailedError
		if errors.As(err, &vpm) {
			reason = vpm.Msg
		} else if errors.As(err, &vpf) {
			reason = vpf.Msg
		}
		func() {
			defer func() { _ = recover() }()
			e.failJob(db, job, reason)
		}()
		return "failed"
	}
	return res
}

func (e *Engine) runInner(db *gorm.DB, job ParseJob) (string, error) {
	if err := setJob(db, job.JobID, map[string]any{"status": "parsing", "substatus": "detecting", "started_at": time.Now()}); err != nil {
		return "", err
	}

	var ver struct {
		ObjectKey string  `gorm:"column:object_key"`
		Name      string  `gorm:"column:name"`
		MimeType  *string `gorm:"column:mime_type"`
	}
	_ = db.Raw(
		`SELECT dv.object_key, d.name, d.mime_type
		 FROM document_versions dv JOIN documents d ON d.document_id = dv.document_id
		 WHERE dv.document_id = ? AND dv.document_version = ? AND dv.tenant_id = ?`,
		job.DocumentID, job.DocumentVersion, job.TenantID,
	).Scan(&ver).Error
	if ver.ObjectKey == "" {
		e.failJob(db, job, "文档版本不存在")
		return "failed", nil
	}

	path := detectParsePath(ver.Name, ver.MimeType)
	var segments []TextSegment
	sourceType := "document"

	if path == "direct_text" {
		buf, err := e.store.Get(context.Background(), ver.ObjectKey)
		if err != nil {
			return "", err
		}
		segments = ChunkPlainText(string(buf), 0)
	} else {
		_ = setJob(db, job.JobID, map[string]any{"substatus": "visual_parsing"})
		mime := "application/octet-stream"
		if ver.MimeType != nil && *ver.MimeType != "" {
			mime = *ver.MimeType
		}
		jobID := job.JobID
		visual, err := RunVisualParse(db, VisualParseTarget{
			TenantID: job.TenantID, DocumentID: job.DocumentID, DocumentVersion: job.DocumentVersion,
			ObjectKey: ver.ObjectKey, Filename: ver.Name, Mime: mime, JobID: &jobID,
		})
		if err != nil {
			return "", err
		}
		segments = ChunkFromVisual(visual)
	}

	_ = setJob(db, job.JobID, map[string]any{"substatus": "chunking"})
	if len(segments) == 0 {
		e.failJob(db, job, "未抽取到可切分文本")
		return "failed", nil
	}

	// chunk_acl：默认继承文档级 ACL
	var aclRows []map[string]any
	_ = db.Raw(`SELECT principal_type, principal_id, permission_level FROM document_permissions WHERE document_id = ?`, job.DocumentID).Scan(&aclRows).Error
	if aclRows == nil {
		aclRows = []map[string]any{}
	}
	chunkACL := mustJSON(map[string]any{"inheritedFrom": "document", "entries": aclRows})

	_ = setJob(db, job.JobID, map[string]any{"substatus": "embedding"})
	inputs := make([]string, len(segments))
	for i, s := range segments {
		inputs[i] = s.Text
	}
	embedRes, err := model.InvokeEmbed(db, model.EmbedRequest{Input: inputs}, model.InvokeContext{TenantID: job.TenantID})
	if err != nil {
		return "", err
	}
	if len(embedRes.Vectors) != len(segments) {
		e.failJob(db, job, "embedding 数量与 chunk 数量不一致")
		return "failed", nil
	}

	// 最终事务：supersede 旧版本 chunk + 写 chunk/embedding + 终态。失败整体回滚不残留半成品。
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`UPDATE document_chunks SET superseded = TRUE WHERE document_id = ? AND superseded = FALSE AND document_version <= ?`,
			job.DocumentID, job.DocumentVersion,
		).Error; err != nil {
			return err
		}
		for i, seg := range segments {
			var chunkID string
			if err := tx.Raw(
				`INSERT INTO document_chunks
				   (tenant_id, document_id, document_version, source_type, source_title, source_url, pubmed_id, doi, journal, year,
				    section, page, paragraph_index, chunk_text, chunk_acl, superseded)
				 VALUES (?,?,?,?,?,NULL,NULL,NULL,NULL,NULL,?,?,?,?,?::jsonb,FALSE) RETURNING id`,
				job.TenantID, job.DocumentID, job.DocumentVersion, sourceType, ver.Name,
				seg.Section, seg.Page, seg.ParagraphIndex, seg.Text, chunkACL,
			).Scan(&chunkID).Error; err != nil {
				return err
			}
			if err := tx.Exec(
				`INSERT INTO embeddings (chunk_id, vector, model, dim) VALUES (?, ?::jsonb, ?, ?)`,
				chunkID, mustJSON(embedRes.Vectors[i]), embedRes.Model, embedRes.Dim,
			).Error; err != nil {
				return err
			}
		}
		now := time.Now()
		return setJob(tx, job.JobID, map[string]any{
			"substatus": "indexing_handoff", "status": "succeeded",
			"index_ready_at": now, "completed_at": now, "failure_reason": nil,
		})
	})
	if txErr != nil {
		return "", txErr
	}

	_ = audit.Write(db, audit.Entry{
		TenantID: job.TenantID, ActionType: "parse_job", TargetType: audit.P("document"), TargetID: audit.P(job.DocumentID),
		Result:   "成功",
		Metadata: map[string]any{"jobId": job.JobID, "documentVersion": job.DocumentVersion, "status": "succeeded", "path": path, "chunkCount": len(segments)},
	})

	emitIndexReady(IndexReadyEvent{
		TenantID: job.TenantID, DocumentID: job.DocumentID, DocumentVersion: job.DocumentVersion,
		JobID: job.JobID, ChunkCount: len(segments),
	})
	return "succeeded", nil
}
