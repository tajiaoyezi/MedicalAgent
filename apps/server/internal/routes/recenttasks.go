package routes

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/httpx"
)

var sourceValues = map[string]bool{
	"AIMed 学术助手": true,
	"医疗知识库问答":    true,
	"医疗数字员工":     true,
	"医学翻译":       true,
	"在线文档 AI 操作": true,
	"模板生成文档":     true,
}

const digitalStaffSource = "医疗数字员工"

func groupByTime(t time.Time) string {
	diff := time.Since(t)
	day := 24 * time.Hour
	switch {
	case diff < day:
		return "today"
	case diff < 7*day:
		return "7d"
	case diff < 30*day:
		return "30d"
	case diff < 365*day:
		return "1y"
	default:
		return "all"
	}
}

func titlePreviewOf(title string) string {
	if r := []rune(title); len(r) > 10 {
		return string(r[:10])
	}
	return title
}

// RegisterRecentTasks 复刻 routes/recent-tasks.ts，并补 c05 六类来源聚合/恢复编排/关联文档删除差异。
func RegisterRecentTasks(r *gin.Engine, db *gorm.DB) {
	r.GET("/api/recent-tasks", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var filter []string
		if s := c.Query("sources"); s != "" {
			for _, x := range strings.Split(s, ",") {
				if x != "" {
					filter = append(filter, x)
				}
			}
		}
		var rows []struct {
			TaskID            string    `gorm:"column:task_id"`
			Source            string    `gorm:"column:source"`
			Title             string    `gorm:"column:title"`
			TitlePreview      *string   `gorm:"column:title_preview"`
			Status            *string   `gorm:"column:status"`
			RefType           *string   `gorm:"column:ref_type"`
			RefID             *string   `gorm:"column:ref_id"`
			RelatedDocumentID *string   `gorm:"column:related_document_id"`
			UpdatedAt         time.Time `gorm:"column:updated_at"`
		}
		_ = db.Raw(
			`SELECT task_id, source, title, title_preview, status, ref_type, ref_id, related_document_id, updated_at
			 FROM recent_tasks WHERE tenant_id = ? AND user_id = ? AND deleted_at IS NULL
			 ORDER BY updated_at DESC`, user.TenantID, user.UserID,
		).Scan(&rows).Error

		tasks := []gin.H{}
		for _, row := range rows {
			if len(filter) > 0 && !contains(filter, row.Source) {
				continue
			}
			preview := titlePreviewOf(row.Title)
			if row.TitlePreview != nil && *row.TitlePreview != "" {
				preview = *row.TitlePreview
			}
			// 继续追问仅会话类来源（AIMed / 医疗知识库问答，ref_type=conversation）；数字员工占位不可恢复（§22.2）。
			canContinue := row.RefType != nil && *row.RefType == "conversation"
			restorable := row.Source != digitalStaffSource
			tasks = append(tasks, gin.H{
				"taskId": row.TaskID, "source": row.Source, "title": row.Title,
				"titlePreview": preview, "refType": row.RefType, "refId": row.RefID,
				"relatedDocumentId": row.RelatedDocumentID, "status": row.Status,
				"updatedAt": row.UpdatedAt, "timeGroup": groupByTime(row.UpdatedAt),
				"restorable": restorable, "canContinue": canContinue,
			})
		}
		c.JSON(http.StatusOK, gin.H{"tasks": tasks})
	})

	r.POST("/api/recent-tasks", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			Source  string `json:"source"`
			Title   string `json:"title"`
			RefType string `json:"refType"`
			RefID   string `json:"refId"`
		}
		_ = c.ShouldBindJSON(&body)
		if body.Source == "" || body.Title == "" {
			httpx.Fail(c, 400, "缺少 source 或 title")
			return
		}
		if !sourceValues[body.Source] {
			httpx.Fail(c, 400, "无效的 source")
			return
		}
		taskID := uuid.NewString()
		_ = db.Exec(
			`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, title_preview, ref_type, ref_id, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())
			 ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
			 DO UPDATE SET title = EXCLUDED.title, title_preview = EXCLUDED.title_preview, updated_at = NOW(), deleted_at = NULL`,
			taskID, user.TenantID, user.UserID, body.Source, body.Title, titlePreviewOf(body.Title), nullIfEmpty(body.RefType), nullIfEmpty(body.RefID),
		).Error
		c.JSON(http.StatusOK, gin.H{"taskId": taskID})
	})

	// ── 恢复分发器：仅凭 ref_type 判定回源表，返回各来源恢复描述（详情由各来源 owner 保证）──
	r.GET("/api/recent-tasks/:id/restore", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var task struct {
			Source            string  `gorm:"column:source"`
			RefType           *string `gorm:"column:ref_type"`
			RefID             *string `gorm:"column:ref_id"`
			RelatedDocumentID *string `gorm:"column:related_document_id"`
		}
		_ = db.Raw(`SELECT source, ref_type, ref_id, related_document_id FROM recent_tasks
			WHERE task_id = ? AND tenant_id = ? AND user_id = ? AND deleted_at IS NULL`,
			c.Param("id"), user.TenantID, user.UserID).Scan(&task)
		if task.Source == "" {
			httpx.Fail(c, 404, "任务不存在")
			return
		}
		if task.Source == digitalStaffSource {
			c.JSON(http.StatusOK, gin.H{"restorable": false, "source": task.Source, "planned": true, "message": "医疗数字员工执行历史规划中，暂不可恢复"})
			return
		}
		if task.RefType == nil || task.RefID == nil {
			httpx.Fail(c, 404, "该任务无可恢复来源")
			return
		}
		switch *task.RefType {
		case "conversation":
			// AIMed（c04）/ kb_qa（c06）会话：回源 conversations，可续聊；检索源六维过滤由 c04/c06 召回前执行。
			action := "open_aimed"
			if task.Source == "医疗知识库问答" {
				action = "open_kb_qa"
			}
			c.JSON(http.StatusOK, gin.H{"restorable": true, "source": task.Source, "refType": "conversation",
				"conversationId": *task.RefID, "action": action, "canContinue": true})
		case "writeback_confirmation":
			// 在线文档 AI：回源 writeback_confirmations 单次操作记录，从非哈希字段恢复选区/操作类型/输出结果。
			var conf struct {
				SubjectID       string  `gorm:"column:subject_id"`
				ConfirmedScope  *string `gorm:"column:confirmed_scope"`
				OperationType   *string `gorm:"column:operation_type"`
				OutputVersionID *string `gorm:"column:output_version_id"`
			}
			_ = db.Raw(`SELECT subject_id, confirmed_scope, operation_type, output_version_id
				FROM writeback_confirmations WHERE confirmation_id = ? AND tenant_id = ? AND subject_type = 'document'`,
				*task.RefID, user.TenantID).Scan(&conf)
			if conf.SubjectID == "" {
				httpx.Fail(c, 404, "写回记录不存在")
				return
			}
			_, docFound, _ := getDoc(db, conf.SubjectID, user.TenantID)
			c.JSON(http.StatusOK, gin.H{"restorable": true, "source": task.Source, "refType": "writeback_confirmation",
				"documentId": conf.SubjectID, "documentAvailable": docFound,
				"confirmedScope": conf.ConfirmedScope, "operationType": conf.OperationType, "outputVersionId": conf.OutputVersionID,
				"action": "open_document_locate", "canContinue": false})
		case "translation_job":
			// 医学翻译（c07）：回源 translation_jobs（随 c07 落地），本期返回指针，详情由 c07 保证。
			c.JSON(http.StatusOK, gin.H{"restorable": true, "source": task.Source, "refType": "translation_job",
				"jobId": *task.RefID, "action": "open_translation", "canContinue": false})
		case "document":
			// 模板生成（c08）：回源 documents 行主键。
			_, docFound, _ := getDoc(db, *task.RefID, user.TenantID)
			c.JSON(http.StatusOK, gin.H{"restorable": true, "source": task.Source, "refType": "document",
				"documentId": *task.RefID, "documentAvailable": docFound, "action": "open_document", "canContinue": false})
		default:
			httpx.Fail(c, 400, "未知 ref_type")
		}
	})

	r.PATCH("/api/recent-tasks/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		var body struct {
			Title string `json:"title"`
		}
		_ = c.ShouldBindJSON(&body)
		if strings.TrimSpace(body.Title) == "" {
			httpx.Fail(c, 400, "缺少标题")
			return
		}
		t := strings.TrimSpace(body.Title)
		_ = db.Exec(
			`UPDATE recent_tasks SET title = ?, title_preview = ?, updated_at = NOW()
			 WHERE task_id = ? AND tenant_id = ? AND user_id = ?`,
			t, titlePreviewOf(t), id, user.TenantID, user.UserID,
		).Error
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.DELETE("/api/recent-tasks/:id", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		id := c.Param("id")
		var body struct {
			DeleteLinkedDocument bool `json:"deleteLinkedDocument"`
		}
		_ = c.ShouldBindJSON(&body)

		var task struct {
			TaskID            string  `gorm:"column:task_id"`
			RefType           *string `gorm:"column:ref_type"`
			RefID             *string `gorm:"column:ref_id"`
			RelatedDocumentID *string `gorm:"column:related_document_id"`
		}
		_ = db.Raw(`SELECT task_id, ref_type, ref_id, related_document_id FROM recent_tasks WHERE task_id = ? AND tenant_id = ? AND user_id = ?`, id, user.TenantID, user.UserID).Scan(&task)
		if task.TaskID == "" {
			httpx.Fail(c, 404, "任务不存在")
			return
		}

		// 仅勾选「同时删除关联文档」时按各来源解析关联文档对象（doc_ai/translation/template 有，会话来源/数字员工无）；
		// 解析得到的文档删除仍交 c01 删除规则执行（ACL 校验 + 软删进回收站 + 删除审计）。ref_id/related 为空时为空操作。
		if body.DeleteLinkedDocument {
			linkedDocID := resolveLinkedDocumentID(task.RelatedDocumentID, task.RefType, task.RefID)
			if linkedDocID != "" {
				doc, found, _ := getDoc(db, linkedDocID, user.TenantID)
				if found {
					level, _ := docperm.Resolve(db, user, doc)
					if !docperm.CanEdit(level) && level != docperm.Manage {
						httpx.Fail(c, 403, "无删除关联文档权限")
						return
					}
					_ = db.Exec(`UPDATE documents SET is_deleted = TRUE WHERE document_id = ?`, linkedDocID).Error
					_ = audit.Write(db, audit.Entry{
						TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
						ActionType: "file_delete", TargetType: audit.P("document"), TargetID: audit.P(linkedDocID),
						Result: "成功", Metadata: map[string]any{"fromRecentTask": id},
					})
				}
			}
		}

		// §6.7「同步更新历史记录」：对 recent_tasks 该条软删即完整达成；未勾选删关联文档时 MUST NOT 改动任何来源源表。
		_ = db.Exec(`UPDATE recent_tasks SET deleted_at = NOW() WHERE task_id = ?`, id).Error
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/api/recent-tasks/batch-delete", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		var body struct {
			TaskIDs              []string `json:"taskIds"`
			DeleteLinkedDocument bool     `json:"deleteLinkedDocument"`
		}
		_ = c.ShouldBindJSON(&body)
		for _, id := range body.TaskIDs {
			_ = db.Exec(`UPDATE recent_tasks SET deleted_at = NOW() WHERE task_id = ? AND tenant_id = ? AND user_id = ?`, id, user.TenantID, user.UserID).Error
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": len(body.TaskIDs)})
	})
}

// resolveLinkedDocumentID 解析最近任务的关联文档：优先 related_document_id（doc_ai/translation/template 写入侧落），
// 回退 ref_type=document 的 ref_id（模板来源）。会话来源/数字员工占位无关联文档时返回空。
func resolveLinkedDocumentID(relatedDocumentID, refType, refID *string) string {
	if relatedDocumentID != nil && *relatedDocumentID != "" {
		return *relatedDocumentID
	}
	if refType != nil && *refType == "document" && refID != nil {
		return *refID
	}
	return ""
}
