# ADR-001 — Product Name

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-001 — Product Name,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

The product needs a stable name while its repository migration strategy remains a separate choice.

## Decision

The product name remains **Revolvr**. Implementation may continue in the current Revolvr repository or begin from a clean branch or rewrite; the architecture does not depend on repository history.

## Consequences

- Product and architecture documentation use the Revolvr name.
- The Phase 0 choice between in-place evolution and a clean branch or rewrite remains open.
