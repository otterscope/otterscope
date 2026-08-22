package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

// RunRef identifies a run for evaluation. A trace id alone does not: the
// same id can exist in more than one project, and resolving it without the
// project scored runs against the wrong project's assertions (#98).
type RunRef struct {
	Project string
	ID      string
}

// EvaluateRuns scores each completed run against its project's enabled
// assertions. Deterministic assertions always run; llm_judge assertions run
// subject to their sampleRate, or unconditionally when judgeAll is set
// (on-demand backfill). Safe to re-run: results upsert by
// (project, run, assertion).
func EvaluateRuns(ctx context.Context, st *store.Store, judge evals.Endpoint, refs []RunRef, judgeAll bool) error {
	cache := map[string][]evals.Assertion{} // project → enabled assertions
	for _, ref := range refs {
		project := ref.Project
		if project == "" {
			project = "default"
		}
		run, steps, err := st.GetRunInProject(ctx, project, ref.ID)
		if err != nil {
			return fmt.Errorf("load run %s/%s: %w", project, ref.ID, err)
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
		// Existing results let us skip re-judging (a paid call) when a
		// batch for an already-scored run is redelivered.
		var judged map[int64]bool
		if !judgeAll {
			judged = map[int64]bool{}
			if existing, err := st.ResultsForRun(ctx, run.Project, run.ID); err == nil {
				for _, r := range existing {
					judged[r.AssertionID] = true
				}
			}
		}
		var results []evals.Result
		for _, a := range asserts {
			if a.Type == "llm_judge" {
				if judged[a.ID] {
					continue // already scored; don't re-pay on redelivery
				}
				if judgeAll || sampled(a) {
					results = append(results, evals.Judge(ctx, judge, a, run, steps))
				}
				continue
			}
			results = append(results, evals.Evaluate(a, run, steps))
		}
		if err := st.SaveAssertionResults(ctx, run.Project, run.ID, results); err != nil {
			return fmt.Errorf("save results for %s/%s: %w", run.Project, run.ID, err)
		}
	}
	return nil
}

// sampled applies the judge's sampleRate (0 or unset = judge everything).
func sampled(a evals.Assertion) bool {
	var cfg struct {
		SampleRate float64 `json:"sampleRate"`
	}
	if err := json.Unmarshal([]byte(a.Config), &cfg); err != nil {
		return false
	}
	if cfg.SampleRate <= 0 || cfg.SampleRate >= 1 {
		return true
	}
	return rand.Float64() < cfg.SampleRate
}

// EvaluateProject backfills assertion results over every completed run in a
// project (on-demand evaluation).
func EvaluateProject(ctx context.Context, st *store.Store, judge evals.Endpoint, project string) (int, error) {
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
		refs := make([]RunRef, 0, len(runs))
		for _, r := range runs {
			if r.Status != model.StatusRunning {
				refs = append(refs, RunRef{Project: r.Project, ID: r.ID})
			}
		}
		if err := EvaluateRuns(ctx, st, judge, refs, true); err != nil {
			return total, err
		}
		total += len(refs)
	}
}
