# Agent State

Updated: 2026-09-02

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
  TUI-022's live-to-committed reconciliation, TUI-030's primary composer, and
  TUI-031's reviewed idle plain-text route are complete. TUI-032 is the only
  pending canonical task, and later TUI tasks remain unpublished drafts.

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

- `go test ./... -count=1` — PASS; 4,139 tests in 91 packages, including the
  socket-based packages that the external agent sandbox could not execute.
- `go test ./internal/evaluation -run
  '^TestFixtureIdentityUsesGitPortableFileModes$' -count=1` — PASS.
- `go test ./internal/evaluation -count=2` — PASS; 28 test executions.
- Architecture 022 golden audit — PASS; after recursively removing SHA-256
  fields, the prior and regenerated baselines are byte-equivalent.
- `go test ./internal/tui -run
  'Test(TranscriptShellProof|TranscriptShellResize|TranscriptShellSettlement|StatusModelInstallsTranscriptShell)'`
  — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellSettlement'` — PASS.
- `go test ./internal/tui -run 'TestTranscriptCell'` — PASS.
- `go test ./internal/tui -run 'TestHistoricalTranscript'` — PASS.
- `go test ./internal/tui -run
  'TestLiveTranscript(Reconciles|RejectsStale)'` — PASS.
- `go test ./internal/tui -run 'TestComposer|TestCommand'` — PASS.
- `go test ./internal/tui -run
  'TestPlainTextComposer|TestStatusModelTaskEntry' -count=1` — PASS; 13 tests.
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
- TUI-031 reviewed-draft, single-publication, cancellation, rejected-state,
  post-settlement, typed-needs-input, and unchanged-authority gates — PASS.
- `go run ./cmd/revolvr init` and `go run ./cmd/revolvr status` — PASS; local
  ignored runtime is restored and TUI-032 is the sole next task.
- Application/domain callbacks, dependencies, and TUI authority are unchanged.

## Blockers And Next Task

- Blockers: none. The prior socket-permission failure was specific to the
  external agent sandbox; the required full suite passes in this checkout.
- TUI-031 is complete. TUI-032 is the only pending canonical task and is
  dependency-satisfied by completed TUI-030.
- Do not revive deferred EXT/PTC work, republish TUI-013, bulk-publish later
  TUI tasks, or reopen Architecture 025.

## Latest Pass

- Task selected: `tui-031-implement-plain-text-input`.
- Files changed: the TUI plain-text routing path and regression tests, TUI-031
  completion and TUI-032 publication metadata, and affected planning, state,
  task-list, reference, and handoff records.
- Verification commands run: required `gofmt`, focused plain-text/task-entry,
  TUI, and full Go tests, TUI help, task list/status selectors, state line
  limit, publication/status audits, and `git diff --check`.
- Verification result: PASS.
- What remains: TUI-032 is pending for the next fresh pass; later tasks remain
  unpublished drafts.
- Blockers: none.
