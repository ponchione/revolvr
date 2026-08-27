# E4 — Surface Runs and Loops Live

- Status: Draft; not canonical or runnable
- Design authority: [TUI overhaul](../README.md)
- Depends on: [E2](e2-semantic-transcript.md) and [E3](e3-primary-composer.md)

## Outcome

An operator can understand and interrupt the active operation from one bounded
live region without reading a progress log or leaving the transcript.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-040](../tasks/tui-040-render-live-operation.md) | render one live operation cell | TUI-022, TUI-030 |

## Boundaries

E4 presents existing progress and terminal outcomes. D5 rejects active
steering and queued/deferred operator messages, so E4 does not render or create
them. The autonomous task queue remains a distinct scheduler-owned operation.

## Exit Gate

- Active operation state occupies one bounded live region.
- Single pass, loop, autonomous task, and queue modes remain distinguishable.
- Cancellation and settlement retain existing domain behavior.
