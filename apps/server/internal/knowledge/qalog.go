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

	var rows []struct {
		AuditID   string  `gorm:"column:audit_id"`
		ActorID   *string `gorm:"column:actor_id"`
		TargetID  *string `gorm:"column:target_id"`
		UserName  *string `gorm:"column:display_name"`
		Metadata  string  `gorm:"column:metadata"`
		CreatedAt string  `gorm:"column:created_at"`
	}
	if err := db.Raw(
		`SELECT a.audit_id, a.actor_id, a.target_id, u.display_name, a.metadata::text AS metadata, a.created_at
		 FROM audit_logs a LEFT JOIN users u ON u.user_id = a.actor_id
		 WHERE a.tenant_id = ? AND a.action_type = 'kb_qa' AND a.result = '成功'
		 ORDER BY a.created_at DESC LIMIT ?`,
		u.TenantID, limit,
	).Scan(&rows).Error; err != nil {
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

		// 权限范围裁剪：库管理员仅见与其管理库相交的问答日志。
		if !all && !intersects(md.KBIDs, mset) {
			continue
		}
		// kbFilter 命中裁剪。
		if kbFilter != "" && !contains(md.KBIDs, kbFilter) {
			continue
		}

		e := QALogEntry{
			LogID: r.AuditID, Query: md.Query, KBIDs: md.KBIDs,
			MessageID: md.MessageID, CitationCount: md.CitationCount, OccurredAt: r.CreatedAt,
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
		// 对应引用来源：按答案 message_id 取 citations（日志已在权限范围内，引用随答案天然同范围）。
		if md.MessageID != "" {
			var cites []QACitation
			_ = db.Raw(
				`SELECT source_type, COALESCE(source_title,'') AS source_title, COALESCE(source_url,'') AS source_url,
				        COALESCE(kb_id::text,'') AS kb_id
				 FROM citations WHERE tenant_id = ? AND message_id = ? ORDER BY cite_index`,
				u.TenantID, md.MessageID,
			).Scan(&cites).Error
			e.Citations = cites
		}
		out = append(out, e)
	}
	return out, nil
}

func intersects(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

func contains(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
