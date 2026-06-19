package knowledge

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/model"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/pubmed"
)

// ErrSourceOffline：URL/白名单来源需公网抓取，公网不可用时该来源不可用（D7），引导改用「批量上传已下载的授权文件」。
var ErrSourceOffline = errors.New("公网不可用，URL/白名单来源不可用，请改用「批量上传已下载的授权文件」完成入库")

// SourceAdapter 受控公网来源适配器（四段式管线的「来源适配器」层，D3/D4/D7）：
// 把 c04 pubmed-data-service 接入 c06 导入管线作为 PubMed/PMC 适配器后端，并对 URL/白名单来源做离线降级守卫。
// c06 仅消费 c04 取数与授权三态标记，MUST NOT 重建检索/授权内核；最终落库裁决以 c06 kb-import 契约为唯一真值。
type SourceAdapter struct {
	pub *pubmed.Service
}

// NewSourceAdapter 用 c04 pubmed.Service 构造（公网/离线双路径 + publicEnabled 由 Service 持有）。
func NewSourceAdapter(pub *pubmed.Service) *SourceAdapter { return &SourceAdapter{pub: pub} }

// ImportFromPubMed 4.6/4.7 PubMed/PMC 来源适配器：按 kind(pubmed/pmc/doi)+id 经 c04 Service 取结构化文献
// （公网可用时在线真实拉取——4.6 连通性路径；否则离线缓存——4.7 降级，路由裁决由 c04 Service.useOnline 内部完成），
// 把文献摘要落 c01 documents + chunk（§16.3 来源元数据：pubmed_id/doi/journal/year/source_url/source_title），
// 再走四段式 PreviewImport→（authorized 时）ConfirmImport→HandleIndexReady。授权三态初值取自 c04 返回的
// RetrievedSource.AuthStatus（C04AuthHint），c06 仅在其上叠加裁决、MUST NOT 从零重建（不与 c04 漂移）。
// 返回 kbDocID（preview_only/rejected 不入正式库，仅落 staging 或红线阻断）。
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
	// c04 已判 rejected（红线来源）→ 直接阻断，不创建任何文档（避免孤儿 c01 document）。
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

	// 内容载体：把文献摘要落 c01 documents + 1 chunk（§16.3 pubmed 来源元数据），使离线导入可检索可溯源。
	// chunk 初始 source_type='document'（HandleIndexReady 翻为 kb），与 demoseed/真实管线产出同构。
	docID := uuid.NewString()
	if err := db.Transaction(func(tx *gorm.DB) error {
		verID := uuid.NewString()
		if err := tx.Exec(`INSERT INTO documents (document_id, tenant_id, owner_id, name, space, app_source, mime_type)
			VALUES (?, ?, ?, ?, 'app', 'kb', 'text/plain')`, docID, u.TenantID, u.UserID, title+".txt").Error; err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO document_versions (version_id, document_id, tenant_id, document_version, file_hash, saved_by, source, object_key, size_bytes)
			VALUES (?, ?, ?, 1, ?, ?, 'import', ?, ?)`, verID, docID, u.TenantID, "pubmed-"+identifier, u.UserID, "pubmed/"+kbID+"/"+docID, len(abstract)).Error; err != nil {
			return err
		}
		if err := tx.Exec(`UPDATE documents SET current_version_id = ? WHERE document_id = ?`, verID, docID).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO document_chunks (tenant_id, document_id, document_version, source_type, source_title, source_url, pubmed_id, doi, journal, year, paragraph_index, chunk_text, chunk_acl, superseded)
			VALUES (?, ?, 1, 'document', ?, ?, ?, ?, ?, ?, 0, ?, '{"inheritedFrom":"document","entries":[]}'::jsonb, FALSE)`,
			u.TenantID, docID, src.Title, src.URL, nullIfBlank(src.PubmedID), nullIfBlank(src.DOI), nullIfBlank(src.Journal), nullIfZero(src.Year), abstract).Error
	}); err != nil {
		return "", err
	}

	kbDocID, err := PreviewImport(db, u, ImportRequest{
		KBID: kbID, SourceType: sourceType, SourceURL: src.URL, Title: title,
		C04AuthHint: src.AuthStatus, DocumentID: docID, SourceIdentifier: identifier,
	})
	if err != nil {
		return "", err
	}
	// 仅 authorized 推进入库 + 索引（preview_only 停在 staging 临时预览，等管理员补授权后确认）。
	if src.AuthStatus == pubmed.AuthAuthorized {
		if err := ConfirmImport(db, u, kbDocID); err != nil {
			return kbDocID, err
		}
		if err := HandleIndexReady(db, parsing.IndexReadyEvent{TenantID: u.TenantID, DocumentID: docID, DocumentVersion: 1, ChunkCount: 1}); err != nil {
			return kbDocID, err
		}
		// 文献内容已直接成 chunk，无真实文件需解析 → 删除 ConfirmImport 入队的悬挂解析作业（同 demoseed 口径）。
		db.Exec(`DELETE FROM document_parse_jobs WHERE document_id = ?`, docID)
	}
	return kbDocID, nil
}

// ImportFromURL 4.4/4.8 URL/白名单来源适配器：URL 抓取依赖公网。公网不可用时该来源置不可用（D7），
// 留痕并返回 ErrSourceOffline 引导改用「批量上传已下载授权文件」（等效入库走上传入口、闭环不中断）。
// 公网可用时落 staging 预览（授权三态由 classifyAuthorization 按白名单/管理员裁决，PublicNetwork=true 先过出网脱敏门禁）。
func (a *SourceAdapter) ImportFromURL(db *gorm.DB, u auth.AuthUser, kbID, sourceURL string, adminAuthorized bool) (string, error) {
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
			Metadata: map[string]any{"sourceType": SrcURL, "sourceUrl": sourceURL, "fallback": "batch_upload"},
		})
		return "", ErrSourceOffline
	}
	return PreviewImport(db, u, ImportRequest{
		KBID: kbID, SourceType: SrcURL, SourceURL: sourceURL, AdminAuthorized: adminAuthorized, PublicNetwork: true,
	})
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
