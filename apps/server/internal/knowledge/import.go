package knowledge

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/model"
)

// 导入管线语义错误（补充 knowledge.go 的基础错误）。
var (
	ErrRejectedSource = errors.New("来源被红线禁止")  // D4 rejected：未授权商业库/镜像站/下载链接
	ErrNotAuthorized  = errors.New("来源未授权，仅可临时预览") // D4 preview_only：不可入正式公共库
	ErrMissingMeta    = errors.New("缺少必录元数据字段")    // §11.5.1 8 个硬门禁字段缺值
	ErrRedactionBlock = errors.New("脱敏门禁拦截：识别服务不可用或命中敏感信息") // c09 上传闸/出网门禁
)

// D4 授权三态 + 暂存态（与迁移 009 kb_documents.authorization_status CHECK 对齐）。
const (
	AuthPendingPreview = "pending_preview"
	AuthAuthorized     = "authorized"
	AuthPreviewOnly    = "preview_only"
	AuthRejected       = "rejected"
)

// 来源类型（kb_documents.source_type）。
const (
	SrcUpload    = "upload"
	SrcURL       = "url"
	SrcPubMed    = "pubmed"
	SrcPMC       = "pmc"
	SrcWhitelist = "whitelist"
)

// 未授权商业数据库红线黑名单（D4：默认抓取/镜像站/下载链接 → rejected）。
var commercialBlocklist = map[string]bool{
	"wanfangdata.com.cn": true, "www.wanfangdata.com.cn": true,
	"cnki.net": true, "www.cnki.net": true,
	"cqvip.com": true, "www.cqvip.com": true,
}

// ImportRequest 导入入参（来源适配器层的统一输入）。
type ImportRequest struct {
	KBID       string
	SourceType string // upload / url / pubmed / pmc / whitelist
	SourceURL  string // URL/文件来源标识
	Title      string
	// AdminAuthorized：未命中白名单的合法 URL，由管理员显式授权确认（4.4 分支）。
	AdminAuthorized bool
	// C04AuthHint：URL/PMC/PubMed 取数路径由 c04 pubmed-data-service 返回的 RetrievedSource.AuthStatus
	// 作为闸门初始输入（4.6/D4：消费 c04 标记、不从零重建三态）。空表示无 c04 取数标记（如 upload）。
	C04AuthHint string
	// DocumentID：upload 路径已先建 c01 documents（route 层 storage.Put 后传入）；空表示尚无落盘文档。
	DocumentID string
	// PublicNetwork：本次导入是否需要调用公网模型（解析/向量化/抓取）。本期默认 false（公网关闭）。
	PublicNetwork bool
}

// hostOf 解析 URL 取规范化主机名（小写、去端口、去尾点）。仅接受 http/https；非法 URL/非 http(s)/无 host → ""。
// MUST NOT 回退用原始串当 host（否则授权匹配被旁路放宽）。
func hostOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
}

// isCommercialBlocked 红线黑名单匹配：规范化主机名精确等于或为黑名单域的子域（host 以 ".bad" 结尾），
// 使镜像子域（如 mirror.cnki.net）同样被红线阻断。
func isCommercialBlocked(host string) bool {
	if host == "" {
		return false
	}
	for bad := range commercialBlocklist {
		if host == bad || strings.HasSuffix(host, "."+bad) {
			return true
		}
	}
	return false
}

// whitelistHit 按规范化主机名「精确等于 或 为白名单域子域（host 以 .identifier 结尾）」命中白名单规则，
// 返回 (规则ID, 命中)。MUST NOT 用无锚点子串匹配（避免 evil-<id>.example.org / <id>.attacker.com 旁路放行）；
// URL 解析失败不回退原始串。source_identifier 为管理员配置的主机名（可信、不含 LIKE 通配符）。
func whitelistHit(db *gorm.DB, tenantID, sourceURL string) (string, bool) {
	host := hostOf(sourceURL)
	if host == "" {
		return "", false
	}
	var rows []struct {
		ID string `gorm:"column:whitelist_rule_id"`
	}
	_ = db.Raw(
		`SELECT whitelist_rule_id FROM source_whitelist_rules
		 WHERE is_allowed = TRUE AND (tenant_id IS NULL OR tenant_id = ?)
		   AND (LOWER(?) = LOWER(source_identifier) OR LOWER(?) LIKE '%.' || LOWER(source_identifier))
		 LIMIT 1`, tenantID, host, host,
	).Scan(&rows)
	if len(rows) > 0 {
		return rows[0].ID, true
	}
	return "", false
}

// classifyAuthorization 是 D4 授权状态机：来源类型决定默认门，白名单/管理员授权放行。
// 消费 c04 取数授权标记（C04AuthHint）作为初值，c06 在其上叠加白名单/管理员裁决，最终落库裁决以本函数为唯一真值。
// 返回 (status, whitelistRuleID)。
func classifyAuthorization(db *gorm.DB, tenantID string, req ImportRequest) (string, string) {
	switch req.SourceType {
	case SrcUpload:
		// 上传（管理员/库管理员到授权库）→ authorized（上传权限已在 CanUploadToKB 前置校验）。
		return AuthAuthorized, ""
	case SrcPubMed, SrcPMC:
		// PubMed/PMC：合规开放来源，初值取 c04 标记；c04 未给则默认 authorized（白名单体系内）。
		if req.C04AuthHint == AuthRejected {
			return AuthRejected, ""
		}
		if req.C04AuthHint == AuthPreviewOnly {
			return AuthPreviewOnly, ""
		}
		return AuthAuthorized, ""
	case SrcURL, SrcWhitelist:
		host := hostOf(req.SourceURL)
		// 红线：未授权商业库/镜像站/下载链接（含其子域）→ rejected（不抓取不写库）。
		if isCommercialBlocked(host) {
			return AuthRejected, ""
		}
		// c04 已判 rejected 的取数路径直接红线。
		if req.C04AuthHint == AuthRejected {
			return AuthRejected, ""
		}
		// 命中白名单规则 → authorized + 规则 ID 留痕。
		if ruleID, ok := whitelistHit(db, tenantID, req.SourceURL); ok {
			return AuthAuthorized, ruleID
		}
		// 未命中白名单但管理员显式授权 → authorized（授权确认人留痕）。
		if req.AdminAuthorized {
			return AuthAuthorized, ""
		}
		// 授权状态不明确 → preview_only（仅临时预览，禁止写正式公共库）。
		return AuthPreviewOnly, ""
	default:
		return AuthPreviewOnly, ""
	}
}

// CanUploadToKB 上传入口权限分级（kb-import「上传入口与权限分级」/§11.4）。
// PR2 口径：平台管理员（admin:console）→ 任意库；库创建人 → 自建库；普通用户 → 不得写公共/预置库。
// per-kb 上传/导入级 ACL 授予的「库管理员」判定随 PR3（ACL）补齐。
func CanUploadToKB(db *gorm.DB, u auth.AuthUser, kbID string) (bool, error) {
	var rows []struct {
		CreatedBy *string `gorm:"column:created_by"`
		IsSeed    bool    `gorm:"column:is_seed"`
	}
	if err := db.Raw(`SELECT created_by, is_seed FROM knowledge_bases WHERE tenant_id = ? AND kb_id = ?`, u.TenantID, kbID).Scan(&rows).Error; err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, ErrNotFound
	}
	if isPlatformAdmin(u) {
		return true, nil
	}
	if rows[0].CreatedBy != nil && *rows[0].CreatedBy == u.UserID {
		return true, nil
	}
	// 库管理员/上传导入级授予者（对该库文档持 edit|manage|owner per-kb 授予）可上传到自管库
	// （kb-import「知识库管理员仅能上传到自管库」；锚定 c01 document_permissions 授予，非新全局角色）。
	var n int
	db.Raw(
		`SELECT COUNT(*)::int FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		 WHERE kbd.tenant_id = ? AND kbd.kb_id = ? AND dp.permission_level IN ('edit','manage','owner') AND (
		   (dp.principal_type='user' AND dp.principal_id = ?)
		   OR (dp.principal_type='role' AND dp.principal_id IN ?)
		   OR (dp.principal_type='dept' AND dp.principal_id = ?))`,
		u.TenantID, kbID, u.UserID, roleSlugsForIN(u), deptForMatch(u),
	).Scan(&n)
	return n > 0, nil
}

// redactionGateOutbound 是「公网导入前 PHI/PII 脱敏门禁」（5.1/5.2）：调用公网模型（解析/向量化/抓取）前消费
// c09 redaction-gateway；识别失败/不可用 → 禁止公网、降级私有化/离线（本期默认公网关闭，门禁默认拒绝）。
// 返回是否放行公网；不放行即走私有化/离线（不阻断导入本身，仅约束出网）。
func redactionGateOutbound(db *gorm.DB, tenantID, text string) bool {
	v := model.EvaluateRedaction(model.RedactionInput{TenantID: tenantID, Text: text})
	if !v.Available || !v.Passed {
		_ = audit.Write(db, audit.Entry{
			TenantID: tenantID, ActionType: "kb_import_redaction_block", TargetType: audit.P("knowledge_base"),
			Result: "失败", FailureReason: audit.P(v.Reason),
			Metadata: map[string]any{"switchTo": "private_offline"},
		})
		return false
	}
	return true
}

// PreviewImport 执行四段式管线的「来源适配器 → 暂存预览(staging)」：
// 经授权闸门定状态、落 staging kb_documents 行（is_staging=true，与正式库物理隔离，不进正式检索索引）。
// rejected 来源直接红线阻断、不落 staging。返回 kb_document_id。
func PreviewImport(db *gorm.DB, u auth.AuthUser, req ImportRequest) (string, error) {
	can, err := CanUploadToKB(db, u, req.KBID)
	if err != nil {
		return "", err
	}
	if !can {
		return "", ErrForbidden
	}
	// 公网导入需出网时先过脱敏门禁（本期默认公网关闭→不放行→走私有化/离线，不阻断 staging）。
	if req.PublicNetwork {
		_ = redactionGateOutbound(db, u.TenantID, req.Title+" "+req.SourceURL)
	}

	status, ruleID := classifyAuthorization(db, u.TenantID, req)
	if status == AuthRejected {
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
			ActionType: "kb_import_rejected", TargetType: audit.P("knowledge_base"), TargetID: audit.P(req.KBID),
			Result: "失败", FailureReason: audit.P("来源被红线禁止（未授权商业库/镜像站/下载链接）"),
			Metadata: map[string]any{"sourceType": req.SourceType, "sourceUrl": req.SourceURL},
		})
		return "", ErrRejectedSource
	}

	kbDocID := uuid.NewString()
	copyrightStatus := "open_or_licensed"
	if status == AuthPreviewOnly {
		copyrightStatus = "unknown"
	}
	var rulePtr, authByPtr, docPtr any
	if ruleID != "" {
		rulePtr = ruleID
	}
	if status == AuthAuthorized && req.AdminAuthorized && ruleID == "" {
		authByPtr = u.UserID // 管理员显式授权路径留痕授权确认人
	}
	if req.DocumentID != "" {
		docPtr = req.DocumentID
	}
	if err := db.Exec(
		`INSERT INTO kb_documents
		   (kb_document_id, tenant_id, kb_id, document_id, source_url, source_type, imported_by, copyright_status,
		    parse_status, index_status, whitelist_rule_id, authorized_by, authorization_status, is_staging, title)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', 'pending', ?, ?, ?, TRUE, ?)`,
		kbDocID, u.TenantID, req.KBID, docPtr, req.SourceURL, req.SourceType, u.UserID, copyrightStatus,
		rulePtr, authByPtr, status, req.Title,
	).Error; err != nil {
		return "", err
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_import_preview", TargetType: audit.P("knowledge_base"), TargetID: audit.P(req.KBID),
		Result: "成功", Metadata: map[string]any{"kbDocumentId": kbDocID, "sourceType": req.SourceType,
			"sourceUrl": req.SourceURL, "authorizationStatus": status, "whitelistRuleId": ruleID, "authorizedBy": authByPtr},
	})
	return kbDocID, nil
}

// metaComplete 校验 §11.5.1 的 8 个「值 MUST 非空」硬门禁字段（whitelist_rule_id/authorized_by 为条件填充、不校验）。
func metaComplete(r kbDocMetaRow) bool {
	return r.SourceURL != "" && r.SourceType != "" && r.ImportedBy != nil && !r.ImportedAt.IsZero() &&
		r.CopyrightStatus != "" && r.SourceVersion != "" && r.ParseStatus != "" && r.IndexStatus != ""
}

type kbDocMetaRow struct {
	KBID                string     `gorm:"column:kb_id"`
	SourceURL           string     `gorm:"column:source_url"`
	SourceType          string     `gorm:"column:source_type"`
	ImportedBy          *string   `gorm:"column:imported_by"`
	ImportedAt          time.Time `gorm:"column:imported_at"`
	CopyrightStatus     string    `gorm:"column:copyright_status"`
	SourceVersion       string    `gorm:"column:source_version"`
	ParseStatus         string    `gorm:"column:parse_status"`
	IndexStatus         string    `gorm:"column:index_status"`
	AuthorizationStatus string    `gorm:"column:authorization_status"`
	DocumentID          *string   `gorm:"column:document_id"`
	WhitelistRuleID     *string   `gorm:"column:whitelist_rule_id"`
	AuthorizedBy        *string   `gorm:"column:authorized_by"`
}

// ConfirmImport 执行四段式管线的「授权闸门 → 入库」（入库前预览确认 / 人工确认链路）：
// 仅 authorized 且 8 必录字段非空可入正式库（is_staging=false）；preview_only/rejected 不可入；
// 已落盘文档（DocumentID 非空）则入库后入队 c03 解析作业，索引就绪由 c06 索引消费方收尾。
func ConfirmImport(db *gorm.DB, u auth.AuthUser, kbDocID string) error {
	var rows []kbDocMetaRow
	if err := db.Raw(
		`SELECT kb_id, source_url, source_type, imported_by, imported_at, copyright_status, source_version,
		        parse_status, index_status, authorization_status, document_id, whitelist_rule_id, authorized_by
		 FROM kb_documents WHERE tenant_id = ? AND kb_document_id = ?`, u.TenantID, kbDocID,
	).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	r := rows[0]
	can, err := CanUploadToKB(db, u, r.KBID)
	if err != nil {
		return err
	}
	if !can {
		return ErrForbidden
	}
	if r.AuthorizationStatus == AuthRejected {
		return ErrRejectedSource
	}
	if r.AuthorizationStatus != AuthAuthorized {
		return ErrNotAuthorized // preview_only：仅临时预览，禁止写正式公共库
	}
	if !metaComplete(r) {
		return ErrMissingMeta
	}
	// 确认入库：staging → 正式库（物理进入正式检索范围由 D3 的 is_staging=false 标记）。
	if err := db.Exec(`UPDATE kb_documents SET is_staging = FALSE, updated_at = NOW() WHERE tenant_id = ? AND kb_document_id = ?`, u.TenantID, kbDocID).Error; err != nil {
		return err
	}
	// 物化 document_acl（8.4）：seed 公共库授全角色 view、传播 KB 既有授权到新文档、刷新 member_count。
	if r.DocumentID != nil && *r.DocumentID != "" {
		var seedRows []struct {
			IsSeed bool `gorm:"column:is_seed"`
		}
		_ = db.Raw(`SELECT is_seed FROM knowledge_bases WHERE kb_id = ? AND tenant_id = ?`, r.KBID, u.TenantID).Scan(&seedRows)
		_ = applyImportGrants(db, u.TenantID, r.KBID, *r.DocumentID, len(seedRows) > 0 && seedRows[0].IsSeed)
	}
	// 已落盘文档 → 入队 c03 解析（worker 解析/分块/向量化后发「索引就绪」事件，由 c06 索引消费方置 indexed + 刷新计数）。
	if r.DocumentID != nil && *r.DocumentID != "" {
		_ = db.Exec(`UPDATE kb_documents SET parse_status = 'pending', index_status = 'pending' WHERE kb_document_id = ?`, kbDocID).Error
		var ver int
		_ = db.Raw(`SELECT COALESCE(MAX(document_version),1) FROM document_versions WHERE document_id = ?`, *r.DocumentID).Scan(&ver)
		_ = db.Exec(
			`INSERT INTO document_parse_jobs (tenant_id, document_id, document_version, status, triggered_by)
			 VALUES (?, ?, ?, 'pending', 'kb_import')`,
			u.TenantID, *r.DocumentID, ver,
		).Error
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_import_confirm", TargetType: audit.P("knowledge_base"), TargetID: audit.P(r.KBID),
		Result: "成功", Metadata: map[string]any{"kbDocumentId": kbDocID, "sourceType": r.SourceType,
			"sourceUrl": r.SourceURL, "whitelistRuleId": r.WhitelistRuleID, "authorizedBy": r.AuthorizedBy},
	})
	return nil
}

// CancelImport 取消预览（不落正式库、不建索引）：删除 staging 行。
func CancelImport(db *gorm.DB, u auth.AuthUser, kbDocID string) error {
	var rows []struct {
		KBID      string `gorm:"column:kb_id"`
		IsStaging bool   `gorm:"column:is_staging"`
	}
	if err := db.Raw(`SELECT kb_id, is_staging FROM kb_documents WHERE tenant_id = ? AND kb_document_id = ?`, u.TenantID, kbDocID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	can, err := CanUploadToKB(db, u, rows[0].KBID)
	if err != nil {
		return err
	}
	if !can {
		return ErrForbidden
	}
	return db.Exec(`DELETE FROM kb_documents WHERE tenant_id = ? AND kb_document_id = ? AND is_staging = TRUE`, u.TenantID, kbDocID).Error
}

// roleCSV2 本包内审计角色串（routes 包有同名 helper，本包独立一份避免跨包耦合）。
func roleCSV2(u auth.AuthUser) *string {
	if len(u.RoleSlugs) == 0 {
		return nil
	}
	return audit.P(strings.Join(u.RoleSlugs, ","))
}
