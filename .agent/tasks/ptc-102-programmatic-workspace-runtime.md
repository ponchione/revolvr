---
id: ptc-102-programmatic-workspace-runtime
status: superseded
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-101-trajectory-manifest-query-service
---

# Add the pinned programmatic workspace protocol and sandbox runtime

## Supersession

- Superseded on 2026-08-27 by ADR-025. This task is terminal and not runnable.
- Direct tools remain the default; no measured dogfood failure and small
  prototype justify a custom Python workspace.

Everything below is retained as historical proposal context, not current
instruction.

## Sequence and status

- Sequence: PTC-102 after PTC-101.
- Status: blocked and deferred.
- Phase gate: PTC-101 and its post-core dogfood gate must be complete; the
  exact rootless sandbox image/runtime identity and security evidence must be
  available. Missing evidence produces deferral.

## Primary outcome

Start, execute, inspect, cancel, and stop one task/run-scoped Python workspace
inside the existing disposable rootless worker sandbox using a pinned bounded
host-owned protocol.

## Required behavior

- Use a custom versioned framed JSON protocol over container-local stdio or a
  private Unix socket; expose no host-public service and do not require Jupyter
  Server or ZeroMQ.
- Bind project, task/version, run, worker occurrence, role, plan/step,
  workspace/source, image/profile, runtime/Python version, context, trajectory,
  and exact empty/inactive initial skill-set identities.
- Preserve transient globals only within one admitted occurrence; recovery
  starts a fresh pinned runtime and never claims transient state survived.
- Bound frames, executions, wall time, CPU, memory, PIDs, tmpfs, output, and
  total artifacts. Timeout/cancellation kills descendants and removes the
  disposable container with typed evidence.
- Admit Python only inside the existing disposable rootless sandbox with no
  network, host/original-checkout/database/runtime/model credentials, home
  directory, runtime socket, or ambient environment.
- Prohibit runtime package installation, remote code fetch, arbitrary process
  snapshots, and pickle authority.

## Authority and scope boundaries

- The host remains authoritative for model calls, lifecycle, policy, source,
  verification, evidence, and completion. Python is untrusted computation and
  cannot mutate canonical state.
- Keep execution sequential and add no skills, durable scratch, `python_exec`
  worker registry entry, refinement, Graphiti, or automatic activation.

## Acceptance criteria

- Deterministic protocol fixtures prove lifecycle, transient generation,
  bounded inline/artifact results, stale identity refusal, malformed/oversized
  frames, Python error, runtime loss, timeout, cancellation, and cleanup.
- Host/database/runtime/model credentials, original checkout, home, sockets,
  disallowed network, and package installation are unreachable in the approved
  rootless sandbox fixture.
- Crash/restart creates a new occurrence with exact prior trajectory evidence
  and no false process-state recovery.
- Focused sandbox/runtime and full Go tests pass with a pinned reproducible
  runtime identity and no live model call.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go test ./internal/programmatic ./internal/sandbox
go test ./...
git diff --check
```

Record the separate opt-in real rootless runtime security command and exact
image/Python/protocol identities; deterministic fixtures remain mandatory.

## Expected completion report

Report the admitted phase-gate evidence, protocol/runtime/image identities,
rootless isolation proof, lifecycle and recovery outcomes, every resource and
credential denial, dependencies/image changes, and tests.
