# TUI-053 — Move Preflight into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Present the existing preflight projection and actions in an overlay without
changing admission authority.

## Scope

- Render every current preflight check, status, explanation, and next action.
- Route refresh/run actions through existing callbacks and guards.
- Keep old navigation entry during migration and add/retain command entry.
- Preserve underlying transcript, composer buffer, and active-operation state.

## Acceptance

- Overlay content matches the current app projection for pass, warning, and
  refusal cases.
- Safety/admission state remains textual and cannot be reduced to color.
- Refresh/action results update through app callbacks only.
- Dismissal restores the exact pre-open shell state.
- Existing preflight behavior tests remain green.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestPreflight.*Overlay'
go test ./internal/tui
```

## Not Included

- No preflight rule, safety policy, admission, or old-key removal.
