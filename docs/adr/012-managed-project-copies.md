# ADR-012 — Managed Project Copies

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-012 — Managed Project Copies,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Autonomous work should not mutate the operator's original checkout directly.

## Decision

Use a Revolvr-managed Git mirror or clone with ephemeral worktrees as the safest default project mode. Do not bind-mount the operator's original checkout read-write into worker containers. Successful work becomes a commit in the managed project repository, an exportable patch, or a branch that the operator can explicitly push or apply.

## Consequences

- Worker mutations are isolated from the original checkout.
- Moving successful work outside the managed repository requires an explicit export, push, or apply action.
