---
id: tui-011-prove-resize-reflow
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-010-prove-shell-composition
---

# TUI-011 — Prove Resize and Reflow

- Status: Completed 2026-08-27
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-011 draft](../../docs/architecture/tui-overhaul/tasks/tui-011-prove-resize-reflow.md)
- Epic:
  [E1 — Prove the terminal shell](../../docs/architecture/tui-overhaul/epics/e1-terminal-shell.md)
- Depends on:
  [completed TUI-010](tui-010-prove-shell-composition.md)
- Design authority:
  [D3 transcript ownership](../../docs/architecture/tui-overhaul/README.md#d3--transcript-and-scrollback-ownership)
  and the accepted
  [width contract](../../docs/architecture/tui-overhaul/README.md#width-wrapping-and-truncation-contract)

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

## Completion Evidence

- `TestTranscriptShellResize` drives explicit 80-to-40-to-24-to-80 resize
  messages against the TUI-010 proof, measures styled rows with
  `ansi.StringWidth`, and preserves committed identity, order, and source.
- Every resize returns no command, leaves `session-start` emitted exactly once,
  and cannot clear or replay terminal-owned history. The live cell remains
  replaceable and the composer remains reachable at every width.
- `gofmt -w internal/tui/model.go internal/tui/model_test.go` — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellResize'` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
