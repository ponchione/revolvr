# TUI-030 — Make the Composer the Primary Focus

- Status: Draft; not canonical or runnable
- Epic: [E3 — Make the composer primary](../epics/e3-primary-composer.md)
- Depends on: [TUI-013](tui-013-install-terminal-shell.md)

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
