# TUI-011 — Prove Resize and Reflow

- Status: Completed 2026-08-27 as
  [the canonical task](../../../../.agent/tasks/tui-011-prove-resize-reflow.md)
- Epic: [E1 — Prove the terminal shell](../epics/e1-terminal-shell.md)
- Depends on: [TUI-010](tui-010-prove-shell-composition.md)

## Outcome

Prove that retained source cells and the managed live/composer frame follow the
accepted D3 behavior across wide-to-narrow and narrow-to-wide resize without
re-emitting terminal-owned rows.

## Scope

- Drive the shell proof through explicit resize messages in both directions.
- Assert display width after ANSI styling, not byte length.
- Verify committed source identity, order, and content across each resize.
- Verify live-cell replacement and composer placement after reflow.
- Assert that resize emits no previously committed identity and sends no
  terminal-history clear/replay operation.
- Assert that wide-to-narrow-to-wide resize never emits `session-start` again.
- Leave native soft-wrap/reflow measurement to TUI-061.

## Acceptance

- No tested row exceeds the current terminal width.
- Resize does not duplicate, reorder, or lose committed source meaning.
- The active cell remains replaceable and the composer remains reachable.
- Previously emitted rows remain terminal-owned and are not reinserted.
- Behavior at and below the accepted minimum width matches TUI-005.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTranscriptShellResize'
go test ./internal/tui
```

## Not Included

- No cancellation/quit settlement, semantic projection, native-scrollback
  measurement, or full snapshot matrix.

## Result

The test-only shell proof reflows the managed live/composer frame through
explicit 80-to-40-to-24-to-80 resize messages while ANSI display-cell widths
stay bounded. Committed source and emitted identities remain unchanged, so
resize cannot clear, replay, or re-emit `session-start`.
