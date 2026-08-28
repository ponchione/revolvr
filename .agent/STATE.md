# Agent State

Updated: 2026-08-27

## Architecture Status

- Architecture 001-025 are complete, including the approved Architecture 016a
  compatibility task. No architecture task is pending.
- The legacy EXT backlog remains deferred. PTC-101 through PTC-108B are
  terminally superseded and non-selectable.

## Current Authority

- ADR-025 is current authority. Revolvr is terminal-first and extends the
  existing Go/Bubble Tea TUI through existing application services.
- Direct tools remain the default harness. No desktop/Wails/Vue/REST/SSE or
  custom Python workspace, execution, skill, scratch, or refinement roadmap is
  authorized.
- Canonical truth remains in the Go/PostgreSQL ledger, artifacts, verification,
  audit, and completion model.
- The accepted TUI-overhaul decisions retain a native Go/Bubble Tea
  reimplementation, terminal-owned committed history, managed live/composer/
  overlay state, reviewed idle task drafts, no active steering or deferred
  operator messages, parity-gated focused-view migration, and one startup-only
  session cell. TUI-010's composition proof is complete; later TUI tasks remain
  unpublished drafts.

## Architecture 024 TUI

- Architecture 024 completed at `phase: simplify` on 2026-08-27.
- The TUI centers canonical run events and provides a command composer, typed
  operator responses, compact status and command discovery, and focused change
  summary, evidence, and approval views over existing application services.
- Business, lifecycle, scheduling, verification, approval, and completion
  authority remain outside the TUI.

## Architecture 025 Graphiti Gate

- Architecture 025 completed at `phase: simplify` with decision **defer**.
- Retrieval/context baselines exist, but qualifying current-brain usage,
  repeated source-linked failures, and smaller-fix insufficiency evidence do
  not. No Graphiti comparison prototype or implementation is authorized.
- The exact re-evaluation trigger is in
  [the Architecture 025 phase-gate record](../docs/architecture/memory-graphiti-phase-gate.md).

## Current Verification

- `go test ./...` — PASS.
- `go run ./cmd/revolvr task list` — PASS; all canonical tasks are terminal.
- `go run ./cmd/revolvr status` — PASS; no pending canonical task is selected.
- `.agent/STATE.md` line limit — PASS; fewer than 200 lines.
- `git diff --check` — PASS.
- `git diff --name-only 1d959ad` — PASS; only `.agent/STATE.md`, the canonical
  compaction task, `.agent/TASKS.md`, and `.agent/HANDOFF.md` differ.
- `.agent/DECISIONS.md`, canonical architecture/evidence, product code,
  dependencies, and `.revolvr/` runtime state are unchanged from `1d959ad`.

## Blockers And Next Task

- Blockers: none.
- The current-state compaction task is complete. No canonical task is pending.
- Do not revive deferred EXT/PTC work, publish a later TUI task, or reopen
  Architecture 025 without a separately authorized task.
