package ingest

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/model"
)

func normalizeNamed(t *testing.T, file string) map[string]model.Step {
	t.Helper()
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON(fixture(t, file)); err != nil {
		t.Fatalf("unmarshal %s: %v", file, err)
	}
	steps := Normalize(req.Traces())
	byName := make(map[string]model.Step, len(steps))
	for _, s := range steps {
		byName[s.Name] = s
	}
	return byName
}

func TestExperimentalDialectFixture(t *testing.T) {
	steps := normalizeNamed(t, "genai_experimental_chat.json")
	if len(steps) != 4 {
		t.Fatalf("got %d steps, want 4", len(steps))
	}

	wf := steps["invoke_workflow deep-research"]
	if wf.Kind != model.StepAgent {
		t.Errorf("invoke_workflow kind = %s, want agent", wf.Kind)
	}
	if wf.AgentName != "deep-research" {
		t.Errorf("workflow name fallback: agent name = %q, want deep-research", wf.AgentName)
	}

	llm := steps["chat claude-fable-5"]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("chat span not an llm step: %+v", llm)
	}
	if llm.LLM.Provider != "anthropic" {
		t.Errorf("provider from gen_ai.provider.name = %q", llm.LLM.Provider)
	}
	if llm.LLM.InputTokens != 20000 || llm.LLM.OutputTokens != 3200 {
		t.Errorf("tokens = %d/%d", llm.LLM.InputTokens, llm.LLM.OutputTokens)
	}
	if llm.LLM.CacheReadTokens != 18000 || llm.LLM.CacheCreationTokens != 1500 || llm.LLM.ReasoningTokens != 2100 {
		t.Errorf("token subsets = %d/%d/%d, want 18000/1500/2100",
			llm.LLM.CacheReadTokens, llm.LLM.CacheCreationTokens, llm.LLM.ReasoningTokens)
	}

	if plan := steps["plan researcher"]; plan.Kind != model.StepAgent || plan.AgentName != "researcher" {
		t.Errorf("plan span: kind=%s agent=%q", plan.Kind, plan.AgentName)
	}
	if ret := steps["retrieval docs-index"]; ret.Kind != model.StepGeneric {
		t.Errorf("retrieval kind = %s, want generic (visible, unclassified)", ret.Kind)
	}
}

// Both dialects in one batch must normalize coherently — that's the whole
// point of graceful per-attribute detection.
func TestMixedDialectBatch(t *testing.T) {
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON(fixture(t, "pydantic_ai_chat.json")); err != nil {
		t.Fatal(err)
	}
	old := Normalize(req.Traces())

	req2 := ptraceotlp.NewExportRequest()
	if err := req2.UnmarshalJSON(fixture(t, "genai_experimental_chat.json")); err != nil {
		t.Fatal(err)
	}
	req2.Traces().ResourceSpans().MoveAndAppendTo(req.Traces().ResourceSpans())

	mixed := Normalize(req.Traces())
	if len(mixed) != len(old)+4 {
		t.Fatalf("mixed batch normalized %d steps, want %d", len(mixed), len(old)+4)
	}
	providers := map[string]bool{}
	for _, s := range mixed {
		if s.LLM != nil {
			providers[s.LLM.Provider] = true
		}
	}
	if !providers["anthropic"] || len(providers) != 1 {
		t.Errorf("providers across dialects = %v", providers)
	}
}
