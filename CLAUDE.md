# Otterscope — agent working guide

Lightweight, self-hosted observability + evals for AI agents. One Go static binary, embedded SQLite, embedded web UI. Positioning: "Plausible for AI agents" — the anti-ClickHouse-stack. Read `docs/ROADMAP.md` for milestones and `docs/adr/` for decisions already made (do not relitigate them silently — write a new ADR to change one).

## Architecture

- `cmd/otterscope/` — CLI entrypoint (`serve` is the main command).
- `internal/ingest/` — OTLP/HTTP receiver (protobuf + JSON) and GenAI-dialect normalization (OTel gen_ai old + experimental, OpenInference). Normalization maps raw spans → domain model; it is the most churn-prone area (upstream conventions are unstable) so keep it isolated behind `internal/model`.
- `internal/model/` — the domain: Run, Step, LLMCall, ToolCall, Eval, Score. Everything downstream depends on this, not on OTel types.
- `internal/store/` — SQLite persistence (modernc.org/sqlite, CGO-free). Migrations in `internal/store/migrations/`, embedded, sequential, append-only.
- `internal/server/` — HTTP API for the UI + embedded static frontend via `go:embed`.
- `web/` — Vite + React + TypeScript frontend. Built output is embedded into the binary; never served from disk in production.

## Hard rules

- **The binary stays single and static.** No CGO, no external services, no sidecar dependencies. Any feature that "needs Redis/Postgres" needs a different design instead.
- **OTel types never leak past `internal/ingest`.** Downstream code sees only `internal/model`.
- **SQLite schema changes only via new migration files.** Never edit an existing migration.
- **No breaking changes to the ingest endpoint** once released; agents in the wild must not lose data because we refactored.
- Dependencies are a liability: prefer stdlib; justify each new Go module in the PR/commit description.

## Development

- Go 1.26+ (installed at `~/.local/go/bin`), Node 24 for `web/`.
- Build: `go build ./...` · Test: `go test ./...` · Lint: `go vet ./...` (golangci-lint if installed).
- Frontend: `cd web && npm run build` produces `web/dist`, embedded by the server package.
- Run locally: `go run ./cmd/otterscope serve` then POST OTLP to `:4318`, UI on `:8317`.
- Test fixtures for ingest live in `internal/ingest/testdata/` as captured OTLP JSON payloads from real frameworks — extend these whenever a new framework/dialect quirk is found.
- The store runs on a SINGLE SQLite connection: never hold a rows cursor open while issuing another query/write on the same call path — it self-deadlocks. Drain and close cursors first (see `EachRawBatch`).
- Any command whose exit code gates a decision (tests, builds, `gh pr checks`, `gh run watch`) must not be piped through `tail`/`grep`/etc. without `set -o pipefail` — a pipe masks the failure and the gate silently passes (this has bitten twice: PR #21 locally, PR #30 at merge). Always run tests with an explicit `-timeout` well under 10m so hangs fail fast.
- CI check failures aren't always code failures — module-proxy flakes happen (PR #30). Re-run the failed job (`gh run rerun <id> --failed`) before debugging, but never merge on a red check regardless of suspected cause.

## Workflow

Every unit of work follows `docs/WORKFLOW.md` — read it before starting any issue/feature. Summary: work is issue-driven, one branch per issue, tests + dry-run verification before merge, ADR for any architectural decision, roadmap kept truthful.
