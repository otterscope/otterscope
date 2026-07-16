package store

import (
	"context"
	"testing"
	"time"
)

func TestProjects(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	// Keyless requests always resolve to default.
	if p, ok := st.ProjectForKey(ctx, ""); !ok || p != "default" {
		t.Fatalf("empty key → %q ok=%v", p, ok)
	}

	created, err := st.CreateProject(ctx, "prod")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if len(created.IngestKey) != 32 {
		t.Fatalf("key length %d, want 32 hex chars", len(created.IngestKey))
	}
	if p, ok := st.ProjectForKey(ctx, created.IngestKey); !ok || p != "prod" {
		t.Fatalf("key → %q ok=%v", p, ok)
	}
	if _, ok := st.ProjectForKey(ctx, "bogus"); ok {
		t.Fatal("bogus key must not resolve")
	}
	if _, err := st.CreateProject(ctx, "prod"); err == nil {
		t.Fatal("duplicate project name must fail")
	}

	projects, err := st.ListProjects(ctx)
	if err != nil || len(projects) != 2 {
		t.Fatalf("ListProjects: %v, %d", err, len(projects))
	}
}

func TestSweep(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	if err := st.UpsertSteps(ctx, sampleRun("old", 1000)); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSteps(ctx, sampleRun("new", 5000)); err != nil {
		t.Fatal(err)
	}

	n, err := st.Sweep(ctx, time.Unix(3000, 0))
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if n != 1 {
		t.Fatalf("swept %d runs, want 1", n)
	}
	runs, _ := st.ListRuns(ctx, Filter{}, 10, 0)
	if len(runs) != 1 || runs[0].ID != "new" {
		t.Fatalf("after sweep: %+v", runs)
	}
	var steps int
	st.DB().QueryRowContext(ctx, `SELECT count(*) FROM steps WHERE run_id = 'old'`).Scan(&steps)
	if steps != 0 {
		t.Fatalf("old steps remain: %d", steps)
	}
}

// A step with the same span/trace ID in a different project must NOT
// overwrite the original project's data (audit #49).
func TestCrossProjectIsolation(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()

	a := sampleRun("shared-trace", 1000)
	for i := range a {
		a[i].Project = "alpha"
	}
	if err := st.UpsertSteps(ctx, a); err != nil {
		t.Fatal(err)
	}

	// Same IDs, different project, different tokens.
	b := sampleRun("shared-trace", 5000)
	for i := range b {
		b[i].Project = "beta"
		if b[i].LLM != nil {
			b[i].LLM.InputTokens = 999
		}
	}
	if err := st.UpsertSteps(ctx, b); err != nil {
		t.Fatal(err)
	}

	alpha, _ := st.ListRuns(ctx, Filter{Project: "alpha"}, 10, 0)
	beta, _ := st.ListRuns(ctx, Filter{Project: "beta"}, 10, 0)
	if len(alpha) != 1 || len(beta) != 1 {
		t.Fatalf("expected one run per project, got alpha=%d beta=%d", len(alpha), len(beta))
	}
	if alpha[0].InputTokens == 999 {
		t.Fatal("project beta overwrote project alpha's run")
	}
	if alpha[0].InputTokens != 800 || beta[0].InputTokens != 999 {
		t.Fatalf("token isolation broken: alpha=%d beta=%d", alpha[0].InputTokens, beta[0].InputTokens)
	}

	// Each project's GetRun sees only its own steps.
	_, aSteps, _ := st.GetRun(ctx, "shared-trace")
	if len(aSteps) != 3 {
		t.Fatalf("GetRun mixed steps across projects: %d", len(aSteps))
	}
}
