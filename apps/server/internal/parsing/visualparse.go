package parsing

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
	"medoffice/server/internal/model"
)

// LowConfidenceThreshold：低于此置信度标记低置信度供下游人工复核。
const LowConfidenceThreshold = 0.6

type VisualParagraph struct {
	ParagraphIndex int       `json:"paragraphIndex"`
	Text           string    `json:"text"`
	Bbox           []float64 `json:"bbox,omitempty"`
	HeadingLevel   *int      `json:"headingLevel,omitempty"`
}

type VisualPage struct {
	Page          int               `json:"page"`
	Paragraphs    []VisualParagraph `json:"paragraphs"`
	Tables        []any             `json:"tables"`
	Images        []any             `json:"images"`
	Header        *string           `json:"header,omitempty"`
	Footer        *string           `json:"footer,omitempty"`
	Confidence    float64           `json:"confidence"`
	LowConfidence bool              `json:"lowConfidence"`
}

type chunkLocator struct {
	Page           int `json:"page"`
	ParagraphIndex int `json:"paragraphIndex"`
}

type VisualParseResult struct {
	FullText       string         `json:"fullText"`
	Pages          []VisualPage   `json:"pages"`
	ChunkLocators  []chunkLocator `json:"chunkLocators"`
	Confidence     float64        `json:"confidence"`
	BackendKind    string         `json:"backendKind"`
	DeploymentKind string         `json:"deploymentKind"`
}

type VisualParseFailedError struct{ Msg string }

func (e *VisualParseFailedError) Error() string { return e.Msg }

type VisualProviderMissingError struct{ Msg string }

func (e *VisualProviderMissingError) Error() string { return e.Msg }

type rawVisualResponse struct {
	Pages []struct {
		Page       *int `json:"page"`
		Paragraphs []struct {
			Text         string    `json:"text"`
			Bbox         []float64 `json:"bbox"`
			HeadingLevel *int      `json:"heading_level"`
		} `json:"paragraphs"`
		Tables     []any    `json:"tables"`
		Images     []any    `json:"images"`
		Header     *string  `json:"header"`
		Footer     *string  `json:"footer"`
		Confidence *float64 `json:"confidence"`
	} `json:"pages"`
	Confidence    *float64 `json:"confidence"`
	FailureReason *string  `json:"failure_reason"`
}

type VisualParseTarget struct {
	TenantID        string
	DocumentID      string
	DocumentVersion int
	ObjectKey       string
	Filename        string
	Mime            string
	JobID           *string
	ActorID         string
	ActorRole       string
}

func normalize(raw rawVisualResponse, conn model.ProviderConnection) VisualParseResult {
	pages := make([]VisualPage, 0, len(raw.Pages))
	for pi, p := range raw.Pages {
		pageNo := pi + 1
		if p.Page != nil {
			pageNo = *p.Page
		}
		paras := make([]VisualParagraph, 0, len(p.Paragraphs))
		for idx, para := range p.Paragraphs {
			paras = append(paras, VisualParagraph{ParagraphIndex: idx, Text: para.Text, Bbox: para.Bbox, HeadingLevel: para.HeadingLevel})
		}
		conf := 1.0
		if p.Confidence != nil {
			conf = *p.Confidence
		} else if raw.Confidence != nil {
			conf = *raw.Confidence
		}
		pages = append(pages, VisualPage{
			Page: pageNo, Paragraphs: paras, Tables: orEmpty(p.Tables), Images: orEmpty(p.Images),
			Header: p.Header, Footer: p.Footer, Confidence: conf, LowConfidence: conf < LowConfidenceThreshold,
		})
	}
	locators := []chunkLocator{}
	var textParts []string
	for _, pg := range pages {
		for _, para := range pg.Paragraphs {
			if strings.TrimSpace(para.Text) != "" {
				locators = append(locators, chunkLocator{Page: pg.Page, ParagraphIndex: para.ParagraphIndex})
				textParts = append(textParts, para.Text)
			}
		}
	}
	var confidence float64
	if raw.Confidence != nil {
		confidence = *raw.Confidence
	} else if len(pages) > 0 {
		var sum float64
		for _, p := range pages {
			sum += p.Confidence
		}
		confidence = sum / float64(len(pages))
	}
	backendKind := conn.BackendKind
	if backendKind == "" {
		backendKind = "unknown"
	}
	return VisualParseResult{
		FullText: strings.Join(textParts, "\n"), Pages: pages, ChunkLocators: locators,
		Confidence: confidence, BackendKind: backendKind, DeploymentKind: string(conn.DeploymentKind),
	}
}

func orEmpty(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

func callBackend(conn model.ProviderConnection, target VisualParseTarget) (*VisualParseResult, error) {
	headers := map[string]string{}
	if conn.Credential != "" {
		headers["authorization"] = "Bearer " + conn.Credential
	}
	rb, err := model.ProviderFetch(conn, "parse", map[string]any{
		"document_id": target.DocumentID, "object_key": target.ObjectKey,
		"filename": target.Filename, "mime": target.Mime,
	}, headers)
	if err != nil {
		return nil, err
	}
	var raw rawVisualResponse
	_ = json.Unmarshal(rb, &raw)
	if raw.FailureReason != nil && *raw.FailureReason != "" {
		return nil, &VisualParseFailedError{Msg: *raw.FailureReason}
	}
	result := normalize(raw, conn)
	if strings.TrimSpace(result.FullText) == "" {
		return nil, &VisualParseFailedError{Msg: "视觉解析未产出可用文本（疑似清晰度过低）"}
	}
	return &result, nil
}

// RunVisualParse 复刻 runVisualParse：链 + 公网脱敏门控 + 落 document_visual_parse_results + 审计 + fallback。
func RunVisualParse(db *gorm.DB, target VisualParseTarget) (*VisualParseResult, error) {
	chain, err := model.ResolveVisualChain(db, target.TenantID)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, &VisualProviderMissingError{Msg: "未配置可用的视觉解析 provider"}
	}

	for i := range chain {
		conn := chain[i]
		var nextName any
		if i+1 < len(chain) {
			nextName = chain[i+1].Name
		}

		if conn.DeploymentKind == model.DeployPublic {
			v := model.EvaluateRedaction(model.RedactionInput{TenantID: target.TenantID, Text: "<document>"})
			if !v.Available || !v.Passed {
				_ = audit.Write(db, audit.Entry{
					TenantID: target.TenantID, ActorID: actorP(target.ActorID), ActorRole: actorP(target.ActorRole),
					ActionType: "visual_redaction_block", TargetType: audit.P("document"), TargetID: audit.P(target.DocumentID),
					Result: "失败", FailureReason: audit.P(v.Reason),
					Metadata: map[string]any{"provider": conn.Name, "deploymentKind": "public", "switchTo": nextName},
				})
				continue
			}
		}

		result, cerr := callBackend(conn, target)
		if cerr == nil {
			_ = db.Exec(
				`INSERT INTO document_visual_parse_results
				   (tenant_id, document_id, document_version, job_id, full_text, pages, chunk_locators,
				    confidence, failure_reason, backend_kind, deployment_kind)
				 VALUES (?,?,?,?,?,?::jsonb,?::jsonb,?,NULL,?,?)`,
				target.TenantID, target.DocumentID, target.DocumentVersion, target.JobID, result.FullText,
				mustJSON(result.Pages), mustJSON(result.ChunkLocators), result.Confidence, result.BackendKind, result.DeploymentKind,
			).Error
			_ = audit.Write(db, audit.Entry{
				TenantID: target.TenantID, ActorID: actorP(target.ActorID), ActorRole: actorP(target.ActorRole),
				ActionType: "visual_parse", TargetType: audit.P("document"), TargetID: audit.P(target.DocumentID), Result: "成功",
				Metadata: map[string]any{"provider": conn.Name, "deploymentKind": conn.DeploymentKind, "backendKind": conn.BackendKind, "confidence": result.Confidence, "lowConfidence": result.Confidence < LowConfidenceThreshold},
			})
			return result, nil
		}

		var vpf *VisualParseFailedError
		if errors.As(cerr, &vpf) {
			_ = db.Exec(
				`INSERT INTO document_visual_parse_results
				   (tenant_id, document_id, document_version, job_id, full_text, pages, chunk_locators,
				    confidence, failure_reason, backend_kind, deployment_kind)
				 VALUES (?,?,?,?,NULL,'[]'::jsonb,'[]'::jsonb,0,?,?,?)`,
				target.TenantID, target.DocumentID, target.DocumentVersion, target.JobID, vpf.Msg, nullStr(conn.BackendKind), conn.DeploymentKind,
			).Error
			_ = audit.Write(db, audit.Entry{
				TenantID: target.TenantID, ActorID: actorP(target.ActorID), ActorRole: actorP(target.ActorRole),
				ActionType: "visual_parse", TargetType: audit.P("document"), TargetID: audit.P(target.DocumentID),
				Result: "失败", FailureReason: audit.P(vpf.Msg),
				Metadata: map[string]any{"provider": conn.Name, "deploymentKind": conn.DeploymentKind},
			})
			return nil, vpf
		}

		var pe *model.ProviderError
		if !errors.As(cerr, &pe) {
			pe = &model.ProviderError{Class: model.ErrUnknown, Msg: cerr.Error()}
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: target.TenantID, ActorID: actorP(target.ActorID), ActorRole: actorP(target.ActorRole),
			ActionType: "visual_parse", TargetType: audit.P("document"), TargetID: audit.P(target.DocumentID),
			Result: "失败", FailureReason: audit.P(pe.Msg),
			Metadata: map[string]any{"provider": conn.Name, "deploymentKind": conn.DeploymentKind, "errorClass": pe.Class, "switchTo": nextName},
		})
		if !model.IsFallbackable(pe.Class) {
			return nil, pe
		}
		_ = model.RecordHealth(db, model.HealthRecord{
			TenantID: target.TenantID, ProviderID: conn.ProviderID, ProviderKind: "visual",
			CheckKind: "passive", Status: "down", Error: pe.Msg,
		})
	}

	return nil, &VisualProviderMissingError{Msg: "所有视觉解析 provider 依次失败或被脱敏门禁拒绝"}
}

func actorP(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ——— 质量指标（task 6.8）：c03 仅自验计算逻辑 ———

type VisualEvalCase struct {
	PredictedPage         int
	ExpectedPage          int
	TableStructureCorrect bool
}

type VisualQualityMetrics struct {
	PageLocalizationRate float64
	TableStructureRate   float64
	MaxPageError         int
	Count                int
}

func ComputeVisualMetrics(cases []VisualEvalCase) VisualQualityMetrics {
	if len(cases) == 0 {
		return VisualQualityMetrics{}
	}
	pageOK, tableOK, maxErr := 0, 0, 0
	for _, c := range cases {
		err := c.PredictedPage - c.ExpectedPage
		if err < 0 {
			err = -err
		}
		if err <= 1 {
			pageOK++
		}
		if c.TableStructureCorrect {
			tableOK++
		}
		if err > maxErr {
			maxErr = err
		}
	}
	n := float64(len(cases))
	return VisualQualityMetrics{
		PageLocalizationRate: float64(pageOK) / n,
		TableStructureRate:   float64(tableOK) / n,
		MaxPageError:         maxErr,
		Count:                len(cases),
	}
}
