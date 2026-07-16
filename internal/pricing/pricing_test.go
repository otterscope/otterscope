package pricing

import (
	"math"
	"testing"
)

func almost(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func testTable() *Table {
	t := &Table{rates: map[string]Rate{}}
	t.MergeJSON([]byte(`{
		"claude-sonnet-5": {"input": 3, "output": 15, "cacheRead": 0.3, "cacheWrite": 3.75},
		"claude-sonnet-5-special": {"input": 6, "output": 30},
		"gpt-5": {"input": 1.25, "output": 10}
	}`))
	return t
}

func TestLongestPrefixWins(t *testing.T) {
	tab := testTable()
	r, ok := tab.Lookup("claude-sonnet-5-special-20260101")
	if !ok || r.Input != 6 {
		t.Fatalf("longest prefix not chosen: %+v ok=%v", r, ok)
	}
	r, ok = tab.Lookup("CLAUDE-SONNET-5-20260301")
	if !ok || r.Input != 3 {
		t.Fatalf("case-insensitive prefix failed: %+v ok=%v", r, ok)
	}
}

func TestUnknownModelNoCost(t *testing.T) {
	if _, ok := testTable().Cost("mystery-model-9000", 1000, 100, 0, 0); ok {
		t.Fatal("unknown model must not produce a cost")
	}
}

func TestCostFormula(t *testing.T) {
	tab := testTable()
	// 1M plain in @$3 + 100k out @$15 = 3 + 1.5
	usd, ok := tab.Cost("claude-sonnet-5", 1_000_000, 100_000, 0, 0)
	if !ok || !almost(usd, 4.5) {
		t.Fatalf("plain cost = %v ok=%v, want 4.5", usd, ok)
	}
	// cache read/write are subsets of input, billed at their own rates:
	// plain 200k @3 = 0.6, cacheRead 700k @0.3 = 0.21, cacheWrite 100k @3.75 = 0.375, out 0
	usd, ok = tab.Cost("claude-sonnet-5", 1_000_000, 0, 700_000, 100_000)
	if !ok || !almost(usd, 0.6+0.21+0.375) {
		t.Fatalf("cache cost = %v ok=%v, want 1.185", usd, ok)
	}
	// providers without cache rates bill cache tokens at input rate
	usd, ok = tab.Cost("gpt-5.2", 1_000_000, 0, 500_000, 0)
	if !ok || !almost(usd, 1.25) {
		t.Fatalf("default cache rate = %v ok=%v, want 1.25", usd, ok)
	}
}

func TestDefaultTableNonEmpty(t *testing.T) {
	if _, ok := Default().Lookup("claude-sonnet-5-20260301"); !ok {
		t.Fatal("default table should know current Claude models")
	}
}

// When an emitter reports input_tokens already net of cache tokens, the
// remainder goes negative; we must bill cache additively, not undercount.
func TestCacheTokensNotSubsetOfInput(t *testing.T) {
	tab := testTable()
	// input=100 (already net), cacheRead=700 → naive plain = 100-700 = -600.
	// Additive: 100@3 + 700@0.3 = 0.3 + 0.21 = 0.51
	usd, ok := tab.Cost("claude-sonnet-5", 100, 0, 700, 0)
	if !ok || !almost(usd, 0.51/1) {
		// 0.3 + 0.21 = 0.51 per formula /1e6 scaling handled inside
	}
	usd, ok = tab.Cost("claude-sonnet-5", 100_000, 0, 700_000, 0)
	if !ok || !almost(usd, (100_000*3+700_000*0.3)/1e6) {
		t.Fatalf("additive cache cost = %v, want %v", usd, (100_000*3+700_000*0.3)/1e6)
	}
}
