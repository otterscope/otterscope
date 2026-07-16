package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func do(t *testing.T, h http.Handler, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) jsonrpcResponse {
	t.Helper()
	var r jsonrpcResponse
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("bad JSON-RPC: %v (%s)", err, w.Body.String())
	}
	return r
}

func TestInitializeEchoesSupportedVersion(t *testing.T) {
	h := NewHandler(testStore(t), "test")

	// Client requesting a supported (older) version must get it echoed back.
	w := do(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`, nil)
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	res := decode(t, w).Result.(map[string]any)
	if res["protocolVersion"] != "2025-06-18" {
		t.Errorf("version = %v, want echo 2025-06-18", res["protocolVersion"])
	}
	caps := res["capabilities"].(map[string]any)
	if _, ok := caps["tools"]; !ok {
		t.Error("tools capability not advertised")
	}

	// Unknown requested version → server returns its latest.
	w = do(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01","capabilities":{}}}`, nil)
	if decode(t, w).Result.(map[string]any)["protocolVersion"] != latestVersion {
		t.Error("unknown version should fall back to latest")
	}
}

func TestNotificationReturns202(t *testing.T) {
	h := NewHandler(testStore(t), "test")
	w := do(t, h, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("notification response must be empty, got %q", w.Body.String())
	}
}

func TestToolsListAndCall(t *testing.T) {
	st := testStore(t)
	seed(t, st, "r1", 1000)
	h := NewHandler(st, "test")

	w := do(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		map[string]string{"MCP-Protocol-Version": "2025-11-25"})
	res := decode(t, w).Result.(map[string]any)
	tools := res["tools"].([]any)
	if len(tools) < 3 {
		t.Fatalf("expected several tools, got %d", len(tools))
	}
	first := tools[0].(map[string]any)
	if first["inputSchema"] == nil || first["name"] == "" {
		t.Errorf("tool def incomplete: %+v", first)
	}
	if ann, ok := first["annotations"].(map[string]any); !ok || ann["readOnlyHint"] != true {
		t.Errorf("readOnlyHint annotation missing: %+v", first)
	}

	// Call get_run and confirm an in-band text result.
	w = do(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_run","arguments":{"id":"r1"}}}`,
		map[string]string{"MCP-Protocol-Version": "2025-11-25"})
	call := decode(t, w).Result.(map[string]any)
	if call["isError"] == true {
		t.Fatalf("get_run errored: %+v", call)
	}
	content := call["content"].([]any)
	text := content[0].(map[string]any)
	if text["type"] != "text" || text["text"] == "" {
		t.Errorf("bad content block: %+v", text)
	}
}

func TestToolErrorIsInBand(t *testing.T) {
	h := NewHandler(testStore(t), "test")
	// Unknown run → tool execution error (isError:true result), not JSON-RPC error.
	w := do(t, h, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_run","arguments":{"id":"missing"}}}`, nil)
	r := decode(t, w)
	if r.Error != nil {
		t.Fatalf("tool failure should be in-band, got JSON-RPC error %+v", r.Error)
	}
	if r.Result.(map[string]any)["isError"] != true {
		t.Error("expected isError:true")
	}

	// Unknown tool → JSON-RPC protocol error.
	w = do(t, h, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`, nil)
	if decode(t, w).Error == nil {
		t.Error("unknown tool should be a JSON-RPC error")
	}
}

func TestRejectsBadOriginAndVersion(t *testing.T) {
	h := NewHandler(testStore(t), "test")
	if w := do(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		map[string]string{"Origin": "https://evil.example.com"}); w.Code != http.StatusForbidden {
		t.Errorf("bad origin status = %d, want 403", w.Code)
	}
	if w := do(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		map[string]string{"MCP-Protocol-Version": "1999-01-01"}); w.Code != http.StatusBadRequest {
		t.Errorf("bad version status = %d, want 400", w.Code)
	}
	// Local origin is fine.
	if w := do(t, h, `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
		map[string]string{"Origin": "http://localhost:8317"}); w.Code != http.StatusOK {
		t.Errorf("local origin status = %d, want 200", w.Code)
	}
}
