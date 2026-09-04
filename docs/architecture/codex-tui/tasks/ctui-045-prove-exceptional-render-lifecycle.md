# CTUI-045 — Prove Exceptional Render Lifecycle

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-040

## Outcome

Prove exceptional cleanup and redraw stability, then make an evidence-based
synchronized-update and cell-diff decision.

## Scope

- Prove panic restoration, representative resize behavior, and repeated redraw
  stability in the established inline shell.
- Verify enabled modes, cursor, canonical input, and echo after panic.
- Evaluate visible and raw PTY evidence for synchronized updates and cell-level
  diffing; record pass/no-change or one bounded follow-up proposal.
- Use existing renderer facilities only; this is an evidence gate.

## Acceptance

- Panic leaves a usable terminal with all enabled modes restored.
- Resize and repeated updates produce one stable surface without duplication,
  stale cells, crashes, or visible tearing in the captured cases.
- The evidence gate records exactly one justified result and preserves failing
  evidence when a follow-up is needed.
- The proof is repeatable and bounded to one fresh pass.

## Verification

- Run focused PTY panic, resize, and repeated-redraw cases with raw captures.
- Inspect mode pairing, cursor, echo, canonical input, and visual stability.
- Run `go test ./...`.

## Not Included

- Adapter, custom renderer, synchronized-update, or cell-diff implementation.
- Interactive-child handoff, product code unrelated to a proven lifecycle gap,
  new dependencies, or requirements from the retired TUI plan.
