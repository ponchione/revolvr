# TUI-051 — Move Tasks into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Make Tasks a parity-tested overlay while retaining its current command/key
entry and app-service actions.

## Scope

- Render the current task list, selection, state, and details in the overlay.
- Preserve task creation and retry callbacks, validations, confirmations, and
  clean-worktree requirements.
- Preserve selection across refresh by canonical task identity.
- Keep old navigation entry during migration and add/retain command entry.
- Return to the unchanged transcript/composer state on dismissal.

## Acceptance

- Every current Tasks action has an equivalent overlay path.
- Refresh keeps the same task selected when its identity still exists and uses
  the documented fallback otherwise.
- Add/retry still routes through existing app authority and preflight behavior.
- Escape and successful/failed actions preserve the accepted return state.
- No task file or store is read/mutated directly by overlay code.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTasks.*Overlay'
go test ./internal/tui
```

## Not Included

- No task workflow redesign, task-publication change, or removal of old keys.
