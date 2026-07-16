package ingest

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

const fixtureSpanCount = 3

// openResolver mimics an instance with only the keyless default project.
func openResolver(_ context.Context, key string) (string, bool) {
	if key == "" {
		return "default", true
	}
	if key == "valid-key" {
		return "prod", true
	}
	return "", false
}

type countSink struct {
	spans       atomic.Int64
	lastProject atomic.Pointer[string]
	err         error
}

func (s *countSink) ConsumeTraces(_ context.Context, project string, td ptrace.Traces) error {
	if s.err != nil {
		return s.err
	}
	s.lastProject.Store(&project)
	s.spans.Add(int64(td.SpanCount()))
	return nil
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return b
}

func post(t *testing.T, h http.Handler, contentType string, body []byte, gzipped bool) *httptest.ResponseRecorder {
	t.Helper()
	if gzipped {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write(body)
		gz.Close()
		body = buf.Bytes()
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	if gzipped {
		req.Header.Set("Content-Encoding", "gzip")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestJSONIngest(t *testing.T) {
	sink := &countSink{}
	w := post(t, NewHandler(sink, openResolver), ctJSON, fixture(t, "pydantic_ai_chat.json"), false)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := sink.spans.Load(); got != fixtureSpanCount {
		t.Fatalf("sink got %d spans, want %d", got, fixtureSpanCount)
	}
	if ct := w.Header().Get("Content-Type"); ct != ctJSON {
		t.Fatalf("response content type = %q, want %q", ct, ctJSON)
	}
	resp := ptraceotlp.NewExportResponse()
	if err := resp.UnmarshalJSON(w.Body.Bytes()); err != nil {
		t.Fatalf("response is not a valid ExportTraceServiceResponse: %v", err)
	}
}

func TestProtoIngest(t *testing.T) {
	sink := &countSink{}
	w := post(t, NewHandler(sink, openResolver), ctProto, fixture(t, "pydantic_ai_chat.pb"), false)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := sink.spans.Load(); got != fixtureSpanCount {
		t.Fatalf("sink got %d spans, want %d", got, fixtureSpanCount)
	}
	resp := ptraceotlp.NewExportResponse()
	if err := resp.UnmarshalProto(w.Body.Bytes()); err != nil {
		t.Fatalf("response is not a valid proto ExportTraceServiceResponse: %v", err)
	}
}

func TestGzipJSONIngest(t *testing.T) {
	sink := &countSink{}
	w := post(t, NewHandler(sink, openResolver), ctJSON, fixture(t, "pydantic_ai_chat.json"), true)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if got := sink.spans.Load(); got != fixtureSpanCount {
		t.Fatalf("sink got %d spans, want %d", got, fixtureSpanCount)
	}
}

func TestMalformedPayload(t *testing.T) {
	for _, ct := range []string{ctJSON, ctProto} {
		w := post(t, NewHandler(&countSink{}, openResolver), ct, []byte(`{"resourceSpans": [{]`), false)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", ct, w.Code)
		}
	}
}

func TestBadGzip(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte("not gzip")))
	req.Header.Set("Content-Type", ctJSON)
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	NewHandler(&countSink{}, openResolver).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestUnsupportedContentType(t *testing.T) {
	w := post(t, NewHandler(&countSink{}, openResolver), "text/plain", []byte("hi"), false)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", w.Code)
	}
}

func TestSinkErrorIs500(t *testing.T) {
	sink := &countSink{err: errors.New("disk full")}
	w := post(t, NewHandler(sink, openResolver), ctJSON, fixture(t, "pydantic_ai_chat.json"), false)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// TestFixtureRoundTrip guarantees the .pb fixture stays the protobuf twin of
// the JSON fixture. Regenerate the .pb after editing the JSON with:
//
//	OTTERSCOPE_REGEN_FIXTURES=1 go test ./internal/ingest -run TestFixtureRoundTrip
func TestFixtureRoundTrip(t *testing.T) {
	jsonReq := ptraceotlp.NewExportRequest()
	if err := jsonReq.UnmarshalJSON(fixture(t, "pydantic_ai_chat.json")); err != nil {
		t.Fatalf("unmarshal JSON fixture: %v", err)
	}
	pb, err := jsonReq.MarshalProto()
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}

	pbPath := filepath.Join("testdata", "pydantic_ai_chat.pb")
	if os.Getenv("OTTERSCOPE_REGEN_FIXTURES") == "1" {
		if err := os.WriteFile(pbPath, pb, 0o644); err != nil {
			t.Fatalf("write .pb fixture: %v", err)
		}
	}

	onDisk, err := os.ReadFile(pbPath)
	if err != nil {
		t.Fatalf("read .pb fixture (regenerate with OTTERSCOPE_REGEN_FIXTURES=1): %v", err)
	}
	if !bytes.Equal(onDisk, pb) {
		t.Fatal(".pb fixture is stale relative to the JSON fixture; regenerate with OTTERSCOPE_REGEN_FIXTURES=1")
	}

	protoReq := ptraceotlp.NewExportRequest()
	if err := protoReq.UnmarshalProto(onDisk); err != nil {
		t.Fatalf("unmarshal .pb fixture: %v", err)
	}
	if got, want := protoReq.Traces().SpanCount(), jsonReq.Traces().SpanCount(); got != want {
		t.Fatalf("proto fixture has %d spans, JSON fixture has %d", got, want)
	}
}

func TestIngestAuth(t *testing.T) {
	sink := &countSink{}
	h := NewHandler(sink, openResolver)

	// Valid key routes to its project.
	req := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(fixture(t, "pydantic_ai_chat.json")))
	req.Header.Set("Content-Type", ctJSON)
	req.Header.Set("Authorization", "Bearer valid-key")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("valid key: status %d", w.Code)
	}
	if p := sink.lastProject.Load(); p == nil || *p != "prod" {
		t.Fatalf("project = %v, want prod", p)
	}

	// Unknown key is rejected before decoding.
	req = httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(fixture(t, "pydantic_ai_chat.json")))
	req.Header.Set("Content-Type", ctJSON)
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown key: status %d, want 401", w.Code)
	}

	// No header lands in default.
	w = post(t, h, ctJSON, fixture(t, "pydantic_ai_chat.json"), false)
	if w.Code != http.StatusOK {
		t.Fatalf("keyless: status %d", w.Code)
	}
	if p := sink.lastProject.Load(); p == nil || *p != "default" {
		t.Fatalf("keyless project = %v, want default", p)
	}
}
