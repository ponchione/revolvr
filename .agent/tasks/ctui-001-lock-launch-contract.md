---
id: ctui-001-lock-launch-contract
status: completed
workflow: mixed-pass-v1
phase: implement
priority: 0
---

# CTUI-001 — Lock the Ordinary Launch Contract

- Status: Completed 2026-09-04
- Accepted plan: [Codex TUI task plan](../../docs/architecture/codex-tui/README.md)
- Accepted draft: [CTUI-001](../../docs/architecture/codex-tui/tasks/ctui-001-lock-launch-contract.md)
- Research source: [Codex TUI baseline](../../docs/architecture/codex-tui-baseline.md)
- Accepted contract: [Ordinary initialized launch contract](../../docs/architecture/codex-tui/launch-contract.md)
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

- [x] The launch matrix gives one deterministic route, output, and exit result for
  bare and explicit TUI invocations across TTY and redirected stdin/stdout,
  and states exact help and existing-subcommand behavior.
- [x] Literal loading and ready fixtures exist at 80x24 and the named narrow
  geometry; source citations and a complete field mapping make every visible
  value, loading value, and omission deliberate.
- [x] Terminal ownership and the minimal pending-frame boundary are unambiguous;
  the pending frame is not an additional accepted visual fixture.
- [x] CTUI-010 can implement dispatch and bootstrap ownership without choosing
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

## Completion Evidence

- Baseline guard: local `main` remained at
  `ac61907f7469f8a5836e9ee57a59066c854f2b4d`; `git rev-list --left-right
  --count origin/main...HEAD` returned `0 1`.
- Initial read-only selector: `go run ./cmd/revolvr status` selected only
  `ctui-001-lock-launch-contract` as the pending ready task.
- Fresh authenticated Codex 0.153.2 and initialized Revolvr captures were made
  in `standard-80x24` and `narrow-60x20` PTYs. The contract records UTC start
  times, byte counts, SHA-256 hashes, first-frame timing, literal fixtures, and
  pinned primary-source citations.
- Fresh redirected-I/O probes established current behavior. Focused CLI probes
  also confirmed `--version` is the only version route and that positional TUI
  input is rejected before command execution.
- A read-only acceptance script reported all 16 contract coverage checks
  `PASS`. A local Markdown validator reported `validated 36 local Markdown
  links and source line anchors in 8 changed files`.
- `git diff --check` returned no output; explicit no-index checks also passed
  for both untracked documentation files.
- Final read-only selector output after successor publication began `Total
  tasks: 2`, `Pending tasks: 1`, `Blocked tasks: 0`, `Completed tasks: 1`, and
  `Next task: ctui-010-launch-tui-by-default - CTUI-010 — Establish Early TUI
  Ownership`. `task list` reported CTUI-001 `completed` and CTUI-010 `pending`,
  `ready`, and selected with CTUI-001 as its only dependency.
