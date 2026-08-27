# Agent Handoff

Updated: 2026-08-27

## Where We Stopped

- Added `/.reference/` to the root ignore rules and shallow-cloned the official
  `openai/codex` repository to `.reference/codex` at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`; the reference TUI is in
  `codex-rs/tui`.
- Fixed TUI task creation without weakening preflight: it refuses a dirty
  worktree, creates the canonical task, and path-scoped commits only that new
  file. CLI task add/import remains review-and-commit.
- Added a real-Git regression proving the new task is the only committed path,
  the worktree is clean, and attended preflight admission succeeds.
- Collapsed the dashboard into a transcript-first operator console: one-line
  header, latest-run operator status plus semantic timeline, concise idle and
  uninitialized states, persistent `› / for commands` affordance, and a
  two-line-at-most narrow dashboard footer with active cancellation visible.
- Removed dashboard task counts, latest/recent run summaries, and raw events;
  those projections remain in Tasks, Runs, Run Detail, Workflow, and Help.
- Compacted dashboard activity to the latest eight meaningful semantic
  transitions, omitting low-level Codex lifecycle noise and the duplicated
  full task body; Run Detail retains the complete timeline.
- Removed redundant Transcript/Activity labels and dashboard UUID rows, and
  shortened file lists, commit hashes, receipt paths, and warning prefixes so
  the 80-column operator view remains readable without wrapped log walls.
- Simplified shared terminal styles to default/bold/dim text plus ANSI cyan,
  green, and red. Keyboard actions, focused views, scrolling, refresh,
  commands, typed responses, safety state, and cancellation are unchanged.
- Decomposed the draft TUI overhaul plan under
  `docs/architecture/tui-overhaul/`: `README.md` holds the product/design
  authority and index, `epics/` has one file for each of eight epics, and
  `tasks/` now has 33 audited, bounded task files after D5 removed TUI-041.
- Split composite work into separate product decisions, shell resize and
  settlement proofs, independently reachable focused-view migrations,
  scrollback and terminal-lifecycle checks, and documentation and final
  closeout tasks. Every task states dependencies, scope, acceptance,
  verification, and exclusions.
- The draft does not publish runnable `.agent/tasks`. TUI-000 and D1 are
  accepted: Revolvr will reimplement the accepted Codex interaction behavior
  in its existing Go/Bubble Tea TUI; the pinned Codex source and snapshots are
  behavioral evidence only, and no implementation source may be copied or
  ported. ADR-025 remains unchanged.
- Completed the pinned Codex TUI behavioral study and added exactly four
  evidence documents under `docs/architecture/tui-overhaul/reference/`:
  the reference index/task lookup, interaction model, terminal mechanics, and
  current Revolvr mapping. The consumed study prompt was removed.
- The references trace shell ownership, transcript settlement/replay, composer
  and interrupt behavior, overlays/questions/approvals, terminal
  scrollback/reflow/restoration, styles, tests, snapshots, and current Go seams.
  Every retained TUI-010 through TUI-072 proof/implementation task has a
  relevant link or an explicit no-useful-analog entry.
- TUI-001 is accepted and closes D2/D5. Initialized idle plain text opens the
  existing reviewed Add Task draft; uninitialized, active, and unavailable
  text rejects without an app effect; typed needs-input remains exact option-
  only input. Active steering and queued/deferred operator messages are not
  supported.
- TUI-031 is narrowed to the reviewed idle task-draft route, TUI-041 is removed,
  and no app/domain prerequisite was added. D3, D4, and D6 remain open; their
  leading gaps are the Go/Bubble Tea shell/scrollback proof, overlay parity,
  and session-header lifecycle.
- Committed the task-publication fix, console presentation/test update,
  planning tree, behavioral study, and durable-state snapshot as `24be655`
  (`feat: prepare transcript-first TUI overhaul`) and pushed `main` to
  `origin/main` with raw Git.
- Consumed and removed `docs/architecture/tui-overhaul/TUI_001_DECISION_PROMPT.md`.
  No product code, runnable task, or commit changed in the TUI-001 pass.

## Exact Next Command

Run one fresh Codex pass for TUI-002:

```bash
codex exec "Execute TUI-002 from docs/architecture/tui-overhaul/tasks/tui-002-choose-transcript-ownership.md as one bounded documentation decision pass. Read AGENTS.md, the durable state, and the TUI-overhaul design/reference files first; do not implement product code, publish TUI-010, commit, or continue into TUI-003."
```

Do not create a separate TUI-002 prompt before that pass.

## Verification

- `gofmt -w internal/tui/model.go internal/tui/model_test.go
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
  internal/cli/root_test.go` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
- `go run ./cmd/revolvr tui --help` — PASS.
- `git diff --check` — PASS.
- `git diff --name-only` — PASS; the approved snapshot contains only the
  reviewed product, test, planning, reference, and durable-state files.
- Real 80x24 PTY render — PASS; compact status and activity fit above the
  composer/footer without wrapped activity rows or the prior event wall.
- TUI-000 decision checks — PASS; D1, ADR-025, and durable decisions agree that
  Codex references are behavioral evidence only and implementation remains
  native Go/Bubble Tea with no source copying or porting.
- Codex behavioral reference gate — PASS; pinned identity, path-scoped diff,
  trailing-whitespace, marker, and pin checks passed.
- Reference link/range audit — PASS; every relative Markdown target exists and
  every cited Codex/Revolvr line range is within its named local source file.
- Scope audit for the behavioral study — PASS; exactly four reference files
  were created, its prompt was deleted, and no product source, ADR, dependency,
  D2-D6 decision, or task selector was changed by that study pass.
- `git show --check --oneline 24be655` — PASS.
- `git push origin main` — PASS; `f6f1db5..24be655  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-001 `git diff --check`, required D2/D5 `rg`,
  `git diff --name-only 24be655`, and `git status --short` — PASS.
- TUI-001 relative Markdown link, 33-task count, changed-path scope, accepted-
  decision consistency, and rejected-behavior task audits — PASS.
- `test ! -e docs/architecture/tui-overhaul/TUI_001_DECISION_PROMPT.md` — PASS.

Run the exact next command above for the next bounded product-decision pass.
