# Agent Handoff

Updated: 2026-08-04

## Where We Stopped

- Architecture task 011 is complete at implementation commit
  `6d1c72edd34ccc7c9d1968a6390249fdec36fdac`.
- `internal/sandbox` now provides the narrow `SandboxRuntime` lifecycle, a
  fail-closed rootless Docker backend, descriptor identity rechecks, bounded
  lifecycle evidence/artifacts, and the permission-restricted framed Unix
  socket used by `cmd/revolvr-sandboxd`.
- The opt-in gate passed against a disposable Docker 29.7.1 rootless daemon
  and pinned `alpine:3.22` digest. It proved the required host-access, socket,
  network, read-only-root, timeout, cancellation, leak, and orphan controls.
  The daemon and its isolated temporary runtime/data root were removed.
- Formatting, focused tests, the real rootless integration, sandbox race
  tests, `go test ./...`, CLI help, and `git diff --check` pass.
- Tasks 001-011 are completed with Git provenance in their task files. Task
  012 was not started; tasks 012-025 remain pending.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-012-workspace-lifecycle.md`.

Read `AGENTS.md`, `README.md`, this handoff, the canonical specification
sections named by task 012, and the completed foundations it identifies. Do
not rerun tasks 001-011 or begin task 013.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, and .agent/tasks/architecture-012-workspace-lifecycle.md. Complete only architecture-012-workspace-lifecycle, run its verification, update durable state, and stop.'
```

Graphiti remains deferred: task 025 is a decision gate and requires successful
core-loop dogfooding evidence before any adoption decision.
