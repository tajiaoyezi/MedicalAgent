package routes

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/aimed"
	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/writeback"
)

// RegisterWriteback 挂载 c05 写回确认网关（AI 改文档的唯一服务端收口）：
// preview（四要素 + 策略 + 风险 + 权限）/ confirm（document 写回门禁 + 落确认记录 + doc_ai 最近任务）/
// dispatch-confirm（message/translation_job 下发前高风险确认，task 4.7）。
func RegisterWriteback(r *gin.Engine, db *gorm.DB, svc *editor.Service) {
	// 解析 bridgeToken → 编辑器会话 + 文档，并做租户/归属/存在性校验。
	resolveDoc := func(c *gin.Context, user auth.AuthUser, bridgeToken string) (*editor.EditorSession, docperm.DocumentRow, bool) {
		session := svc.Sessions.GetByBridgeToken(bridgeToken)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效或过期 token", "permitted": false})
			return nil, docperm.DocumentRow{}, false
		}
		if session.TenantID != user.TenantID || session.UserID != user.UserID {
			c.JSON(http.StatusForbidden, gin.H{"error": "跨租户或会话不匹配", "permitted": false})
			return nil, docperm.DocumentRow{}, false
		}
		doc, found, _ := getDoc(db, session.DocumentID, user.TenantID)
		if !found || doc.IsDeleted {
			c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在", "permitted": false})
			return nil, docperm.DocumentRow{}, false
		}
		return session, doc, true
	}

	// ── 写回前四要素预览：原文 / 修改后 / 修改说明 / 影响范围 + 默认策略 + 风险 + 权限 + 免责声明 ──
	r.POST("/api/writeback/preview", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			BridgeToken   string `json:"bridgeToken"`
			OperationType string `json:"operationType"`
			OriginalText  string `json:"originalText"`
			ModifiedText  string `json:"modifiedText"`
			Explanation   string `json:"explanation"`
			ConfirmedScope string `json:"confirmedScope"`
		}
		_ = c.ShouldBindJSON(&body)
		if !writeback.IsKnownOperation(body.OperationType) {
			httpx.Fail(c, 400, "未知 AI 操作类型")
			return
		}
		_, doc, ok := resolveDoc(c, user, body.BridgeToken)
		if !ok {
			return
		}
		level, _ := docperm.Resolve(db, user, doc)
		strat, _ := writeback.StrategyFor(body.OperationType)
		riskType, high := writeback.ClassifyRisk(body.OriginalText + "\n" + body.ModifiedText)
		canConfirm := writeback.CanConfirm(user)
		canApply := docperm.CanEdit(level)
		canCopy := docperm.CanCopy(level)

		actions := []string{}
		if canApply && !(high && !canConfirm) {
			actions = append(actions, "apply")
		}
		if canCopy {
			actions = append(actions, "copy")
		}
		if high && !canConfirm {
			actions = append(actions, "submit_review") // 普通用户高风险仅能提交审核
		}
		actions = append(actions, "cancel")

		c.JSON(http.StatusOK, gin.H{
			"fourElements": gin.H{
				"originalText": body.OriginalText, "modifiedText": body.ModifiedText,
				"explanation": body.Explanation, "impactScope": strat.ImpactScope,
			},
			"strategy":                    strat,
			"risk":                        gin.H{"riskType": riskType, "high": high},
			"requiresHighRiskConfirmation": high && !canConfirm,
			"permission":                  gin.H{"canApply": canApply, "canCopy": canCopy},
			"disclaimer":                  aimed.DisclaimerText,
			"actions":                     actions,
		})
	})

	// ── document 写回确认：冲突/权限/高风险角色门禁 + 落 writeback_confirmations + 写 audit + doc_ai 最近任务 upsert ──
	r.POST("/api/writeback/confirm", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			BridgeToken      string `json:"bridgeToken"`
			OperationType    string `json:"operationType"`
			Action           string `json:"action"` // apply | copy | submit_review
			OriginalText     string `json:"originalText"`
			ModifiedText     string `json:"modifiedText"`
			ExpectedRevision string `json:"expectedRevision"`
			ConfirmedScope   string `json:"confirmedScope"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Action != "apply" && body.Action != "copy" && body.Action != "submit_review" {
			httpx.Fail(c, 400, "无效的确认动作")
			return
		}
		if !writeback.IsKnownOperation(body.OperationType) {
			httpx.Fail(c, 400, "未知 AI 操作类型")
			return
		}
		session, doc, ok := resolveDoc(c, user, body.BridgeToken)
		if !ok {
			return
		}
		level, _ := docperm.Resolve(db, user, doc)

		// 冲突门禁（仅就地写回 apply 路径）：读取时 revision 与当前不一致即阻止，MUST NOT 用过期结果覆盖（§9.9）。
		if body.Action == "apply" {
			revision, _, _ := svc.Sessions.Snapshot(session)
			if body.ExpectedRevision == "" || body.ExpectedRevision != revision {
				c.JSON(http.StatusConflict, gin.H{"error": "文档已变更，请重新读取上下文", "permitted": false, "staleRevision": true})
				return
			}
		}
		// 权限 + 高风险角色统一服务端裁决（共享 writeback.GateDocumentWrite，与 smoke 同一决策）。
		riskType, high := writeback.ClassifyRisk(body.ModifiedText)
		canConfirm := writeback.CanConfirm(user)
		decision := writeback.GateDocumentWrite(body.Action, docperm.CanEdit(level), docperm.CanCopy(level), high, canConfirm)
		if !decision.Allowed {
			resp := gin.H{"error": decision.Reason}
			if decision.Review {
				resp["requiresHighRiskConfirmation"] = true
			}
			c.JSON(decision.Status, resp)
			return
		}
		confirmedRole := ""
		if high && canConfirm {
			confirmedRole = writeback.ConfirmedRole(user)
		}

		// output_version_id 不取自客户端：确认时新版本尚未由 c02 保存回调生成，
		// 待回调落 document_versions 后经 writeback.SetOutputVersion 异步回填（避免客户端注入跨租户/不存在的 version_id）。
		confID, err := writeback.Record(db, writeback.RecordInput{
			TenantID: user.TenantID, SubjectType: "document", SubjectID: doc.DocumentID,
			ConfirmedBy: user.UserID, ConfirmedRole: confirmedRole, ConfirmedScope: body.ConfirmedScope,
			RiskType: riskType, BeforeHash: writeback.ContentHash(body.OriginalText), AfterHash: writeback.ContentHash(body.ModifiedText),
			Action: body.Action, OperationType: body.OperationType,
			ActorRole: strings.Join(user.RoleSlugs, ","), AuditAction: "writeback_confirm",
			AuditMeta: map[string]any{"documentId": doc.DocumentID, "operationType": body.OperationType},
		})
		if err != nil {
			httpx.Fail(c, 500, "确认记录写入失败")
			return
		}
		// doc_ai 写入最近任务（一次写回一条，幂等键 ref_type+ref_id=writeback_ref）。
		status := "confirmed"
		if body.Action == "submit_review" {
			status = "pending_review"
		}
		upsertDocAiRecentTask(db, user, confID, body.OperationType, doc, status)

		if body.Action == "submit_review" {
			c.JSON(http.StatusOK, gin.H{"approved": false, "submittedForReview": true, "confirmationId": confID})
			return
		}
		strat, _ := writeback.StrategyFor(body.OperationType)
		if body.Action == "copy" {
			strat = writeback.StrategyForCopy(body.OperationType)
		}
		c.JSON(http.StatusOK, gin.H{
			"approved": true, "confirmationId": confID, "action": body.Action,
			"bridgeMethod": strat.BridgeMethod, "writebackSource": strat.WritebackSource,
		})
	})

	// ── message / translation_job 下发前高风险确认（task 4.7：AIMed 答案 c04 / kb_qa c06 / 译文 c07 三类产生方复用本链路）──
	r.POST("/api/writeback/dispatch-confirm", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			SubjectType string `json:"subjectType"` // message | translation_job
			SubjectID   string `json:"subjectId"`
			Content     string `json:"content"`
			Action      string `json:"action"` // dispatch | submit_review
		}
		_ = c.ShouldBindJSON(&body)
		if body.SubjectType != "message" && body.SubjectType != "translation_job" {
			httpx.Fail(c, 400, "无效的 subjectType")
			return
		}
		if _, err := uuid.Parse(body.SubjectID); err != nil {
			httpx.Fail(c, 400, "无效的 subjectId")
			return
		}
		// message 主体须按本人归属校验（c04 per-user 隔离：同租户他人不可代为确认下发）。
		if body.SubjectType == "message" {
			msg, _ := aimed.GetMessage(db, user.TenantID, user.UserID, body.SubjectID)
			if msg == nil {
				httpx.Fail(c, 404, "消息不存在")
				return
			}
		}
		// translation_job 主体由 c07 拥有（translation_jobs 表随 c07 落地），本期消费其稳定 job_id 作 subject_id，不向 c04 写 message 行。

		riskType, high := writeback.ClassifyRisk(body.Content)
		if !high {
			// 非高风险无需进入确认链路，可直接下发（不落确认记录）。
			c.JSON(http.StatusOK, gin.H{"approved": true, "highRisk": false})
			return
		}
		canConfirm := writeback.CanConfirm(user)
		if !canConfirm {
			if body.Action != "submit_review" {
				c.JSON(http.StatusForbidden, gin.H{"error": "高风险内容需医生或授权审核确认后下发，仅可提交审核", "requiresHighRiskConfirmation": true})
				return
			}
			confID, err := recordDispatch(db, user, body.SubjectType, body.SubjectID, riskType, "submit_review", "")
			if err != nil {
				httpx.Fail(c, 500, "确认记录写入失败")
				return
			}
			c.JSON(http.StatusOK, gin.H{"approved": false, "submittedForReview": true, "confirmationId": confID})
			return
		}
		confID, err := recordDispatch(db, user, body.SubjectType, body.SubjectID, riskType, "dispatch", writeback.ConfirmedRole(user))
		if err != nil {
			httpx.Fail(c, 500, "确认记录写入失败")
			return
		}
		c.JSON(http.StatusOK, gin.H{"approved": true, "confirmationId": confID})
	})
}

// recordDispatch 落 message/translation_job 下发前确认记录（subject 多态键，复用同一 writeback_confirmations 表与同一裁决）。
func recordDispatch(db *gorm.DB, user auth.AuthUser, subjectType, subjectID, riskType, action, confirmedRole string) (string, error) {
	return writeback.Record(db, writeback.RecordInput{
		TenantID: user.TenantID, SubjectType: subjectType, SubjectID: subjectID,
		ConfirmedBy: user.UserID, ConfirmedRole: confirmedRole, RiskType: riskType, Action: action,
		ActorRole: strings.Join(user.RoleSlugs, ","), AuditAction: "writeback_dispatch_confirm",
		AuditMeta: map[string]any{"subjectType": subjectType},
	})
}

// upsertDocAiRecentTask 投递「在线文档 AI 操作」最近任务条目：ref_type=writeback_confirmation、ref_id=writeback_ref，
// title=「AI 操作类型 · 目标文档名」（文档名缺失回退仅操作类型），title_preview 取前 10 字，幂等键 (tenant,user,ref_type,ref_id)。
func upsertDocAiRecentTask(db *gorm.DB, user auth.AuthUser, confID, operationType string, doc docperm.DocumentRow, status string) {
	title := operationType
	if doc.Name != "" {
		title = operationType + " · " + doc.Name
	}
	preview := title
	if rr := []rune(title); len(rr) > 10 {
		preview = string(rr[:10])
	}
	_ = db.Exec(
		`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, title_preview, status, ref_type, ref_id, related_document_id, updated_at)
		 VALUES (?, ?, ?, '在线文档 AI 操作', ?, ?, ?, 'writeback_confirmation', ?, ?, NOW())
		 ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
		 DO UPDATE SET title = EXCLUDED.title, title_preview = EXCLUDED.title_preview, status = EXCLUDED.status,
		   related_document_id = EXCLUDED.related_document_id, updated_at = NOW(), deleted_at = NULL`,
		uuid.NewString(), user.TenantID, user.UserID, title, preview, status, confID, doc.DocumentID,
	).Error
	_ = audit.Write(db, audit.Entry{
		TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
		ActionType: "recent_task_upsert", TargetType: audit.P("writeback_confirmation"), TargetID: audit.P(confID),
		Result: "成功", Metadata: map[string]any{"source": "在线文档 AI 操作", "documentId": doc.DocumentID},
	})
}
