package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/otterscope/otterscope/internal/model"
)

// benchRun builds one realistic run (agent + llm-with-message + tool) with a
// unique id derived from i.
func benchRun(i int) []model.Step {
	id := fmt.Sprintf("run%029d", i)
	base := time.Unix(1_700_000_000+int64(i), 0)
	return []model.Step{
		{ID: id + "r", RunID: id, Project: "default", Kind: model.StepAgent,
			Name: "invoke_agent support", Service: "support-agent", AgentName: "support",
			Status: model.StatusOK, Start: base, End: base.Add(5 * time.Second)},
		{ID: id + "l", RunID: id, ParentID: id + "r", Project: "default", Kind: model.StepLLM,
			Name: "chat claude-sonnet-5", Status: model.StatusOK,
			Start: base.Add(time.Second), End: base.Add(3 * time.Second),
			LLM: &model.LLMCall{RequestModel: "claude-sonnet-5", InputTokens: 800, OutputTokens: 120,
				OutputMessages: []model.Message{{Role: "assistant", Content: "Your order A-1042 has shipped."}}}},
		{ID: id + "t", RunID: id, ParentID: id + "r", Project: "default", Kind: model.StepTool,
			Name: "execute_tool lookup_order", Status: model.StatusOK,
			Start: base.Add(3 * time.Second), End: base.Add(4 * time.Second),
			Tool: &model.ToolCall{Name: "lookup_order", Arguments: `{"id":"A-1042"}`}},
	}
}

// seedRuns ingests n runs in batches (outside any active benchmark timer).
func seedRuns(tb testing.TB, st *Store, n int) {
	ctx := context.Background()
	const perBatch = 1000
	buf := make([]model.Step, 0, perBatch*3)
	for i := 0; i < n; i++ {
		buf = append(buf, benchRun(i)...)
		if len(buf) >= perBatch*3 {
			if err := st.UpsertSteps(ctx, buf); err != nil {
				tb.Fatal(err)
			}
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		if err := st.UpsertSteps(ctx, buf); err != nil {
			tb.Fatal(err)
		}
	}
}

func BenchmarkIngest(b *testing.B) {
	st := openTest(b)
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := st.UpsertSteps(ctx, benchRun(i)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	// 3 spans per run.
	b.ReportMetric(float64(b.N)*3/b.Elapsed().Seconds(), "spans/s")
}

func benchQuery(b *testing.B, name string, run func(*Store)) {
	for _, n := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("%s/%d", name, n), func(b *testing.B) {
			st := openTest(b)
			seedRuns(b, st, n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				run(st)
			}
		})
	}
}

func BenchmarkListRuns(b *testing.B) {
	ctx := context.Background()
	benchQuery(b, "list", func(st *Store) {
		if _, err := st.ListRuns(ctx, Filter{}, 50, 0); err != nil {
			b.Fatal(err)
		}
	})
}

func BenchmarkGetStats(b *testing.B) {
	ctx := context.Background()
	benchQuery(b, "stats", func(st *Store) {
		if _, err := st.GetStats(ctx, Filter{}); err != nil {
			b.Fatal(err)
		}
	})
}

func BenchmarkSearch(b *testing.B) {
	ctx := context.Background()
	benchQuery(b, "search", func(st *Store) {
		if _, err := st.ListRuns(ctx, Filter{Query: "shipped"}, 50, 0); err != nil {
			b.Fatal(err)
		}
	})
}
