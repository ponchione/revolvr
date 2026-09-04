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

## Task State

- Canonical task files: 1.
- Pending tasks: 1: `ctui-001-lock-launch-contract`.
- Accepted unpublished drafts: 11.
- CTUI-010 is the exact completion successor for CTUI-001.
- The former canonical and TUI-overhaul planning task trees are retired; Git
  history preserves them.

## Repository State

- The preserved TUI-overhaul implementation, planning reset, Codex research,
  accepted replacement plan, and CTUI-001 publication form the clean baseline
  for the next fresh pass.
- No replacement UI code was implemented in this reset.
- No dependency was added.

## Next Action

- Start a fresh thread and execute only the canonical CTUI-001 documentation
  task.
- Capture and accept the ordinary initialized loading/ready launch contract at
  80x24 and one named narrow geometry, including field and CLI/I/O matrices.
- After CTUI-001 is terminally complete and verified, publish CTUI-010 only and
  stop without implementing it.

## Baseline Verification

- Task selected: CTUI-001 for the next fresh pass; it is not started here.
- Changed Go files match `gofmt`; `go test ./...` passes.
- `go run ./cmd/revolvr task list` and `go run ./cmd/revolvr status` pass with
  one ready pending task and select `ctui-001-lock-launch-contract`.
- Root and TUI help checks, documentation-link validation, and
  `git diff --check` pass.
- What remains: execute the accepted CTUI task sequence one fresh task at a
  time.
- Blockers: none.
