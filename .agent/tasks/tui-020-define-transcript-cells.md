---
id: tui-020-define-transcript-cells
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-013-install-terminal-shell
---

# TUI-020 — Define and Render Transcript Cells

- Status: Completed 2026-08-28
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-020 draft](../../docs/architecture/tui-overhaul/tasks/tui-020-define-transcript-cells.md)
- Epic:
  [E2 — Build semantic transcript cells](../../docs/architecture/tui-overhaul/epics/e2-semantic-transcript.md)
- Depends on:
  [completed TUI-013](tui-013-install-terminal-shell.md)
- Design authority:
  [accepted experience-state snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots)

## Outcome

Introduce the smallest package-local presentation vocabulary required to render
the accepted transcript snapshots.

## Scope

- Define only the cell kinds required by the accepted source snapshots:
  session, operator action, status, progress, result, warning, and question.
- Keep the session kind limited to the three D6 sources and the typed local
  identity `session-start`; do not add mutable context fields.
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

## Completion Evidence

- `internal/tui` now has exactly the accepted session, operator-action, status,
  progress, result, warning, and question cell kinds. Each cell stores only its
  package-local kind, stable identity, and display source.
- The installed `session-start` cell is built only from the local product
  label, inspected project root, and initialization state at process start.
- Cell rendering uses the existing wrapping and content styles, preserves
  malformed or unknown source as warning-prefixed generic evidence, and bounds
  styled rows by terminal display width.
- `TestTranscriptCellKindsRenderDeterministically`,
  `TestTranscriptCellUnknownAndMalformedInputRemainVisible`, and
  `TestTranscriptCellWrapsByDisplayWidth` cover the accepted vocabulary,
  text-only meaning, invalid input, deterministic output, and width bounds.
- Required formatting, focused transcript-cell tests, TUI package tests, and
  `go test ./...` — PASS.
