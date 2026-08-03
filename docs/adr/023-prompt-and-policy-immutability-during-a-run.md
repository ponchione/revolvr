# ADR-023 — Prompt and Policy Immutability During a Run

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-023 — Prompt and Policy Immutability During a Run,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

A run must remain attributable to the exact instructions and authority admitted at its start.

## Decision

Pin the exact prompt version, tool schema version, policy version, and task version for each run. A worker may not alter these inputs during that run.

## Consequences

- Run identity includes immutable versions of all model instructions and authority inputs.
- Changes to those inputs require a different run rather than mutating an active one.
