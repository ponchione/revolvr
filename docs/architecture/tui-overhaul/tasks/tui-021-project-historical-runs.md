# TUI-021 — Project Historical Runs into Committed Cells

- Status: Draft; not canonical or runnable
- Epic: [E2 — Build semantic transcript cells](../epics/e2-semantic-transcript.md)
- Depends on: [TUI-020](tui-020-define-transcript-cells.md)

## Outcome

Replace the compact latest-eight dashboard activity list with deterministic
committed transcript cells derived from canonical app projections.

## Scope

- Map current latest-run status and `app.RunTimeline` rows into TUI cells.
- Use stable domain identities and canonical order for committed-cell identity.
- Apply the accepted history window without manufacturing an unbounded memory
  log; preserve Run Detail as the complete audit/debug surface.
- Rebuild the same committed cells on startup and refresh.
- Preserve current filtering of raw ledger events, duplicated task bodies, and
  low-level Codex lifecycle noise.

## Acceptance

- A completed run reads as the accepted short operator narrative.
- The same canonical projection yields the same cell identity, order, and text.
- Refresh/startup neither duplicate nor silently drop cells inside the accepted
  history window.
- No run or task status is inferred from prose, color, or timestamps.
- Run Detail retains access to complete canonical evidence.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestHistoricalTranscript'
go test ./internal/app
go test ./internal/tui
```

## Not Included

- No live-operation state, app timeline redesign, raw-event transcript, or Run
  Detail migration.
