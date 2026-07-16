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

func TestEvaluatorDrainsOnStop(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// A deterministic assertion so no network is needed.
	if _, err := st.CreateAssertion(ctx, evals.Assertion{
		Name: "fast", Type: "max_latency_ms", Config: "60000", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1000, 0)
	st.UpsertSteps(ctx, []model.Step{{
		ID: "r1-root", RunID: "r1", Project: "default", Kind: model.StepAgent,
		Name: "invoke_agent x", Status: model.StatusOK, Start: base, End: base.Add(time.Second),
	}})

	e := NewEvaluator(st, evals.Endpoint{})
	e.Start()
	e.Enqueue([]string{"r1"})
	e.Stop() // must block until the queued evaluation is persisted

	results, err := st.ResultsForRun(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Pass {
		t.Fatalf("evaluation did not complete before Stop returned: %+v", results)
	}
}
