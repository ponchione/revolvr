# TUI-005 — Accept the Experience-State Snapshots

- Status: Draft; not canonical or runnable
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

- Every required state has one accepted text snapshot or wireframe.
- The snapshots implement D1-D6 without placeholders or contradictory routes.
- No fact is required simultaneously in transcript, live cell, session cell,
  overlay, and footer.
- Important state and action remain textual and visible at 40 columns.
- TUI-010 can copy the snapshots into test fixtures without product inference.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n \
  "Idle sketch|Running sketch|Completed sketch|failed|cancelled|needs-input|40-column" \
  docs/architecture/tui-overhaul/README.md
```

## Not Included

- No production code, test fixture implementation, or canonical task promotion.
