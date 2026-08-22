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

// settleHarness drives a watcher over a scripted sequence of readings with a
// controllable clock, recording every webhook it sends.
type settleHarness struct {
	t     *testing.T
	w     *Watcher
	fs    *fakeStore
	now   time.Time
	mu    sync.Mutex
	sent  []Notification
	close func()
}

func newSettleHarness(t *testing.T, settleSecs int64) *settleHarness {
	t.Helper()
	h := &settleHarness{t: t, now: time.Unix(1_000_000, 0)}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var n Notification
		json.NewDecoder(r.Body).Decode(&n)
		h.mu.Lock()
		h.sent = append(h.sent, n)
		h.mu.Unlock()
	}))
	h.close = srv.Close

	h.fs = &fakeStore{
		rules: []store.Rule{{
			ID: 1, Project: "default", Name: "errs", Type: "error_rate",
			Threshold: 0.2, WindowSecs: 3600, WebhookURL: srv.URL,
			Enabled: true, SettleSecs: settleSecs,
		}},
		set: map[int64]bool{},
	}
	h.w = NewWatcher(h.fs, time.Hour)
	h.w.now = func() time.Time { return h.now }
	return h
}

// tick advances the clock and evaluates once with the given error rate.
func (h *settleHarness) tick(d time.Duration, errors int64) {
	h.t.Helper()
	h.now = h.now.Add(d)
	h.fs.mu.Lock()
	h.fs.stats = store.Stats{Runs: 10, Errors: errors}
	h.fs.mu.Unlock()
	h.w.EvaluateOnce(context.Background())
}

func (h *settleHarness) statuses() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.sent))
	for _, n := range h.sent {
		out = append(out, n.Status)
	}
	return out
}

func (h *settleHarness) firing() bool {
	h.fs.mu.Lock()
	defer h.fs.mu.Unlock()
	return h.fs.rules[0].Firing
}

// A metric oscillating across the threshold must not produce a notification
// per evaluation (#108). With a settle window it produces none at all: the
// condition never holds long enough to be worth telling anyone about.
func TestOscillationIsSilencedBySettle(t *testing.T) {
	h := newSettleHarness(t, 300) // 5 minutes
	defer h.close()

	// 10 minutes of alternating readings, one evaluation a minute.
	for i := 0; i < 10; i++ {
		errors := int64(5) // 50% > 20% threshold
		if i%2 == 1 {
			errors = 0
		}
		h.tick(time.Minute, errors)
	}

	if got := h.statuses(); len(got) != 0 {
		t.Errorf("oscillating metric sent %d notifications (%v), want none", len(got), got)
	}
	if h.firing() {
		t.Error("rule latched to firing despite never settling")
	}
}

// Without a settle window the same sequence is exactly the noise the issue
// describes — this pins why the feature exists.
func TestOscillationWithoutSettleIsNoisy(t *testing.T) {
	h := newSettleHarness(t, 0)
	defer h.close()

	for i := 0; i < 10; i++ {
		errors := int64(5)
		if i%2 == 1 {
			errors = 0
		}
		h.tick(time.Minute, errors)
	}

	if got := h.statuses(); len(got) != 10 {
		t.Errorf("got %d notifications (%v), want 10 — the un-damped behaviour", len(got), got)
	}
}

// A condition that genuinely persists still fires, once, after the settle
// window elapses — suppression must not mean silence.
func TestPersistentConditionFiresAfterSettling(t *testing.T) {
	h := newSettleHarness(t, 300)
	defer h.close()

	h.tick(time.Minute, 5) // starts the settle timer, no notification yet
	if got := h.statuses(); len(got) != 0 {
		t.Fatalf("notified before settling: %v", got)
	}

	h.tick(2*time.Minute, 5) // 2 min held — still short of 5
	if got := h.statuses(); len(got) != 0 {
		t.Fatalf("notified after 2 min of a 5 min settle window: %v", got)
	}

	h.tick(4*time.Minute, 5) // 6 min held — settled
	if got := h.statuses(); len(got) != 1 || got[0] != "firing" {
		t.Fatalf("got %v, want exactly one firing notification", got)
	}
	if !h.firing() {
		t.Error("firing state not persisted")
	}

	// Still firing on later evaluations: no repeats.
	h.tick(10*time.Minute, 5)
	if got := h.statuses(); len(got) != 1 {
		t.Errorf("re-fired while already firing: %v", got)
	}
}

// Recovery is damped the same way, so a value dipping below the threshold for
// one evaluation does not produce a premature "resolved".
func TestRecoveryMustAlsoSettle(t *testing.T) {
	h := newSettleHarness(t, 300)
	defer h.close()

	h.tick(time.Minute, 5)
	h.tick(6*time.Minute, 5) // fires
	if got := h.statuses(); len(got) != 1 || got[0] != "firing" {
		t.Fatalf("setup: got %v", got)
	}

	h.tick(time.Minute, 0) // recovered — timer starts
	h.tick(time.Minute, 5) // back over threshold before settling — timer clears
	h.tick(time.Minute, 0) // recovered again — timer restarts from here
	if got := h.statuses(); len(got) != 1 {
		t.Fatalf("resolved prematurely: %v", got)
	}
	if !h.firing() {
		t.Error("rule left firing state without a settled recovery")
	}

	h.tick(6*time.Minute, 0) // held clear long enough
	if got := h.statuses(); len(got) != 2 || got[1] != "resolved" {
		t.Fatalf("got %v, want a resolved notification after the recovery settled", got)
	}
	if h.firing() {
		t.Error("firing state not cleared after resolve")
	}
}

// A flap that reverses before settling must reset the timer, not accumulate
// toward it — otherwise a metric that is over the threshold half the time
// eventually fires anyway.
func TestFlapResetsTheSettleTimer(t *testing.T) {
	h := newSettleHarness(t, 300)
	defer h.close()

	// Four minutes over threshold, then one reading under, then four more.
	// Total time over threshold is eight minutes, but never five in a row.
	h.tick(time.Minute, 5)
	h.tick(4*time.Minute, 5) // 4 min held, not yet settled
	h.tick(time.Minute, 0)   // flap back: timer cleared
	h.tick(4*time.Minute, 5) // starts over, 0 min held at this point

	if got := h.statuses(); len(got) != 0 {
		t.Errorf("settle timer accumulated across a flap: %v", got)
	}
}

func TestValidateSettleSecs(t *testing.T) {
	base := store.Rule{
		Name: "n", Type: "error_rate", Threshold: 0.5,
		WindowSecs: 3600, WebhookURL: "https://example.invalid/hook",
	}
	ok := func(secs int64) store.Rule { r := base; r.SettleSecs = secs; return r }

	for _, secs := range []int64{0, 1, 300, maxSettleSecs} {
		if err := Validate(ok(secs)); err != nil {
			t.Errorf("settleSecs=%d rejected: %v", secs, err)
		}
	}
	for _, secs := range []int64{-1, maxSettleSecs + 1, 300_000} {
		if err := Validate(ok(secs)); err == nil {
			t.Errorf("settleSecs=%d accepted, want an error", secs)
		}
	}
}
