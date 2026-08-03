# ADR-006 — SQL Migrations

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-006 — SQL Migrations,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Database evolution needs an inspectable, repeatable path for new and existing installations.

## Decision

Use a SQL-first migration tool such as Goose. Migrations must be ordered, reversible when reasonably possible, tested against an empty database and the previous released schema, and included in backup and restore validation.

## Consequences

- Migration verification covers both fresh installation and released-schema upgrades.
- Backup and restore checks include schema evolution.
