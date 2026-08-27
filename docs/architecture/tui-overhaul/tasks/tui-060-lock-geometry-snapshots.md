# TUI-060 — Lock Width and Resize Geometry

- Status: Draft; not canonical or runnable
- Epic: [E6 — Harden terminal behavior](../epics/e6-terminal-hardening.md)
- Depends on: E1-E5 exit gates

## Outcome

Turn the accepted experience states into automated 80-column, 40-column, and
resize-sequence geometry regressions.

## Scope

- Cover idle, uninitialized, running, each terminal result, composer,
  discovery, and every overlay family at 80x24 and 40x24.
- Cover wide-to-narrow-to-wide sequences with retained committed source,
  managed live content, and assertions that committed identities are not
  re-emitted.
- Measure ANSI-stripped display width rather than bytes or rune count.
- Assert required composer, cancellation, safety, and current-state text.
- Prefer current test helpers and focused assertions over a new snapshot system.

## Acceptance

- No visible row exceeds its test width.
- Required state/action text remains visible at 40 columns.
- Resize does not re-emit committed cells, lose the live cell, or corrupt the
  composer buffer/selection.
- Test failures show the offending state and row clearly.
- No dependency or golden-update framework is added.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go \
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
go test ./internal/tui -run 'Test.*(Wide|Narrow|Resize)'
go test ./internal/tui
```

## Not Included

- No real-terminal scrollback, signal/lifecycle matrix, or theme assessment.
