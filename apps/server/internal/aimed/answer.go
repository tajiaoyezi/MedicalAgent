package aimed

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/citation"
	"medoffice/server/internal/model"
	"medoffice/server/internal/rag"
)

// 固定文案。
const (
	NoResultsText  = "未找到相关文献，建议调整提问关键词。"
	DisclaimerText = "免责声明：本回答由 AI 生成，仅供医学参考，不构成诊断、处方或医嘱，请由医生结合临床实际判断。"
	DraftLabel     = "草稿 / 辅助建议"
)

// 答案生成过程提示（§5.2 / §8 答案生成过程）。
var ProcessSteps = []string{"正在理解问题", "正在检索内容", "正在分析证据", "正在生成回答"}

// Service AIMed 编排服务（持有 RAG 引擎）。
type Service struct {
	rag *rag.Engine
}

func NewService(ragEngine *rag.Engine) *Service { return &Service{rag: ragEngine} }

// AnswerRequest 一次问答输入。
type AnswerRequest struct {
	User         auth.AuthUser
	Conversation *Conversation
	Query        string
	CurrentDocID string
}

// AnswerStats 检索统计（§5.2 找到 N 篇、M 篇重点参考、思考 S 秒）。
type AnswerStats struct {
	Found           int     `json:"found"`
	Key             int     `json:"key"`
	ThinkingSeconds float64 `json:"thinkingSeconds"`
}

// AnswerResult 答案输出。
type AnswerResult struct {
	UserMessageID        string                `json:"userMessageId"`
	MessageID            string                `json:"messageId"`
	Content              string                `json:"content"`
	Process              []string              `json:"process"`
	Stats                AnswerStats           `json:"stats"`
	Citations            []citation.Citation   `json:"citations"`
	NoResults            bool                  `json:"noResults"`
	Draft                bool                  `json:"draft"`
	DraftLabel           string                `json:"draftLabel"`
	Disclaimer           string                `json:"disclaimer"`
	HighRisk             bool                  `json:"highRisk"`
	RiskType             string                `json:"riskType,omitempty"`
	RequiresConfirmation bool                  `json:"requiresConfirmation"`
	PubmedRoute          string                `json:"pubmedRoute"`
	RunID                string                `json:"runId"`
}

// Answer 主问答闭环：建用户消息 → RAG 检索 → 生成带引用答案 → 落消息+引用 → 高风险前置 → 审计。
func (s *Service) Answer(db *gorm.DB, req AnswerRequest) (AnswerResult, error) {
	start := time.Now()
	conv := req.Conversation
	mode := conv.Mode
	if mode == "" {
		mode = string(ModeGeneral)
	}

	userMsgID, err := AddMessage(db, conv.TenantID, conv.UserID, conv.ConversationID, "user", req.Query, mode, nil, nil)
	if err != nil {
		return AnswerResult{}, err
	}
	res := AnswerResult{UserMessageID: userMsgID, Process: ProcessSteps, Draft: true, DraftLabel: DraftLabel, Disclaimer: DisclaimerText}

	// 收集会话已解析上传文件的 document_id
	var uploadDocs []string
	for _, f := range conv.Files() {
		if f.Status == FileParsed && f.DocumentID != "" {
			uploadDocs = append(uploadDocs, f.DocumentID)
		}
	}

	rr, err := s.rag.Retrieve(db, rag.RetrieveRequest{
		User:            req.User,
		ModeLabel:       GetPolicy(Mode(mode)).Label,
		AllowPubmed:     conv.AllowPubmed,
		AllowUpload:     conv.AllowUpload,
		AllowKB:         conv.AllowKB,
		AllowCurrentDoc: conv.AllowCurrentDoc,
		Query:           req.Query,
		UploadDocIDs:    uploadDocs,
		CurrentDocID:    req.CurrentDocID,
		ConversationID:  conv.ConversationID,
	})
	if err != nil {
		return AnswerResult{}, err
	}
	res.RunID = rr.RunID
	res.PubmedRoute = rr.PubmedRoute

	// 未检索到资料：固定提示，不输出无依据诊疗建议（§8.8）
	if len(rr.Candidates) == 0 {
		content := NoResultsText + "\n\n" + DisclaimerText + "（" + DraftLabel + "）"
		msgID, _ := AddMessage(db, conv.TenantID, conv.UserID, conv.ConversationID, "assistant", content, mode, &userMsgID,
			map[string]any{"draft": true, "draftLabel": DraftLabel, "disclaimer": DisclaimerText, "noResults": true})
		res.MessageID = msgID
		res.Content = content
		res.NoResults = true
		res.Stats = AnswerStats{Found: 0, Key: 0, ThinkingSeconds: secondsSince(start)}
		_ = audit.Write(db, audit.Entry{
			TenantID: conv.TenantID, ActorID: audit.P(conv.UserID), ActorRole: audit.P(strings.Join(req.User.RoleSlugs, ",")),
			ActionType: "aimed_answer", TargetType: audit.P("message"), TargetID: audit.P(msgID),
			Result: "成功", Metadata: map[string]any{"noResults": true, "mode": mode},
		})
		return res, nil
	}

	// 生成答案正文：模型 prose（公网经 runChain 脱敏门禁，本期私有化优先）+ 确定性角标拼装保证 [n]↔citations 一一对应
	prose := s.generateProse(db, req.User, req.Query, rr.Candidates)
	content := composeAnswer(prose, rr.Candidates)

	// 高风险前置（§19.2 / §24.7 第三项）：判定+确认 owner=c05，本能力仅前置消费 seam。
	// 在「问题 + 答案正文」上判定，排除固定免责声明（其含诊断/处方/医嘱字样会造成恒为高风险的误判）。
	riskType, high := ClassifyRisk(req.Query + "\n" + prose)
	requiresConfirmation := high && !CanConfirmHighRisk(req.User)

	msgID, err := AddMessage(db, conv.TenantID, conv.UserID, conv.ConversationID, "assistant", content, mode, &userMsgID,
		map[string]any{
			"draft": true, "draftLabel": DraftLabel, "disclaimer": DisclaimerText,
			"found": rr.TotalFound, "key": rr.KeyCount, "thinkingSeconds": secondsSince(start),
			"pubmedRoute": rr.PubmedRoute, "highRisk": high, "riskType": riskType,
			"requiresConfirmation": requiresConfirmation,
		})
	if err != nil {
		return AnswerResult{}, err
	}

	// 落引用集合（角标序号=cite_ref）
	inputs := make([]citation.Input, 0, len(rr.Candidates))
	for _, c := range rr.Candidates {
		inputs = append(inputs, candidateToCitation(c))
	}
	if err := citation.Save(db, conv.TenantID, msgID, inputs); err != nil {
		return AnswerResult{}, err
	}
	cites, _ := citation.ListByMessage(db, conv.TenantID, msgID)

	_ = audit.Write(db, audit.Entry{
		TenantID: conv.TenantID, ActorID: audit.P(conv.UserID), ActorRole: audit.P(strings.Join(req.User.RoleSlugs, ",")),
		ActionType: "aimed_answer", TargetType: audit.P("message"), TargetID: audit.P(msgID),
		Result:   "成功",
		Metadata: map[string]any{"mode": mode, "found": rr.TotalFound, "key": rr.KeyCount, "citations": len(cites), "highRisk": high, "requiresConfirmation": requiresConfirmation},
	})
	if high {
		// 进入 c05 高风险确认链路（subject_type=message）的前置消费审计；writeback_confirmations 由 c05 落
		_ = audit.Write(db, audit.Entry{
			TenantID: conv.TenantID, ActorID: audit.P(conv.UserID), ActorRole: audit.P(strings.Join(req.User.RoleSlugs, ",")),
			ActionType: "aimed_highrisk_gate", TargetType: audit.P("message"), TargetID: audit.P(msgID),
			Result:   "成功",
			Metadata: map[string]any{"riskType": riskType, "subjectType": "message", "subjectId": msgID, "requiresConfirmation": requiresConfirmation, "confirmRoles": []string{"doctor", "reviewer"}},
		})
	}

	res.MessageID = msgID
	res.Content = content
	res.Citations = cites
	res.HighRisk = high
	res.RiskType = riskType
	res.RequiresConfirmation = requiresConfirmation
	res.Stats = AnswerStats{Found: rr.TotalFound, Key: rr.KeyCount, ThinkingSeconds: secondsSince(start)}
	return res, nil
}

// generateProse 调模型生成分析正文（best-effort；不可用则用候选标题抽取式兜底）。
func (s *Service) generateProse(db *gorm.DB, user auth.AuthUser, query string, cands []rag.Candidate) string {
	var ctxBuf strings.Builder
	for _, c := range cands {
		ctxBuf.WriteString(fmt.Sprintf("[%d] %s：%s\n", c.CiteRef, c.SourceTitle, c.Text))
	}
	ictx := model.InvokeContext{TenantID: user.TenantID, ActorID: user.UserID, ActorRole: strings.Join(user.RoleSlugs, ",")}
	prompt := "基于以下带编号的参考资料回答医学问题，关键结论后用 [编号] 标注来源，不得编造无依据内容。\n问题：" + query + "\n参考资料：\n" + ctxBuf.String()
	if g, err := model.InvokeGeneration(db, model.CapChat, model.GenerationRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: prompt}}, Hint: "aimed_answer",
	}, ictx); err == nil && strings.TrimSpace(g.Content) != "" {
		return strings.TrimSpace(g.Content)
	}
	// 兜底：抽取式综合候选标题
	var b strings.Builder
	b.WriteString("根据检索到的文献，相关研究要点如下：")
	for _, c := range cands {
		b.WriteString("\n- " + c.SourceTitle)
	}
	return b.String()
}

// composeAnswer 拼装最终答案：分析正文 + 关键结论（带 [n] 角标，n↔citations 一一对应）+ 结构化参考资料 + 免责声明 + 草稿标记。
func composeAnswer(prose string, cands []rag.Candidate) string {
	var b strings.Builder
	b.WriteString("## 分析\n")
	b.WriteString(prose)
	b.WriteString("\n\n## 关键结论\n")
	for _, c := range cands {
		title := c.SourceTitle
		if title == "" {
			title = "相关来源"
		}
		b.WriteString(fmt.Sprintf("- 依据「%s」的证据，相关结论可参考来源 [%d]。\n", title, c.CiteRef))
	}
	b.WriteString("\n## 参考资料\n")
	for _, c := range cands {
		ref := citation.Citation{
			CiteIndex: c.CiteRef, SourceType: c.SourceType, PubmedID: c.PubmedID, DOI: c.DOI, SourceURL: c.SourceURL,
			DocumentID: c.DocumentID, Page: c.Page, ParagraphIndex: c.ParagraphIndex, Section: c.Section,
			KBID: c.KBID, ChunkID: c.ChunkID, SourceTitle: c.SourceTitle, Journal: c.Journal, Year: c.Year,
		}
		b.WriteString(ref.Render() + "\n")
	}
	b.WriteString("\n---\n")
	b.WriteString(DisclaimerText + "（" + DraftLabel + "）")
	return b.String()
}

// candidateToCitation 把 rag 候选映射为引用输入（cite_index=cite_ref）。
func candidateToCitation(c rag.Candidate) citation.Input {
	return citation.Input{
		CiteIndex: c.CiteRef, SourceType: c.SourceType, PubmedID: c.PubmedID, DOI: c.DOI, SourceURL: c.SourceURL,
		DocumentID: c.DocumentID, Page: c.Page, ParagraphIndex: c.ParagraphIndex, Section: c.Section,
		KBID: c.KBID, ChunkID: c.ChunkID, SourceTitle: c.SourceTitle, Journal: c.Journal, Year: c.Year,
	}
}

func secondsSince(t time.Time) float64 {
	return float64(int(time.Since(t).Seconds()*10)) / 10
}
