# ADR-021 — REST + Server-Sent Events

- Status: Superseded by ADR-025
- Date: 2026-08-03
- Superseded: 2026-08-27 by [ADR-025](025-terminal-first-simplified-harness.md)
- Source: Section 4, “ADR-021 — REST + Server-Sent Events,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Historical context

Local clients need command, query, and progress interfaces without unnecessary bidirectional streaming complexity.

## Historical decision

Use local REST APIs for commands and queries. Use Server-Sent Events for progress and event streaming unless a concrete requirement proves WebSockets necessary.

## Historical consequences

- REST and SSE are the baseline local API protocols.
- WebSockets remain deferred until an identified requirement cannot be met by SSE.
- Current terminal clients call shared Go application services in-process; no
  local REST/SSE surface is planned.
