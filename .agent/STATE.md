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
  rejects active steering and queued/deferred operator messages. D3, D4, and
  D6 remain open; no implementation task is authorized yet.

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
  operator-message contract exists. D3, D4, and D6 remain open.

## Latest Documentation Pass

- Task selected: TUI-001, the bounded D2/D5 composer-semantics decision.
- Files changed: accepted D2/D5 in the design and TUI-001 task, narrowed
  TUI-031 to reviewed idle task entry, removed TUI-041, reconciled the epic and
  reference documents, updated durable state, and consumed the decision prompt.
- Result: initialized idle plain text creates only an ephemeral prefilled task
  review; all active and unavailable states reject it without an app effect;
  needs-input remains exact option-only input. No active steering, queued
  operator-message contract, or app/domain prerequisite was accepted.
- Verification: every required prompt command passed; all relative Markdown
  links resolve; 33 task files remain; changed paths are limited to the TUI
  overhaul and four durable-state files; the consumed prompt is absent.
- Evidence gaps: D3 still needs the bounded shell/settlement/reflow decision and
  proof; D4 needs overlay-order parity; D6 needs a session-header lifecycle.
- What remains: execute TUI-002, then continue E0 one decision task at a time
  and accept TUI-005 snapshots before promoting TUI-010.
- Blockers: none for TUI-002.

## Next Decision Pass

- Commit `24be655` (`feat: prepare transcript-first TUI overhaul`) remains the
  committed baseline; this TUI-001 pass is intentionally uncommitted.
- TUI-002 in
  `docs/architecture/tui-overhaul/tasks/tui-002-choose-transcript-ownership.md`
  is the exact next bounded decision pass. No separate TUI-002 prompt exists.

## Blockers And Next Task

- The approved task-publication, console, plan, and behavioral-study snapshot
  is committed and pushed at `24be655`.
- TUI-001 is complete and uncommitted. TUI-002 is next; complete E0 decisions
  and snapshots before publishing TUI-010. Do not revive EXT/PTC work or reopen
  Architecture 025.
