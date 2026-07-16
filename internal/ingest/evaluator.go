package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/store"
)

// Evaluator runs assertion evaluation off the ingest path on a single
// background worker owned by the server lifecycle. This bounds concurrency
// (the previous per-batch goroutine fan-out was unbounded and outlived
// shutdown), and its queue applies backpressure instead of unbounded growth
// (audit #50).
type Evaluator struct {
	st    *store.Store
	judge evals.Endpoint
	ch    chan []string
	wg    sync.WaitGroup
}

// NewEvaluator creates an unstarted evaluator with a bounded queue.
func NewEvaluator(st *store.Store, judge evals.Endpoint) *Evaluator {
	return &Evaluator{st: st, judge: judge, ch: make(chan []string, 256)}
}

// Start launches the worker. Call Stop before closing the store.
func (e *Evaluator) Start() {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for runIDs := range e.ch {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			if err := EvaluateRuns(ctx, e.st, e.judge, runIDs, false); err != nil {
				slog.Error("assertion evaluation failed", "err", err)
			}
			cancel()
		}
	}()
}

// Enqueue schedules evaluation of the given runs. Non-blocking: if the queue
// is full it drops the batch (deterministic assertions can be recovered via
// the on-demand evaluate endpoint) rather than stalling ingest.
func (e *Evaluator) Enqueue(runIDs []string) {
	if len(runIDs) == 0 {
		return
	}
	select {
	case e.ch <- runIDs:
	default:
		slog.Warn("evaluator queue full; skipping assertion eval for a batch (use POST /api/assertions/evaluate to backfill)")
	}
}

// Stop drains the queue and waits for the worker to finish. Idempotent-safe
// to call once during shutdown, before the store is closed.
func (e *Evaluator) Stop() {
	close(e.ch)
	e.wg.Wait()
}
