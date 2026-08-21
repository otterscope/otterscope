package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/otterscope/otterscope/internal/store"
)

// do issues a request against the UI handler and returns the recorder.
func do(t *testing.T, srv *Server, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, r)
	return w
}

// A malformed filter bound must be an error, not a silently unfiltered
// result set (#99).
func TestMalformedTimeBoundIsRejected(t *testing.T) {
	srv, st := testServer(t)
	seedRun(t, st, "r-1", 1000)
	seedRun(t, st, "r-2", 2000)

	for _, path := range []string{"/api/runs", "/api/stats", "/api/runs.csv"} {
		for _, q := range []string{"?since=nonsense", "?until=2026-08-21", "?since=2026-13-01T00:00:00Z"} {
			w := do(t, srv, http.MethodGet, path+q, "")
			if w.Code != http.StatusBadRequest {
				t.Errorf("GET %s%s = %d, want 400 (body %s)", path, q, w.Code, strings.TrimSpace(w.Body.String()))
			}
		}
	}

	// A valid bound, and an absent one, still work.
	if runs := getRuns(t, srv, "?since=1970-01-01T00:16:40Z"); len(runs) != 2 {
		t.Errorf("valid since: got %d runs, want 2", len(runs))
	}
	if runs := getRuns(t, srv, "?since="); len(runs) != 2 {
		t.Errorf("empty since: got %d runs, want 2", len(runs))
	}
}

// Deleting something that is not there is a 404, not a silent success (#107).
func TestDeleteUnknownIDIs404(t *testing.T) {
	srv, _ := testServer(t)
	for _, target := range []string{
		"/api/views/99999",
		"/api/assertions/99999",
		"/api/alerts/99999",
		"/api/tokens/no-such-token",
		"/api/shares/no-such-token",
	} {
		if w := do(t, srv, http.MethodDelete, target, ""); w.Code != http.StatusNotFound {
			t.Errorf("DELETE %s = %d, want 404", target, w.Code)
		}
	}
}

// The happy path still answers 204 and actually removes the row (#107).
func TestDeleteExistingIs204(t *testing.T) {
	srv, _ := testServer(t)
	w := do(t, srv, http.MethodPost, "/api/views", `{"name":"mine","params":{}}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create view = %d (%s)", w.Code, w.Body.String())
	}
	var view store.SavedView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	target := "/api/views/" + itoa(view.ID)
	if w := do(t, srv, http.MethodDelete, target, ""); w.Code != http.StatusNoContent {
		t.Fatalf("first delete = %d, want 204", w.Code)
	}
	if w := do(t, srv, http.MethodDelete, target, ""); w.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", w.Code)
	}
}

// A duplicate name is a 409 that names the conflict; the raw store error is
// never echoed to the client (#100).
func TestDuplicateNameIsConflict(t *testing.T) {
	srv, _ := testServer(t)
	body := `{"name":"dupe","params":{}}`
	if w := do(t, srv, http.MethodPost, "/api/views", body); w.Code != http.StatusCreated {
		t.Fatalf("first create = %d", w.Code)
	}
	w := do(t, srv, http.MethodPost, "/api/views", body)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate create = %d, want 409", w.Code)
	}
	if got := w.Body.String(); !strings.Contains(got, "dupe") || strings.Contains(got, "UNIQUE constraint") {
		t.Errorf("conflict body should name the view, not leak SQL: %s", got)
	}
}

// An internal store failure is a 500, not a 409 mislabelled as a name clash
// (#100). Closing the store makes every write fail.
func TestStoreFailureIsInternalError(t *testing.T) {
	srv, st := testServer(t)
	st.Close()
	w := do(t, srv, http.MethodPost, "/api/views", `{"name":"x","params":{}}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("create with broken store = %d, want 500 (body %s)", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "already exists") {
		t.Errorf("internal failure reported as a name conflict: %s", w.Body.String())
	}
}

// A wrapped ErrNotFound must still produce a 404 — the handlers use
// errors.Is, so wrapping in the store cannot turn a 404 into a 500 (#102).
func TestWrappedNotFoundStillMaps(t *testing.T) {
	if !errors.Is(fmtWrap(store.ErrNotFound), store.ErrNotFound) {
		t.Fatal("errors.Is does not see through a wrap of store.ErrNotFound")
	}
	srv, _ := testServer(t)
	if w := do(t, srv, http.MethodGet, "/api/runs/no-such-run", ""); w.Code != http.StatusNotFound {
		t.Errorf("GET unknown run = %d, want 404", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/runs/no-such-run/shares", ""); w.Code != http.StatusNotFound {
		t.Errorf("GET shares of unknown run = %d, want 404", w.Code)
	}
}

func fmtWrap(err error) error { return errors.Join(errors.New("context"), err) }

// Hitting the export cap must be visible to whoever downloaded the file — a
// truncated CSV that looks complete makes every number derived from it wrong
// (#103).
func TestCSVTruncationIsReported(t *testing.T) {
	srv, st := testServer(t)
	for i, id := range []string{"c1", "c2", "c3"} {
		seedRun(t, st, id, int64(1000+i))
	}

	orig := csvExportLimit
	t.Cleanup(func() { csvExportLimit = orig })

	csvExportLimit = 2
	w := do(t, srv, http.MethodGet, "/api/runs.csv", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if w.Header().Get("X-Otterscope-Truncated") != "true" {
		t.Error("truncated export does not set X-Otterscope-Truncated")
	}
	if got := w.Header().Get("X-Otterscope-Row-Limit"); got != "2" {
		t.Errorf("row limit header = %q, want 2", got)
	}
	// Header row plus exactly the capped number of data rows.
	if lines := len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")); lines != 3 {
		t.Errorf("got %d lines, want 3 (header + 2 rows)", lines)
	}

	csvExportLimit = 10
	w = do(t, srv, http.MethodGet, "/api/runs.csv", "")
	if w.Header().Get("X-Otterscope-Truncated") != "" {
		t.Error("complete export claims to be truncated")
	}
	if lines := len(strings.Split(strings.TrimSpace(w.Body.String()), "\n")); lines != 4 {
		t.Errorf("got %d lines, want 4 (header + 3 rows)", lines)
	}
}
