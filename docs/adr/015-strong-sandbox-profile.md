# ADR-015 — Strong Sandbox Profile

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-015 — Strong Sandbox Profile,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Sandbox isolation must be explicit, evidence-bearing, and adaptable when stronger isolation is incompatible with a workload.

## Decision

Support these sandbox profiles:

- `strict`: rootless runtime plus gVisor when compatible, with no network by default.
- `compatible`: hardened rootless standard OCI runtime.
- `diagnostic`: explicitly attended and less restrictive; it is never selected silently.

Include the selected profile in run identity and evidence.

## Consequences

- Isolation strength and network posture are explicit for every run.
- Less restrictive diagnostic execution always requires attended, explicit selection.
