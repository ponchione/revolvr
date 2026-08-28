---
id: tui-012-prove-active-settlement
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
depends_on: tui-010-prove-shell-composition
---

# TUI-012 — Prove Active-Operation Settlement

- Status: Completed 2026-08-27
- Accepted publication source: `f12690b2be02ce3677aa0ab947b8910ad4f3f8e5`
- Accepted source:
  [TUI-012 draft](../../docs/architecture/tui-overhaul/tasks/tui-012-prove-active-settlement.md)
- Epic:
  [E1 — Prove the terminal shell](../../docs/architecture/tui-overhaul/epics/e1-terminal-shell.md)
- Depends on:
  [completed TUI-010](tui-010-prove-shell-composition.md)
- Design authority:
  [D3 transcript ownership](../../docs/architecture/tui-overhaul/README.md#d3--transcript-and-scrollback-ownership)

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

## Completion Evidence

- `TestTranscriptShellSettlement` proves Escape's composer guard and one-shot
  `c` cancellation, then exercises `q`, Ctrl-C, and composer Ctrl-C across the
  existing run-once, loop, task-run, and queue contexts.
- Matching terminal messages wait for cooperative cancellation, cleanup, and
  refresh; stale tokens cannot release quit or mutate a newer operation.
- Cancelled, failed, and completed final cells retain the live row until their
  append acknowledgement, enter the emitted-identity set once, and only then
  clear live state and release `tea.Quit`.
- A real Bubble Tea program writes the final cancelled cell exactly once and
  exits only after settlement.
- A compiled proof binary run on a pseudo-terminal disables bracketed paste,
  restores the cursor, exits after the final cell, and returns a prompt that
  accepts `printf 'PROMPT_OK\n'`.
- `gofmt -w internal/tui/model.go internal/tui/model_test.go` — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellSettlement'` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
