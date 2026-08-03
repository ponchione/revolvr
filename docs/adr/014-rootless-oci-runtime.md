# ADR-014 — Rootless OCI Runtime

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-014 — Rootless OCI Runtime,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Disposable sandboxes require a container runtime boundary without privileged daemon assumptions.

## Decision

Use a rootless container engine. Abstract Docker and Podman behind a small `SandboxRuntime` interface, and implement one backend first.

## Consequences

- Sandbox execution relies on rootless OCI containers.
- The first release needs one runtime backend, while the interface preserves compatibility with Docker and Podman.
