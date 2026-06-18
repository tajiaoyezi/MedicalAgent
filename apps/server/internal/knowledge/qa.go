package knowledge

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
)

// VisibleKBIDs 返回当前用户可访问的 kb_id 集合（§11.4：预设库 ∪ 自建库；授权私有库随 ACL 阶段并入）。
// 作为知识库问答/检索的数据源可见性边界——越权 kb 不进检索范围。
func VisibleKBIDs(db *gorm.DB, u auth.AuthUser) ([]string, error) {
	cards, err := ListVisible(db, u)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(cards))
	for _, c := range cards {
		ids = append(ids, c.KBID)
	}
	return ids, nil
}

// StartKBQA 创建一个知识库问答会话：module=kb_qa（c04 owner 枚举）、source=「医疗知识库问答」（§6.4），
// 数据源仅知识库（allow_kb=true，其余 false，区别于 AIMed 模式 policy）。复用 c04 conversations 表、不自建会话表。
func StartKBQA(db *gorm.DB, u auth.AuthUser, title string) (string, error) {
	convID, err := aimed.CreateConversation(db, u.TenantID, u.UserID, aimed.ModuleKBQA, "", strings.TrimSpace(title))
	if err != nil {
		return "", err
	}
	if err := db.Exec(
		`UPDATE conversations SET allow_kb = TRUE, allow_pubmed = FALSE, allow_upload = FALSE, allow_current_doc = FALSE
		 WHERE conversation_id = ? AND tenant_id = ? AND user_id = ?`,
		convID, u.TenantID, u.UserID,
	).Error; err != nil {
		return "", err
	}
	return convID, nil
}

// AskKB 知识库问答一轮（§11.7）：校验会话归属与 module=kb_qa、按可见集合裁剪 kb 数据源选择、
// 复用 c04 aimed.Answer 检索 chunk→rerank→生成带引用答案（含 §19.3 免责声明 / 草稿标记 / 无召回不臆造 /
// 高风险经 c05 message 级确认链路前置消费），并持久化 kb 选择（恢复用）+ upsert 最近任务（§6.4）。
func AskKB(db *gorm.DB, svc *aimed.Service, u auth.AuthUser, convID string, kbIDs []string, query string) (*aimed.AnswerResult, error) {
	if strings.TrimSpace(query) == "" {
		return nil, ErrInvalidInput
	}
	conv, err := aimed.GetConversation(db, u.TenantID, u.UserID, convID)
	if err != nil {
		if err == aimed.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if conv.Module != aimed.ModuleKBQA {
		return nil, ErrInvalidInput // 非知识库问答会话不走本入口（不污染 AIMed 会话）
	}

	// 数据源选择：按可见集合裁剪 kb 选择（越权 kb 不进检索范围，§11.9 读取侧前置）。
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
		return nil, ErrForbidden // 指定的 kb 全部越权
	}
	if len(kbIDs) == 0 {
		scope = visible // 未指定 = 全部可见库
	}

	res, err := svc.Answer(db, aimed.AnswerRequest{
		User:         u,
		Conversation: conv,
		Query:        query,
		KBIDs:        scope,
	})
	if err != nil {
		return nil, err
	}

	// 持久化本轮 kb 选择到 user 消息 metadata（§6.6 恢复「知识库选择」）。
	if res.UserMessageID != "" {
		sel, _ := json.Marshal(map[string]any{"kbIds": scope})
		_ = db.Exec(`UPDATE messages SET metadata = metadata || ?::jsonb WHERE message_id = ? AND tenant_id = ?`, string(sel), res.UserMessageID, u.TenantID).Error
	}

	// upsert 最近任务（§6.4：source=医疗知识库问答、ref_type=conversation、ref_id=conversation_id；
	// 标题取最初提问、(ref_type,ref_id) 幂等，同会话多轮只更新 updated_at 不改标题）。
	upsertKBQARecentTask(db, u, convID, query)
	// 问答行为审计 + 问答日志（9.2）：记录用户/tenant_id/所选 kb_id/查询/返回引用/时间，供管理员事后查看（9.3）。
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_qa", TargetType: audit.P("conversation"), TargetID: audit.P(convID),
		Result: "成功", Metadata: map[string]any{
			"messageId": res.MessageID, "query": query, "kbIds": scope,
			"citationCount": len(res.Citations), "highRisk": res.HighRisk,
			"requiresConfirmation": res.RequiresConfirmation,
		},
	})
	return &res, nil
}

func titlePreview(s string) string {
	if r := []rune(s); len(r) > 10 {
		return string(r[:10])
	}
	return s
}

func upsertKBQARecentTask(db *gorm.DB, u auth.AuthUser, convID, firstQuery string) {
	title := strings.TrimSpace(firstQuery)
	if title == "" {
		title = "知识库问答"
	}
	_ = db.Exec(
		`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, title_preview, status, ref_type, ref_id, updated_at)
		 VALUES (?, ?, ?, '医疗知识库问答', ?, ?, 'answered', 'conversation', ?, NOW())
		 ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
		 DO UPDATE SET updated_at = NOW(), deleted_at = NULL`,
		uuid.NewString(), u.TenantID, u.UserID, title, titlePreview(title), convID,
	).Error
}
