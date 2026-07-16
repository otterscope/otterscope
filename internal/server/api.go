package server

import (
	"net/http"
	"strconv"

	"github.com/otterscope/otterscope/internal/model"
)

// runJSON is the wire shape of a run in the UI API.
type runJSON struct {
	ID           string `json:"id"`
	Service      string `json:"service"`
	AgentName    string `json:"agentName"`
	Status       string `json:"status"`
	Start        string `json:"start"` // RFC 3339 with ms
	DurationMS   int64  `json:"durationMs"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
	LLMCalls     int64  `json:"llmCalls"`
	ToolCalls    int64  `json:"toolCalls"`
	Models       string `json:"models"`
	Error        string `json:"error"`
}

func toRunJSON(r model.Run) runJSON {
	return runJSON{
		ID:           r.ID,
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
		Error:        r.Error,
	}
}

// handleListRuns serves GET /api/runs?limit=&offset=.
func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 500)
	offset := queryInt(r, "offset", 0, 0, 1<<30)

	runs, err := s.st.ListRuns(r.Context(), limit, offset)
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
