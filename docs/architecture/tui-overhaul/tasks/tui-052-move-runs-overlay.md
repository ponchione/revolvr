# TUI-052 — Move Runs and Run Detail into an Overlay

- Status: Completed 2026-09-03
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-051](tui-051-move-tasks-overlay.md)

## Outcome

Move the parent Runs list and its selected Run Detail child into one coherent
overlay navigation flow.

## Scope

- Render the current Runs list with stable run selection and refresh behavior.
- Open Run Detail as the child state of the selected run and return to the same
  Runs selection on back.
- Make direct `4` and `/detail` entry construct that same Runs parent before
  showing the current detail or existing empty-detail result.
- Preserve Run Detail scrolling, raw audit/debug timeline, artifacts, warnings,
  and receipt validation action.
- Retain `3`, `/runs`, `4`, `/detail`, and Runs `Enter`/`o`; cut those entries
  over only when this task's parity gate passes.
- Restore transcript/composer state when the Runs overlay closes.
- Retain the Runs and Run Detail page renderers as the D4 rollback path until
  TUI-070.

## Acceptance

- Runs selection survives refresh by stable run identity.
- Back from Run Detail returns to the same run and list scroll position.
- A second Escape from the Runs parent dismisses the root overlay and restores
  exact pre-open shell state.
- Key, command, direct-detail, and Runs-selection entry reach the same accepted
  parent/child state.
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

- No Run Detail redesign, transcript history change, old-route removal, page-
  renderer deletion, or general overlay stack.

## Completion Evidence

- All four retained entries reach the shared overlay; direct detail entry
  constructs the same Runs parent before showing loaded or empty detail.
- Stable run identity, parent list offset, explicit child back, exact root
  dismissal, and 80-/40-column bounds pass focused parity coverage.
- Run Detail keeps its existing timeline, raw-event, artifact, warning,
  scrolling, reload, and receipt-validation behavior through existing app
  projections and callbacks.
- No transcript authority, dependency, general overlay stack, domain state, or
  rollback page renderer changed.
