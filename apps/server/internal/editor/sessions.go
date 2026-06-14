package editor

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"medoffice/server/internal/config"
)

// 写回意图窗口（与 editor-sessions.ts 一致：arm 窗 30s，意图 TTL 2min）。
const (
	writebackArmWindow = 30 * time.Second
	writebackIntentTTL = 2 * time.Minute
)

type PendingWritebackSave struct {
	Source       string
	SaveIntentID string
	ArmedAt      time.Time
	Armed        bool
}

type EditorSession struct {
	OpenToken     string
	CallbackToken string
	BridgeToken   string
	DocumentID    string
	DocumentKey   string
	TenantID      string
	UserID        string
	VersionID     string
	Revision      string
	ExpiresAt     time.Time
	Pending       *PendingWritebackSave
}

// SessionStore 复刻 editor-sessions.ts 的内存三映射；Go 下并发访问，统一用 mu 保护映射与会话可变字段。
type SessionStore struct {
	mu            sync.Mutex
	sessions      map[string]*EditorSession // openToken → session
	callbackIndex map[string]string         // callbackToken → openToken
	bridgeIndex   map[string]string         // bridgeToken → openToken
	callbackTTL   time.Duration
}

func NewSessionStore(cfg config.OnlyOffice) *SessionStore {
	return &SessionStore{
		sessions:      map[string]*EditorSession{},
		callbackIndex: map[string]string{},
		bridgeIndex:   map[string]string{},
		callbackTTL:   time.Duration(cfg.CallbackTokenTTLSeconds) * time.Second,
	}
}

func newToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *SessionStore) purgeLocked() {
	now := time.Now()
	for tok, sess := range s.sessions {
		if sess.ExpiresAt.Before(now) {
			delete(s.sessions, tok)
			delete(s.callbackIndex, sess.CallbackToken)
			delete(s.bridgeIndex, sess.BridgeToken)
		}
	}
}

type CreateInput struct {
	DocumentID  string
	DocumentKey string
	TenantID    string
	UserID      string
	VersionID   string
	Revision    string
}

func (s *SessionStore) Create(in CreateInput) *EditorSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	sess := &EditorSession{
		OpenToken:     newToken(24),
		CallbackToken: newToken(24),
		BridgeToken:   newToken(24),
		DocumentID:    in.DocumentID,
		DocumentKey:   in.DocumentKey,
		TenantID:      in.TenantID,
		UserID:        in.UserID,
		VersionID:     in.VersionID,
		Revision:      in.Revision,
		ExpiresAt:     time.Now().Add(s.callbackTTL),
	}
	s.sessions[sess.OpenToken] = sess
	s.callbackIndex[sess.CallbackToken] = sess.OpenToken
	s.bridgeIndex[sess.BridgeToken] = sess.OpenToken
	return sess
}

func (s *SessionStore) GetByOpenToken(tok string) *EditorSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	return s.sessions[tok]
}

func (s *SessionStore) GetByCallbackToken(tok string) *EditorSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	if open, ok := s.callbackIndex[tok]; ok {
		return s.sessions[open]
	}
	return nil
}

func (s *SessionStore) GetByBridgeToken(tok string) *EditorSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	if open, ok := s.bridgeIndex[tok]; ok {
		return s.sessions[open]
	}
	return nil
}

func (s *SessionStore) Touch(sess *EditorSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.ExpiresAt = time.Now().Add(s.callbackTTL)
}

// Snapshot 在锁内读取会话可变字段（Revision/VersionID/DocumentKey），避免与 UpdateRevision（回调路径写）竞争。
// 不可变字段（TenantID/UserID/DocumentID/各 token）可直接读，无需快照。
func (s *SessionStore) Snapshot(sess *EditorSession) (revision, versionID, documentKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return sess.Revision, sess.VersionID, sess.DocumentKey
}

func (s *SessionStore) UpdateRevision(sess *EditorSession, versionID, revision, documentKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.VersionID = versionID
	sess.Revision = revision
	sess.DocumentKey = documentKey
}

func (s *SessionStore) CreateSaveIntent(sess *EditorSession, source string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := newToken(16)
	sess.Pending = &PendingWritebackSave{Source: source, SaveIntentID: id, Armed: false}
	return id
}

func (s *SessionStore) ArmWritebackSaveIntent(sess *EditorSession, saveIntentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := sess.Pending
	if p == nil || p.SaveIntentID != saveIntentID || p.Armed {
		return false
	}
	p.Armed = true
	p.ArmedAt = time.Now()
	return true
}

// PeekPendingWritebackSave：仅 forcesave(status=6) 且已 arm 且在窗口内时返回 (source,true)。
func (s *SessionStore) PeekPendingWritebackSave(sess *EditorSession, callbackStatus int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := sess.Pending
	if p == nil || callbackStatus != 6 || !p.Armed {
		return "", false
	}
	since := time.Since(p.ArmedAt)
	if since > writebackArmWindow || since > writebackIntentTTL {
		return "", false
	}
	return p.Source, true
}

func (s *SessionStore) ConfirmPendingWritebackSave(sess *EditorSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.Pending = nil
}

func (s *SessionStore) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]*EditorSession{}
	s.callbackIndex = map[string]string{}
	s.bridgeIndex = map[string]string{}
}
