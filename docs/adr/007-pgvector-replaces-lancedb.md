# ADR-007 — pgvector Replaces LanceDB

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-007 — pgvector Replaces LanceDB,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Retrieval should share the canonical PostgreSQL data platform instead of retaining a separate vector database.

## Decision

Store embedding vectors in PostgreSQL through `pgvector`. Use PostgreSQL full-text search with `pgvector` as the baseline hybrid retrieval system. Do not carry LanceDB into the new architecture.

## Consequences

- Text and vector retrieval use PostgreSQL as their common storage system.
- LanceDB is removed rather than maintained alongside `pgvector`.
