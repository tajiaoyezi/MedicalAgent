package parsing

import (
	"context"
	"errors"
	"log"
	"time"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
)

const (
	consumerName   = "c03-parse"
	stuckParsingMs = 10 * 60 * 1000
)

var errAlreadyConsumed = errors.New("already consumed")

// dispatchPendingEvents 复刻 event-consumer.ts：取未消费事件，每条单事务建作业 + 标记消费 + 审计。
func dispatchPendingEvents(db *gorm.DB) int {
	var rows []struct {
		EventID         string `gorm:"column:event_id"`
		EventType       string `gorm:"column:event_type"`
		DocumentID      string `gorm:"column:document_id"`
		TenantID        string `gorm:"column:tenant_id"`
		DocumentVersion int    `gorm:"column:document_version"`
	}
	if err := db.Raw(
		`SELECT e.event_id, e.event_type, e.document_id, e.tenant_id, dv.document_version
		 FROM document_events e
		 JOIN document_versions dv ON dv.version_id = e.version_id
		 LEFT JOIN document_event_consumptions c ON c.event_id = e.event_id AND c.consumer = ?
		 WHERE c.event_id IS NULL
		 ORDER BY e.occurred_at ASC LIMIT 200`, consumerName,
	).Scan(&rows).Error; err != nil {
		log.Printf("[parse-dispatch] 查询事件失败: %v", err)
		return 0
	}

	created := 0
	for _, ev := range rows {
		err := db.Transaction(func(tx *gorm.DB) error {
			var jobID string
			if err := tx.Raw(
				`INSERT INTO document_parse_jobs (tenant_id, document_id, document_version, status, triggered_by)
				 VALUES (?, ?, ?, 'pending', ?) RETURNING job_id`,
				ev.TenantID, ev.DocumentID, ev.DocumentVersion, ev.EventType,
			).Scan(&jobID).Error; err != nil {
				return err
			}
			cons := tx.Exec(`INSERT INTO document_event_consumptions (event_id, consumer) VALUES (?, ?) ON CONFLICT DO NOTHING`, ev.EventID, consumerName)
			if cons.Error != nil {
				return cons.Error
			}
			if cons.RowsAffected == 0 {
				return errAlreadyConsumed // 并发消费 → 回滚刚建作业，避免重复
			}
			return audit.Write(tx, audit.Entry{
				TenantID: ev.TenantID, ActionType: "parse_job", TargetType: audit.P("document"), TargetID: audit.P(ev.DocumentID),
				Result:   "成功",
				Metadata: map[string]any{"jobId": jobID, "documentVersion": ev.DocumentVersion, "status": "pending", "triggeredBy": ev.EventType},
			})
		})
		switch {
		case err == nil:
			created++
		case errors.Is(err, errAlreadyConsumed):
			// 已被并发 dispatcher 消费，跳过
		default:
			log.Printf("[parse-dispatch] 事件消费失败: %v", err)
		}
	}
	return created
}

// reclaimStuckJobs 回收因崩溃滞留 parsing 的作业（超阈值重置 pending）。
func reclaimStuckJobs(db *gorm.DB) int {
	res := db.Exec(
		`UPDATE document_parse_jobs SET status = 'pending', substatus = NULL, updated_at = NOW()
		 WHERE status = 'parsing' AND started_at IS NOT NULL
		   AND started_at < NOW() - (?::bigint * INTERVAL '1 millisecond')`, stuckParsingMs,
	)
	return int(res.RowsAffected)
}

func (e *Engine) runPendingJobs(db *gorm.DB, limit int) int {
	var rows []ParseJob
	if err := db.Raw(
		`SELECT job_id, tenant_id, document_id, document_version
		 FROM document_parse_jobs WHERE status = 'pending' ORDER BY created_at ASC LIMIT ?`, limit,
	).Scan(&rows).Error; err != nil {
		log.Printf("[parse-run] 查询 pending 作业失败: %v", err)
		return 0
	}
	for _, job := range rows {
		e.RunParseJob(db, job)
	}
	return len(rows)
}

// ParseTick 一轮：回收滞留 + 消费事件 + 执行 pending 作业。
func (e *Engine) ParseTick(db *gorm.DB) (reclaimed, dispatched, ran int) {
	reclaimed = reclaimStuckJobs(db)
	dispatched = dispatchPendingEvents(db)
	ran = e.runPendingJobs(db, 20)
	return
}

// RetryJob 复刻 retryJob：失败作业置回 pending，落审计。
func RetryJob(db *gorm.DB, tenantID, jobID, actorID, actorRole string) (bool, error) {
	var docID string
	err := db.Raw(
		`UPDATE document_parse_jobs
		 SET status = 'pending', substatus = NULL, failure_reason = NULL,
		     triggered_by = 'manual_retry', actor_id = ?, started_at = NULL, completed_at = NULL, updated_at = NOW()
		 WHERE job_id = ? AND tenant_id = ? AND status = 'failed'
		 RETURNING document_id`, actorID, jobID, tenantID,
	).Scan(&docID).Error
	if err != nil {
		return false, err
	}
	if docID == "" {
		return false, nil
	}
	_ = audit.Write(db, audit.Entry{
		TenantID: tenantID, ActorID: audit.P(actorID), ActorRole: audit.P(actorRole),
		ActionType: "parse_job_retry", TargetType: audit.P("document"), TargetID: audit.P(docID),
		Result: "成功", Metadata: map[string]any{"jobId": jobID},
	})
	return true, nil
}

// StartWorker 启动后台轮询。intervalMs<=0 关闭；ctx 取消时退出。同步执行，无重入。
func StartWorker(ctx context.Context, db *gorm.DB, engine *Engine, intervalMs int) {
	if intervalMs <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[parse-worker] tick panic: %v", r)
						}
					}()
					engine.ParseTick(db)
				}()
			}
		}
	}()
}
