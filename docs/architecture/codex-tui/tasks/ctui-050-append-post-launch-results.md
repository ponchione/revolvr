# CTUI-050 — Append One Finalized Result

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-045

## Outcome

Deliver one tracer that commits one finalized Revolvr result exactly once to
normal terminal scrollback.

## Scope

- Drive one existing operation from the ready shell to its finalized result.
- Append only that finalized result above the managed region; keep live states
  replaceable and domain finality authoritative.
- Preserve clean startup and stable committed output across active redraws.

## Acceptance

- The selected finalized result appears in normal scrollback exactly once.
- Redraws neither rewrite nor duplicate it, and restart does not replay history
  into the opening frame.
- Existing domain finality and settlement guards remain unchanged.
- Focused terminal evidence demonstrates the tracer within one fresh pass.

## Verification

- Drive deterministic live-to-final state and count committed output.
- Capture settlement and later redraws in a PTY.
- Run focused tests and `go test ./...`.

## Not Included

- Additional result types, history browser, command discovery, focused
  surfaces, domain changes, new dependencies, or retired-plan requirements.
