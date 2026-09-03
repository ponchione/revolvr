---
id: tui-053-move-preflight-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-052-move-runs-overlay
---

# TUI-053 — Move Preflight into an Overlay

- Status: Completed 2026-09-03
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-053 draft](../../docs/architecture/tui-overhaul/tasks/tui-053-move-preflight-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-052](tui-052-move-runs-overlay.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

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

- `5` and `/preflight` now open the shared overlay while the retained Preflight
  page renderer remains the single content projection and D4 rollback path.
- Ready, failed, error, unavailable, and active-operation states remain
  textual; checks and explanations still come from the existing app callback.
- Check, refresh, run-once, loop-pass, and run-loop actions reuse the existing
  callbacks and guards, with notices kept local to the overlay.
- Exact source/composer return, 80-/40-column bounds, live-state preservation,
  and stale callback-result rejection pass focused coverage.
