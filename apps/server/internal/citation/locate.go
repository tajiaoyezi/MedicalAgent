package citation

import (
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/chunkacl"
	"medoffice/server/internal/docperm"
)

// 引用异常分支文案（§8.9 原文）。
const (
	MsgUnavailable = "该引用源暂时不可用"
	MsgDeleted     = "该引用源已删除"
	MsgChunkFail   = "已打开来源文档，请手动查看相关段落"
)

// LocateResult 点击引用定位结果。OK=false 时 Message 为降级提示（不中断整篇答案展示）。
type LocateResult struct {
	OK      bool           `json:"ok"`
	Action  string         `json:"action,omitempty"` // open_pubmed / open_document_at / open_kb_document
	Target  map[string]any `json:"target,omitempty"`
	Message string         `json:"message,omitempty"`
}

// Locate 点击引用角标定位来源（§8.9）：先实时校验权限，再按 source_type 定位；异常降级不越权。
func Locate(db *gorm.DB, user auth.AuthUser, citationID string) (LocateResult, error) {
	c, err := Get(db, user.TenantID, citationID)
	if err != nil {
		return LocateResult{}, err
	}
	return LocateLoaded(db, user, c)
}

// LocateLoaded 对已取出的引用定位（c==nil 视为已删除）。供路由层先做归属校验后复用，避免二次查库。
func LocateLoaded(db *gorm.DB, user auth.AuthUser, c *Citation) (LocateResult, error) {
	if c == nil {
		return LocateResult{OK: false, Message: MsgDeleted}, nil
	}

	var res LocateResult
	switch c.SourceType {
	case "pubmed":
		if c.SourceURL == "" && c.PubmedID == "" {
			res = LocateResult{OK: false, Message: MsgUnavailable}
		} else {
			res = LocateResult{OK: true, Action: "open_pubmed", Target: map[string]any{"url": c.SourceURL, "pubmedId": c.PubmedID, "doi": c.DOI}}
		}
	case "upload", "kb":
		res = locateDocument(db, user, c)
	default:
		res = LocateResult{OK: false, Message: MsgUnavailable}
	}

	result := "成功"
	if !res.OK {
		result = "失败"
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: audit.P(joinRoles(user)),
		ActionType: "citation_click", TargetType: audit.P("citation"), TargetID: audit.P(c.CitationID),
		Result:   result,
		Metadata: map[string]any{"sourceType": c.SourceType, "action": res.Action, "documentId": c.DocumentID, "pubmedId": c.PubmedID, "outcome": res.Message},
	})
	return res, nil
}

// locateDocument 对 upload/kb 来源定位：复核文档权限 + 页码/段落或 chunk 定位，异常降级。
func locateDocument(db *gorm.DB, user auth.AuthUser, c *Citation) LocateResult {
	if c.DocumentID == "" {
		return LocateResult{OK: false, Message: MsgUnavailable}
	}
	var doc docperm.DocumentRow
	_ = db.Raw(`SELECT * FROM documents WHERE document_id = ? AND tenant_id = ?`, c.DocumentID, user.TenantID).Scan(&doc).Error
	if doc.DocumentID == "" {
		return LocateResult{OK: false, Message: MsgDeleted}
	}
	if doc.IsDeleted {
		return LocateResult{OK: false, Message: MsgDeleted} // 原文已删除
	}
	lvl, _ := docperm.Resolve(db, user, doc)
	if lvl == docperm.None {
		return LocateResult{OK: false, Message: MsgUnavailable} // 权限不足，不暴露内容
	}
	// chunk 级 ACL 复核（owner=c03，定位侧与 rag 检索侧共用 chunkacl，避免「检索拦、定位放」越权缺口）：
	// chunk_acl 可严于文档级，命中拒绝即降级，不返回 chunkId/page/section 定位锚点。
	if c.ChunkID != "" {
		acl, _ := chunkacl.Load(db, user.TenantID, c.ChunkID)
		if !chunkacl.Allows(acl, user) {
			return LocateResult{OK: false, Message: MsgUnavailable}
		}
	}
	// chunk 精确定位：有页码/段落则定位，否则降级到文档
	if c.Page == nil && c.ParagraphIndex == nil && c.ChunkID == "" {
		return LocateResult{OK: true, Action: "open_document", Target: map[string]any{"documentId": c.DocumentID}, Message: MsgChunkFail}
	}
	action := "open_document_at"
	if c.SourceType == "kb" {
		action = "open_kb_document"
	}
	return LocateResult{OK: true, Action: action, Target: map[string]any{
		"documentId": c.DocumentID, "kbId": c.KBID, "chunkId": c.ChunkID,
		"page": c.Page, "paragraphIndex": c.ParagraphIndex, "section": c.Section,
	}}
}

func joinRoles(u auth.AuthUser) string {
	out := ""
	for i, r := range u.RoleSlugs {
		if i > 0 {
			out += ","
		}
		out += r
	}
	return out
}
