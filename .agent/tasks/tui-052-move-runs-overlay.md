---
id: tui-052-move-runs-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-051-move-tasks-overlay
---

# TUI-052 — Move Runs and Run Detail into an Overlay

- Status: Completed 2026-09-03
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-052 draft](../../docs/architecture/tui-overhaul/tasks/tui-052-move-runs-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-051](tui-051-move-tasks-overlay.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

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

- `3`, `/runs`, `4`, and `/detail` now open the shared root overlay while the
  retained Runs and Run Detail page renderers remain the content projections
  and D4 rollback paths.
- Runs selection is overlay-local, survives refresh by canonical run ID, and
  falls back to the first run when that identity disappears.
- Runs `Enter`/`o` opens the selected detail as the explicit child. Back
  restores the same selection and list offset; the next Escape restores the
  exact source/composer state without changing committed transcript cells.
- Run Detail retains scrolling, timeline, raw events, artifacts, warnings, and
  receipt validation through the existing callbacks. Late results cannot
  replace a newer overlay owner.
- Focused Runs/Run Detail overlay, full TUI, and repository-wide Go tests pass.
