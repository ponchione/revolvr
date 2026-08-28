---
id: tui-022-reconcile-live-history
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-021-project-historical-runs
---

# TUI-022 — Reconcile Live State with Committed History

- Status: Completed 2026-08-28
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-022 draft](../../docs/architecture/tui-overhaul/tasks/tui-022-reconcile-live-history.md)
- Epic:
  [E2 — Build semantic transcript cells](../../docs/architecture/tui-overhaul/epics/e2-semantic-transcript.md)
- Depends on:
  [completed TUI-021](tui-021-project-historical-runs.md)
- Design authority:
  [accepted experience-state snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots)

## Outcome

Consolidate one settled operation's replaceable live presentation into its
canonical committed transcript result exactly once.

## Scope

- Give the active presentation a stable operation identity.
- Keep existing progress caps, detail compaction, and redaction behavior while
  progress is replaceable.
- On settlement or refresh, reconcile live state against the canonical app
  result using domain identity.
- Preserve the accepted `session-start` source and emitted identity across
  refresh and live settlement.
- Append the reconciled final cell only when its stable identity is not already
  in the process-local emitted set.
- Ignore late/stale messages that target an older operation identity.
- Cover completion, failure, cancellation, blocked, safety-stop, and
  needs-input terminal results.

## Acceptance

- Intermediate progress never becomes an exhaustive committed log.
- Settlement creates one coherent final story with no duplicate live/final
  transition.
- Refresh during or after settlement converges on the canonical projection.
- A stale asynchronous message cannot rewrite or clear a newer operation.
- No rendered-string or timestamp heuristic participates in reconciliation.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestLiveTranscript(Reconciles|RejectsStale)'
go test ./internal/tui
```

## Not Included

- No richer live-cell content, composer change, queue, or lifecycle authority.

## Completion Evidence

- Matching terminal results now cross the installed append/ack boundary before
  their replaceable live presentation clears or delayed quit is released.
- Run, task-run, and queue results use canonical run or operation identities;
  process-local tokens and domain identities reject stale progress and terminal
  messages without inspecting rendered text or timestamps.
- Completed, failed, cancelled, blocked, safety-stop, and needs-input results
  render through the existing transcript-cell vocabulary. Existing bounded,
  compacted, redacted live progress remains replaceable.
- Refresh during or after settlement preserves `session-start`, the emitted
  identity set, and one final cell without replay.
- Focused reconciliation, stale-message, TUI package, and full Go tests pass.
