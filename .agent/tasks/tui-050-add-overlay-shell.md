---
id: tui-050-add-overlay-shell
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-030-make-composer-primary
---

# TUI-050 — Add the Shared Overlay Shell

- Status: Pending
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-050 draft](../../docs/architecture/tui-overhaul/tasks/tui-050-add-overlay-shell.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-030](tui-030-make-composer-primary.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

## Outcome

Add one package-local overlay focus/layout mechanism that preserves the shell
state beneath it.

## Scope

- Represent one active overlay with content identity, selection/scroll state,
  size, and dismissal.
- Route keys to the overlay while it owns focus and restore prior composer focus
  on close.
- Reflow overlay content within the current terminal width and height.
- Let active-operation messages update underlying live state safely.
- Use Help as the first fixture and retain all current Help content/actions,
  `?`, Enter on bare `/`, `/help`, and `/commands` entry.
- Retain the Help page renderer as the D4 rollback path until TUI-070.

## Acceptance

- Escape closes Help and restores the exact prior composer buffer and focus.
- Every retained Help key/command entry opens the same overlay behavior.
- Opening/closing does not alter committed transcript, live identity, or scroll
  ownership beneath the overlay.
- Opening/closing emits no committed row and does not move terminal-owned
  scrollback.
- Overlay selection, scrolling, and resizing remain bounded at 80 and 40 columns.
- Active settlement behind an overlay is visible correctly after dismissal.
- Help content/action descriptions have parity with the retained page path.
- The implementation adds no generalized window manager or public abstraction.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestOverlayShell'
go test ./internal/tui
```

## Not Included

- No migration of a non-Help view, nested arbitrary overlays, or app/domain
  behavior.
