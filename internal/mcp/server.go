package mcp

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/otterscope/otterscope/internal/store"
)

// latestVersion is the MCP revision this server prefers; supportedVersions
// are those it will negotiate. The server echoes the client's requested
// version when supported, else returns the latest (per the lifecycle spec).
const latestVersion = "2025-11-25"

var supportedVersions = map[string]bool{
	"2025-11-25": true,
	"2025-06-18": true,
	"2025-03-26": true,
}

const serverName = "otterscope"

// jsonrpcRequest is an incoming JSON-RPC 2.0 message. A missing id marks a
// notification (no response expected).
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handler serves the MCP Streamable HTTP endpoint.
type Handler struct {
	tools   []Tool
	byName  map[string]Tool
	version string
}

// NewHandler builds the MCP handler over the store.
func NewHandler(st *store.Store, version string) *Handler {
	tools := Registry(st)
	byName := make(map[string]Tool, len(tools))
	for _, t := range tools {
		byName[t.Name] = t
	}
	return &Handler{tools: tools, byName: byName, version: version}
}

// ServeHTTP handles a single JSON-RPC request per POST (Streamable HTTP,
// non-streaming: simple request/response tools return application/json).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// DNS-rebinding protection: reject cross-origin browser requests. MCP
	// clients (Claude Code, Inspector) send no Origin; browsers do.
	if origin := r.Header.Get("Origin"); origin != "" && !localOrigin(origin) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "parse error")
		return
	}

	// After init, MCP-Protocol-Version must be present and supported. Absent
	// is tolerated (spec default); present-but-unsupported is a 400.
	if req.Method != "initialize" {
		if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !supportedVersions[v] {
			http.Error(w, "unsupported MCP-Protocol-Version", http.StatusBadRequest)
			return
		}
	}

	// Notifications (no id) get 202 Accepted with no body.
	if len(req.ID) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	switch req.Method {
	case "initialize":
		writeRPCResult(w, req.ID, map[string]any{
			"protocolVersion": negotiateVersion(req.Params),
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo": map[string]any{
				"name": serverName, "title": "Otterscope", "version": h.version,
			},
			"instructions": "Query your agent's observability data: list_runs, get_run (with messages and tool i/o), get_stats, list_assertions.",
		})
	case "ping":
		writeRPCResult(w, req.ID, map[string]any{})
	case "tools/list":
		writeRPCResult(w, req.ID, map[string]any{"tools": h.toolDefs()})
	case "tools/call":
		h.callTool(w, r, req)
	default:
		writeRPCError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (h *Handler) toolDefs() []map[string]any {
	defs := make([]map[string]any, 0, len(h.tools))
	for _, t := range h.tools {
		defs = append(defs, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"inputSchema": t.InputSchema,
			// Every Otterscope tool is read-only.
			"annotations": map[string]any{"readOnlyHint": true},
		})
	}
	return defs
}

// negotiateVersion echoes the client's requested protocolVersion when
// supported, else returns the server's latest (lifecycle spec §negotiation).
func negotiateVersion(params json.RawMessage) string {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(params, &p); err == nil && supportedVersions[p.ProtocolVersion] {
		return p.ProtocolVersion
	}
	return latestVersion
}

// localOrigin reports whether an Origin header points at loopback.
func localOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost" || host == "::1" ||
		strings.HasPrefix(host, "127.")
}

func (h *Handler) callTool(w http.ResponseWriter, r *http.Request, req jsonrpcRequest) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}
	tool, ok := h.byName[params.Name]
	if !ok {
		writeRPCError(w, req.ID, -32602, "unknown tool: "+params.Name)
		return
	}
	if params.Arguments == nil {
		params.Arguments = map[string]any{}
	}
	// A tool error is reported in-band (isError: true), not as a JSON-RPC
	// error, so the model can read and react to it.
	text, err := tool.Handler(r.Context(), params.Arguments)
	if err != nil {
		writeRPCResult(w, req.ID, toolResult(err.Error(), true))
		return
	}
	writeRPCResult(w, req.ID, toolResult(text, false))
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

func writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	writeJSON(w, jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	writeJSON(w, jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
