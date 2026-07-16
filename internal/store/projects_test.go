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
