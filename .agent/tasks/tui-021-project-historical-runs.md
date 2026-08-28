---
id: tui-021-project-historical-runs
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-020-define-transcript-cells
---

# TUI-021 — Project Historical Runs into Committed Cells

- Status: Completed 2026-08-28
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-021 draft](../../docs/architecture/tui-overhaul/tasks/tui-021-project-historical-runs.md)
- Epic:
  [E2 — Build semantic transcript cells](../../docs/architecture/tui-overhaul/epics/e2-semantic-transcript.md)
- Depends on:
  [completed TUI-020](tui-020-define-transcript-cells.md)
- Design authority:
  [accepted experience-state snapshots](../../docs/architecture/tui-overhaul/README.md#accepted-experience-state-snapshots)

## Outcome

Replace the compact latest-eight dashboard activity list with deterministic
committed transcript cells derived from canonical app projections.

## Scope

- Map current latest-run status and `app.RunTimeline` rows into TUI cells.
- Use stable domain identities and canonical order for committed-cell identity.
- Apply the accepted history window without manufacturing an unbounded memory
  log; preserve Run Detail as the complete audit/debug surface.
- On startup, replay the accepted bounded source-cell window once after
  `session-start`; on refresh, append only newly discovered stable identities
  and never replay the session cell.
- Preserve current filtering of raw ledger events, duplicated task bodies, and
  low-level Codex lifecycle noise.

## Acceptance

- A completed run reads as the accepted short operator narrative.
- The same canonical projection yields the same cell identity, order, and text.
- Refresh neither duplicates nor silently drops cells inside the accepted
  history window; startup emits each rebuilt identity once after the one
  session cell for that process.
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

## Completion Evidence

- `StatusModel` now retains the latest run's accepted eight-transition window
  as source-backed committed cells after the one process-local session cell.
- Cell identities use the canonical run identity, timeline order, typed phase,
  and typed status; rendered prose, color, and timestamps never select status
  or participate in identity.
- Startup emits the rebuilt window once. Successful refresh replaces the
  bounded source projection and appends only identities not already emitted;
  failed refresh retains the last good projection and session source.
- The managed dashboard no longer redraws historical status or activity.
  Runs and Run Detail retain the complete canonical timeline, artifacts, and
  raw event evidence.
- Focused historical-transcript, app, TUI, CLI, and full Go tests plus the TUI
  help command passed.
