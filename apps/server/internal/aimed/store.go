package aimed

import (
	"encoding/json"
	"errors"

	"gorm.io/gorm"
)

// 会话 module → source 规范值。
const (
	ModuleAimed = "aimed"
	ModuleKBQA  = "kb_qa"

	SourceAimed = "AIMed 学术助手"
	SourceKBQA  = "医疗知识库问答"
)

// UploadedFile 会话级文件清单项（§8.6.4 六个展示字段 + document_id 落库引用）。
type UploadedFile struct {
	FileID     string `json:"fileId"`
	Name       string `json:"name"`       // 文件名
	Type       string `json:"type"`       // 文件类型
	Size       int64  `json:"size"`       // 文件大小
	Status     string `json:"status"`     // 解析状态（五态）
	Source     string `json:"source"`     // 文件来源（§8.6.1）
	UploadedAt string `json:"uploadedAt"` // 上传时间
	DocumentID string `json:"documentId"` // 落库后的真实 document_id（§16.3 定位锚点）
	Reason     string `json:"reason,omitempty"`
}

// Conversation 会话行（覆盖 conversations 全列）。
type Conversation struct {
	ConversationID  string         `gorm:"column:conversation_id" json:"conversationId"`
	TenantID        string         `gorm:"column:tenant_id" json:"-"`
	UserID          string         `gorm:"column:user_id" json:"-"`
	Module          string         `gorm:"column:module" json:"module"`
	Source          string         `gorm:"column:source" json:"source"`
	Mode            string         `gorm:"column:mode" json:"mode"`
	Title           string         `gorm:"column:title" json:"title"`
	AllowPubmed     bool           `gorm:"column:allow_pubmed" json:"allowPubmed"`
	AllowUpload     bool           `gorm:"column:allow_upload" json:"allowUpload"`
	AllowKB         bool           `gorm:"column:allow_kb" json:"allowKb"`
	AllowCurrentDoc bool           `gorm:"column:allow_current_doc" json:"allowCurrentDoc"`
	UploadedFiles   string         `gorm:"column:uploaded_files" json:"-"`
}

// Files 解析 uploaded_files JSON。
func (c Conversation) Files() []UploadedFile {
	var fs []UploadedFile
	if c.UploadedFiles == "" {
		return fs
	}
	_ = json.Unmarshal([]byte(c.UploadedFiles), &fs)
	return fs
}

// Message 消息行。
type Message struct {
	MessageID       string  `gorm:"column:message_id" json:"messageId"`
	ConversationID  string  `gorm:"column:conversation_id" json:"conversationId"`
	TenantID        string  `gorm:"column:tenant_id" json:"-"`
	UserID          string  `gorm:"column:user_id" json:"-"`
	Role            string  `gorm:"column:role" json:"role"`
	Content         string  `gorm:"column:content" json:"content"`
	Mode            string  `gorm:"column:mode" json:"mode"`
	ParentMessageID *string `gorm:"column:parent_message_id" json:"parentMessageId"`
	Metadata        string  `gorm:"column:metadata" json:"-"`
}

var ErrNotFound = errors.New("not found")

// CreateConversation 建会话：按 module 取 source，allow_* 由 policy 派生（aimed 取六模式 policy）。
func CreateConversation(db *gorm.DB, tenantID, userID, module, mode, title string) (string, error) {
	if module == "" {
		module = ModuleAimed
	}
	source := SourceAimed
	if module == ModuleKBQA {
		source = SourceKBQA
	}
	p := GetPolicy(Mode(mode))
	if title == "" {
		title = "新会话"
	}
	var id string
	err := db.Raw(
		`INSERT INTO conversations
		   (tenant_id, user_id, module, source, mode, title, allow_pubmed, allow_upload, allow_kb, allow_current_doc, uploaded_files)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '[]'::jsonb)
		 RETURNING conversation_id`,
		tenantID, userID, module, source, mode, title,
		p.AllowPubmed, p.AllowUpload, p.AllowKB, p.AllowCurrentDoc,
	).Scan(&id).Error
	return id, err
}

// convCols 显式列（uploaded_files::text 以便 JSONB 稳定扫入 string）。
const convCols = `conversation_id, tenant_id, user_id, module, source, COALESCE(mode,'') AS mode, title,
	allow_pubmed, allow_upload, allow_kb, allow_current_doc, uploaded_files::text AS uploaded_files`

// GetConversation 取会话（按 tenant_id/user_id 隔离，软删不返回）。
func GetConversation(db *gorm.DB, tenantID, userID, convID string) (*Conversation, error) {
	var c Conversation
	err := db.Raw(
		`SELECT `+convCols+` FROM conversations WHERE conversation_id = ? AND tenant_id = ? AND user_id = ? AND deleted_at IS NULL`,
		convID, tenantID, userID,
	).Scan(&c).Error
	if err != nil {
		return nil, err
	}
	if c.ConversationID == "" {
		return nil, ErrNotFound
	}
	return &c, nil
}

// ListConversations 列会话（租户/用户隔离，可按 module 过滤；module="" 不过滤）。
func ListConversations(db *gorm.DB, tenantID, userID, module string) ([]Conversation, error) {
	sql := `SELECT ` + convCols + ` FROM conversations WHERE tenant_id = ? AND user_id = ? AND deleted_at IS NULL`
	args := []any{tenantID, userID}
	if module != "" {
		sql += ` AND module = ?`
		args = append(args, module)
	}
	sql += ` ORDER BY updated_at DESC`
	var rows []Conversation
	err := db.Raw(sql, args...).Scan(&rows).Error
	return rows, err
}

// SwitchMode 切换 AIMed 模式（§8.3）：写回 mode + allow_* 快照；clear_files_on_enter 时清空文件。
// 输入框 draft 是前端状态，本接口不触碰（强制保留输入框内容由前端保证）。
func SwitchMode(db *gorm.DB, tenantID, userID, convID string, mode Mode) (*Conversation, error) {
	conv, err := GetConversation(db, tenantID, userID, convID)
	if err != nil {
		return nil, err
	}
	p := GetPolicy(mode)
	files := "uploaded_files"
	if p.ClearFilesOnEnter {
		files = "'[]'::jsonb"
	}
	if err := db.Exec(
		`UPDATE conversations
		   SET mode = ?, allow_pubmed = ?, allow_upload = ?, allow_kb = ?, allow_current_doc = ?,
		       uploaded_files = `+files+`, updated_at = NOW()
		 WHERE conversation_id = ? AND tenant_id = ? AND user_id = ?`,
		string(mode), p.AllowPubmed, p.AllowUpload, p.AllowKB, p.AllowCurrentDoc,
		convID, tenantID, userID,
	).Error; err != nil {
		return nil, err
	}
	_ = conv
	return GetConversation(db, tenantID, userID, convID)
}

// SetUploadedFiles 覆写会话文件清单。
func SetUploadedFiles(db *gorm.DB, tenantID, userID, convID string, files []UploadedFile) error {
	b, _ := json.Marshal(files)
	return db.Exec(
		`UPDATE conversations SET uploaded_files = ?::jsonb, updated_at = NOW()
		 WHERE conversation_id = ? AND tenant_id = ? AND user_id = ?`,
		string(b), convID, tenantID, userID,
	).Error
}

// AddMessage 追加消息（用户输入或答案版本各一条）。
func AddMessage(db *gorm.DB, tenantID, userID, convID, role, content, mode string, parent *string, metadata map[string]any) (string, error) {
	mb := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			mb = string(b)
		}
	}
	var id string
	err := db.Raw(
		`INSERT INTO messages (conversation_id, tenant_id, user_id, role, content, mode, parent_message_id, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?::jsonb) RETURNING message_id`,
		convID, tenantID, userID, role, content, mode, parent, mb,
	).Scan(&id).Error
	if err == nil {
		_ = db.Exec(`UPDATE conversations SET updated_at = NOW() WHERE conversation_id = ?`, convID).Error
	}
	return id, err
}

// msgCols 显式列（metadata::text 稳定扫入 string；mode 可空 COALESCE）。
const msgCols = `message_id, conversation_id, tenant_id, user_id, role, content,
	COALESCE(mode,'') AS mode, parent_message_id, metadata::text AS metadata`

// ListMessages 列会话消息（按时间，软删不返回，租户隔离）。
func ListMessages(db *gorm.DB, tenantID, convID string) ([]Message, error) {
	var rows []Message
	err := db.Raw(
		`SELECT `+msgCols+` FROM messages WHERE conversation_id = ? AND tenant_id = ? AND deleted_at IS NULL ORDER BY created_at ASC`,
		convID, tenantID,
	).Scan(&rows).Error
	return rows, err
}

// GetMessage 取单条消息（租户 + 用户隔离）。messages 是 per-user 资源（user_id NOT NULL），
// 仅按 tenant 取会让同租户他人消息可被反馈/读引用/落地，故与会话访问器一致强制 user_id。
func GetMessage(db *gorm.DB, tenantID, userID, messageID string) (*Message, error) {
	var m Message
	err := db.Raw(`SELECT `+msgCols+` FROM messages WHERE message_id = ? AND tenant_id = ? AND user_id = ? AND deleted_at IS NULL`, messageID, tenantID, userID).Scan(&m).Error
	if err != nil {
		return nil, err
	}
	if m.MessageID == "" {
		return nil, ErrNotFound
	}
	return &m, nil
}

// DeleteMessage 软删消息（§8.10 删除二次确认后同步对话内容）。
func DeleteMessage(db *gorm.DB, tenantID, userID, messageID string) (bool, error) {
	res := db.Exec(
		`UPDATE messages SET deleted_at = NOW()
		 WHERE message_id = ? AND tenant_id = ? AND user_id = ? AND deleted_at IS NULL`,
		messageID, tenantID, userID,
	)
	return res.RowsAffected > 0, res.Error
}
