package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/store"
)

func testServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st, pricing.Default(), evals.Endpoint{}, 0, "test"), st
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

func TestGetRunAPI(t *testing.T) {
	srv, st := testServer(t)
	seedRun(t, st, "r1", 1000)
	// Attach messages + tool detail through the real write path.
	base := time.Unix(1000, 0)
	err := st.UpsertSteps(context.Background(), []model.Step{
		{
			ID: "r1-llm", RunID: "r1", ParentID: "r1-root", Kind: model.StepLLM,
			Name: "chat m1", Status: model.StatusOK,
			Start: base.Add(time.Second), End: base.Add(3 * time.Second),
			LLM: &model.LLMCall{
				RequestModel: "claude-sonnet-5", InputTokens: 100, OutputTokens: 20,
				InputMessages:  []model.Message{{Role: "user", Content: "hi"}},
				OutputMessages: []model.Message{{Role: "assistant", Content: "hello"}},
			},
		},
		{
			ID: "r1-tool", RunID: "r1", ParentID: "r1-root", Kind: model.StepTool,
			Name: "execute_tool t", Status: model.StatusOK,
			Start: base.Add(4 * time.Second), End: base.Add(5 * time.Second),
			Tool: &model.ToolCall{Name: "t", Arguments: `{"a":1}`, Result: `"done"`},
		},
	})
	if err != nil {
		t.Fatalf("seed detail: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/runs/r1", nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Run   runJSON    `json:"run"`
		Steps []stepJSON `json:"steps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.Run.ID != "r1" || len(resp.Steps) != 3 {
		t.Fatalf("run %s with %d steps", resp.Run.ID, len(resp.Steps))
	}
	var llm, tool *stepJSON
	for i := range resp.Steps {
		switch resp.Steps[i].Kind {
		case "llm":
			llm = &resp.Steps[i]
		case "tool":
			tool = &resp.Steps[i]
		}
	}
	if llm == nil || llm.LLM == nil || len(llm.LLM.InputMessages) != 1 || llm.LLM.InputMessages[0].Content != "hi" {
		t.Errorf("llm messages did not survive round-trip: %+v", llm)
	}
	if llm.OffsetMS != 1000 || llm.DurationMS != 2000 {
		t.Errorf("llm timing: offset=%d duration=%d", llm.OffsetMS, llm.DurationMS)
	}
	if tool == nil || tool.Tool == nil || tool.Tool.Arguments != `{"a":1}` || tool.Tool.Result != `"done"` {
		t.Errorf("tool detail did not survive round-trip: %+v", tool)
	}
}

func TestGetRunNotFoundAPI(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/runs/nope", nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestAssertionsAPI(t *testing.T) {
	srv, st := testServer(t)
	seedRun(t, st, "r1", 1000)

	// Create two assertions via the API.
	mkAssert := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/assertions", strings.NewReader(body))
		w := httptest.NewRecorder()
		srv.uiHandler().ServeHTTP(w, req)
		return w
	}
	if w := mkAssert(`{"name":"fast","type":"max_latency_ms","config":"20000"}`); w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if w := mkAssert(`{"name":"bad","type":"regex","config":"("}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid regex accepted: %d", w.Code)
	}

	// On-demand evaluation backfills the existing run.
	req := httptest.NewRequest(http.MethodPost, "/api/assertions/evaluate?project=default", nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("evaluate: %d %s", w.Code, w.Body.String())
	}

	// Results appear on the run detail.
	req = httptest.NewRequest(http.MethodGet, "/api/runs/r1", nil)
	w = httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	var resp struct {
		AssertionResults []struct {
			Name string `json:"name"`
			Pass bool   `json:"pass"`
		} `json:"assertionResults"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if len(resp.AssertionResults) != 1 || resp.AssertionResults[0].Name != "fast" || !resp.AssertionResults[0].Pass {
		t.Fatalf("results: %+v", resp.AssertionResults)
	}
}

func TestShareLifecycle(t *testing.T) {
	srv, st := testServer(t)
	seedRun(t, st, "r1", 1000)

	// Mint a share.
	req := httptest.NewRequest(http.MethodPost, "/api/runs/r1/share", nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("mint: %d %s", w.Code, w.Body.String())
	}
	var mint struct{ Token, URL string }
	json.Unmarshal(w.Body.Bytes(), &mint)
	if len(mint.Token) != 32 || mint.URL != "/s/"+mint.Token {
		t.Fatalf("mint response: %+v", mint)
	}

	// Public endpoint returns exactly that run, no assertionResults key.
	req = httptest.NewRequest(http.MethodGet, "/api/shared/"+mint.Token, nil)
	w = httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("shared fetch: %d %s", w.Code, w.Body.String())
	}
	var shared struct {
		Run   runJSON    `json:"run"`
		Steps []stepJSON `json:"steps"`
	}
	json.Unmarshal(w.Body.Bytes(), &shared)
	if shared.Run.ID != "r1" || len(shared.Steps) == 0 {
		t.Fatalf("shared run wrong: %+v", shared.Run)
	}

	// It's listed for the run.
	req = httptest.NewRequest(http.MethodGet, "/api/runs/r1/shares", nil)
	w = httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	var list struct {
		Shares []struct{ Token string } `json:"shares"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	if len(list.Shares) != 1 || list.Shares[0].Token != mint.Token {
		t.Fatalf("share not listed: %+v", list)
	}

	// Revoke → public endpoint 404s.
	req = httptest.NewRequest(http.MethodDelete, "/api/shares/"+mint.Token, nil)
	w = httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", w.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/shared/"+mint.Token, nil)
	w = httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("revoked share should 404, got %d", w.Code)
	}
}

func TestSharedUnknownToken(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/shared/deadbeef", nil)
	w := httptest.NewRecorder()
	srv.uiHandler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token should 404, got %d", w.Code)
	}
}
