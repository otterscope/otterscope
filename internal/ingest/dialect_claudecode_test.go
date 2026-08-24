package ingest

import (
	"strings"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/pricing"
)

// Span IDs from the captured fixture (Claude Code 2.1.241, one headless run
// that read a file and answered).
const (
	ccInteraction = "99483a068ece8574"
	ccLLMToolUse  = "29903f83637c6d1c" // first request; stop_reason tool_use
	ccTool        = "1bf15d19445acb22" // Read
	ccBlocked     = "9346a69ba1a118c9"
	ccExecution   = "7dd0b69d8515823e"
	ccLLMFinal    = "8fc6f67c996714d4" // second request; stop_reason end_turn
)

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
	if len(steps) != 6 {
		t.Fatalf("got %d steps, want 6", len(steps))
	}

	interaction := steps[ccInteraction]
	if interaction.Kind != model.StepAgent || interaction.ParentID != "" {
		t.Errorf("interaction should be the agent root step: %+v", interaction)
	}
	if interaction.Service != "capture-probe" {
		t.Errorf("service = %q, want the OTEL_SERVICE_NAME override", interaction.Service)
	}

	llm := steps[ccLLMToolUse]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("llm_request not an llm step: %+v", llm)
	}
	if llm.LLM.Provider != "anthropic" {
		t.Errorf("provider = %q", llm.LLM.Provider)
	}
	// Claude Code reports the model with its context-window suffix. Pricing
	// resolves it by longest-prefix match, so the raw value is kept.
	if llm.LLM.RequestModel != "claude-opus-5[1m]" {
		t.Errorf("model = %q, want the raw id including the [1m] suffix", llm.LLM.RequestModel)
	}
	if llm.LLM.InputTokens != 2 || llm.LLM.OutputTokens != 110 {
		t.Errorf("tokens = %d/%d, want 2/110", llm.LLM.InputTokens, llm.LLM.OutputTokens)
	}
	// Cache tokens come from bare keys, not gen_ai.usage.*. They dwarf
	// input_tokens on a real run, which is the whole reason they matter.
	if llm.LLM.CacheReadTokens != 16024 || llm.LLM.CacheCreationTokens != 15181 {
		t.Errorf("cache tokens = %d/%d, want 16024/15181",
			llm.LLM.CacheReadTokens, llm.LLM.CacheCreationTokens)
	}
	if llm.Status != model.StatusOK {
		t.Errorf("successful request marked %q", llm.Status)
	}

	tool := steps[ccTool]
	if tool.Kind != model.StepTool || tool.Tool == nil {
		t.Fatalf("tool span not a tool step: %+v", tool)
	}
	if tool.Tool.Name != "Read" || tool.Tool.CallID != "toolu_01A7oeJTwLNmJ5VfabAjmJds" {
		t.Errorf("tool identity: %+v", tool.Tool)
	}
	if tool.Tool.Arguments != "/workspace/orders.txt" {
		t.Errorf("tool arguments = %q, want the file_path attribute", tool.Tool.Arguments)
	}
	// The output body is on the tool.output event under the key `content` —
	// not "output", which is what the docs' wording implies.
	if want := "A-1042 shipped 2026-08-20"; !strings.Contains(tool.Tool.Result, want) {
		t.Errorf("tool result = %q, want it to contain %q", tool.Tool.Result, want)
	}

	// Permission wait and execution are timing detail, not a second tool
	// call — counting them would double every tool in the run stats.
	for _, id := range []string{ccBlocked, ccExecution} {
		if s := steps[id]; s.Kind != model.StepGeneric || s.Tool != nil {
			t.Errorf("%s should be generic, got %+v", s.Name, s)
		}
	}

	if final := steps[ccLLMFinal]; final.Kind != model.StepLLM {
		t.Errorf("second llm_request kind = %q", final.Kind)
	}
}

// input_tokens excludes the cache tokens on a real Claude Code run (2 vs
// 16024 + 15181), the opposite of the OTel GenAI convention. Cost must bill
// cache additively rather than clamping the remainder to zero.
func TestClaudeAgentSDKCostBillsCacheAdditively(t *testing.T) {
	llm := claudeCodeSteps(t)[ccLLMToolUse].LLM
	priceSteps([]model.Step{{Kind: model.StepLLM, LLM: llm}}, pricing.Default())
	if llm.CostUSD == nil {
		t.Fatal("no cost for claude-opus-5[1m] — is the model missing from the pricing table?")
	}
	// 2 in @ $5 + 16024 read @ $0.50 + 15181 write @ $6.25 + 110 out @ $25 /1e6
	want := (2*5.0 + 16024*0.50 + 15181*6.25 + 110*25.0) / 1e6
	if diff := *llm.CostUSD - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost = %v, want %v", *llm.CostUSD, want)
	}
}

// The capture had no failed request, so this span is synthetic — the point is
// that `success` is a *bool* on the wire while the reference documents it as
// a string, and that a failed request leaves the span status unset.
func TestClaudeAgentSDKFailedRequest(t *testing.T) {
	const payload = `{"resourceSpans":[{"resource":{"attributes":[
	  {"key":"service.name","value":{"stringValue":"probe"}}]},
	  "scopeSpans":[{"spans":[{
	    "traceId":"9f2c41a7be004d18a1c7d3e5b6084411","spanId":"1a2b3c4d5e6f7006",
	    "name":"claude_code.llm_request","startTimeUnixNano":"1756900012500000000",
	    "endTimeUnixNano":"1756900014100000000","attributes":[
	      {"key":"gen_ai.request.model","value":{"stringValue":"claude-opus-5[1m]"}},
	      {"key":"success","value":{"boolValue":false}},
	      {"key":"status_code","value":{"intValue":"529"}},
	      {"key":"error","value":{"stringValue":"overloaded_error"}}],
	    "status":{}}]}]}]}`

	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON([]byte(payload)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	steps := Normalize(req.Traces())
	if len(steps) != 1 {
		t.Fatalf("got %d steps", len(steps))
	}
	failed := steps[0]
	if failed.Kind != model.StepLLM {
		t.Fatalf("kind = %q, want llm", failed.Kind)
	}
	if failed.Status != model.StatusError {
		t.Errorf("status = %q, want error — a bool success=false was not honoured", failed.Status)
	}
	if failed.Error != "overloaded_error" {
		t.Errorf("error = %q", failed.Error)
	}
}
