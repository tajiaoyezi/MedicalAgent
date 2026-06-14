package model

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"medoffice/server/internal/audit"
)

type InvokeContext struct {
	TenantID  string
	ActorID   string
	ActorRole string
}

type CapabilityUnavailableError struct{ Msg string }

func (e *CapabilityUnavailableError) Error() string { return e.Msg }

func asProviderError(err error) *ProviderError {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe
	}
	return newProviderError(ErrUnknown, err.Error())
}

func actorPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type chainCall[T any] struct {
	Capability   Capability
	OutboundText string
	Run          func(a Adapter, conn ProviderConnection) (T, error)
}

// runChain 复刻 router.ts runChain：fallback 链 + 公网脱敏门控 + 出网 + 全程审计。
func runChain[T any](db *gorm.DB, ctx InvokeContext, call chainCall[T]) (T, error) {
	var zero T
	chain, err := ResolveModelChain(db, ctx.TenantID, call.Capability)
	if err != nil {
		return zero, err
	}
	if len(chain) == 0 {
		_ = audit.Write(db, audit.Entry{
			TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
			ActionType: "model_route_missing", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
			Result: "失败", FailureReason: audit.P("该用途未配置可用模型"),
		})
		return zero, &CapabilityUnavailableError{fmt.Sprintf("该用途未配置可用模型：%s", string(call.Capability))}
	}

	for i := range chain {
		conn := chain[i]
		var next *ProviderConnection
		if i+1 < len(chain) {
			next = &chain[i+1]
		}
		nextName := func() any {
			if next != nil {
				return next.Name
			}
			return nil
		}

		// D6：公网 provider 必须先过 c09 脱敏门禁；未通过则跳过公网
		if conn.DeploymentKind == DeployPublic {
			v := redactionGateway.Evaluate(RedactionInput{TenantID: ctx.TenantID, Text: call.OutboundText})
			if !v.Available || !v.Passed {
				_ = audit.Write(db, audit.Entry{
					TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
					ActionType: "model_redaction_block", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
					Result: "失败", FailureReason: audit.P(v.Reason),
					Metadata: map[string]any{"provider": conn.Name, "providerId": conn.ProviderID, "deploymentKind": "public", "switchTo": nextName(), "confidence": v.Confidence},
				})
				continue
			}
		}

		adapter, aerr := getAdapter(conn.Protocol)
		if aerr != nil {
			return zero, aerr
		}
		var lastErr *ProviderError
		for attempt := 0; attempt <= conn.MaxRetries; attempt++ {
			result, rerr := call.Run(adapter, conn)
			if rerr == nil {
				_ = audit.Write(db, audit.Entry{
					TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
					ActionType: "model_invoke", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
					Result: "成功", Metadata: map[string]any{"provider": conn.Name, "providerId": conn.ProviderID, "deploymentKind": conn.DeploymentKind, "attempt": attempt},
				})
				return result, nil
			}
			pe := asProviderError(rerr)
			lastErr = pe
			if !IsFallbackable(pe.Class) {
				_ = audit.Write(db, audit.Entry{
					TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
					ActionType: "model_invoke", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
					Result: "失败", FailureReason: audit.P(pe.Msg),
					Metadata: map[string]any{"provider": conn.Name, "providerId": conn.ProviderID, "errorClass": pe.Class},
				})
				return zero, pe
			}
		}

		reason := "调用失败"
		var errClass ErrorClass = ErrUnknown
		if lastErr != nil {
			reason = lastErr.Msg
			errClass = lastErr.Class
		}
		_ = RecordHealth(db, HealthRecord{TenantID: ctx.TenantID, ProviderID: conn.ProviderID, ProviderKind: "model", CheckKind: "passive", Status: "down", Error: reason})
		var toProviderID any
		if next != nil {
			toProviderID = next.ProviderID
		}
		_ = audit.Write(db, audit.Entry{
			TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
			ActionType: "model_fallback", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
			Result: "失败", FailureReason: audit.P(reason),
			Metadata: map[string]any{
				"fromProvider": conn.Name, "fromProviderId": conn.ProviderID, "errorClass": errClass,
				"toProvider": nextName(), "toProviderId": toProviderID, "capability": call.Capability,
			},
		})
	}

	_ = audit.Write(db, audit.Entry{
		TenantID: ctx.TenantID, ActorID: actorPtr(ctx.ActorID), ActorRole: actorPtr(ctx.ActorRole),
		ActionType: "model_unavailable", TargetType: audit.P("capability"), TargetID: audit.P(string(call.Capability)),
		Result: "失败", FailureReason: audit.P("该用途所有 provider 依次失败或被脱敏门禁拒绝"),
	})
	return zero, &CapabilityUnavailableError{fmt.Sprintf("该用途所有 provider 均不可用：%s", string(call.Capability))}
}

func InvokeGeneration(db *gorm.DB, capability Capability, req GenerationRequest, ctx InvokeContext) (GenerationResponse, error) {
	var parts []string
	for _, m := range req.Messages {
		parts = append(parts, m.Content)
	}
	return runChain(db, ctx, chainCall[GenerationResponse]{
		Capability: capability, OutboundText: join(parts),
		Run: func(a Adapter, conn ProviderConnection) (GenerationResponse, error) { return a.Generate(req, conn) },
	})
}

func InvokeEmbed(db *gorm.DB, req EmbedRequest, ctx InvokeContext) (EmbedResponse, error) {
	return runChain(db, ctx, chainCall[EmbedResponse]{
		Capability: CapEmbed, OutboundText: join(req.Input),
		Run: func(a Adapter, conn ProviderConnection) (EmbedResponse, error) { return a.Embed(req, conn) },
	})
}

func InvokeRerank(db *gorm.DB, req RerankRequest, ctx InvokeContext) (RerankResponse, error) {
	parts := append([]string{req.Query}, req.Documents...)
	return runChain(db, ctx, chainCall[RerankResponse]{
		Capability: CapRerank, OutboundText: join(parts),
		Run: func(a Adapter, conn ProviderConnection) (RerankResponse, error) { return a.Rerank(req, conn) },
	})
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\n"
		}
		out += p
	}
	return out
}
