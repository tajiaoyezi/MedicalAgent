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

// RegisterRecentTasks 复刻 routes/recent-tasks.ts。
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
			TaskID    string    `gorm:"column:task_id"`
			Source    string    `gorm:"column:source"`
			Title     string    `gorm:"column:title"`
			RefType   *string   `gorm:"column:ref_type"`
			RefID     *string   `gorm:"column:ref_id"`
			UpdatedAt time.Time `gorm:"column:updated_at"`
		}
		_ = db.Raw(
			`SELECT task_id, source, title, ref_type, ref_id, updated_at
			 FROM recent_tasks WHERE tenant_id = ? AND user_id = ? AND deleted_at IS NULL
			 ORDER BY updated_at DESC`, user.TenantID, user.UserID,
		).Scan(&rows).Error

		tasks := []gin.H{}
		for _, row := range rows {
			if len(filter) > 0 && !contains(filter, row.Source) {
				continue
			}
			titlePreview := row.Title
			if r := []rune(row.Title); len(r) > 10 {
				titlePreview = string(r[:10])
			}
			tasks = append(tasks, gin.H{
				"taskId": row.TaskID, "source": row.Source, "title": row.Title,
				"titlePreview": titlePreview, "refType": row.RefType, "refId": row.RefID,
				"updatedAt": row.UpdatedAt, "timeGroup": groupByTime(row.UpdatedAt),
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
			`INSERT INTO recent_tasks (task_id, tenant_id, user_id, source, title, ref_type, ref_id, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, NOW())
			 ON CONFLICT (tenant_id, user_id, ref_type, ref_id)
			 DO UPDATE SET title = EXCLUDED.title, updated_at = NOW(), deleted_at = NULL`,
			taskID, user.TenantID, user.UserID, body.Source, body.Title, nullIfEmpty(body.RefType), nullIfEmpty(body.RefID),
		).Error
		c.JSON(http.StatusOK, gin.H{"taskId": taskID})
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
		_ = db.Exec(
			`UPDATE recent_tasks SET title = ?, updated_at = NOW()
			 WHERE task_id = ? AND tenant_id = ? AND user_id = ?`,
			strings.TrimSpace(body.Title), id, user.TenantID, user.UserID,
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
			TaskID  string  `gorm:"column:task_id"`
			RefType *string `gorm:"column:ref_type"`
			RefID   *string `gorm:"column:ref_id"`
		}
		_ = db.Raw(`SELECT task_id, ref_type, ref_id FROM recent_tasks WHERE task_id = ? AND tenant_id = ? AND user_id = ?`, id, user.TenantID, user.UserID).Scan(&task).Error
		if task.TaskID == "" {
			httpx.Fail(c, 404, "任务不存在")
			return
		}

		if body.DeleteLinkedDocument && task.RefType != nil && *task.RefType == "document" && task.RefID != nil && *task.RefID != "" {
			doc, found, _ := getDoc(db, *task.RefID, user.TenantID)
			if found {
				level, _ := docperm.Resolve(db, user, doc)
				if !docperm.CanEdit(level) && level != docperm.Manage {
					httpx.Fail(c, 403, "无删除关联文档权限")
					return
				}
				_ = db.Exec(`UPDATE documents SET is_deleted = TRUE WHERE document_id = ?`, *task.RefID).Error
				_ = audit.Write(db, audit.Entry{
					TenantID: user.TenantID, ActorID: audit.P(user.UserID), ActorRole: roleCSV(user),
					ActionType: "file_delete", TargetType: audit.P("document"), TargetID: audit.P(*task.RefID),
					Result: "成功", Metadata: map[string]any{"fromRecentTask": id},
				})
			}
		}

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
			_ = db.Exec(`UPDATE recent_tasks SET deleted_at = NOW() WHERE task_id = ? AND user_id = ?`, id, user.UserID).Error
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "deleted": len(body.TaskIDs)})
	})
}
