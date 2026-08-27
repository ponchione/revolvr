# TUI-011 — Prove Resize and Reflow

- Status: Draft; not canonical or runnable
- Epic: [E1 — Prove the terminal shell](../epics/e1-terminal-shell.md)
- Depends on: [TUI-010](tui-010-prove-shell-composition.md)

## Outcome

Prove that source-backed committed and live content follows the accepted D3
behavior across wide-to-narrow and narrow-to-wide resize.

## Scope

- Drive the shell proof through explicit resize messages in both directions.
- Assert display width after ANSI styling, not byte length.
- Verify committed source identity, order, and content across each resize.
- Verify live-cell replacement and composer placement after reflow.
- Document any intentional terminal-owned history that cannot be reflowed.

## Acceptance

- No tested row exceeds the current terminal width.
- Resize does not duplicate, reorder, or lose committed source meaning.
- The active cell remains replaceable and the composer remains reachable.
- Behavior at and below the accepted minimum width matches TUI-005.
- Any terminal limitation has an explicit operator behavior, not a silent loss.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTranscriptShellResize'
go test ./internal/tui
```

## Not Included

- No cancellation/quit settlement, semantic projection, or full snapshot
  matrix.
