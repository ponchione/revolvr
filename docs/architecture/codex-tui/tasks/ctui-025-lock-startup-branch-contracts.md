# CTUI-025 — Lock Startup Branch Contracts

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-001

## Outcome

Decide the exact visible and actionable contracts for uninitialized workspaces
and startup errors before either branch is implemented.

## Scope

- Record fixed-geometry fixtures for uninitialized and startup-error states.
- Define composer visibility and an action matrix for each state, including
  retry and exit behavior.
- Decide how user-facing diagnostics are contained and treated without
  exposing progress logs or stack traces outside the shell.

## Acceptance

- Each branch has one literal fixture and complete action matrix.
- Composer, retry, exit, diagnostic, focus, and cursor behavior are explicit.
- CTUI-030 can implement both branches without selecting policy or presentation.

## Verification

- Compare fixtures with fresh side-by-side evidence and baseline citations.
- Review every visible element against the action and diagnostic matrices.

## Not Included

- Product code, dependency changes, or canonical publication.
- Initialized loading/ready behavior, onboarding semantics, restoration proof,
  or any startup branch beyond uninitialized and startup error.
