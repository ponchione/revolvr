---
id: tui-054-move-workflow-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-053-move-preflight-overlay
---

# TUI-054 — Move Workflow into an Overlay

- Status: Completed 2026-09-03
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-054 draft](../../docs/architecture/tui-overhaul/tasks/tui-054-move-workflow-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-053](tui-053-move-preflight-overlay.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

## Outcome

Present the existing autonomous workflow projection and controls in an overlay
without changing workflow semantics.

## Scope

- Render current workflow phase, pass/limit, task/run identity, stop evidence,
  and available actions.
- Preserve start/continue/cancel/refresh callbacks and operation guards.
- Show needs-input state but leave typed answer interaction on its current path
  until TUI-058.
- Retain `6` and `/workflow`; move both from the page to the same overlay only
  when this task's parity gate passes.
- Preserve underlying live updates and return state.
- Retain the Workflow page renderer as the D4 rollback path until TUI-070.

## Acceptance

- Every workflow state and control exposed before migration remains reachable.
- Both `6` and `/workflow` open that parity-tested path.
- Active updates behind the overlay reconcile without duplicating history.
- Operation guards and app results remain the only source of availability.
- Needs-input state stays visible and its existing answer path still works.
- Dismissal restores the exact pre-open shell state.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestWorkflow.*Overlay'
go test ./internal/tui
```

## Not Included

- No workflow policy, typed-answer migration, queue behavior, old-route
  removal, or page-renderer deletion.

## Completion Evidence

- `6`, `/workflow`, and the Tasks handoff now open the shared Workflow overlay
  while the retained page renderer remains the D4 rollback path.
- Selector identity, notices, scrolling, refresh, loading results, and stale
  callback ownership remain local to the overlay.
- Existing autonomous task, queue, cancellation, refresh, and typed-answer
  callbacks and guards remain authoritative and reachable.
- Focused coverage passes renderer parity, every lifecycle projection, current
  task/run/pass/stop evidence, 80-/40-column bounds, live exact-once
  settlement, needs-input handling, and exact source/composer return.
