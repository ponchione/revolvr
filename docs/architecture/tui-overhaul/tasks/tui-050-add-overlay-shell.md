# TUI-050 — Add the Shared Overlay Shell

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-030](tui-030-make-composer-primary.md)

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
- Use Help as the first fixture and retain all current Help content/actions.

## Acceptance

- Escape closes Help and restores the exact prior composer buffer and focus.
- Opening/closing does not alter committed transcript, live identity, or scroll
  ownership beneath the overlay.
- Overlay selection, scrolling, and resizing remain bounded at 80 and 40 columns.
- Active settlement behind an overlay is visible correctly after dismissal.
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
