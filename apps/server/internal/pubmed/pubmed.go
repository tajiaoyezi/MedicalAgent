// Package pubmed 实现 c04 PubMed 数据服务：统一取数接口（search/fetch_detail/import_by_id）、
// 公网在线 / 离线缓存双路径、白名单授权标记，公网调用前消费 c09 脱敏门禁（本期默认拒绝→降级离线）。
package pubmed

import (
	"net/url"
	"strings"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/model"
)

// 授权状态标记（§16.1）。落库裁决归 c06 kb-import，本 phase 仅返回标记、不发起 KB 落库。
const (
	AuthAuthorized   = "authorized"
	AuthPreviewOnly  = "preview_only"
	AuthRejected     = "rejected"
)

// RetrievedSource 内部归一化检索条目（与上传文件/知识库 chunk 同构后进入 RAG 合并去重）。
type RetrievedSource struct {
	SourceType string `json:"sourceType"` // 恒为 "pubmed"
	PubmedID   string `json:"pubmedId"`
	DOI        string `json:"doi"`
	Title      string `json:"title"`
	Journal    string `json:"journal"`
	Year       int    `json:"year"`
	URL        string `json:"url"`
	Abstract   string `json:"abstract"`
	AuthStatus string `json:"authStatus"` // authorized/preview_only/rejected
}

// Provider 统一取数接口（design D4）。
type Provider interface {
	Search(query string, limit int) ([]RetrievedSource, error)
	FetchDetail(id string) (*RetrievedSource, error)
	Name() string
}

// 白名单（§16.1）：PubMed/PMC/DOI 标准来源。未授权商业库（万方/知网/维普）禁止默认抓取。
var whitelistHosts = map[string]bool{
	"pubmed.ncbi.nlm.nih.gov": true,
	"www.ncbi.nlm.nih.gov":    true,
	"ncbi.nlm.nih.gov":        true,
	"pmc.ncbi.nlm.nih.gov":    true,
	"doi.org":                 true,
	"dx.doi.org":              true,
}

// 未授权商业库黑名单（§16.1）。注：URL 红线的最终落库裁决以 c06 knowledge.isCommercialBlocked（子域感知）为唯一真值，
// 本表仅为 c04 取数侧的初值标记；二者条目保持一致以免观感漂移（子域匹配能力归 c06）。
var commercialBlocklist = map[string]bool{
	"wanfangdata.com.cn": true,
	"cnki.net":           true,
	"www.cnki.net":       true,
	"cqvip.com":          true,
	"www.cqvip.com":      true,
}

// Service 统一编排：公网脱敏门禁 + 在线/离线选择 + 取数授权标记。
type Service struct {
	online  Provider
	offline Provider
	// publicEnabled：本期默认 false（§16.4/§24.9 公网未放开）。即使为 true，公网调用仍须先过脱敏门禁。
	publicEnabled bool
}

// NewService 构造。online 可为 nil（离线部署）。publicEnabled 本期默认 false。
func NewService(online, offline Provider, publicEnabled bool) *Service {
	return &Service{online: online, offline: offline, publicEnabled: publicEnabled}
}

// PublicEnabled 公网取数是否可用（publicEnabled 且 online 非空）。供消费方（c06 URL/白名单导入）判定
// 公网不可用时降级（D7：URL 来源置不可用、引导改用上传授权文件）。脱敏门禁仍在每次实际出网时另行裁决。
func (s *Service) PublicEnabled() bool { return s.publicEnabled && s.online != nil }

// Search 统一检索：公网可用且脱敏通过→在线；否则→离线缓存（design D4 熔断降级）。
// 返回 route 标识数据来源（"online"/"offline"）供答案过程提示。
func (s *Service) Search(db *gorm.DB, ctx model.InvokeContext, query string, limit int) ([]RetrievedSource, string, error) {
	if s.useOnline(db, ctx, query) {
		res, err := s.online.Search(query, limit)
		if err == nil {
			return res, "online", nil
		}
		// 在线失败 → 熔断降级离线（不把外部抖动暴露为答案失败）
		_ = audit.Write(db, audit.Entry{
			TenantID: ctx.TenantID, ActorID: ptr(ctx.ActorID), ActorRole: ptr(ctx.ActorRole),
			ActionType: "pubmed_online_fallback", TargetType: audit.P("pubmed"), Result: "失败",
			FailureReason: audit.P("在线 PubMed 调用失败，降级离线缓存"),
		})
	}
	if s.offline == nil {
		return nil, "none", nil
	}
	res, err := s.offline.Search(query, limit)
	return res, "offline", err
}

// useOnline 判定本次是否走公网：publicEnabled 且 online 非空，且脱敏门禁放行。
// 脱敏门禁默认拒绝（c09 未接入）→ 返回 false → 降级离线（design D4：数据源降级离线而非私有化模型）。
func (s *Service) useOnline(db *gorm.DB, ctx model.InvokeContext, query string) bool {
	if !s.publicEnabled || s.online == nil {
		return false
	}
	v := model.EvaluateRedaction(model.RedactionInput{TenantID: ctx.TenantID, Text: query})
	if !v.Available || !v.Passed {
		_ = audit.Write(db, audit.Entry{
			TenantID: ctx.TenantID, ActorID: ptr(ctx.ActorID), ActorRole: ptr(ctx.ActorRole),
			ActionType: "pubmed_redaction_block", TargetType: audit.P("pubmed"), Result: "失败",
			FailureReason: audit.P(v.Reason),
			Metadata:      map[string]any{"switchTo": "offline_cache"},
		})
		return false
	}
	return true
}

// ImportByID 按 PMC/DOI/URL 取数并归一化，返回授权状态标记。本 phase MUST NOT 向 knowledge_bases/kb_documents 落库。
func (s *Service) ImportByID(db *gorm.DB, ctx model.InvokeContext, kind, id string) (*RetrievedSource, error) {
	authStatus := classifyAuth(kind, id)

	var src *RetrievedSource
	// 取数路径（design D4/D7 连通性 + 离线降级，与 Search 对称）：
	// 公网可用且脱敏放行 → 在线真实拉取（4.6 连通性路径）；公网不可用/在线失败 → 离线缓存（4.7 降级）。
	// rejected 直接不取（红线来源不抓取）。
	if authStatus != AuthRejected && s.useOnline(db, ctx, id) {
		if d, err := s.online.FetchDetail(id); err == nil && d != nil {
			src = d
		}
	}
	if src == nil && authStatus != AuthRejected && s.offline != nil {
		if d, err := s.offline.FetchDetail(id); err == nil && d != nil {
			src = d
		}
	}
	if src == nil {
		src = &RetrievedSource{SourceType: "pubmed", URL: id}
		switch kind {
		case "pmc":
			src.PubmedID = id
		case "doi":
			src.DOI = id
		}
	}
	src.AuthStatus = authStatus

	_ = audit.Write(db, audit.Entry{
		TenantID: ctx.TenantID, ActorID: ptr(ctx.ActorID), ActorRole: ptr(ctx.ActorRole),
		ActionType: "pubmed_import", TargetType: audit.P("pubmed"), TargetID: audit.P(id),
		Result:   "成功",
		Metadata: map[string]any{"kind": kind, "authStatus": authStatus, "kbWriteback": false},
	})
	return src, nil
}

// classifyAuth 按白名单/商业库黑名单裁决授权标记（§16.1）。落库最终裁决归 c06。
func classifyAuth(kind, id string) string {
	switch kind {
	case "pubmed":
		return AuthAuthorized // PubMed 属 PubMed/PMC 体系白名单开放来源（与 pmc 对称）
	case "pmc":
		return AuthAuthorized // PMC 属 PubMed 体系白名单
	case "doi":
		// 演示口径：DOI 标准前缀视为 authorized（真实部署由管理员白名单细化）
		if strings.HasPrefix(strings.TrimSpace(id), "10.") {
			return AuthAuthorized
		}
		return AuthPreviewOnly
	case "url":
		host := hostOf(id)
		if commercialBlocklist[host] {
			return AuthRejected // 未授权商业库
		}
		if whitelistHosts[host] {
			return AuthAuthorized
		}
		return AuthPreviewOnly // 不在白名单且无管理员授权
	default:
		return AuthPreviewOnly
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Host)
}

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
