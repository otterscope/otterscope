package store

import (
	"context"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

func stepWithMsg(id, runID, out string, sec int64) []model.Step {
	base := time.Unix(sec, 0)
	return []model.Step{
		{ID: runID + "-root", RunID: runID, Project: "default", Kind: model.StepAgent,
			Name: "invoke_agent a", Status: model.StatusOK, Start: base, End: base.Add(time.Second)},
		{ID: id, RunID: runID, ParentID: runID + "-root", Project: "default", Kind: model.StepLLM,
			Name: "chat m", Status: model.StatusOK, Start: base, End: base.Add(time.Second),
			LLM: &model.LLMCall{RequestModel: "m",
				OutputMessages: []model.Message{{Role: "assistant", Content: out}}}},
	}
}

func TestFullTextSearch(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	if err := st.UpsertSteps(ctx, stepWithMsg("s1", "run-ship", "Your order A-1042 has shipped today", 1000)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSteps(ctx, stepWithMsg("s2", "run-refund", "I have processed your refund", 2000)); err != nil {
		t.Fatal(err)
	}

	// Phrase in one run's message.
	runs, err := st.ListRuns(ctx, Filter{Query: "shipped"}, 10, 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != "run-ship" {
		t.Fatalf("search 'shipped' returned %+v", runs)
	}

	// Word only in the other.
	if runs, _ := st.ListRuns(ctx, Filter{Query: "refund"}, 10, 0); len(runs) != 1 || runs[0].ID != "run-refund" {
		t.Fatalf("search 'refund' returned %+v", runs)
	}

	// Punctuation-heavy token must not throw (FTS5 operators neutralized).
	if _, err := st.ListRuns(ctx, Filter{Query: "A-1042"}, 10, 0); err != nil {
		t.Fatalf("hyphenated query errored: %v", err)
	}
	if runs, _ := st.ListRuns(ctx, Filter{Query: "A-1042"}, 10, 0); len(runs) != 1 {
		t.Fatalf("search 'A-1042' returned %d, want 1", len(runs))
	}
	// Quote in query must not throw.
	if _, err := st.ListRuns(ctx, Filter{Query: `say "hi"`}, 10, 0); err != nil {
		t.Fatalf("quoted query errored: %v", err)
	}

	// Re-delivery keeps FTS in sync (no duplicate hit).
	if err := st.UpsertSteps(ctx, stepWithMsg("s1", "run-ship", "Your order A-1042 has shipped today", 1000)); err != nil {
		t.Fatal(err)
	}
	if runs, _ := st.ListRuns(ctx, Filter{Query: "shipped"}, 10, 0); len(runs) != 1 {
		t.Fatalf("re-delivery duplicated FTS hit: %d runs", len(runs))
	}

	// Stats respect the search filter too.
	stats, _ := st.GetStats(ctx, Filter{Query: "refund"})
	if stats.Runs != 1 {
		t.Fatalf("stats with search = %d runs, want 1", stats.Runs)
	}
}
