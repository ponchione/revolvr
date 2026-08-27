# E2 — Build Semantic Transcript Cells

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E1](e1-terminal-shell.md)

## Outcome

Stored history and live activity use one small presentation vocabulary backed
by app-level meaning rather than raw event strings.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-020](../tasks/tui-020-define-transcript-cells.md) | define and render presentation cells | TUI-013 |
| [TUI-021](../tasks/tui-021-project-historical-runs.md) | project canonical run history into committed cells | TUI-020 |
| [TUI-022](../tasks/tui-022-reconcile-live-history.md) | reconcile replaceable live state with committed history | TUI-021 |

## Boundaries

E2 owns TUI presentation state only. Lifecycle, status, verification, and
completion authority remain in existing app/domain projections.

## Exit Gate

- Historical and live states share tested cell rendering and stable identity.
- Refresh reproduces only newly discovered committed meaning; restart emits one
  new `session-start` and replays the bounded canonical window once.
- The dashboard activity renderer is no longer the primary history surface.
