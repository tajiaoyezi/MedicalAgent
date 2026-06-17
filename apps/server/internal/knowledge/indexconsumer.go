package knowledge

import (
	"log"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/auth"
	"medoffice/server/internal/parsing"
	"medoffice/server/internal/rag"
)

// RegisterIndexConsumer 订阅 c03「索引就绪」事件作为知识库侧消费方（kb-import「消费 c03 索引就绪事件」Requirement、5.4a）。
// c03 是该事件唯一产生方，c04（检索索引）与 c06（知识库入库/重建收尾）共同消费。
// 须在 server.New 装配时、且在 rag.RegisterIndexConsumer 之后调用一次，使本消费方把 chunk 标记为 kb 后再让 rag 重载索引。
func RegisterIndexConsumer(db *gorm.DB) {
	parsing.OnIndexReady(func(ev parsing.IndexReadyEvent) {
		if err := HandleIndexReady(db, ev); err != nil {
			log.Printf("[kb-index] 知识库索引就绪收尾失败 doc=%s v=%d: %v", ev.DocumentID, ev.DocumentVersion, err)
		}
	})
}

// HandleIndexReady 知识库侧索引就绪收尾（5.4a，唯一触发源；事件到达前 MUST NOT 自行置 indexed）：
// 对该文档的正式（非 staging）kb_documents——把其 chunk 标记为 kb 来源 + 写入 chunk_acl.kb_id（c06 仅写值，
// 不改 c03 表结构），将 index_status 置 indexed、parse_status 置 parsed，并在同一事务内增量刷新所属知识库
// document_count（仅计 index_status=indexed 且非 staging）与 updated_at。随后重载 rag 索引使 kb chunk 可被检索。
func HandleIndexReady(db *gorm.DB, ev parsing.IndexReadyEvent) error {
	if ev.DocumentID == "" {
		return nil
	}
	// 该文档对应的正式 KB 文档（可能属多个 KB）。
	var kbDocs []struct {
		KBID string `gorm:"column:kb_id"`
	}
	if err := db.Raw(
		`SELECT DISTINCT kb_id FROM kb_documents WHERE document_id = ? AND is_staging = FALSE`,
		ev.DocumentID,
	).Scan(&kbDocs).Error; err != nil {
		return err
	}
	if len(kbDocs) == 0 {
		return nil // 非知识库来源文档（c04 上传等），不归本消费方处理
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, kd := range kbDocs {
			// chunk 标记为 kb 来源 + 注入 chunk_acl.kb_id（rag.IndexedChunk.KBID()/EffectiveSourceType() 据此识别）。
			if err := tx.Exec(
				`UPDATE document_chunks
				 SET source_type = 'kb',
				     chunk_acl = jsonb_set(COALESCE(chunk_acl, '{}'::jsonb), '{kb_id}', to_jsonb(?::text), true)
				 WHERE document_id = ? AND document_version = ? AND superseded = FALSE`,
				kd.KBID, ev.DocumentID, ev.DocumentVersion,
			).Error; err != nil {
				return err
			}
			// 置 indexed（唯一触发源为本事件）。
			if err := tx.Exec(
				`UPDATE kb_documents SET index_status = 'indexed', parse_status = 'parsed', updated_at = NOW()
				 WHERE document_id = ? AND kb_id = ? AND is_staging = FALSE`,
				ev.DocumentID, kd.KBID,
			).Error; err != nil {
				return err
			}
			// 增量刷新该库 document_count（仅计 indexed 且非 staging）+ updated_at。
			if err := tx.Exec(
				`UPDATE knowledge_bases SET
				   document_count = (SELECT COUNT(*) FROM kb_documents WHERE kb_id = ? AND index_status = 'indexed' AND is_staging = FALSE),
				   updated_at = NOW()
				 WHERE kb_id = ?`,
				kd.KBID, kd.KBID,
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 重载 rag 进程内索引：rag 在本消费方之前已按 source_type='document' 装载，需以更新后的 kb 标记重载，
	// 使 kbChunks() 能识别（c06 注册在 rag.RegisterIndexConsumer 之后，故此重载发生在 rag 初次装载之后）。
	if err := rag.Default().IndexDocument(db, ev.DocumentID, ev.DocumentVersion); err != nil {
		log.Printf("[kb-index] 重载 rag 索引失败 doc=%s v=%d: %v", ev.DocumentID, ev.DocumentVersion, err)
	}
	return nil
}

// Reindex 管理员触发重建索引（kb-import「管理员触发重建索引产生 manual_reindex 事件」、5.4）：
// 向 c01 所建 document_events 产生 event_type=manual_reindex（携 §10.6 契约字段，由 c03 消费触发重解析），
// 把 kb_documents 索引状态回退至待解析；重建动作审计写 audit_logs（不写 document_events）。收尾走同一索引就绪消费路径。
func Reindex(db *gorm.DB, u auth.AuthUser, kbDocID string) error {
	var rows []struct {
		KBID       string  `gorm:"column:kb_id"`
		DocumentID *string `gorm:"column:document_id"`
	}
	if err := db.Raw(`SELECT kb_id, document_id FROM kb_documents WHERE tenant_id = ? AND kb_document_id = ?`, u.TenantID, kbDocID).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	can, err := CanUploadToKB(db, u, rows[0].KBID)
	if err != nil {
		return err
	}
	if !can {
		return ErrForbidden
	}
	if rows[0].DocumentID == nil || *rows[0].DocumentID == "" {
		return ErrInvalidInput // 无落盘文档无从重建
	}
	docID := *rows[0].DocumentID
	var verRow []struct {
		VersionID string `gorm:"column:current_version_id"`
	}
	_ = db.Raw(`SELECT current_version_id FROM documents WHERE document_id = ? AND tenant_id = ?`, docID, u.TenantID).Scan(&verRow)
	if len(verRow) == 0 || verRow[0].VersionID == "" {
		return ErrInvalidInput
	}
	if err := db.Exec(
		`INSERT INTO document_events (event_type, document_id, version_id, tenant_id, payload)
		 VALUES ('manual_reindex', ?, ?, ?, ?::jsonb)`,
		docID, verRow[0].VersionID, u.TenantID, `{"trigger":"kb_manual_reindex","kbDocumentId":"`+kbDocID+`"}`,
	).Error; err != nil {
		return err
	}
	_ = db.Exec(`UPDATE kb_documents SET index_status = 'pending', parse_status = 'pending', updated_at = NOW() WHERE kb_document_id = ?`, kbDocID).Error
	_ = audit.Write(db, audit.Entry{
		TenantID: u.TenantID, ActorID: audit.P(u.UserID), ActorRole: roleCSV2(u),
		ActionType: "kb_manual_reindex", TargetType: audit.P("document"), TargetID: audit.P(docID),
		Result: "成功", Metadata: map[string]any{"kbDocumentId": kbDocID},
	})
	return nil
}
