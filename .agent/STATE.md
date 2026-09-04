# Agent State

Updated: 2026-09-04

## Current Direction

- The transcript-first Revolvr TUI overhaul is rejected as the product
  baseline. Its preserved implementation is only the starting code for the
  accepted replacement sequence.
- The visible Codex TUI is now the baseline for Revolvr's TUI appearance and
  interaction presentation. Match it first; add Revolvr-specific presentation
  only after the baseline is established.
- The local baseline anchors are installed `codex-cli 0.153.2` and the pinned
  `.reference/codex` checkout at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`.
- Visual fidelity does not authorize copying, vendoring, or porting Codex
  source. Reimplementation remains in Go/Bubble Tea unless a later explicit
  decision changes that boundary.
- The accepted task graph and publication order are recorded in
  `docs/architecture/codex-tui/README.md`.
- CTUI-001 accepted the authoritative ordinary initialized launch contract in
  `docs/architecture/codex-tui/launch-contract.md` from fresh fixed-geometry
  executable evidence and pinned source citations.

## Task State

- Canonical task files: 2: one completed and one pending.
- Completed tasks: 1: `ctui-001-lock-launch-contract`.
- Pending tasks: 1: `ctui-010-launch-tui-by-default`.
- Accepted unpublished drafts: 10.
- CTUI-010 was published as CTUI-001's exact completion successor and was not
  executed in the completion pass.
- The former canonical and TUI-overhaul planning task trees are retired; Git
  history preserves them.

## Repository State

- Local `main` remains at
  `ac61907f7469f8a5836e9ee57a59066c854f2b4d`, exactly one commit ahead of
  `origin/main`; no history operation was performed.
- CTUI-001 changes are uncommitted documentation and durable task-state changes
  ready for review.
- No replacement UI code was implemented by CTUI-001.
- No dependency was added.

## Next Action

- Start a fresh pass and execute only
  `ctui-010-launch-tui-by-default` from its canonical task file.
- Treat `docs/architecture/codex-tui/launch-contract.md` as authoritative; do
  not reopen its launch, stream, help, field, or fixture decisions.
- Do not recover or execute any later unpublished draft in the same pass.

## CTUI-001 Completion Verification

- Fresh authenticated Codex and initialized Revolvr launches were captured in
  fixed `standard-80x24` and `narrow-60x20` PTYs. Redirected stdin and stdout
  were probed separately.
- Literal Codex loading/ready evidence, Revolvr fixtures and field mapping,
  route/I/O matrix, exact help, terminal ownership, and task boundaries are
  locked in the accepted contract.
- All 16 acceptance-coverage checks passed; all 36 local Markdown links and
  source line anchors in the 8 changed files resolved. All 48 literal card
  rows have their locked 41- or 51-column width.
- `git diff --check` and both untracked-file no-index checks passed. The final
  selector reported 2 total, 1 pending, 0 blocked, and 1 completed task, with
  only `ctui-010-launch-tui-by-default` next. The task inventory reported it
  pending, ready, and selected.
- Blockers: none.
