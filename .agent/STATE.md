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
- The TUI overhaul has completed its product review and first implementation
  proof. D1 accepts a native
  Go/Bubble Tea behavioral reimplementation. D2 accepts reviewed idle task
  drafts and rejects plain text elsewhere, including typed needs-input. D5
  rejects active steering and queued/deferred operator messages. D3 accepts
  source-backed semantic cells with one-time `tea.Println` commitment to
  terminal-owned history and a managed live/composer/overlay frame. D4 accepts
  a one-view-at-a-time Help-to-typed-needs-input overlay migration with
  retained key/command entries, exact return ownership, and parity-gated page
  removal. D6 accepts one startup-only `session-start` cell per TUI process,
  sourced from the inspected absolute project root and process-start
  initialization; refresh, resize, and overlays never re-emit it, restart does,
  and no Revolvr clear action is added. TUI-005 accepts literal experience
  states at 80 columns and the 40-column minimum with exclusive row ownership
  and intentional below-minimum behavior. E0 has exited. TUI-010 is the sole
  published TUI implementation task and its composition proof is complete; all
  later TUI tasks remain unpublished drafts.

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
- Only TUI-010 was copied into `.agent/tasks/` after the review gate. All later
  draft tasks remain unpublished and non-runnable.
- D1, D2, and D5 are accepted. Initialized idle plain text opens the existing
  reviewed Add Task draft; uninitialized and active text rejects without an app
  effect; typed needs-input remains option-only; and no steering or queued
  operator-message contract exists. D3 is accepted as a bounded hybrid:
  `internal/tui` owns semantic source/live state, terminal history owns emitted
  rows and native reflow/copy, and viewports are overlay-only. D4 is accepted:
  Help, Tasks, Runs/Run Detail, Preflight, Workflow, Change Summary, Evidence,
  Approval, and typed needs-input migrate in that order with retained routes
  and page rollback until their parity/removal gates pass. D6 is accepted with
  one startup-only session cell, exclusive presentation ownership, typed local
  identity, and proof gates before persistent dashboard chrome is removed.
- TUI-005 and E0 are accepted. The design authority fixes all required literal
  state snapshots, exact safety/cancellation/outcome/next-action text, 80x24
  normal geometry, 40x24 minimum geometry, and below-minimum behavior. TUI-010
  is complete without installing the production shell.

## Latest Implementation Pass

- Task selected: `tui-010-prove-shell-composition` — prove transcript-shell
  composition.
- Files changed: `internal/tui/model_test.go`, the canonical TUI-010 task,
  affected TUI planning records, `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`.
- Verification: `gofmt -w internal/tui/model.go internal/tui/model_test.go`,
  `go test ./internal/tui -run 'TestTranscriptShellProof'`,
  `go test ./internal/tui`, `go test ./...`, an interactive PTY proof, and
  `go run ./cmd/revolvr task list`, `go run ./cmd/revolvr status`, the 169-line
  state-file check, and `git diff --check` pass.
- Result: the test-only proof appends accepted committed identities once,
  replaces only the managed live frame, preserves composer input, works with
  two output-buffer types, and restores the PTY after normal quit. The old
  dashboard composition is explicitly rejected.
- What remains: the production shell remains TUI-013 work after the unpublished
  TUI-011 and TUI-012 proofs. The next canonical selector is the existing
  bounded `.agent/STATE.md` compaction task.
- Blockers: none.

## Next Fresh Pass

- Commit `19d80f8` (`docs: accept TUI experience states`) was pushed to
  `origin/main` and is the accepted publication source.
- `.agent/tasks/01a043b3-ad2d-7979-8d33-b6875643af8d.md` is the next canonical
  selector. It is the pre-existing bounded current-state compaction task, not
  deferred EXT/PTC work.

## Blockers And Next Task

- The approved task-publication, console, plan, behavioral study, accepted
  TUI-001 through TUI-005 decisions, and E0 closure are committed and pushed
  through `19d80f8`.
- TUI-010 is complete. The exact next task is the bounded current-state
  compaction selected above. Do not select deferred EXT/PTC work, publish a
  later TUI task, or reopen Architecture 025 in that pass.
