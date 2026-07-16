package ingest

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/otterscope/otterscope/internal/model"
)

// Vercel AI SDK dialect (experimental_telemetry): span names double as
// operation identifiers (ai.generateText, ai.generateText.doGenerate,
// ai.toolCall) with camelCase ai.* attributes. Provider-call spans
// (*.doGenerate/*.doStream) additionally duplicate gen_ai.* attributes —
// a hybrid emitter, so this handler runs before gen_ai heuristics.
// Spec: ai-sdk.dev/docs/ai-sdk-core/telemetry

func isVercelAI(sp ptrace.Span) bool {
	return strings.HasPrefix(sp.Name(), "ai.")
}

func applyVercelAI(sp ptrace.Span, st *model.Step) {
	attrs := sp.Attributes()
	name := sp.Name()
	switch {
	case name == "ai.toolCall":
		st.Kind = model.StepTool
		st.Tool = &model.ToolCall{
			Name:      stringAttr(attrs, "ai.toolCall.name"),
			CallID:    stringAttr(attrs, "ai.toolCall.id"),
			Arguments: stringAttr(attrs, "ai.toolCall.args"),
			Result:    stringAttr(attrs, "ai.toolCall.result"),
		}
		if st.Tool.Name == "" {
			st.Tool.Name = name
		}
	case strings.HasSuffix(name, ".doGenerate"), strings.HasSuffix(name, ".doStream"), strings.HasSuffix(name, ".doEmbed"):
		st.Kind = model.StepLLM
		st.LLM = &model.LLMCall{
			Provider:      stringAttr(attrs, "gen_ai.provider.name", "gen_ai.system", "ai.model.provider"),
			RequestModel:  stringAttr(attrs, "gen_ai.request.model", "ai.model.id"),
			ResponseModel: stringAttr(attrs, "gen_ai.response.model", "ai.response.model"),
			InputTokens:   intAttr(attrs, "gen_ai.usage.input_tokens", "ai.usage.promptTokens"),
			OutputTokens:  intAttr(attrs, "gen_ai.usage.output_tokens", "ai.usage.completionTokens"),
			InputMessages: vercelPromptMessages(attrs, "ai.prompt.messages"),
		}
		if text := stringAttr(attrs, "ai.response.text"); text != "" {
			st.LLM.OutputMessages = []model.Message{{Role: "assistant", Content: text}}
		} else if calls := stringAttr(attrs, "ai.response.toolCalls"); calls != "" {
			st.LLM.OutputMessages = []model.Message{{Role: "assistant", Content: "[tool_calls " + calls + "]"}}
		}
	default:
		// Wrapper spans (ai.generateText, ai.streamText, ...) repeat the
		// child provider-call's ai.usage.* totals; classifying them as
		// generic (token-less) prevents double counting per run.
		st.Kind = model.StepGeneric
	}
}
