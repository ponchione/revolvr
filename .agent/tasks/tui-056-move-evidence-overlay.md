---
id: tui-056-move-evidence-overlay
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-055-move-change-summary-overlay
---

# TUI-056 — Move Evidence into an Overlay

- Status: Completed 2026-09-03
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-056 draft](../../docs/architecture/tui-overhaul/tasks/tui-056-move-evidence-overlay.md)
- Epic:
  [E5 — Move focused work to overlays](../../docs/architecture/tui-overhaul/epics/e5-overlays.md)
- Depends on:
  [completed TUI-055](tui-055-move-change-summary-overlay.md)
- Design authority:
  [accepted overlay migration](../../docs/architecture/tui-overhaul/README.md#d4--overlay-migration)

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

- `e` and `/evidence` open the shared Evidence overlay while the retained page
  renderer remains the D4 rollback path.
- Canonical and autonomous evidence retain run, task, criterion, status,
  warning, artifact, source, hash, and occurrence identities from app-supplied
  projections.
- Stable autonomous selection and selected-run refresh survive reordering;
  stale loads cannot replace a newer owner, and receipt validation remains on
  the loaded canonical run.
- Focused coverage passes renderer parity, missing/invalid/warning/satisfied
  distinctions, scrolling, 80-/40-column bounds, and exact return.
