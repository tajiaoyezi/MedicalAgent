package knowledge

import (
	"encoding/json"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
)

// QACitation 问答日志条目的「对应引用来源」（§11.5「查看问答日志」展示用）。
type QACitation struct {
	SourceType  string `gorm:"column:source_type" json:"sourceType"`
	SourceTitle string `gorm:"column:source_title" json:"sourceTitle"`
	SourceURL   string `gorm:"column:source_url" json:"sourceUrl"`
	KBID        string `gorm:"column:kb_id" json:"kbId"`
}

// QALogEntry 一条问答日志（来源 = audit_logs action_type=kb_qa，引用来源 = citations 按 message_id 关联）。
type QALogEntry struct {
	LogID          string       `json:"logId"`
	UserID         string       `json:"userId"`
	UserName       string       `json:"userName"`
	ConversationID string       `json:"conversationId"`
	MessageID      string       `json:"messageId"`
	Query          string       `json:"query"`
	KBIDs          []string     `json:"kbIds"`
	CitationCount  int          `json:"citationCount"`
	Citations      []QACitation `json:"citations"`
	OccurredAt     string       `json:"occurredAt"`
}

// managedKBIDs 返回用户作为「库管理员」可管理的 kb_id 集合（§19.1 per-kb 管理级 ACL，非新全局角色）：
// 平台管理员 → (nil, true) 表示全租户；否则 → 自建库 ∪ 对其文档持 manage|owner 授予的库 + (false)。
func managedKBIDs(db *gorm.DB, u auth.AuthUser) ([]string, bool, error) {
	if isPlatformAdmin(u) {
		return nil, true, nil
	}
	var ids []string
	err := db.Raw(
		`SELECT kb_id FROM knowledge_bases WHERE tenant_id = ? AND created_by = ?
		 UNION
		 SELECT DISTINCT kbd.kb_id FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		 WHERE kbd.tenant_id = ? AND dp.permission_level IN ('manage','owner') AND (
		   (dp.principal_type='user' AND dp.principal_id = ?)
		   OR (dp.principal_type='role' AND dp.principal_id IN ?)
		   OR (dp.principal_type='dept' AND dp.principal_id = ?))`,
		u.TenantID, u.UserID, u.TenantID, u.UserID, roleSlugsForIN(u), deptForMatch(u),
	).Scan(&ids).Error
	return ids, false, err
}

// ListQALogs 管理员在权限范围内查看问答日志（9.3，§11.5「查看问答日志」）：
//   - 平台管理员 → 本租户全部 kb_qa 问答日志；
//   - 库管理员 → 仅与其管理库相交的问答日志（按 metadata.kbIds 裁剪）；
//   - 既非平台管理员又不管理任何库的普通用户 → ErrForbidden（管理类视图，普通用户不可见）。
//
// kbFilter 非空时按该 kb_id 进一步过滤，且库管理员越界查看非自管库直接 ErrForbidden。
// 每条返回用户/所选 kb_id/查询/时间与对应引用来源（citations 按 message_id 关联）。
func ListQALogs(db *gorm.DB, u auth.AuthUser, kbFilter string, limit int) ([]QALogEntry, error) {
	managed, all, err := managedKBIDs(db, u)
	if err != nil {
		return nil, err
	}
	if !all && len(managed) == 0 {
		return nil, ErrForbidden // 非平台管理员且不管理任何库 → 无权查看问答日志
	}
	mset := map[string]bool{}
	for _, id := range managed {
		mset[id] = true
	}
	if kbFilter != "" && !all && !mset[kbFilter] {
		return nil, ErrForbidden // 库管理员越界查看非自管库的问答日志
	}
	if limit <= 0 || limit > 500 {
		limit = 200 // 管理后台视图上限；超量截断（§24.3 POC 验收规模足够）
	}

	// 权限/过滤维下推到 SQL，使 LIMIT 作用在「已按可管理库过滤」的结果集上——
	// 否则先 LIMIT 后在 Go 侧过滤会把权限范围内、但被租户级 top-N 挤出的日志漏掉。
	// jsonb 包含判断刻意避开「?」操作符（与 gorm 占位符冲突）：用 jsonb_array_elements_text + IN ?（切片展开）、@> ?::jsonb。
	q := `SELECT a.audit_id, a.actor_id, a.target_id, u.display_name, a.metadata::text AS metadata, a.created_at
		 FROM audit_logs a LEFT JOIN users u ON u.user_id = a.actor_id
		 WHERE a.tenant_id = ? AND a.action_type = 'kb_qa' AND a.result = '成功'`
	args := []any{u.TenantID}
	if !all {
		q += ` AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(a.metadata->'kbIds') e WHERE e IN ?)`
		args = append(args, managed) // 已保证 managed 非空（前置 ErrForbidden），不会 IN ()
	}
	if kbFilter != "" {
		kf, _ := json.Marshal([]string{kbFilter})
		q += ` AND a.metadata->'kbIds' @> ?::jsonb`
		args = append(args, string(kf))
	}
	q += ` ORDER BY a.created_at DESC LIMIT ?`
	args = append(args, limit)

	var rows []struct {
		AuditID   string  `gorm:"column:audit_id"`
		ActorID   *string `gorm:"column:actor_id"`
		TargetID  *string `gorm:"column:target_id"`
		UserName  *string `gorm:"column:display_name"`
		Metadata  string  `gorm:"column:metadata"`
		CreatedAt string  `gorm:"column:created_at"`
	}
	if err := db.Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := make([]QALogEntry, 0, len(rows))
	for _, r := range rows {
		var md struct {
			MessageID     string   `json:"messageId"`
			Query         string   `json:"query"`
			KBIDs         []string `json:"kbIds"`
			CitationCount int      `json:"citationCount"`
		}
		_ = json.Unmarshal([]byte(r.Metadata), &md)

		// 库管理员视图：把 kb 集合裁剪为「∩ 自管库」，不回显非自管库的存在性（平台管理员 all=true 见全集）。
		// 一条跨库问答（kbIds 含自管 ∪ 非自管）虽因命中自管库而可见，但非自管部分 MUST NOT 披露。
		kbids := md.KBIDs
		if !all {
			kbids = clipToSet(md.KBIDs, mset)
		}
		e := QALogEntry{
			LogID: r.AuditID, Query: md.Query, KBIDs: kbids,
			MessageID: md.MessageID, OccurredAt: r.CreatedAt,
		}
		if r.TargetID != nil {
			e.ConversationID = *r.TargetID
		}
		if r.ActorID != nil {
			e.UserID = *r.ActorID
		}
		if r.UserName != nil {
			e.UserName = *r.UserName
		}
		// 对应引用来源：按答案 message_id 取 citations；库管理员视图再按自管库裁剪引用，
		// 防跨库问答把非自管库的引用标题/URL/章节/片段（可能含 PHI 源数据）泄露给只管理另一库的管理员。
		if md.MessageID != "" {
			var cites []QACitation
			_ = db.Raw(
				`SELECT source_type, COALESCE(source_title,'') AS source_title, COALESCE(source_url,'') AS source_url,
				        COALESCE(kb_id::text,'') AS kb_id
				 FROM citations WHERE tenant_id = ? AND message_id = ? ORDER BY cite_index`,
				u.TenantID, md.MessageID,
			).Scan(&cites).Error
			if !all {
				kept := make([]QACitation, 0, len(cites))
				for _, ct := range cites {
					if ct.KBID != "" && mset[ct.KBID] {
						kept = append(kept, ct)
					}
				}
				cites = kept
			}
			e.Citations = cites
		}
		e.CitationCount = len(e.Citations) // 计数与裁剪后实际返回的引用对齐，不泄露被裁掉的引用存在性
		out = append(out, e)
	}
	return out, nil
}

// clipToSet 返回 ids 中落在 set 内的子集（保持顺序），用于把日志 kb 集合裁剪到库管理员自管范围。
func clipToSet(ids []string, set map[string]bool) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if set[id] {
			out = append(out, id)
		}
	}
	return out
}
