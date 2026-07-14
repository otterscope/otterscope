# Workflow: executing an issue or feature

This is the operating procedure for all work on Otterscope, whether done by a human or an AI agent. Its purpose is to keep velocity high without accumulating tech debt: every change is traceable to an issue, verified before merge, and reflected in the docs that describe the system.

## 1. Before writing code

1. **Work only from a GitHub issue.** If the work has no issue, create one first with: problem statement, acceptance criteria, and which milestone it belongs to. Unplanned work discovered mid-task becomes a *new issue*, not scope creep on the current one.
2. **Read the context**: `CLAUDE.md`, the relevant ADRs in `docs/adr/`, and the code you're about to touch. If the issue contradicts an ADR, resolve that first (new ADR or issue comment), don't code around it.
3. **State the plan on the issue** (a short comment): approach, files touched, how it will be verified. This is the tech-debt checkpoint — if the plan needs a hack, say so explicitly and file the follow-up issue *before* merging the hack.

## 2. While coding

4. **Branch per issue**: `feat/<issue-number>-short-slug` or `fix/<issue-number>-short-slug` off `main`. `main` stays green and releasable at all times.
5. **Tests are part of the change, not a follow-up.** New behavior → new test. Bug fix → regression test that fails without the fix. Ingest changes → a captured-payload fixture in `internal/ingest/testdata/`.
6. **Keep the diff scoped to the issue.** Drive-by refactors go in their own issue/PR unless they're trivially small and directly enable the change.
7. **New dependency? Justify it** in the commit message (what it does, why stdlib can't, maintenance status).

## 3. Before merging

8. **Verify end-to-end, not just unit tests**: build the binary, run `otterscope serve`, exercise the changed behavior for real (send an OTLP payload, click through the UI change). Record what you did in the PR description ("Verified: …").
9. **Quality gate**: `go build ./... && go vet ./... && go test ./...` (and `cd web && npm run build && npm test` if the frontend changed) must pass locally and in CI.
10. **Update the paper trail in the same PR**: ADR if an architectural decision was made, `docs/ROADMAP.md` if scope/timeline shifted, `CLAUDE.md` if the architecture map changed, README if user-facing behavior changed.
11. **PR → squash-merge to `main`**, referencing the issue (`Closes #N`). Delete the branch.

## 4. After merging

12. **Close the loop**: confirm the issue auto-closed, move follow-up items into their own issues with milestone labels.
13. **Never leave `main` broken.** If CI fails post-merge, fixing it preempts all other work.

## Tech-debt policy

- Debt is only acceptable when *recorded*: a `debt`-labeled issue linked from a `// TODO(#issue):` comment at the site. TODO comments without an issue number fail review.
- Every 4th milestone-week includes a debt-repayment pass: pick the top `debt` issues and clear them before new features.
- The ingest normalization layer is expected churn (upstream OTel GenAI conventions are unstable) — quirks go in `internal/ingest` with fixtures, never patched downstream.

## Releases

- Tag `vX.Y.Z` on `main`; CI builds static binaries (linux/amd64, linux/arm64, darwin/arm64, windows/amd64) and the Docker image.
- Pre-1.0: minor = features, patch = fixes. Breaking config/API changes need a migration note in the release notes.
