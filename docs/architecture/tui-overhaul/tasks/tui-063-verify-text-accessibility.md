# TUI-063 — Verify Styling and Text Accessibility

- Status: Draft; not canonical or runnable
- Epic: [E6 — Harden terminal behavior](../epics/e6-terminal-hardening.md)
- Depends on: [TUI-060](tui-060-lock-geometry-snapshots.md)

## Outcome

Prove that every important state is understandable as text while current
terminal-default and ANSI semantic styles remain legible.

## Scope

- Check default foreground, dim secondary text, bold attention, and ANSI
  cyan/green/red in representative light and dark terminal themes.
- Disable/strip color in tests and manually inspect selection, success, failure,
  warning, cancellation, needs-input, disabled action, and focus state.
- Add the smallest textual marker where a state currently depends on color.
- Retain terminal-default primary text and current semantic color roles.

## Acceptance

- Every scoped state remains distinguishable with color disabled.
- No fixed black, white, blue, yellow, or custom RGB foreground is required.
- Dim text carries no safety-critical fact by itself.
- Light/dark observations and terminal settings are recorded.
- Style tests assert semantic constraints rather than brittle ANSI byte dumps.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'Test.*Style'
go test ./internal/tui
git diff --check -- docs internal/tui
```

## Not Included

- No theme system, user color configuration, geometry matrix, or terminal
  scrollback behavior.
