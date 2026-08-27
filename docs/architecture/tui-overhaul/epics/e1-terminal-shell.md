# E1 — Prove the Terminal Shell

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E0](e0-product-contract.md)

## Outcome

One small Bubble Tea proof establishes the accepted append-only committed
history plus live/composer/overlay-frame hybrid before the dashboard is
migrated.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-010](../tasks/tui-010-prove-shell-composition.md) | prove `tea.Println` history plus managed-frame composition and test IO | E0 |
| [TUI-011](../tasks/tui-011-prove-resize-reflow.md) | prove managed-frame reflow without history replay | TUI-010 |
| [TUI-012](../tasks/tui-012-prove-active-settlement.md) | prove one-time final emission, cancellation, and quit settlement | TUI-010 |
| [TUI-013](../tasks/tui-013-install-terminal-shell.md) | install the proven shell behind current behavior | TUI-011, TUI-012 |

## Boundaries

E1 proves and installs presentation mechanics only. It does not add semantic
run projection, plain-text actions, overlay migration, or a general terminal
backend.

## Exit Gate

- Append composition, test IO, managed-frame reflow, and active-settlement
  proofs pass without clearing or replaying terminal history.
- The `session-start` cell is emitted once before bounded history and is not
  re-emitted by redraw or resize.
- The accepted shell runs in the current program without an application
  callback, domain-authority change, or new dependency; it only adds the
  inspected root to the existing status projection.
- Existing commands, focused views, and operation guards remain reachable.
