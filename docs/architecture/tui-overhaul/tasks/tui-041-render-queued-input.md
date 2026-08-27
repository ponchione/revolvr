# TUI-041 — Render Domain-Owned Queued Input

- Status: Conditional draft; delete if D2 rejects queued/deferred input
- Epic: [E4 — Surface runs and loops live](../epics/e4-live-operations.md)
- Depends on: [TUI-040](tui-040-render-live-operation.md), accepted
  D2/D5, and the completed app/domain queue prerequisite from TUI-001

## Outcome

Present accepted queued/deferred operator input and its permitted controls
without giving the TUI queue authority.

## Scope

- Render the app projection's queued items and stable identities above the
  composer in canonical order.
- Render persistence/next-pass meaning and any accepted cancellation/edit
  affordance.
- Route cancellation or editing only to the accepted app service.
- Rebuild the same order and status after refresh/restart.
- Reconcile consumed, cancelled, rejected, and stale items from returned domain
  state.

## Acceptance

- UI order and canonical queue order agree before and after refresh/restart.
- The TUI cannot fabricate, reorder, consume, or silently discard an item.
- One control action targets one stable item identity and reflects the returned
  result.
- Queued state, failure, and consumption remain textual without color.
- Stale responses cannot mutate a newer queue projection.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestQueuedOperatorInput'
go test ./internal/app
go test ./internal/tui
```

## Not Included

- No queue schema, persistence, ordering, consumption, or restart authority.
