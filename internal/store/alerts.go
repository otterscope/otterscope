package store

import (
	"context"
	"time"
)

// Rule is one alert definition. It lives in the store (like Filter/Stats)
// because the alert watcher evaluates it against store aggregates; the
// alerts package owns the behavior (validate, evaluate, notify).
type Rule struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Name    string `json:"name"`
	// Type: error_rate | cost | p95_latency | assertion_fail_rate
	Type       string  `json:"type"`
	Threshold  float64 `json:"threshold"`
	WindowSecs int64   `json:"windowSecs"`
	Config     string  `json:"config"` // assertion name for assertion_fail_rate
	WebhookURL string  `json:"webhookUrl"`
	Enabled    bool    `json:"enabled"`
	Firing     bool    `json:"firing"`
	// SettleSecs is how long the condition must hold its new state before
	// the watcher notifies, in both directions. 0 notifies immediately.
	SettleSecs int64 `json:"settleSecs"`
	// PendingSinceNS is when the condition first flipped away from Firing,
	// or 0 when nothing is pending. Read-only to API clients; the watcher
	// owns it.
	PendingSinceNS int64 `json:"pendingSinceNs,omitempty"`
}


// CreateAlert stores a validated alert rule.
func (s *Store) CreateAlert(ctx context.Context, r Rule) (Rule, error) {
	if r.Project == "" {
		r.Project = "default"
	}
	res, err := s.writer.ExecContext(ctx, `
		INSERT INTO alerts (project, name, type, threshold, window_secs, config, webhook_url, enabled, settle_secs, created_ns)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		r.Project, r.Name, r.Type, r.Threshold, r.WindowSecs, r.Config, r.WebhookURL, r.Enabled, r.SettleSecs, time.Now().UnixNano())
	if err != nil {
		return Rule{}, classifyWrite(err)
	}
	r.ID, _ = res.LastInsertId()
	s.audit(ctx, "create", "alert", r.Name)
	return r, nil
}

// ListAlerts returns alerts, all projects when project is "".
func (s *Store) ListAlerts(ctx context.Context, project string) ([]Rule, error) {
	q := `SELECT id, project, name, type, threshold, window_secs, config, webhook_url,
	             enabled, firing, settle_secs, pending_since_ns FROM alerts`
	var args []any
	if project != "" {
		q += ` WHERE project = ?`
		args = append(args, project)
	}
	return s.queryAlerts(ctx, q+` ORDER BY id`, args...)
}

// ListEnabledAlerts returns every enabled alert across projects (watcher).
func (s *Store) ListEnabledAlerts(ctx context.Context) ([]Rule, error) {
	return s.queryAlerts(ctx,
		`SELECT id, project, name, type, threshold, window_secs, config, webhook_url,
		        enabled, firing, settle_secs, pending_since_ns
		 FROM alerts WHERE enabled = 1 ORDER BY id`)
}

func (s *Store) queryAlerts(ctx context.Context, q string, args ...any) ([]Rule, error) {
	rows, err := s.reader.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.Project, &r.Name, &r.Type, &r.Threshold,
			&r.WindowSecs, &r.Config, &r.WebhookURL, &r.Enabled, &r.Firing,
			&r.SettleSecs, &r.PendingSinceNS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetAlertFiring updates an alert's firing state; sets last_fired_ns when
// transitioning into firing. Either way the transition is no longer pending,
// so the settle timer is cleared.
func (s *Store) SetAlertFiring(ctx context.Context, id int64, firing bool) error {
	if firing {
		_, err := s.writer.ExecContext(ctx,
			`UPDATE alerts SET firing = 1, last_fired_ns = ?, pending_since_ns = 0 WHERE id = ?`,
			time.Now().UnixNano(), id)
		return err
	}
	_, err := s.writer.ExecContext(ctx,
		`UPDATE alerts SET firing = 0, pending_since_ns = 0 WHERE id = ?`, id)
	return err
}

// SetAlertPending records when a rule's condition flipped away from the state
// we last notified about, starting its settle window. ns = 0 clears it,
// which is what happens when the condition flaps back before settling.
func (s *Store) SetAlertPending(ctx context.Context, id int64, ns int64) error {
	_, err := s.writer.ExecContext(ctx,
		`UPDATE alerts SET pending_since_ns = ? WHERE id = ?`, ns, id)
	return err
}

// DeleteAlert removes an alert.
func (s *Store) DeleteAlert(ctx context.Context, id int64) error {
	res, err := s.writer.ExecContext(ctx, `DELETE FROM alerts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if err := deleted(res); err != nil {
		return err
	}
	s.audit(ctx, "delete", "alert", itoa64(id))
	return nil
}
