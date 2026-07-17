package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// ReadToken is a Bearer credential for the read API.
type ReadToken struct {
	Token     string    `json:"token"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateReadToken mints a named read token.
func (s *Store) CreateReadToken(ctx context.Context, name string) (ReadToken, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return ReadToken{}, err
	}
	t := ReadToken{Token: hex.EncodeToString(buf), Name: name, CreatedAt: time.Now()}
	_, err := s.writer.ExecContext(ctx,
		`INSERT INTO read_tokens (token, name, created_ns) VALUES (?,?,?)`,
		t.Token, t.Name, t.CreatedAt.UnixNano())
	if err != nil {
		return ReadToken{}, err
	}
	s.audit(ctx, "create", "token", name)
	return t, nil
}

// ListReadTokens returns all read tokens, oldest first.
func (s *Store) ListReadTokens(ctx context.Context) ([]ReadToken, error) {
	rows, err := s.reader.QueryContext(ctx,
		`SELECT token, name, created_ns FROM read_tokens ORDER BY created_ns`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReadToken
	for rows.Next() {
		var t ReadToken
		var ns int64
		if err := rows.Scan(&t.Token, &t.Name, &ns); err != nil {
			return nil, err
		}
		t.CreatedAt = time.Unix(0, ns)
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteReadToken revokes a read token.
func (s *Store) DeleteReadToken(ctx context.Context, token string) error {
	if _, err := s.writer.ExecContext(ctx, `DELETE FROM read_tokens WHERE token = ?`, token); err != nil {
		return err
	}
	s.audit(ctx, "delete", "token", shortToken(token))
	return nil
}

// ValidReadToken reports whether token is a live read token.
func (s *Store) ValidReadToken(ctx context.Context, token string) bool {
	if token == "" {
		return false
	}
	var n int
	err := s.reader.QueryRowContext(ctx,
		`SELECT count(*) FROM read_tokens WHERE token = ?`, token).Scan(&n)
	return err == nil && n > 0
}
