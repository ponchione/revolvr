# TUI-052 — Move Runs and Run Detail into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Move the parent Runs list and its selected Run Detail child into one coherent
overlay navigation flow.

## Scope

- Render the current Runs list with stable run selection and refresh behavior.
- Open Run Detail as the child state of the selected run and return to the same
  Runs selection on back.
- Preserve Run Detail scrolling, raw audit/debug timeline, artifacts, warnings,
  and receipt validation action.
- Keep old navigation entry during migration and add/retain command entry.
- Restore transcript/composer state when the Runs overlay closes.

## Acceptance

- Runs selection survives refresh by stable run identity.
- Back from Run Detail returns to the same run and list scroll position.
- Receipt validation and complete canonical evidence behave as before.
- Overlay close returns to the pre-open composer buffer and transcript.
- No run state is inferred from rendered transcript cells.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'Test(Runs|RunDetail).*Overlay'
go test ./internal/tui
```

## Not Included

- No Run Detail redesign, transcript history change, or removal of old keys.
