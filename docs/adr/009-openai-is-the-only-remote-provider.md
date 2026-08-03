# ADR-009 — OpenAI Is the Only Remote Provider

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-009 — OpenAI Is the Only Remote Provider,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

The initial product needs remote model access without inheriting broad multi-provider machinery.

## Decision

Implement only OpenAI as the initial remote provider. Keep the provider abstraction narrow enough to avoid hard coupling, but do not carry forward broad multi-provider complexity from Sodoryard.

## Consequences

- Initial provider work and testing cover OpenAI only.
- The provider boundary stays narrow without speculative implementations for other providers.
