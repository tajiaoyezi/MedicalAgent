package rag

import (
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
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
		if !chunkACLAllows(c.chunkACL, user) {
			dropped++
			continue
		}
		kept = append(kept, c)
	}
	return kept, dropped
}

// chunkACLAllows 解释 chunk_acl 物理列（owner=c03，本 phase 仅消费）：
//   - {} 或缺省 → 继承文档级（放行）
//   - {"deny_users":[...]} 命中当前用户 → 拒绝
//   - {"allow_users":[...]} 非空且不含当前用户 → 拒绝（严于文档级）
//   - {"allow_roles":[...]} 非空且与用户角色无交集 → 拒绝
func chunkACLAllows(acl map[string]any, user auth.AuthUser) bool {
	if len(acl) == 0 {
		return true
	}
	if deny, ok := acl["deny_users"].([]any); ok {
		for _, v := range deny {
			if s, _ := v.(string); s == user.UserID {
				return false
			}
		}
	}
	if allow, ok := acl["allow_users"].([]any); ok && len(allow) > 0 {
		found := false
		for _, v := range allow {
			if s, _ := v.(string); s == user.UserID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if allowRoles, ok := acl["allow_roles"].([]any); ok && len(allowRoles) > 0 {
		found := false
		for _, v := range allowRoles {
			s, _ := v.(string)
			for _, rs := range user.RoleSlugs {
				if rs == s {
					found = true
					break
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}
