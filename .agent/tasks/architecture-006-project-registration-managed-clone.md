---
id: architecture-006-project-registration-managed-clone
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-005-core-ids-artifacts-events-transactions
---

# Add project registration and managed repository copies

## Sequence and status

- Sequence: `006` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-005-core-ids-artifacts-events-transactions`.
- Phase gate: Phase 2 project work starts only after canonical IDs,
  transactions, and events exist.

## Primary outcome

Register one local Git worktree by canonical identity and create a
Revolvr-managed mirror without mutating the operator's checkout.

## Required reading

- ADR-012 and the trust-boundary ADRs it relies on.
- Specification Sections 6, 9.1-9.2, 10.6, 29 Phase 2, 37.1, 43-44, 53,
  and NFR-002/NFR-006/NFR-011.

## Existing foundations to inspect

- `internal/project/register.go` and `register_test.go`.
- `internal/runner`, `internal/gitstate`, `internal/gitoid`, and
  `internal/repositorypath` for existing bounded Git/process behavior.
- `core.projects`, `core.project_sources`, the artifact/event primitives, and
  their sqlc queries.

## Starting assumptions

- Input is an existing non-bare local Git worktree with a valid `HEAD`.
- The managed data root is trusted host configuration, never model input.
- Registration records dirty state but never copies uncommitted work into the
  managed mirror as canonical source.

## Implementation requirements

- Resolve the real worktree and Git common directory safely and reject
  non-repositories, bare/unborn repositories, aliases, and unsafe paths.
- Capture commit, tree, current/default branch, remotes, and bounded dirty-state
  evidence.
- Derive a collision-resistant managed destination and create/adopt a bare
  managed mirror without overwrite or hidden network access.
- Insert project, source, and `project.registered` event atomically.
- Reject duplicate canonical registrations and conflicting managed
  destinations without partial database or filesystem authority.

## Scope boundaries and non-goals

- Do not create task worktrees, fetch automatically, push, export patches, or
  mutate the original checkout.
- Do not add sandbox execution, task intake, indexing, project removal, or
  destructive cleanup.
- Do not broaden this into a general Git hosting layer.

## Acceptance criteria

- Canonical path aliases resolve to one registration.
- The managed mirror contains the pinned commit/tree and the original checkout
  remains byte-for-byte untouched.
- Dirty state and bounded remote evidence are recorded accurately.
- Database/event failure and destination conflict leave no accepted partial
  registration; a safe retry succeeds.
- Integration and full Go tests pass.

## Deterministic verification

```bash
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/project
go test ./...
git diff --check
```

## Completed provenance

- Implementation commit:
  `ebc3457044f01a45315865ec54611d64e0895ace` (`Add project registration and managed mirrors`).

## Expected completion report

Report the commit, schema/query/package files, canonical-path and duplicate
tests, managed-mirror identity, original-checkout proof, rollback/retry result,
and complete test result.
