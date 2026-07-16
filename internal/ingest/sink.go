package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

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
}

// NewStoreSink returns a Sink writing to st, pricing calls via prices.
func NewStoreSink(st *store.Store, prices *pricing.Table) *StoreSink {
	return &StoreSink{st: st, prices: prices}
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

	seen := map[string]bool{}
	var runIDs []string
	for _, st := range steps {
		if !seen[st.RunID] {
			seen[st.RunID] = true
			runIDs = append(runIDs, st.RunID)
		}
	}
	// Detached: judge assertions make network calls that must never hold up
	// the exporter's OTLP request.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := EvaluateRuns(ctx, s.st, runIDs, false); err != nil {
			slog.Error("assertion evaluation failed", "err", err)
		}
	}()
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
