# ADR-0005: No hosted or paid offering — Otterscope is self-host only

Date: 2026-08-21 · Status: accepted · Amends: ADR-0003

## Context
ADR-0003 left a door open: "Monetization is explicitly out of scope pre-1.0. If ever pursued: optional managed hosting." That hedge produced a milestone (M9 — Hosted (optional, paid)) and four open issues — managed multi-tenant hosting, Stripe billing and plan tiers, hosted MCP + team dashboards, and a demo instance framed as a paid funnel.

Intent doesn't reach a reader; the tracker does. Anyone evaluating Otterscope sees the issue list before they see an ADR, and an open "Billing + plan tiers (Stripe)" issue reads as open-core with an upsell in flight. That is precisely the Phoenix objection ADR-0003 cites as an adoption blocker. The hedge cost us the positioning it was meant to preserve, while buying nothing: no hosting work was in progress, and the self-hosted segment ADR-0003 targets has low willingness to pay anyway.

The hosting lift is also architecturally hostile to ADR-0001. Multi-tenancy, signup, provisioning, and usage metering are exactly the "needs a different design" pressure the single-static-binary rule exists to refuse.

## Decision
- **There is no paid tier, no usage metering, no seat fees, and no managed hosting** — not pre-1.0, not post-1.0. Otterscope is software you run.
- Milestone M9 is closed; issues #69, #70 and #72 are closed as not planned.
- A public demo instance (#71) stays, scoped as adoption and documentation: read-only sample data, a live link for the README and launch posts. It is not a funnel and gains no signup, tenancy, or billing.
- Features that happen to matter to teams (auth, RBAC, MCP) are judged on whether a self-hoster needs them, never on whether they could be sold. They ship in the OSS binary or not at all.
- Reversing this requires a new ADR that supersedes it, not an issue.

## Consequences
- The project has no revenue path. Accepted, and now stated plainly rather than deferred — sponsorship or support work would need its own ADR and would not gate features.
- Contributors can read the tracker and see a project they can build on without a rug-pull risk; "no paid tier" becomes a claim the README can make unconditionally.
- Any future proposal whose main argument is "this enables hosting" is out of scope by default.
