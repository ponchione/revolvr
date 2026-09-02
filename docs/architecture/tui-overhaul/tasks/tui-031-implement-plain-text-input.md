# TUI-031 — Route Idle Plain Text to Task Review

- Status: Completed 2026-09-02
- Epic: [E3 — Make the composer primary](../epics/e3-primary-composer.md)
- Depends on: [TUI-030](tui-030-make-composer-primary.md) and accepted D2/D5

## Outcome

Move initialized idle plain text into the existing reviewed Add Task flow
without treating it as a command, run instruction, answer, or queued message.

## Scope

- Classify the current input state using the accepted D2/D5 state table.
- In initialized idle state, transfer nonblank text to the existing task-entry
  state as the editable task body; create no task yet.
- Keep the current task/summary review and route its explicit confirmation
  through the existing `AddTask` callback backed by `app.AddTaskAndCommit`.
- Reject uninitialized, active-operation, unavailable, and error states with no
  app call, preserving the composer buffer and current domain state.
- Preserve exact typed needs-input option identity and confirmation behavior.

## Acceptance

- Empty and whitespace-only submissions create no visible or durable effect.
- Idle composer Enter opens one prefilled review and makes no app call.
- Review Enter causes exactly one existing task-publication action; Escape
  cancels without a write.
- Rejected plain text preserves the composer buffer and creates no transcript
  success cell or durable effect.
- Active text is never steered, queued, persisted, replayed, or automatically
  consumed after settlement.
- Tests prove typed needs-input cannot be bypassed by free-form text.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestPlainTextComposer|TestStatusModelTaskEntry'
go test ./internal/tui
```

## Not Included

- No new app/domain capability, generalized chat, queue, run instruction,
  command discovery, or task-publication shortcut.

## Completion Evidence

- Initialized idle text transfers into the existing editable Add Task review;
  only review confirmation invokes the existing publication callback.
- Blank/cancelled input and every rejected D2/D5 state have no app or
  transcript-success effect, while the rejected composer buffer stays intact.
- Active text remains inert through settlement, typed needs-input stays
  option-only, and focused, package, and full Go tests pass.
