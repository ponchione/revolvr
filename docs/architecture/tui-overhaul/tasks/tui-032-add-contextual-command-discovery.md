# TUI-032 — Add Contextual Command Discovery

- Status: Completed 2026-09-02
- Epic: [E3 — Make the composer primary](../epics/e3-primary-composer.md)
- Depends on: [TUI-030](tui-030-make-composer-primary.md)

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

## Completion Evidence

- Leading-slash input now filters every retained command with exact-match
  precedence, bounded keyboard selection, textual descriptions, and shared
  guard explanations.
- Discovery stays within 40 display columns and five command rows while keeping
  its selected row and active cancellation state visible.
- Escape preserves composer text/focus, bare `/` retains the full-Help route,
  and focused, package, and full Go tests pass.
