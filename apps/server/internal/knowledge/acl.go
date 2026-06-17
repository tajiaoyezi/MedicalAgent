package knowledge

import (
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
)

// 知识库级 ACL 经 c01 document_permissions「per-kb 授予」表达（design 收口：不引入平行 ACL/kb_members 表）：
// KB 级 ACL = 其文档 document_permissions 的聚合。读取/问答=view、上传导入=edit、管理=manage（对齐 PRD §19.1
// 四类 per-kb 资源级 ACL；锚定 c01 auth-rbac，不在 permissions 表自造 kb:* 平台权限点）。

func validPrincipalType(t string) bool { return t == "user" || t == "role" || t == "dept" }
func validLevel(l string) bool {
	switch l {
	case "view", "comment", "edit", "manage", "owner", "none":
		return true
	}
	return false
}

// roleSlugsForIN 供 SQL IN ? 用；空角色返回不可能命中的占位，避免 gorm 生成 IN () 语法错误。
func roleSlugsForIN(u auth.AuthUser) []string {
	if len(u.RoleSlugs) == 0 {
		return []string{"__no_role__"}
	}
	return u.RoleSlugs
}

func deptForMatch(u auth.AuthUser) string {
	if u.DeptID == "" {
		return "__no_dept__"
	}
	return u.DeptID
}

// grantDoc 在单文档落一条 document_permissions（幂等 upsert）。
func grantDoc(db *gorm.DB, tenantID, documentID, pType, pID, level string) error {
	return db.Exec(
		`INSERT INTO document_permissions (tenant_id, document_id, principal_type, principal_id, permission_level)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT (document_id, principal_type, principal_id) DO UPDATE SET permission_level = EXCLUDED.permission_level`,
		tenantID, documentID, pType, pID, level,
	).Error
}

// applyImportGrants 物化 document_acl 写入侧（8.4）：文档入正式库时落 document_permissions。
//   - seed（公共）库 → 授予本租户全部角色 view（人人可读问答），使预置库对全租户可检索。
//   - 并把该 KB 既有正式文档上的 KB 级授权（去重）传播到新文档，使「先授权后导入 / 先导入后授权」聚合一致。
//   - 私有库无既有授权时仅 owner（导入人，doc.owner_id）可读，符合「私有库默认仅创建人」。
func applyImportGrants(db *gorm.DB, tenantID, kbID, documentID string, isSeed bool) error {
	if documentID == "" {
		return nil
	}
	if isSeed {
		var slugs []string
		_ = db.Raw(`SELECT slug FROM roles WHERE tenant_id = ?`, tenantID).Scan(&slugs).Error
		for _, s := range slugs {
			if err := grantDoc(db, tenantID, documentID, "role", s, "view"); err != nil {
				return err
			}
		}
	}
	var existing []struct {
		PT  string `gorm:"column:principal_type"`
		PID string `gorm:"column:principal_id"`
		Lvl string `gorm:"column:permission_level"`
	}
	_ = db.Raw(
		`SELECT DISTINCT dp.principal_type, dp.principal_id, dp.permission_level
		 FROM document_permissions dp
		 WHERE dp.tenant_id = ? AND dp.document_id IN (
		   SELECT document_id FROM kb_documents WHERE kb_id = ? AND document_id IS NOT NULL AND document_id <> ?
		 )`, tenantID, kbID, documentID,
	).Scan(&existing).Error
	for _, g := range existing {
		if err := grantDoc(db, tenantID, documentID, g.PT, g.PID, g.Lvl); err != nil {
			return err
		}
	}
	return refreshMemberCount(db, tenantID, kbID)
}

// CanManageKB 库管理判定（§19.1）：平台管理员 / 库创建人 / 对该库文档持 manage|owner 授权（per-kb 管理级 ACL 授予）。
// 普通 c01 角色不自动等同库管理员（须有 per-kb manage 授予）。
func CanManageKB(db *gorm.DB, u auth.AuthUser, kbID string) (bool, error) {
	var rows []struct {
		CreatedBy *string `gorm:"column:created_by"`
	}
	if err := db.Raw(`SELECT created_by FROM knowledge_bases WHERE tenant_id = ? AND kb_id = ?`, u.TenantID, kbID).Scan(&rows).Error; err != nil {
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
	var n int
	db.Raw(
		`SELECT COUNT(*)::int FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		 WHERE kbd.tenant_id = ? AND kbd.kb_id = ? AND dp.permission_level IN ('manage','owner') AND (
		   (dp.principal_type='user' AND dp.principal_id = ?)
		   OR (dp.principal_type='role' AND dp.principal_id IN ?)
		   OR (dp.principal_type='dept' AND dp.principal_id = ?))`,
		u.TenantID, kbID, u.UserID, roleSlugsForIN(u), deptForMatch(u),
	).Scan(&n)
	return n > 0, nil
}

// GrantKB 知识库级 ACL 授予（8.3）：把 (principal, level) 应用到该 KB 当前所有正式文档的 document_permissions，
// 仅平台管理员或库管理员可操作；授予后刷新 member_count。level: view=读取/问答、edit=上传导入、manage=管理。
func GrantKB(db *gorm.DB, u auth.AuthUser, kbID, pType, pID, level string) error {
	if !validPrincipalType(pType) || !validLevel(level) || pID == "" {
		return ErrInvalidInput
	}
	can, err := CanManageKB(db, u, kbID)
	if err != nil {
		return err
	}
	if !can {
		// 无权管理接口被拒须留痕（spec「普通用户绕过 UI 直接调用被拒绝」AND 写 audit_logs）。
		_ = audit.Write(db, audit.Entry{
			TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
			ActionType: "kb_acl_grant", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID),
			Result: "失败", FailureReason: audit.P("非库管理员，无权授予"),
		})
		return ErrForbidden
	}
	var docs []string
	_ = db.Raw(`SELECT document_id FROM kb_documents WHERE tenant_id = ? AND kb_id = ? AND document_id IS NOT NULL AND is_staging = FALSE`, u.TenantID, kbID).Scan(&docs).Error
	for _, d := range docs {
		if err := grantDoc(db, u.TenantID, d, pType, pID, level); err != nil {
			return err
		}
	}
	_ = refreshMemberCount(db, u.TenantID, kbID)
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_acl_grant", TargetType: audit.P("knowledge_base"), TargetID: audit.P(kbID), Result: "成功",
		Metadata: map[string]any{"principalType": pType, "principalId": pID, "level": level, "docsAffected": len(docs)},
	})
	return nil
}

// refreshMemberCount 物化 member_count（D2/3.2）：对该库正式文档持 read+ 的去重用户数 =
// 文档 owner ∪ （user/role/dept 授权解析到的用户）。
func refreshMemberCount(db *gorm.DB, tenantID, kbID string) error {
	return db.Exec(
		`UPDATE knowledge_bases SET member_count = (
		   SELECT COUNT(DISTINCT uid) FROM (
		     SELECT d.owner_id::text AS uid
		       FROM kb_documents kbd JOIN documents d ON d.document_id = kbd.document_id
		       WHERE kbd.kb_id = ? AND kbd.is_staging = FALSE
		     UNION
		     SELECT u.user_id::text AS uid FROM users u
		       WHERE u.tenant_id = ? AND EXISTS (
		         SELECT 1 FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		         WHERE kbd.kb_id = ? AND kbd.is_staging = FALSE AND dp.permission_level <> 'none' AND (
		           (dp.principal_type='user' AND dp.principal_id = u.user_id::text)
		           OR (dp.principal_type='role' AND dp.principal_id IN (SELECT r.slug FROM roles r JOIN user_roles ur ON ur.role_id = r.role_id WHERE ur.user_id = u.user_id))
		           OR (dp.principal_type='dept' AND dp.principal_id = u.dept_id)))
		   ) m
		 ) WHERE kb_id = ? AND tenant_id = ?`,
		kbID, tenantID, kbID, kbID, tenantID,
	).Error
}

// docGrantVisibleKBSubquery 返回「用户经文档级授权可见的私有库」EXISTS 子句（拼到 ListVisible 的 WHERE）。
// 参数顺序：dp.tenant_id, user_id, roleSlugs(IN), deptID。
func docGrantVisibleKBSubquery() string {
	return `kb.kb_id IN (
		SELECT DISTINCT kbd.kb_id FROM kb_documents kbd
		JOIN document_permissions dp ON dp.document_id = kbd.document_id
		WHERE dp.tenant_id = ? AND dp.permission_level <> 'none' AND (
		  (dp.principal_type='user' AND dp.principal_id = ?)
		  OR (dp.principal_type='role' AND dp.principal_id IN ?)
		  OR (dp.principal_type='dept' AND dp.principal_id = ?)))`
}

// hasKBDocGrant 判定用户是否对某 KB 的任一文档持 read+ 授权（Get 单库可见性用）。
func hasKBDocGrant(db *gorm.DB, u auth.AuthUser, kbID string) bool {
	var n int
	db.Raw(
		`SELECT COUNT(*)::int FROM kb_documents kbd JOIN document_permissions dp ON dp.document_id = kbd.document_id
		 WHERE kbd.tenant_id = ? AND kbd.kb_id = ? AND dp.permission_level <> 'none' AND (
		   (dp.principal_type='user' AND dp.principal_id = ?)
		   OR (dp.principal_type='role' AND dp.principal_id IN ?)
		   OR (dp.principal_type='dept' AND dp.principal_id = ?))`,
		u.TenantID, kbID, u.UserID, roleSlugsForIN(u), deptForMatch(u),
	).Scan(&n)
	return n > 0
}
