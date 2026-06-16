package aimed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/storage"
)

// 保存范围 / 格式（§8.10.2）。
const (
	ScopeCurrent      = "current" // 当前回答
	ScopeConversation = "conversation"
	ScopeAll          = "all"

	FormatOnline   = "online"
	FormatWord     = "word"
	FormatPDF      = "pdf"
	FormatMarkdown = "markdown"
)

// LandingResult 答案落地结果。
type LandingResult struct {
	DocumentID  string `json:"documentId,omitempty"`
	VersionID   string `json:"versionId,omitempty"`
	OpenInOO    bool   `json:"openInOnlyoffice"`        // 在线/Word：经 c02 在 ONLYOFFICE 打开
	ExpandPanel bool   `json:"expandPanel,omitempty"`  // 引用 c05「打开后默认展示医疗 AI 面板」触发（owner=c05）
	Filename    string `json:"filename"`
	Format      string `json:"format"`
	ExportText  string `json:"exportText,omitempty"`   // PDF/Markdown 离线导出文本（不依赖 ONLYOFFICE）
}

// GenerateWord 生成在线 Word：渲染答案为 docx 内容 → 经 c01 文档中心创建契约落库 → 产 upload_success → 返回打开信号。
// 本能力不依赖 c02 createNewDocument 服务端新建变体，亦不直接产生 document_events（复用 c01 创建入口）。
func (s *Service) GenerateWord(db *gorm.DB, store *storage.Storage, user auth.AuthUser, conv *Conversation, messageID string) (LandingResult, error) {
	msg, err := GetMessage(db, conv.TenantID, conv.UserID, messageID)
	if err != nil {
		return LandingResult{}, err
	}
	content := msg.Content
	name := docName(conv.Title)
	docID, verID, err := createOnlineDocument(db, store, user, name, content)
	if err != nil {
		return LandingResult{}, err
	}
	_ = WriteRecentTask(db, user, conv)
	return LandingResult{DocumentID: docID, VersionID: verID, OpenInOO: true, ExpandPanel: true, Filename: name + ".docx", Format: FormatWord}, nil
}

// SaveAs 保存为：保存范围 × 格式组合（§8.10.2）。在线/Word 走 c01 创建落库；PDF/Markdown 走纯文本导出。
func (s *Service) SaveAs(db *gorm.DB, store *storage.Storage, user auth.AuthUser, conv *Conversation, scope, format, currentMessageID string) (LandingResult, error) {
	content, err := s.renderScope(db, conv, scope, currentMessageID)
	if err != nil {
		return LandingResult{}, err
	}
	name := docName(conv.Title)

	switch format {
	case FormatOnline, FormatWord:
		docID, verID, err := createOnlineDocument(db, store, user, name, content)
		if err != nil {
			return LandingResult{}, err
		}
		_ = WriteRecentTask(db, user, conv)
		ext := ".docx"
		if format == FormatOnline {
			ext = ".docx"
		}
		return LandingResult{DocumentID: docID, VersionID: verID, OpenInOO: true, ExpandPanel: true, Filename: name + ext, Format: format}, nil
	case FormatPDF, FormatMarkdown:
		// 离线降级：不依赖 ONLYOFFICE，直接返回渲染文本供前端导出
		ext := ".md"
		if format == FormatPDF {
			ext = ".pdf"
		}
		_ = WriteRecentTask(db, user, conv)
		return LandingResult{Filename: name + ext, Format: format, ExportText: content, OpenInOO: false}, nil
	default:
		return LandingResult{}, fmt.Errorf("不支持的格式：%s", format)
	}
}

// renderScope 按保存范围聚合内容（当前回答 / 当前对话 / 全部对话）。
func (s *Service) renderScope(db *gorm.DB, conv *Conversation, scope, currentMessageID string) (string, error) {
	switch scope {
	case ScopeCurrent:
		if currentMessageID == "" {
			return "", fmt.Errorf("缺少 messageId")
		}
		m, err := GetMessage(db, conv.TenantID, conv.UserID, currentMessageID)
		if err != nil {
			return "", err
		}
		return m.Content, nil
	default: // conversation / all：本期按当前对话仅未删除消息聚合（Open Question 2 暂定口径）
		msgs, err := ListMessages(db, conv.TenantID, conv.ConversationID)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		b.WriteString("# " + conv.Title + "\n\n")
		for _, m := range msgs {
			role := "用户"
			if m.Role == "assistant" {
				role = "AIMed"
			}
			b.WriteString("## " + role + "\n" + m.Content + "\n\n")
		}
		return b.String(), nil
	}
}

// createOnlineDocument 经 c01 文档中心创建契约（owner=c01）落 documents/document_versions（app_source=aimed、
// 落点「我的文档中心/应用/AIMed 学术助手/保存内容」语义经 space='app'+app_source='aimed'），
// 该首版入库产生 §10.6 upload_success（产生方=c01、消费方=c03），c03 解析→chunk→索引就绪→可检索。
func createOnlineDocument(db *gorm.DB, store *storage.Storage, user auth.AuthUser, name, content string) (documentID, versionID string, err error) {
	documentID = uuid.NewString()
	versionID = uuid.NewString()
	buffer := []byte(content)
	fileHash := storage.ComputeFileHash(buffer)
	objectKey := storage.ObjectKeyForVersion(user.TenantID, documentID, versionID)
	if perr := store.Put(context.Background(), objectKey, buffer, "text/markdown"); perr != nil {
		return "", "", perr
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec(
			`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
			 VALUES (?, ?, ?, ?, 'app', 'aimed', 'text/markdown')`,
			documentID, user.TenantID, user.UserID, name,
		).Error; e != nil {
			return e
		}
		if e := tx.Exec(
			`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			 VALUES (?, ?, ?, 1, ?, ?, 'ai_writeback', ?, ?)`,
			versionID, documentID, user.TenantID, fileHash, user.UserID, objectKey, len(buffer),
		).Error; e != nil {
			return e
		}
		if e := tx.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, documentID).Error; e != nil {
			return e
		}
		if e := tx.Exec(
			`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
			 VALUES ('upload_success', ?, ?, ?, ?::jsonb)`,
			documentID, versionID, user.TenantID, `{"source":"aimed_generate"}`,
		).Error; e != nil {
			return e
		}
		return audit.Write(tx, audit.Entry{
			TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: audit.P(strings.Join(user.RoleSlugs, ",")),
			ActionType: "aimed_generate_document", TargetType: audit.P("document"), TargetID: audit.P(documentID), Result: "成功",
			Metadata: map[string]any{"name": name},
		})
	})
	if err != nil {
		// 事务回滚后补偿删除已写入对象，避免 MinIO 孤儿对象（与 editor 写回同一口径）
		_ = store.Delete(context.Background(), objectKey)
		return "", "", err
	}
	return documentID, versionID, nil
}

// WriteRecentTask 保存成功后写最近任务（§6.4 source 规范枚举值，ref_type=conversation，幂等）。
// 展示与恢复编排归 c05，本能力仅写入条目。source 取会话自身的规范来源（aimed→AIMed 学术助手、
// kb_qa→医疗知识库问答），不硬编码，避免 kb_qa 会话经落地路径被误标为 AIMed 来源。
func WriteRecentTask(db *gorm.DB, user auth.AuthUser, conv *Conversation) error {
	taskID := uuid.NewString()
	source := conv.Source
	if source == "" {
		source = SourceAimed
	}
	return db.Exec(
		`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, ref_type, ref_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, 'conversation', ?, NOW())
		 ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
		 DO UPDATE SET title = EXCLUDED.title, updated_at = NOW(), deleted_at = NULL`,
		taskID, user.TenantID, user.UserID, source, conv.Title, conv.ConversationID,
	).Error
}

func docName(title string) string {
	if strings.TrimSpace(title) == "" {
		title = "AIMed 会话"
	}
	return time.Now().Format("20060102") + "_" + title
}
