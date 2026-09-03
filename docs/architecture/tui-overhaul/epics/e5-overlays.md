# E5 — Move Focused Work to Overlays

- Status: Active planning record; TUI-050 through TUI-051 are complete and
  TUI-052 is pending
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E3](e3-primary-composer.md)

## Outcome

Each secondary workflow temporarily covers the transcript and restores the
same composer/transcript state when dismissed.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-050](../tasks/tui-050-add-overlay-shell.md) | add the shared overlay focus/layout shell | TUI-030 |
| [TUI-051](../tasks/tui-051-move-tasks-overlay.md) | migrate Tasks | TUI-050 |
| [TUI-052](../tasks/tui-052-move-runs-overlay.md) | migrate Runs and its Run Detail child | TUI-051 |
| [TUI-053](../tasks/tui-053-move-preflight-overlay.md) | migrate Preflight | TUI-052 |
| [TUI-054](../tasks/tui-054-move-workflow-overlay.md) | migrate Workflow | TUI-053 |
| [TUI-055](../tasks/tui-055-move-change-summary-overlay.md) | migrate Change Summary | TUI-054 |
| [TUI-056](../tasks/tui-056-move-evidence-overlay.md) | migrate Evidence | TUI-055 |
| [TUI-057](../tasks/tui-057-move-approval-overlay.md) | migrate Approval | TUI-056 |
| [TUI-058](../tasks/tui-058-move-needs-input-overlay.md) | migrate typed needs-input | TUI-057 |

Accepted D4 makes this the exact migration order: Help, Tasks, Runs/Run Detail,
Preflight, Workflow, Change Summary, Evidence, Approval, then typed
needs-input. Each task starts only after the preceding parity gate passes.
Runs and Run Detail stay together because Run Detail is a child selection of
the Runs workflow. Every other independently reachable view has its own task.

## Boundaries

Each migration reuses the current callback and projection. E5 changes focus,
layout, entry, and return behavior; it changes no workflow or store authority.

## Exit Gate

- Every focused view is reachable from commands and retained migration keys.
- Each view has its own content/action, key/command-entry, geometry, and return-
  state parity check.
- Closing any overlay restores the same transcript, live state, and composer
  buffer that existed before it opened.
- Page-only rendering remains available for rollback until TUI-070 satisfies
  the accepted D4 removal criteria.
