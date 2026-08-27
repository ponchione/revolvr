# TUI-003 — Accept the Overlay Migration Order

- Status: Draft; not canonical or runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: [TUI-002](tui-002-choose-transcript-ownership.md)
- Decision closed: D4

## Outcome

Record a reversible order for moving each focused view behind the shared
overlay shell without losing an existing route.

## Scope

- Accept or replace the proposed incremental overlay migration.
- Order Help, Tasks, Runs/Run Detail, Preflight, Workflow, Change Summary,
  Evidence, Approval, and typed needs-input migrations.
- Define how old keys coexist with command entry during migration and the
  evidence required before each page-only path is removable.
- Define overlay dismissal, nested Runs-to-Run-Detail behavior, and return-state
  ownership at the product level.

## Acceptance

- D4 names one migration order and a parity gate for each focused view.
- Every current action remains reachable until its accepted replacement passes.
- Page removal is gated by tested command/key entry and overlay return behavior.
- The order does not require changing application callbacks or domain state.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n "D4|overlay|migration|parity" docs/architecture/tui-overhaul
```

## Not Included

- No overlay shell or view migration implementation.
