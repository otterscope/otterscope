package ingest

import (
	"testing"

	"github.com/otterscope/otterscope/internal/model"
)

func TestVercelAISDKFixture(t *testing.T) {
	steps := normalizeNamed(t, "vercel_ai_sdk.json")
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}

	wrapper := steps["ai.generateText"]
	if wrapper.Kind != model.StepGeneric || wrapper.LLM != nil {
		t.Errorf("wrapper span must be generic and token-less (double-count guard): %+v", wrapper)
	}

	llm := steps["ai.generateText.doGenerate"]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("doGenerate not an llm step: %+v", llm)
	}
	if llm.LLM.Provider != "anthropic" || llm.LLM.RequestModel != "claude-sonnet-5" {
		t.Errorf("llm identity: %+v", llm.LLM)
	}
	if llm.LLM.InputTokens != 950 || llm.LLM.OutputTokens != 120 {
		t.Errorf("tokens = %d/%d", llm.LLM.InputTokens, llm.LLM.OutputTokens)
	}

	tool := steps["ai.toolCall"]
	if tool.Kind != model.StepTool || tool.Tool == nil {
		t.Fatalf("ai.toolCall not a tool step: %+v", tool)
	}
	if tool.Tool.Name != "searchOrders" || tool.Tool.CallID != "call_v1" {
		t.Errorf("tool identity: %+v", tool.Tool)
	}

	// Per-run totals must count the provider call once, not wrapper+child.
	var totalIn int64
	for _, s := range steps {
		if s.LLM != nil {
			totalIn += s.LLM.InputTokens
		}
	}
	if totalIn != 950 {
		t.Errorf("run input tokens = %d, want 950 (no double count)", totalIn)
	}
}

// A doGenerate span with only ai.* attrs (no gen_ai duplicates) must still
// normalize via the camelCase fallbacks.
func TestVercelFallbackWithoutGenAI(t *testing.T) {
	td := span("", map[string]any{
		"ai.model.id":               "gpt-5.2",
		"ai.model.provider":         "openai",
		"ai.usage.promptTokens":     40,
		"ai.usage.completionTokens": 7,
	})
	td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).SetName("ai.streamText.doStream")

	steps := Normalize(td)
	if len(steps) != 1 || steps[0].LLM == nil {
		t.Fatalf("not normalized: %+v", steps)
	}
	llm := steps[0].LLM
	if llm.Provider != "openai" || llm.RequestModel != "gpt-5.2" || llm.InputTokens != 40 || llm.OutputTokens != 7 {
		t.Errorf("ai.* fallbacks: %+v", llm)
	}
}
