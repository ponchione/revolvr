---
id: architecture-011-sandboxd
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-010-sandbox-specification-validator
---

# Implement revolvr-sandboxd

## Sequence and status

- Sequence: `011` of `025`.
- Status: pending.
- Prerequisite: `architecture-010-sandbox-specification-validator`.
- Phase gate: no runtime command is accepted until the typed request validator
  and managed-path resolution boundary pass their abuse tests.

## Primary outcome

Add the minimal trusted local `revolvr-sandboxd` process and one rootless OCI
runtime backend that executes only already-validated sandbox specifications.

## Required reading

- ADR-013 through ADR-015.
- Specification Sections 5-7, 9.17, 12.3-12.4, 16-17,
  29 Phase 3, 37.5, 39.5, 40.6, 47, 56.1-56.3, 58.4, and 60.

## Existing foundations to inspect

- The validator produced by task 010.
- `internal/runner`, `internal/outputcap`, `internal/pathguard`,
  `internal/runtimepath`, and `internal/redact`.
- `cmd/revolvr/main.go` for command conventions; create
  `cmd/revolvr-sandboxd` only because the specification requires it.

## Starting assumptions

- The service runs as the local unprivileged operator and listens on a
  permission-restricted Unix socket.
- Implement one configured rootless Docker or Podman backend, selected from
  actual workstation support; the other backend is deferred.
- The model never contacts the service directly and never chooses raw runtime
  flags.

## Implementation requirements

- Implement the narrow create/exec/stop/inspect/remove lifecycle behind the
  specified `SandboxRuntime` responsibility boundary.
- Authenticate the local socket by filesystem ownership/mode and accept only
  the normalized request from task 010 with bounded framing and deadlines.
- Translate the request into non-root, capability-free,
  `no-new-privileges`, read-only-root, bounded PID/CPU/memory/tmpfs/time,
  network-policy, and approved-mount runtime arguments.
- Never mount a runtime socket, original checkout, host home, unapproved
  device, secret, or arbitrary path.
- Record exact image digest, runtime/profile, command, resource/network
  settings, timestamps, exit/timeout/cancel status, and bounded output artifact
  references.
- Make stop/remove and restart orphan reconciliation idempotent; force cleanup
  at timeout/cancellation without broad deletion.

## Scope boundaries and non-goals

- Do not implement both OCI backends, remote control, TCP exposure, GPU worker
  access, daemonized autonomous queues, or model calls.
- Do not add gVisor fallback that silently changes the recorded profile.
- Do not implement workspace creation; task 012 owns worktrees.

## Acceptance criteria

- A validated fixture runs rootlessly and records complete lifecycle evidence.
- The container cannot read host home/test secrets, reach the runtime socket,
  write outside its workspace, or use network under `none`.
- Timeout, cancellation, process-tree cleanup, malformed protocol, client
  disconnect, and restart reconciliation are deterministic and leak no active
  container.
- Strict-profile unavailability is a typed failure, never a compatible-mode
  fallback.
- Unit/fake-runtime, opt-in real-runtime security, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go test ./internal/sandbox ./cmd/revolvr-sandboxd
REVOLVR_SANDBOX_INTEGRATION=1 go test ./internal/sandbox -run TestRootlessRuntimeSecurityProfile
go test ./...
git diff --check
```

## Expected completion report

Report the chosen first backend and why, socket boundary, enforced isolation
flags, lifecycle evidence, host-access/network/timeout/cancel/orphan results,
changed files, and test results.
