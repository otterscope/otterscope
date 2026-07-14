package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"

	"github.com/otterscope/otterscope/internal/store"
)

// StoreSink normalizes trace batches and persists them together with the
// raw payload, so batches can be re-normalized later (ADR-0002).
type StoreSink struct {
	st *store.Store
}

// NewStoreSink returns a Sink writing to st.
func NewStoreSink(st *store.Store) *StoreSink {
	return &StoreSink{st: st}
}

// ConsumeTraces implements Sink.
func (s *StoreSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	raw, err := compressTraces(td)
	if err != nil {
		return fmt.Errorf("compress raw batch: %w", err)
	}
	return s.st.IngestBatch(ctx, raw, Normalize(td))
}

// Renormalize replays every stored raw batch through the current normalizer.
// UpsertSteps is idempotent, so re-running it is always safe; run after a
// normalizer improvement to backfill existing data.
func Renormalize(ctx context.Context, st *store.Store) error {
	return st.EachRawBatch(ctx, func(payload []byte) error {
		td, err := decompressTraces(payload)
		if err != nil {
			return fmt.Errorf("decode raw batch: %w", err)
		}
		return st.UpsertSteps(ctx, Normalize(td))
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
