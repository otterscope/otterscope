// Package ingest owns the OTLP/HTTP receiver and (in later issues) the
// normalization of GenAI span dialects into internal/model. OpenTelemetry
// types must not be used outside this package.
//
// Spec deviation, deliberate: error response bodies are text/plain instead
// of google.rpc.Status protos. OTLP exporters act on HTTP status codes;
// encoding Status would add a genproto dependency for bodies nothing reads.
// Revisit via an issue if a real SDK misbehaves on it.
package ingest

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptraceotlp"
)

// maxBodyBytes caps decompressed request bodies. 32 MiB is far above any
// sane single OTLP batch.
const maxBodyBytes = 32 << 20

const (
	ctProto = "application/x-protobuf"
	ctJSON  = "application/json"
)

// Sink receives every decoded trace batch. Implementations must be safe for
// concurrent use.
type Sink interface {
	ConsumeTraces(ctx context.Context, td ptrace.Traces) error
}

// NewHandler returns the OTLP/HTTP handler, routing POST /v1/traces to sink.
func NewHandler(sink Sink) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /v1/traces", &tracesHandler{sink: sink})
	return mux
}

type tracesHandler struct {
	sink Sink
}

func (h *tracesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (contentType != ctProto && contentType != ctJSON) {
		http.Error(w, "unsupported content type: use application/x-protobuf or application/json", http.StatusUnsupportedMediaType)
		return
	}

	body, err := readBody(r)
	if err != nil {
		status := http.StatusBadRequest
		if _, ok := err.(*http.MaxBytesError); ok {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}

	req := ptraceotlp.NewExportRequest()
	switch contentType {
	case ctProto:
		err = req.UnmarshalProto(body)
	case ctJSON:
		err = req.UnmarshalJSON(body)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("malformed %s payload: %v", contentType, err), http.StatusBadRequest)
		return
	}

	if err := h.sink.ConsumeTraces(r.Context(), req.Traces()); err != nil {
		slog.Error("ingest: sink failed", "err", err)
		http.Error(w, "failed to store traces", http.StatusInternalServerError)
		return
	}

	writeExportResponse(w, contentType)
}

func readBody(r *http.Request) ([]byte, error) {
	var reader io.Reader = http.MaxBytesReader(nil, r.Body, maxBodyBytes)
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("bad gzip body: %w", err)
		}
		defer gz.Close()
		// Cap the decompressed size too, or a tiny gzip bomb bypasses the
		// body limit.
		reader = io.LimitReader(gz, maxBodyBytes+1)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > maxBodyBytes {
		return nil, &http.MaxBytesError{Limit: maxBodyBytes}
	}
	return body, nil
}

func writeExportResponse(w http.ResponseWriter, contentType string) {
	resp := ptraceotlp.NewExportResponse()
	var body []byte
	var err error
	switch contentType {
	case ctProto:
		body, err = resp.MarshalProto()
	case ctJSON:
		body, err = resp.MarshalJSON()
	}
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Write(body)
}
