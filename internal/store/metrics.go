package store

import (
	"context"
	"os"
)

// Metrics is a point-in-time snapshot of instance counts for /metrics.
type Metrics struct {
	Runs         int64
	RunsOK       int64
	RunsError    int64
	RunsRunning  int64
	Steps        int64
	Projects     int64
	AlertsFiring int64
	DBSizeBytes  int64
}

// Metrics gathers instance counts (queried on demand, so Prometheus can
// derive rates from successive scrapes).
func (s *Store) Metrics(ctx context.Context) (Metrics, error) {
	var m Metrics
	err := s.reader.QueryRowContext(ctx, `
		SELECT count(*),
		       coalesce(sum(status='ok'),0),
		       coalesce(sum(status='error'),0),
		       coalesce(sum(status='running'),0)
		FROM runs`).Scan(&m.Runs, &m.RunsOK, &m.RunsError, &m.RunsRunning)
	if err != nil {
		return Metrics{}, err
	}
	if err := s.reader.QueryRowContext(ctx, `SELECT count(*) FROM steps`).Scan(&m.Steps); err != nil {
		return Metrics{}, err
	}
	if err := s.reader.QueryRowContext(ctx, `SELECT count(*) FROM projects`).Scan(&m.Projects); err != nil {
		return Metrics{}, err
	}
	if err := s.reader.QueryRowContext(ctx, `SELECT coalesce(sum(firing),0) FROM alerts`).Scan(&m.AlertsFiring); err != nil {
		return Metrics{}, err
	}
	if s.path != "" {
		if fi, err := os.Stat(s.path); err == nil {
			m.DBSizeBytes = fi.Size()
		}
	}
	return m, nil
}
