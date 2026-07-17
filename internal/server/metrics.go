package server

import (
	"fmt"
	"net/http"
)

// handleMetrics serves GET /metrics in Prometheus text exposition format.
// Hand-rolled (no client dependency); aggregate counts only, no run content.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.st.Metrics(r.Context())
	if err != nil {
		http.Error(w, "metrics unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	metric := func(name, typ, help string, value int64, labels string) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s%s %d\n", name, help, name, typ, name, labels, value)
	}

	fmt.Fprintf(w, "# HELP otterscope_build_info Build information.\n# TYPE otterscope_build_info gauge\notterscope_build_info{version=%q} 1\n", s.version)
	metric("otterscope_runs_total", "gauge", "Total runs stored.", m.Runs, "")
	metric("otterscope_runs", "gauge", "Runs by status.", m.RunsOK, `{status="ok"}`)
	metric("otterscope_runs", "gauge", "Runs by status.", m.RunsError, `{status="error"}`)
	metric("otterscope_runs", "gauge", "Runs by status.", m.RunsRunning, `{status="running"}`)
	metric("otterscope_steps_total", "gauge", "Total steps stored.", m.Steps, "")
	metric("otterscope_projects_total", "gauge", "Number of projects.", m.Projects, "")
	metric("otterscope_alerts_firing", "gauge", "Alerts currently firing.", m.AlertsFiring, "")
	metric("otterscope_db_size_bytes", "gauge", "SQLite database file size in bytes.", m.DBSizeBytes, "")
}
