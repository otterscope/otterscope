package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, "test"), st
}

func seedRun(t *testing.T, st *store.Store, id string, startSec int64) {
	t.Helper()
	base := time.Unix(startSec, 0)
	err := st.UpsertSteps(context.Background(), []model.Step{
		{
			ID: id + "-root", RunID: id, Kind: model.StepAgent,
			Name: "invoke_agent demo", Service: "demo-svc", AgentName: "demo",
			Status: model.StatusOK, Start: base, End: base.Add(9 * time.Second),
		},
		{
			ID: id + "-llm", RunID: id, ParentID: id + "-root", Kind: model.StepLLM,
			Name: "chat m1", Status: model.StatusOK,
			Start: base.Add(time.Second), End: base.Add(3 * time.Second),
			LLM: &model.LLMCall{RequestModel: "claude-sonnet-5", InputTokens: 100, OutputTokens: 20},
		},
	})
	if err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

func getRuns(t *testing.T, srv *Server, query string) []runJSON {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/runs"+query, nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Runs []runJSON `json:"runs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	return resp.Runs
}

func TestListRunsAPI(t *testing.T) {
	srv, st := testServer(t)
	seedRun(t, st, "r-old", 1000)
	seedRun(t, st, "r-new", 2000)

	runs := getRuns(t, srv, "")
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2", len(runs))
	}
	if runs[0].ID != "r-new" || runs[1].ID != "r-old" {
		t.Errorf("order: %s, %s", runs[0].ID, runs[1].ID)
	}
	r := runs[0]
	if r.Status != "ok" || r.Service != "demo-svc" || r.AgentName != "demo" {
		t.Errorf("identity: %+v", r)
	}
	if r.DurationMS != 9000 || r.InputTokens != 100 || r.OutputTokens != 20 {
		t.Errorf("metrics: %+v", r)
	}
	if r.Models != "claude-sonnet-5" || r.LLMCalls != 1 {
		t.Errorf("models: %+v", r)
	}
}

func TestListRunsPagingAndClamping(t *testing.T) {
	srv, st := testServer(t)
	for i := int64(0); i < 3; i++ {
		seedRun(t, st, "r"+string(rune('a'+i)), 1000+i*100)
	}

	if runs := getRuns(t, srv, "?limit=2"); len(runs) != 2 {
		t.Errorf("limit=2 returned %d", len(runs))
	}
	if runs := getRuns(t, srv, "?limit=2&offset=2"); len(runs) != 1 {
		t.Errorf("offset page returned %d", len(runs))
	}
	// Nonsense params fall back to sane values instead of erroring.
	if runs := getRuns(t, srv, "?limit=banana&offset=-5"); len(runs) != 3 {
		t.Errorf("clamped params returned %d", len(runs))
	}
}

func TestListRunsEmpty(t *testing.T) {
	srv, _ := testServer(t)
	if runs := getRuns(t, srv, ""); len(runs) != 0 {
		t.Errorf("expected empty list, got %d", len(runs))
	}
}
