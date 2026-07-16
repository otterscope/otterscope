package ingest

import (
	"testing"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/model"
)

func normalizeFixture(t *testing.T) map[string]model.Step {
	t.Helper()
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON(fixture(t, "pydantic_ai_chat.json")); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	steps := Normalize(req.Traces())
	byName := make(map[string]model.Step, len(steps))
	for _, s := range steps {
		byName[s.Name] = s
	}
	return byName
}

func TestNormalizeFixture(t *testing.T) {
	steps := normalizeFixture(t)
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}

	agent := steps["invoke_agent support-agent"]
	if agent.Kind != model.StepAgent {
		t.Errorf("agent kind = %s", agent.Kind)
	}
	if agent.AgentName != "support-agent" || agent.Service != "support-agent" {
		t.Errorf("agent identity: %+v", agent)
	}
	if agent.ParentID != "" {
		t.Errorf("agent should be root, parent = %q", agent.ParentID)
	}
	if agent.RunID != "5b8efff798038103d269b633813fc60c" {
		t.Errorf("run ID = %q", agent.RunID)
	}

	llm := steps["chat claude-sonnet-5"]
	if llm.Kind != model.StepLLM || llm.LLM == nil {
		t.Fatalf("llm step not classified: %+v", llm)
	}
	if llm.LLM.Provider != "anthropic" || llm.LLM.RequestModel != "claude-sonnet-5" {
		t.Errorf("llm identity: %+v", llm.LLM)
	}
	if llm.LLM.InputTokens != 812 || llm.LLM.OutputTokens != 142 {
		t.Errorf("tokens = %d/%d, want 812/142", llm.LLM.InputTokens, llm.LLM.OutputTokens)
	}
	if llm.ParentID != agent.ID {
		t.Errorf("llm parent = %q, want %q", llm.ParentID, agent.ID)
	}
	if !llm.End.After(llm.Start) {
		t.Errorf("llm duration not positive: %v..%v", llm.Start, llm.End)
	}

	tool := steps["execute_tool get_ticket"]
	if tool.Kind != model.StepTool || tool.Tool == nil {
		t.Fatalf("tool step not classified: %+v", tool)
	}
	if tool.Tool.Name != "get_ticket" || tool.Tool.CallID != "toolu_01A" {
		t.Errorf("tool identity: %+v", tool.Tool)
	}
}

func span(op string, extra map[string]any) ptrace.Traces {
	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	sp := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	sp.SetName("test-span")
	sp.SetTraceID(pcommon.TraceID([16]byte{1}))
	sp.SetSpanID(pcommon.SpanID([8]byte{2}))
	sp.SetStartTimestamp(pcommon.Timestamp(1_000_000_000))
	sp.SetEndTimestamp(pcommon.Timestamp(2_000_000_000))
	if op != "" {
		sp.Attributes().PutStr("gen_ai.operation.name", op)
	}
	for k, v := range extra {
		switch v := v.(type) {
		case string:
			sp.Attributes().PutStr(k, v)
		case int:
			sp.Attributes().PutInt(k, int64(v))
		}
	}
	return td
}

func TestNormalizeTable(t *testing.T) {
	tests := []struct {
		name  string
		td    ptrace.Traces
		check func(t *testing.T, s model.Step)
	}{
		{
			name: "non-genai span kept as generic",
			td:   span("", nil),
			check: func(t *testing.T, s model.Step) {
				if s.Kind != model.StepGeneric {
					t.Errorf("kind = %s, want generic", s.Kind)
				}
			},
		},
		{
			name: "unknown operation kept as generic",
			td:   span("summon_demon", nil),
			check: func(t *testing.T, s model.Step) {
				if s.Kind != model.StepGeneric {
					t.Errorf("kind = %s, want generic", s.Kind)
				}
			},
		},
		{
			name: "legacy prompt/completion token aliases",
			td: span("chat", map[string]any{
				"gen_ai.usage.prompt_tokens":     100,
				"gen_ai.usage.completion_tokens": 25,
			}),
			check: func(t *testing.T, s model.Step) {
				if s.LLM == nil || s.LLM.InputTokens != 100 || s.LLM.OutputTokens != 25 {
					t.Errorf("legacy tokens not read: %+v", s.LLM)
				}
			},
		},
		{
			name: "tool without name falls back to span name",
			td:   span("execute_tool", nil),
			check: func(t *testing.T, s model.Step) {
				if s.Tool == nil || s.Tool.Name != "test-span" {
					t.Errorf("tool fallback: %+v", s.Tool)
				}
			},
		},
		{
			name: "error status carries message",
			td: func() ptrace.Traces {
				td := span("chat", nil)
				st := td.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(0).Status()
				st.SetCode(ptrace.StatusCodeError)
				st.SetMessage("rate limited")
				return td
			}(),
			check: func(t *testing.T, s model.Step) {
				if s.Status != model.StatusError || s.Error != "rate limited" {
					t.Errorf("status/error = %s/%q", s.Status, s.Error)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			steps := Normalize(tc.td)
			if len(steps) != 1 {
				t.Fatalf("got %d steps, want 1 — spans must never be dropped", len(steps))
			}
			tc.check(t, steps[0])
		})
	}
}
