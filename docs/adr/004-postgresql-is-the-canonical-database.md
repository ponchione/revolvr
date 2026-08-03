# ADR-004 — PostgreSQL Is the Canonical Database

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-004 — PostgreSQL Is the Canonical Database,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Canonical application state requires one production database and one persistence model.

## Decision

Use PostgreSQL as the sole canonical application database. SQLite is not a supported production backend for the new architecture. Tests may use ephemeral PostgreSQL containers; do not create a second SQLite implementation for test convenience.

## Consequences

- Production and integration tests exercise PostgreSQL semantics.
- Revolvr does not maintain parallel PostgreSQL and SQLite persistence implementations.
