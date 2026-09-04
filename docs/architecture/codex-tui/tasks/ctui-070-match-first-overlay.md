# CTUI-070 — Implement the First Focused Surface

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-065

## Outcome

Implement the locked focused surface and its evidence-based viewport policy.

## Scope

- Reproduce only the CTUI-065 surface, fixtures, entry, completion, dismissal,
  and viewport policy.
- Preserve prior draft, active state, committed scrollback, focus, cursor, and
  existing action guards across every return path.
- Use alternate screen only if CTUI-065 requires it.

## Acceptance

- Normal and narrow snapshots match the locked fixtures.
- Entry, completion, and dismissal obey the locked inline or alternate-screen
  lifecycle without duplicated or erased committed output.
- Return restores all locked shell state and cannot bypass action guards.
- Focused model and PTY evidence demonstrates the slice within one fresh pass.

## Verification

- Run locked snapshot, transition, guarded-action, and restoration tests.
- Capture viewport transitions and both return paths in a PTY.
- Run `go test ./...`.

## Not Included

- Unconditional alternate-screen use, surface or fixture selection during
  implementation, a second surface, new dependencies, or retired-plan
  requirements.
