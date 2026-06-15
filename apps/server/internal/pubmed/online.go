package pubmed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// OnlineProvider 真实调用 PubMed E-utilities（esearch/efetch）。
// 注意：调用方（Service.useOnline）保证仅在脱敏门禁放行后才走在线；本期默认公网关闭，故通常只跑 SelfCheck。
type OnlineProvider struct {
	baseURL    string
	httpClient *http.Client
	maxRetries int
}

// NewOnlineProvider 构造。baseURL 默认官方 E-utilities。
func NewOnlineProvider(baseURL string, timeout time.Duration) *OnlineProvider {
	if baseURL == "" {
		baseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &OnlineProvider{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
		maxRetries: 2,
	}
}

func (p *OnlineProvider) Name() string { return "online" }

// SelfCheck 连通性自检（探测 einfo 可达性）。供 provider 级验收，不发送任何用户查询/PHI。
func (p *OnlineProvider) SelfCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/einfo.fcgi?retmode=json", nil)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("einfo status %d", resp.StatusCode)
	}
	return nil
}

// doWithBreaker 带超时熔断 + 指数退避的 GET。
func (p *OnlineProvider) doWithBreaker(reqURL string, out any) error {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(200*(1<<uint(attempt-1))) * time.Millisecond)
		}
		ctx, cancel := context.WithTimeout(context.Background(), p.httpClient.Timeout)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		resp, err := p.httpClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		dec := json.NewDecoder(resp.Body)
		derr := dec.Decode(out)
		resp.Body.Close()
		cancel()
		if derr != nil {
			lastErr = derr
			continue
		}
		return nil
	}
	return lastErr
}

// Search 调 esearch 取 PMID 列表，再 esummary 取摘要元数据，归一化为 RetrievedSource。
func (p *OnlineProvider) Search(query string, limit int) ([]RetrievedSource, error) {
	if limit <= 0 {
		limit = 10
	}
	esearch := fmt.Sprintf("%s/esearch.fcgi?db=pubmed&retmode=json&retmax=%d&term=%s",
		p.baseURL, limit, urlEscape(query))
	var sr struct {
		ESearchResult struct {
			IDList []string `json:"idlist"`
		} `json:"esearchresult"`
	}
	if err := p.doWithBreaker(esearch, &sr); err != nil {
		return nil, err
	}
	ids := sr.ESearchResult.IDList
	if len(ids) == 0 {
		return nil, nil
	}
	esummary := fmt.Sprintf("%s/esummary.fcgi?db=pubmed&retmode=json&id=%s", p.baseURL, strings.Join(ids, ","))
	var sm struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := p.doWithBreaker(esummary, &sm); err != nil {
		return nil, err
	}
	var out []RetrievedSource
	for _, id := range ids {
		raw, ok := sm.Result[id]
		if !ok {
			continue
		}
		var d struct {
			Title    string `json:"title"`
			FullJrnl string `json:"fulljournalname"`
			PubDate  string `json:"pubdate"`
			ELoc     string `json:"elocationid"`
		}
		_ = json.Unmarshal(raw, &d)
		out = append(out, RetrievedSource{
			SourceType: "pubmed",
			PubmedID:   id,
			Title:      d.Title,
			Journal:    d.FullJrnl,
			Year:       parseYear(d.PubDate),
			URL:        "https://pubmed.ncbi.nlm.nih.gov/" + id + "/",
			AuthStatus: AuthAuthorized,
		})
	}
	return out, nil
}

func (p *OnlineProvider) FetchDetail(id string) (*RetrievedSource, error) {
	res, err := p.Search(id+"[uid]", 1)
	if err != nil || len(res) == 0 {
		return nil, err
	}
	return &res[0], nil
}

func urlEscape(s string) string {
	r := strings.NewReplacer(" ", "+", "&", "%26", "?", "%3F", "#", "%23")
	return r.Replace(s)
}

func parseYear(pubdate string) int {
	f := strings.Fields(pubdate)
	if len(f) == 0 {
		return 0
	}
	y := 0
	for _, ch := range f[0] {
		if ch < '0' || ch > '9' {
			break
		}
		y = y*10 + int(ch-'0')
	}
	if y < 1800 || y > 3000 {
		return 0
	}
	return y
}
