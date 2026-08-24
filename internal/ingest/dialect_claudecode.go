package ingest

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/otterscope/otterscope/internal/model"
)

// Claude Agent SDK / Claude Code dialect. The CLI emits its own `claude_code.*`
// span names with mostly bare attribute keys (`model`, `input_tokens`,
// `tool_name`) rather than the gen_ai.* conventions — only a handful of
// gen_ai.* aliases are duplicated alongside them, and `gen_ai.operation.name`
// is not among them. Without this handler every Agent SDK span falls through
// to StepGeneric: no model calls, no tokens, no cost, no tool loop.
//
// Tracing is explicitly beta upstream ("span names and attributes may change
// between releases"), which is exactly the churn ADR-0002 keeps in this
// package.
//
// Spec: code.claude.com/docs/en/monitoring-usage#traces-beta
//       code.claude.com/docs/en/agent-sdk/observability

func isClaudeCode(sp ptrace.Span) bool {
	return strings.HasPrefix(sp.Name(), "claude_code.")
}

func applyClaudeCode(sp ptrace.Span, st *model.Step) {
	attrs := sp.Attributes()

	// A subagent's spans carry the type it was spawned as; surfacing it makes
	// the delegation chain readable in the run list.
	if st.AgentName == "" {
		st.AgentName = stringAttr(attrs, "subagent_type")
	}

	switch sp.Name() {
	case "claude_code.llm_request":
		st.Kind = model.StepLLM
		st.LLM = &model.LLMCall{
			// gen_ai.system is duplicated by the CLI and is always "anthropic";
			// keep the bare-key fallbacks for older/newer builds.
			Provider:            stringAttr(attrs, "gen_ai.system", "gen_ai.provider.name"),
			RequestModel:        stringAttr(attrs, "gen_ai.request.model", "model"),
			InputTokens:         intAttr(attrs, "input_tokens"),
			OutputTokens:        intAttr(attrs, "output_tokens"),
			CacheReadTokens:     intAttr(attrs, "cache_read_tokens"),
			CacheCreationTokens: intAttr(attrs, "cache_creation_tokens"),
		}
		// No response model is emitted; pricing falls back to the request
		// model, which is what was billed here anyway.
		if st.LLM.Provider == "" {
			st.LLM.Provider = "anthropic"
		}
		applyClaudeCodeFailure(attrs, st)

	case "claude_code.tool":
		st.Kind = model.StepTool
		st.Tool = &model.ToolCall{
			Name:      stringAttr(attrs, "tool_name"),
			CallID:    stringAttr(attrs, "gen_ai.tool.call.id", "tool_use_id"),
			Arguments: claudeCodeToolArgs(attrs),
		}
		if st.Tool.Name == "" {
			st.Tool.Name = sp.Name()
		}
		// The output body arrives as a span event, not an attribute, and only
		// under OTEL_LOG_TOOL_CONTENT=1.
		if out := claudeCodeToolResult(sp); out != "" {
			st.Tool.Result = out
		}

	case "claude_code.tool.execution":
		// Timing detail under the tool span. Kept generic so it does not
		// double-count as a second tool call, but its failure is the one that
		// actually says the tool broke.
		st.Kind = model.StepGeneric
		applyClaudeCodeFailure(attrs, st)

	default:
		// claude_code.interaction (the run root), .tool.blocked_on_user,
		// .hook, and anything added by a later release.
		st.Kind = model.StepGeneric
		if sp.Name() == "claude_code.interaction" {
			st.Kind = model.StepAgent
		}
	}
}

// applyClaudeCodeFailure marks a step failed from the CLI's own attributes.
// The span status is left unset even on a failed request, so span status alone
// is not enough to notice an error.
//
// The upstream reference documents `success` as the string "true"/"false"; on
// the wire it is a bool. stringAttr goes through Value.AsString, which renders
// both as "true"/"false", so this comparison holds either way — do not
// "simplify" it to a bool accessor.
func applyClaudeCodeFailure(attrs pcommon.Map, st *model.Step) {
	if stringAttr(attrs, "success") != "false" {
		return
	}
	st.Status = model.StatusError
	if st.Error == "" {
		st.Error = stringAttr(attrs, "error")
	}
	if st.Error == "" {
		if code := stringAttr(attrs, "status_code"); code != "" {
			st.Error = "request failed with status " + code
		} else {
			st.Error = "error"
		}
	}
}

// claudeCodeToolArgs summarizes what a tool was asked to do from the
// documented per-tool attributes (all gated behind OTEL_LOG_TOOL_DETAILS=1).
// Returns "" when none are present, which is the default.
func claudeCodeToolArgs(attrs pcommon.Map) string {
	for _, key := range []string{"full_command", "file_path", "skill_name", "subagent_type"} {
		if v := stringAttr(attrs, key); v != "" {
			return v
		}
	}
	return ""
}

// claudeCodeToolResult pulls the tool's output body from the `tool.output`
// span event emitted under OTEL_LOG_TOOL_CONTENT=1.
//
// The upstream reference names the event but not its attributes. A real
// capture (Claude Code 2.1.241, Read tool) carries them as `content` for the
// body plus an echo of the tool's own input attributes — `file_path` here —
// so there is no key called "output" to look for. `content` is the body for
// every tool; the suffix match is a hedge for a rename in a beta dialect.
func claudeCodeToolResult(sp ptrace.Span) string {
	events := sp.Events()
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if ev.Name() != "tool.output" {
			continue
		}
		if v := stringAttr(ev.Attributes(), "content"); v != "" {
			return v
		}
		var fallback string
		ev.Attributes().Range(func(k string, v pcommon.Value) bool {
			lk := strings.ToLower(k)
			if fallback == "" && (strings.Contains(lk, "output") || strings.Contains(lk, "result")) {
				fallback = v.AsString()
			}
			return true
		})
		if fallback != "" {
			return fallback
		}
	}
	return ""
}
