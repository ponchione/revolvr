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
  `tasks/` has 34 audited, bounded task files.
- Split composite work into separate product decisions, shell resize and
  settlement proofs, independently reachable focused-view migrations,
  scrollback and terminal-lifecycle checks, and documentation and final
  closeout tasks. Every task states dependencies, scope, acceptance,
  verification, and exclusions.
- The draft does not publish runnable `.agent/tasks`. TUI-000 and D1 are
  accepted: Revolvr will reimplement the accepted Codex interaction behavior
  in its existing Go/Bubble Tea TUI; the pinned Codex source and snapshots are
  behavioral evidence only, and no implementation source may be copied or
  ported. ADR-025 remains unchanged. Plain-text composer semantics and
  transcript/scrollback ownership remain open. No product code changed in this
  decision pass.
- Completed the pinned Codex TUI behavioral study and added exactly four
  evidence documents under `docs/architecture/tui-overhaul/reference/`:
  the reference index/task lookup, interaction model, terminal mechanics, and
  current Revolvr mapping. The consumed study prompt was removed.
- The references trace shell ownership, transcript settlement/replay, composer
  and interrupt behavior, overlays/questions/approvals, terminal
  scrollback/reflow/restoration, styles, tests, snapshots, and current Go seams.
  Every TUI-010 through TUI-072 proof/implementation task has a relevant link
  or an explicit no-useful-analog entry.
- D2-D6 remain open. The leading gaps are application ownership for plain and
  queued input, a Go/Bubble Tea D3 shell proof, real-terminal scrollback and
  lifecycle evidence, styles-disabled proof, the D6 header lifecycle, and D4
  per-view parity.
- The task-publication fix, console presentation/test update, planning tree,
  behavioral study, and durable-state changes form one operator-approved
  snapshot for raw-Git commit and push. Branch `main` was already one commit
  ahead of `origin/main` before finalizing it.

## Exact Next Command

Run one fresh Codex pass for TUI-001 using:

```bash
codex exec "Execute TUI-001 from docs/architecture/tui-overhaul/tasks/tui-001-resolve-composer-semantics.md as one bounded documentation decision pass. Read AGENTS.md and the durable state first; do not implement product code or continue into another task."
```

TUI-001 remains the next product-decision task. Use the design index and the
new evidence references as inputs, then follow E0 one decision task at a time
until D1-D6 and the experience snapshots are accepted. Do not promote TUI-010
or change product code before that review gate.

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
- Scope audit — PASS; exactly four reference files were created, the consumed
  prompt was deleted, and no product source, ADR, dependency, D2-D6 decision,
  or task selector was changed by the study pass.

Run the exact next command above for the next bounded product-decision pass.
