package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	var n int
	if err := st.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if n == 0 {
		t.Fatal("no migrations applied")
	}

	var created string
	if err := st.DB().QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'created_at'`).Scan(&created); err != nil {
		t.Fatalf("meta table missing created_at: %v", err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	st1, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	st1.Close()

	st2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("second Open (re-running migrations): %v", err)
	}
	st2.Close()
}
