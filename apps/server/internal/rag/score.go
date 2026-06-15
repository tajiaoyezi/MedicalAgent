package rag

import (
	"math"
	"sort"
	"strings"
)

// —— BM25（全文检索打分；以候选集为语料即时计算）——

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

func tokenize(s string) []string {
	lower := strings.ToLower(s)
	repl := strings.NewReplacer(",", " ", "，", " ", "。", " ", "、", " ", "/", " ", "\n", " ", ".", " ", "(", " ", ")", " ", "[", " ", "]", " ")
	lower = repl.Replace(lower)
	var out []string
	for _, w := range strings.Fields(lower) {
		if len(w) >= 2 {
			out = append(out, w)
		}
	}
	// 中文 2-gram，提升无空格中文召回
	runes := []rune(lower)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x4e00 && a <= 0x9fff && b >= 0x4e00 && b <= 0x9fff {
			out = append(out, string([]rune{a, b}))
		}
	}
	return out
}

// bm25Scores 对候选文本集计算 BM25 分（query 的查询词在各文档的相关度）。
func bm25Scores(query string, docs []string) []float64 {
	qTerms := tokenize(query)
	n := len(docs)
	scores := make([]float64, n)
	if n == 0 || len(qTerms) == 0 {
		return scores
	}
	docTokens := make([][]string, n)
	var totalLen float64
	df := map[string]int{}
	for i, d := range docs {
		toks := tokenize(d)
		docTokens[i] = toks
		totalLen += float64(len(toks))
		seen := map[string]bool{}
		for _, t := range toks {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}
	avgdl := totalLen / float64(n)
	if avgdl == 0 {
		avgdl = 1
	}
	for i, toks := range docTokens {
		tf := map[string]int{}
		for _, t := range toks {
			tf[t]++
		}
		dl := float64(len(toks))
		var s float64
		for _, qt := range qTerms {
			f := float64(tf[qt])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (float64(n)-float64(df[qt])+0.5)/(float64(df[qt])+0.5))
			s += idf * (f * (bm25K1 + 1)) / (f + bm25K1*(1-bm25B+bm25B*dl/avgdl))
		}
		scores[i] = s
	}
	return scores
}

// —— 向量余弦（暴力，本期无 pgvector）——

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// —— RRF 融合（rerank 不可用时的离线兜底，design D3）——

// rrf 按 BM25 与向量两路排名做 Reciprocal Rank Fusion，返回每个候选的融合分。
func rrf(bm25, vec []float64) []float64 {
	const k = 60.0
	n := len(bm25)
	out := make([]float64, n)
	bmRank := rankOf(bm25)
	vecRank := rankOf(vec)
	for i := 0; i < n; i++ {
		out[i] = 1/(k+float64(bmRank[i])) + 1/(k+float64(vecRank[i]))
	}
	return out
}

// rankOf 返回每个下标的名次（分数降序，1 为最高）。
func rankOf(scores []float64) []int {
	idx := make([]int, len(scores))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return scores[idx[a]] > scores[idx[b]] })
	rank := make([]int, len(scores))
	for r, i := range idx {
		rank[i] = r + 1
	}
	return rank
}

// —— 抽取式压缩（句子级相关性截取，保留定位元数据）——

// compressExtractive 截取与 query 最相关的句子窗口，控制注入 token 且不改写原文。
func compressExtractive(text, query string, maxRunes int) string {
	if len([]rune(text)) <= maxRunes {
		return text
	}
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		r := []rune(text)
		return string(r[:maxRunes])
	}
	qTerms := tokenize(query)
	type sc struct {
		idx, score int
	}
	scored := make([]sc, len(sentences))
	for i, s := range sentences {
		low := strings.ToLower(s)
		c := 0
		for _, t := range qTerms {
			if strings.Contains(low, t) {
				c++
			}
		}
		scored[i] = sc{i, c}
	}
	sort.SliceStable(scored, func(a, b int) bool { return scored[a].score > scored[b].score })
	picked := map[int]bool{}
	total := 0
	for _, s := range scored {
		rl := len([]rune(sentences[s.idx]))
		if total+rl > maxRunes && total > 0 {
			break
		}
		picked[s.idx] = true
		total += rl
	}
	var b strings.Builder
	for i, s := range sentences {
		if picked[i] {
			b.WriteString(s)
		}
	}
	out := b.String()
	if out == "" {
		r := []rune(text)
		return string(r[:maxRunes])
	}
	return out
}

func splitSentences(text string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range text {
		cur.WriteRune(r)
		if r == '.' || r == '。' || r == '!' || r == '！' || r == '?' || r == '？' || r == '\n' {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
