# ADR-003 — Remove Shunter Entirely

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-003 — Remove Shunter Entirely,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Shunter is an independent product and must not remain coupled to Revolvr at runtime.

## Decision

Remove Shunter entirely as a Revolvr runtime dependency. Do not port Shunter modules, reducers, protocol clients, generated TypeScript bindings, RPC, snapshots, or subscriptions. Reimplement relevant data and behavior with PostgreSQL transactions, SQL queries, application events, and a simple local streaming API.

## Consequences

- Revolvr owns its persistence, state transitions, events, and local streaming behavior.
- No Shunter runtime or protocol surface is retained in the new architecture.
