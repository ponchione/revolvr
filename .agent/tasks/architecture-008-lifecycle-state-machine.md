---
id: architecture-008-lifecycle-state-machine
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-007-task-schema-import
---

# Add the PostgreSQL task lifecycle state machine

## Sequence and status

- Sequence: `008` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-007-task-schema-import`.
- Phase gate: imported immutable task versions exist before approval and legal
  runtime transitions can be authoritative.

## Primary outcome

Make lifecycle and acceptance state transitions explicit, authority-checked,
optimistically concurrent, and atomic with append-only PostgreSQL events.

## Required reading

- ADR-019 and ADR-024.
- Specification Sections 3.1-3.3, 9.5-9.8, 10.5-10.6, 12, 18.7,
  29 Phase 2, 37.4, 39, 40.3, and 44.

## Existing foundations to inspect

- `internal/tasklifecycle/state.go`, `postgres.go`, and their tests.
- `db/migrations/00004_core_tasks.sql` and
  `00005_task_lifecycle.sql`.
- Task, task-version, acceptance, and event queries generated under
  `internal/storage/postgres`.
- Legacy `internal/passpolicy` and `internal/autonomousstate` only for reusable
  behavior; filesystem state does not replace PostgreSQL authority.

## Starting assumptions

- Imported canonical tasks begin as unaccepted `draft` aggregates at version
  one.
- Models may propose actions but never hold transition authority.
- Every accepted transition names the expected aggregate version and exact task
  version.

## Implementation requirements

- Encode the task and criterion transition matrices from Section 39 with
  explicit allowed authorities and preconditions.
- Add task aggregate version and lifecycle/approval consistency constraints.
- Implement draft-to-compiled, compiled-to-awaiting-approval, and exact
  operator approval as the first persisted transition slice.
- Update current state and append the matching event in one transaction.
- Reject stale aggregate versions, wrong project/task/version ownership,
  illegal transitions, model authority, empty operator identity, and
  unimplemented future transitions without mutation.
- Prove concurrent equal-version transitions have one winner and one event;
  event failure must roll back state and permit one clean retry.

## Scope boundaries and non-goals

- Do not implement scheduling, leases, model invocation, sandboxing, planning,
  verification, audit, or completion finalization.
- Do not allow in-place reversal of completed state or mutable accepted task
  versions.
- Do not infer authority from model prose or caller naming.

## Acceptance criteria

- The full task/criterion matrices accept every specified legal edge and reject
  every unspecified edge or wrong authority.
- Review-gate and approval transitions bind immutable task-version provenance.
- Stale, cross-project, cross-task, duplicate, and malformed commands fail
  without state/event changes.
- Concurrent transition and forced-event-failure rollback/retry tests pass.
- Migration round-trip, sqlc generation, and full Go tests pass.

## Deterministic verification

```bash
go test ./internal/tasklifecycle -run 'TestTaskTransitionMatrix|TestCriterionTransitionMatrix'
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/tasklifecycle
go test ./...
git diff --check
```

## Completed provenance

- Implementation commit:
  `541cd428c89b6a5b1ee22cd86664ce56c02e0ae6` (`Add PostgreSQL task lifecycle`).

## Expected completion report

Report the commit, lifecycle migration/query/package paths, matrix coverage,
approval provenance, illegal/stale/ownership rejection, concurrency winner,
rollback/retry evidence, and full-suite result.
