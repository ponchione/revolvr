# TUI-055 — Move Change Summary into an Overlay

- Status: Completed 2026-09-03
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-054](tui-054-move-workflow-overlay.md)

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

- Both retained entries reach the shared overlay and reuse the existing Change
  Summary renderer over app-supplied run and autonomous projections.
- Changed-file, commit, event, and exact-diff artifact identities retain their
  source labels and distinct evidence meaning.
- Refresh, active-operation guards, scrolling, narrow rendering, selected-run
  context, stale-result rejection, and exact source/composer return pass.
- No Git query, artifact read, application authority, dependency, or rollback
  page renderer changed.
