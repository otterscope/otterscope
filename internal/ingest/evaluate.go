package ingest

import (
	"context"
	"fmt"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

// EvaluateRuns scores each completed run against its project's enabled
// assertions. Safe to re-run: results upsert by (run, assertion).
func EvaluateRuns(ctx context.Context, st *store.Store, runIDs []string) error {
	cache := map[string][]evals.Assertion{} // project → enabled assertions
	for _, id := range runIDs {
		run, steps, err := st.GetRun(ctx, id)
		if err != nil {
			return fmt.Errorf("load run %s: %w", id, err)
		}
		if run.Status == model.StatusRunning {
			continue // scored once the root span arrives
		}
		asserts, ok := cache[run.Project]
		if !ok {
			all, err := st.ListAssertions(ctx, run.Project)
			if err != nil {
				return err
			}
			asserts = asserts[:0]
			for _, a := range all {
				if a.Enabled {
					asserts = append(asserts, a)
				}
			}
			cache[run.Project] = asserts
		}
		if len(asserts) == 0 {
			continue
		}
		results := make([]evals.Result, 0, len(asserts))
		for _, a := range asserts {
			results = append(results, evals.Evaluate(a, run, steps))
		}
		if err := st.SaveAssertionResults(ctx, run.ID, results); err != nil {
			return fmt.Errorf("save results for %s: %w", id, err)
		}
	}
	return nil
}

// EvaluateProject backfills assertion results over every completed run in a
// project (on-demand evaluation).
func EvaluateProject(ctx context.Context, st *store.Store, project string) (int, error) {
	const page = 200
	total := 0
	for offset := 0; ; offset += page {
		runs, err := st.ListRuns(ctx, store.Filter{Project: project}, page, offset)
		if err != nil {
			return total, err
		}
		if len(runs) == 0 {
			return total, nil
		}
		ids := make([]string, 0, len(runs))
		for _, r := range runs {
			if r.Status != model.StatusRunning {
				ids = append(ids, r.ID)
			}
		}
		if err := EvaluateRuns(ctx, st, ids); err != nil {
			return total, err
		}
		total += len(ids)
	}
}
