# TUI-005 — Accept the Experience-State Snapshots

- Status: Accepted 2026-08-27; documentation-only decision complete
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: [TUI-000](tui-000-resolve-source-reuse.md),
  [TUI-001](tui-001-resolve-composer-semantics.md),
  [TUI-002](tui-002-choose-transcript-ownership.md),
  [TUI-003](tui-003-accept-overlay-migration.md), and
  [TUI-004](tui-004-accept-session-header.md)

## Outcome

Produce the accepted source snapshots that judge all later presentation work.

## Scope

- Replace the illustrative sketches with accepted initialized-idle,
  uninitialized, running, completed, failed, cancelled, needs-input, overlay,
  and 40-column narrow states.
- Annotate each state with committed transcript, replaceable live cell,
  composer, overlay, and transient footer ownership.
- Start each process-state snapshot with the accepted `session-start` facts and
  point-in-time initialization wording; do not add a clear action or duplicate
  those facts in persistent chrome.
- Record the normal width, minimum supported width, wrapping/truncation rules,
  and behavior below the minimum.
- Settle exact operator-visible wording for safety state, cancellation, current
  work, terminal outcome, and next useful action.

## Acceptance

- **Passed 2026-08-27.** Every required state has one accepted literal source
  snapshot in the [design authority](../README.md#accepted-experience-state-snapshots).
- The snapshots implement D1-D6 without placeholders, contradictory routes,
  duplicate ownership, a clear action, or a new app/domain capability.
- Safety, cancellation, current work, terminal outcome, next action, focus, and
  typed confirmation remain textual and visible at 40 columns.
- Normal width is 80 columns, minimum supported width is 40 columns, and wrap,
  truncation, scrolling, and below-minimum behavior are explicit.
- TUI-010 can copy the source rows into test fixtures without product
  inference; it is published as pending and remains unstarted.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n \
  "initialized|uninitialized|running|completed|failed|cancelled|needs-input|overlay|40-column|session-start" \
  docs/architecture/tui-overhaul/README.md
```

## Completed Result

- Accepted nine literal source snapshots plus exact cancellation-requested,
  blocked, and safety-stop cells.
- Assigned every visible fact to committed session/transcript, replaceable
  live, composer, overlay, or transient-footer ownership.
- Closed E0 without changing product code, publishing TUI-010, adding a
  dependency, or creating a runtime capability.

## Not Included

- No production code, test fixture implementation, or canonical task promotion.
