---
id: tui-051-move-tasks-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-050-add-overlay-shell
---

# TUI-051 — Move Tasks into an Overlay

- Status: Completed 2026-09-03
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-051 draft](../../docs/architecture/tui-overhaul/tasks/tui-051-move-tasks-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-050](tui-050-add-overlay-shell.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

## Outcome

Make Tasks a parity-tested overlay while retaining its current command/key
entry and app-service actions.

## Scope

- Render the current task list, selection, state, and details in the overlay.
- Preserve task creation and retry callbacks, validations, confirmations, and
  clean-worktree requirements.
- Preserve selection across refresh by canonical task identity.
- Retain `2` and `/tasks`; move both from the page to the same overlay only
  when this task's parity gate passes.
- Return to the unchanged transcript/composer state on dismissal.
- Retain the Tasks page renderer as the D4 rollback path until TUI-070.

## Acceptance

- Every current Tasks action has an equivalent overlay path.
- Both `2` and `/tasks` open that parity-tested path.
- Refresh keeps the same task selected when its identity still exists and uses
  the documented fallback otherwise.
- Add/retry still routes through existing app authority and preflight behavior.
- Add Task, retry, and open-Workflow confirmation, guard, success, and failure
  behavior match the retained page path.
- Escape and successful/failed actions preserve the accepted return state.
- No task file or store is read/mutated directly by overlay code.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTasks.*Overlay'
go test ./internal/tui
```

## Not Included

- No task workflow redesign, task-publication change, old-route removal, or
  page-renderer deletion.

## Completion Evidence

- `2` and `/tasks` now open the shared root overlay over the unchanged source
  view and saved composer state while the retained Tasks page renderer remains
  the single content projection and rollback path.
- Task selection, notices, and scrolling stay overlay-local. Refresh preserves
  selection by canonical task ID and falls back to the first task when that ID
  disappears.
- Add Task, retry, refresh, and open-Workflow reuse the existing callbacks,
  guards, confirmations, and result handling. Add failures retain the entry
  state; successful Add and retry return to the owning Tasks overlay.
- Escape restores the exact underlying source/composer state, and 80- and
  40-column rendering remains bounded without a transcript append.
- Focused Tasks-overlay, full TUI, and repository-wide Go tests pass.
