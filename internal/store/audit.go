package store

import (
	"context"
	"strconv"
	"time"
)

// AuditEntry is one recorded mutation.
type AuditEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Entity string    `json:"entity"`
	Detail string    `json:"detail"`
}

// audit records a mutation. Best-effort: an audit failure must never fail the
// operation it describes.
func (s *Store) audit(ctx context.Context, action, entity, detail string) {
	_, _ = s.writer.ExecContext(ctx,
		`INSERT INTO audit_log (at_ns, action, entity, detail) VALUES (?,?,?,?)`,
		time.Now().UnixNano(), action, entity, detail)
}

// ListAudit returns recent audit entries, newest first.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.reader.QueryContext(ctx,
		`SELECT at_ns, action, entity, detail FROM audit_log ORDER BY at_ns DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ns int64
		if err := rows.Scan(&ns, &e.Action, &e.Entity, &e.Detail); err != nil {
			return nil, err
		}
		e.At = time.Unix(0, ns)
		out = append(out, e)
	}
	return out, rows.Err()
}

// shortToken redacts a secret to a logged prefix.
func shortToken(t string) string {
	if len(t) > 8 {
		return t[:8] + "…"
	}
	return t
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
