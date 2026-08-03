# ADR-021 — REST + Server-Sent Events

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-021 — REST + Server-Sent Events,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Local clients need command, query, and progress interfaces without unnecessary bidirectional streaming complexity.

## Decision

Use local REST APIs for commands and queries. Use Server-Sent Events for progress and event streaming unless a concrete requirement proves WebSockets necessary.

## Consequences

- REST and SSE are the baseline local API protocols.
- WebSockets remain deferred until an identified requirement cannot be met by SSE.
