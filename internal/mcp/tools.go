// Package mcp exposes Otterscope's read-only data as MCP tools over a
// Streamable HTTP endpoint, so an agent can query its own traces.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/otterscope/otterscope/internal/store"
)

// Tool is one MCP tool: metadata plus a handler that takes decoded arguments
// and returns a text result. Handlers are read-only.
type Tool struct {
	Name        string
	Title       string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

// Registry builds the tool set backed by st.
func Registry(st *store.Store) []Tool {
	return []Tool{
		{
			Name:        "list_runs",
			Title:       "List agent runs",
			Description: "List recent agent runs, newest first. Optional filters: project, status (ok|error|running), model substring, service, and limit.",
			InputSchema: object(props{
				"project": strProp("Project name (default: all)"),
				"status":  strProp("Filter by run status: ok, error, or running"),
				"model":   strProp("Substring match on model id"),
				"service": strProp("Filter by service name"),
				"limit":   intProp("Max runs to return (default 20, max 200)"),
			}, nil),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				f := store.Filter{
					Project: argStr(args, "project"),
					Status:  argStr(args, "status"),
					Model:   argStr(args, "model"),
					Service: argStr(args, "service"),
				}
				limit := argInt(args, "limit", 20)
				if limit <= 0 || limit > 200 {
					limit = 20
				}
				runs, err := st.ListRuns(ctx, f, limit, 0)
				if err != nil {
					return "", err
				}
				return jsonResult(map[string]any{"runs": runsToMCP(runs)})
			},
		},
		{
			Name:        "get_run",
			Title:       "Get a run with its steps",
			Description: "Fetch one run by id (trace id) with its full step tree, including LLM messages and tool arguments/results.",
			InputSchema: object(props{
				"id": strProp("Run id (trace id, hex)"),
			}, []string{"id"}),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				id := argStr(args, "id")
				if id == "" {
					return "", fmt.Errorf("id is required")
				}
				run, steps, err := st.GetRun(ctx, id)
				if err == store.ErrNotFound {
					return "", fmt.Errorf("run %q not found", id)
				}
				if err != nil {
					return "", err
				}
				return jsonResult(map[string]any{
					"run":   runToMCP(run),
					"steps": stepsToMCP(steps),
				})
			},
		},
		{
			Name:        "get_stats",
			Title:       "Aggregate run statistics",
			Description: "Aggregate stats over a filtered slice of runs: count, error rate, p50/p95 latency, cost, tokens, and assertion pass rates. Same filters as list_runs plus sinceMinutes.",
			InputSchema: object(props{
				"project":      strProp("Project name (default: all)"),
				"status":       strProp("Filter by run status"),
				"model":        strProp("Substring match on model id"),
				"service":      strProp("Filter by service name"),
				"sinceMinutes": intProp("Only runs started within the last N minutes"),
			}, nil),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				f := store.Filter{
					Project: argStr(args, "project"),
					Status:  argStr(args, "status"),
					Model:   argStr(args, "model"),
					Service: argStr(args, "service"),
				}
				if m := argInt(args, "sinceMinutes", 0); m > 0 {
					f.Since = timeNow().Add(-time.Duration(m) * time.Minute)
				}
				stats, err := st.GetStats(ctx, f)
				if err != nil {
					return "", err
				}
				return jsonResult(stats)
			},
		},
		{
			Name:        "list_assertions",
			Title:       "List eval assertions",
			Description: "List the eval assertions configured for a project (contains, regex, latency/cost thresholds, llm_judge).",
			InputSchema: object(props{
				"project": strProp("Project name (default: all)"),
			}, nil),
			Handler: func(ctx context.Context, args map[string]any) (string, error) {
				as, err := st.ListAssertions(ctx, argStr(args, "project"))
				if err != nil {
					return "", err
				}
				return jsonResult(map[string]any{"assertions": as})
			},
		},
	}
}

// timeNow is swappable in tests.
var timeNow = time.Now

// --- argument helpers ---

func argStr(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func argInt(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64: // JSON numbers decode to float64
		return int(v)
	case int:
		return v
	}
	return def
}

func jsonResult(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// --- schema builders ---

type props map[string]map[string]any

func object(p props, required []string) map[string]any {
	m := map[string]any{"type": "object", "properties": map[string]any(convert(p))}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func convert(p props) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func intProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
