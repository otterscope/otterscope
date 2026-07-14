package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func ts(sec int64) time.Time { return time.Unix(sec, 0) }

// sampleRun returns a complete run: agent root, one LLM call, one tool call.
func sampleRun(runID string, base int64) []model.Step {
	return []model.Step{
		{
			ID: runID + "-root", RunID: runID, Kind: model.StepAgent,
			Name: "invoke_agent support", Service: "support-agent", AgentName: "support",
			Status: model.StatusOK, Start: ts(base), End: ts(base + 10),
		},
		{
			ID: runID + "-llm", RunID: runID, ParentID: runID + "-root", Kind: model.StepLLM,
			Name: "chat claude-sonnet-5", Status: model.StatusOK,
			Start: ts(base + 1), End: ts(base + 4),
			LLM: &model.LLMCall{Provider: "anthropic", RequestModel: "claude-sonnet-5", InputTokens: 800, OutputTokens: 150},
		},
		{
			ID: runID + "-tool", RunID: runID, ParentID: runID + "-root", Kind: model.StepTool,
			Name: "execute_tool get_ticket", Status: model.StatusOK,
			Start: ts(base + 5), End: ts(base + 6),
			Tool: &model.ToolCall{Name: "get_ticket", CallID: "call-1"},
		},
	}
}

func TestUpsertAndGetRun(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.UpsertSteps(ctx, sampleRun("r1", 1000)); err != nil {
		t.Fatalf("UpsertSteps: %v", err)
	}

	run, steps, err := st.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != model.StatusOK {
		t.Errorf("status = %s, want ok", run.Status)
	}
	if run.Service != "support-agent" || run.AgentName != "support" {
		t.Errorf("service/agent = %q/%q", run.Service, run.AgentName)
	}
	if run.InputTokens != 800 || run.OutputTokens != 150 {
		t.Errorf("tokens = %d/%d, want 800/150", run.InputTokens, run.OutputTokens)
	}
	if run.LLMCalls != 1 || run.ToolCalls != 1 {
		t.Errorf("calls = %d llm / %d tool, want 1/1", run.LLMCalls, run.ToolCalls)
	}
	if !run.Start.Equal(ts(1000)) || !run.End.Equal(ts(1010)) {
		t.Errorf("time bounds = %v..%v", run.Start, run.End)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps, want 3", len(steps))
	}
	if steps[1].LLM == nil || steps[1].LLM.RequestModel != "claude-sonnet-5" {
		t.Errorf("llm step detail missing: %+v", steps[1])
	}
	if steps[2].Tool == nil || steps[2].Tool.Name != "get_ticket" {
		t.Errorf("tool step detail missing: %+v", steps[2])
	}
}

func TestIdempotentRedelivery(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	batch := sampleRun("r1", 1000)
	for i := 0; i < 3; i++ {
		if err := st.UpsertSteps(ctx, batch); err != nil {
			t.Fatalf("UpsertSteps #%d: %v", i, err)
		}
	}

	run, steps, err := st.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("got %d steps after triple delivery, want 3", len(steps))
	}
	if run.InputTokens != 800 || run.LLMCalls != 1 {
		t.Errorf("aggregates inflated by redelivery: %+v", run)
	}
}

func TestRunningUntilRootArrives(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	batch := sampleRun("r1", 1000)
	// Deliver children first, root last — common with batched exporters.
	if err := st.UpsertSteps(ctx, batch[1:]); err != nil {
		t.Fatalf("UpsertSteps children: %v", err)
	}
	run, _, err := st.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != model.StatusRunning {
		t.Errorf("status before root = %s, want running", run.Status)
	}

	if err := st.UpsertSteps(ctx, batch[:1]); err != nil {
		t.Fatalf("UpsertSteps root: %v", err)
	}
	run, _, err = st.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != model.StatusOK {
		t.Errorf("status after root = %s, want ok", run.Status)
	}
}

func TestErrorStatusWins(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	batch := sampleRun("r1", 1000)
	batch[2].Status = model.StatusError
	batch[2].Error = "tool exploded"
	if err := st.UpsertSteps(ctx, batch); err != nil {
		t.Fatalf("UpsertSteps: %v", err)
	}
	run, _, err := st.GetRun(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.Status != model.StatusError {
		t.Errorf("status = %s, want error", run.Status)
	}
	if run.Error != "tool exploded" {
		t.Errorf("error = %q", run.Error)
	}
}

func TestListRunsNewestFirst(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.UpsertSteps(ctx, sampleRun("old", 1000)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSteps(ctx, sampleRun("new", 2000)); err != nil {
		t.Fatal(err)
	}

	runs, err := st.ListRuns(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != "new" || runs[1].ID != "old" {
		t.Fatalf("wrong order: %+v", runs)
	}

	page, err := st.ListRuns(ctx, 1, 1)
	if err != nil {
		t.Fatalf("ListRuns paged: %v", err)
	}
	if len(page) != 1 || page[0].ID != "old" {
		t.Fatalf("paging broken: %+v", page)
	}
}

func TestGetRunNotFound(t *testing.T) {
	st := openTest(t)
	if _, _, err := st.GetRun(context.Background(), "nope"); err != ErrNotFound {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
