// Package alerts evaluates alert rules against run statistics and fires
// webhooks on state transitions. Rules (store.Rule) reuse the store's Stats
// aggregation, so an alert is just a threshold over a filtered window.
package alerts

import (
	"fmt"

	"github.com/otterscope/otterscope/internal/store"
)

// Validate checks a rule's type/config/threshold before storing it.
func Validate(r store.Rule) error {
	if r.Name == "" {
		return fmt.Errorf("alert needs a name")
	}
	if r.WebhookURL == "" {
		return fmt.Errorf("alert needs a webhook URL")
	}
	if r.WindowSecs <= 0 {
		return fmt.Errorf("windowSecs must be positive")
	}
	switch r.Type {
	case "error_rate", "assertion_fail_rate":
		if r.Threshold < 0 || r.Threshold > 1 {
			return fmt.Errorf("%s threshold is a fraction 0..1", r.Type)
		}
		if r.Type == "assertion_fail_rate" && r.Config == "" {
			return fmt.Errorf("assertion_fail_rate needs config = assertion name")
		}
	case "cost", "p95_latency":
		if r.Threshold <= 0 {
			return fmt.Errorf("%s threshold must be positive", r.Type)
		}
	default:
		return fmt.Errorf("unknown alert type %q", r.Type)
	}
	return nil
}

// Evaluate computes whether the rule is currently firing over stats, plus a
// human-readable detail. firable=false means the metric isn't computable
// (e.g. no runs, or an unknown assertion) and the rule's state is left as-is.
func Evaluate(r store.Rule, stats store.Stats) (firing, firable bool, detail string) {
	switch r.Type {
	case "error_rate":
		if stats.Runs == 0 {
			return false, false, ""
		}
		rate := float64(stats.Errors) / float64(stats.Runs)
		return rate > r.Threshold, true,
			fmt.Sprintf("error rate %.1f%% over %d runs (threshold %.1f%%)",
				rate*100, stats.Runs, r.Threshold*100)
	case "cost":
		if stats.TotalCostUSD == nil {
			return false, false, ""
		}
		return *stats.TotalCostUSD > r.Threshold, true,
			fmt.Sprintf("window cost $%.4f (threshold $%.4f)", *stats.TotalCostUSD, r.Threshold)
	case "p95_latency":
		if stats.Runs == 0 {
			return false, false, ""
		}
		return float64(stats.P95DurationMS) > r.Threshold, true,
			fmt.Sprintf("p95 latency %dms (threshold %.0fms)", stats.P95DurationMS, r.Threshold)
	case "assertion_fail_rate":
		for _, a := range stats.AssertionRates {
			if a.Name == r.Config {
				if a.Total == 0 {
					return false, false, ""
				}
				failRate := 1 - float64(a.Passed)/float64(a.Total)
				return failRate > r.Threshold, true,
					fmt.Sprintf("%q fail rate %.1f%% over %d runs (threshold %.1f%%)",
						a.Name, failRate*100, a.Total, r.Threshold*100)
			}
		}
		return false, false, "" // assertion not seen in window
	}
	return false, false, ""
}
