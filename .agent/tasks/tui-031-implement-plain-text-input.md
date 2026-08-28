---
id: tui-031-implement-plain-text-input
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-030-make-composer-primary
---

# TUI-031 — Route Idle Plain Text to Task Review

- Status: Pending
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-031 draft](../../docs/architecture/tui-overhaul/tasks/tui-031-implement-plain-text-input.md)
- Epic:
  [E3 — Make the composer primary](../../docs/architecture/tui-overhaul/epics/e3-primary-composer.md)
- Depends on:
  [completed TUI-030](tui-030-make-composer-primary.md) and accepted D2/D5
- Design authority:
  [accepted composer semantics](../../docs/architecture/tui-overhaul/README.md#d2--plain-text-composer-meaning)

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
