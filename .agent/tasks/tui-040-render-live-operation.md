---
id: tui-040-render-live-operation
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-022-reconcile-live-history,tui-030-make-composer-primary
---

# TUI-040 — Render One Live Operation Cell

- Status: Pending
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-040 draft](../../docs/architecture/tui-overhaul/tasks/tui-040-render-live-operation.md)
- Epic:
  [E4 — Surface runs and loops live](../../docs/architecture/tui-overhaul/epics/e4-live-operations.md)
- Depends on:
  [completed TUI-022](tui-022-reconcile-live-history.md) and
  [completed TUI-030](tui-030-make-composer-primary.md)
- Design authority:
  [accepted experience-state snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots)

## Outcome

Present the active operation's current semantic state in one bounded,
replaceable cell.

## Scope

- Consolidate task identity, operation mode, pass/limit, elapsed time, latest
  meaningful detail, and cancellation affordance from existing progress state.
- Use the exact running, cancellation-requested, blocked, safety-stop, and
  terminal-result wording in the
  [accepted snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots).
- Distinguish single pass, bounded loop, autonomous task, and queue modes.
- Replace repeated command-lifecycle detail in place.
- Map existing terminal results to distinct completion, failure, cancellation,
  blocked, safety-stop, and needs-input text.
- Point to Run Detail or Workflow for complete canonical evidence.

## Acceptance

- Repeated progress messages do not grow the visible transcript.
- Cell height stays within the accepted bound at 80 and 40 columns.
- `Safety: admitted`, `Current:`, cancellation, and `Next:` remain literal and
  visible at 40 columns without duplicate footer ownership.
- Mode, current pass/limit, cancellation availability, and terminal result are
  understandable without color.
- The cell cannot infer lifecycle or completion state from detail prose.
- Existing cancellation and settlement tests remain authoritative and green.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestLiveOperationCell'
go test ./internal/tui
```

## Not Included

- No queued input, richer workflow detail, progress-event schema, or domain
  lifecycle change.
