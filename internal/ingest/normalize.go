package ingest

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/otterscope/otterscope/internal/model"
)

// Normalize converts an OTLP trace batch into domain steps. Every span
// becomes a step — spans we don't recognize are kept as generic steps, never
// dropped. Dialect quirks live here and nowhere downstream (ADR-0002).
func Normalize(td ptrace.Traces) []model.Step {
	var steps []model.Step
	rss := td.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		service := stringAttr(rs.Resource().Attributes(), "service.name")
		sss := rs.ScopeSpans()
		for j := 0; j < sss.Len(); j++ {
			spans := sss.At(j).Spans()
			for k := 0; k < spans.Len(); k++ {
				steps = append(steps, normalizeSpan(spans.At(k), service))
			}
		}
	}
	return steps
}

func normalizeSpan(sp ptrace.Span, service string) model.Step {
	attrs := sp.Attributes()
	st := model.Step{
		ID:        sp.SpanID().String(),
		RunID:     sp.TraceID().String(),
		Name:      sp.Name(),
		Service:   service,
		AgentName: stringAttr(attrs, "gen_ai.agent.name"),
		Start:     sp.StartTimestamp().AsTime(),
		End:       sp.EndTimestamp().AsTime(),
	}
	if parent := sp.ParentSpanID(); !parent.IsEmpty() {
		st.ParentID = parent.String()
	}

	st.Status = model.StatusOK
	if sp.Status().Code() == ptrace.StatusCodeError {
		st.Status = model.StatusError
		st.Error = sp.Status().Message()
		if st.Error == "" {
			st.Error = "error"
		}
	}

	switch op := stringAttr(attrs, "gen_ai.operation.name"); op {
	case "chat", "text_completion", "generate_content", "embeddings":
		st.Kind = model.StepLLM
		st.LLM = &model.LLMCall{
			Provider:      stringAttr(attrs, "gen_ai.system"),
			RequestModel:  stringAttr(attrs, "gen_ai.request.model"),
			ResponseModel: stringAttr(attrs, "gen_ai.response.model"),
			InputTokens:   intAttr(attrs, "gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens"),
			OutputTokens:  intAttr(attrs, "gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens"),
		}
	case "execute_tool":
		st.Kind = model.StepTool
		st.Tool = &model.ToolCall{
			Name:   stringAttr(attrs, "gen_ai.tool.name"),
			CallID: stringAttr(attrs, "gen_ai.tool.call.id"),
		}
		if st.Tool.Name == "" {
			st.Tool.Name = sp.Name()
		}
	case "invoke_agent", "create_agent":
		st.Kind = model.StepAgent
	default:
		st.Kind = model.StepGeneric
	}

	// Zero timestamps confuse duration math downstream; clamp end >= start.
	if st.End.Before(st.Start) {
		st.End = st.Start
	}
	return st
}

func stringAttr(attrs pcommon.Map, key string) string {
	if v, ok := attrs.Get(key); ok {
		return v.AsString()
	}
	return ""
}

// intAttr returns the first present key's integer value. Legacy emitters use
// prompt/completion token names; both dialects appear in the wild.
func intAttr(attrs pcommon.Map, keys ...string) int64 {
	for _, key := range keys {
		if v, ok := attrs.Get(key); ok {
			if v.Type() == pcommon.ValueTypeInt {
				return v.Int()
			}
		}
	}
	return 0
}
