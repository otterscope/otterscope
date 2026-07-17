package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/otterscope/otterscope/internal/store"
)

// Store is the subset of the store the watcher needs (kept small for tests).
type Store interface {
	ListEnabledAlerts(ctx context.Context) ([]store.Rule, error)
	GetStats(ctx context.Context, f store.Filter) (store.Stats, error)
	SetAlertFiring(ctx context.Context, id int64, firing bool) error
}

// Notification is the JSON payload POSTed to an alert's webhook. It carries
// alert data only — never secrets or environment (see issue #48).
type Notification struct {
	Alert   string  `json:"alert"`
	Project string  `json:"project"`
	Type    string  `json:"type"`
	Status  string  `json:"status"` // "firing" | "resolved"
	Detail  string  `json:"detail"`
	Value   float64 `json:"threshold"`
	FiredAt string  `json:"firedAt"`
}

// Watcher periodically evaluates enabled alerts and fires webhooks on
// ok→firing / firing→ok transitions. Owned by the server lifecycle.
type Watcher struct {
	st       Store
	interval time.Duration
	client   *http.Client
	now      func() time.Time
	stop     chan struct{}
	wg       sync.WaitGroup
}

// NewWatcher creates a watcher evaluating every interval.
func NewWatcher(st Store, interval time.Duration) *Watcher {
	return &Watcher{
		st:       st,
		interval: interval,
		client:   &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
		stop:     make(chan struct{}),
	}
}

// Start launches the evaluation loop.
func (w *Watcher) Start() {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		tick := time.NewTicker(w.interval)
		defer tick.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-tick.C:
				ctx, cancel := context.WithTimeout(context.Background(), w.interval)
				w.evaluateOnce(ctx)
				cancel()
			}
		}
	}()
}

// Stop halts the loop and waits for the in-flight evaluation to finish.
func (w *Watcher) Stop() {
	close(w.stop)
	w.wg.Wait()
}

// evaluateOnce evaluates all enabled alerts a single time. Exported-ish for
// tests via EvaluateOnce.
func (w *Watcher) evaluateOnce(ctx context.Context) {
	rules, err := w.st.ListEnabledAlerts(ctx)
	if err != nil {
		slog.Error("alerts: list failed", "err", err)
		return
	}
	// Cache stats per (project, window) — many alerts share a window.
	type key struct {
		project string
		window  int64
	}
	statsCache := map[key]store.Stats{}

	for _, r := range rules {
		k := key{r.Project, r.WindowSecs}
		stats, ok := statsCache[k]
		if !ok {
			since := w.now().Add(-time.Duration(r.WindowSecs) * time.Second)
			stats, err = w.st.GetStats(ctx, store.Filter{Project: r.Project, Since: since})
			if err != nil {
				slog.Error("alerts: stats failed", "alert", r.Name, "err", err)
				continue
			}
			statsCache[k] = stats
		}

		firing, firable, detail := Evaluate(r, stats)
		if !firable {
			continue
		}
		switch {
		case firing && !r.Firing:
			w.notify(r, "firing", detail)
			if err := w.st.SetAlertFiring(ctx, r.ID, true); err != nil {
				slog.Error("alerts: set firing failed", "alert", r.Name, "err", err)
			}
		case !firing && r.Firing:
			w.notify(r, "resolved", detail)
			if err := w.st.SetAlertFiring(ctx, r.ID, false); err != nil {
				slog.Error("alerts: clear firing failed", "alert", r.Name, "err", err)
			}
		}
	}
}

// EvaluateOnce runs a single evaluation cycle synchronously (for tests and a
// possible future manual trigger).
func (w *Watcher) EvaluateOnce(ctx context.Context) { w.evaluateOnce(ctx) }

func (w *Watcher) notify(r store.Rule, status, detail string) {
	n := Notification{
		Alert:   r.Name,
		Project: r.Project,
		Type:    r.Type,
		Status:  status,
		Detail:  detail,
		Value:   r.Threshold,
		FiredAt: w.now().UTC().Format(time.RFC3339),
	}
	payload := webhookPayload(r.WebhookURL, n)
	req, err := http.NewRequest(http.MethodPost, r.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		slog.Error("alerts: bad webhook url", "alert", r.Name, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		slog.Error("alerts: webhook post failed", "alert", r.Name, "err", err)
		return
	}
	resp.Body.Close()
	slog.Info("alert notified", "alert", r.Name, "status", status)
}

// webhookPayload renders the notification in the shape the destination
// actually accepts: Slack and Discord ignore arbitrary JSON, so we detect
// them by host; everything else gets the generic Notification (for
// programmatic consumers).
func webhookPayload(webhookURL string, n Notification) []byte {
	firing := n.Status == "firing"
	line := n.message()

	switch destination(webhookURL) {
	case destSlack:
		color := "#5cb85c" // resolved: green
		if firing {
			color = "#d9534f" // firing: red
		}
		b, _ := json.Marshal(map[string]any{
			"attachments": []map[string]any{{"color": color, "text": line}},
		})
		return b
	case destDiscord:
		color := 0x5cb85c
		if firing {
			color = 0xd9534f
		}
		b, _ := json.Marshal(map[string]any{
			"embeds": []map[string]any{{"description": line, "color": color}},
		})
		return b
	default:
		b, _ := json.Marshal(n)
		return b
	}
}

// message is the one-line human summary used in Slack/Discord bodies.
func (n Notification) message() string {
	icon := "✅" // ✅ resolved
	label := "resolved"
	if n.Status == "firing" {
		icon = "\U0001F534" // 🔴 firing
		label = "firing"
	}
	return icon + " [" + label + "] " + n.Alert + " (" + n.Project + "): " + n.Detail
}

type dest int

const (
	destGeneric dest = iota
	destSlack
	destDiscord
)

func destination(webhookURL string) dest {
	u, err := url.Parse(webhookURL)
	if err != nil {
		return destGeneric
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case strings.Contains(host, "hooks.slack.com"):
		return destSlack
	case host == "discord.com" || host == "discordapp.com" ||
		strings.HasSuffix(host, ".discord.com") || strings.HasSuffix(host, ".discordapp.com"):
		return destDiscord
	default:
		return destGeneric
	}
}
