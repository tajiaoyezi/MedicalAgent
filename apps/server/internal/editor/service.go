package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/config"
	"medoffice/server/internal/storage"
)

// Service 聚合 c02 桥的有状态依赖：内存会话、JWT、回调指标、对象存储、配置。
type Service struct {
	cfg      config.OnlyOffice
	store    *storage.Storage
	Sessions *SessionStore
	JWT      *JWT
	Metrics  *Metrics
}

func NewService(cfg config.OnlyOffice, store *storage.Storage) *Service {
	return &Service{
		cfg:      cfg,
		store:    store,
		Sessions: NewSessionStore(cfg),
		JWT:      NewJWT(cfg.JWTSecret, cfg.JWTEnabled),
		Metrics:  NewMetrics(),
	}
}

func (s *Service) Config() config.OnlyOffice { return s.cfg }

// Forcesave 经 DS 命令服务触发强制保存（产生 status=6 forcesave 回调）。
// 用途：写回意图 arm 之后触发，使保存回调走 ai_writeback 分支落版本（Api.Save 仅产生 status=2 user_edit，
// 故插件侧不再 Api.Save，由本方法在已 arm 时统一触发 forcesave）。error=0 成功、error=4 文档无改动，其余失败。
func (s *Service) Forcesave(docKey string) error {
	payload := map[string]any{"c": "forcesave", "key": docKey}
	body := map[string]any{"c": "forcesave", "key": docKey}
	if s.JWT.Enabled() {
		body["token"] = s.JWT.Sign(payload)
	}
	b, _ := json.Marshal(body)
	endpoint := strings.TrimRight(s.cfg.DSURL, "/") + "/coauthoring/CommandService.ashx"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.JWT.Enabled() {
		req.Header.Set("Authorization", "Bearer "+s.JWT.Sign(payload))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out struct {
		Error int `json:"error"`
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	if out.Error != 0 && out.Error != 4 {
		return fmt.Errorf("forcesave 命令失败: error=%d", out.Error)
	}
	return nil
}

type CallbackBody struct {
	Key    string
	Status int
	URL    string
}

// ParseCallback 解析回调体；JWT 启用时要求 body.token 并以验签 claims 覆盖明文字段。ok=false → 应答 403{error:1}。
func ParseCallback(raw []byte, j *JWT) (CallbackBody, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	if j.Enabled() {
		tok, _ := m["token"].(string)
		if tok == "" {
			return CallbackBody{}, false
		}
		verified, ok := j.Verify(tok)
		if !ok {
			return CallbackBody{}, false
		}
		for k, v := range verified {
			m[k] = v
		}
	}
	cb := CallbackBody{}
	if v, ok := m["key"].(string); ok {
		cb.Key = v
	}
	if v, ok := m["url"].(string); ok {
		cb.URL = v
	}
	if v, ok := m["status"].(float64); ok {
		cb.Status = int(v)
	}
	return cb, true
}

// AssertDsDownloadURL 复刻 assertDsDownloadUrl：回调下载 URL 须与 DS host+port 同源。
func AssertDsDownloadURL(dsURL, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return errors.New("回调下载 URL 无效")
	}
	dsBase, err := url.Parse(dsURL)
	if err != nil {
		return errors.New("回调下载 URL 无效")
	}
	if parsed.Hostname() != dsBase.Hostname() {
		return fmt.Errorf("回调下载 URL 主机不匹配: %s", parsed.Hostname())
	}
	if portOf(parsed) != portOf(dsBase) {
		return fmt.Errorf("回调下载 URL 端口不匹配: %s", portOf(parsed))
	}
	return nil
}

func portOf(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func (s *Service) downloadFromURL(rawURL string) ([]byte, error) {
	if err := AssertDsDownloadURL(s.cfg.DSURL, rawURL); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	res, err := client.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("下载失败: %d", res.StatusCode)
	}
	return io.ReadAll(res.Body)
}

type versionResult struct {
	VersionID       string
	FileHash        string
	DocumentVersion int
	Deduplicated    bool
}

// createVersionFromBuffer 复刻 callback-processor：去重短路；新版本 storage.put 在事务前，事务失败回滚 + 删对象补偿。
func (s *Service) createVersionFromBuffer(ctx context.Context, db *gorm.DB, sess *EditorSession, buffer []byte, savedBy, source, writebackSource string) (versionResult, error) {
	fileHash := storage.ComputeFileHash(buffer)

	var dup struct {
		VersionID       string `gorm:"column:version_id"`
		DocumentVersion int    `gorm:"column:document_version"`
	}
	_ = db.Raw(`SELECT version_id, document_version FROM document_versions WHERE document_id = ? AND file_hash = ?`, sess.DocumentID, fileHash).Scan(&dup).Error
	if dup.VersionID != "" {
		_ = db.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, dup.VersionID, sess.DocumentID).Error
		s.Sessions.UpdateRevision(sess, dup.VersionID, fileHash, BuildDocumentKey(sess.DocumentID, dup.VersionID))
		return versionResult{VersionID: dup.VersionID, FileHash: fileHash, DocumentVersion: dup.DocumentVersion, Deduplicated: true}, nil
	}

	versionID := uuid.NewString()
	var nextVersion int
	_ = db.Raw(`SELECT COALESCE(MAX(document_version), 0) + 1 FROM document_versions WHERE document_id = ?`, sess.DocumentID).Scan(&nextVersion).Error
	objectKey := storage.ObjectKeyForVersion(sess.TenantID, sess.DocumentID, versionID)
	var mimeType string
	_ = db.Raw(`SELECT mime_type FROM documents WHERE document_id = ?`, sess.DocumentID).Scan(&mimeType).Error
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	if err := s.store.Put(ctx, objectKey, buffer, mimeType); err != nil {
		return versionResult{}, err
	}

	eventType := "save_new_version"
	if source == "ai_writeback" {
		eventType = "ai_writeback"
	}
	var wsb any
	if writebackSource != "" {
		wsb = writebackSource
	}
	payload, _ := json.Marshal(map[string]any{"file_hash": fileHash, "source": source, "writebackSource": wsb})

	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(
			`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, saved_at, source, object_key, size_bytes)
			 VALUES (?, ?, ?, ?, ?, ?, NOW(), ?, ?, ?)`,
			versionID, sess.DocumentID, sess.TenantID, nextVersion, fileHash, savedBy, source, objectKey, len(buffer),
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE documents SET current_version_id = ?, updated_at = NOW() WHERE document_id = ?`, versionID, sess.DocumentID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload) VALUES (?, ?, ?, ?, ?::jsonb)`,
			eventType, sess.DocumentID, versionID, sess.TenantID, string(payload),
		).Error
	})
	if txErr != nil {
		_ = s.store.Delete(ctx, objectKey)
		return versionResult{}, txErr
	}

	s.Sessions.UpdateRevision(sess, versionID, fileHash, BuildDocumentKey(sess.DocumentID, versionID))
	return versionResult{VersionID: versionID, FileHash: fileHash, DocumentVersion: nextVersion, Deduplicated: false}, nil
}

// ProcessSaveCallback 复刻 processSaveCallback：key 校验、状态过滤(2/6)、写回意图 peek、3 次指数退避下载入库、审计。返回 {error: n} 的 n。
func (s *Service) ProcessSaveCallback(ctx context.Context, db *gorm.DB, sess *EditorSession, body CallbackBody, actorID, actorRole string) int {
	s.Metrics.RecordAttempt()
	s.Sessions.Touch(sess)

	_, _, docKey := s.Sessions.Snapshot(sess) // 锁内读 DocumentKey，避免与并发回调的 UpdateRevision 竞争
	if body.Key != "" && body.Key != docKey {
		s.Metrics.RecordFailure()
		_ = audit.Write(db, audit.Entry{
			TenantID: sess.TenantID, ActorID: audit.P(actorID), ActorRole: audit.P(actorRole),
			ActionType: "editor_callback", TargetType: audit.P("document"), TargetID: audit.P(sess.DocumentID),
			Result: "失败", FailureReason: audit.P("document.key 不匹配"),
		})
		return 1
	}

	status := body.Status
	if status != 2 && status != 6 {
		return 0
	}
	if body.URL == "" {
		s.Metrics.RecordFailure()
		return 1
	}

	writebackSource, hasWriteback := s.Sessions.PeekPendingWritebackSave(sess, status)
	source := "user_edit"
	if hasWriteback {
		source = "ai_writeback"
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		buffer, derr := s.downloadFromURL(body.URL)
		if derr == nil {
			result, verr := s.createVersionFromBuffer(ctx, db, sess, buffer, actorID, source, writebackSource)
			if verr == nil {
				if hasWriteback {
					s.Sessions.ConfirmPendingWritebackSave(sess)
				}
				s.Metrics.RecordSuccess()
				_ = audit.Write(db, audit.Entry{
					TenantID: sess.TenantID, ActorID: audit.P(actorID), ActorRole: audit.P(actorRole),
					ActionType: "editor_save", TargetType: audit.P("document"), TargetID: audit.P(sess.DocumentID),
					Result: "成功", Metadata: map[string]any{"deduplicated": result.Deduplicated, "source": source, "callbackStatus": status},
				})
				return 0
			}
			derr = verr
		}
		lastErr = derr
		if attempt < 2 {
			time.Sleep(time.Duration(500*(1<<attempt)) * time.Millisecond)
		}
	}

	s.Metrics.RecordFailure()
	reason := "保存回调重试耗尽"
	if lastErr != nil {
		reason = lastErr.Error()
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: sess.TenantID, ActorID: audit.P(actorID), ActorRole: audit.P(actorRole),
		ActionType: "editor_callback", TargetType: audit.P("document"), TargetID: audit.P(sess.DocumentID),
		Result: "失败", FailureReason: audit.P(reason), Metadata: map[string]any{"alert": true, "retries": 3},
	})
	return 1
}
