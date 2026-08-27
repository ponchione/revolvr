# TUI-055 — Move Change Summary into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Render the existing Change Summary projection in an overlay with parity for
selection, scrolling, refresh, and return behavior.

## Scope

- Render current changed-file, commit, and available exact-diff metadata.
- Preserve the distinction between canonical metadata and an exact diff
  artifact.
- Preserve scrolling, refresh, active-operation guards, and current entries.
- Keep old navigation entry during migration and add/retain command entry.

## Acceptance

- Every current Change Summary fact remains visible and source-traceable.
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

- No diff generation, Git query, evidence migration, or old-key removal.
