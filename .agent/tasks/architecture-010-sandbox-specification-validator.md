---
id: architecture-010-sandbox-specification-validator
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-009-scheduler-leases
---

# Implement the sandbox specification and validator

## Sequence and status

- Sequence: `010` of `025`.
- Status: pending.
- Prerequisite: `architecture-009-scheduler-leases`.
- Phase gate: Phase 3 begins only after the host can admit and pin one task;
  no untrusted process may start before this validator exists.

## Primary outcome

Define one versioned sandbox request and a fail-closed host validator that can
authorize only managed paths, approved images, explicit resources, bounded
commands, environment names, runtime profiles, and network profiles.

## Required reading

- ADR-010 and ADR-012 through ADR-015.
- Specification Sections 6, 7.2-7.8, 16, 17, 29 Phase 3, 37.5,
  40.6, 47, 58.4, and 60.

## Existing foundations to inspect

- Registered managed paths in `internal/project` and scheduler-pinned
  project/run identities.
- `internal/pathguard`, `internal/repositorypath`, `internal/runtimepath`, and
  `internal/redact` for existing path and secret-boundary behavior.
- `internal/runner` for bounded command representation; do not execute a
  container in this task.

## Starting assumptions

- The trusted caller supplies symbolic managed source IDs, not arbitrary host
  paths.
- `strict`, `compatible`, and attended-only `diagnostic` are the only sandbox
  profiles; network defaults to `none`.
- Worker sandboxes receive no ambient credentials or host configuration.

## Implementation requirements

- Define the versioned typed request described in Sections 7.2 and 47,
  including task/run/role, pinned image digest, command argv, symbolic mounts,
  network, resources, environment names/values, and runtime profile.
- Reject unknown fields/versions, empty or oversized values, mutable image tags
  without an approved digest, unsafe roles/profiles, and nonpositive or
  over-policy resources/timeouts.
- Resolve symbolic mounts under configured managed roots with traversal,
  symlink-substitution, hard-link/file-type, overlap, target, and mode checks.
- Enforce one writable workspace, only approved read-only context/cache mounts,
  bounded tmpfs, direct argv, environment allowlisting, and default-deny
  network.
- Reject privileged/host namespaces, added capabilities, devices, runtime
  sockets, host home, SSH agent, PostgreSQL/OpenAI credentials, and unknown
  images even if encoded through malformed or duplicate input.
- Return a normalized immutable specification suitable for hashing and later
  runtime evidence.

## Scope boundaries and non-goals

- Do not create containers, start `sandboxd`, manage worktrees, or implement a
  second runtime backend.
- Do not accept raw OCI flags or arbitrary source paths from models/API
  callers.
- Do not silently downgrade strict mode, enable network, or inject secrets.

## Acceptance criteria

- Valid requests normalize deterministically and hash identically.
- Every prohibited host access and privilege form fails before runtime work.
- Path traversal, symlink swap, hard-link/wrong-type, mount overlap, runtime
  socket, host-home, and unapproved image tests fail closed.
- Malformed JSON/schema, duplicate fields, unknown versions, invalid resource
  bounds, network escalation, and secret environment tests leave no effects.
- The full Go suite passes.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go test ./internal/sandbox -run 'Test.*Valid|Test.*Reject|Test.*Path|Test.*Symlink'
go test ./...
git diff --check
```

## Expected completion report

Report the request/schema version, normalized hash inputs, allowed profiles and
mounts, every abuse-case category tested, changed files, and full-suite result.
