package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"

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
func (s *StoreSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	raw, err := compressTraces(td)
	if err != nil {
		return fmt.Errorf("compress raw batch: %w", err)
	}
	steps := Normalize(td)
	priceSteps(steps, s.prices)
	return s.st.IngestBatch(ctx, raw, steps)
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
// and pricing table. UpsertSteps is idempotent, so re-running it is always
// safe; run after a normalizer or pricing improvement to backfill.
func Renormalize(ctx context.Context, st *store.Store, prices *pricing.Table) error {
	return st.EachRawBatch(ctx, func(payload []byte) error {
		td, err := decompressTraces(payload)
		if err != nil {
			return fmt.Errorf("decode raw batch: %w", err)
		}
		steps := Normalize(td)
		priceSteps(steps, prices)
		return st.UpsertSteps(ctx, steps)
	})
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
