package store

import (
	"context"
	"testing"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
)

func TestGetStats(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	// Two ok runs (10s each) and one error run.
	for i, id := range []string{"a", "b"} {
		if err := st.UpsertSteps(ctx, sampleRun(id, 1000+int64(i)*100)); err != nil {
			t.Fatal(err)
		}
	}
	bad := sampleRun("c", 2000)
	bad[1].Status = model.StatusError
	bad[1].Error = "boom"
	if err := st.UpsertSteps(ctx, bad); err != nil {
		t.Fatal(err)
	}

	a, err := st.CreateAssertion(ctx, evals.Assertion{Name: "n", Type: "is_json", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	st.SaveAssertionResults(ctx, "a", []evals.Result{{AssertionID: a.ID, Pass: true}})
	st.SaveAssertionResults(ctx, "b", []evals.Result{{AssertionID: a.ID, Pass: false}})

	stats, err := st.GetStats(ctx, Filter{})
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.Runs != 3 || stats.Errors != 1 {
		t.Errorf("runs/errors = %d/%d", stats.Runs, stats.Errors)
	}
	if stats.AvgDurationMS != 10000 || stats.P50DurationMS != 10000 || stats.P95DurationMS != 10000 {
		t.Errorf("durations: %+v", stats)
	}
	if stats.InputTokens != 2400 {
		t.Errorf("input tokens = %d, want 2400", stats.InputTokens)
	}
	if len(stats.AssertionRates) != 1 || stats.AssertionRates[0].Passed != 1 || stats.AssertionRates[0].Total != 2 {
		t.Errorf("assertion rates: %+v", stats.AssertionRates)
	}

	// Filter narrows: only the error run.
	stats, err = st.GetStats(ctx, Filter{Status: "error"})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Runs != 1 || stats.Errors != 1 || len(stats.AssertionRates) != 0 {
		t.Errorf("filtered stats: %+v", stats)
	}

	// Empty slice: zeros, no percentile query panic.
	stats, err = st.GetStats(ctx, Filter{Service: "nothing"})
	if err != nil || stats.Runs != 0 {
		t.Errorf("empty stats: %+v err=%v", stats, err)
	}
}
