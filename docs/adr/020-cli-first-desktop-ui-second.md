# ADR-020 — CLI First, Desktop UI Second

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-020 — CLI First, Desktop UI Second,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

The product needs a complete operational surface before a desktop interface is added.

## Decision

Make the core fully operable from the CLI. A desktop UI uses the same application service interfaces and contains no unique business logic. The preferred desktop stack is Wails, Vue 3, and TypeScript.

## Consequences

- CLI completeness is required independently of the desktop UI.
- Desktop behavior remains a presentation layer over shared application services.
