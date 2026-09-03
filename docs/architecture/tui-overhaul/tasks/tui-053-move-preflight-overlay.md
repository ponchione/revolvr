# TUI-053 — Move Preflight into an Overlay

- Status: Completed 2026-09-03
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-052](tui-052-move-runs-overlay.md)

## Outcome

Present the existing preflight projection and actions in an overlay without
changing admission authority.

## Scope

- Render every current preflight check, status, explanation, and next action.
- Route refresh/run actions through existing callbacks and guards.
- Retain `5` and `/preflight`; move both from the page to the same overlay only
  when this task's parity gate passes.
- Preserve underlying transcript, composer buffer, and active-operation state.
- Retain the Preflight page renderer as the D4 rollback path until TUI-070.

## Acceptance

- Overlay content matches the current app projection for pass, warning, and
  refusal cases.
- Both `5` and `/preflight` open that parity-tested path.
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

- No preflight rule, safety policy, admission, old-route removal, or page-
  renderer deletion.

## Completion Evidence

- Both retained entries reach the shared overlay and reuse the retained
  Preflight renderer, callback, and textual check projection.
- Check, refresh, run-once, loop-pass, and run-loop controls retain their
  existing callbacks and admission/active-operation guards.
- Exact source/composer return, 80-/40-column bounds, callback errors,
  unavailable actions, and stale result ownership pass focused coverage.
- No preflight rule, safety authority, dependency, or rollback page renderer
  changed.
