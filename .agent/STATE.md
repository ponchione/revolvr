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
- CTUI-010 implemented that contract's shared launch dispatcher, exact TTY
  gate, early inline Bubble Tea ownership, asynchronous bootstrap, and removal
  of startup history emission without implementing CTUI-020 presentation.

## Task State

- Canonical task files: 3: two completed and one pending.
- Completed tasks: 2: `ctui-001-lock-launch-contract` and
  `ctui-010-launch-tui-by-default`.
- Pending tasks: 1: `ctui-020-match-initial-frame`.
- Accepted unpublished drafts: 9.
- CTUI-020 was published as CTUI-010's exact completion successor and was not
  executed in the completion pass.
- The former canonical and TUI-overhaul planning task trees are retired; Git
  history preserves them.

## Repository State

- Local `main`, `HEAD`, and `origin/main` remain at
  `e0b6372f54f8721aa59a767376b0266d29f97876`; no history operation was
  performed.
- CTUI-010 product code, tests, existing-dependency source replacement,
  documentation, and durable task-state changes are uncommitted and ready for
  review.
- Bubble Tea remains v1.3.4 and the normalized 77-module identity/version set
  is unchanged. A provenance-recorded local source replacement removes only
  v1.3.4's pre-main terminal probe so the accepted gate can emit zero bytes.
- No dependency or Codex source was added.

## Next Action

- Start a fresh pass and execute only `ctui-020-match-initial-frame` from its
  canonical task file.
- Treat `docs/architecture/codex-tui/launch-contract.md` as authoritative and
  replace CTUI-010's disposable `Loading…` tracer with the locked initialized
  loading and ready fixtures without reopening launch or stream decisions.
- Do not recover or execute any later unpublished draft in the same pass.

## CTUI-010 Completion Verification

- Focused tests cover shared dispatch, exact help, parsing and version bypass,
  stdin-first stream checks, every TTY matrix row, zero pre-bootstrap refusal,
  process initialization, blocked bootstrap, same-program completion, and no
  startup history. `go test ./...` passed.
- All six redirected 80x24 PTY rows exited 1 with empty stdout and exact
  24-byte stdin or 25-byte stdout errors. Bare and explicit interactive 80x24
  captures were byte-identical at 241 bytes, exited 0 after Ctrl-C, and had
  empty stderr; inspected live replays contained no startup history or new
  fixture presentation.
- Root/TUI help, `--version`, `config check`, `status`, unknown inputs,
  positional arguments, and malformed flags passed focused executable checks.
- `gofmt`, `git diff --check`, and no-index whitespace checks for every
  untracked path passed. The normalized module list retained all 77 baseline
  identities and versions.
- The final selector reported 3 total, 1 pending, 0 blocked, and 2 completed
  tasks, with only `ctui-020-match-initial-frame` next and ready.
- Blockers: none.
