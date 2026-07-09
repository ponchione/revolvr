# Agent State

## Current Focus

No task is currently in progress. The next unchecked backlog item is to add `revolvr task import <path>` with `--dry-run`.

## Dogfood Timestamp Verification

- 2026-07-08T13:04:17Z live run `019f41d3-9120-7a77-92fd-d799f76ba000`: verifies receipt timestamp finalization after the prior fix by writing the receipt with the prompt-provided stale timestamp.

## Last Run

Task completed on 2026-07-09:

- Selected task: add an app-level task import and dry-run operation.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added app-level Markdown task import parsing and import execution. `internal/app.ParseTaskImport` exposes normalized parsed tasks, `ImportTasksFromMarkdown` parses and imports in one call, and `ImportTasks` supports dry-run and write modes over parsed tasks. Dry-run returns the tasks that would be created without opening or mutating `.revolvr/`; write mode validates every task before opening the store, creates tasks in input order, and returns created IDs.
- Tests added: focused app tests for dry-run reporting without state mutation, ordered write/import ID returns, validation failure without partial writes, parse failure without partial writes, and empty parsed imports without state creation.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go`; `go test ./internal/app`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add `revolvr task import <path>` with `--dry-run`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a Markdown spec-to-task parser that preserves human-readable acceptance and verification notes.
- Files changed: `internal/taskimport/parser.go`, `internal/taskimport/parser_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/taskimport`, a dependency-free Markdown parser for repeated `## Task` sections. It returns ordered task specs with `Task` and `Summary`, preserves acceptance and verification notes in task text, keeps unknown subsections in task text, supports explicit task body subsections, and reports parse errors with line context.
- Tests added: focused parser tests for ordered repeated tasks, multiline task body readability, explicit task body sections, preserved acceptance/verification/unknown notes, empty task text, malformed pre-task sections, duplicate known sections, and empty input.
- Verification run: `gofmt -w internal/taskimport/parser.go internal/taskimport/parser_test.go`; `go test ./internal/taskimport`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add an app-level task import and dry-run operation.
- Blockers: none.

Planning update on 2026-07-09:

- Selected task: clean the completed TUI backlog out of the active task list and seed next-phase development tasks from the current design discussion.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: none; durable planning state only.
- Task list changed: `.agent/TASKS.md` now starts with a Markdown spec-to-task import workflow, then TUI next-task visibility, blocked-task retry, human-readable run timelines, and a bounded multi-pass TUI flow.
- Runtime queue changed: preserved the existing `.revolvr/` completed run history and added 9 new pending tasks in the same order as `.agent/TASKS.md`.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: first unchecked backlog item is to add a Markdown spec-to-task parser.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: polish TUI layout, styling, and documentation for daily use.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: TUI rendering now wraps plain content, header lines, and key help against the active terminal width before applying semantic Lip Gloss styles. Narrow layouts use a compact recent-run list, long values wrap instead of spilling, empty states distinguish unavailable runtime state from absent tasks/runs, and important states remain visible through words like `Status: failed`, `PASS`, `FAIL`, `OK`, and `! blocked`.
- Documentation added: README now includes a `revolvr tui` section covering views, key actions, live progress, receipt validation, preflight checks, and current limitations that still require the CLI.
- Tests added: snapshot-style wide and narrow TUI render coverage with max-width assertions for narrow output.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go run ./cmd/revolvr tui --help`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a nonblocking TUI run-once action with live progress and cancellation.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI now exposes an uppercase `R` run-once action guarded by the latest ready preflight result, tracks one active run at a time, streams Codex progress into a persistent Run Progress pane, hides or blocks conflicting actions while running, and supports `c` cancellation through the run context. Run completion refreshes the status snapshot, selects and loads the completed run detail when a run ID exists, and reports terminal states for success, failure, no-task, and cancellation.
- CLI wiring changed: `revolvr tui` now passes a `RunOnce` callback that invokes `internal/app.RunOnce` with the same fakeable runner hook used by `revolvr run --once`.
- Tests added: focused TUI model coverage for preflight and active-run guards, progress streaming, successful completion refresh, failed outcomes, and cancellation. CLI TUI wiring coverage now verifies the run callback and progress callback are available.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to polish TUI layout, styling, and documentation for daily use.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move doctor/preflight orchestration behind `internal/app` and add a TUI Preflight view.
- Files changed: `internal/app/preflight.go`, `internal/app/app_test.go`, `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: existing doctor checks now run through `internal/app.Preflight`, which returns structured readiness checks and preserves the same check order, status labels, and detail strings. `revolvr doctor` remains a thin CLI renderer over that app result. The TUI now has a `5 Preflight` view with a `p` rerun action, displays ready/failed/error preflight states, and shows each check detail inline.
- Tests added: app-level preflight snapshot tests for ready and failed checks, a deterministic byte-for-byte CLI doctor output test, CLI TUI wiring coverage for the preflight callback, and TUI model tests for ready and failed preflight views.
- Verification run: `gofmt -w internal/app/preflight.go internal/app/app_test.go internal/cli/doctor.go internal/cli/doctor_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/app -run 'TestPreflight'`; `go test ./internal/cli -run 'TestDoctor|TestTUIRunnerReceivesRefreshOpenAndAddActions'`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli -run 'TestDoctor|TestTUI'`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a nonblocking TUI run-once action with live progress and cancellation.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add receipt validation status to the TUI Run Detail view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Run Detail view now exposes a `v` validation action backed by a `ValidateReceipt` callback wired from `revolvr tui` to `internal/app.ValidateReceipt`. Run Detail renders a dedicated Receipt Validation section showing not-run, passed, failed, and error states, and each returned validation check is shown with explicit `PASS` or `FAIL` messaging. Opening or reloading a run resets stale validation state for that run.
- Tests added: focused TUI model coverage for fully valid receipts, failed validation checks, missing receipt errors, and validation callback errors. CLI TUI wiring coverage now verifies the ValidateReceipt callback is present and works against a ledger-backed valid receipt.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `git diff --check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched because it waits for terminal input.
- What remains: next unchecked backlog item is to move doctor/preflight orchestration behind `internal/app` and add a TUI Preflight view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a dedicated TUI Runs view and richer Run Detail view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Runs view now lists recent runs with status, verification status, commit SHA, and summary. Opening a selected run still uses the app-backed `ShowRun` callback and now renders Run Detail as separate Summary, Diagnostics, Changed Files, Artifacts, and Events sections. Run Detail supports explicit `home`/`end` jumps alongside viewport scrolling, artifact sections show missing paths, changed-file capture gaps are visible, and receipt warnings are surfaced in diagnostics.
- Tests added: focused TUI model coverage for recent-run navigation, opening the selected run detail, diagnostics and receipt warning rendering, artifact and missing-artifact rendering, and scrolling long event output.
- CLI test update: the existing TUI CLI snapshot assertion now expects the richer recent-run row format.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all final commands passed. An initial `go test ./...` run failed only because `internal/cli/root_test.go` still expected the old TUI run row text; the expectation was updated and the full suite then passed.
- What remains: next unchecked backlog item is to add receipt validation status to the TUI Run Detail view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a TUI task creation flow backed by `internal/app.AddTask`.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI now supports an `a` action that opens Add Task mode with task text and summary fields. Empty task text is rejected inline without calling app callbacks, `esc` cancels back to the previous view without writing, and successful submit calls the app-backed add callback, refreshes status, switches to Tasks, and selects the added task when it is present in the refreshed snapshot.
- CLI wiring changed: `revolvr tui` now passes `internal/app.AddTask` through `internal/tui.RunOptions` alongside the existing status refresh and run open callbacks.
- Tests added: focused TUI model coverage for empty validation, cancel without writes, submit with add/refresh callback order, trimmed task and summary input, and selecting the new pending task. CLI TUI wiring coverage now verifies refresh, open, and add callbacks through the command setup.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a dedicated TUI Runs view and richer Run Detail view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a dedicated TUI Tasks view with selection and task detail rendering.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Tasks view now keeps an independent selected task, supports `j/k` and arrow-key movement, renders pending, blocked, and completed tasks in a scannable list, and shows an inline detail section for the selected task with ID, status, summary, task text, blocker, and present timestamps. Blocked tasks use a visible `! blocked` list marker without relying on color.
- Tests added: focused TUI coverage for populated task lists, empty task state, pending task details, blocked task details, and completed task details.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a TUI task creation flow backed by `internal/app.AddTask`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a multi-view TUI shell with explicit Dashboard, Tasks, Runs, Run Detail, and Help/keys areas.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/tui.StatusModel` now has an explicit read-only view model for Dashboard, Tasks, Runs, Run Detail, and Help. Number keys switch views, Runs keeps selection/open actions, Run Detail preserves loaded details while switching away and back, and the shell renders header/footer key help with resize-aware content sizing.
- Tests added: focused TUI coverage for explicit shell rendering, refresh with view switching, loaded run detail preservation, Help/footer rendering, and narrow resize behavior. CLI TUI tests now size the model before snapshot assertions.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched because it waits for terminal input.
- What remains: next unchecked backlog item is to add a dedicated TUI Tasks view with selection and task detail rendering.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: clean the completed backlog out of the active task list and seed a detailed next-phase TUI backlog.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Task list changed: `.agent/TASKS.md` now keeps completed history as a short pointer to `.agent/STATE.md` and Git history, while the active backlog focuses on TUI workflow work: multi-view shell, Tasks view, task creation, Runs/detail views, receipt validation in details, preflight view, nonblocking run-once with cancellation, and layout/documentation polish.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: commit the planning-state update, then seed the runtime queue before using `revolvr run --max-passes`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add basic TUI actions for refresh, opening selected run details, and quit, without starting real Codex runs yet.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/tui.StatusModel` now supports refresh, recent-run selection, opening selected run details, returning from details, and quit actions. `revolvr tui` passes read-only callbacks backed by `internal/app.Status` and `internal/app.ShowRun`; it still does not start Codex or invoke run orchestration.
- Tests added: focused TUI coverage for refresh reloading a status snapshot, selecting/opening a run detail view, and quit command behavior. Focused CLI coverage verifies the TUI runner receives refresh/open callbacks and that the run hook is not invoked.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run 'TestTUI'`; `go test ./internal/tui ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched as a smoke command because it waits for terminal input.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add `revolvr tui` showing task counts, latest run summary, and recent runs from `internal/app`.
- Files changed: `internal/tui/model.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr tui`, which loads the current `internal/app.Status` snapshot and opens the Bubble Tea status model. The command renders uninitialized state, task counts, latest run summary fields, and recent runs through `internal/tui`.
- Tests added: focused CLI coverage for command discovery/help, uninitialized TUI snapshots without creating runtime state, and populated TUI snapshots backed by app-loaded tasks and recent runs.
- Verification run: `gofmt -w internal/tui/model.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'Test(NewRootCommandConstructsExpectedCommands|RootHelpWorks|TUI)'`; `go test ./internal/tui`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched as a smoke command because it waits for terminal input.
- What remains: next unchecked backlog item is to add basic TUI actions for refresh, opening selected run details, and quit, without starting real Codex runs yet.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add stable Charm dependencies for Bubble Tea, Bubbles, and Lip Gloss, and create a minimal `internal/tui` model that renders a static app status snapshot in tests.
- Files changed: `go.mod`, `go.sum`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Dependencies added: direct Charm requirements for Bubble Tea `v1.3.4`, Bubbles `v0.20.0`, and Lip Gloss `v1.1.0`. These are the newest stable tagged versions found that keep compatibility with the repo's `go 1.22` directive; newer Bubble Tea and Bubbles releases require Go 1.23+ or 1.24+.
- Behavior changed: added `internal/tui.StatusModel`, a Bubble Tea model backed by a Bubbles viewport that renders an `internal/app.StatusResult` snapshot for initialized and uninitialized state. It currently supports static rendering, terminal resize messages, viewport updates, and quit keys without adding a CLI command.
- Tests added: focused `internal/tui` coverage for uninitialized output and an initialized static snapshot with task counts, latest run details, and recent runs.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go mod tidy`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add `revolvr tui` showing task counts, latest run summary, and recent runs from `internal/app`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move run once and run loop orchestration behind `internal/app`, preserving CLI output and `run --max-passes` guardrails.
- Files changed: `internal/app/config.go`, `internal/app/run.go`, `internal/app/app_test.go`, `internal/cli/config.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr run --once` and `revolvr run --max-passes` now call `internal/app` for run config loading, pass execution, loop stats, stop reasons, outcome errors, and max-pass guardrail decisions. CLI rendering stays in `internal/cli`, preserving run summaries, Codex progress lines, and final loop summary output. `config check` and `doctor` now share the app-owned run config loader.
- Tests added: focused `internal/app` coverage for run config loading, progress callback wiring, invalid config short-circuiting, repeated-failure guardrails, immediate stop for blocked or dirty failed outcomes, and config-error loop stats. Focused CLI coverage was added for `run --max-passes` config-error summary output.
- Verification run: `gofmt -w internal/app/config.go internal/app/run.go internal/app/app_test.go internal/cli/config.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/app -run 'TestRun(Once|Loop)'`; `go test ./internal/cli -run 'TestRun(Once|MaxPasses)'`; `go test ./internal/app ./internal/cli`; `go test ./...`; `git diff --check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add stable Charm dependencies for Bubble Tea, Bubbles, and Lip Gloss, and create a minimal `internal/tui` model that renders a static app status snapshot in tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move task add/list/retry orchestration behind `internal/app`, update CLI task commands to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/state.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr task add`, `revolvr task list`, `revolvr task retry`, and the shared `task unblock` recovery path now call `internal/app` task operations for state resolution, store access, persistence, and blocked-to-pending transitions. CLI rendering stays in `internal/cli`, preserving existing output.
- Tests added: focused `internal/app` coverage for task add/list persistence, trimmed input, retrying blocked tasks, missing task IDs, missing tasks, and non-blocked task rejection.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go internal/cli/state.go`; `go test ./internal/app -run 'TestTask(Add|Operations|List|Retry)'`; `go test ./internal/cli -run 'TestTask(Add|List|Retry|Unblock)'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move run once and run loop orchestration behind `internal/app`, preserving CLI output and `run --max-passes` guardrails.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move receipt validation orchestration behind `internal/app`, update CLI `receipt validate` to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr receipt validate <run-id>` now uses `app.ValidateReceipt` for state resolution, ledger lookup, and receipt validation. CLI rendering and the failed-check command error remain in `internal/cli`, preserving existing output.
- Tests added: focused `internal/app` coverage for consistent receipt validation, failed checks returned as a result rather than a command error, empty run IDs, uninitialized state, and missing runs.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go`; `go test ./internal/app`; `go test ./internal/cli -run 'TestReceiptValidate'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr receipt --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr receipt validate 019f42a2-5584-7eff-827d-f4420b4e2000`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move task add/list/retry orchestration behind `internal/app`, update CLI task commands to use it without changing output, and add focused tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: introduce `internal/app` with read-only `Status` and `ShowRun` operations, update CLI `status` and `show` to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr status` and `revolvr show <run-id>` now load their read-only snapshots through `internal/app`; CLI rendering remains in `internal/cli` and existing output is unchanged.
- Tests added: focused `internal/app` coverage for uninitialized status reads, populated status snapshots with latest run events, persisted show history, missing runs, and uninitialized show reads.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go`; `go test ./internal/app`; `go test ./internal/cli -run 'TestStatus|TestShow'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr show 019f42a2-5584-7eff-827d-f4420b4e2000`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move receipt validation orchestration behind `internal/app`, update CLI `receipt validate` to use it without changing output, and add focused tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: seed the durable backlog with a TUI-readiness sequence for an `internal/app` service boundary and a future Bubble Tea frontend.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Tasks added: extract read-only `internal/app` status/show operations; move receipt validation into app; move task add/list/retry into app; move run once/loop orchestration into app; add stable Charm dependencies and a minimal `internal/tui` model; add `revolvr tui` dashboard; add basic TUI refresh/open/quit actions.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: commit these planning-state updates, then run a bounded multi-pass dogfood loop starting with the first unchecked backlog item.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a live dogfood verification script or README checklist that resets runtime state, queues a tiny task, runs once, and verifies receipt, ledger, commit, receipt validation, and clean-worktree consistency.
- Files changed: `scripts/dogfood-live.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Script added: `scripts/dogfood-live.sh` is an opt-in real-Codex dogfood check. It requires `codex`, Git identity, and a clean source worktree; builds a temporary `revolvr` binary; removes `.revolvr/`; initializes fresh runtime state; writes a one-command `go test ./...` verification config; queues a tiny single-file task; runs `revolvr run --once`; and checks the final receipt, ledger-backed `status` and `show` output, commit SHA, `receipt validate`, and final clean worktree.
- Documentation added: README Dogfooding and Development Checks sections now point to the live dogfood script and warn that it resets local `.revolvr/` state and creates a commit on success.
- Verification run: `bash -n scripts/dogfood-live.sh`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go test ./...`.
- Verification result: all commands passed. The live script itself was not executed in this pass because the selected run rules prohibit launching nested Codex runs.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add safer `run --max-passes` loop guardrails for repeated failures or blocked tasks, and show a concise final loop summary.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `run --max-passes` now always prints one final `Loop summary` line for no-task, max-pass, failed, blocked, runner-error, context, and config-error exits. The bounded loop stops immediately after blocked outcomes or failed outcomes that report changed files/capture errors, and clean repeated failed outcomes trip a two-pass failure guardrail.
- Tests added: focused CLI coverage for final summaries on no-task and max-pass exits, repeated clean failures, blocked outcomes, and failed dirty passes.
- Documentation added: README run docs now note the final loop summary and early stop behavior for failed or blocked passes.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestRunMaxPassesStopsAfterNoTask|TestRunMaxPassesStopsAfterRepeatedFailuresWithSummary|TestRunMaxPassesStopsAfterBlockedOutcomeWithSummary|TestRunMaxPassesStopsAfterFailedPassWithChangedFiles|TestRunMaxPassesCapIsHonored'`; `go test ./internal/cli`; `git diff --check`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a live dogfood verification script or README checklist that resets runtime state, queues a tiny task, runs once, and verifies receipt, ledger, commit, and clean-worktree consistency.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add focused failure-recovery CLI support for blocked tasks with `revolvr task retry <task-id>`.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr task retry <task-id>` as the recovery-oriented command for blocked tasks. It reuses the existing blocked-to-pending store update, rejects missing or non-blocked tasks, clears current blocker fields, preserves the same task ID/text/summary/created timestamp, and leaves the existing `task unblock` command available.
- Tests added: focused CLI coverage for command discovery/help, successful retry making the same blocked task selectable by `run --once`, completed-task rejection, and missing-task rejection.
- Documentation added: README task queue recovery example now uses `task retry <task-id>`.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestNewRootCommandConstructsExpectedCommands|TestParentCommandHelpOutput|TestTaskRetryMakesBlockedTaskRunnableForRunOnce|TestTaskRetryDoesNotRevertCompletedTask|TestTaskRetryMissingTaskReturnsClearError|TestTaskUnblockDoesNotRevertCompletedTask|TestTaskUnblockMissingTaskReturnsClearError'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr task retry --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add safer `run --max-passes` loop guardrails for repeated failures or blocked tasks, and show a concise final loop summary.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a first-class receipt validation command that checks a run receipt against ledger completion time, commit SHA, changed files, verification results, and artifact existence.
- Files changed: `internal/receipt/validation.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr receipt validate <run-id>`, which loads the run ledger history, parses the recorded receipt, compares receipt identity, finalized timestamp, commit SHA, changed files, verification results, and recorded artifact paths, prints per-check results, and exits nonzero when validation fails.
- Tests added: focused CLI coverage for a fully consistent receipt and for mismatched timestamp, changed files, verification results, and a missing artifact.
- Documentation added: README now documents `receipt validate <run-id>` in the status/show inspection flow.
- Verification run: `gofmt -w internal/receipt/validation.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/receipt`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr receipt --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`; final `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add focused failure-recovery CLI support for blocked tasks, starting with one command to retry or unblock a blocked task safely.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: expand `revolvr status` to show latest run summary, verification status, commit SHA, and artifact path hints when a run exists.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `status` now loads the latest run's event history and prints the latest summary, verification status, commit SHA, and artifact paths after the existing latest run line. Missing latest-run fields render as `none`; artifact paths reuse the same order as `show`.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a first-class receipt validation command.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: document the next harness-usefulness improvements in the durable task backlog for continued dogfooding.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Tasks added: richer latest-run `status` output; receipt validation command; blocked-task retry/unblock support; `run --max-passes` guardrails and loop summary; live dogfood verification script/checklist.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: start the first unchecked backlog item with a fresh dogfood pass.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: fix finalized receipt timestamps so the harness overwrites stale agent-authored timestamps with the run completion time.
- Files changed: `internal/receipt/update.go`, `internal/receipt/receipt_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: receipt finalization now writes the harness completion timestamp into parsed receipts, and run completion uses the same timestamp for the ledger and final receipt.
- Verification run: `gofmt -w internal/receipt/update.go internal/receipt/receipt_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/receipt`; `go test ./internal/runonce`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: commit if requested, then run another real dogfood pass to confirm receipt timestamps in the live path.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add one concise README Dogfooding note that real dogfood runs should start from a clean worktree and use `status`/`show` to inspect the result.
- Files changed: `README.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: Dogfooding now explicitly notes that real runs should start from a clean worktree and inspect recorded results with `status` and `show <run-id>`.
- Verification run: not run; documentation-only change and the Revolvr harness owns pass verification.
- Verification result: not run.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: resolve the dogfood run diagnostics found after the README dogfooding pass: stale receipt body facts, false `.agent/STATE.md` changed-file mismatch warning, zero Codex usage metrics, and missing live `run` progress output.
- Files changed: `internal/receipt/claims.go`, `internal/receipt/metrics.go`, `internal/receipt/update.go`, `internal/receipt/receipt_test.go`, `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: final receipt rewrites now refresh harness-owned `Changed Files` and `Verification` body sections to match finalized frontmatter; dotfile path claims keep their leading `.`; Codex usage parsing continues past malformed JSONL fragments when a later valid usage event exists; `revolvr run --once` and `run --max-passes` stream summarized Codex progress to stdout before the final summary.
- Verification run: `gofmt -w internal/receipt/claims.go internal/receipt/metrics.go internal/receipt/update.go internal/receipt/receipt_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/receipt`; `go test ./internal/codexexec`; `go test ./internal/cli`; `go test ./internal/runonce`; `bash -n scripts/smoke-local.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: commit the completed repair slice.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a README Dogfooding section with the commands for `doctor`, `task add`, `run --once`, `status`, and `show`.
- Files changed: `README.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: grouped the dogfooding flow into one README section with commands for preflight, queueing a task, running one pass, checking status, and showing a recorded run.
- Verification run: not run; documentation-only change and the Revolvr harness owns pass verification.
- Verification result: not run.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a `revolvr doctor` dogfood preflight for Codex, Git identity, clean worktree, runtime ignore state, and verification readiness.
- Files changed: `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr doctor` now reports initialized state, effective config loading, configured Codex executable availability, configured Git executable availability, Git identity, clean worktree state, `.revolvr/` Git ignore readiness, and effective verification command readiness. It exits nonzero with `doctor: preflight failed` when required checks are not ready.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/doctor.go internal/cli/root_test.go internal/cli/doctor_test.go`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; expected-failure `go run ./cmd/revolvr doctor` on the dirty implementation checkout, asserting `Dogfood preflight:`, `Ready: false`, and `doctor: preflight failed`; final `go test ./internal/cli` after cleanup.
- Verification result: all commands passed, with the doctor command failing only in the expected pre-commit dirty-worktree check.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: make `revolvr init` locally ignore `.revolvr/` in Git worktrees so fresh dogfood repos do not start dirty.
- Files changed: `internal/cli/state.go`, `internal/cli/root_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr init` now idempotently adds `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, leaving non-Git directories alone. The fake-Codex smoke tests no longer create a tracked `.gitignore` and assert that post-init Git status stays clean.
- Verification run: `gofmt -w internal/cli/state.go internal/cli/root_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` verification failure path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex-verification-failure.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, has fake Codex create `generated.txt` and a matching failure receipt, intentionally fails verification with `test -f required.txt`, checks the run failure summary, confirms the task is blocked, confirms no commit is created, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` success path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, verifies `generated.txt`, runs `revolvr run --once`, checks completed task/run status, confirms a commit, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Files changed: `scripts/smoke-local.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-local.sh` builds a temporary `revolvr` binary, runs `init`, `task add`, `task list`, `config check`, and `status` in a temporary workspace, and asserts expected outputs without invoking Codex.
- Verification run: `bash -n scripts/smoke-local.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: root README covering setup/build, `init`, task queue commands, optional `.revolvr/config.yaml`, `config check`, `run --once`, `run --max-passes`, `status`, `show`, and development checks.
- Verification run: `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: expand `revolvr config check` output to show effective verification command details, not only the command count.
- Files changed: `internal/cli/config.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `config check` now prints one deterministic detail row per effective verification command after `Verification command count`, including command index, name, args, and optional dir/timeout fields. No detail rows are printed when the effective command list is empty.
- Verification run: `gofmt -w internal/cli/config.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI; the targeted smoke-test note/script task also remains.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add Codex yolo/dangerous bypass support for autonomous harness runs, fix fresh-session wrapper flags, and update focused tests.
- Files changed: `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/cli/config.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `agent-one.sh`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: CLI-initiated harness runs now default to Codex dangerous bypass/yolo mode and pass `--dangerously-bypass-approvals-and-sandbox` instead of separate sandbox/approval flags when enabled. Config supports `codex.dangerously_bypass_approvals_and_sandbox` and `codex.yolo` aliases, including explicit `false` to disable the default. `agent-one.sh` now uses the dangerous bypass flag with valid `codex exec` ordering.
- Verification run: refreshed Codex manual with `node /home/gernsback/.codex/skills/.system/openai-docs/scripts/fetch-codex-manual.mjs`; `gofmt -w internal/cli/root.go internal/cli/config.go internal/cli/root_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config`; `bash -n agent-one.sh agent-loop.sh`; `codex exec --dangerously-bypass-approvals-and-sandbox --help`; `codex exec --yolo --help`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous task completed on 2026-07-07:

- Selected task: replace bare parent command placeholder output for `revolvr task` and `revolvr config` with normal help output, and update focused CLI tests.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: bare `revolvr task` and `revolvr config` now show Cobra help instead of placeholder "not implemented yet" output.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr task`; `go run ./cmd/revolvr config`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous setup performed on 2026-06-29:

- Initialized local Revolvr runtime state with `go run ./cmd/revolvr init`.
- Created fresh-session agent loop setup files.
- Did not run `agent-one.sh`; that would invoke a nested Codex session.

## Current Repository Understanding

- Stack: Go 1.22 CLI application using Cobra, YAML config, and SQLite.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test ./...`.
- Lint/typecheck command: none configured; use `gofmt -w <changed go files>` and `go test ./...`.
- Important directories: `cmd/revolvr`, `internal/cli`, `internal/runonce`, `internal/ledger`, `internal/taskqueue`, `internal/codexexec`, `internal/receipt`, `internal/verification`, `internal/commit`, `internal/gitstate`, `internal/runner`.
- Runtime state: `.revolvr/`, created by `revolvr init` and ignored by Git.

## Verification Gaps

- No separate lint command is configured.

## Notes For Next Fresh Session

- Read `AGENTS.md` first.
- Read `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md` before making changes.
- Do one task, verify, update state, and stop.
- Do not run nested Codex from inside another active Codex session.
