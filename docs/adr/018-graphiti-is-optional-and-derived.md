# ADR-018 — Graphiti Is Optional and Derived

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-018 — Graphiti Is Optional and Derived,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

A temporal knowledge projection may improve retrieval, but it must not compete with canonical lifecycle state.

## Decision

A future Graphiti integration may consume accepted documents, tasks, runs, findings, and decisions to produce a temporal knowledge projection. It may improve retrieval but may never become canonical lifecycle authority.

## Consequences

- Any Graphiti projection is derived from accepted canonical inputs and may only improve retrieval.
- Lifecycle authorization and state remain outside Graphiti.
- ADR-025 keeps Graphiti deferred unless real usage proves a concrete gap in
  the existing brain; no implementation is currently planned.
