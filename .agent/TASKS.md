# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, write the blocker under `Blocked` and stop.
- Add new tasks only when they are directly discovered while working on the current task.
- New tasks must be specific, small, and verifiable.
- Do not invent broad roadmap items.

## Current Backlog

- [x] Add a Markdown spec-to-task parser that preserves human-readable acceptance and verification notes.
  Scope: create a small internal parser for a documented Markdown task format without adding dependencies. Support repeated task sections with a required task body and optional summary, acceptance, and verification notes; preserve unknown section text in the generated task body.
  Acceptance: parser returns ordered task specs suitable for `internal/app.AddTask`; empty task text and malformed sections produce clear errors with line context; multiline task text remains readable in Codex prompts.
  Verification: add focused parser tests; run `go test ./internal/taskimport` and `go test ./...`.

- [x] Add an app-level task import and dry-run operation.
  Scope: expose parsed task imports through `internal/app`, with dry-run and write modes. Validate all parsed tasks before writing; write mode creates tasks in input order and returns created IDs.
  Acceptance: dry-run reports the tasks that would be created without mutating `.revolvr/`; write mode persists every valid task in order; parse and validation errors do not partially write tasks.
  Verification: add `internal/app` tests for dry-run, ordered import, validation failure, parse failure, and empty import; run `go test ./internal/app` and `go test ./...`.

- [ ] Add `revolvr task import <path>` with `--dry-run`.
  Scope: wire the CLI to the app import operation. Print numbered dry-run rows and created task IDs, while keeping existing `task add` and `task list` output unchanged.
  Acceptance: `--dry-run` does not mutate task state; import creates tasks in parsed order; unreadable files and parse failures return clear command errors.
  Verification: add focused CLI tests for help, dry-run, successful import, parse errors, and unreadable paths; run `go test ./internal/cli -run 'TestTaskImport|TestTask(Add|List)'`, `go test ./...`, and `go run ./cmd/revolvr task import --help`.

- [ ] Document the chat/spec-to-task workflow and import format.
  Scope: add README guidance for using web chat to design specs, saving a Markdown task file, dry-running/importing it, refreshing the TUI, and running one pass from the TUI. Include the caution that chat and TUI can share task state, but concurrent code edits against the same repo should be avoided.
  Acceptance: docs include a minimal import-file example with summary, acceptance, and verification notes, plus commands for dry-run, import, TUI refresh, preflight, and run-once.
  Verification: run `go test ./...` and `go run ./cmd/revolvr task import --help`.

- [ ] Surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
  Scope: highlight the first pending task, show pending/blocked/completed counts near the task area, and distinguish `ready to run` from `nothing runnable` without relying on color.
  Acceptance: Dashboard shows the next task ID and summary when present; Tasks view marks both the current selection and the next runnable task; uninitialized and empty states still render coherently.
  Verification: add focused `internal/tui` render tests for pending, blocked-only, completed-only, and empty queues; run `go test ./internal/tui` and `go test ./...`.

- [ ] Add TUI blocked-task retry for the selected task.
  Scope: add a Tasks-view action backed by `internal/app.RetryTask`, refresh after success, and display clear inline messages for non-blocked tasks, missing callbacks, and retry errors.
  Acceptance: a blocked selected task can be returned to pending without leaving the TUI; pending and completed selected tasks are not mutated; the footer/help reflects when retry is available.
  Verification: add TUI model tests for successful retry, non-blocked rejection, callback error, and refresh failure; add CLI wiring coverage if command setup changes; run `go test ./internal/tui ./internal/cli ./internal/app` and `go test ./...`.

- [ ] Add an app-level run timeline projection from ledger events.
  Scope: build a reusable `internal/app` projection that converts a run history into ordered human-readable timeline rows for prompt creation, Codex start/progress/completion, verification, commit, receipt, and terminal outcome.
  Acceptance: timeline rows include timestamp, phase, status, and concise detail; completed, failed-verification, Codex-failed, blocked, and missing-artifact histories degrade gracefully without panics.
  Verification: add `internal/app` tests for completed, failed verification, Codex failed, blocked, and missing-event histories; run `go test ./internal/app` and `go test ./...`.

- [ ] Render the run timeline in CLI `show` and TUI Run Detail.
  Scope: surface the app timeline in `revolvr show <run-id>` and the TUI Run Detail view, while keeping raw event visibility available in the TUI when practical.
  Acceptance: users can understand the run flow without reading raw JSON or ledger payloads; long timelines remain scrollable; CLI output remains deterministic in tests.
  Verification: add focused CLI and TUI tests for timeline rendering and long timeline scrolling; run `go test ./internal/cli ./internal/tui` and `go test ./...`.

- [ ] Add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
  Scope: add a bounded multi-pass action in the TUI that runs up to a small user-selected pass count, reuses preflight readiness and cancellation controls, and streams pass summaries into the progress pane.
  Acceptance: users can run multiple passes without leaving the TUI; the TUI honors the same stop reasons and guardrails as `run --max-passes`; cancellation reports a clear terminal state and refreshes state.
  Verification: add fake-runner TUI tests for max-pass completion, no-task stop, failure guardrail, blocked stop, and cancellation; run `go test ./internal/tui ./internal/app ./internal/cli` and `go test ./...`.

## Completed History

The previous harness, app-boundary, and TUI operator-console tasks were completed on 2026-07-08. Details are preserved in `.agent/STATE.md` and the Git history.

## Blocked

None.
