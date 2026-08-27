# E7 — Remove the Old Dashboard Shell

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E1](e1-terminal-shell.md) through [E6](e6-terminal-hardening.md)

## Outcome

The accepted transcript application is the only presentation model, its
operator documentation is current, and the full acceptance record is closed.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-070](../tasks/tui-070-remove-dashboard-presentation.md) | remove obsolete dashboard/page presentation | E1-E6 |
| [TUI-071](../tasks/tui-071-update-operator-docs.md) | update shipped operator documentation | TUI-070 |
| [TUI-072](../tasks/tui-072-close-overhaul-acceptance.md) | run and record final overhaul acceptance | TUI-071 |

## Boundaries

E7 deletes migration scaffolding, updates user-facing documentation, and closes
the plan. It does not redesign CLI behavior outside `revolvr tui`.

## Exit Gate

- No obsolete Dashboard label, inactive composer, or page-only navigation path
  remains.
- Every D4 key and command opens the accepted overlay after page-only
  presentation is removed.
- Operator documentation describes the shipped interface.
- Every whole-overhaul acceptance criterion has recorded evidence.
- Full tests and the final supported-terminal matrix pass.
