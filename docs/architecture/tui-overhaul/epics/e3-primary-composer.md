# E3 — Make the Composer Primary

- Status: Active planning record; TUI-030 is pending and later tasks remain
  draft
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E1](e1-terminal-shell.md) and accepted D2

## Outcome

The composer is the default focus owner, retains safe slash-command behavior,
and routes initialized idle plain text only to the existing reviewed Add Task
flow.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-030](../tasks/tui-030-make-composer-primary.md) | make the existing composer always available | TUI-013 |
| [TUI-031](../tasks/tui-031-implement-plain-text-input.md) | route idle plain text to reviewed task entry | TUI-030, D2/D5 |
| [TUI-032](../tasks/tui-032-add-contextual-command-discovery.md) | add contextual slash-command discovery | TUI-030 |

## Boundaries

E3 reuses existing reviewed task publication. It does not invent direct
publication, run instructions, steering, queuing, or typed-answer authority.

## Exit Gate

- The composer is visible and focused whenever no overlay or typed question
  owns focus.
- Every retained slash command remains executable and discoverable.
- Idle task-draft and rejected-state semantics have focused TUI regression
  checks.
