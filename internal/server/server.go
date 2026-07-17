// Package server runs the two HTTP listeners: the UI/API server and the
// OTLP/HTTP receiver. The receiver is a separate listener so the standard
// OTLP port (:4318) works untouched next to the UI.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/otterscope/otterscope/internal/alerts"
	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/ingest"
	"github.com/otterscope/otterscope/internal/mcp"
	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/store"
	"github.com/otterscope/otterscope/web"
)

// Server hosts the UI/API and the OTLP receiver.
type Server struct {
	st            *store.Store
	prices        *pricing.Table
	judge         evals.Endpoint
	eval          *ingest.Evaluator
	alertInterval time.Duration
	hub           *hub
	readAuth      bool
	ingestRate    float64
	ingestBurst   float64
	version       string
}

// New creates a Server backed by st, pricing LLM calls via prices, judging
// with the server-configured endpoint, and evaluating alerts every
// alertInterval (0 disables the watcher).
func New(st *store.Store, prices *pricing.Table, judge evals.Endpoint, alertInterval time.Duration, readAuth bool, ingestRate, ingestBurst float64, version string) *Server {
	return &Server{st: st, prices: prices, judge: judge, alertInterval: alertInterval, hub: newHub(), readAuth: readAuth, ingestRate: ingestRate, ingestBurst: ingestBurst, version: version}
}

// Run serves until ctx is canceled, then shuts both listeners down and
// drains the evaluator (before the caller closes the store).
func (s *Server) Run(ctx context.Context, uiAddr, otlpAddr string) error {
	s.eval = ingest.NewEvaluator(s.st, s.judge)
	s.eval.Start()
	defer s.eval.Stop() // drains queued evaluation before Run returns

	if s.alertInterval > 0 {
		watcher := alerts.NewWatcher(s.st, s.alertInterval)
		watcher.Start()
		defer watcher.Stop()
	}

	ui := &http.Server{Addr: uiAddr, Handler: s.authWrap(s.uiHandler())}
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
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": s.version})
	})
	mux.HandleFunc("GET /api/runs", s.handleListRuns)
	mux.HandleFunc("GET /api/runs.csv", s.handleRunsCSV)
	mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	mux.HandleFunc("POST /api/runs/{id}/share", s.handleCreateShare)
	mux.HandleFunc("GET /api/runs/{id}/shares", s.handleListShares)
	mux.HandleFunc("DELETE /api/shares/{token}", s.handleDeleteShare)
	mux.HandleFunc("GET /api/shared/{token}", s.handleSharedRun)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("GET /api/assertions", s.handleListAssertions)
	mux.HandleFunc("POST /api/assertions", s.handleCreateAssertion)
	mux.HandleFunc("DELETE /api/assertions/{id}", s.handleDeleteAssertion)
	mux.HandleFunc("POST /api/assertions/evaluate", s.handleEvaluate)
	mux.HandleFunc("GET /api/stream", s.handleStream)
	mux.HandleFunc("GET /api/tokens", s.handleListTokens)
	mux.HandleFunc("POST /api/tokens", s.handleCreateToken)
	mux.HandleFunc("DELETE /api/tokens/{token}", s.handleDeleteToken)
	mux.HandleFunc("GET /api/views", s.handleListViews)
	mux.HandleFunc("POST /api/views", s.handleCreateView)
	mux.HandleFunc("DELETE /api/views/{id}", s.handleDeleteView)
	mux.HandleFunc("GET /api/alerts", s.handleListAlerts)
	mux.HandleFunc("POST /api/alerts", s.handleCreateAlert)
	mux.HandleFunc("DELETE /api/alerts/{id}", s.handleDeleteAlert)
	mux.Handle("POST /mcp", mcp.NewHandler(s.st, s.version))
	mux.Handle("GET /", uiRoot())
	return mux
}

// uiRoot serves the embedded frontend with SPA fallback, or the placeholder
// page when the binary was built without `npm run build`.
func uiRoot() http.Handler {
	fsys, ok := web.Dist()
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(placeholderHTML))
		})
	}
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(fsys, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		// Unknown paths get index.html — client-side routing owns them.
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}

// authWrap enforces read-token auth on the API + MCP when -read-auth is set.
// The static UI, health check, and public share endpoint stay open; the UI's
// own API calls carry a token (see the frontend apiFetch wrapper).
func (s *Server) authWrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.readAuth || !authProtected(r.URL.Path) {
			h.ServeHTTP(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			// SSE (EventSource) can't set headers; accept a query token.
			token = r.URL.Query().Get("token")
		}
		if !s.st.ValidReadToken(r.Context(), token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "read token required"})
			return
		}
		h.ServeHTTP(w, r)
	})
}

// authProtected reports whether a path needs a read token under -read-auth.
func authProtected(path string) bool {
	if path == "/healthz" || path == "/metrics" || strings.HasPrefix(path, "/api/shared/") {
		return false // health, metrics, and public shares stay open
	}
	return strings.HasPrefix(path, "/api/") || path == "/mcp"
}

func (s *Server) otlpHandler() http.Handler {
	return ingest.NewHandler(ingest.NewStoreSink(s.st, s.prices, s.eval, s.hub.broadcast), s.st.ProjectForKey, s.ingestRate, s.ingestBurst)
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
