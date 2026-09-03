---
id: tui-055-move-change-summary-overlay
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-054-move-workflow-overlay
---

# TUI-055 — Move Change Summary into an Overlay

- Status: Pending
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-055 draft](../../docs/architecture/tui-overhaul/tasks/tui-055-move-change-summary-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-054](tui-054-move-workflow-overlay.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

## Outcome

Render the existing Change Summary projection in an overlay with parity for
selection, scrolling, refresh, and return behavior.

## Scope

- Render current changed-file, commit, and available exact-diff metadata.
- Preserve the distinction between canonical metadata and an exact diff
  artifact.
- Preserve scrolling, refresh, active-operation guards, and current entries.
- Retain `d` and `/diff`; move both from the page to the same overlay only when
  this task's parity gate passes.
- Retain the Change Summary page renderer as the D4 rollback path until
  TUI-070.

## Acceptance

- Every current Change Summary fact remains visible and source-traceable.
- Both `d` and `/diff` open that parity-tested path.
- Metadata is never relabeled as file-content diff evidence.
- Narrow rendering is bounded and long paths use the accepted compaction.
- Dismissal restores the exact pre-open shell state.
- No Git or artifact store is read directly by overlay code.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestChangeSummary.*Overlay'
go test ./internal/tui
```

## Not Included

- No diff generation, Git query, evidence migration, old-route removal, or
  page-renderer deletion.
