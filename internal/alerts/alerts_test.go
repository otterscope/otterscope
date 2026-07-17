package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestWebhookPayloadShapes(t *testing.T) {
	n := Notification{Alert: "spend", Project: "default", Type: "cost",
		Status: "firing", Detail: "window cost $6.00 (threshold $5.00)"}

	// Slack: attachments with a color and the message text.
	slack := map[string]any{}
	if err := json.Unmarshal(webhookPayload("https://hooks.slack.com/services/T/B/x", n), &slack); err != nil {
		t.Fatal(err)
	}
	att, ok := slack["attachments"].([]any)
	if !ok || len(att) != 1 {
		t.Fatalf("slack payload not attachments: %v", slack)
	}
	a0 := att[0].(map[string]any)
	if a0["color"] != "#d9534f" || a0["text"] == "" {
		t.Errorf("slack attachment: %+v", a0)
	}
	if s, _ := a0["text"].(string); !strings.Contains(s, "firing") || !strings.Contains(s, "spend") {
		t.Errorf("slack text missing content: %q", a0["text"])
	}

	// Discord: embeds with an integer color.
	discord := map[string]any{}
	if err := json.Unmarshal(webhookPayload("https://discord.com/api/webhooks/1/abc", n), &discord); err != nil {
		t.Fatal(err)
	}
	emb, ok := discord["embeds"].([]any)
	if !ok || len(emb) != 1 {
		t.Fatalf("discord payload not embeds: %v", discord)
	}
	e0 := emb[0].(map[string]any)
	if e0["description"] == "" || e0["color"] == nil {
		t.Errorf("discord embed: %+v", e0)
	}

	// Resolved uses the green color.
	resolved := n
	resolved.Status = "resolved"
	slackR := map[string]any{}
	json.Unmarshal(webhookPayload("https://hooks.slack.com/x", resolved), &slackR)
	if slackR["attachments"].([]any)[0].(map[string]any)["color"] != "#5cb85c" {
		t.Error("resolved should be green")
	}

	// Generic destination keeps the raw Notification shape.
	generic := map[string]any{}
	if err := json.Unmarshal(webhookPayload("https://example.com/hook", n), &generic); err != nil {
		t.Fatal(err)
	}
	if generic["alert"] != "spend" || generic["status"] != "firing" || generic["threshold"] == nil {
		t.Errorf("generic payload changed shape: %+v", generic)
	}
}

func TestDestinationDetection(t *testing.T) {
	cases := map[string]dest{
		"https://hooks.slack.com/services/T/B/x":   destSlack,
		"https://discord.com/api/webhooks/1/abc":   destDiscord,
		"https://discordapp.com/api/webhooks/1/ab": destDiscord,
		"https://ptb.discord.com/api/webhooks/1/x": destDiscord,
		"https://example.com/hook":                 destGeneric,
		"not a url":                                destGeneric,
	}
	for url, want := range cases {
		if got := destination(url); got != want {
			t.Errorf("destination(%q) = %d, want %d", url, got, want)
		}
	}
}
