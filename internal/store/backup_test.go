package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupProducesValidCopy(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := Open(ctx, filepath.Join(dir, "src.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSteps(ctx, sampleRun("r1", 1000)); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(dir, "backup.db")
	if err := st.Backup(ctx, dest); err != nil {
		t.Fatalf("Backup: %v", err)
	}
	st.Close()

	// The backup opens as a valid store with the data intact.
	restored, err := Open(ctx, dest)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer restored.Close()
	runs, err := restored.ListRuns(ctx, Filter{}, 10, 0)
	if err != nil || len(runs) != 1 || runs[0].ID != "r1" {
		t.Fatalf("backup missing data: %+v err=%v", runs, err)
	}
}

func TestAutoBackupBeforeMigrate(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "up.db")

	// Fully migrate, add data.
	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSteps(ctx, sampleRun("r1", 1000)); err != nil {
		t.Fatal(err)
	}
	// Simulate being one migration behind so the next Open sees a pending
	// migration on an existing (non-fresh) db. Re-applying a real migration
	// fails (its objects already exist) — that's fine: the point is that the
	// backup must be taken BEFORE the migration is attempted.
	if _, err := st.DB().ExecContext(ctx,
		`DELETE FROM schema_migrations WHERE name = (SELECT max(name) FROM schema_migrations)`); err != nil {
		t.Fatal(err)
	}
	st.Close()

	if n := len(backupFiles(t, path)); n != 0 {
		t.Fatalf("unexpected pre-existing backups: %d", n)
	}

	// Reopen: migrate backs up, then the (artificial) re-apply fails.
	_, err = Open(ctx, path)
	if err == nil {
		t.Fatal("expected the re-applied migration to fail in this setup")
	}

	// The backup must exist despite the failed migration, and hold the
	// pre-migration data. Query it with a raw connection so we don't
	// re-trigger migration on the snapshot.
	baks := backupFiles(t, path)
	if len(baks) != 1 {
		t.Fatalf("expected exactly one pre-migration backup, got %d", len(baks))
	}
	raw, err := sql.Open("sqlite", "file:"+baks[0])
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRowContext(ctx, `SELECT count(*) FROM runs`).Scan(&n); err != nil {
		t.Fatalf("query backup: %v", err)
	}
	if n != 1 {
		t.Fatalf("backup didn't capture pre-migration data: %d runs", n)
	}
}

func TestNoBackupOnFreshInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	st, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	// First init applies all migrations but must NOT create a .bak (nothing
	// to protect yet).
	if n := len(backupFiles(t, path)); n != 0 {
		t.Fatalf("fresh init created %d backups, want 0", n)
	}
}

func backupFiles(t *testing.T, dbPath string) []string {
	t.Helper()
	matches, err := filepath.Glob(dbPath + ".pre-migrate-*.bak")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range matches {
		if _, err := os.Stat(m); err == nil {
			out = append(out, m)
		}
	}
	return out
}
