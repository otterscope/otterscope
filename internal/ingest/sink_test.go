package ingest

import (
	"context"
	"path/filepath"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/store"
)

func fixtureTraces(t *testing.T) ptraceotlp.ExportRequest {
	t.Helper()
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalJSON(fixture(t, "pydantic_ai_chat.json")); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return req
}

func TestStoreSinkPersistsAndRenormalizes(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	sink := NewStoreSink(st, pricing.Default(), evals.Endpoint{})
	if err := sink.ConsumeTraces(ctx, "default", fixtureTraces(t).Traces()); err != nil {
		t.Fatalf("ConsumeTraces: %v", err)
	}

	const runID = "5b8efff798038103d269b633813fc60c"
	run, steps, err := st.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun after ingest: %v", err)
	}
	if len(steps) != 3 || run.InputTokens != 812 {
		t.Fatalf("ingest result wrong: %d steps, %d input tokens", len(steps), run.InputTokens)
	}

	// Simulate a pre-improvement gap: drop a step row, then renormalize
	// from the retained raw batch. The step must come back.
	if _, err := st.DB().ExecContext(ctx, `DELETE FROM steps WHERE kind = 'tool'`); err != nil {
		t.Fatalf("delete step: %v", err)
	}
	if _, _, err := st.GetRun(ctx, runID); err != nil {
		t.Fatalf("GetRun after delete: %v", err)
	}

	if _, err := Renormalize(ctx, st, pricing.Default()); err != nil {
		t.Fatalf("Renormalize: %v", err)
	}
	run, steps, err = st.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun after renormalize: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("renormalize restored %d steps, want 3", len(steps))
	}
	if run.ToolCalls != 1 || run.InputTokens != 812 {
		t.Fatalf("aggregates after renormalize: %+v", run)
	}

	// Renormalizing again must not duplicate anything.
	if _, err := Renormalize(ctx, st, pricing.Default()); err != nil {
		t.Fatalf("second Renormalize: %v", err)
	}
	_, steps, err = st.GetRun(ctx, runID)
	if err != nil {
		t.Fatalf("GetRun after second renormalize: %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("second renormalize changed step count to %d", len(steps))
	}
}
