// Package pricing maps model IDs to per-token USD rates and computes LLM
// call costs. The embedded defaults are maintained by hand (see table.go);
// users override or extend them with a JSON file via `serve -pricing`.
// Unknown models yield no cost — never a fabricated one.
package pricing

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Rate holds USD per **million** tokens. Zero-valued cache rates mean the
// provider bills those tokens at the plain input rate.
type Rate struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cacheRead,omitempty"`
	CacheWrite float64 `json:"cacheWrite,omitempty"`
}

// Table resolves model IDs to rates by longest-prefix match,
// case-insensitive, so "claude-sonnet-5" matches "claude-sonnet-5-20260301".
type Table struct {
	rates map[string]Rate // key: lowercase model-ID prefix
}

// Default returns the embedded pricing table.
func Default() *Table {
	t := &Table{rates: make(map[string]Rate, len(defaultRates))}
	for k, v := range defaultRates {
		t.rates[strings.ToLower(k)] = v
	}
	return t
}

// MergeJSON overlays user-supplied rates: {"model-prefix": {"input": 3, ...}}.
func (t *Table) MergeJSON(data []byte) error {
	var overrides map[string]Rate
	if err := json.Unmarshal(data, &overrides); err != nil {
		return fmt.Errorf("pricing overrides: %w", err)
	}
	for k, v := range overrides {
		t.rates[strings.ToLower(k)] = v
	}
	return nil
}

// Lookup finds the rate whose prefix is the longest match for model.
func (t *Table) Lookup(model string) (Rate, bool) {
	m := strings.ToLower(model)
	bestLen := -1
	var best Rate
	for prefix, rate := range t.rates {
		if len(prefix) > bestLen && strings.HasPrefix(m, prefix) {
			bestLen = len(prefix)
			best = rate
		}
	}
	return best, bestLen >= 0
}

// Cost computes the USD cost of one call, or ok=false for unknown models.
// cacheRead and cacheWrite are subsets of in (the OTel GenAI convention).
func (t *Table) Cost(model string, in, out, cacheRead, cacheWrite int64) (float64, bool) {
	r, ok := t.Lookup(model)
	if !ok {
		return 0, false
	}
	crRate, cwRate := r.CacheRead, r.CacheWrite
	if crRate == 0 {
		crRate = r.Input
	}
	if cwRate == 0 {
		cwRate = r.Input
	}
	// The OTel GenAI convention treats cache tokens as a subset of
	// input_tokens, so we bill the remainder at the plain rate. But some
	// emitters report input_tokens already net of cache tokens; detect that
	// (remainder would go negative) and bill cache tokens additively instead
	// of clamping to zero, which would undercount cost.
	plain := in - cacheRead - cacheWrite
	if plain < 0 {
		plain = in
	}
	usd := (float64(plain)*r.Input +
		float64(cacheRead)*crRate +
		float64(cacheWrite)*cwRate +
		float64(out)*r.Output) / 1e6
	return usd, true
}
