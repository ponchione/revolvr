# E1 — Prove the Terminal Shell

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E0](e0-product-contract.md)

## Outcome

One small Bubble Tea proof establishes the accepted transcript, live-region,
composer, resize, and settlement mechanics before the dashboard is migrated.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-010](../tasks/tui-010-prove-shell-composition.md) | prove shell composition and test IO | E0 |
| [TUI-011](../tasks/tui-011-prove-resize-reflow.md) | prove resize and reflow | TUI-010 |
| [TUI-012](../tasks/tui-012-prove-active-settlement.md) | prove cancellation and quit settlement | TUI-010 |
| [TUI-013](../tasks/tui-013-install-terminal-shell.md) | install the proven shell behind current behavior | TUI-011, TUI-012 |

## Boundaries

E1 proves and installs presentation mechanics only. It does not add semantic
run projection, plain-text actions, overlay migration, or a general terminal
backend.

## Exit Gate

- Composition, test IO, resize/reflow, and active-settlement proofs pass.
- The accepted shell runs in the current program without an app/domain change
  or new dependency.
- Existing commands, focused views, and operation guards remain reachable.
