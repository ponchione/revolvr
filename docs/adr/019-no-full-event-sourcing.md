# ADR-019 — No Full Event Sourcing

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-019 — No Full Event Sourcing,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Revolvr needs durable audit history without making event replay the foundation of all application state.

## Decision

Use ordinary current-state tables plus an append-only event and audit table. Do not rebuild the system around full event sourcing. State transitions atomically update current state and append the associated durable event.

## Consequences

- Current state is queried directly from relational tables.
- Every state transition retains a durable event in the same atomic operation.
