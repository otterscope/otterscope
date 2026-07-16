package store

import (
	"context"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
)

// CreateAssertion stores a new assertion (already Validated by the caller).
func (s *Store) CreateAssertion(ctx context.Context, a evals.Assertion) (evals.Assertion, error) {
	if a.Project == "" {
		a.Project = "default"
	}
	res, err := s.writer.ExecContext(ctx,
		`INSERT INTO assertions (project, name, type, config, enabled, created_ns)
		 VALUES (?,?,?,?,?,?)`,
		a.Project, a.Name, a.Type, a.Config, a.Enabled, time.Now().UnixNano())
	if err != nil {
		return evals.Assertion{}, err
	}
	a.ID, _ = res.LastInsertId()
	return a, nil
}

// ListAssertions returns assertions, all projects when project is "".
func (s *Store) ListAssertions(ctx context.Context, project string) ([]evals.Assertion, error) {
	q := `SELECT id, project, name, type, config, enabled FROM assertions`
	var args []any
	if project != "" {
		q += ` WHERE project = ?`
		args = append(args, project)
	}
	rows, err := s.reader.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evals.Assertion
	for rows.Next() {
		var a evals.Assertion
		if err := rows.Scan(&a.ID, &a.Project, &a.Name, &a.Type, &a.Config, &a.Enabled); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAssertion removes an assertion and its results.
func (s *Store) DeleteAssertion(ctx context.Context, id int64) error {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM assertion_results WHERE assertion_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM assertions WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveAssertionResults upserts results for a run (idempotent by PK).
func (s *Store) SaveAssertionResults(ctx context.Context, runID string, results []evals.Result) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixNano()
	for _, r := range results {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR REPLACE INTO assertion_results (run_id, assertion_id, pass, detail, evaluated_ns)
			 VALUES (?,?,?,?,?)`, runID, r.AssertionID, r.Pass, r.Detail, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ResultsForRun returns a run's assertion results with assertion identity.
func (s *Store) ResultsForRun(ctx context.Context, runID string) ([]evals.Result, error) {
	rows, err := s.reader.QueryContext(ctx, `
		SELECT r.assertion_id, a.name, a.type, r.pass, r.detail
		FROM assertion_results r JOIN assertions a ON a.id = r.assertion_id
		WHERE r.run_id = ? ORDER BY r.assertion_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evals.Result
	for rows.Next() {
		var r evals.Result
		if err := rows.Scan(&r.AssertionID, &r.Name, &r.Type, &r.Pass, &r.Detail); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
