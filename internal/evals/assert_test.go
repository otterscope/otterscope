package evals

import (
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

func run(costUSD float64, durMS int64) model.Run {
	start := time.Unix(1000, 0)
	return model.Run{
		Start:   start,
		End:     start.Add(time.Duration(durMS) * time.Millisecond),
		CostUSD: &costUSD,
	}
}

func stepsWithOutput(out string) []model.Step {
	return []model.Step{
		{Kind: model.StepLLM, LLM: &model.LLMCall{
			OutputMessages: []model.Message{{Role: "assistant", Content: out}},
		}},
	}
}

func TestEvaluateTable(t *testing.T) {
	tests := []struct {
		name  string
		a     Assertion
		run   model.Run
		steps []model.Step
		pass  bool
	}{
		{"contains pass", Assertion{Type: "contains", Config: "hello"}, run(0.1, 100), stepsWithOutput("well hello there"), true},
		{"contains fail", Assertion{Type: "contains", Config: "hello"}, run(0.1, 100), stepsWithOutput("goodbye"), false},
		{"not_contains pass", Assertion{Type: "not_contains", Config: "I cannot"}, run(0.1, 100), stepsWithOutput("sure, done"), true},
		{"not_contains fail", Assertion{Type: "not_contains", Config: "I cannot"}, run(0.1, 100), stepsWithOutput("I cannot help"), false},
		{"regex pass", Assertion{Type: "regex", Config: `#\d+`}, run(0.1, 100), stepsWithOutput("see ticket #42"), true},
		{"regex fail", Assertion{Type: "regex", Config: `#\d+`}, run(0.1, 100), stepsWithOutput("no ticket"), false},
		{"is_json pass", Assertion{Type: "is_json"}, run(0.1, 100), stepsWithOutput(`{"ok": true}`), true},
		{"is_json fail", Assertion{Type: "is_json"}, run(0.1, 100), stepsWithOutput("not json"), false},
		{"latency pass", Assertion{Type: "max_latency_ms", Config: "500"}, run(0.1, 100), nil, true},
		{"latency fail", Assertion{Type: "max_latency_ms", Config: "500"}, run(0.1, 900), nil, false},
		{"cost pass", Assertion{Type: "max_cost_usd", Config: "0.5"}, run(0.1, 100), nil, true},
		{"cost fail", Assertion{Type: "max_cost_usd", Config: "0.05"}, run(0.1, 100), nil, false},
		{"multi-llm uses last output", Assertion{Type: "contains", Config: "final"},
			run(0.1, 100),
			append(stepsWithOutput("draft"), stepsWithOutput("final answer")...), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Evaluate(tc.a, tc.run, tc.steps); got.Pass != tc.pass {
				t.Errorf("pass = %v, want %v (detail: %s)", got.Pass, tc.pass, got.Detail)
			}
		})
	}
}

func TestCostUnknownFails(t *testing.T) {
	r := model.Run{Start: time.Unix(1000, 0), End: time.Unix(1001, 0)}
	res := Evaluate(Assertion{Type: "max_cost_usd", Config: "1"}, r, nil)
	if res.Pass || res.Detail == "" {
		t.Fatalf("unknown cost must fail with detail: %+v", res)
	}
}

func TestValidate(t *testing.T) {
	good := []Assertion{
		{Type: "contains", Config: "x"},
		{Type: "regex", Config: `\d+`},
		{Type: "is_json"},
		{Type: "max_latency_ms", Config: "1000"},
	}
	for _, a := range good {
		if err := Validate(a); err != nil {
			t.Errorf("%s: %v", a.Type, err)
		}
	}
	bad := []Assertion{
		{Type: "nope"},
		{Type: "contains", Config: ""},
		{Type: "regex", Config: "("},
		{Type: "max_cost_usd", Config: "-1"},
	}
	for _, a := range bad {
		if err := Validate(a); err == nil {
			t.Errorf("%s should be invalid", a.Type)
		}
	}
}
