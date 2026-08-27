# TUI-058 — Move Typed Needs-Input into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-057](tui-057-move-approval-overlay.md)

## Outcome

Make typed needs-input questions a modal overlay that preserves exact question
and option authority.

## Scope

- Render the current question text, exact option identities, labels, and
  descriptions from the app projection.
- Require explicit selection and confirmation before submitting one answer.
- Route the answer through the existing typed callback and exact selector.
- Preserve cancel/back behavior, error recovery, refresh, and stale-question
  handling.
- Retain context-specific `a` from Workflow or Approval and
  `/answer <option-id>`; move both to the same typed child only when this
  task's parity gate passes.
- Restore composer focus only after the question no longer owns focus.
- Retain the prior typed-answer presentation as the D4 rollback path until
  TUI-070.

## Acceptance

- Free-form composer text cannot answer a typed question.
- Both retained entry routes preserve the exact Workflow or Approval parent,
  selection, and scroll state.
- One completed two-step confirmation submits one exact question/option
  identity.
- Failure preserves the question and selection with a readable error.
- Stale results cannot answer or dismiss a newer question.
- Existing typed-response tests and new overlay parity tests pass.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go internal/tui/checkpoint_test.go
go test ./internal/tui -run 'TestNeedsInput.*Overlay|Test.*TypedResponse'
go test ./internal/tui
```

## Not Included

- No question schema, answer authority, generalized form/overlay stack,
  workflow policy change, old-route removal, or page-renderer deletion.
