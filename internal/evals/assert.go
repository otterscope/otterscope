// Package evals scores runs against user-defined assertions. Results live
// on the run (ADR-0003: evals fused into the trace store, no second
// product).
package evals

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/otterscope/otterscope/internal/model"
)

// Assertion is one per-project check applied to completed runs.
type Assertion struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Name    string `json:"name"`
	// Type: contains | not_contains | regex | is_json | max_latency_ms |
	// max_cost_usd
	Type    string `json:"type"`
	Config  string `json:"config"` // meaning depends on Type
	Enabled bool   `json:"enabled"`
}

// Result of one assertion against one run.
type Result struct {
	AssertionID int64  `json:"assertionId"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Pass        bool   `json:"pass"`
	Detail      string `json:"detail"`
}

// Validate checks that the assertion's type/config combination is usable.
func Validate(a Assertion) error {
	switch a.Type {
	case "contains", "not_contains":
		if a.Config == "" {
			return fmt.Errorf("%s needs a non-empty config string", a.Type)
		}
	case "regex":
		if _, err := regexp.Compile(a.Config); err != nil {
			return fmt.Errorf("bad regex: %w", err)
		}
	case "is_json":
		// no config
	case "max_latency_ms", "max_cost_usd":
		var n float64
		if err := json.Unmarshal([]byte(a.Config), &n); err != nil || n <= 0 {
			return fmt.Errorf("%s needs a positive number config", a.Type)
		}
	default:
		return fmt.Errorf("unknown assertion type %q", a.Type)
	}
	return nil
}

// FinalOutput returns the text an output assertion targets: the last LLM
// step's output messages, concatenated.
func FinalOutput(steps []model.Step) string {
	for i := len(steps) - 1; i >= 0; i-- {
		llm := steps[i].LLM
		if llm == nil || len(llm.OutputMessages) == 0 {
			continue
		}
		var sb strings.Builder
		for _, m := range llm.OutputMessages {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(m.Content)
		}
		return sb.String()
	}
	return ""
}

// Evaluate runs one assertion against a completed run.
func Evaluate(a Assertion, run model.Run, steps []model.Step) Result {
	res := Result{AssertionID: a.ID, Name: a.Name, Type: a.Type}
	switch a.Type {
	case "contains":
		out := FinalOutput(steps)
		res.Pass = strings.Contains(out, a.Config)
		if !res.Pass {
			res.Detail = fmt.Sprintf("output does not contain %q", a.Config)
		}
	case "not_contains":
		out := FinalOutput(steps)
		res.Pass = !strings.Contains(out, a.Config)
		if !res.Pass {
			res.Detail = fmt.Sprintf("output contains forbidden %q", a.Config)
		}
	case "regex":
		re, err := regexp.Compile(a.Config)
		if err != nil {
			res.Detail = "invalid regex"
			return res
		}
		out := FinalOutput(steps)
		res.Pass = re.MatchString(out)
		if !res.Pass {
			res.Detail = fmt.Sprintf("output does not match /%s/", a.Config)
		}
	case "is_json":
		out := strings.TrimSpace(FinalOutput(steps))
		res.Pass = json.Valid([]byte(out)) && out != ""
		if !res.Pass {
			res.Detail = "output is not valid JSON"
		}
	case "max_latency_ms":
		var maxMS float64
		json.Unmarshal([]byte(a.Config), &maxMS)
		got := float64(run.End.Sub(run.Start).Milliseconds())
		res.Pass = got <= maxMS
		if !res.Pass {
			res.Detail = fmt.Sprintf("run took %.0fms > %.0fms", got, maxMS)
		}
	case "max_cost_usd":
		var maxUSD float64
		json.Unmarshal([]byte(a.Config), &maxUSD)
		if run.CostUSD == nil {
			res.Pass = false
			res.Detail = "run cost unknown (unpriced model)"
			return res
		}
		res.Pass = *run.CostUSD <= maxUSD
		if !res.Pass {
			res.Detail = fmt.Sprintf("run cost $%.4f > $%.4f", *run.CostUSD, maxUSD)
		}
	default:
		res.Detail = fmt.Sprintf("unknown type %q", a.Type)
	}
	return res
}
