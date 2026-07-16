package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/ingest"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/store"
)

// runJSON is the wire shape of a run in the UI API.
type runJSON struct {
	ID           string   `json:"id"`
	Project      string   `json:"project"`
	Service      string   `json:"service"`
	AgentName    string   `json:"agentName"`
	Status       string   `json:"status"`
	Start        string   `json:"start"` // RFC 3339 with ms
	DurationMS   int64    `json:"durationMs"`
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	LLMCalls     int64    `json:"llmCalls"`
	ToolCalls    int64    `json:"toolCalls"`
	Models       string   `json:"models"`
	CostUSD      *float64 `json:"costUsd,omitempty"`
	CostPartial  bool     `json:"costPartial,omitempty"`
	Error        string   `json:"error"`
}

func toRunJSON(r model.Run) runJSON {
	return runJSON{
		ID:           r.ID,
		Project:      r.Project,
		Service:      r.Service,
		AgentName:    r.AgentName,
		Status:       string(r.Status),
		Start:        r.Start.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		DurationMS:   r.End.Sub(r.Start).Milliseconds(),
		InputTokens:  r.InputTokens,
		OutputTokens: r.OutputTokens,
		LLMCalls:     r.LLMCalls,
		ToolCalls:    r.ToolCalls,
		Models:       r.Models,
		CostUSD:      r.CostUSD,
		CostPartial:  r.CostPartial,
		Error:        r.Error,
	}
}

// stepJSON is the wire shape of a step in the run-detail API.
type stepJSON struct {
	ID         string    `json:"id"`
	ParentID   string    `json:"parentId"`
	Kind       string    `json:"kind"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Start      string    `json:"start"`
	OffsetMS   int64     `json:"offsetMs"` // from run start
	DurationMS int64     `json:"durationMs"`
	Error      string    `json:"error,omitempty"`
	LLM        *llmJSON  `json:"llm,omitempty"`
	Tool       *toolJSON `json:"tool,omitempty"`
}

type llmJSON struct {
	Provider       string          `json:"provider"`
	RequestModel   string          `json:"requestModel"`
	ResponseModel  string          `json:"responseModel"`
	InputTokens    int64           `json:"inputTokens"`
	OutputTokens   int64           `json:"outputTokens"`
	CacheRead      int64           `json:"cacheReadTokens"`
	Reasoning      int64           `json:"reasoningTokens"`
	CostUSD        *float64        `json:"costUsd,omitempty"`
	InputMessages  []model.Message `json:"inputMessages,omitempty"`
	OutputMessages []model.Message `json:"outputMessages,omitempty"`
}

type toolJSON struct {
	Name      string `json:"name"`
	CallID    string `json:"callId"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
}

// handleGetRun serves GET /api/runs/{id}.
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	run, steps, err := s.st.GetRun(r.Context(), r.PathValue("id"))
	if err == store.ErrNotFound {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	out := make([]stepJSON, 0, len(steps))
	for _, st := range steps {
		sj := stepJSON{
			ID:         st.ID,
			ParentID:   st.ParentID,
			Kind:       string(st.Kind),
			Name:       st.Name,
			Status:     string(st.Status),
			Start:      st.Start.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
			OffsetMS:   st.Start.Sub(run.Start).Milliseconds(),
			DurationMS: st.End.Sub(st.Start).Milliseconds(),
			Error:      st.Error,
		}
		if st.LLM != nil {
			sj.LLM = &llmJSON{
				Provider:       st.LLM.Provider,
				RequestModel:   st.LLM.RequestModel,
				ResponseModel:  st.LLM.ResponseModel,
				InputTokens:    st.LLM.InputTokens,
				OutputTokens:   st.LLM.OutputTokens,
				CacheRead:      st.LLM.CacheReadTokens,
				Reasoning:      st.LLM.ReasoningTokens,
				CostUSD:        st.LLM.CostUSD,
				InputMessages:  st.LLM.InputMessages,
				OutputMessages: st.LLM.OutputMessages,
			}
		}
		if st.Tool != nil {
			sj.Tool = &toolJSON{
				Name:      st.Tool.Name,
				CallID:    st.Tool.CallID,
				Arguments: st.Tool.Arguments,
				Result:    st.Tool.Result,
			}
		}
		out = append(out, sj)
	}
	results, err := s.st.ResultsForRun(r.Context(), run.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run": toRunJSON(run), "steps": out, "assertionResults": results,
	})
}

// handleListAssertions serves GET /api/assertions?project=.
func (s *Server) handleListAssertions(w http.ResponseWriter, r *http.Request) {
	out, err := s.st.ListAssertions(r.Context(), r.URL.Query().Get("project"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if out == nil {
		out = []evals.Assertion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"assertions": out})
}

// handleCreateAssertion serves POST /api/assertions.
func (s *Server) handleCreateAssertion(w http.ResponseWriter, r *http.Request) {
	var a evals.Assertion
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad JSON"})
		return
	}
	a.Enabled = true
	if a.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if err := evals.Validate(a); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := s.st.CreateAssertion(r.Context(), a)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

// handleDeleteAssertion serves DELETE /api/assertions/{id}.
func (s *Server) handleDeleteAssertion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad id"})
		return
	}
	if err := s.st.DeleteAssertion(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEvaluate serves POST /api/assertions/evaluate?project= — on-demand
// backfill over all completed runs.
func (s *Server) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	project := r.URL.Query().Get("project")
	if project == "" {
		project = "default"
	}
	n, err := ingest.EvaluateProject(r.Context(), s.st, s.judge, project)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runsEvaluated": n})
}

// handleListRuns serves GET /api/runs?limit=&offset=.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 500)
	offset := queryInt(r, "offset", 0, 0, 1<<30)

	runs, err := s.st.ListRuns(r.Context(), parseFilter(r), limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	out := make([]runJSON, 0, len(runs))
	for _, run := range runs {
		out = append(out, toRunJSON(run))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

// parseFilter reads the shared run-filter query params.
func parseFilter(r *http.Request) store.Filter {
	q := r.URL.Query()
	f := store.Filter{
		Project: q.Get("project"),
		Status:  q.Get("status"),
		Service: q.Get("service"),
		Model:   q.Get("model"),
	}
	if v := q.Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t
		}
	}
	if v := q.Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Until = t
		}
	}
	return f
}

// handleStats serves GET /api/stats with the same filter params as
// /api/runs — the compare view calls it once per side.
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.st.GetStats(r.Context(), parseFilter(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if stats.AssertionRates == nil {
		stats.AssertionRates = []store.AssertionRate{}
	}
	writeJSON(w, http.StatusOK, stats)
}

// queryInt parses an integer query param with default and clamping.
func queryInt(r *http.Request, key string, def, min, max int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
