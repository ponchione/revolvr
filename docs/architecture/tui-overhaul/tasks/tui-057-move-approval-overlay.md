# TUI-057 — Move Approval into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-056](tui-056-move-evidence-overlay.md)

## Outcome

Move the existing approval presentation and confirmation flow into an overlay
without weakening approval authority.

## Scope

- Render current approval request identity, evidence summary, state, and
  available decisions.
- Preserve explicit selection/confirmation and existing app callbacks.
- Preserve refusal, stale request, active-operation, and refresh behavior.
- Retain `A` and `/approval`; move both from the page to the same overlay only
  when this task's parity gate passes.
- Retain the Approval page renderer as the D4 rollback path until TUI-070.

## Acceptance

- No approval occurs from overlay open, navigation, or unconfirmed selection.
- Both `A` and `/approval` open that parity-tested path.
- One confirmed decision targets one exact approval identity.
- Stale or rejected results remain visible and cannot be rendered as success.
- Dismissal restores the exact pre-open shell state.
- Existing approval tests remain green.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestApproval.*Overlay'
go test ./internal/tui
```

## Not Included

- No approval policy, evidence rule, automatic approval, old-route removal, or
  page-renderer deletion.
