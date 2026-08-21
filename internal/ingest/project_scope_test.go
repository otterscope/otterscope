package ingest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

// seedRunIn writes a one-step run into a project. The run id is deliberately
// reusable across projects — that is legal since #49 rekeyed by (project, id).
func seedRunIn(t *testing.T, st *store.Store, project, runID, output string) {
	t.Helper()
	base := time.Unix(1000, 0)
	err := st.UpsertSteps(context.Background(), []model.Step{
		{
			ID: project + "-" + runID + "-root", RunID: runID, Project: project,
			Kind: model.StepAgent, Name: "invoke_agent x", Status: model.StatusOK,
			Start: base, End: base.Add(time.Second),
		},
		{
			ID: project + "-" + runID + "-llm", RunID: runID, Project: project,
			ParentID: project + "-" + runID + "-root",
			Kind:     model.StepLLM, Name: "chat", Status: model.StatusOK,
			Start: base, End: base.Add(time.Second),
			LLM: &model.LLMCall{
				RequestModel:   "test-model",
				OutputMessages: []model.Message{{Role: "assistant", Content: output}},
			},
		},
	})
	if err != nil {
		t.Fatalf("seed %s/%s: %v", project, runID, err)
	}
}

// Two projects can hold the same trace id. Each run must be scored only
// against its own project's assertions, and neither may see the other's
// results (#98).
//
// Before the fix, EvaluateRuns resolved a bare run id with GetRun ("newest
// wins" across projects), so the run loaded here belonged to whichever
// project was written last, and the single (run_id, assertion_id) key meant
// one project's verdict overwrote the other's.
func TestEvaluationIsScopedToProject(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "scope.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.CreateProject(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateProject(ctx, "beta"); err != nil {
		t.Fatal(err)
	}

	// Each project asserts something only its own run satisfies.
	alphaAssert, err := st.CreateAssertion(ctx, evals.Assertion{
		Project: "alpha", Name: "says-alpha", Type: "contains",
		Config: "alpha", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	betaAssert, err := st.CreateAssertion(ctx, evals.Assertion{
		Project: "beta", Name: "says-beta", Type: "contains",
		Config: "beta", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The same trace id in both projects, with content matching only its own.
	const runID = "0123456789abcdef"
	seedRunIn(t, st, "alpha", runID, "hello from alpha")
	seedRunIn(t, st, "beta", runID, "hello from beta")

	refs := []RunRef{{Project: "alpha", ID: runID}, {Project: "beta", ID: runID}}
	if err := EvaluateRuns(ctx, st, evals.Endpoint{}, refs, false); err != nil {
		t.Fatalf("EvaluateRuns: %v", err)
	}

	for _, tc := range []struct {
		project string
		wantID  int64
		wantNam string
	}{
		{"alpha", alphaAssert.ID, "says-alpha"},
		{"beta", betaAssert.ID, "says-beta"},
	} {
		results, err := st.ResultsForRun(ctx, tc.project, runID)
		if err != nil {
			t.Fatalf("ResultsForRun(%s): %v", tc.project, err)
		}
		if len(results) != 1 {
			t.Fatalf("%s: got %d results, want exactly its own: %+v", tc.project, len(results), results)
		}
		if results[0].AssertionID != tc.wantID {
			t.Errorf("%s: scored against assertion %d (%s), want %d (%s)",
				tc.project, results[0].AssertionID, results[0].Name, tc.wantID, tc.wantNam)
		}
		if !results[0].Pass {
			t.Errorf("%s: assertion failed, so the wrong run's content was scored: %+v", tc.project, results[0])
		}
	}
}

// The stats join must not fan out across projects: one result per project
// must count once, not once per project sharing the trace id (#98).
func TestAssertionRatesDoNotFanOutAcrossProjects(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "rates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, err := st.CreateAssertion(ctx, evals.Assertion{
		Project: "alpha", Name: "says-alpha", Type: "contains",
		Config: "alpha", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	const runID = "cafebabecafebabe"
	seedRunIn(t, st, "alpha", runID, "hello from alpha")
	seedRunIn(t, st, "beta", runID, "hello from beta")

	if err := st.SaveAssertionResults(ctx, "alpha", runID,
		[]evals.Result{{AssertionID: a.ID, Pass: true}}); err != nil {
		t.Fatal(err)
	}

	stats, err := st.GetStats(ctx, store.Filter{Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats.AssertionRates) != 1 {
		t.Fatalf("got %d assertion rates, want 1: %+v", len(stats.AssertionRates), stats.AssertionRates)
	}
	if got := stats.AssertionRates[0].Total; got != 1 {
		t.Errorf("total = %d, want 1 — the join counted the other project's run too", got)
	}
}
