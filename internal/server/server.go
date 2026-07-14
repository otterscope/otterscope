// Package server runs the two HTTP listeners: the UI/API server and the
// OTLP/HTTP receiver. The receiver is a separate listener so the standard
// OTLP port (:4318) works untouched next to the UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/DimitrovK/otterscope/internal/store"
)

// Server hosts the UI/API and the OTLP receiver.
type Server struct {
	st      *store.Store
	version string
}

// New creates a Server backed by st.
func New(st *store.Store, version string) *Server {
	return &Server{st: st, version: version}
}

// Run serves until ctx is canceled, then shuts both listeners down.
func (s *Server) Run(ctx context.Context, uiAddr, otlpAddr string) error {
	ui := &http.Server{Addr: uiAddr, Handler: s.uiHandler()}
	otlp := &http.Server{Addr: otlpAddr, Handler: s.otlpHandler()}

	errc := make(chan error, 2)
	go func() { errc <- ui.ListenAndServe() }()
	go func() { errc <- otlp.ListenAndServe() }()
	slog.Info("listening", "ui", uiAddr, "otlp", otlpAddr)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ui.Shutdown(shutdownCtx)
		otlp.Shutdown(shutdownCtx)
		return nil
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) uiHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(placeholderHTML))
	})
	return mux
}

func (s *Server) otlpHandler() http.Handler {
	mux := http.NewServeMux()
	// M1 replaces this stub with the real OTLP trace receiver.
	mux.HandleFunc("POST /v1/traces", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "ingest not implemented yet (M1)"})
	})
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

const placeholderHTML = `<!doctype html>
<meta charset="utf-8">
<title>Otterscope</title>
<style>body{font-family:system-ui;display:grid;place-items:center;min-height:100vh;margin:0;background:#0f1419;color:#e6e1cf}p{color:#8a919c}</style>
<div style="text-align:center">
  <h1>🦦 Otterscope</h1>
  <p>Lightweight, self-hosted observability for AI agents.<br>UI arrives in milestone M2 — the OTLP receiver lands in M1.</p>
</div>`
