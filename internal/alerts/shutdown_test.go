package alerts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/store"
)

// hangStore reports one alert that is always over threshold, so the watcher
// notifies on its first cycle.
type hangStore struct{ url string }

func (h hangStore) ListEnabledAlerts(context.Context) ([]store.Rule, error) {
	return []store.Rule{{
		ID: 1, Project: "default", Name: "hang", Type: "error_rate",
		Threshold: 0, WindowSecs: 60, WebhookURL: h.url, Enabled: true,
	}}, nil
}

func (h hangStore) GetStats(context.Context, store.Filter) (store.Stats, error) {
	return store.Stats{Runs: 10, Errors: 10}, nil
}

func (h hangStore) SetAlertFiring(context.Context, int64, bool) error   { return nil }
func (h hangStore) SetAlertPending(context.Context, int64, int64) error { return nil }

// Stop must cancel an in-flight webhook rather than wait out the HTTP client
// timeout (#104). Before the fix the POST was built with http.NewRequest, so
// nothing could cancel it and shutdown blocked for the full client timeout.
func TestStopCancelsInFlightWebhook(t *testing.T) {
	blocked := make(chan struct{}, 1)
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case blocked <- struct{}{}:
		default:
		}
		// Hold the request open until the client gives up or the test ends.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	// Declared after srv.Close so it runs first: Close waits for outstanding
	// requests, which are exactly the ones parked above.
	defer srv.Close()
	defer close(release)

	w := NewWatcher(hangStore{url: srv.URL}, 10*time.Millisecond)
	w.Start()

	select {
	case <-blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("watcher never posted the webhook")
	}

	done := make(chan struct{})
	go func() { w.Stop(); close(done) }()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked on the hung webhook instead of cancelling it")
	}
}
