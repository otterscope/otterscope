package ingest

import (
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/otterscope/otterscope/internal/model"
)

// OpenInference (Arize) dialect — the de-facto convention for the OpenAI
// Agents SDK and CrewAI instrumentors. `openinference.span.kind` is required
// on every OpenInference span, which makes detection unambiguous.
// Spec: github.com/Arize-ai/openinference/blob/main/spec/semantic_conventions.md

// isOpenInference reports whether the span carries the dialect's required
// kind attribute.
func isOpenInference(attrs pcommon.Map) bool {
	_, ok := attrs.Get("openinference.span.kind")
	return ok
}

// applyOpenInference classifies st from OpenInference attributes.
func applyOpenInference(sp ptrace.Span, st *model.Step) {
	attrs := sp.Attributes()
	switch stringAttr(attrs, "openinference.span.kind") {
	case "LLM":
		st.Kind = model.StepLLM
		st.LLM = &model.LLMCall{
			Provider:            stringAttr(attrs, "llm.provider", "llm.system"),
			RequestModel:        stringAttr(attrs, "llm.model_name"),
			InputTokens:         intAttr(attrs, "llm.token_count.prompt"),
			OutputTokens:        intAttr(attrs, "llm.token_count.completion"),
			CacheReadTokens:     intAttr(attrs, "llm.token_count.prompt_details.cache_read"),
			CacheCreationTokens: intAttr(attrs, "llm.token_count.prompt_details.cache_write"),
			ReasoningTokens:     intAttr(attrs, "llm.token_count.completion_details.reasoning"),
			InputMessages:       openInferenceMessages(attrs, "llm.input_messages."),
			OutputMessages:      openInferenceMessages(attrs, "llm.output_messages."),
		}
	case "EMBEDDING":
		st.Kind = model.StepLLM
		st.LLM = &model.LLMCall{
			Provider:     stringAttr(attrs, "llm.provider", "llm.system"),
			RequestModel: stringAttr(attrs, "embedding.model_name", "llm.model_name"),
			InputTokens:  intAttr(attrs, "llm.token_count.prompt"),
		}
	case "TOOL":
		st.Kind = model.StepTool
		st.Tool = &model.ToolCall{
			Name:      stringAttr(attrs, "tool.name"),
			CallID:    stringAttr(attrs, "tool_call.id"),
			Arguments: stringAttr(attrs, "tool_call.function.arguments", "input.value"),
			Result:    stringAttr(attrs, "output.value"),
		}
		if st.Tool.Name == "" {
			st.Tool.Name = sp.Name()
		}
	case "AGENT":
		st.Kind = model.StepAgent
		if st.AgentName == "" {
			st.AgentName = stringAttr(attrs, "agent.name")
		}
		if st.AgentName == "" {
			st.AgentName = sp.Name()
		}
	default:
		// CHAIN, RETRIEVER, RERANKER, GUARDRAIL, EVALUATOR, PROMPT, and
		// future kinds stay visible as generic steps.
		st.Kind = model.StepGeneric
	}
}
