# TUI-031 — Implement Accepted Plain-Text Input

- Status: Draft; not canonical or runnable
- Epic: [E3 — Make the composer primary](../epics/e3-primary-composer.md)
- Depends on: [TUI-030](tui-030-make-composer-primary.md), accepted
  D2/D5, and every app/domain prerequisite created by TUI-001

## Outcome

Route a non-command composer submission through the single app service accepted
in D2 and represent that submission once in the transcript.

## Scope

- Classify the current input state using the accepted D2 state table.
- Submit nonblank text only to the named app service; unavailable states return
  the accepted readable explanation without a domain effect.
- Preserve exact typed needs-input option identity and confirmation behavior.
- On success, commit one operator transcript cell tied to the returned domain
  identity.
- On failure, preserve the editable text and show one non-authoritative error.

## Acceptance

- Empty and whitespace-only submissions create no visible or durable effect.
- One successful Enter causes exactly one app action and one operator cell.
- Submission failure preserves the buffer and creates no success cell.
- Active input follows the accepted persistence/restart contract; unavailable
  active input remains unavailable.
- Tests prove typed needs-input cannot be bypassed by free-form text.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestPlainTextComposer'
go test ./internal/app
go test ./internal/tui
```

## Not Included

- No new app/domain capability, generalized chat, queue, command discovery, or
  task-publication shortcut.
