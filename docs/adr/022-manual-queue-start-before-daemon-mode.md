# ADR-022 — Manual Queue Start Before Daemon Mode

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-022 — Manual Queue Start Before Daemon Mode,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Unattended continuous execution requires reliability and recovery evidence that the initial product does not yet have.

## Decision

Require the operator to explicitly start bounded queue execution. Defer a persistent unattended daemon until evaluation and recovery gates demonstrate reliability.

## Consequences

- Queue runs begin only through an explicit operator action and remain bounded.
- Daemon mode is not implemented until its reliability gates pass.
