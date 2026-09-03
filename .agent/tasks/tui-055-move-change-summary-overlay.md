---
id: tui-055-move-change-summary-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-054-move-workflow-overlay
---

# TUI-055 — Move Change Summary into an Overlay

- Status: Completed 2026-09-03
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

## Completion Evidence

- `d` and `/diff` now open the shared Change Summary overlay while the retained
  page renderer remains the D4 rollback path.
- Canonical changed-file, commit, and event metadata remain distinct from
  provenance-backed exact-diff artifact identities.
- Existing status and run-detail callbacks supply refresh data; overlay-local
  notices, active-run guards, and owner checks preserve focus and reject stale
  reloads.
- Focused coverage passes both entries, renderer parity, source selection,
  scrolling, long-path compaction, 80-/40-column bounds, and exact return.
