package rag

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"medoffice/server/internal/auth"
	"medoffice/server/internal/model"
	"medoffice/server/internal/pubmed"
)

// Candidate 统一检索候选（PubMed / 上传文件 / 知识库 chunk 同构）。
type Candidate struct {
	SourceType      string `json:"sourceType"` // pubmed / upload / kb
	DocumentID      string `json:"documentId,omitempty"`
	DocumentVersion int    `json:"-"`
	ChunkID         string `json:"chunkId,omitempty"`
	Page            *int   `json:"page,omitempty"`
	ParagraphIndex  *int   `json:"paragraphIndex,omitempty"`
	Section         string `json:"section,omitempty"`
	KBID            string `json:"kbId,omitempty"`
	PubmedID        string `json:"pubmedId,omitempty"`
	DOI             string `json:"doi,omitempty"`
	SourceURL       string `json:"sourceUrl,omitempty"`
	SourceTitle     string `json:"sourceTitle,omitempty"`
	Journal         string `json:"journal,omitempty"`
	Year            int    `json:"year,omitempty"`
	Text            string `json:"text"`
	CiteRef         int    `json:"citeRef,omitempty"` // 注入时分配的角标序号 [n]

	embedding []float64
	tenantID  string
	chunkACL  map[string]any
	bm25      float64
	vec       float64
	score     float64
}

// RetrieveRequest 检索输入。
type RetrieveRequest struct {
	User            auth.AuthUser
	ModeLabel       string
	AllowPubmed     bool
	AllowUpload     bool
	AllowKB         bool
	AllowCurrentDoc bool
	Query           string
	UploadDocIDs    []string
	CurrentDocID    string
	ConversationID  string
	MessageID       string
	TopN            int
	// KBIDs：知识库数据源 kb_id 选择（c06 §11.6/§11.7「选定的一个或多个 kb_id」前置数据源选择）。
	// 空 = 不按 kb 限定（保留既有 AIMed AllowKB 行为不变）；非空 = 仅纳入这些 kb 的 chunk 进检索候选。
	KBIDs []string
}

// RetrieveResult 检索输出。
type RetrieveResult struct {
	Candidates   []Candidate
	PubmedRoute  string // online / offline / none
	TotalFound   int    // 找到 N 篇相关资料
	KeyCount     int    // M 篇重点参考（最终注入数）
	RerankMethod string // model / rrf
	RunID        string
}

// Engine RAG 检索引擎。
type Engine struct {
	Pubmed *pubmed.Service
}

func NewEngine(p *pubmed.Service) *Engine { return &Engine{Pubmed: p} }

// Retrieve 执行完整 RAG 管线，记一条 agent_run + 每节点 agent_step。
func (e *Engine) Retrieve(db *gorm.DB, req RetrieveRequest) (RetrieveResult, error) {
	ictx := model.InvokeContext{TenantID: req.User.TenantID, ActorID: req.User.UserID, ActorRole: strings.Join(req.User.RoleSlugs, ",")}
	runID := startRun(db, req.User.TenantID, req.User.UserID, req.ConversationID, req.MessageID)
	res := RetrieveResult{RunID: runID}
	if req.TopN <= 0 {
		req.TopN = 6
	}

	// 1. Query Rewrite（私有化优先；不可用则用原查询）
	rewritten := req.Query
	if rw, err := model.InvokeGeneration(db, model.CapChat, model.GenerationRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "改写以下医学检索问题为更利于检索的查询，只输出查询本身：" + req.Query}},
		Hint:     "query_rewrite",
	}, ictx); err == nil && strings.TrimSpace(rw.Content) != "" {
		rewritten = strings.TrimSpace(rw.Content)
	}
	recordStep(db, req.User.TenantID, runID, "query_rewrite", req.Query, rewritten, nil)

	// 2. 数据源选择（按 mode policy 的 allow_* 限定）
	var sources []string
	if req.AllowPubmed {
		sources = append(sources, "pubmed")
	}
	if req.AllowUpload {
		sources = append(sources, "upload")
	}
	if req.AllowKB {
		sources = append(sources, "kb")
	}
	if req.AllowCurrentDoc {
		sources = append(sources, "current_doc")
	}
	recordStep(db, req.User.TenantID, runID, "source_selection", req.ModeLabel, strings.Join(sources, ","), map[string]any{"sources": sources})

	// 3. 收集候选
	var cands []Candidate
	if req.AllowUpload || req.AllowCurrentDoc {
		docIDs := append([]string{}, req.UploadDocIDs...)
		if req.AllowCurrentDoc && req.CurrentDocID != "" {
			docIDs = append(docIDs, req.CurrentDocID)
		}
		for _, ic := range Default().chunksForDocs(uniq(docIDs)) {
			if ic.EffectiveSourceType() == "kb" {
				continue // upload 数据源不含 kb chunk
			}
			cands = append(cands, chunkToCandidate(ic))
		}
	}
	if req.AllowKB {
		kbScope := map[string]bool{}
		for _, id := range req.KBIDs {
			if id != "" {
				kbScope[id] = true
			}
		}
		for _, ic := range Default().kbChunks() {
			if len(kbScope) > 0 && !kbScope[ic.KBID()] {
				continue // 数据源选择：仅纳入选定 kb_id 的 chunk（空 scope 表示不限定）
			}
			c := chunkToCandidate(ic)
			c.SourceType = "kb"
			c.KBID = ic.KBID()
			cands = append(cands, c)
		}
	}
	if req.AllowPubmed && e.Pubmed != nil {
		ps, route, _ := e.Pubmed.Search(db, ictx, rewritten, req.TopN*2)
		res.PubmedRoute = route
		for _, s := range ps {
			cands = append(cands, pubmedToCandidate(s))
		}
	} else {
		res.PubmedRoute = "none"
	}
	rawCount := len(cands)

	// 4. 权限过滤（六维，先于 rerank 与注入）
	cands, dropped := filterPermissions(db, req.User, cands)
	recordStep(db, req.User.TenantID, runID, "permission_filter", fmt.Sprintf("候选 %d", rawCount), fmt.Sprintf("保留 %d 丢弃 %d", len(cands), dropped), map[string]any{"dropped": dropped})
	// 「找到 N 篇」对用户展示的是**授权后**可访问命中数，不含被 tenant/document_acl/chunk_acl 丢弃的越权候选。
	accessibleCount := len(cands)

	if len(cands) == 0 {
		endRun(db, runID, "succeeded")
		res.TotalFound = 0
		return res, nil
	}

	// 5. BM25 ∥ 向量检索
	docs := make([]string, len(cands))
	for i, c := range cands {
		docs[i] = c.SourceTitle + " " + c.Text
	}
	bm := bm25Scores(rewritten, docs)
	vecScores := make([]float64, len(cands))
	if emb, err := model.InvokeEmbed(db, model.EmbedRequest{Input: []string{rewritten}}, ictx); err == nil && len(emb.Vectors) == 1 {
		qv := emb.Vectors[0]
		dimMismatch := 0
		for i := range cands {
			// 有向量但维度与 query 不一致（换嵌入模型/provider fallback）→ cosine 恒为 0，
			// 该候选的向量分静默失效。显式计数并落降级 step，避免「悄悄退化为纯 BM25」无迹可查。
			if len(cands[i].embedding) > 0 && len(cands[i].embedding) != len(qv) {
				dimMismatch++
				continue
			}
			vecScores[i] = cosine(qv, cands[i].embedding)
		}
		if dimMismatch > 0 {
			recordStep(db, req.User.TenantID, runID, "vector_dim_mismatch",
				fmt.Sprintf("query 维度 %d", len(qv)),
				fmt.Sprintf("%d/%d 候选向量维度不匹配，向量分降级（退化为 BM25）", dimMismatch, len(cands)),
				map[string]any{"mismatched": dimMismatch, "total": len(cands), "queryDim": len(qv)})
		}
	}
	for i := range cands {
		cands[i].bm25 = bm[i]
		cands[i].vec = vecScores[i]
	}
	recordStep(db, req.User.TenantID, runID, "bm25_vector", rewritten, fmt.Sprintf("BM25+向量打分 %d 候选", len(cands)), nil)

	// 6. 合并去重（同源不重复占引用位）
	cands = dedup(cands)
	recordStep(db, req.User.TenantID, runID, "merge_dedup", "", fmt.Sprintf("去重后 %d", len(cands)), nil)

	// 7. rerank（模型优先；不可用 RRF 兜底）
	method := "rrf"
	if scores, err := e.modelRerank(db, ictx, rewritten, cands); err == nil {
		for i := range cands {
			cands[i].score = scores[i]
		}
		method = "model"
	} else {
		fused := rrf(collect(cands, func(c Candidate) float64 { return c.bm25 }), collect(cands, func(c Candidate) float64 { return c.vec }))
		for i := range cands {
			cands[i].score = fused[i]
		}
	}
	res.RerankMethod = method
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	recordStep(db, req.User.TenantID, runID, "rerank", method, fmt.Sprintf("rerank=%s", method), map[string]any{"method": method})

	// 8. 截断 topN + source compression（抽取式，保留定位元数据）
	if len(cands) > req.TopN {
		cands = cands[:req.TopN]
	}
	for i := range cands {
		cands[i].Text = compressExtractive(cands[i].Text, rewritten, 600)
		cands[i].CiteRef = i + 1 // 注入时分配稳定角标序号
	}
	recordStep(db, req.User.TenantID, runID, "source_compression", "", fmt.Sprintf("注入 %d 片段", len(cands)), nil)

	res.Candidates = cands
	res.TotalFound = accessibleCount
	res.KeyCount = len(cands)
	endRun(db, runID, "succeeded")
	return res, nil
}

// modelRerank 调 c03 reranker 能力；不可用返回 error → 调用方走 RRF。
func (e *Engine) modelRerank(db *gorm.DB, ictx model.InvokeContext, query string, cands []Candidate) ([]float64, error) {
	docs := make([]string, len(cands))
	for i, c := range cands {
		docs[i] = c.SourceTitle + " " + c.Text
	}
	rr, err := model.InvokeRerank(db, model.RerankRequest{Query: query, Documents: docs}, ictx)
	if err != nil {
		return nil, err
	}
	if len(rr.Scores) != len(cands) {
		return nil, fmt.Errorf("rerank 分数数量不匹配")
	}
	return rr.Scores, nil
}

func chunkToCandidate(ic IndexedChunk) Candidate {
	c := Candidate{
		SourceType: ic.EffectiveSourceType(), DocumentID: ic.DocumentID, DocumentVersion: ic.DocumentVersion,
		ChunkID: ic.ChunkID, Page: ic.Page, ParagraphIndex: ic.ParagraphIndex, Section: ic.Section,
		PubmedID: ic.PubmedID, DOI: ic.DOI, SourceURL: ic.SourceURL, SourceTitle: ic.SourceTitle,
		Journal: ic.Journal, Year: ic.Year, Text: ic.Text,
		embedding: ic.Embedding, tenantID: ic.TenantID, chunkACL: ic.ChunkACL,
	}
	return c
}

func pubmedToCandidate(s pubmed.RetrievedSource) Candidate {
	return Candidate{
		SourceType: "pubmed", PubmedID: s.PubmedID, DOI: s.DOI, SourceURL: s.URL,
		SourceTitle: s.Title, Journal: s.Journal, Year: s.Year,
		Text: s.Title + "。" + s.Abstract,
	}
}

// dedup 去重键 = pubmed_id 或 (document_id,page,paragraph_index) 或 chunk_id；保留分高者。
func dedup(cands []Candidate) []Candidate {
	seen := map[string]int{} // key → index in out
	var out []Candidate
	for _, c := range cands {
		var key string
		switch {
		case c.PubmedID != "":
			key = "pmid:" + c.PubmedID
		case c.ChunkID != "":
			key = "chunk:" + c.ChunkID
		default:
			key = fmt.Sprintf("doc:%s:%v:%v", c.DocumentID, c.Page, c.ParagraphIndex)
		}
		if idx, ok := seen[key]; ok {
			if c.bm25+c.vec > out[idx].bm25+out[idx].vec {
				out[idx] = c
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, c)
	}
	return out
}

func collect(cands []Candidate, f func(Candidate) float64) []float64 {
	out := make([]float64, len(cands))
	for i, c := range cands {
		out[i] = f(c)
	}
	return out
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
