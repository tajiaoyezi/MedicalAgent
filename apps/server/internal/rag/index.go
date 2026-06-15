// Package rag 实现 c04 RAG 检索：消费 c03「索引就绪」事件构建内存 BM25+向量索引、
// 六维权限过滤、合并去重、rerank/RRF、source compression、注入带引用上下文。
package rag

import (
	"encoding/json"
	"log"
	"sync"

	"gorm.io/gorm"

	"medoffice/server/internal/parsing"
)

// IndexedChunk 内存索引中的 chunk（§16.3 元数据 + 向量）。
type IndexedChunk struct {
	ChunkID         string
	TenantID        string
	DocumentID      string
	DocumentVersion int
	SourceType      string // upload / kb（来自 document_chunks.source_type，空视为 upload）
	SourceTitle     string
	SourceURL       string
	PubmedID        string
	DOI             string
	Journal         string
	Year            int
	Section         string
	Page            *int
	ParagraphIndex  *int
	Text            string
	ChunkACL        map[string]any
	Embedding       []float64
}

// KBID 取 chunk_acl 内 c06 注入的 kb_id（POC 桥接：c06 落库时把 kb_id 写入 chunk_acl）。
func (c IndexedChunk) KBID() string {
	if v, ok := c.ChunkACL["kb_id"].(string); ok {
		return v
	}
	return ""
}

// EffectiveSourceType：source_type 为空视为 upload。
func (c IndexedChunk) EffectiveSourceType() string {
	if c.SourceType == "kb" {
		return "kb"
	}
	return "upload"
}

type docEntry struct {
	version int
	chunks  []IndexedChunk
}

// Index 进程内检索索引（按 document_id 持有最新已就绪版本）。
type Index struct {
	mu   sync.RWMutex
	docs map[string]*docEntry
}

var defaultIndex = &Index{docs: map[string]*docEntry{}}

// Default 返回进程内单例索引。
func Default() *Index { return defaultIndex }

// RegisterIndexConsumer 订阅 c03「索引就绪」事件（design D8：c04 是该 handoff 事件唯一检索侧消费方）。
// 在 server.New 装配时调用一次。
func RegisterIndexConsumer(db *gorm.DB) {
	parsing.OnIndexReady(func(ev parsing.IndexReadyEvent) {
		if err := defaultIndex.HandleIndexReady(db, ev); err != nil {
			log.Printf("[rag-index] 构建索引失败 doc=%s v=%d: %v", ev.DocumentID, ev.DocumentVersion, err)
		}
	})
}

// HandleIndexReady 消费索引就绪事件：加载该 (document_id, version) 的 chunk+embedding，
// 按 (document_id, version) 幂等替换；新版本就绪后旧版本索引失效（仅保留最新版本）。
func (ix *Index) HandleIndexReady(db *gorm.DB, ev parsing.IndexReadyEvent) error {
	ix.mu.RLock()
	cur, exists := ix.docs[ev.DocumentID]
	ix.mu.RUnlock()
	if exists && cur.version > ev.DocumentVersion {
		return nil // 已有更新版本，忽略旧事件
	}
	if exists && cur.version == ev.DocumentVersion {
		return nil // 同版本重复事件，幂等不重复构建
	}
	chunks, err := loadChunks(db, ev.DocumentID, ev.DocumentVersion)
	if err != nil {
		return err
	}
	ix.mu.Lock()
	ix.docs[ev.DocumentID] = &docEntry{version: ev.DocumentVersion, chunks: chunks}
	ix.mu.Unlock()
	return nil
}

// IndexDocument 主动加载某文档当前已就绪版本（供路由/冒烟在事件之外按需装载）。
func (ix *Index) IndexDocument(db *gorm.DB, documentID string, version int) error {
	return ix.HandleIndexReady(db, parsing.IndexReadyEvent{DocumentID: documentID, DocumentVersion: version})
}

// Version 返回某文档已索引版本（0=未索引）。
func (ix *Index) Version(documentID string) int {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	if e, ok := ix.docs[documentID]; ok {
		return e.version
	}
	return 0
}

// chunksForDocs 取若干文档的已索引 chunk。
func (ix *Index) chunksForDocs(docIDs []string) []IndexedChunk {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []IndexedChunk
	for _, id := range docIDs {
		if e, ok := ix.docs[id]; ok {
			out = append(out, e.chunks...)
		}
	}
	return out
}

// kbChunks 取所有 source_type=kb 的已索引 chunk（医疗知识库来源）。
func (ix *Index) kbChunks() []IndexedChunk {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	var out []IndexedChunk
	for _, e := range ix.docs {
		for _, c := range e.chunks {
			if c.EffectiveSourceType() == "kb" {
				out = append(out, c)
			}
		}
	}
	return out
}

// loadChunks 从库读 (document_id, version) 的未 superseded chunk + 向量。
func loadChunks(db *gorm.DB, documentID string, version int) ([]IndexedChunk, error) {
	var rows []struct {
		ID              string  `gorm:"column:id"`
		TenantID        string  `gorm:"column:tenant_id"`
		DocumentID      string  `gorm:"column:document_id"`
		DocumentVersion int     `gorm:"column:document_version"`
		SourceType      *string `gorm:"column:source_type"`
		SourceTitle     *string `gorm:"column:source_title"`
		SourceURL       *string `gorm:"column:source_url"`
		PubmedID        *string `gorm:"column:pubmed_id"`
		DOI             *string `gorm:"column:doi"`
		Journal         *string `gorm:"column:journal"`
		Year            *int    `gorm:"column:year"`
		Section         *string `gorm:"column:section"`
		Page            *int    `gorm:"column:page"`
		ParagraphIndex  *int    `gorm:"column:paragraph_index"`
		ChunkText       string  `gorm:"column:chunk_text"`
		ChunkACL        string  `gorm:"column:chunk_acl"`
		Vector          *string `gorm:"column:vector"`
	}
	if err := db.Raw(
		`SELECT c.id, c.tenant_id, c.document_id, c.document_version, c.source_type, c.source_title, c.source_url,
		        c.pubmed_id, c.doi, c.journal, c.year, c.section, c.page, c.paragraph_index, c.chunk_text,
		        c.chunk_acl::text AS chunk_acl, e.vector::text AS vector
		 FROM document_chunks c
		 LEFT JOIN embeddings e ON e.chunk_id = c.id
		 WHERE c.document_id = ? AND c.document_version = ? AND c.superseded = FALSE`,
		documentID, version,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]IndexedChunk, 0, len(rows))
	for _, r := range rows {
		acl := map[string]any{}
		if r.ChunkACL != "" {
			_ = json.Unmarshal([]byte(r.ChunkACL), &acl)
		}
		var emb []float64
		if r.Vector != nil && *r.Vector != "" {
			_ = json.Unmarshal([]byte(*r.Vector), &emb)
		}
		out = append(out, IndexedChunk{
			ChunkID: r.ID, TenantID: r.TenantID, DocumentID: r.DocumentID, DocumentVersion: r.DocumentVersion,
			SourceType: deref(r.SourceType), SourceTitle: deref(r.SourceTitle), SourceURL: deref(r.SourceURL),
			PubmedID: deref(r.PubmedID), DOI: deref(r.DOI), Journal: deref(r.Journal), Year: derefInt(r.Year),
			Section: deref(r.Section), Page: r.Page, ParagraphIndex: r.ParagraphIndex,
			Text: r.ChunkText, ChunkACL: acl, Embedding: emb,
		})
	}
	return out, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func derefInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}
