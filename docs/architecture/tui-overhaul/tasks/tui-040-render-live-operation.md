# TUI-040 — Render One Live Operation Cell

- Status: Draft; not canonical or runnable
- Epic: [E4 — Surface runs and loops live](../epics/e4-live-operations.md)
- Depends on: [TUI-022](tui-022-reconcile-live-history.md) and
  [TUI-030](tui-030-make-composer-primary.md)

## Outcome

Present the active operation's current semantic state in one bounded,
replaceable cell.

## Scope

- Consolidate task identity, operation mode, pass/limit, elapsed time, latest
  meaningful detail, and cancellation affordance from existing progress state.
- Distinguish single pass, bounded loop, autonomous task, and queue modes.
- Replace repeated command-lifecycle detail in place.
- Map existing terminal results to distinct completion, failure, cancellation,
  blocked, safety-stop, and needs-input text.
- Point to Run Detail or Workflow for complete canonical evidence.

## Acceptance

- Repeated progress messages do not grow the visible transcript.
- Cell height stays within the accepted bound at 80 and 40 columns.
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
