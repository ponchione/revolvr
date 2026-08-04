---
id: architecture-012-workspace-lifecycle
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-011-sandboxd
---

# Implement the managed workspace lifecycle

## Sequence and status

- Sequence: `012` of `025`.
- Status: pending.
- Prerequisite: `architecture-011-sandboxd`.
- Phase gate: workspace paths may be mounted only through the validated
  sandbox boundary and managed project repository from tasks 006 and 010-011.

## Primary outcome

Create, pin, capture, reconcile, and clean one task-specific managed Git
worktree while proving the operator's original checkout is never mutated.

## Required reading

- ADR-012, ADR-013, and ADR-016.
- Specification Sections 9.2, 12.1-12.4, 17.4, 18.5,
  29 Phase 3, 37.1, 37.5, 39.4, 40.7, 44.2-44.3,
  53, and 56.1-56.4.

## Existing foundations to inspect

- `internal/project` managed mirror registration and scheduler-pinned run
  identities.
- `internal/gitstate`, `internal/gitoid`, `internal/commit`,
  `internal/repositorypath`, and `internal/pathguard`.
- Existing `internal/autonomousworkspace` and `internal/autonomousstate`
  workspace history for reusable recovery behavior only; PostgreSQL becomes
  canonical for this architecture.
- The sandbox mount resolver from tasks 010-011.

## Starting assumptions

- Every workspace is created from an exact managed-repository commit/tree and
  stable run/workspace identity.
- Only trusted host code performs Git administration.
- Workspace files are disposable; metadata, snapshots, diffs, and artifacts
  remain durable.

## Implementation requirements

- Add the minimal reversible workspace schema/events and explicit lifecycle
  transitions from Section 39.4.
- Create a unique task branch/worktree under the managed workspace root from
  the scheduler-pinned source identity; reject existing divergent paths.
- Bind mount identity to the sandbox symbolic workspace ID without exposing
  the original checkout or Git/runtime sockets.
- Capture actual Git status, changed-file manifest, diff artifact/hash, and
  candidate commit/tree through trusted host operations.
- Reconcile every external Git/filesystem effect with a stable operation ID
  before advancing canonical state; retries adopt only exact matching effects.
- Cancel/fail/complete cleanup removes only the exact admitted worktree and
  records cleanup while retaining evidence.

## Scope boundaries and non-goals

- Do not export/push to the operator repository, run model roles, verify code,
  or decide completion.
- Do not mount or modify the original checkout, run Git hooks by default, or
  inherit user Git configuration without explicit controlled need.
- Do not recursively delete unresolved or unvalidated paths.

## Acceptance criteria

- Workspace creation pins the expected commit/tree and original checkout
  before/after identities remain equal.
- Actual diff/changed paths, candidate commit/tree, and workspace lifecycle
  events agree.
- Path collision, symlink substitution, wrong source revision, Git-hook
  attempt, cancellation, timeout, and cleanup failure stop safely.
- Crash injection after worktree/branch/commit creation resumes idempotently or
  reports a typed conflict; no duplicate worktree or commit is fabricated.
- Migration/sqlc, focused Git fixtures, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/workspace ./internal/project
go test ./...
git diff --check
```

## Expected completion report

Report schema/package changes, pinned source and operation identities,
before/after original-checkout proof, diff/commit evidence, collision and
crash-recovery cases, cleanup outcome, and test results.
