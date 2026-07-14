package ingest

import (
	"testing"

	"github.com/otterscope/otterscope/internal/model"
)

func TestOpenInferenceFixture(t *testing.T) {
	steps := normalizeNamed(t, "openai_agents_openinference.json")
	if len(steps) != 5 {
		t.Fatalf("got %d steps, want 5", len(steps))
	}

	agent := steps["Triage workflow"]
	if agent.Kind != model.StepAgent || agent.AgentName != "Triage Agent" {
		t.Errorf("AGENT span: kind=%s agent=%q", agent.Kind, agent.AgentName)
	}
	if agent.Service != "triage-agents" {
		t.Errorf("openinference.project.name not used as service: %q", agent.Service)
	}

	llm := steps["Response"]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("LLM span not classified: %+v", llm)
	}
	if llm.LLM.Provider != "openai" || llm.LLM.RequestModel != "gpt-5.2" {
		t.Errorf("llm identity: %+v", llm.LLM)
	}
	if llm.LLM.InputTokens != 1450 || llm.LLM.OutputTokens != 210 || llm.LLM.CacheReadTokens != 1200 {
		t.Errorf("token counts: %+v", llm.LLM)
	}

	tool := steps["lookup_order"]
	if tool.Kind != model.StepTool || tool.Tool == nil {
		t.Fatalf("TOOL span not classified: %+v", tool)
	}
	if tool.Tool.Name != "lookup_order" || tool.Tool.CallID != "call_88" {
		t.Errorf("tool identity: %+v", tool.Tool)
	}

	handoff := steps["handoff to Order Agent"]
	if handoff.Kind != model.StepTool || handoff.Tool == nil || handoff.Tool.Name != "handoff to Order Agent" {
		t.Errorf("handoff TOOL span-name fallback: %+v", handoff.Tool)
	}

	if g := steps["guardrail check"]; g.Kind != model.StepGeneric {
		t.Errorf("GUARDRAIL kind = %s, want generic", g.Kind)
	}
}
