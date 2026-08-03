# ADR-013 — Disposable Sandboxes

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-013 — Disposable Sandboxes,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Roles that execute commands may alter source or the execution environment and therefore require isolation.

## Decision

Every role that may execute commands operates against a disposable sandbox, including at minimum the implementer, corrector, and verifier. Auditors should normally inspect a read-only source snapshot and evidence without mutation.

## Consequences

- Command execution occurs in disposable environments rather than the trusted control plane or original checkout.
- Auditing is normally read-only and evidence-based.
