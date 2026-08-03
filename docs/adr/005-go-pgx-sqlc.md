# ADR-005 — Go + pgx + sqlc

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-005 — Go + pgx + sqlc,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Persistence code should stay explicit, inspectable, and aligned with PostgreSQL.

## Decision

Use Go, `pgx`, `sqlc`, and SQL-first migrations for application persistence. Keep queries explicit and inspectable. A narrowly contained handwritten query layer may serve dynamic or highly specialized search queries when `sqlc` is unsuitable, but it is exceptional.

## Consequences

- SQL remains the primary persistence interface and generated query code comes from `sqlc`.
- Handwritten query machinery is limited to cases that `sqlc` cannot reasonably express.
