package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Share is a public read-only link to one run.
type Share struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateShare mints an unguessable token for (project, runID).
func (s *Store) CreateShare(ctx context.Context, project, runID string) (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := hex.EncodeToString(buf)
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO shared_runs (token, project, run_id, created_ns) VALUES (?,?,?,?)`,
		token, project, runID, time.Now().UnixNano())
	if err != nil {
		return "", err
	}
	return token, nil
}

// ResolveShare maps a token to its (project, runID). ok=false if unknown.
func (s *Store) ResolveShare(ctx context.Context, token string) (project, runID string, ok bool) {
	err := s.reader.QueryRowContext(ctx,
		`SELECT project, run_id FROM shared_runs WHERE token = ?`, token).Scan(&project, &runID)
	if err != nil {
		return "", "", false
	}
	return project, runID, true
}

// SharesForRun lists active shares for a run.
func (s *Store) SharesForRun(ctx context.Context, project, runID string) ([]Share, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT token, created_ns FROM shared_runs WHERE project = ? AND run_id = ? ORDER BY created_ns`,
		project, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Share
	for rows.Next() {
		var sh Share
		var ns int64
		if err := rows.Scan(&sh.Token, &ns); err != nil {
			return nil, err
		}
		sh.CreatedAt = time.Unix(0, ns)
		out = append(out, sh)
	}
	return out, rows.Err()
}

// DeleteShare revokes a share token.
func (s *Store) DeleteShare(ctx context.Context, token string) error {
	_, err := s.writer.ExecContext(ctx, `DELETE FROM shared_runs WHERE token = ?`, token)
	return err
}
