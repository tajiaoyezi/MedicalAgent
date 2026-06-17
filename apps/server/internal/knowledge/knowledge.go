// Package knowledge 实现 c06 知识库管理的服务层：知识库卡片查询、确定性排序、
// 创建（kb:create 锚定 c01 auth-rbac）、置顶/权重配置。检索问答（接 c04）、导入管线、
// ACL 过滤等由后续 PR 在本包扩展。数据访问沿用仓库既有「raw SQL via gorm」约定。
package knowledge

import (
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
)

// 服务层语义错误，供路由映射 HTTP 状态码、供 smoke 直接断言。
var (
	ErrForbidden    = errors.New("无权限")
	ErrNotFound     = errors.New("知识库不存在")
	ErrConflict     = errors.New("同名知识库已存在")
	ErrInvalidInput = errors.New("参数不合法")
)

// Card 是知识库首页卡片的 9 个规定字段（§11.2）+ 管理可见性辅助字段。
type Card struct {
	KBID          string  `json:"kbId"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	CreatedByName string  `json:"createdBy"`    // 创建人显示名；seed 库回退「系统预置」
	MemberCount   int     `json:"memberCount"`  // 物化：知识库授权用户去重计数（ACL 阶段刷新）
	DocumentCount int     `json:"documentCount"` // 物化：index_status=indexed 计数（索引就绪事件刷新）
	UpdatedAt     string  `json:"updatedAt"`
	DataSource    string  `json:"dataSource"`
	IsPinned      bool    `json:"isPinned"` // 置顶状态
	IsSeed        bool    `json:"isSeed"`
	ManualWeight  *int    `json:"manualWeight"`
	CanManage     bool    `json:"canManage"` // 当前用户可否配置排序/置顶（PR1：admin 或创建人）
}

type kbRow struct {
	KBID          string  `gorm:"column:kb_id"`
	Name          string  `gorm:"column:name"`
	Description   string  `gorm:"column:description"`
	CreatedBy     *string `gorm:"column:created_by"`
	CreatedByName *string `gorm:"column:created_by_name"`
	IsSeed        bool    `gorm:"column:is_seed"`
	IsPinned      bool    `gorm:"column:is_pinned"`
	ManualWeight  *int    `gorm:"column:manual_weight"`
	DataSource    string  `gorm:"column:data_source"`
	MemberCount   int     `gorm:"column:member_count"`
	DocumentCount int     `gorm:"column:document_count"`
	UpdatedAt     string  `gorm:"column:updated_at"`
}

// 确定性多级排序（§11.3 / D2）：置顶 → 权重降序(NULLS LAST) → 更新时间倒序 → 创建时间倒序。
// 列须用 kb. 限定：JOIN users 后 created_at/updated_at 在两表同名，不限定会 SQLSTATE 42702 歧义。
const orderBy = ` ORDER BY kb.is_pinned DESC, kb.manual_weight DESC NULLS LAST, kb.updated_at DESC, kb.created_at DESC`

const selectCols = `SELECT kb.kb_id, kb.name, kb.description, kb.created_by, u.display_name AS created_by_name,
	kb.is_seed, kb.is_pinned, kb.manual_weight, kb.data_source, kb.member_count, kb.document_count, kb.updated_at
	FROM knowledge_bases kb LEFT JOIN users u ON u.user_id = kb.created_by`

func isPlatformAdmin(u auth.AuthUser) bool { return u.HasPermission("admin:console") }

// canManage：PR1 口径——平台管理员或该库创建人。per-kb 管理级 ACL 授予的「库管理员」判定随 c06 ACL 阶段（PR3）补齐。
func canManage(u auth.AuthUser, createdBy *string) bool {
	if isPlatformAdmin(u) {
		return true
	}
	return createdBy != nil && *createdBy == u.UserID
}

func toCard(u auth.AuthUser, r kbRow) Card {
	name := "系统预置"
	if r.CreatedByName != nil && *r.CreatedByName != "" {
		name = *r.CreatedByName
	}
	return Card{
		KBID: r.KBID, Name: r.Name, Description: r.Description, CreatedByName: name,
		MemberCount: r.MemberCount, DocumentCount: r.DocumentCount, UpdatedAt: r.UpdatedAt,
		DataSource: r.DataSource, IsPinned: r.IsPinned, IsSeed: r.IsSeed, ManualWeight: r.ManualWeight,
		CanManage: canManage(u, r.CreatedBy),
	}
}

// ListVisible 返回当前用户在其租户内可见的知识库（已按确定性多级排序）。
// 可见性（§11.4）：平台管理员见本租户全部；普通用户仅见预设库（is_seed）∪ 自己创建的库。
// 「授权私有库」（per-kb ACL 授予可见）随 c06 ACL 阶段（PR3）并入可见集合。
func ListVisible(db *gorm.DB, u auth.AuthUser) ([]Card, error) {
	q := selectCols + ` WHERE kb.tenant_id = ?`
	args := []any{u.TenantID}
	if !isPlatformAdmin(u) {
		q += ` AND (kb.is_seed = TRUE OR kb.created_by = ?)`
		args = append(args, u.UserID)
	}
	q += orderBy
	var rows []kbRow
	if err := db.Raw(q, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	cards := make([]Card, 0, len(rows))
	for _, r := range rows {
		cards = append(cards, toCard(u, r))
	}
	return cards, nil
}

// Get 取单个知识库卡片（租户隔离 + 可见性校验）。不可见或跨租户一律 ErrNotFound（不泄露存在性）。
func Get(db *gorm.DB, u auth.AuthUser, kbID string) (*Card, error) {
	var rows []kbRow
	if err := db.Raw(selectCols+` WHERE kb.tenant_id = ? AND kb.kb_id = ?`, u.TenantID, kbID).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	r := rows[0]
	if !isPlatformAdmin(u) && !r.IsSeed && (r.CreatedBy == nil || *r.CreatedBy != u.UserID) {
		return nil, ErrNotFound
	}
	card := toCard(u, r)
	return &card, nil
}

// Create 创建一个空知识库（§11.5/§17.3）。授权谓词唯一为租户级 kb:create 权限点（c01 auth-rbac 唯一真值，
// 默认授予 admin），MUST NOT 以 per-kb ACL 表达创建授权（待创建库尚不存在）。返回新 kb_id。
func Create(db *gorm.DB, u auth.AuthUser, name, description, dataSource string) (string, error) {
	if !u.HasPermission("kb:create") {
		return "", ErrForbidden
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrInvalidInput
	}
	kbID := uuid.NewString()
	res := db.Exec(
		`INSERT INTO knowledge_bases (kb_id, tenant_id, name, description, created_by, is_seed, data_source)
		 VALUES (?, ?, ?, ?, ?, FALSE, ?)
		 ON CONFLICT (tenant_id, name) DO NOTHING`,
		kbID, u.TenantID, name, strings.TrimSpace(description), u.UserID, strings.TrimSpace(dataSource),
	)
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", ErrConflict
	}
	return kbID, nil
}

// SetRanking 配置置顶/手动权重（§11.5，仅平台管理员或库管理员）。manualWeight 传 nil 表示清空（回退时间排序）。
// 不改 updated_at（排序即时由 ORDER BY 重算，置顶/权重变更不等同内容更新）。
func SetRanking(db *gorm.DB, u auth.AuthUser, kbID string, isPinned *bool, manualWeight *int, clearWeight bool) error {
	var row struct {
		CreatedBy *string `gorm:"column:created_by"`
	}
	var found []struct {
		CreatedBy *string `gorm:"column:created_by"`
	}
	if err := db.Raw(`SELECT created_by FROM knowledge_bases WHERE tenant_id = ? AND kb_id = ?`, u.TenantID, kbID).Scan(&found).Error; err != nil {
		return err
	}
	if len(found) == 0 {
		return ErrNotFound
	}
	row = found[0]
	if !canManage(u, row.CreatedBy) {
		return ErrForbidden
	}
	if isPinned != nil {
		if err := db.Exec(`UPDATE knowledge_bases SET is_pinned = ? WHERE tenant_id = ? AND kb_id = ?`, *isPinned, u.TenantID, kbID).Error; err != nil {
			return err
		}
	}
	if clearWeight {
		if err := db.Exec(`UPDATE knowledge_bases SET manual_weight = NULL WHERE tenant_id = ? AND kb_id = ?`, u.TenantID, kbID).Error; err != nil {
			return err
		}
	} else if manualWeight != nil {
		if err := db.Exec(`UPDATE knowledge_bases SET manual_weight = ? WHERE tenant_id = ? AND kb_id = ?`, *manualWeight, u.TenantID, kbID).Error; err != nil {
			return err
		}
	}
	return nil
}
