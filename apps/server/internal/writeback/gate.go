package writeback

// Decision 是 document 写回的服务端裁决结果（不含 revision 冲突，冲突由调用方单独比对）。
type Decision struct {
	Allowed bool   // 放行写回（apply/copy）
	Review  bool   // 仅可提交审核（高风险普通用户的 apply）
	Status  int    // 被拒/受限时的 http 状态
	Reason  string // 提示文案
}

// GateDocumentWrite 裁决 document 写回是否放行（§9.9 权限 + §19.2 高风险角色裁决）：
//   - apply（应用到文档=最终确认）：需编辑权限；高风险且无 highrisk:confirm 时仅可提交审核（MUST NOT 直接覆盖权威文档）。
//   - copy（生成副本=草稿副本）：需复制权限；副本不覆盖原文、属「生成草稿」，普通用户对高风险内容亦可生成副本。
//   - submit_review（提交审核）：始终记录为待审核。
func GateDocumentWrite(action string, canEdit, canCopy, high, canConfirm bool) Decision {
	switch action {
	case "apply":
		if !canEdit {
			return Decision{Status: 403, Reason: "无编辑权限，仅可生成副本或查看"}
		}
		if high && !canConfirm {
			return Decision{Review: true, Status: 403, Reason: "高风险写回需医生或授权审核确认，仅可提交审核"}
		}
		return Decision{Allowed: true}
	case "copy":
		if !canCopy {
			return Decision{Status: 403, Reason: "无复制权限"}
		}
		return Decision{Allowed: true}
	case "submit_review":
		return Decision{Allowed: true, Review: true}
	default:
		return Decision{Status: 400, Reason: "无效的确认动作"}
	}
}
