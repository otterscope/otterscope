package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// UpsertSteps writes a batch of steps and re-derives the aggregates of every
// touched run, all in one transaction. Steps are keyed by span ID, so
// re-delivered OTLP batches are naturally idempotent.
func (s *Store) UpsertSteps(ctx context.Context, steps []model.Step) error {
	if len(steps) == 0 {
		return nil
	}
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertStepsTx(ctx, tx, steps); err != nil {
		return err
	}
	return tx.Commit()
}

// upsertStepsTx is the transactional body of UpsertSteps, shared with
// IngestBatch.
func upsertStepsTx(ctx context.Context, tx *sql.Tx, steps []model.Step) error {
	if len(steps) == 0 {
		return nil
	}
	ins, err := tx.PrepareContext(ctx, `
		INSERT OR REPLACE INTO steps
		(id, run_id, parent_id, kind, name, project, service, agent_name, status,
		 start_ns, end_ns, error,
		 provider, request_model, response_model, input_tokens, output_tokens,
		 cache_read_tokens, cache_creation_tokens, reasoning_tokens,
		 tool_name, tool_call_id, detail, cost_usd)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()

	// Runs are keyed by (project, run_id) — re-derive each touched pair.
	type runKey struct{ project, runID string }
	runKeys := make(map[runKey]bool)
	for _, st := range steps {
		if st.Project == "" {
			st.Project = "default"
		}
		var llm model.LLMCall
		if st.LLM != nil {
			llm = *st.LLM
		}
		var tool model.ToolCall
		if st.Tool != nil {
			tool = *st.Tool
		}
		if _, err := ins.ExecContext(ctx,
			st.ID, st.RunID, st.ParentID, string(st.Kind), st.Name, st.Project, st.Service, st.AgentName, string(st.Status),
			st.Start.UnixNano(), st.End.UnixNano(), st.Error,
			llm.Provider, llm.RequestModel, llm.ResponseModel, llm.InputTokens, llm.OutputTokens,
			llm.CacheReadTokens, llm.CacheCreationTokens, llm.ReasoningTokens,
			tool.Name, tool.CallID, marshalDetail(st), nullableFloat(llm.CostUSD),
		); err != nil {
			return fmt.Errorf("insert step %s: %w", st.ID, err)
		}
		runKeys[runKey{st.Project, st.RunID}] = true
	}

	for k := range runKeys {
		if err := rederiveRun(ctx, tx, k.project, k.runID); err != nil {
			return fmt.Errorf("rederive run %s/%s: %w", k.project, k.runID, err)
		}
	}
	return nil
}

// rederiveRun recomputes a run row entirely from its steps within one
// project, making run aggregates structurally idempotent instead of
// bookkept. Scoping by project prevents one project's steps from being
// merged into another's run (audit #49).
func rederiveRun(ctx context.Context, tx *sql.Tx, project, runID string) error {
	var (
		startNS, endNS             int64
		inTok, outTok              int64
		llmCalls, toolCalls        int64
		hasError, hasRoot, partial bool
		service, agent, oerr, models string
		cost                       sql.NullFloat64
	)
	err := tx.QueryRowContext(ctx, `
		SELECT min(start_ns), max(end_ns),
		       sum(input_tokens), sum(output_tokens),
		       sum(kind = 'llm'), sum(kind = 'tool'),
		       max(error != ''), max(parent_id = ''),
		       coalesce((SELECT service FROM steps WHERE project = ?1 AND run_id = ?2 AND service != '' LIMIT 1), ''),
		       coalesce((SELECT agent_name FROM steps WHERE project = ?1 AND run_id = ?2 AND agent_name != '' LIMIT 1), ''),
		       coalesce((SELECT error FROM steps WHERE project = ?1 AND run_id = ?2 AND error != '' ORDER BY start_ns LIMIT 1), ''),
		       coalesce((SELECT group_concat(DISTINCT request_model) FROM steps WHERE project = ?1 AND run_id = ?2 AND request_model != ''), ''),
		       sum(cost_usd),
		       max(kind = 'llm' AND cost_usd IS NULL AND (input_tokens > 0 OR output_tokens > 0))
		FROM steps WHERE project = ?1 AND run_id = ?2`, project, runID).
		Scan(&startNS, &endNS, &inTok, &outTok, &llmCalls, &toolCalls, &hasError, &hasRoot, &service, &agent, &oerr, &models, &cost, &partial)
	if err != nil {
		return err
	}

	status := model.StatusRunning
	switch {
	case hasError:
		status = model.StatusError
	case hasRoot:
		status = model.StatusOK
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO runs (project, id, service, agent_name, status, start_ns, end_ns,
		                  input_tokens, output_tokens, llm_calls, tool_calls, models,
		                  cost_usd, cost_partial, error)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(project, id) DO UPDATE SET
		  service=excluded.service, agent_name=excluded.agent_name,
		  status=excluded.status, start_ns=excluded.start_ns, end_ns=excluded.end_ns,
		  input_tokens=excluded.input_tokens, output_tokens=excluded.output_tokens,
		  llm_calls=excluded.llm_calls, tool_calls=excluded.tool_calls,
		  models=excluded.models, cost_usd=excluded.cost_usd,
		  cost_partial=excluded.cost_partial, error=excluded.error`,
		project, runID, service, agent, string(status), startNS, endNS,
		inTok, outTok, llmCalls, toolCalls, models, cost, partial, oerr)
	return err
}

// nullableFloat converts *float64 to a driver-friendly NULL-able value.
func nullableFloat(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

func floatPtr(f sql.NullFloat64) *float64 {
	if !f.Valid {
		return nil
	}
	return &f.Float64
}

// Filter narrows ListRuns. Zero values mean "no constraint".
type Filter struct {
	Project string    // exact match
	Status  string    // exact match: running | ok | error
	Service string    // prefix match (index-friendly)
	Model   string    // substring match against the run's models list
	Since   time.Time // start >= Since
	Until   time.Time // start <= Until
}

// ListRuns returns runs newest-first. offset-based paging is fine at target
// scale; revisit with keyset paging if it ever shows up in profiles.
func (s *Store) ListRuns(ctx context.Context, f Filter, limit, offset int) ([]model.Run, error) {
	where, args := filterWhere(f)
	args = append(args, limit, offset)

	rows, err := s.reader.QueryContext(ctx, `
		SELECT id, project, service, agent_name, status, start_ns, end_ns,
		       input_tokens, output_tokens, llm_calls, tool_calls, models,
		       cost_usd, cost_partial, error
		FROM runs`+where+` ORDER BY start_ns DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []model.Run
	for rows.Next() {
		var r model.Run
		var status string
		var startNS, endNS int64
		var cost sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.Project, &r.Service, &r.AgentName, &status, &startNS, &endNS,
			&r.InputTokens, &r.OutputTokens, &r.LLMCalls, &r.ToolCalls, &r.Models,
			&cost, &r.CostPartial, &r.Error); err != nil {
			return nil, err
		}
		r.CostUSD = floatPtr(cost)
		r.Status = model.Status(status)
		r.Start = time.Unix(0, startNS)
		r.End = time.Unix(0, endNS)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// escapeLike neutralizes LIKE wildcards in user input.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ErrNotFound is returned by GetRun for unknown run IDs.
var ErrNotFound = sql.ErrNoRows

// GetRun returns a run and its steps ordered by start time.
func (s *Store) GetRun(ctx context.Context, id string) (model.Run, []model.Step, error) {
	var r model.Run
	var status string
	var startNS, endNS int64
	var cost sql.NullFloat64
	// Trace ID can now exist in more than one project (forging can't
	// overwrite, only coexist); newest wins for the bare-ID lookup.
	err := s.reader.QueryRowContext(ctx, `
		SELECT id, project, service, agent_name, status, start_ns, end_ns,
		       input_tokens, output_tokens, llm_calls, tool_calls, models,
		       cost_usd, cost_partial, error
		FROM runs WHERE id = ? ORDER BY start_ns DESC LIMIT 1`, id).
		Scan(&r.ID, &r.Project, &r.Service, &r.AgentName, &status, &startNS, &endNS,
			&r.InputTokens, &r.OutputTokens, &r.LLMCalls, &r.ToolCalls, &r.Models,
			&cost, &r.CostPartial, &r.Error)
	if err != nil {
		return model.Run{}, nil, err
	}
	r.CostUSD = floatPtr(cost)
	r.Status = model.Status(status)
	r.Start = time.Unix(0, startNS)
	r.End = time.Unix(0, endNS)

	rows, err := s.reader.QueryContext(ctx, `
		SELECT id, run_id, parent_id, kind, name, service, agent_name, status,
		       start_ns, end_ns, error,
		       provider, request_model, response_model, input_tokens, output_tokens,
		       cache_read_tokens, cache_creation_tokens, reasoning_tokens,
		       tool_name, tool_call_id, detail, cost_usd
		FROM steps WHERE project = ? AND run_id = ? ORDER BY start_ns`, r.Project, id)
	if err != nil {
		return model.Run{}, nil, err
	}
	defer rows.Close()

	var steps []model.Step
	for rows.Next() {
		var st model.Step
		var kind, stStatus string
		var sNS, eNS int64
		var llm model.LLMCall
		var tool model.ToolCall
		var detail string
		var stepCost sql.NullFloat64
		if err := rows.Scan(&st.ID, &st.RunID, &st.ParentID, &kind, &st.Name, &st.Service, &st.AgentName, &stStatus,
			&sNS, &eNS, &st.Error,
			&llm.Provider, &llm.RequestModel, &llm.ResponseModel, &llm.InputTokens, &llm.OutputTokens,
			&llm.CacheReadTokens, &llm.CacheCreationTokens, &llm.ReasoningTokens,
			&tool.Name, &tool.CallID, &detail, &stepCost); err != nil {
			return model.Run{}, nil, err
		}
		llm.CostUSD = floatPtr(stepCost)
		st.Kind = model.StepKind(kind)
		st.Status = model.Status(stStatus)
		st.Start = time.Unix(0, sNS)
		st.End = time.Unix(0, eNS)
		if st.Kind == model.StepLLM {
			st.LLM = &llm
		}
		if st.Kind == model.StepTool {
			st.Tool = &tool
		}
		applyDetail(&st, detail)
		steps = append(steps, st)
	}
	return r, steps, rows.Err()
}
