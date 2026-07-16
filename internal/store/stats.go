package store

import (
	"context"
	"database/sql"
	"strings"
)

// AssertionRate is the pass rate of one assertion over a filtered run set.
type AssertionRate struct {
	AssertionID int64  `json:"assertionId"`
	Name        string `json:"name"`
	Passed      int64  `json:"passed"`
	Total       int64  `json:"total"`
}

// Stats aggregates a filtered slice of runs.
type Stats struct {
	Runs           int64           `json:"runs"`
	Errors         int64           `json:"errors"`
	AvgDurationMS  float64         `json:"avgDurationMs"`
	P50DurationMS  int64           `json:"p50DurationMs"`
	P95DurationMS  int64           `json:"p95DurationMs"`
	TotalCostUSD   *float64        `json:"totalCostUsd,omitempty"`
	AvgCostUSD     *float64        `json:"avgCostUsd,omitempty"`
	InputTokens    int64           `json:"inputTokens"`
	OutputTokens   int64           `json:"outputTokens"`
	AssertionRates []AssertionRate `json:"assertionRates"`
}

// filterWhere builds the shared WHERE clause for run filters.
func filterWhere(f Filter) (string, []any) {
	where := " WHERE 1=1"
	var args []any
	if f.Project != "" {
		where += " AND project = ?"
		args = append(args, f.Project)
	}
	if f.Status != "" {
		where += " AND status = ?"
		args = append(args, f.Status)
	}
	if f.Service != "" {
		where += " AND service LIKE ? ESCAPE '\\'"
		args = append(args, escapeLike(f.Service)+"%")
	}
	if f.Model != "" {
		where += " AND models LIKE ? ESCAPE '\\'"
		args = append(args, "%"+escapeLike(f.Model)+"%")
	}
	if q := ftsQuery(f.Query); q != "" {
		where += " AND EXISTS (SELECT 1 FROM steps_fts" +
			" WHERE steps_fts.project = runs.project AND steps_fts.run_id = runs.id" +
			" AND steps_fts MATCH ?)"
		args = append(args, q)
	}
	if !f.Since.IsZero() {
		where += " AND start_ns >= ?"
		args = append(args, f.Since.UnixNano())
	}
	if !f.Until.IsZero() {
		where += " AND start_ns <= ?"
		args = append(args, f.Until.UnixNano())
	}
	return where, args
}

// GetStats aggregates all runs matching f.
func (s *Store) GetStats(ctx context.Context, f Filter) (Stats, error) {
	where, args := filterWhere(f)
	var st Stats
	var totalCost, avgCost sql.NullFloat64
	err := s.reader.QueryRowContext(ctx, `
		SELECT count(*),
		       coalesce(sum(status = 'error'), 0),
		       coalesce(avg((end_ns - start_ns) / 1e6), 0),
		       sum(cost_usd), avg(cost_usd),
		       coalesce(sum(input_tokens), 0), coalesce(sum(output_tokens), 0)
		FROM runs`+where, args...).
		Scan(&st.Runs, &st.Errors, &st.AvgDurationMS, &totalCost, &avgCost,
			&st.InputTokens, &st.OutputTokens)
	if err != nil {
		return Stats{}, err
	}
	st.TotalCostUSD = floatPtr(totalCost)
	st.AvgCostUSD = floatPtr(avgCost)

	if st.Runs > 0 {
		if st.P50DurationMS, err = s.durationPercentile(ctx, where, args, st.Runs, 50); err != nil {
			return Stats{}, err
		}
		if st.P95DurationMS, err = s.durationPercentile(ctx, where, args, st.Runs, 95); err != nil {
			return Stats{}, err
		}
	}

	rows, err := s.reader.QueryContext(ctx, `
		SELECT a.id, a.name, coalesce(sum(ar.pass), 0), count(*)
		FROM assertion_results ar
		JOIN assertions a ON a.id = ar.assertion_id
		JOIN runs ON runs.id = ar.run_id`+
		// reuse the runs filter against the joined table
		replaceRunsAlias(where)+`
		GROUP BY a.id ORDER BY a.id`, args...)
	if err != nil {
		return Stats{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var r AssertionRate
		if err := rows.Scan(&r.AssertionID, &r.Name, &r.Passed, &r.Total); err != nil {
			return Stats{}, err
		}
		st.AssertionRates = append(st.AssertionRates, r)
	}
	return st, rows.Err()
}

// replaceRunsAlias qualifies filter columns for the joined stats query.
// filterWhere emits each column as " AND <col> ..." so the token is
// unambiguous.
func replaceRunsAlias(where string) string {
	out := where
	for _, col := range []string{"project", "status", "service", "models", "start_ns"} {
		out = strings.ReplaceAll(out, " AND "+col+" ", " AND runs."+col+" ")
	}
	return out
}

// durationPercentile computes the pth percentile of run durations under the
// filter using OFFSET on the known count.
func (s *Store) durationPercentile(ctx context.Context, where string, args []any, count int64, p int64) (int64, error) {
	offset := count * p / 100
	if offset >= count {
		offset = count - 1
	}
	q := `SELECT (end_ns - start_ns) / 1000000 FROM runs` + where +
		` ORDER BY (end_ns - start_ns) LIMIT 1 OFFSET ?`
	var ms int64
	err := s.reader.QueryRowContext(ctx, q, append(append([]any{}, args...), offset)...).Scan(&ms)
	return ms, err
}
