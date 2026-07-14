# ADR-0003: Apache-2.0 license, OSS-adoption-first positioning

Date: 2026-07-14 · Status: accepted

## Context
Phoenix's ELv2 license and upsell funnel is a documented adoption objection; Langfuse's MIT license is part of why it's the "safe default." The self-hosted community (our primary channel: r/selfhosted, awesome-selfhosted, Show HN) strongly prefers OSI licenses. Willingness to pay in this segment is low; the play is adoption first.

## Decision
- **Apache-2.0** (OSI-approved, patent grant, credible for infra).
- Positioning follows Plausible's honesty model: your disk, unlimited retention, no unit metering, no seat fees. All core functionality (ingest, run explorer, evals) stays open.
- Monetization is explicitly out of scope pre-1.0. If ever pursued: optional managed hosting — never features clawed back from OSS.

## Consequences
- Anyone (including competitors) may commercialize the code; accepted — distribution and trust are the scarce assets here.
- No CLA for now; DCO-style sign-off not required. Revisit only if a legal need emerges.
