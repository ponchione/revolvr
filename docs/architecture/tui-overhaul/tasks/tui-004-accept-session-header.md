# TUI-004 — Accept the Session-Header Lifecycle

- Status: Draft; not canonical or runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: [TUI-002](tui-002-choose-transcript-ownership.md)
- Decision closed: D6

## Outcome

Define when the Revolvr session identity appears and which current-context
facts remain outside committed history.

## Scope

- Accept or replace the proposed one-time session transcript cell.
- Define startup, explicit clear, refresh, resize, restart, and overlay behavior
  for the cell.
- Specify its exact source facts, including initialization and project identity.
- Assign active-only context to the live/footer region so it is not duplicated
  in session history.

## Acceptance

- D6 has one lifecycle with no persistent duplicate dashboard header.
- Refresh and resize do not create another session cell.
- Restart and explicit clear behavior are intentional and snapshot-testable.
- Every displayed fact has one source and one presentation owner.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n "D6|session header|session cell|explicit clear" docs/architecture/tui-overhaul
```

## Not Included

- No shell code, general transcript cells, footer redesign, or clear command.
