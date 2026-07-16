package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Project is an isolation dimension: each has its own ingest key.
type Project struct {
	Name      string
	IngestKey string
	CreatedAt time.Time
}

// CreateProject adds a project with a fresh random ingest key and returns it.
func (s *Store) CreateProject(ctx context.Context, name string) (Project, error) {
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return Project{}, err
	}
	p := Project{Name: name, IngestKey: hex.EncodeToString(keyBytes), CreatedAt: time.Now()}
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO projects (name, ingest_key, created_ns) VALUES (?,?,?)`,
		p.Name, p.IngestKey, p.CreatedAt.UnixNano())
	if err != nil {
		return Project{}, fmt.Errorf("create project %q: %w", name, err)
	}
	return p, nil
}

// ListProjects returns all projects, oldest first.
func (s *Store) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT name, ingest_key, created_ns FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Project
	for rows.Next() {
		var p Project
		var ns int64
		if err := rows.Scan(&p.Name, &p.IngestKey, &ns); err != nil {
			return nil, err
		}
		p.CreatedAt = time.Unix(0, ns)
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProjectForKey resolves an ingest key to its project name. ok=false means
// the key is unknown. The empty key always resolves to "default".
func (s *Store) ProjectForKey(ctx context.Context, key string) (string, bool) {
	if key == "" {
		return "default", true
	}
	var name string
	err := s.reader.QueryRowContext(ctx,
		`SELECT name FROM projects WHERE ingest_key = ?`, key).Scan(&name)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		return "", false
	}
	return name, true
}

// Sweep deletes runs (with their steps) started before cutoff and raw
// batches received before cutoff. Returns the number of runs removed.
func (s *Store) Sweep(ctx context.Context, cutoff time.Time) (int64, error) {
	tx, err := s.writer.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	ns := cutoff.UnixNano()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM steps WHERE run_id IN (SELECT id FROM runs WHERE start_ns < ?)`, ns); err != nil {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE start_ns < ?`, ns)
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM raw_batches WHERE received_ns < ?`, ns); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, tx.Commit()
}
