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
- The TUI overhaul is in documentation review only. D1 is accepted: implement
  the accepted Codex interaction behavior in the existing Go/Bubble Tea
  program, treating the pinned Codex source and snapshots as behavioral
  evidence only. ADR-025's no-clone/no-port boundary remains unchanged, and no
  implementation task is authorized yet.

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
  gated epics. `docs/architecture/tui-overhaul/tasks/` contains 34 task files,
  each with one decision, proof, component change, focused-view migration, or
  verification responsibility plus explicit dependencies, acceptance,
  verification, and exclusions.
- The task audit separated previously combined decision gates, resize/reflow
  from active settlement, independently reachable focused views, real-terminal
  scrollback from lifecycle restoration, and operator docs from final closeout.
- No draft item was copied into `.agent/tasks/`. Promote only one bounded task
  after the review gate is complete; TUI-010 is the first implementation
  candidate.
- D1 is accepted: Codex source and snapshots are behavioral evidence only, and
  Revolvr will reimplement the accepted interaction behavior natively in
  Go/Bubble Tea. Open decisions remain idle/active plain-text composer meaning,
  transcript/scrollback ownership, overlay migration, queued input, and the
  one-time session header.

## Latest Documentation Pass

- Task selected: execute the bounded Codex TUI behavioral-reference study
  before TUI-001.
- Files changed: added exactly four evidence documents under
  `docs/architecture/tui-overhaul/reference/` (`README.md`,
  `interaction-model.md`, `terminal-mechanics.md`, and `revolvr-mapping.md`),
  removed the consumed `CODEX_BEHAVIOR_STUDY_PROMPT.md`, and updated durable
  state and handoff.
- Verification: the pinned checkout still resolves to
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`; required path-scoped diff,
  whitespace, marker, and pin checks pass; every relative Markdown target and
  cited Codex/Revolvr line range was checked against the local files.
- Result: all six requested study areas are source-cited, all TUI-010 through
  TUI-072 proof/implementation tasks have a reference or explicit no-analog
  entry, and observations, current behavior, candidates, and open decisions
  are separated. D2-D6 remain open.
- Evidence gaps: Revolvr still needs D2/D5 application ownership for plain and
  queued input, a D3 Go/Bubble Tea shell/settlement/reflow proof, real-terminal
  scrollback and lifecycle records, a styles-disabled target snapshot, a D6
  session-header lifecycle, and per-view D4 parity evidence.
- What remains: resolve D2 and D5 in TUI-001, then continue the E0 decisions and
  accept TUI-005 snapshots before promoting TUI-010.
- Blockers: none for TUI-001.

## Blockers And Next Task

- The task-publication fix, transcript-first console, planning tree, behavioral
  study, tests, and durable-state updates form one operator-approved snapshot
  for raw-Git commit and push.
- Branch `main` was one commit ahead of `origin/main` before finalizing this
  snapshot.
- TUI-001 remains the next product-decision task. Complete E0 decisions and
  snapshots before publishing TUI-010. Do not revive EXT/PTC work or reopen
  Architecture 025.
