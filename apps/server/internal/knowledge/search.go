package knowledge

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/rag"
)

// SearchFilters 多维筛选（§11.6）。文档类型与来源用两个不同承载字段区分、不混同：
// DocType 取自 kb_documents.document_id 关联的 c01 documents 文件类型（pdf/docx/...）；
// Source 取自 kb_documents.source_type（来源渠道 upload/url/pubmed/pmc/whitelist）。
type SearchFilters struct {
	DocType      string     // 文件类型（按 documents 文件名扩展名/mime 判定）
	Source       string     // 来源类型（kb_documents.source_type）
	UpdatedAfter *time.Time // 更新时间下界（documents.updated_at）
}

// SearchHit 单条检索结果（直接复用 rag.Candidate 的溯源元数据：kb_id/chunk_id/page/section/...）。
type SearchResult struct {
	Mode  string          `json:"mode"`
	Hits  []rag.Candidate `json:"hits"`
	Total int             `json:"total"`
}

func normMode(m string) string {
	switch m {
	case rag.SearchKeyword, rag.SearchSemantic, rag.SearchHybrid:
		return m
	default:
		return rag.SearchHybrid
	}
}

func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return strings.ToLower(name[i+1:])
	}
	return ""
}

// KBSearch 全局搜索（§11.6）：三模式（keyword/semantic/hybrid）+ 多维筛选（知识库/文档类型/来源/更新时间）。
// 数据源选择按可见集合裁剪 kb_id（越权 kb 不进检索范围），检索复用 c04 rag.Retrieve（权限六维过滤在召回前），
// 再按文档类型/来源/更新时间多维筛选（文档类型取文件类型字段、来源取 source_type 字段，两维不混同）。
func KBSearch(db *gorm.DB, eng *rag.Engine, u auth.AuthUser, kbIDs []string, query, mode string, f SearchFilters) (*SearchResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, ErrInvalidInput
	}
	visible, err := VisibleKBIDs(db, u)
	if err != nil {
		return nil, err
	}
	vset := map[string]bool{}
	for _, id := range visible {
		vset[id] = true
	}
	scope := make([]string, 0, len(kbIDs))
	for _, id := range kbIDs {
		if vset[id] {
			scope = append(scope, id)
		}
	}
	if len(kbIDs) > 0 && len(scope) == 0 {
		return nil, ErrForbidden
	}
	if len(kbIDs) == 0 {
		scope = visible
	}

	m := normMode(mode)
	rr, err := eng.Retrieve(db, rag.RetrieveRequest{
		User:       u,
		ModeLabel:  "知识库全局搜索",
		AllowKB:    true,
		Query:      query,
		KBIDs:      scope,
		SearchMode: m,
		TopN:       20,
	})
	if err != nil {
		return nil, err
	}

	hits := rr.Candidates
	// 多维筛选：仅当设置了 DocType/Source/UpdatedAfter 时按 documents/kb_documents 元数据过滤。
	if f.DocType != "" || f.Source != "" || f.UpdatedAfter != nil {
		docIDs := make([]string, 0, len(hits))
		for _, h := range hits {
			if h.DocumentID != "" {
				docIDs = append(docIDs, h.DocumentID)
			}
		}
		meta := map[string]struct {
			source    string
			fileType  string
			updatedAt time.Time
		}{}
		if len(docIDs) > 0 {
			var rows []struct {
				DocumentID string    `gorm:"column:document_id"`
				SourceType string    `gorm:"column:source_type"`
				Name       string    `gorm:"column:name"`
				MimeType   *string   `gorm:"column:mime_type"`
				UpdatedAt  time.Time `gorm:"column:updated_at"`
			}
			_ = db.Raw(
				`SELECT kbd.document_id, kbd.source_type, d.name, d.mime_type, d.updated_at
				 FROM kb_documents kbd JOIN documents d ON d.document_id = kbd.document_id
				 WHERE kbd.tenant_id = ? AND kbd.document_id IN ?`,
				u.TenantID, docIDs,
			).Scan(&rows).Error
			for _, r := range rows {
				meta[r.DocumentID] = struct {
					source    string
					fileType  string
					updatedAt time.Time
				}{source: r.SourceType, fileType: fileExt(r.Name), updatedAt: r.UpdatedAt}
			}
		}
		filtered := hits[:0:0]
		for _, h := range hits {
			md, ok := meta[h.DocumentID]
			if !ok {
				continue // 无法解析元数据的候选在有筛选时排除（fail-closed）
			}
			if f.DocType != "" && !strings.EqualFold(md.fileType, f.DocType) {
				continue
			}
			if f.Source != "" && md.source != f.Source {
				continue
			}
			if f.UpdatedAfter != nil && md.updatedAt.Before(*f.UpdatedAfter) {
				continue
			}
			filtered = append(filtered, h)
		}
		hits = filtered
	}

	return &SearchResult{Mode: m, Hits: hits, Total: len(hits)}, nil
}
