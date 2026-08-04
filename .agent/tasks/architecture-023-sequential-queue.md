---
id: architecture-023-sequential-queue
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-022-deterministic-eval-suite
---

# Add the bounded sequential queue

## Sequence and status

- Sequence: `023` of `025`.
- Status: pending.
- Prerequisite: `architecture-022-deterministic-eval-suite`.
- Phase gate: Phase 10 queue work begins only after deterministic recovery,
  scope, sandbox, and false-completion scenarios pass. Real-project queue use
  also requires acceptable measured quality gates from Section 23.3.

## Primary outcome

Run a manually started, bounded queue operation that repeatedly selects and
executes exactly one task at a time, yields blocked work, and resumes safely
from canonical evidence.

## Required reading

- ADR-011 and ADR-022.
- Specification Sections 2.4, 3.4, 9.6, 12,
  21, 23.3, 24.1, 26.4, 29 Phase 10 and Phase 12,
  37.3, 41.4, 56, and NFR-002/NFR-005/NFR-006.

## Existing foundations to inspect

- Scheduler/global lease from task 009, lifecycle/control loop from tasks
  014-019, completion/release behavior, and task 022 evaluation fixtures.
- Existing `internal/autonomousqueue`, `internal/autonomoustaskrun`,
  `internal/autonomousexec`, and application/CLI queue code for recovery and
  outcome concepts only; do not port parallel workers from the legacy runtime.

## Starting assumptions

- The operator explicitly starts the queue with stable operation identity and
  finite task/cycle/token/cost/time limits.
- The scheduler remains the sole selector and the admitted task stays pinned
  until its terminal-for-now outcome.
- Only one source-mutating worker may exist globally under this architecture.

## Implementation requirements

- Add the minimal canonical queue operation/task-occurrence schema, events,
  stable operation ID, pinned limits/config, ordered outcomes, and terminal
  marker.
- At each task boundary, rebuild/revalidate the task graph and select through
  task 009; never pre-admit a stale batch or replace a pinned task mid-run.
- Drive one task through bounded supervisor/worker/verification/audit/
  completion cycles, then re-evaluate before selecting another.
- Yield typed blocked, needs-input, dependency-waiting, and task-local bounded
  outcomes so unrelated ready work can continue without rewriting priority.
- Stop deterministically on drained, waiting-on-dependencies, waiting-on-input,
  all-remaining-blocked, budget-exhausted, cancelled, unsafe, or system-failure
  outcomes.
- Persist intent and exact occurrence identity before each external effect;
  resume only unresolved exact work, return prior terminal results without
  rerunning, and reconcile cancellation/leases/workspaces before returning.
- Expose bounded CLI start/status/cancel behavior using the same application
  services; no hidden network fetch or daemon start.

## Scope boundaries and non-goals

- Do not add parallel workers, swarms, task delegation, automatic daemon mode,
  background service installation, or unbounded operation defaults.
- Do not skip unsafe graph/state ambiguity or let one yielded task starve later
  unrelated work forever.
- Do not archive, export, push, deploy, or merge completed work automatically.

## Acceptance criteria

- Instrumented tests prove peak source-mutating workers is exactly one.
- Deterministic ordering, dependency unlock, blocked/input yield, unrelated
  progress, all stop reasons, and exact budget limits match canonical state.
- Concurrent queue/direct-run admission has one winner under the global lease.
- Crash injection before/after task selection, worker effect, completion, and
  queue checkpoint resumes idempotently without duplicate runs/events/commits.
- Cancellation stops the active child, reconciles workspace/lease/evidence, and
  leaves a typed result; replay of terminal queue state starts no work.
- Migration/sqlc, deterministic evals, CLI tests, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/queue ./internal/evaluation
go run ./cmd/revolvr queue --help
go test ./...
git diff --check
```

## Expected completion report

Report schema/CLI/package changes, limits and ordering, proof of peak one
worker, yield/stop outcomes, admission race, each crash/replay boundary,
cancellation cleanup, quality-gate status, and test results.
