# TUI-020 — Define and Render Transcript Cells

- Status: Draft; not canonical or runnable
- Epic: [E2 — Build semantic transcript cells](../epics/e2-semantic-transcript.md)
- Depends on: [TUI-013](tui-013-install-terminal-shell.md)

## Outcome

Introduce the smallest package-local presentation vocabulary required to render
the accepted transcript snapshots.

## Scope

- Define only accepted cell kinds, expected to include session, operator action,
  status, progress, result, warning, and question when TUI-005 requires them.
- Store display source and stable presentation identity, never duplicated
  lifecycle policy.
- Render every cell within a supplied display width using current text styles.
- Render unknown or malformed presentation input as visible generic evidence.
- Keep types and helpers package-local until a proven non-TUI consumer exists.

## Acceptance

- Every field has a rendering or reconciliation use in an accepted snapshot.
- Important meaning remains textual with styles disabled.
- Unknown input remains visible and cannot be mistaken for success.
- Rendering is deterministic and no row exceeds the supplied width.
- The diff adds no interface, factory, public package, or domain enum.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTranscriptCell'
go test ./internal/tui
```

## Not Included

- No run-history projection, live reconciliation, app-service change, or new
  semantic state.
