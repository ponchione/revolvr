---
id: ctui-001-lock-launch-contract
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 0
---

# CTUI-001 — Lock the Ordinary Launch Contract

- Status: Pending
- Accepted plan: [Codex TUI task plan](../../docs/architecture/codex-tui/README.md)
- Accepted draft: [CTUI-001](../../docs/architecture/codex-tui/tasks/ctui-001-lock-launch-contract.md)
- Research source: [Codex TUI baseline](../../docs/architecture/codex-tui-baseline.md)
- Depends on: None
- Completion handoff: publish only CTUI-010; do not execute it in this pass

## Outcome

Lock the complete ordinary initialized launch contract so later launch, frame,
and lifecycle tasks can implement it without inventing behavior.

## Scope

- Record the accepted behavior of bare and explicit TUI launch, help, and every
  existing subcommand for an initialized workspace.
- Decide redirected stdin and stdout routing, output, and exit status for each
  applicable launch route.
- Record literal, source-cited loading and ready visual fixtures at 80x24 and
  at one explicitly named narrow geometry.
- Map every visible fixture field to its Revolvr value and state its loading or
  omission treatment when that value is unavailable.
- Identify initial terminal ownership and the boundary between CTUI-010's
  minimal pending frame and the locked fixtures implemented by CTUI-020.
- Distinguish observed Codex evidence from Revolvr decisions.

## Acceptance

- The launch matrix gives one deterministic route, output, and exit result for
  bare and explicit TUI invocations across TTY and redirected stdin/stdout,
  and states exact help and existing-subcommand behavior.
- Literal loading and ready fixtures exist at 80x24 and the named narrow
  geometry; source citations and a complete field mapping make every visible
  value, loading value, and omission deliberate.
- Terminal ownership and the minimal pending-frame boundary are unambiguous;
  the pending frame is not an additional accepted visual fixture.
- CTUI-010 can implement dispatch and bootstrap ownership without choosing
  styling, draft, or startup-branch presentation semantics, and CTUI-020 can
  implement the initialized fixtures without making new visual decisions.

## Verification

- Compare the launch matrix with current CLI behavior and fresh TTY and
  redirected-I/O evidence.
- Compare each fixture cell and field mapping with the installed Codex launch
  and pinned primary-source citations.
- Confirm uninitialized and startup-error fixtures remain exclusively in
  CTUI-025.
- Run `git diff --check` and the read-only task selector.

## Not Included

- Product code, UI implementation, dependency changes, or execution of
  CTUI-010.
- Editable-draft implementation, startup-branch fixtures, terminal lifecycle
  proof, or later interactions.
