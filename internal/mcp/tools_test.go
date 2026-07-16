package mcp

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seed(t *testing.T, st *store.Store, id string, startSec int64) {
	t.Helper()
	base := time.Unix(startSec, 0)
	err := st.UpsertSteps(context.Background(), []model.Step{
		{ID: id + "-root", RunID: id, Project: "default", Kind: model.StepAgent,
			Name: "invoke_agent demo", Service: "svc", AgentName: "demo",
			Status: model.StatusOK, Start: base, End: base.Add(5 * time.Second)},
		{ID: id + "-llm", RunID: id, ParentID: id + "-root", Project: "default", Kind: model.StepLLM,
			Name: "chat m1", Status: model.StatusOK, Start: base.Add(time.Second), End: base.Add(3 * time.Second),
			LLM: &model.LLMCall{RequestModel: "claude-sonnet-5", InputTokens: 100, OutputTokens: 20,
				InputMessages:  []model.Message{{Role: "user", Content: "hi"}},
				OutputMessages: []model.Message{{Role: "assistant", Content: "hello"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func toolByName(reg []Tool, name string) Tool {
	for _, tl := range reg {
		if tl.Name == name {
			return tl
		}
	}
	panic("no tool " + name)
}

func TestListRunsTool(t *testing.T) {
	st := testStore(t)
	seed(t, st, "old", 1000)
	seed(t, st, "new", 2000)
	reg := Registry(st)

	out, err := toolByName(reg, "list_runs").Handler(context.Background(), map[string]any{"limit": float64(10)})
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Runs []map[string]any `json:"runs"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.Runs) != 2 || resp.Runs[0]["id"] != "new" {
		t.Fatalf("expected newest-first 2 runs, got %+v", resp.Runs)
	}
}

func TestGetRunTool(t *testing.T) {
	st := testStore(t)
	seed(t, st, "r1", 1000)
	reg := Registry(st)

	out, err := toolByName(reg, "get_run").Handler(context.Background(), map[string]any{"id": "r1"})
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Run   map[string]any   `json:"run"`
		Steps []map[string]any `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Run["id"] != "r1" || len(resp.Steps) != 2 {
		t.Fatalf("run/steps wrong: %+v", resp)
	}
	// The LLM step should carry its messages for the agent to read.
	var llmStep map[string]any
	for _, s := range resp.Steps {
		if s["kind"] == "llm" {
			llmStep = s
		}
	}
	if llmStep == nil || llmStep["llm"] == nil {
		t.Fatalf("llm step missing detail: %+v", resp.Steps)
	}
}

func TestGetRunToolMissing(t *testing.T) {
	reg := Registry(testStore(t))
	if _, err := toolByName(reg, "get_run").Handler(context.Background(), map[string]any{"id": "nope"}); err == nil {
		t.Fatal("expected not-found error")
	}
	if _, err := toolByName(reg, "get_run").Handler(context.Background(), map[string]any{}); err == nil {
		t.Fatal("expected required-id error")
	}
}

func TestStatsTool(t *testing.T) {
	st := testStore(t)
	seed(t, st, "r1", 1000)
	reg := Registry(st)
	out, err := toolByName(reg, "get_stats").Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var stats store.Stats
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Runs != 1 {
		t.Fatalf("stats runs = %d, want 1", stats.Runs)
	}
}
