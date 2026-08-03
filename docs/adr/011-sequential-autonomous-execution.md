# ADR-011 — Sequential Autonomous Execution

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-011 — Sequential Autonomous Execution,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Concurrent source mutation would add coordination and recovery complexity to autonomous execution.

## Decision

Do not run workers in parallel. A queue may hold many tasks, but only one source-mutating task is active at a time.

## Consequences

- Scheduling and recovery assume sequential source mutation.
- Queue capacity does not imply concurrent worker execution.
