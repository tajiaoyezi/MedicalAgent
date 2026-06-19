package knowledge

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/model"
	"medoffice/server/internal/pubmed"
)

// ErrSourceOffline：URL/白名单来源需公网抓取，公网不可用时该来源不可用（D7），引导改用「批量上传已下载的授权文件」。
var ErrSourceOffline = errors.New("公网不可用，URL/白名单来源不可用，请改用「批量上传已下载的授权文件」完成入库")

// previewPayload 是 PubMed/PMC 适配器取数得到、暂存到 kb_documents.preview_payload 的文献预览内容。
// 确认入库（ConfirmImport）时才据此物化为 c01 documents + document_chunks（§16.3）。预览阶段不落任何正式内容。
type previewPayload struct {
	Title    string `json:"title"`
	URL      string `json:"url"`
	PubmedID string `json:"pubmedId"`
	DOI      string `json:"doi"`
	Journal  string `json:"journal"`
	Year     int    `json:"year"`
	Abstract string `json:"abstract"`
}

// SourceAdapter 受控公网来源适配器（四段式管线的「来源适配器」层，D3/D4/D7）：
// 把 c04 pubmed-data-service 接入 c06 导入管线作为 PubMed/PMC 适配器后端，并对 URL/白名单来源做离线降级守卫。
// c06 仅消费 c04 取数与授权三态标记，MUST NOT 重建检索/授权内核；最终落库裁决以 c06 kb-import 契约为唯一真值。
type SourceAdapter struct {
	pub *pubmed.Service
}

// NewSourceAdapter 用 c04 pubmed.Service 构造（公网/离线双路径 + publicEnabled 由 Service 持有）。
func NewSourceAdapter(pub *pubmed.Service) *SourceAdapter { return &SourceAdapter{pub: pub} }

// ImportFromPubMed 4.6/4.7 PubMed/PMC 来源适配器（仅「来源适配器→暂存预览」段，不自动入库）：
// 按 kind(pubmed/pmc/doi)+id 经 c04 Service 取结构化文献（公网可用→在线真实拉取 4.6 连通性路径；否则→离线缓存 4.7
// 降级，路由由 c04 Service.useOnline 内部裁决），把文献预览内容（§16.3 来源元数据 + 摘要）暂存到 staging
// kb_documents.preview_payload，**不创建任何正式 c01 文档/chunk、不自动确认**（D3：确认前不落正式可检索内容；
// 取消则无残留）。授权三态初值取自 c04 RetrievedSource.AuthStatus（C04AuthHint），c06 仅消费、不从零重建（不漂移）。
// 入正式库与索引一律经人工 ConfirmImport（入库前预览确认 MUST）；preview_only 需管理员补授权后方可确认。返回 staging kbDocID。
func (a *SourceAdapter) ImportFromPubMed(db *gorm.DB, u auth.AuthUser, kbID, kind, id string) (string, error) {
	can, err := CanUploadToKB(db, u, kbID)
	if err != nil {
		return "", err
	}
	if !can {
		return "", ErrForbidden
	}

	ctx := model.InvokeContext{TenantID: u.TenantID, ActorID: u.UserID, ActorRole: strings.Join(u.RoleSlugs, ",")}
	src, err := a.pub.ImportByID(db, ctx, kind, id)
	if err != nil {
		return "", err
	}
	if src == nil {
		return "", ErrNotFound
	}
	// c04 已判 rejected（红线来源）→ 直接阻断，不落 staging。防御性分支：当前 kind∈{pubmed,pmc,doi} 经 c04
	// classifyAuth 不产生 rejected（rejected 仅 kind=url 命中商业库黑名单时出现，本适配器不走 url）；仅当 c04 取数标记
	// 演进出 rejected（如离线缓存注入）时本分支生效。
	if src.AuthStatus == pubmed.AuthRejected {
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
			ActionType: "kb_import_rejected", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID),
			Result: "失败", FailureReason: audit.P("来源被红线禁止（c04 取数标记 rejected）"),
			Metadata: map[string]any{"sourceType": pubmedSourceType(kind), "sourceIdentifier": id},
		})
		return "", ErrRejectedSource
	}

	sourceType := pubmedSourceType(kind)
	identifier := firstNonBlank(src.PubmedID, src.DOI, id)
	title := src.Title
	if title == "" {
		title = "PubMed " + identifier
	}
	abstract := src.Abstract
	if abstract == "" {
		abstract = title
	}
	// 预览内容暂存到 preview_payload（确认时才物化 c01 文档/chunk）。
	payload, err := json.Marshal(previewPayload{
		Title: title, URL: src.URL, PubmedID: src.PubmedID, DOI: src.DOI, Journal: src.Journal, Year: src.Year, Abstract: abstract,
	})
	if err != nil {
		return "", err
	}

	return PreviewImport(db, u, ImportRequest{
		KBID: kbID, SourceType: sourceType, SourceURL: src.URL, Title: title,
		C04AuthHint: src.AuthStatus, SourceIdentifier: identifier, PreviewPayload: string(payload),
	})
}

// ImportFromURL 4.4/4.8 URL/白名单来源适配器：URL 抓取依赖公网。公网不可用时该来源置不可用（D7），
// 留痕并返回 ErrSourceOffline 引导改用「批量上传已下载授权文件」（等效入库走上传入口、闭环不中断）。
// 公网可用时落 staging 预览（授权三态由 classifyAuthorization 按白名单/管理员裁决，PublicNetwork=true 先过出网脱敏门禁）。
// sourceType 取 SrcURL 或 SrcWhitelist（二者授权裁决同维，白名单性质由命中的 whitelist_rule_id 记录）。
func (a *SourceAdapter) ImportFromURL(db *gorm.DB, u auth.AuthUser, kbID, sourceType, sourceURL string, adminAuthorized bool) (string, error) {
	if sourceType != SrcWhitelist {
		sourceType = SrcURL
	}
	can, err := CanUploadToKB(db, u, kbID)
	if err != nil {
		return "", err
	}
	if !can {
		return "", ErrForbidden
	}
	if !a.pub.PublicEnabled() {
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
			ActionType: "kb_import_source_offline", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID),
			Result: "失败", FailureReason: audit.P("公网不可用，URL/白名单来源降级为上传授权文件"),
			Metadata: map[string]any{"sourceType": sourceType, "sourceUrl": sourceURL, "fallback": "batch_upload"},
		})
		return "", ErrSourceOffline
	}
	return PreviewImport(db, u, ImportRequest{
		KBID: kbID, SourceType: sourceType, SourceURL: sourceURL, AdminAuthorized: adminAuthorized, PublicNetwork: true,
	})
}

// materializePreview 在确认入库时把 staging 的 preview_payload 物化为 c01 documents + document_version + 单 chunk
// （§16.3 来源元数据：source_title/source_url/pubmed_id/doi/journal/year）。返回新建 document_id。
// chunk 初始 source_type='document'（HandleIndexReady 翻为 kb），与 demoseed/真实管线产出同构。
func materializePreview(db *gorm.DB, u auth.AuthUser, kbID, payloadJSON string) (string, error) {
	var p previewPayload
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return "", err
	}
	docID := uuid.NewString()
	title := p.Title
	if title == "" {
		title = "PubMed " + firstNonBlank(p.PubmedID, p.DOI)
	}
	abstract := p.Abstract
	if abstract == "" {
		abstract = title
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		verID := uuid.NewString()
		if err := tx.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
			VALUES (?, ?, ?, ?, 'app', 'kb', 'text/plain')`, docID, u.TenantID, u.UserID, title+".txt").Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			VALUES (?, ?, ?, 1, ?, ?, 'import', ?, ?)`, verID, docID, u.TenantID, "pubmed-"+firstNonBlank(p.PubmedID, p.DOI, docID), u.UserID, "pubmed/"+kbID+"/"+docID, len(abstract)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, source_title, source_url, pubmed_id, doi, journal, year, paragraph_index, chunk_text, chunk_acl, superseded)
			VALUES (?, ?, 1, 'document', ?, ?, ?, ?, ?, ?, 0, ?, '{"inheritedFrom":"document","entries":[]}'::jsonb, FALSE)`,
			u.TenantID, docID, p.Title, p.URL, nullIfBlank(p.PubmedID), nullIfBlank(p.DOI), nullIfBlank(p.Journal), nullIfZero(p.Year), abstract).Error
	})
	if err != nil {
		return "", err
	}
	return docID, nil
}

// pubmedSourceType 把 c04 取数 kind 映射为 kb_documents.source_type（pmc→pmc，其余 pubmed/doi→pubmed）。
func pubmedSourceType(kind string) string {
	if kind == "pmc" {
		return SrcPMC
	}
	return SrcPubMed
}

func firstNonBlank(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func nullIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}
