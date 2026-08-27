# E6 — Harden Terminal Behavior

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E1](e1-terminal-shell.md) through [E5](e5-overlays.md)

## Outcome

The application is readable, copy-friendly, and recoverable across the
explicitly supported terminal conditions.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-060](../tasks/tui-060-lock-geometry-snapshots.md) | lock width and resize geometry | E1-E5 |
| [TUI-061](../tasks/tui-061-verify-terminal-scrollback.md) | verify real-terminal history navigation and copying | TUI-060 |
| [TUI-062](../tasks/tui-062-verify-terminal-lifecycle.md) | verify terminal/process restoration | TUI-060 |
| [TUI-063](../tasks/tui-063-verify-text-accessibility.md) | verify styling and text-only meaning | TUI-060 |

## Boundaries

E6 documents measured support and focused fixes. It does not add broad terminal
emulation or claim support for an environment that is not reproducible.

## Exit Gate

- Automated geometry checks prove one session cell at startup/restart and none
  on refresh, resize, or overlay transitions.
- The supported-terminal scrollback and lifecycle matrices pass.
- Remaining limitations are explicit and do not hide safety or action state.
- Important meaning remains understandable without color.
