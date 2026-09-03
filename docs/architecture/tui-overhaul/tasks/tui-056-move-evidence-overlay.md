# TUI-056 — Move Evidence into an Overlay

- Status: Completed 2026-09-03
- Epic: [E5 — Move focused work to overlays](../epics/e5-overlays.md)
- Depends on: [TUI-055](tui-055-move-change-summary-overlay.md)

## Outcome

Render the current evidence projection in an overlay without changing evidence
authority or artifact provenance.

## Scope

- Render current evidence groups, criteria, statuses, artifact references, and
  warnings.
- Preserve selection, scrolling, refresh, and current validation/action routes.
- Keep IDs/paths available where needed for traceability while using accepted
  display compaction.
- Retain `e` and `/evidence`; move both from the page to the same overlay only
  when this task's parity gate passes.
- Retain the Evidence page renderer as the D4 rollback path until TUI-070.

## Acceptance

- Every current evidence item remains linked to its app projection/artifact.
- Both `e` and `/evidence` open that parity-tested path.
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

- No evidence schema, verifier, artifact loading boundary, old-route removal,
  or page-renderer deletion.

## Completion Evidence

- Both retained entries reach the shared overlay and reuse the Evidence page
  renderer over existing app-supplied run and autonomous projections.
- Evidence groups retain textual status, missing, invalid, warning, satisfied,
  artifact, source, hash, run, occurrence, and criterion traceability.
- Refresh preserves stable source identity, stale results retain the current
  owner, receipt validation targets the loaded run, and exact dismissal passes.
- No evidence schema, verifier, artifact reader, application authority,
  dependency, or rollback renderer changed.
