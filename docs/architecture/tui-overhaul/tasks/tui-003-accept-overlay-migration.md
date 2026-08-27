# TUI-003 — Accept the Overlay Migration Order

- Status: Accepted 2026-08-27; decision-only and not runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: [TUI-002](tui-002-choose-transcript-ownership.md)
- Decision closed: D4

## Outcome

Record a reversible order for moving each focused view behind the shared
overlay shell without losing an existing route.

## Decision

Use the one-view-at-a-time order recorded in
[D4](../README.md#d4--overlay-migration):

1. Help in TUI-050;
2. Tasks in TUI-051;
3. Runs and its Run Detail child in TUI-052;
4. Preflight in TUI-053;
5. Workflow in TUI-054;
6. Change Summary in TUI-055;
7. Evidence in TUI-056;
8. Approval in TUI-057;
9. typed needs-input in TUI-058.

Each migration starts only after the preceding parity gate passes. The current
key and command entries stay available, first pointing to the current page and
then moving together to the accepted overlay. Page renderers remain as
rollback-only presentation until TUI-070 confirms that every E5 parity gate,
entry-route test, return/back test, and E6 geometry check passes and that no
page-only fact, action, guard, or error remains.

`internal/tui.StatusModel` owns overlay focus, overlay-local state, and the
saved composer/source return state. Canonical projections and live operation
state continue updating beneath the overlay. Root Escape restores the exact
saved composer state against the latest live state without changing terminal
history. Runs-to-Run-Detail is one explicit parent/child state that restores
run identity and list offset on back. Typed needs-input similarly retains its
Workflow or Approval parent; neither case introduces a general overlay stack.

No application callback, domain state, runtime dependency, or new app/domain
prerequisite is accepted.

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
git diff --check -- docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
rg -n "D4|overlay|migration|parity|return state|Run Detail" \
  docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
```

## Not Included

- No overlay shell or view migration implementation.
