// Package citation 实现 c04 引用溯源：citations 承载、按 source_type 结构化渲染、点击定位（三态）、异常分支。
package citation

import (
	"fmt"

	"gorm.io/gorm"
)

// Input 写入一条引用所需字段（由 aimed 从 rag.Candidate 映射，避免 citation→rag 依赖）。
type Input struct {
	CiteIndex      int
	SourceType     string // pubmed / upload / kb
	PubmedID       string
	DOI            string
	SourceURL      string
	DocumentID     string
	Page           *int
	ParagraphIndex *int
	Section        string
	KBID           string
	ChunkID        string
	SourceTitle    string
	Journal        string
	Year           int
}

// Citation 引用行。
type Citation struct {
	CitationID     string `gorm:"column:citation_id" json:"citationId"`
	MessageID      string `gorm:"column:message_id" json:"-"`
	TenantID       string `gorm:"column:tenant_id" json:"-"`
	CiteIndex      int    `gorm:"column:cite_index" json:"citeIndex"`
	SourceType     string `gorm:"column:source_type" json:"sourceType"`
	PubmedID       string `gorm:"column:pubmed_id" json:"pubmedId,omitempty"`
	DOI            string `gorm:"column:doi" json:"doi,omitempty"`
	SourceURL      string `gorm:"column:source_url" json:"sourceUrl,omitempty"`
	DocumentID     string `gorm:"column:document_id" json:"documentId,omitempty"`
	Page           *int   `gorm:"column:page" json:"page,omitempty"`
	ParagraphIndex *int   `gorm:"column:paragraph_index" json:"paragraphIndex,omitempty"`
	Section        string `gorm:"column:section" json:"section,omitempty"`
	KBID           string `gorm:"column:kb_id" json:"kbId,omitempty"`
	ChunkID        string `gorm:"column:chunk_id" json:"chunkId,omitempty"`
	SourceTitle    string `gorm:"column:source_title" json:"sourceTitle,omitempty"`
	Journal        string `gorm:"column:journal" json:"journal,omitempty"`
	Year           int    `gorm:"column:year" json:"year,omitempty"`
}

// Save 批量写入某条 message 的引用集合（角标序号 cite_index 与答案 [n] 一一对应）。
func Save(db *gorm.DB, tenantID, messageID string, inputs []Input) error {
	for _, in := range inputs {
		if err := db.Exec(
			`INSERT INTO citations
			   (message_id, tenant_id, cite_index, source_type, pubmed_id, doi, source_url,
			    document_id, page, paragraph_index, section, kb_id, chunk_id, source_title, journal, year)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (message_id, cite_index) DO NOTHING`,
			messageID, tenantID, in.CiteIndex, in.SourceType,
			nullStr(in.PubmedID), nullStr(in.DOI), nullStr(in.SourceURL),
			nullStr(in.DocumentID), in.Page, in.ParagraphIndex, nullStr(in.Section),
			nullStr(in.KBID), nullStr(in.ChunkID), nullStr(in.SourceTitle), nullStr(in.Journal), nullInt(in.Year),
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// ListByMessage 取某 message 的引用（按角标序号，租户隔离）。
func ListByMessage(db *gorm.DB, tenantID, messageID string) ([]Citation, error) {
	var rows []Citation
	err := db.Raw(
		`SELECT * FROM citations WHERE message_id = ? AND tenant_id = ? ORDER BY cite_index ASC`,
		messageID, tenantID,
	).Scan(&rows).Error
	return rows, err
}

// Get 取单条引用（租户隔离）。
func Get(db *gorm.DB, tenantID, citationID string) (*Citation, error) {
	var c Citation
	err := db.Raw(`SELECT * FROM citations WHERE citation_id = ? AND tenant_id = ?`, citationID, tenantID).Scan(&c).Error
	if err != nil {
		return nil, err
	}
	if c.CitationID == "" {
		return nil, nil
	}
	return &c, nil
}

// Render 按 source_type 结构化渲染参考资料条目（§8.8）。
func (c Citation) Render() string {
	switch c.SourceType {
	case "pubmed":
		return fmt.Sprintf("[%d] PMID: %s, %s, %s, %d", c.CiteIndex, orDash(c.PubmedID), orDash(c.SourceTitle), orDash(c.Journal), c.Year)
	case "upload":
		page := "?"
		if c.Page != nil {
			page = fmt.Sprintf("%d", *c.Page)
		}
		sec := c.Section
		if sec == "" {
			sec = "正文"
		}
		return fmt.Sprintf("[%d] 上传文档：%s，第 %s 页，%s 段", c.CiteIndex, orDash(c.SourceTitle), page, sec)
	case "kb":
		return fmt.Sprintf("[%d] 知识库文档：%s（chunk %s）", c.CiteIndex, orDash(c.SourceTitle), orDash(c.ChunkID))
	default:
		return fmt.Sprintf("[%d] %s", c.CiteIndex, orDash(c.SourceTitle))
	}
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullInt(i int) any {
	if i == 0 {
		return nil
	}
	return i
}
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
