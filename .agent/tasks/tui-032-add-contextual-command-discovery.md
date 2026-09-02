---
id: tui-032-add-contextual-command-discovery
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-030-make-composer-primary
---

# TUI-032 — Add Contextual Command Discovery

- Status: Pending
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-032 draft](../../docs/architecture/tui-overhaul/tasks/tui-032-add-contextual-command-discovery.md)
- Epic:
  [E3 — Make the composer primary](../../docs/architecture/tui-overhaul/epics/e3-primary-composer.md)
- Depends on:
  [completed TUI-030](tui-030-make-composer-primary.md)
- Design authority:
  [accepted commands and history model](../../docs/architecture/tui-overhaul/reference/interaction-model.md#commands-and-history)

## Outcome

Show a compact slash-command popup filtered by the current composer text and
existing command availability.

## Scope

- Open discovery when the composer begins with `/` and close it under the
  accepted focus rules.
- Filter current commands by typed command prefix without changing their names.
- Use current guards to enable, disable, or explain each command.
- Support keyboard selection and preserve full Help as the exhaustive reference.
- Bound popup height/width and keep active cancellation text visible.

## Acceptance

- Every current command can be found by name and executed from discovery.
- Selected, disabled, and explanatory states are textual, not color-only.
- A 40-column popup stays within width and keeps the selected row visible.
- Command availability has one source: existing guards.
- Closing discovery preserves the underlying composer buffer and focus.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestCommandDiscovery'
go test ./internal/tui
```

## Not Included

- No new command, guard, Help redesign, or general overlay migration.
