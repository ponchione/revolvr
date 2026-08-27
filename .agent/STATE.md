# Agent State

Updated: 2026-08-27

## Architecture Status

- Architecture 001-025 are complete, including the approved Architecture 016a
  compatibility task. Their canonical decisions, implementation, and evidence
  remain in the repository and Git history.
- No architecture task is pending. The historical EXT backlog remains deferred,
  and PTC-101 through PTC-108B remain terminally superseded and non-selectable.

## Current Authority

- ADR-025 is current authority. Revolvr is terminal-first and extends the
  existing Bubble Tea TUI through existing Go application services.
- Direct tools remain the default harness. No desktop/Wails/Vue/REST/SSE or
  custom Python workspace, execution, skill, scratch, or refinement roadmap is
  authorized.
- Canonical truth remains in the existing Go/PostgreSQL ledger, artifacts,
  verification, audit, and completion model.
- The TUI overhaul is in documentation review only. D1 accepts a native
  Go/Bubble Tea behavioral reimplementation. D2 accepts reviewed idle task
  drafts and rejects plain text elsewhere, including typed needs-input. D5
  rejects active steering and queued/deferred operator messages. D3 accepts
  source-backed semantic cells with one-time `tea.Println` commitment to
  terminal-owned history and a managed live/composer/overlay frame. D4 and D6
  remain open; no implementation task is authorized yet.

## Architecture 024 TUI

- Architecture 024 completed at `phase: simplify` on 2026-08-27.
- The TUI centers the canonical run-event transcript and provides a command
  composer, typed operator responses, compact status and command discovery,
  and focused change-summary, evidence, and approval views.
- It reuses existing application services and terminal dependencies; business,
  lifecycle, scheduling, verification, approval, and completion authority
  remain outside the TUI.

## Architecture 025 Graphiti Gate

- Architecture 025 completed at `phase: simplify` with decision **defer**.
- Existing retrieval/context baselines are supported, but qualifying current-
  brain usage, repeated source-linked failures, and smaller-fix insufficiency
  evidence are absent. No Graphiti comparison prototype or implementation is
  authorized.
- The evidence and exact re-evaluation trigger are in
  [the Architecture 025 phase-gate record](../docs/architecture/memory-graphiti-phase-gate.md).

## Current Verification

- `gofmt -w internal/tui/model.go internal/tui/model_test.go
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
  internal/cli/root_test.go` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
- `go run ./cmd/revolvr tui --help` — PASS.
- `git diff --check` — PASS.
- `git diff --name-only` — PASS.
- TUI-001 path-scoped `git diff --check` and required D2/D5 `rg` audit — PASS.
- TUI-001 relative Markdown link, changed-path scope, task-count, and rejected-
  behavior task audits — PASS.

## Current TUI Work

- The official `openai/codex` repository is available as an ignored, shallow
  local reference at `.reference/codex`; its TUI source is under
  `.reference/codex/codex-rs/tui` at commit
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`.
- TUI task creation now requires a clean worktree and commits only the newly
  created canonical task file. It no longer makes its own following preflight
  fail. CLI task add/import remains an explicit review-and-commit flow.
- The dashboard is now a transcript-first operator console. Its single-line
  header names Revolvr, the current view, and initialization state; initialized
  content is the latest run's operator status and semantic `app.RunTimeline`,
  while an empty history shows only idle state and the next runnable task.
- Dashboard task counts, latest/recent run blocks, and raw ledger events were
  removed. They remain available in the existing focused views. The dashboard
  always shows `› / for commands`, uses a concise contextual footer, and keeps
  cancellation visible during an active run.
- Dashboard activity shows only the latest eight meaningful semantic
  transitions, omits low-level Codex lifecycle noise and the duplicated full
  task body, and points operators to Run Detail for the complete timeline.
- Redundant Transcript/Activity labels and dashboard UUID rows are gone;
  dashboard-only detail compaction removes long file lists, full hashes,
  receipt paths, and machine warning prefixes at 80 columns.
- Shared styles now use terminal-default primary text, bold headings/attention,
  dim secondary text, and ANSI cyan/green/red accents. This is presentation
  only; application services and all existing input behavior remain unchanged.

## TUI Overhaul Planning

- `docs/architecture/tui-overhaul/README.md` is the draft product/design and
  plan index. It records the current renderer, proposed committed transcript,
  live cell, always-focused composer, overlays, app-service seam, invariants,
  decisions, and whole-overhaul acceptance.
- `docs/architecture/tui-overhaul/epics/` contains one file for each of eight
  gated epics. `docs/architecture/tui-overhaul/tasks/` contains 33 task files,
  each with one decision, proof, component change, focused-view migration, or
  verification responsibility plus explicit dependencies, acceptance,
  verification, and exclusions.
- The task audit separated previously combined decision gates, resize/reflow
  from active settlement, independently reachable focused views, real-terminal
  scrollback from lifecycle restoration, and operator docs from final closeout.
- No draft item was copied into `.agent/tasks/`. Promote only one bounded task
  after the review gate is complete; TUI-010 is the first implementation
  candidate.
- D1, D2, and D5 are accepted. Initialized idle plain text opens the existing
  reviewed Add Task draft; uninitialized and active text rejects without an app
  effect; typed needs-input remains option-only; and no steering or queued
  operator-message contract exists. D3 is accepted as a bounded hybrid:
  `internal/tui` owns semantic source/live state, terminal history owns emitted
  rows and native reflow/copy, and viewports are overlay-only. D4 and D6 remain
  open.

## Latest Documentation Pass

- Task selected: TUI-002, the bounded D3 transcript-ownership decision.
- Files changed: accepted D3 in the design and TUI-002 task, reconciled E1 and
  affected proof/implementation tasks, updated all four reference documents,
  updated durable state, and consumed the decision prompt.
- Result: `internal/tui` retains bounded semantic source cells and the managed
  live/composer/overlay frame; finalized identities append once through the
  installed Bubble Tea renderer; terminal history owns emitted rows,
  scroll/copy, and native reflow. Viewport-owned history and application-driven
  terminal-history reflow are rejected. No app/domain prerequisite, dependency,
  terminal backend, or escape layer was accepted.
- Verification: the required path-scoped diff, D3 term audit, baseline path
  list, status, prompt deletion, relative-link, changed-scope, owner-table, and
  rejected-task-path checks pass.
- Evidence gaps: TUI-010/TUI-011/TUI-012 must prove append composition, test
  output, managed-frame reflow without replay, and exact-once settlement;
  TUI-061/TUI-062 must record terminal/tmux scrollback and lifecycle behavior.
  D4 still needs overlay-order parity, and D6 needs a session-header lifecycle.
- What remains: execute TUI-003, then continue E0 one decision task at a time
  and accept TUI-005 snapshots before promoting TUI-010.
- Blockers: none for TUI-003.

## Next Decision Pass

- Commit `f4e7ebf` (`docs: resolve TUI composer semantics`) was pushed to
  `origin/main`; the branch was clean and synchronized immediately after the
  push.
- TUI-002 is complete and intentionally uncommitted. TUI-003 in
  `docs/architecture/tui-overhaul/tasks/tui-003-accept-overlay-migration.md` is
  the exact next bounded decision pass. No TUI-003 prompt exists.

## Blockers And Next Task

- The approved task-publication, console, plan, and behavioral-study snapshot
  and the accepted TUI-001 decision are committed and pushed through `f4e7ebf`.
- TUI-002 is complete and uncommitted. Complete E0 decisions and snapshots
  before publishing TUI-010. Do not revive EXT/PTC work or reopen Architecture
  025.
