---
id: architecture-009-scheduler-leases
status: completed
workflow: mixed-pass-v1
phase: simplify
depends_on: architecture-008-lifecycle-state-machine
---

# Implement the PostgreSQL scheduler and execution leases

## Sequence and status

- Sequence: `009` of `025`.
- Status: completed; this historical authority must not be scheduled again.
- Prerequisite: `architecture-008-lifecycle-state-machine`.
- Phase gate: Phase 2 scheduling starts only after accepted task versions and
  lifecycle transitions are canonical and concurrency-tested.

## Primary outcome

Select one ready task deterministically and admit it under one canonical global
source-mutation lease without allowing a model or racing coordinator to change
the selected task.

## Required reading

- ADR-011 and ADR-019.
- Specification Sections 3.1-3.4, 9.6-9.8, 10.5-10.6, 12.1, 21,
  29 Phase 2, 37.2-37.4, 39.1, 44.2, and NFR-002/NFR-006/NFR-011.

## Existing foundations to inspect

- `internal/tasklifecycle`, `internal/taskintake`, `internal/project`, and
  `internal/storage/postgres`.
- Migrations `00002` through `00005` and `db/queries/core.sql`.
- Existing `internal/taskschedule`, `internal/taskscheduler`,
  `internal/autonomousscheduler`, `internal/autonomousexec`, and
  `internal/lock` for reusable selection/locking behavior only; their
  filesystem runtime state is not PostgreSQL authority.

## Starting assumptions

- A runnable task is `pending`, has an accepted immutable version, satisfied
  dependencies, no terminal-unsatisfied dependency, no awaiting checkpoint,
  and a healthy registered project baseline.
- Selection order is priority ascending, canonical creation/path authority,
  then stable task ID.
- v1 permits one source-mutating run globally, not one per project.

## Implementation requirements

- Add the minimal reversible schema and named queries for scheduler admission,
  runs, and the global execution lease.
- Build one validated graph projection that rejects missing, duplicate, self,
  cyclic, ambiguous, and unsatisfied dependency/conflict state before
  selection.
- Select deterministically from canonical PostgreSQL state; never fall back to
  file order or let a model substitute another task.
- In one transaction, revalidate the candidate and expected aggregate version,
  acquire the global lease, create/pin the run and task version/source
  identities, transition the task to `admitted`, and append events.
- Provide explicit release and restart reconciliation that inspect canonical
  run/task/lease evidence before changing anything.
- Return typed no-ready, conflict, waiting, unsafe-graph, and lease-busy
  outcomes.

## Scope boundaries and non-goals

- Do not start a worker, invoke a model, create a workspace, or implement the
  multi-task queue.
- Do not add parallel workers, per-project concurrency, daemon polling, or
  model-controlled prioritization.
- Do not treat cancelled, abandoned, superseded, blocked, or needs-input tasks
  as satisfied dependencies.

## Acceptance criteria

- Identical database state always selects the same task.
- Invalid graphs select nothing and report the exact invalidity.
- Two concurrent admissions produce one admitted run/lease/event sequence and
  one typed loser; no duplicate active mutation is possible.
- Forced failure between lease, run, state, and event writes rolls back all of
  them.
- Reconciliation is idempotent and never steals a lease from unresolved
  authoritative work.
- Migration up/down/up, sqlc generation, PostgreSQL integration tests, and the
  full Go suite pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/scheduler ./internal/tasklifecycle ./internal/storage/postgres
go test ./...
git diff --check
```

Exercise fixtures for deterministic ties, each invalid graph shape, pending
and terminal-unsatisfied dependencies, concurrent admission, forced
transaction rollback, release, and crash reconciliation.

## Completed provenance

- Implementation commit:
  `d190de9916a6b70df100c345f1165337f8097bd9` (`Add PostgreSQL scheduler leases`).

## Expected completion report

Report changed migrations/queries/packages, selection order, lease identity and
transaction boundary, graph rejection coverage, concurrency winner/loser,
rollback/recovery evidence, sqlc status, and full-suite result.
