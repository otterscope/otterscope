package store

import (
	"context"
	"testing"
	"time"
)

// A held reader cursor must never block a write — the exact deadlock class
// from PR #21, now structurally prevented by the reader/writer split.
func TestWriteProceedsWhileReaderCursorHeld(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.UpsertSteps(ctx, sampleRun("r1", 1000)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rows, err := st.reader.QueryContext(ctx, `SELECT id FROM steps ORDER BY id`)
	if err != nil {
		t.Fatalf("open reader cursor: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected at least one row")
	}

	// With the cursor still open, a write must complete promptly.
	done := make(chan error, 1)
	go func() { done <- st.UpsertSteps(ctx, sampleRun("r2", 2000)) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write while cursor held: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("write blocked behind an open reader cursor")
	}

	// The held cursor still works afterwards.
	n := 1
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("cursor after concurrent write: %v", err)
	}
	if n != 3 {
		t.Fatalf("cursor saw %d rows, want 3 (snapshot from before the write)", n)
	}
}
