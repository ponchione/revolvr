# TUI-056 — Move Evidence into an Overlay

- Status: Draft; not canonical or runnable
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-050](tui-050-add-overlay-shell.md)

## Outcome

Render the current evidence projection in an overlay without changing evidence
authority or artifact provenance.

## Scope

- Render current evidence groups, criteria, statuses, artifact references, and
  warnings.
- Preserve selection, scrolling, refresh, and current validation/action routes.
- Keep IDs/paths available where needed for traceability while using accepted
  display compaction.
- Keep old navigation entry during migration and add/retain command entry.

## Acceptance

- Every current evidence item remains linked to its app projection/artifact.
- Missing, invalid, warning, and satisfied evidence remain distinct text.
- Refresh preserves selection by stable evidence identity where available.
- Dismissal restores the exact pre-open shell state.
- The overlay grants no evidence, verification, or completion authority.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestEvidence.*Overlay'
go test ./internal/tui
```

## Not Included

- No evidence schema, verifier, artifact loading boundary, or old-key removal.
