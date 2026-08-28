# Agent State

Updated: 2026-08-28

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
  session cell. TUI-010's composition proof, TUI-011's managed-resize proof,
  TUI-012's active-settlement proof, TUI-013's installed terminal shell,
  TUI-020's semantic cell vocabulary, TUI-021's bounded historical projection,
  TUI-022's live-to-committed reconciliation, and TUI-030's primary composer
  are complete. TUI-031 is the only pending canonical task, and later TUI tasks
  remain unpublished drafts.

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
- `go test ./internal/tui -run
  'Test(TranscriptShellProof|TranscriptShellResize|TranscriptShellSettlement|StatusModelInstallsTranscriptShell)'`
  — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellSettlement'` — PASS.
- `go test ./internal/tui -run 'TestTranscriptCell'` — PASS.
- `go test ./internal/tui -run 'TestHistoricalTranscript'` — PASS.
- `go test ./internal/tui -run
  'TestLiveTranscript(Reconciles|RejectsStale)'` — PASS.
- `go test ./internal/tui -run 'TestComposer|TestCommand'` — PASS.
- `go test ./internal/app` — PASS.
- `go test ./internal/tui` — PASS.
- Compiled settlement proof on a pseudo-terminal — PASS; modes restored and
  the returned prompt accepted `printf 'PROMPT_OK\n'`.
- `go run ./cmd/revolvr tui --help` — PASS.
- `.agent/STATE.md` line limit — PASS; fewer than 200 lines.
- `git diff --check` — PASS.
- TUI-030 immediate-focus, accepted footer, plain-text preservation, Escape,
  retained shortcut, slash-command, typed-question, active-settlement, and
  single-publication gates — PASS.
- `.agent/DECISIONS.md`, application/domain callbacks and authority,
  dependencies, and `.revolvr/` runtime state are unchanged in this pass.

## Blockers And Next Task

- Blockers: none.
- TUI-030 is complete. TUI-031 is the only pending canonical task and is
  dependency-satisfied by completed TUI-030 and accepted D2/D5.
- Do not revive deferred EXT/PTC work, republish TUI-013, bulk-publish later
  TUI tasks, or reopen Architecture 025.

## Latest Pass

- Task selected: `tui-030-make-composer-primary`.
- Files changed: TUI composer focus/routing and tests, TUI-030 completion and
  TUI-031 publication metadata, and affected planning, state, task-list,
  reference, and handoff records.
- Verification commands run: required `gofmt`, focused composer/command, TUI,
  CLI, and full Go tests, TUI help, task list/status selectors, state line
  limit, publication/status audits, and `git diff --check`.
- Verification result: PASS.
- What remains: TUI-031 is pending for the next fresh pass; later tasks remain
  unpublished drafts.
- Blockers: none.
