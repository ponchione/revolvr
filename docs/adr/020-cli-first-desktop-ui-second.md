# ADR-020 — CLI First, Desktop UI Second

- Status: Superseded by ADR-025
- Date: 2026-08-03
- Superseded: 2026-08-27 by [ADR-025](025-terminal-first-simplified-harness.md)
- Source: Section 4, “ADR-020 — CLI First, Desktop UI Second,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Historical context

The product needs a complete operational surface before a desktop interface is added.

## Historical decision

Make the core fully operable from the CLI. A desktop UI uses the same application service interfaces and contains no unique business logic. The preferred desktop stack is Wails, Vue 3, and TypeScript.

## Historical consequences

- CLI completeness is required independently of the desktop UI.
- Desktop behavior remains a presentation layer over shared application services.
- Current direction is terminal-first; no desktop UI is planned.
