// Package store owns SQLite persistence. All schema changes go through
// embedded, append-only migrations in migrations/.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store wraps the SQLite database with two handles: a single-connection
// writer (SQLite serializes writes anyway; one connection avoids
// SQLITE_BUSY churn) and a small reader pool (WAL makes concurrent reads
// safe alongside the writer). Reads must go through the reader so a held
// rows cursor can never starve a write on the same call path (#22).
type Store struct {
	writer *sql.DB
	reader *sql.DB
	path   string // db file path, for backups
}

// Open opens (creating if needed) the database at path and applies pending
// migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	// WAL for concurrent reads during ingest; busy_timeout so the UI and
	// ingest paths don't fail on transient write contention.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	writer, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)

	reader, err := sql.Open("sqlite", dsn)
	if err != nil {
		writer.Close()
		return nil, err
	}
	reader.SetMaxOpenConns(4)

	s := &Store{writer: writer, reader: reader, path: path}
	if err := s.migrate(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Backup writes a consistent snapshot of the database to dest using SQLite's
// VACUUM INTO (includes committed WAL). dest must not already exist.
func (s *Store) Backup(ctx context.Context, dest string) error {
	if _, err := s.writer.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return fmt.Errorf("backup to %s: %w", dest, err)
	}
	return nil
}

// Close closes both database handles.
func (s *Store) Close() error {
	rerr := s.reader.Close()
	if werr := s.writer.Close(); werr != nil {
		return werr
	}
	return rerr
}

// DB exposes the raw writer handle for package-internal tests. It must not
// be used outside internal/.
func (s *Store) DB() *sql.DB { return s.writer }

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.writer.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		return err
	}

	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	// Count already-applied migrations: >0 means this is an existing db being
	// upgraded (not first init), so back it up before touching it.
	var applied int
	if err := s.writer.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		return err
	}
	pending := 0
	for _, name := range names {
		var done int
		if err := s.writer.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&done); err != nil {
			return err
		}
		if done == 0 {
			pending++
		}
	}
	if pending > 0 && applied > 0 {
		if err := s.backupBeforeMigrate(ctx); err != nil {
			// A failed pre-migration backup must not silently proceed to a
			// possibly-destructive migration.
			return fmt.Errorf("pre-migration backup failed (refusing to migrate): %w", err)
		}
	}

	for _, name := range names {
		var done int
		if err := s.writer.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, name).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrationFS.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.writer.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// backupBeforeMigrate snapshots the db next to itself before an upgrade, so a
// bad migration on real data is recoverable. Skipped for in-memory dbs.
func (s *Store) backupBeforeMigrate(ctx context.Context) error {
	if s.path == "" || strings.Contains(s.path, ":memory:") {
		return nil
	}
	dest := fmt.Sprintf("%s.pre-migrate-%d.bak", s.path, time.Now().Unix())
	if err := s.Backup(ctx, dest); err != nil {
		return err
	}
	slog.Info("backed up database before applying migrations", "backup", dest)
	return nil
}
