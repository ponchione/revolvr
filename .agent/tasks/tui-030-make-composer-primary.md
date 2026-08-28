---
id: tui-030-make-composer-primary
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-013-install-terminal-shell
---

# TUI-030 — Make the Composer the Primary Focus

- Status: Completed 2026-08-28
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-030 draft](../../docs/architecture/tui-overhaul/tasks/tui-030-make-composer-primary.md)
- Epic:
  [E3 — Make the composer primary](../../docs/architecture/tui-overhaul/epics/e3-primary-composer.md)
- Depends on:
  [completed TUI-013](tui-013-install-terminal-shell.md)
- Design authority:
  [accepted composer semantics](../../docs/architecture/tui-overhaul/README.md#d2--plain-text-composer-meaning)

## Outcome

Remove the inactive composer mode so ordinary focus begins in an always-visible
composer without changing what submitted commands do.

## Scope

- Replace `› / for commands` activation with the accepted always-visible prompt.
- Preserve current slash command names, parsing, validation, guards, and Enter
  dispatch.
- Preserve non-command text until TUI-031 transfers or rejects it under the
  accepted D2/D5 state table; never dispatch it through slash-command parsing.
- Define focus transitions for an empty buffer, populated buffer, command
  popup, overlay, typed question, and active operation.
- Implement the accepted Escape behavior for each focus state.
- Preserve Ctrl-C, `q`, and active-operation settlement rules.

## Acceptance

- An operator can type immediately when no overlay or typed question owns focus.
- Every current slash command still executes through its existing action path.
- Escape behavior matches one tested state table and cannot accidentally quit
  or discard populated input.
- Opening and closing non-composer focus restores the prior composer buffer.
- Plain text is preserved without dispatch until TUI-031 supplies the accepted
  reviewed task-draft route.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestComposer|TestCommand'
go test ./internal/tui
```

## Not Included

- No plain-text action, contextual command popup, overlay shell, or command
  policy change.

## Completion Evidence

- The existing composer starts focused with the accepted always-visible prompt
  and 80-/40-column discovery footer; pages and typed questions retain their
  current input ownership.
- Slash commands keep their existing dispatch paths. Non-command and blank
  submissions remain in the composer without entering slash-command parsing.
- Populated Escape preserves input, empty Escape yields focus to the retained
  single-key actions, bare-slash Help restores its prior buffer, and typed
  question dismissal restores the underlying composer state.
- Active Escape and `c` request cancellation without clearing composer text;
  `q` and Ctrl-C retain delayed quit until matching operation settlement.
- Focused composer/command, TUI package, CLI, and full Go tests pass.
