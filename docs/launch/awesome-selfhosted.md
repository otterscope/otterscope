# awesome-selfhosted submission

> **Read this before submitting.** Their guidelines state: *"Machine/LLM-generated
> contributions, that do not respect project guidelines are not allowed and will
> result in a ban."* Everything below is a starting point that a human must read,
> verify against the live guidelines, and submit. Do not paste it unreviewed.

## Where it goes

**Not** `awesome-selfhosted/awesome-selfhosted`. That repo's `README.md` is
generated. Submissions are a **YAML file** in the data repo:

- Repo: <https://github.com/awesome-selfhosted/awesome-selfhosted-data>
- File: `software/otterscope.yml`
- Guidelines: <https://github.com/awesome-selfhosted/awesome-selfhosted-data/blob/master/CONTRIBUTING.md>
- PR template (extra rules): `.github/PULL_REQUEST_TEMPLATE.md` in that repo

Category: **Generative Artificial Intelligence (GenAI)** — the tag Agenta,
Khoj, LibreChat and the other LLM tooling sits under. In single-page mode a
project shows only under the *first* tag listed, so keep GenAI first.

## When it is eligible

The clock is **four months from the first tagged release**, not from repo
creation. Otterscope's first release, `v0.0.1`, was published **2026-07-14**,
so the earliest submission date is around **2026-11-14**.

One thing to fix before then: every release so far is marked **pre-release**,
so the repo has no "latest release" on GitHub. Their reviewers check for a
tagged release explicitly, and a project showing none gets closed with a
request to cut one. Publish a normal (non-pre-release) version before
submitting.

The Show HN and r/selfhosted posts have no such gate.

## Draft entry

```yaml
name: Otterscope
website_url: https://github.com/otterscope/otterscope
source_code_url: https://github.com/otterscope/otterscope
description: Observability and evals for AI agents — run timelines, token and cost tracking, assertions and LLM-as-judge scoring over OpenTelemetry traces. Single Go binary with embedded SQLite. (alternative to Langfuse, Arize Phoenix)
licenses:
  - Apache-2.0
platforms:
  - Go
  - Docker
tags:
  - Generative Artificial Intelligence (GenAI)
```

Notes on the fields, from their guidelines:

- Descriptions must not say "open-source", "free" or "self-hosted" — the list
  already implies all three.
- Prefer the short form ("Observability and evals for…", not "Otterscope is a…").
- `(alternative to …)` is their convention for positioning against an existing
  product, which is how the README already frames the project.
- `Go` is listed because the project ships a single static binary, and their
  guidelines ask for the language in that case.
- Leave out `stargazers_count`, `updated_at`, `current_release` and
  `commit_history` — their tooling fills those in.
