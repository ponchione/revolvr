# ADR-017 — Relational Relationship Graph First

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-017 — Relational Relationship Graph First,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Project relationships need a canonical representation before a separate graph system is justified.

## Decision

Store typed project relationships initially in PostgreSQL tables. Defer Graphiti, Neo4j, and FalkorDB to dogfooding experiments.

## Consequences

- The initial relationship graph uses the canonical relational database.
- Separate graph technologies remain deferred dogfooding experiments.
