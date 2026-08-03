# ADR-024 — Verification Is Host-Owned

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-024 — Verification Is Host-Owned,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

An implementing agent must not be able to redefine the checks that grant acceptance to its own work.

## Decision

Admit verification commands and acceptance methods before execution. Editing tests, scripts, or verification definitions does not grant the implementing agent new authority. Detect changes to verification authority and either reject or escalate them.

## Consequences

- The trusted host owns the verification boundary used for acceptance.
- Worker changes to verification material cannot silently weaken or replace admitted authority.
