// Package sample seeds realistic demo data: several services and models,
// tool loops, an error run, and assertion results — enough to make every
// screen meaningful on first launch.
package sample

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/otterscope/otterscope/internal/evals"
	"github.com/otterscope/otterscope/internal/model"
	"github.com/otterscope/otterscope/internal/pricing"
	"github.com/otterscope/otterscope/internal/store"
)

type flavor struct {
	service, agent, model, provider string
	tools                           []string
}

var flavors = []flavor{
	{"support-bot", "triage", "claude-sonnet-5", "anthropic", []string{"lookup_order", "search_kb", "escalate"}},
	{"research-agent", "deep-research", "claude-fable-5", "anthropic", []string{"web_search", "fetch_page", "summarize_doc"}},
	{"checkout-assistant", "cart-helper", "gpt-5.4-mini", "openai", []string{"get_cart", "apply_coupon"}},
}

// Seed inserts n sample runs spread over the past 7 days plus two demo
// assertions with results.
func Seed(ctx context.Context, st *store.Store, n int) error {
	rng := rand.New(rand.NewSource(42)) // deterministic demo data
	prices := pricing.Default()

	fast, err := st.CreateAssertion(ctx, evals.Assertion{
		Name: "under-30s", Type: "max_latency_ms", Config: "30000", Enabled: true,
	})
	if err != nil {
		return err
	}
	polite, err := st.CreateAssertion(ctx, evals.Assertion{
		Name: "no-refusals", Type: "not_contains", Config: "I cannot help", Enabled: true,
	})
	if err != nil {
		return err
	}

	now := time.Now()
	for i := 0; i < n; i++ {
		fl := flavors[rng.Intn(len(flavors))]
		start := now.Add(-time.Duration(rng.Int63n(7*24)) * time.Hour).
			Add(-time.Duration(rng.Int63n(3600)) * time.Second)
		failed := rng.Float64() < 0.12

		steps := buildRun(rng, fmt.Sprintf("%032x", rng.Uint64()), fl, start, failed, prices)
		if err := st.UpsertSteps(ctx, steps); err != nil {
			return err
		}

		runID := steps[0].RunID
		results := []evals.Result{
			{AssertionID: fast.ID, Pass: rng.Float64() > 0.1, Detail: ""},
			{AssertionID: polite.ID, Pass: !failed || rng.Float64() > 0.5},
		}
		if err := st.SaveAssertionResults(ctx, runID, results); err != nil {
			return err
		}
	}
	return nil
}

func buildRun(rng *rand.Rand, traceID string, fl flavor, start time.Time, failed bool, prices *pricing.Table) []model.Step {
	rootID := fmt.Sprintf("%016x", rng.Uint64())
	cursor := start
	steps := []model.Step{{
		ID: rootID, RunID: traceID, Kind: model.StepAgent,
		Name: "invoke_agent " + fl.agent, Service: fl.service, AgentName: fl.agent,
		Status: model.StatusOK, Start: start,
	}}

	turns := 1 + rng.Intn(3)
	for t := 0; t < turns; t++ {
		in := int64(400 + rng.Intn(4000))
		out := int64(50 + rng.Intn(800))
		llm := &model.LLMCall{
			Provider: fl.provider, RequestModel: fl.model, ResponseModel: fl.model,
			InputTokens: in, OutputTokens: out,
			CacheReadTokens: int64(float64(in) * rng.Float64() * 0.7),
			// Two prompt versions in the mix so the compare view has an axis.
			Prompt: fl.agent + " v" + map[bool]string{true: "3", false: "2"}[rng.Float64() < 0.5],
			InputMessages: []model.Message{
				{Role: "user", Content: sampleQuestion(rng)},
			},
			OutputMessages: []model.Message{
				{Role: "assistant", Content: sampleAnswer(rng, t == turns-1, failed)},
			},
		}
		if usd, ok := prices.Cost(fl.model, llm.InputTokens, llm.OutputTokens, llm.CacheReadTokens, 0); ok {
			llm.CostUSD = &usd
		}
		dur := time.Duration(800+rng.Intn(4000)) * time.Millisecond
		steps = append(steps, model.Step{
			ID: fmt.Sprintf("%016x", rng.Uint64()), RunID: traceID, ParentID: rootID,
			Kind: model.StepLLM, Name: "chat " + fl.model, Service: fl.service,
			Status: model.StatusOK, Start: cursor.Add(200 * time.Millisecond),
			End: cursor.Add(200*time.Millisecond + dur), LLM: llm,
		})
		cursor = cursor.Add(dur + 400*time.Millisecond)

		// Failed runs must emit the final tool step that carries the error.
		if t < turns-1 || (failed && t == turns-1) || rng.Float64() < 0.5 {
			tool := fl.tools[rng.Intn(len(fl.tools))]
			tdur := time.Duration(100+rng.Intn(1500)) * time.Millisecond
			st := model.Step{
				ID: fmt.Sprintf("%016x", rng.Uint64()), RunID: traceID, ParentID: rootID,
				Kind: model.StepTool, Name: "execute_tool " + tool, Service: fl.service,
				Status: model.StatusOK, Start: cursor, End: cursor.Add(tdur),
				Tool: &model.ToolCall{
					Name: tool, CallID: fmt.Sprintf("call_%04x", rng.Uint32()),
					Arguments: `{"query": "order A-1042"}`, Result: `{"status": "ok"}`,
				},
			}
			if failed && t == turns-1 {
				st.Status = model.StatusError
				st.Error = "tool timeout after 3 retries"
			}
			steps = append(steps, st)
			cursor = cursor.Add(tdur + 200*time.Millisecond)
		}
	}

	steps[0].End = cursor.Add(300 * time.Millisecond)
	return steps
}

func sampleQuestion(rng *rand.Rand) string {
	qs := []string{
		"Where is my order A-1042? It was supposed to arrive Tuesday.",
		"Summarize the latest research on battery recycling.",
		"Can I apply two coupons to the same cart?",
		"My invoice shows a charge I don't recognize.",
	}
	return qs[rng.Intn(len(qs))]
}

func sampleAnswer(rng *rand.Rand, final, failed bool) string {
	if failed && final {
		return "I cannot help with that right now — an internal tool keeps timing out."
	}
	if !final {
		return "Let me look that up for you."
	}
	as := []string{
		"Your order shipped yesterday and arrives tomorrow before noon. Tracking: 1Z-99XY.",
		"Three recent studies stand out; the 2026 Nature paper reports 94% lithium recovery.",
		"Only one coupon per cart, but COMBO10 covers both discounts — I've applied it.",
		"That charge is the annual plan renewal from March 3rd; I've emailed the invoice.",
	}
	return as[rng.Intn(len(as))]
}
