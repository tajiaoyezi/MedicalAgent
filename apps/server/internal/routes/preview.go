package routes

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/config"
	"medoffice/server/internal/docperm"
	"medoffice/server/internal/editor"
	"medoffice/server/internal/httpx"
	"medoffice/server/internal/storage"
)

func minimalPdfBuffer(title string) []byte {
	text := "OFD Preview: " + title
	pdf := fmt.Sprintf(`%%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj
3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R/Contents 4 0 R>>endobj
4 0 obj<</Length %d>>stream
BT /F1 12 Tf 72 720 Td (%s) Tj ET
endstream endobj
xref
0 5
trailer<</Size 5/Root 1 0 R>>
startxref
0
%%%%EOF`, len([]rune(text))+20, text)
	return []byte(pdf)
}

// RegisterPreview 复刻 routes/preview.ts：预览（pdf/image/ofd）+ parse-status（to_regclass 降级）。
func RegisterPreview(r *gin.Engine, db *gorm.DB, store *storage.Storage, cfg config.OnlyOffice) {
	r.GET("/api/preview/:documentId", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		documentID := c.Param("documentId")
		var row struct {
			DocumentID string `gorm:"column:document_id"`
			TenantID   string `gorm:"column:tenant_id"`
			OwnerID    string `gorm:"column:owner_id"`
			Name       string `gorm:"column:name"`
			Space      string `gorm:"column:space"`
			ObjectKey  string `gorm:"column:object_key"`
			FileHash   string `gorm:"column:file_hash"`
		}
		_ = db.Raw(
			`SELECT d.*, dv.version_id, dv.object_key, dv.file_hash
			 FROM documents d JOIN document_versions dv ON d.current_version_id = dv.version_id
			 WHERE d.document_id = ? AND d.is_deleted = FALSE`, documentID,
		).Scan(&row).Error
		if row.DocumentID == "" {
			httpx.Fail(c, 404, "文档不存在")
			return
		}
		if row.TenantID != user.TenantID {
			httpx.Fail(c, 403, "无权限")
			return
		}
		doc := docperm.DocumentRow{DocumentID: row.DocumentID, TenantID: row.TenantID, OwnerID: row.OwnerID, Space: row.Space}
		level, _ := docperm.Resolve(db, user, doc)
		if level == docperm.None {
			httpx.Fail(c, 403, "无权限")
			return
		}
		info := editor.ResolveEditorRoute(row.Name)
		ctx := c.Request.Context()

		switch info.Route {
		case editor.RoutePreviewOFD:
			sum := sha256.Sum256([]byte(row.TenantID + ":" + row.FileHash))
			cacheKey := hex.EncodeToString(sum[:])
			var previewKey string
			_ = db.Raw(`SELECT target_object_key FROM editor_conversion_cache WHERE source_hash = ?`, cacheKey).Scan(&previewKey).Error
			if previewKey == "" {
				pdfBuf := minimalPdfBuffer(row.Name)
				previewKey = storage.ObjectKeyForVersion(row.TenantID, documentID, "ofd-preview-"+cacheKey[:8])
				if err := store.Put(ctx, previewKey, pdfBuf, "application/pdf"); err != nil {
					httpx.Fail(c, 500, "服务器错误")
					return
				}
				_ = db.Exec(
					`INSERT INTO editor_conversion_cache (source_hash, target_object_key, target_mime)
					 VALUES (?, ?, 'application/pdf') ON CONFLICT (source_hash) DO NOTHING`,
					cacheKey, previewKey,
				).Error
			}
			url, err := store.PresignedURL(ctx, previewKey, 300*time.Second)
			if err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"previewType": "ofd", "label": "只读预览（OFD 转 PDF）", "readOnly": true,
				"url": url, "dsUrl": cfg.DSURL, "aiEntries": []string{"aimed", "translation"},
			})
		case editor.RoutePreviewPDF:
			url, err := store.PresignedURL(ctx, row.ObjectKey, 300*time.Second)
			if err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"previewType": "pdf", "readOnly": true, "url": url, "dsUrl": cfg.DSURL,
				"aiEntries": []string{"aimed", "translation"}, "currentPage": 1,
			})
		case editor.RoutePreviewImage:
			url, err := store.PresignedURL(ctx, row.ObjectKey, 300*time.Second)
			if err != nil {
				httpx.Fail(c, 500, "服务器错误")
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"previewType": "image", "url": url, "fileHash": row.FileHash, "visualParse": true,
			})
		default:
			httpx.Fail(c, 400, "不支持的预览类型")
		}
	})

	r.GET("/api/preview/:documentId/parse-status", func(c *gin.Context) {
		user, ok := auth.Require(c)
		if !ok {
			return
		}
		documentID := c.Param("documentId")
		var exists string
		_ = db.Raw(`SELECT document_id FROM documents WHERE document_id = ? AND tenant_id = ?`, documentID, user.TenantID).Scan(&exists).Error
		if exists == "" {
			httpx.Fail(c, 404, "文档不存在")
			return
		}

		// document_parse_jobs 由 c03 建表（migration 005）；建表前优雅降级避免 42P01
		var reg *string
		_ = db.Raw(`SELECT to_regclass('public.document_parse_jobs')::text AS t`).Scan(&reg).Error
		if reg == nil || *reg == "" {
			c.JSON(http.StatusOK, gin.H{
				"status": "pending", "jobs": []any{},
				"message": "等待 c03 解析服务建表并消费 upload_success 事件后创建作业",
			})
			return
		}

		var jobs []struct {
			JobID               string     `gorm:"column:job_id" json:"job_id"`
			Status              string     `gorm:"column:status" json:"status"`
			Substatus           *string    `gorm:"column:substatus" json:"substatus"`
			FailureReason       *string    `gorm:"column:failure_reason" json:"failure_reason"`
			DocumentVersion     int        `gorm:"column:document_version" json:"document_version"`
			IndexReadyAt        *time.Time `gorm:"column:index_ready_at" json:"index_ready_at"`
			UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
			VisualConfidence    *float64   `gorm:"column:visual_confidence" json:"visual_confidence"`
			VisualFailureReason *string    `gorm:"column:visual_failure_reason" json:"visual_failure_reason"`
		}
		_ = db.Raw(
			`SELECT j.job_id, j.status, j.substatus, j.failure_reason, j.document_version,
			        j.index_ready_at, j.updated_at,
			        r.confidence AS visual_confidence, r.failure_reason AS visual_failure_reason
			 FROM document_parse_jobs j
			 LEFT JOIN document_visual_parse_results r
			   ON r.document_id = j.document_id AND r.document_version = j.document_version
			 WHERE j.document_id = ? AND j.tenant_id = ?
			 ORDER BY j.created_at DESC LIMIT 1`, documentID, user.TenantID,
		).Scan(&jobs).Error
		if len(jobs) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"status": "pending", "jobs": []any{},
				"message": "等待 c03 解析服务消费 upload_success 事件后创建作业",
			})
			return
		}
		j := jobs[0]
		var visual any
		if j.VisualConfidence != nil || j.VisualFailureReason != nil {
			visual = gin.H{"confidence": j.VisualConfidence, "failureReason": j.VisualFailureReason}
		}
		c.JSON(http.StatusOK, gin.H{
			"status": j.Status, "substatus": j.Substatus, "documentVersion": j.DocumentVersion,
			"failureReason": j.FailureReason, "indexReadyAt": j.IndexReadyAt, "updatedAt": j.UpdatedAt,
			"visual": visual, "jobs": jobs,
		})
	})
}
