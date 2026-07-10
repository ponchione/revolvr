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

- [x] Reconcile ambiguous Git commit outcomes by comparing pre/post-commit HEAD.
  Scope: capture HEAD before staging, resolve it after every commit attempt, retry a failed post-commit lookup once, and classify a changed HEAD as committed even when the commit command or first SHA lookup reports failure. Preserve an explicit indeterminate state when post-commit HEAD cannot be resolved, without restoring stale task phase metadata.
  Acceptance: successful and initial commits still record their SHA; transient post-commit lookup failure recovers the created commit; a commit-command error with an advanced HEAD is recorded as committed; unchanged HEAD remains failed; unavailable post-commit HEAD is reported as indeterminate and leaves the task blocked at the transitioned phase for inspection.
  Verification: add focused `internal/commit` and `internal/runonce` regression tests; run `go test ./internal/commit ./internal/runonce`, `go test -race ./...`, `go vet ./...`, relevant CLI smoke commands, and `git diff --check`.

- [x] Add task workflow/phase metadata parsing and defaults.
  Scope: extend `internal/taskfile` frontmatter parsing for durable workflow metadata without changing runtime behavior yet. Support optional `workflow` and `phase` keys, default missing workflow to `mixed-pass-v1` and missing phase to `implement`, and accept the initial phases `implement`, `audit`, `document`, and `simplify`.
  Acceptance: existing `.agent/tasks/*.md` files without workflow metadata still load as pending implement-phase tasks; invalid workflow or phase values fail clearly; known frontmatter parsing remains deterministic; task selection order is unchanged.
  Verification: add focused `internal/taskfile` tests for defaults, valid explicit metadata, invalid metadata, duplicate keys, and unchanged runnable ordering; run `go test ./internal/taskfile` and `go test ./...`.

- [x] Add a pass-policy model mapping phases to profiles and outcome semantics.
  Scope: introduce a small runtime policy model for `mixed-pass-v1` that maps each phase to a profile, whether no-change success is allowed, and the next durable phase. Keep the model independent from `runonce` orchestration in this slice.
  Acceptance: `implement` maps to `implementer` and requires meaningful changes; `audit` maps to `auditor` and permits no-change success; `document` maps to `documentor` and permits no-change success; `simplify` maps to `simplifier` and permits no-change success; invalid workflow/phase combinations fail with actionable errors.
  Verification: add focused policy tests for every phase, phase order, terminal completion, no-change permissions, and invalid inputs; run the policy package tests and `go test ./...`.

- [x] Teach `runonce` to load the profile for the selected task phase.
  Scope: after selecting the canonical task file, resolve its workflow phase through the pass-policy model and load the mapped repo-authored profile instead of always loading `implementer`. Include workflow/phase/profile identity in task-selection context where it is useful for later audit.
  Acceptance: implement-phase tasks still load `.agent/profiles/implementer.md`; audit and document phases load their existing profiles; missing mapped profiles block before Codex with a clear message; context manifests continue recording the exact profile file used.
  Verification: add `internal/runonce` coverage for implement/audit/document profile selection, missing mapped profile failure, and context manifest source metadata; run `go test ./internal/runonce ./internal/prompt` and `go test ./...`.

- [x] Allow policy-permitted no-change success and durable phase advancement.
  Scope: update `runonce` finalization so successful phases advance the selected task file before the commit gate when policy allows it. Implementation no-change remains blocked; audit/document/simplify may succeed without code changes, advance the task phase or complete the task, and commit the task-file metadata transition.
  Acceptance: implement with no changes still produces `no_changes`; implement with changes advances to audit instead of completing the durable task; audit/document/simplify pass with no code changes can advance cleanly; the final successful phase marks the task completed; ledger events and receipts make the phase outcome auditable.
  Verification: add `internal/runonce` tests for implement no-change refusal, implement-to-audit advancement, no-change audit/document/simplify advancement, final completion, and changed-file capture after task metadata updates; run `go test ./internal/runonce ./internal/app` and `go test ./...`.

- [x] Seed `.agent/profiles/simplifier.md`.
  Scope: add a repo-authored simplifier profile and init template. The profile should direct the agent to reduce unnecessary complexity, duplication, and line count only when meaningful, create helpers only when they reduce real duplication or complexity, and stop cleanly when no simplification is worthwhile.
  Acceptance: `revolvr init` seeds `simplifier.md` without overwriting an existing file; the checked-in profile is concise and aligned with auditor/documentor style; tests cover template content and non-overwrite behavior.
  Verification: update focused prompt/CLI init tests; run `go test ./internal/prompt ./internal/cli` and `go test ./...`.

- [x] Surface task workflow state in CLI, TUI, status, and timeline views.
  Scope: expose task workflow/phase state through app status/task adapters, CLI task/status output, TUI Dashboard/Tasks/Run Detail surfaces, and timeline rows for phase selection or advancement events. Keep raw task files canonical.
  Acceptance: operators can see the current phase, next phase or completion state, and selected profile without opening the task file; empty and legacy task states render coherently; raw ledger events remain available for audit.
  Verification: add focused `internal/app`, `internal/cli`, and `internal/tui` tests for task list/status rendering, TUI task markers, run detail/timeline phase rows, and legacy/default metadata; run `go test ./internal/app ./internal/cli ./internal/tui` and `go test ./...`.

- [x] Document the mixed-pass task workflow.
  Scope: update operator-facing docs to explain durable tasks versus passes, task-file workflow metadata, phase order, profile responsibilities, no-change success for audit/document/simplify, and how Revolvr advances phases.
  Acceptance: README or an appropriate operator doc shows a minimal mixed-pass task file, explains `implement -> audit -> document -> simplify -> completed`, names the four profiles, and clarifies that Revolvr owns phase transitions based on receipts/outcomes.
  Verification: run `go test ./...`, relevant CLI help commands if docs mention them, and `git diff --check`.

- [x] Add a Markdown spec-to-task parser that preserves human-readable acceptance and verification notes.
  Scope: create a small internal parser for a documented Markdown task format without adding dependencies. Support repeated task sections with a required task body and optional summary, acceptance, and verification notes; preserve unknown section text in the generated task body.
  Acceptance: parser returns ordered task specs suitable for `internal/app.AddTask`; empty task text and malformed sections produce clear errors with line context; multiline task text remains readable in Codex prompts.
  Verification: add focused parser tests; run `go test ./internal/taskimport` and `go test ./...`.

- [x] Add an app-level task import and dry-run operation.
  Scope: expose parsed task imports through `internal/app`, with dry-run and write modes. Validate all parsed tasks before writing; write mode creates tasks in input order and returns created IDs.
  Acceptance: dry-run reports the tasks that would be created without mutating `.revolvr/`; write mode persists every valid task in order; parse and validation errors do not partially write tasks.
  Verification: add `internal/app` tests for dry-run, ordered import, validation failure, parse failure, and empty import; run `go test ./internal/app` and `go test ./...`.

- [x] Add `revolvr task import <path>` with `--dry-run`.
  Scope: wire the CLI to the app import operation. Print numbered dry-run rows and created task IDs, while keeping existing `task add` and `task list` output unchanged.
  Acceptance: `--dry-run` does not mutate task state; import creates tasks in parsed order; unreadable files and parse failures return clear command errors.
  Verification: add focused CLI tests for help, dry-run, successful import, parse errors, and unreadable paths; run `go test ./internal/cli -run 'TestTaskImport|TestTask(Add|List)'`, `go test ./...`, and `go run ./cmd/revolvr task import --help`.

- [x] Document the chat/spec-to-task workflow and import format.
  Scope: add README guidance for using web chat to design specs, saving a Markdown task file, dry-running/importing it, refreshing the TUI, and running one pass from the TUI. Include the caution that chat and TUI can share task state, but concurrent code edits against the same repo should be avoided.
  Acceptance: docs include a minimal import-file example with summary, acceptance, and verification notes, plus commands for dry-run, import, TUI refresh, preflight, and run-once.
  Verification: run `go test ./...` and `go run ./cmd/revolvr task import --help`.

- [x] Surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
  Scope: highlight the first pending task, show pending/blocked/completed counts near the task area, and distinguish `ready to run` from `nothing runnable` without relying on color.
  Acceptance: Dashboard shows the next task ID and summary when present; Tasks view marks both the current selection and the next runnable task; uninitialized and empty states still render coherently.
  Verification: add focused `internal/tui` render tests for pending, blocked-only, completed-only, and empty task lists; run `go test ./internal/tui` and `go test ./...`.

- [x] Add TUI blocked-task retry for the selected task.
  Scope: add a Tasks-view action backed by `internal/app.RetryTask`, refresh after success, and display clear inline messages for non-blocked tasks, missing callbacks, and retry errors.
  Acceptance: a blocked selected task can be returned to pending without leaving the TUI; pending and completed selected tasks are not mutated; the footer/help reflects when retry is available.
  Verification: add TUI model tests for successful retry, non-blocked rejection, callback error, and refresh failure; add CLI wiring coverage if command setup changes; run `go test ./internal/tui ./internal/cli ./internal/app` and `go test ./...`.

- [x] Add an app-level run timeline projection from ledger events.
  Scope: build a reusable `internal/app` projection that converts a run history into ordered human-readable timeline rows for prompt creation, Codex start/progress/completion, verification, commit, receipt, and terminal outcome.
  Acceptance: timeline rows include timestamp, phase, status, and concise detail; completed, failed-verification, Codex-failed, blocked, and missing-artifact histories degrade gracefully without panics.
  Verification: add `internal/app` tests for completed, failed verification, Codex failed, blocked, and missing-event histories; run `go test ./internal/app` and `go test ./...`.

- [x] Render the run timeline in CLI `show` and TUI Run Detail.
  Scope: surface the app timeline in `revolvr show <run-id>` and the TUI Run Detail view, while keeping raw event visibility available in the TUI when practical.
  Acceptance: users can understand the run flow without reading raw JSON or ledger payloads; long timelines remain scrollable; CLI output remains deterministic in tests.
  Verification: add focused CLI and TUI tests for timeline rendering and long timeline scrolling; run `go test ./internal/cli ./internal/tui` and `go test ./...`.

- [x] Add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
  Scope: add a bounded multi-pass action in the TUI that runs up to a small user-selected pass count, reuses preflight readiness and cancellation controls, and streams pass summaries into the progress pane.
  Acceptance: users can run multiple passes without leaving the TUI; the TUI honors the same stop reasons and guardrails as `run --max-passes`; cancellation reports a clear terminal state and refreshes state.
  Verification: add fake-runner TUI tests for max-pass completion, no-task stop, failure guardrail, blocked stop, and cancellation; run `go test ./internal/tui ./internal/app ./internal/cli` and `go test ./...`.

## Completed History

The previous harness, app-boundary, and TUI operator-console tasks were completed on 2026-07-08. Details are preserved in `.agent/STATE.md` and the Git history.

## Blocked

None.
