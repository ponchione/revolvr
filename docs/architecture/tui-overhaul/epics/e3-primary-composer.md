# E3 — Make the Composer Primary

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E1](e1-terminal-shell.md) and accepted D2

## Outcome

The composer is the default focus owner, retains safe slash-command behavior,
and routes plain text only through the accepted app-level contract.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-030](../tasks/tui-030-make-composer-primary.md) | make the existing composer always available | TUI-013 |
| [TUI-031](../tasks/tui-031-implement-plain-text-input.md) | implement the accepted plain-text route | TUI-030, D2, app prerequisite if any |
| [TUI-032](../tasks/tui-032-add-contextual-command-discovery.md) | add contextual slash-command discovery | TUI-030 |

## Boundaries

E3 does not invent task publication, steering, queuing, or typed-answer
authority. A missing domain capability is a separate prerequisite selected in
TUI-001, not work hidden inside TUI-031.

## Exit Gate

- The composer is visible and focused whenever no overlay or typed question
  owns focus.
- Every retained slash command remains executable and discoverable.
- Accepted text semantics have focused app and TUI regression checks.
