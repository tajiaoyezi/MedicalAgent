package rag

import (
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/chunkacl"
	"medoffice/server/internal/docperm"
)

// filterPermissions 六维过滤（§11.9）：tenant_id / kb_id / user_id / role / document_acl / chunk_acl。
// 过滤先于 rerank 与上下文注入；越权 document 与越权 chunk 在结果与引用中均不出现。
// document_acl 维对 c04 自有来源（upload/current/team/kb）一律按 c01 document_permissions 派生执行，不依赖 c06。
func filterPermissions(db *gorm.DB, user auth.AuthUser, cands []Candidate) (kept []Candidate, dropped int) {
	docCache := map[string]docperm.Level{}
	resolveDoc := func(docID string) docperm.Level {
		if lvl, ok := docCache[docID]; ok {
			return lvl
		}
		var doc docperm.DocumentRow
		_ = db.Raw(`SELECT * FROM documents WHERE document_id = ? AND tenant_id = ?`, docID, user.TenantID).Scan(&doc).Error
		lvl := docperm.None
		if doc.DocumentID != "" && !doc.IsDeleted {
			lvl, _ = docperm.Resolve(db, user, doc)
		}
		docCache[docID] = lvl
		return lvl
	}

	for _, c := range cands {
		// PubMed 外部公开来源：无 document_acl/chunk_acl 维，仅经脱敏门禁与白名单，跳过文档级过滤
		if c.SourceType == "pubmed" {
			kept = append(kept, c)
			continue
		}
		// tenant 维
		if c.tenantID != "" && c.tenantID != user.TenantID {
			dropped++
			continue
		}
		// document_acl 维（user_id/role/dept 折叠在 docperm.Resolve 内）
		if resolveDoc(c.DocumentID) == docperm.None {
			dropped++
			continue
		}
		// chunk_acl 维（chunk 级，可严于文档级）
		if !chunkacl.Allows(c.chunkACL, user) {
			dropped++
			continue
		}
		kept = append(kept, c)
	}
	return kept, dropped
}
