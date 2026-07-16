package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
	"io"

	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/store"
)

// StoreSink normalizes trace batches, prices LLM calls, and persists them
// together with the raw payload, so batches can be re-normalized later
// (ADR-0002).
type StoreSink struct {
	st     *store.Store
	prices *pricing.Table
	eval   *Evaluator
	notify func() // called after a batch persists (may be nil)
}

// NewStoreSink returns a Sink writing to st, pricing calls via prices,
// scheduling assertion evaluation on eval (may be nil), and calling notify
// after each batch persists (may be nil) — used to push live updates.
func NewStoreSink(st *store.Store, prices *pricing.Table, eval *Evaluator, notify func()) *StoreSink {
	return &StoreSink{st: st, prices: prices, eval: eval, notify: notify}
}

// ConsumeTraces implements Sink.
func (s *StoreSink) ConsumeTraces(ctx context.Context, project string, td ptrace.Traces) error {
	raw, err := compressTraces(td)
	if err != nil {
		return fmt.Errorf("compress raw batch: %w", err)
	}
	steps := Normalize(td)
	priceSteps(steps, s.prices)
	for i := range steps {
		steps[i].Project = project
	}
	if err := s.st.IngestBatch(ctx, project, raw, steps); err != nil {
		return err
	}

	if s.notify != nil {
		s.notify()
	}
	if s.eval != nil {
		seen := map[string]bool{}
		var runIDs []string
		for _, st := range steps {
			if !seen[st.RunID] {
				seen[st.RunID] = true
				runIDs = append(runIDs, st.RunID)
			}
		}
		s.eval.Enqueue(runIDs)
	}
	return nil
}

// priceSteps stamps CostUSD on llm steps with a known model. Unknown models
// stay nil — never a fabricated cost.
func priceSteps(steps []model.Step, prices *pricing.Table) {
	for i := range steps {
		llm := steps[i].LLM
		if llm == nil || (llm.InputTokens == 0 && llm.OutputTokens == 0) {
			continue
		}
		m := llm.ResponseModel // response model is what was actually billed
		if m == "" {
			m = llm.RequestModel
		}
		if m == "" {
			continue
		}
		if usd, ok := prices.Cost(m, llm.InputTokens, llm.OutputTokens, llm.CacheReadTokens, llm.CacheCreationTokens); ok {
			llm.CostUSD = &usd
		}
	}
}

// Renormalize replays every stored raw batch through the current normalizer
// and pricing table, returning the number of batches replayed. UpsertSteps
// is idempotent, so re-running it is always safe; run after a normalizer or
// pricing improvement to backfill.
func Renormalize(ctx context.Context, st *store.Store, prices *pricing.Table) (int, error) {
	n := 0
	err := st.EachRawBatch(ctx, func(project string, payload []byte) error {
		td, err := decompressTraces(payload)
		if err != nil {
			return fmt.Errorf("decode raw batch: %w", err)
		}
		steps := Normalize(td)
		priceSteps(steps, prices)
		for i := range steps {
			steps[i].Project = project
		}
		if err := st.UpsertSteps(ctx, steps); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

func compressTraces(td ptrace.Traces) ([]byte, error) {
	req := ptraceotlp.NewExportRequestFromTraces(td)
	pb, err := req.MarshalProto()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(pb); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressTraces(payload []byte) (ptrace.Traces, error) {
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return ptrace.Traces{}, err
	}
	defer gz.Close()
	pb, err := io.ReadAll(gz)
	if err != nil {
		return ptrace.Traces{}, err
	}
	req := ptraceotlp.NewExportRequest()
	if err := req.UnmarshalProto(pb); err != nil {
		return ptrace.Traces{}, err
	}
	return req.Traces(), nil
}
