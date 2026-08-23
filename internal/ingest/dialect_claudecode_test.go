package ingest

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/model"
)

// claudeCodeSteps normalizes the fixture and returns steps keyed by span ID,
// since Claude Code reuses the same span name for every model call.
func claudeCodeSteps(t *testing.T) map[string]model.Step {
	t.Helper()
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON(fixture(t, "claude_agent_sdk.json")); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	steps := Normalize(req.Traces())
	byID := make(map[string]model.Step, len(steps))
	for _, s := range steps {
		byID[s.ID] = s
	}
	return byID
}

// Without the claude_code dialect every one of these spans falls through to
// StepGeneric — no model calls, no tokens, no cost, no tool loop.
func TestClaudeAgentSDKFixture(t *testing.T) {
	steps := claudeCodeSteps(t)
	if len(steps) != 7 {
		t.Fatalf("got %d steps, want 7", len(steps))
	}

	interaction := steps["1a2b3c4d5e6f7001"]
	if interaction.Kind != model.StepAgent {
		t.Errorf("interaction kind = %q, want agent (it is the run root)", interaction.Kind)
	}
	if interaction.ParentID != "" {
		t.Errorf("interaction should be the root step, got parent %q", interaction.ParentID)
	}
	if interaction.Service != "support-triage-agent" {
		t.Errorf("service = %q, want the OTEL_SERVICE_NAME override", interaction.Service)
	}

	llm := steps["1a2b3c4d5e6f7002"]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("llm_request not an llm step: %+v", llm)
	}
	if llm.LLM.Provider != "anthropic" || llm.LLM.RequestModel != "claude-sonnet-5" {
		t.Errorf("llm identity: %+v", llm.LLM)
	}
	if llm.LLM.InputTokens != 4210 || llm.LLM.OutputTokens != 180 {
		t.Errorf("tokens = %d/%d, want 4210/180", llm.LLM.InputTokens, llm.LLM.OutputTokens)
	}
	// Claude Code reports cache tokens under bare keys, not gen_ai.usage.*;
	// missing them would overstate the cost of a heavily cached agent.
	if llm.LLM.CacheReadTokens != 3900 || llm.LLM.CacheCreationTokens != 260 {
		t.Errorf("cache tokens = %d/%d, want 3900/260",
			llm.LLM.CacheReadTokens, llm.LLM.CacheCreationTokens)
	}
	if llm.Status != model.StatusOK {
		t.Errorf("successful request marked %q", llm.Status)
	}

	tool := steps["1a2b3c4d5e6f7003"]
	if tool.Kind != model.StepTool || tool.Tool == nil {
		t.Fatalf("tool span not a tool step: %+v", tool)
	}
	if tool.Tool.Name != "Bash" || tool.Tool.CallID != "toolu_01A9F3" {
		t.Errorf("tool identity: %+v", tool.Tool)
	}
	// The tool.output span event wins over the full_command attribute.
	if tool.Tool.Arguments != `{"command":"grep -rn 'order A-1042' ./orders"}` {
		t.Errorf("tool arguments = %q", tool.Tool.Arguments)
	}
	if tool.Tool.Result == "" {
		t.Error("tool result not read from the tool.output span event")
	}

	// The permission wait and the execution are timing detail, not a second
	// tool call — counting them would double every tool in the run stats.
	for _, id := range []string{"1a2b3c4d5e6f7004", "1a2b3c4d5e6f7005"} {
		if s := steps[id]; s.Kind != model.StepGeneric || s.Tool != nil {
			t.Errorf("%s should be generic, got %+v", s.Name, s)
		}
	}
}

// A failed request carries success="false" as a *string* and often leaves the
// span status unset, so the span status alone does not reveal the failure.
func TestClaudeAgentSDKFailedRequest(t *testing.T) {
	failed := claudeCodeSteps(t)["1a2b3c4d5e6f7006"]
	if failed.Kind != model.StepLLM {
		t.Fatalf("kind = %q, want llm", failed.Kind)
	}
	if failed.Status != model.StatusError {
		t.Errorf("status = %q, want error — success=\"false\" was not honoured", failed.Status)
	}
	if failed.Error == "" {
		t.Error("no error message recorded")
	}
}

// Subagent spans nest under the parent's tool span; the subagent type is what
// makes the delegation readable.
func TestClaudeAgentSDKSubagent(t *testing.T) {
	sub := claudeCodeSteps(t)["1a2b3c4d5e6f7007"]
	if sub.Kind != model.StepLLM || sub.LLM == nil {
		t.Fatalf("subagent llm_request not an llm step: %+v", sub)
	}
	if sub.ParentID != "1a2b3c4d5e6f7003" {
		t.Errorf("subagent parent = %q, want the Agent tool span", sub.ParentID)
	}
	if sub.AgentName != "code-reviewer" {
		t.Errorf("agent name = %q, want code-reviewer", sub.AgentName)
	}
	if sub.LLM.RequestModel != "claude-haiku-4-5" {
		t.Errorf("subagent model = %q", sub.LLM.RequestModel)
	}
}
