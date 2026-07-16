package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/store"
)

func f64(v float64) *float64 { return &v }

func TestValidate(t *testing.T) {
	base := store.Rule{Name: "n", WebhookURL: "http://x", WindowSecs: 3600}
	good := []store.Rule{
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "error_rate", Threshold: 0.1},
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "cost", Threshold: 5},
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "p95_latency", Threshold: 1000},
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "assertion_fail_rate", Threshold: 0.2, Config: "a"},
	}
	for _, r := range good {
		if err := Validate(r); err != nil {
			t.Errorf("%s: %v", r.Type, err)
		}
	}
	bad := []store.Rule{
		{Name: "", WebhookURL: "http://x", WindowSecs: 3600, Type: "cost", Threshold: 1},
		func() store.Rule { r := base; r.Type = "cost"; r.WebhookURL = ""; r.Threshold = 1; return r }(),
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "error_rate", Threshold: 5},
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "assertion_fail_rate", Threshold: 0.2},
		{Name: "n", WebhookURL: "http://x", WindowSecs: 3600, Type: "bogus", Threshold: 1},
	}
	for i, r := range bad {
		if err := Validate(r); err == nil {
			t.Errorf("bad[%d] (%s) should be invalid", i, r.Type)
		}
	}
}

func TestEvaluate(t *testing.T) {
	stats := store.Stats{
		Runs: 10, Errors: 3, P95DurationMS: 8000, TotalCostUSD: f64(2.5),
		AssertionRates: []store.AssertionRate{{Name: "polite", Passed: 6, Total: 10}},
	}
	cases := []struct {
		rule    store.Rule
		firing  bool
		firable bool
	}{
		{store.Rule{Type: "error_rate", Threshold: 0.2}, true, true},                            // 30% > 20%
		{store.Rule{Type: "error_rate", Threshold: 0.5}, false, true},                           // 30% < 50%
		{store.Rule{Type: "cost", Threshold: 2.0}, true, true},                                  // 2.5 > 2.0
		{store.Rule{Type: "p95_latency", Threshold: 5000}, true, true},                          // 8000 > 5000
		{store.Rule{Type: "assertion_fail_rate", Threshold: 0.3, Config: "polite"}, true, true}, // 40% fail > 30%
		{store.Rule{Type: "assertion_fail_rate", Threshold: 0.5, Config: "polite"}, false, true},
		{store.Rule{Type: "assertion_fail_rate", Threshold: 0.1, Config: "missing"}, false, false},
	}
	for _, c := range cases {
		firing, firable, _ := Evaluate(c.rule, stats)
		if firing != c.firing || firable != c.firable {
			t.Errorf("%s/%s: got firing=%v firable=%v, want %v/%v",
				c.rule.Type, c.rule.Config, firing, firable, c.firing, c.firable)
		}
	}
}

// fakeStore drives the watcher without a DB.
type fakeStore struct {
	mu    sync.Mutex
	rules []store.Rule
	stats store.Stats
	set   map[int64]bool
}

func (f *fakeStore) ListEnabledAlerts(context.Context) ([]store.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.rules, nil
}
func (f *fakeStore) GetStats(context.Context, store.Filter) (store.Stats, error) {
	return f.stats, nil
}
func (f *fakeStore) SetAlertFiring(_ context.Context, id int64, firing bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.set[id] = firing
	for i := range f.rules {
		if f.rules[i].ID == id {
			f.rules[i].Firing = firing
		}
	}
	return nil
}

func TestWatcherFiresAndResolves(t *testing.T) {
	var got []Notification
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n Notification
		json.NewDecoder(r.Body).Decode(&n)
		mu.Lock()
		got = append(got, n)
		mu.Unlock()
	}))
	defer srv.Close()

	fs := &fakeStore{
		rules: []store.Rule{{ID: 1, Name: "errs", Type: "error_rate", Threshold: 0.2,
			WindowSecs: 3600, WebhookURL: srv.URL, Enabled: true}},
		stats: store.Stats{Runs: 10, Errors: 5}, // 50% > 20% → firing
		set:   map[int64]bool{},
	}
	w := NewWatcher(fs, time.Hour)

	w.EvaluateOnce(context.Background())
	if len(got) != 1 || got[0].Status != "firing" {
		t.Fatalf("expected one firing notification, got %+v", got)
	}
	if !fs.set[1] {
		t.Fatal("firing state not persisted")
	}

	// Same firing state → no re-fire.
	w.EvaluateOnce(context.Background())
	if len(got) != 1 {
		t.Fatalf("re-fired while still firing: %d notifications", len(got))
	}

	// Recover → resolved.
	fs.mu.Lock()
	fs.stats = store.Stats{Runs: 10, Errors: 0}
	fs.mu.Unlock()
	w.EvaluateOnce(context.Background())
	if len(got) != 2 || got[1].Status != "resolved" {
		t.Fatalf("expected resolve notification, got %+v", got)
	}
}
