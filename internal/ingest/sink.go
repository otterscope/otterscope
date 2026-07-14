package ingest

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/otterscope/otterscope/internal/store"
)

// StoreSink normalizes trace batches and persists them.
type StoreSink struct {
	st *store.Store
}

// NewStoreSink returns a Sink writing to st.
func NewStoreSink(st *store.Store) *StoreSink {
	return &StoreSink{st: st}
}

// ConsumeTraces implements Sink.
func (s *StoreSink) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	return s.st.UpsertSteps(ctx, Normalize(td))
}
