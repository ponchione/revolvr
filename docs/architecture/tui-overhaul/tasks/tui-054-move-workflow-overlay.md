# TUI-054 — Move Workflow into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Present the existing autonomous workflow projection and controls in an overlay
without changing workflow semantics.

## Scope

- Render current workflow phase, pass/limit, task/run identity, stop evidence,
  and available actions.
- Preserve start/continue/cancel/refresh callbacks and operation guards.
- Show needs-input state but leave typed answer interaction on its current path
  until TUI-058.
- Keep old navigation entry during migration and add/retain command entry.
- Preserve underlying live updates and return state.

## Acceptance

- Every workflow state and control exposed before migration remains reachable.
- Active updates behind the overlay reconcile without duplicating history.
- Operation guards and app results remain the only source of availability.
- Needs-input state stays visible and its existing answer path still works.
- Dismissal restores the exact pre-open shell state.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestWorkflow.*Overlay'
go test ./internal/tui
```

## Not Included

- No workflow policy, typed-answer migration, queue behavior, or old-key
  removal.
