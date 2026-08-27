# TUI-012 — Prove Active-Operation Settlement

- Status: Draft; not canonical or runnable
- Epic: [E1 — Prove the terminal shell](../epics/e1-terminal-shell.md)
- Depends on: [TUI-010](tui-010-prove-shell-composition.md)

## Outcome

Prove that the new shell preserves the existing cancellation and quit contract
while a live operation is unsettled.

## Scope

- Exercise Escape, `c`, `q`, and Ctrl-C only where current guards allow them.
- Verify the cooperative cancellation signal reaches the active operation.
- Hold the domain result open long enough to prove the program waits for
  settlement instead of erasing the live cell or exiting early.
- Reconcile the matching live operation identity to one final source cell,
  append it once, and only then clear the live cell.
- Verify the final terminal outcome is emitted once and the terminal restores
  after settlement.

## Acceptance

- Each accepted cancel/quit key has one observable effect.
- Quit does not complete before the active domain result settles.
- Late messages from the settled operation cannot mutate a newer shell state.
- The per-session emitted-identity set prevents duplicate final rows.
- Cancellation, failure, and successful completion remain distinct text.
- Existing operation guards and contexts remain the authority.

## Verification

```bash
gofmt -w internal/tui/model.go internal/tui/model_test.go
go test ./internal/tui -run 'TestTranscriptShellSettlement'
go test ./internal/tui
```

## Not Included

- No resize/reflow, new cancellation mechanism, live semantic cell, or queued
  operator input.
