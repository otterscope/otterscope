package mcp

import (
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// Compact, agent-friendly projections of the domain types. Kept separate
// from the HTTP API's wire types so the two can evolve independently.

func runToMCP(r model.Run) map[string]any {
	m := map[string]any{
		"id":           r.ID,
		"project":      r.Project,
		"service":      r.Service,
		"agent":        r.AgentName,
		"status":       string(r.Status),
		"start":        r.Start.UTC().Format(time.RFC3339),
		"durationMs":   r.End.Sub(r.Start).Milliseconds(),
		"inputTokens":  r.InputTokens,
		"outputTokens": r.OutputTokens,
		"llmCalls":     r.LLMCalls,
		"toolCalls":    r.ToolCalls,
		"models":       r.Models,
	}
	if r.CostUSD != nil {
		m["costUsd"] = *r.CostUSD
	}
	if r.Error != "" {
		m["error"] = r.Error
	}
	return m
}

func runsToMCP(runs []model.Run) []map[string]any {
	out := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		out = append(out, runToMCP(r))
	}
	return out
}

func stepsToMCP(steps []model.Step) []map[string]any {
	out := make([]map[string]any, 0, len(steps))
	for _, s := range steps {
		m := map[string]any{
			"id":         s.ID,
			"parentId":   s.ParentID,
			"kind":       string(s.Kind),
			"name":       s.Name,
			"status":     string(s.Status),
			"durationMs": s.End.Sub(s.Start).Milliseconds(),
		}
		if s.Error != "" {
			m["error"] = s.Error
		}
		if s.LLM != nil {
			llm := map[string]any{
				"model":        s.LLM.RequestModel,
				"provider":     s.LLM.Provider,
				"inputTokens":  s.LLM.InputTokens,
				"outputTokens": s.LLM.OutputTokens,
			}
			if s.LLM.CostUSD != nil {
				llm["costUsd"] = *s.LLM.CostUSD
			}
			if len(s.LLM.InputMessages) > 0 {
				llm["inputMessages"] = s.LLM.InputMessages
			}
			if len(s.LLM.OutputMessages) > 0 {
				llm["outputMessages"] = s.LLM.OutputMessages
			}
			m["llm"] = llm
		}
		if s.Tool != nil {
			tool := map[string]any{"name": s.Tool.Name}
			if s.Tool.Arguments != "" {
				tool["arguments"] = s.Tool.Arguments
			}
			if s.Tool.Result != "" {
				tool["result"] = s.Tool.Result
			}
			m["tool"] = tool
		}
		out = append(out, m)
	}
	return out
}
