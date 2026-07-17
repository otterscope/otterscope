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
}

// CreateAlert stores a validated alert rule.
func (s *Store) CreateAlert(ctx context.Context, r Rule) (Rule, error) {
	if r.Project == "" {
		r.Project = "default"
	}
	res, err := s.writer.ExecContext(ctx, `
		INSERT INTO alerts (project, name, type, threshold, window_secs, config, webhook_url, enabled, created_ns)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		r.Project, r.Name, r.Type, r.Threshold, r.WindowSecs, r.Config, r.WebhookURL, r.Enabled, time.Now().UnixNano())
	if err != nil {
		return Rule{}, err
	}
	r.ID, _ = res.LastInsertId()
	s.audit(ctx, "create", "alert", r.Name)
	return r, nil
}

// ListAlerts returns alerts, all projects when project is "".
func (s *Store) ListAlerts(ctx context.Context, project string) ([]Rule, error) {
	q := `SELECT id, project, name, type, threshold, window_secs, config, webhook_url, enabled, firing FROM alerts`
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
		`SELECT id, project, name, type, threshold, window_secs, config, webhook_url, enabled, firing
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
			&r.WindowSecs, &r.Config, &r.WebhookURL, &r.Enabled, &r.Firing); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetAlertFiring updates an alert's firing state; sets last_fired_ns when
// transitioning into firing.
func (s *Store) SetAlertFiring(ctx context.Context, id int64, firing bool) error {
	if firing {
		_, err := s.writer.ExecContext(ctx,
			`UPDATE alerts SET firing = 1, last_fired_ns = ? WHERE id = ?`, time.Now().UnixNano(), id)
		return err
	}
	_, err := s.writer.ExecContext(ctx, `UPDATE alerts SET firing = 0 WHERE id = ?`, id)
	return err
}

// DeleteAlert removes an alert.
func (s *Store) DeleteAlert(ctx context.Context, id int64) error {
	if _, err := s.writer.ExecContext(ctx, `DELETE FROM alerts WHERE id = ?`, id); err != nil {
		return err
	}
	s.audit(ctx, "delete", "alert", itoa64(id))
	return nil
}
