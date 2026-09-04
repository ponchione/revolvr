# CTUI-030 — Implement Startup Branch Transitions

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-020 and CTUI-025

## Outcome

Implement only the locked uninitialized and startup-error transitions inside
the shell established for initialized startup.

## Scope

- Transition to the CTUI-025 fixtures and enforce their composer, retry, exit,
  action, and diagnostic contracts.
- Keep startup diagnostics inside the managed shell.
- Preserve CTUI-020 delayed initialized startup and draft-transfer behavior as
  regressions rather than reimplementing it.

## Acceptance

- Uninitialized and startup-error outcomes render their locked fixtures and
  expose only approved actions.
- Retry and exit follow the locked transitions without duplicate shells or
  output outside the TUI.
- CTUI-020 delayed startup and draft transfer remain unchanged.
- Focused branch tests and PTY evidence make the slice independently
  demonstrable within one fresh pass.

## Verification

- Run deterministic branch, retry, exit, and CTUI-020 regression tests.
- Capture uninitialized and startup-error runs in a fixed-size PTY.
- Run `go test ./...`.

## Not Included

- New onboarding or product semantics, initialized startup redesign, byte-level
  terminal restoration, other startup branches, or new dependencies.
- Requirements from the retired TUI plan.
