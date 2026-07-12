# Session Handoff

Date: 2026-07-12

## Where We Left Off

AW-29 is complete. AW-30 is the next unchecked task and must begin in a fresh
`codex exec` invocation. Do not resume this session and do not start AW-31.

External notifications are explicit opt-in `notifications` configuration and
part of `revolvr-effective-run-config-v4`. The exact event allowlist is task
completed, blocked, typed needs input, trusted safety stop, queue drained, and
daemon failure. Disabled policy performs no notification lookup, secret load,
directory/lock/ledger/temp creation, or subprocess work.

`internal/autonomousnotification` owns strict v1 policy/payload/outbox
contracts, deterministic event/delivery IDs, exact canonical JSON stdin,
immutable intent/payload/history, synchronized journals, bounded runner
execution, replacement environment, redaction, retry/restart, and inspection.
Task, queue, daemon, input, finalization, archive, safety, ledger, and retention
owners remain authoritative. Hook failure is durable and operator-visible but
cannot change their result, bytes, stop reason, or source error precedence.

Direct task and queue app wrappers release the global autonomous-execution
lease before hook execution. Queue-internal tasks do not dispatch; their exact
durable outcomes are adapted once by the enclosing queue. Replays rebuild the
same delivery identity, successful delivery starts no second process, and
running/history-ahead crashes consume and recover one bounded attempt. Stable
receiver keys support deduplication, but external exactly-once is deliberately
not claimed across the receiver-success/local-crash window.

Operator inspection is limited to redacted read-only `notification list/show`.
Config check and doctor show nonsecret policy facts. AW-27 views and AW-28 TUI
refresh remain read-only and do not dispatch or retry. Notification evidence is
kept in its isolated outbox rather than changing AW-25 ledger export/replay or
retention schemas.

## Verification Status

All checks passed:

```text
go test -count=1 <required AW-28 baseline packages>
go test -count=1 ./internal/app ./internal/cli ./internal/tui \
  ./internal/autonomoustaskrun ./internal/autonomousqueue \
  ./internal/autonomousdaemon ./internal/autonomousexec \
  ./internal/autonomousfinalization ./internal/autonomousinput \
  ./internal/autonomousstate ./internal/autonomousarchive \
  ./internal/autonomoussafety ./internal/ledger ./internal/ledgerexport \
  ./internal/artifactretention ./internal/redact ./internal/runner \
  ./internal/pathguard ./internal/lock ./internal/runonce \
  ./internal/autonomousnotification
go test -count=1 ./...
go vet ./internal/app ./internal/cli ./internal/runonce \
  ./internal/autonomousnotification
git diff --check
go run ./cmd/revolvr --help
go run ./cmd/revolvr run --help
go run ./cmd/revolvr notification --help
go run ./cmd/revolvr notification list
go run ./cmd/revolvr config check
go run ./cmd/revolvr doctor # expected nonzero only for accumulated dirty tree
go run ./cmd/revolvr status
```

Tests use injected runners/lookups/clocks/waits and temporary repositories. They
cover six-event payload goldens, strict configuration, redaction, exact process
authority, caps/timeouts/retries/cancellation, duplicate/restart/history-ahead
recovery, disabled no-op behavior, source-durable replay reconciliation,
failure isolation, lease ordering, CLI diagnostics, and read-only inspection.
No live Codex/model, real hook/receiver, network request, daemon service,
archive/retention mutation, metrics/evaluation, or parallel worker ran.

## Worktree State

The AW-01 through AW-16 baseline remains committed at `28371e4` (`Build
autonomous workflow through conditional roles`). AW-17 through AW-29 source,
tests, documentation, and durable-state changes are intentionally uncommitted
and must be preserved. The consumed AW-29 kickoff prompt is deleted as
expected.

Start the next fresh session with `git status --short --branch` and
`git log -1 --oneline`, then read `AGENTS.md`, this handoff, `.agent/TASKS.md`,
`.agent/STATE.md`, `.agent/DECISIONS.md`, and
`.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md` completely. Never reset, clean,
restore, stash, or otherwise discard accumulated work.

## Next Task

AW-30 — add autonomous-loop metrics and deterministic evaluation scenarios.
Do not add bounded parallel workers; AW-31 retains that scope.

## Blockers

None.
