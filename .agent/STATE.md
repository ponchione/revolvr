# Agent State

## Current Focus

No task is currently in progress. No unchecked backlog items remain in `.agent/TASKS.md`.

## Dogfood Timestamp Verification

- 2026-07-08T13:04:17Z live run `019f41d3-9120-7a77-92fd-d799f76ba000`: verifies receipt timestamp finalization after the prior fix by writing the receipt with the prompt-provided stale timestamp.

## Last Run

Task completed on 2026-07-09:

- Selected task: document the mixed-pass task workflow.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Documentation added: README now distinguishes a canonical `.agent/tasks/*.md` durable task from each fresh pass/run, shows a minimal `mixed-pass-v1` task, documents `implement -> audit -> document -> simplify -> completed`, summarizes the four policy-selected profiles and their no-change rules, states that task frontmatter `profile` is not an override, explains Revolvr-owned pre-commit phase advancement and same-phase failure retry, and points operators to the CLI and TUI workflow inspection surfaces. It also clarifies that receipts and ledger events audit policy-driven harness outcomes rather than choosing the next phase.
- Tests/verification run: `go test ./...`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr task list`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: passed.
- Remaining work: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface task workflow state in CLI, TUI, status, and timeline views.
- Files changed: `internal/taskmodel/task.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/app/timeline.go`, `internal/app/timeline_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: app task adaptation now resolves each file-backed task through `internal/passpolicy.Lookup` and exposes workflow, current phase, mapped run profile, and next phase or `completed` on `taskmodel.Task`; lookup failures return a task-specific error. `revolvr task list` now prints stable workflow/phase/profile/next columns, and `revolvr status` prints a concise next-task and next-pass summary when a pending task exists while preserving the empty-task output. The TUI Dashboard and Tasks views now show the next task's workflow state, every task row shows phase/profile/next, and selected task detail shows workflow/phase/profile/next. `task_selected` timeline rows append available workflow/phase/profile payload metadata while legacy payload formatting and raw TUI event rows remain intact.
- Tests/verification run: `gofmt -w internal/taskmodel/task.go internal/app/app.go internal/app/app_test.go internal/app/timeline.go internal/app/timeline_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/app ./internal/cli ./internal/tui`; `go test ./...`; `go run ./cmd/revolvr task list`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to document the mixed-pass task workflow.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: seed `.agent/profiles/simplifier.md`.
- Files changed: `.agent/profiles/simplifier.md`, `internal/prompt/profile.go`, `internal/prompt/profile_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `prompt.DefaultRunProfileTemplates` now includes the repo-authored `simplifier` profile. `revolvr init` seeds `.agent/profiles/simplifier.md` alongside `implementer.md`, `auditor.md`, and `documentor.md` when missing, while preserving any existing simplifier profile file. The checked-in profile instructs agents to simplify only when meaningful, preserve behavior, avoid clever abstractions, create helpers only for real duplication or complexity reduction, and stop cleanly when no worthwhile simplification exists.
- Tests/verification run: `gofmt -w internal/prompt/profile.go internal/prompt/profile_test.go internal/cli/root_test.go`; `go test ./internal/prompt ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to surface task workflow state in CLI, TUI, status, and timeline views.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: allow policy-permitted no-change success and durable phase advancement.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: successful runonce passes now apply the selected task's `internal/passpolicy.Policy` after Codex and verification succeed and before the commit gate. Implement passes still need pre-metadata changed files and now advance the durable task to `phase: audit` with `status: pending`; audit advances to `phase: document`, document advances to `phase: simplify`, and simplify marks the task `completed`. Audit/document/simplify can succeed with no source changes because the task-file metadata update becomes the committed durable change. Changed files are recaptured after metadata updates so commits, receipts, and terminal ledger events include the task-file transition. Failed/blocking outcomes do not advance phase; if a commit fails after a metadata update, the task is marked blocked at its original phase.
- Tests/verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go`; `go test ./internal/runonce ./internal/taskfile`; `go test ./internal/app`; `go test ./internal/passpolicy`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to seed `.agent/profiles/simplifier.md`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: teach `runonce` to load the profile for the selected task phase.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/runonce` now resolves the selected task file's `workflow` and `phase` with `internal/passpolicy.Lookup` and loads the mapped repo-authored profile before building the context bundle. Default and explicit implement phases load `implementer`, audit loads `auditor`, document loads `documentor`, and an unseeded simplify phase blocks before Codex with the existing missing-profile failure path. The task-selected ledger event now records workflow, phase, and profile name for audit; context manifests continue recording the exact loaded profile path, SHA-256, and byte size. Successful runs still mark the selected task file `completed`; no phase advancement or no-change success policy was added.
- Tests/verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/runonce ./internal/prompt`; `go test ./internal/passpolicy ./internal/taskfile`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to allow policy-permitted no-change success and durable phase advancement.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a pass-policy model mapping phases to profiles and outcome semantics.
- Files changed: `internal/passpolicy/policy.go`, `internal/passpolicy/policy_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/passpolicy.Lookup` for `mixed-pass-v1`, mapping `implement -> audit -> document -> simplify -> completed` with profile names `implementer`, `auditor`, `documentor`, and `simplifier`; `implement` disallows no-change success, later phases allow it, and `simplify` is terminal with no next phase. Unsupported workflows and phases now return clear lookup errors from the policy package. No runtime behavior changed in `runonce`.
- Tests/verification run: `gofmt -w internal/passpolicy/policy.go internal/passpolicy/policy_test.go`; `go test ./internal/passpolicy`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to teach `runonce` to load the profile for the selected task phase.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add task workflow/phase metadata parsing and defaults.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `.agent/tasks/*.md` parsing now exposes `workflow` and `phase` on `taskfile.Task`, defaults missing workflow to `mixed-pass-v1`, defaults missing phase to `implement`, accepts `implement`, `audit`, `document`, and `simplify`, validates invalid workflow/phase values clearly, and treats duplicate `workflow`/`phase` as duplicate known frontmatter keys. Status-only task updates continue preserving workflow/phase frontmatter.
- Tests/verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go`; `go test ./internal/taskfile`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to add a pass-policy model mapping phases to profiles and outcome semantics.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: seed the mixed loop pass progression backlog for durable task phases.
- Files changed: `.agent/TASKS.md`, `.agent/DECISIONS.md`, `.agent/STATE.md`.
- Backlog added: small ordered slices for task workflow/phase metadata, pass-policy modeling, phase-based profile loading, no-change success with durable phase advancement, `simplifier` profile seeding, workflow state visibility, and mixed-pass documentation.
- Decision update: durable tasks and passes are distinct; workflow state lives in canonical task files; Revolvr owns phase transitions through policy; audit/document/simplify may succeed without code changes when policy permits; successful advancement should update the task file before the commit gate when appropriate.
- Verification run: `git diff --check`.
- Verification result: passed.
- What remains: next unchecked backlog item is to add task workflow/phase metadata parsing and defaults.
- Blockers: none.

Audit completed on 2026-07-09:

- Selected task: audit obsolete migration surface after moving to file-backed tasks, repo-authored profiles, current context artifacts, and fresh Codex sessions.
- Files changed: `README.md`, `CODEX_AGENT_LOOP_HANDOFF.md`, `CODEX_HARNESS_TARGETS.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`.
- Obsolete surface removed: current operator/developer docs, TUI empty-state text, and CLI test helper names no longer describe tasks as a queue; archived setup/design docs now point readers to the current `.agent/tasks/*.md`, `.agent/profiles/*.md`, `context.md`/`context.json`, and fresh `codex exec` architecture.
- Audit result: no tracked runtime code remains for `internal/taskqueue`, `.revolvr/tasks.sqlite`, `prompt_path`, `prompt.md`, `prompt_built`, or historical context artifact compatibility. Valid SQLite usage is limited to the run ledger at `.revolvr/ledger.sqlite`; ledger artifact decoding only accepts `context_payload_path` and `context_manifest_path` for run context artifacts; app, CLI, TUI, and runonce task paths use `internal/taskfile` plus `internal/taskmodel`.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; stale-term `rg` sweeps; `go list ./...`; `bash -n scripts/smoke-local.sh scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `git diff --check`; `go test ./...`; temp-worktree `revolvr init` smoke confirming no `.revolvr/tasks.sqlite`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `./scripts/smoke-local.sh`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: remove obsolete SQLite task queue and historical prompt artifact compatibility before publishing.
- Files changed: `AGENTS.md`, `CODEX_AGENT_LOOP_HANDOFF.md`, `CODEX_HARNESS_TARGETS.md`, `README.md`, `.agent/DECISIONS.md`, `.agent/STATE.md`, `internal/taskmodel/task.go`, `internal/taskqueue/store.go`, `internal/taskqueue/store_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/state.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/ledger/events.go`, `internal/ledger/artifacts.go`, `internal/ledger/artifacts_test.go`, `internal/app/timeline.go`, `scripts/smoke-local.sh`, `scripts/dogfood-live.sh`.
- Behavior changed: initialization no longer creates or requires `.revolvr/tasks.sqlite`; the old SQLite task queue package has been removed; task display/status data now uses `internal/taskmodel` while canonical task storage remains `.agent/tasks/*.md`.
- Compatibility removed: ledger artifact decoding no longer accepts `prompt_path`, `EventPromptBuilt` has been removed, and timeline/context artifact rendering now only uses `context_built` plus `context_payload_path` / `context_manifest_path`.
- Verification run: `gofmt -w internal/taskmodel/task.go internal/app/app.go internal/app/app_test.go internal/cli/state.go internal/cli/root.go internal/cli/root_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go internal/tui/model.go internal/tui/model_test.go internal/ledger/events.go internal/ledger/artifacts.go internal/ledger/artifacts_test.go internal/app/timeline.go`; `go test ./internal/taskmodel ./internal/ledger ./internal/app`; `go test ./internal/cli ./internal/runonce ./internal/tui`; `go list ./...`; `go test ./...`; `bash -n scripts/smoke-local.sh scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `git diff --check`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: update operator-facing README TUI documentation for bounded multi-pass loops.
- Files changed: `README.md`, `.agent/STATE.md`.
- Documentation changed: README now documents `n` loop max-pass cycling, `L` bounded loop start, cancellation for an active run or loop, and progress-pane pass summaries. The chat-to-task workflow still recommends a single `R` pass for one bounded pass and now also mentions the optional TUI loop; CLI `run --max-passes` remains documented as the non-TUI equivalent.
- Verification run: `go test ./...`; `go run ./cmd/revolvr tui --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
- Files changed: `internal/app/run.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: the TUI now keeps `R` as single-pass run-once and adds a bounded loop action on `L` backed by the app-wired `internal/app.RunLoop`. The loop pass count defaults to 3 and cycles through 2/3/5 with `n`; active loops reuse the existing preflight blocker, cancellation key `c`, progress pane, status refresh, and latest-run detail opening.
- Progress behavior changed: the existing `Run Progress` pane now distinguishes loop mode, shows max passes, pass counts, completed/failed-or-blocked/no-task/consecutive-failure stats, stop reason, latest run ID, Codex progress, and per-pass summaries streamed through `RunLoopInput.OnPass`. Cancellation cancels the loop context, refreshes status, and renders `cancelled` with `context_cancelled` when applicable.
- CLI wiring changed: `revolvr tui` now passes a `RunLoop` callback into `internal/tui` that calls `app.RunLoop` with the same configured runner used by CLI/TUI run once, plus progress and pass callbacks. `app.RunLoop` now reports `context_cancelled` when the runner returns because the loop context was cancelled inside a pass.
- Tests added/updated: focused TUI model coverage for loop max-pass completion, no-task stop, repeated failure guardrail, blocked stop, cancellation, and pass-count cycling; CLI TUI wiring coverage now verifies the `RunLoop` callback; app coverage now verifies runner-context cancellation stop reason.
- Verification run: `gofmt -w internal/app/run.go internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: render the run timeline in CLI `show` and TUI Run Detail.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: `revolvr show <run-id>` now prints a `Timeline:` section immediately after run summary fields and before artifacts, diagnostics, and raw events. The section uses `internal/app.RunTimeline` with deterministic tabular columns for timestamp, phase, status, and detail, uses CLI time formatting for real timestamps, and renders missing row timestamps as `none`.
- TUI behavior changed: Run Detail now renders a `Timeline` section after `Summary` and before `Diagnostics` using the same app projection and compact single-line rows. Raw `Events` remain visible below artifacts, and long timeline/event detail remains scrollable through the existing viewport.
- Tests added/updated: CLI `show` exact-output coverage now includes timeline rows for persisted run start/completion, artifact, diagnostic, warning, and sparse histories, plus an empty timeline helper check. TUI coverage now proves Run Detail includes timeline rows while keeping raw event rows, renders an empty timeline clearly, and scrolls through a long timeline while still reaching raw events.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add an app-level run timeline projection from ledger events.
- Files changed: `internal/app/timeline.go`, `internal/app/timeline_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: added `internal/app.RunTimeline`, which projects a `ledger.RunWithEvents` history into reusable app-level `RunTimelineRow` values with timestamp, phase, status, and concise detail. The projection preserves ledger event order, uses event `CreatedAt` timestamps, covers run start, task selection, context, Codex start/progress/completion, changed files, receipts, verification, commit, and terminal outcome rows, and keeps the slice app-only for later CLI/TUI rendering.
- Fallback behavior added: when start or terminal events are missing, the projection uses the ledger run row for started/completed timestamps, run status, summary, verification status, Codex exit code, and commit SHA. Malformed, missing, or partial event payloads return generic deterministic rows instead of panicking.
- Tests added: focused `internal/app` coverage for completed, verification-failed, Codex-failed, blocked/pre-run failure, sparse missing-event fallback, and malformed payload histories with exact expected row slices.
- Verification run: `gofmt -w internal/app/timeline.go internal/app/timeline_test.go`; `go test ./internal/app`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to render the run timeline in CLI `show` and TUI Run Detail.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: migrate app-level task write operations to canonical `.agent/tasks/*.md`.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model_test.go`, `README.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/app.AddTask`, write-mode `ImportTasks`, `RetryTask`, and `UnblockTask` now operate on canonical Markdown task files under `.agent/tasks/` instead of mutating SQLite task rows. New task files are written with `id` plus `status: pending` frontmatter and an H1 title derived from the provided summary or first task line. Retry/unblock find file-backed tasks by ID and only transition `blocked` tasks back to `pending`, preserving clear missing-task and non-blocked-task errors.
- Taskfile helpers added: `internal/taskfile.Create` writes canonical pending task files with generated IDs, `FindByID` resolves frontmatter or filename-derived task IDs and reports duplicates, and `UpdateBlockedToPending` reuses status-frontmatter updates for blocked-to-pending transitions.
- CLI/TUI behavior preserved: `task add` keeps the concise confirmation output based on the original task text and summary, while `task add` followed by `task list`, write-mode `task import`, TUI add refresh, and retry/unblock refresh paths now expose file-backed task changes through the existing `taskqueue.Task` adapter shape. `task import --dry-run` remains non-mutating, and validation/parse failures do not create `.agent/tasks/` or `.revolvr/`.
- Documentation changed: README now describes `.agent/tasks/*.md` as the shared task state and `.revolvr/` as local runtime state with transitional task database infrastructure.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/app/app.go internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model_test.go`; `go test ./internal/taskfile`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface canonical `.agent/tasks/*.md` tasks through app status, CLI task list/status, and existing TUI task views.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/app.ListTasks` and `internal/app.Status(...).Tasks` now load direct Markdown task files through `internal/taskfile` instead of reading SQLite queue rows. File tasks are adapted into the existing `taskqueue.Task` shape with task-file ID, full Markdown body as `Task`, H1 title as `Summary`, and task-file status preserved, including `running`.
- Status behavior preserved: `app.Status` still uses the existing initialized-state check and still loads recent runs plus latest run events from the ledger exactly as before; only the task slice source changed.
- CLI/TUI changed: `revolvr status`, `revolvr task list`, and `revolvr tui` task/dashboard views now surface file-backed tasks through the existing render paths. `task add`, `task import`, `task retry`, and `task unblock` remain transitional SQLite-backed write operations in this slice, so tests that cover those writes inspect the legacy queue directly instead of expecting app status refreshes to expose SQLite-only rows.
- Tests updated: app coverage now proves file-backed status/list conversion, deterministic taskfile order, full Markdown task bodies, H1 summaries, status preservation, and zero timestamp/blocker metadata. CLI coverage now counts file-backed tasks in status, lists file-backed tasks in filename order, and verifies TUI Dashboard/Tasks rendering from app status for pending, blocked, completed, and next-runnable file tasks while keeping write callback tests scoped to SQLite writes.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root_test.go`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: fix file-backed runonce integration so fake-Codex smoke scripts pass and successful task status updates do not leave the worktree dirty.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `scripts/dogfood-live.sh`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: successful runonce passes now keep the selected task manifest source metadata based on exact pre-run file bytes, wait until Codex and verification have passed, update the selected Markdown task to `status: completed`, recapture changed files, and pass that recaptured list to the commit runner so the task status flip is committed with the successful run. The pre-status changed-file capture still gates `no_changes`, so a task-status-only mutation does not turn an empty Codex pass into a success. Failed and blocking outcomes continue to mark selected task files `blocked` without committing.
- Script changes: the fake-Codex success and verification-failure smokes now create committed canonical `.agent/tasks/*.md` task files instead of using `revolvr task add`; they assert run artifacts and receipts, selected-task manifest source metadata from the pending file bytes, `completed` on success, and `blocked` on verification failure without relying on SQLite task counts. `scripts/dogfood-live.sh` now writes and commits a canonical live dogfood task file before the run and expects the successful run commit to include both the requested dogfood file and the task status update.
- Tests added/updated: runonce coverage now proves the successful commit changed-file capture includes the completed selected task file, a real-Git second pending file task can run after a successful first task without leftover dirty status, and smoke-shaped task files produce `selected_task` manifest metadata from exact pre-run bytes.
- Verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/taskfile`; `go test ./internal/runonce`; `go test ./internal/prompt`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: wire runonce to select canonical file-backed Markdown tasks from `.agent/tasks/*.md`.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/runonce` now selects the next pending Markdown task via `internal/taskfile.SelectNext`, ordered by priority then filename, and returns `OutcomeNoTask` when no pending task files exist without falling back to SQLite queue rows.
- Context/ledger changed: selected run fields use `taskfile.Task.ID`, `ContextBody`, and `Title`; prompt input now passes `prompt.SourceContent{Path: task.SourcePath, Content: task.SourceBytes}` so `context.json` records selected-task path, SHA-256, and byte size from the exact pre-run file bytes. Run profile selection remains the default `implementer` profile only.
- Task status changed: `internal/taskfile.UpdateStatus` can replace, insert, or add deterministic `status` frontmatter while preserving other frontmatter/body content as much as practical. Committed runs mark the selected Markdown task `completed`; blocking/failed terminal outcomes mark it `blocked`; no-task and failures before task selection do not mutate task files.
- Transitional note: the old SQLite task queue package, app/CLI task commands, and compatibility result fields remain in place, but runonce no longer consumes SQLite pending tasks.
- Tests added/updated: taskfile status replacement/insertion/frontmatter-add coverage; runonce coverage for file priority/filename selection, exact selected-task manifest metadata from pre-run bytes, committed-to-completed mutation, failed/blocking-to-blocked mutation, second run after completion returning no-task, pre-selection lock failure leaving files pending, and SQLite pending tasks being ignored when no task file exists.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/taskfile`; `go test ./internal/runonce`; `go test ./internal/prompt`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: introduce file-backed Markdown task specs under `.agent/tasks/` and selected-task source metadata in the context manifest.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `internal/cli/state.go`, `internal/cli/root_test.go`, `internal/prompt/prompt.go`, `internal/prompt/context.go`, `internal/prompt/prompt_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/taskfile` for dependency-free loading of direct `.agent/tasks/*.md` files with optional YAML-ish `id`, `profile`, `status`, and `priority` frontmatter, required non-empty H1 titles, status/profile/path validation, exact source bytes, SHA-256 and byte-size helpers, all-file listing, runnable pending ordering by numeric priority then filename, and select-next behavior.
- Init changed: `revolvr init` now creates `.agent/tasks/` when missing, leaves it empty by default, and preserves existing task Markdown files across repeated init runs.
- Context manifest changed: `prompt.Input` can now carry a selected task source path plus exact source bytes, and `context.json` records `selected_task` path, SHA-256, and byte size from those exact bytes when provided. The existing task-text fallback remains for the transitional queue-backed path.
- Transitional note: this pass did not remove or rewrite the existing SQLite queue-backed runtime selection path; the new file-backed loader is the canonical task-file foundation for the next runtime wiring step.
- Tests added/updated: focused taskfile loader/parser tests for valid files, missing H1, invalid status, unsafe profile, path containment, direct-file loading, deterministic runnable ordering, select-next behavior, and exact source hash/byte size; CLI init coverage for task directory creation, empty default contents, and idempotent non-overwrite; prompt manifest coverage for selected task file source metadata.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/prompt/context.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/cli/state.go internal/cli/root_test.go`; `go test ./internal/taskfile`; `go test ./internal/prompt`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: make run profiles file-backed and repo-authored.
- Files changed: `.agent/profiles/implementer.md`, `.agent/profiles/auditor.md`, `.agent/profiles/documentor.md`, `internal/prompt/profile.go`, `internal/prompt/profile_test.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/prompt/context.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/cli/state.go`, `internal/cli/root_test.go`, `internal/cli/doctor_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: run profiles are now loaded from `.agent/profiles/<name>.md` relative to the repository root. The default runtime profile name remains `implementer`, but missing, empty, or unsafe profile names now fail clearly instead of falling back to embedded text.
- Init changed: `revolvr init` seeds `implementer.md`, `auditor.md`, and `documentor.md` under `.agent/profiles/` without overwriting existing files; `.revolvr/` state and local Git exclude behavior remain intact.
- Context bundle changed: `context.md` renders the loaded profile markdown as the `## Run Profile` body, and `context.json` records the run profile source path plus SHA-256 and byte size for the profile content used in the run.
- Tests added/updated: focused profile-loader coverage for success, default-name loading, missing files, empty files, and unsafe names; runonce coverage for file-backed profile content and manifest metadata plus missing-profile blocking before Codex; CLI init coverage for profile seeding and non-overwrite behavior; doctor/smoke tests updated for repo-authored profile files.
- Verification run: `gofmt -w internal/cli/doctor_test.go internal/cli/root_test.go internal/cli/state.go internal/prompt/context.go internal/prompt/profile.go internal/prompt/profile_test.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: finish context-bundle migration cleanup in smoke scripts and operator-facing docs.
- Files changed: `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `scripts/dogfood-live.sh`, `README.md`, `internal/ledger/ledger_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: fake-Codex smoke checks and the live dogfood script now assert `.revolvr/runs/<run-id>/context.md` and `.revolvr/runs/<run-id>/context.json` instead of obsolete `prompt.md`, validate the main context Markdown sections, validate manifest keys for payload path and SHA-256, and check CLI `show` output for both context artifact paths.
- Documentation changed: README now describes run passes as writing `context.md` plus `context.json` and sending that context payload to Codex.
- Verification run: `gofmt -w internal/ledger/ledger_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/ledger`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: introduce an auditable run context bundle as the canonical prompt architecture.
- Files changed: `internal/prompt/context.go`, `internal/prompt/profile.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/ledger/events.go`, `internal/ledger/artifacts.go`, `internal/ledger/artifacts_test.go`, `internal/receipt/validation.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/app/app_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: each run now writes an exact Markdown context payload at `.revolvr/runs/<run-id>/context.md` and an auditable JSON manifest at `.revolvr/runs/<run-id>/context.json` before Codex starts. The manifest records run/task/profile identity, payload path, payload SHA-256 and byte size, generation time, and source entries for the selected task and run profile. Ledger run artifacts now expose `context_payload_path` and `context_manifest_path`, while old `prompt_path` events still decode as legacy context payloads.
- Runtime flow preserved: `codex exec` still receives the full uncompressed Markdown context payload in a fresh session; no resume flow or context compression was added.
- Tests added/updated: prompt context-manifest hash/byte-size and JSON tests, runonce context-bundle artifact coverage proving the manifest references the exact bytes sent to Codex, ledger artifact context-path coverage plus legacy `prompt_path` fallback, and updated CLI/TUI/app artifact fixtures and rendering expectations.
- Verification run: `gofmt -w internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/ledger/artifacts.go internal/ledger/artifacts_test.go internal/ledger/events.go internal/prompt/context.go internal/prompt/profile.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/receipt/validation.go internal/runonce/runonce.go internal/runonce/runonce_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/ledger`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: begin the new prompt architecture with the smallest safe implementation step by adding run profiles and richer prompt assembly.
- Files changed: `internal/prompt/profile.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/runonce/runonce_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/prompt` now has a `RunProfile` type and built-in default profile named `implementer`. Prompt artifacts now render predictable sections in order: `# Revolvr Codex Pass`, `## Run Profile`, `## Selected Task`, `## Repository Rules`, `## Artifact Paths`, `## Required Receipt Schema`, and `## Stop Condition`.
- Runtime flow preserved: `runonce` still selects the next task from the current queue, writes `.revolvr/runs/<run-id>/prompt.md`, and invokes fresh `codex exec` through the existing path without resume or task-storage migration.
- Tests added/updated: prompt builder snapshot/order coverage for the new profile section, default profile coverage when no explicit profile is provided, default rules/artifacts/receipt/stop-condition assertions, and a runonce test that reads the written prompt artifact and checks the default profile section.
- Verification run: `gofmt -w internal/prompt/profile.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/runonce/runonce_test.go`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add TUI blocked-task retry for the selected task.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Tasks view now supports `u` retry for the selected blocked task. The action calls the app-wired retry callback, refreshes the status snapshot after success, keeps the retried task selected, and reports inline notices for successful retry, non-blocked selections, missing callbacks, retry callback errors, and refresh failures. The Tasks footer shows `u Retry` only when the selected task is blocked and retry/refresh callbacks are available, while Help documents the retry key.
- CLI wiring changed: `revolvr tui` now passes `internal/app.RetryTask` into the TUI action callbacks.
- Tests added: focused TUI model coverage for successful blocked-task retry, pending/completed non-blocked rejection without mutation, missing retry/refresh callbacks, retry callback error, and refresh failure after retry. CLI TUI wiring coverage now verifies the retry callback can return a blocked task to pending through the command setup.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/tui ./internal/cli ./internal/app`; `go test ./...`; `go run ./cmd/revolvr tui --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the Dashboard and Tasks view now show pending, blocked, and completed counts alongside explicit runnable state text. When a pending task exists, both views show `Runnable: ready to run` and the next task ID plus summary/task text; otherwise they show `Runnable: nothing runnable` and `Next task: none`. The Tasks list marks the current selection with `>` and the first pending task with a plain `next` marker, so selection and run readiness are visible without depending on color.
- Tests added: focused `internal/tui` render coverage for pending, blocked-only, completed-only, and empty queues, plus updated dashboard/task snapshots for uninitialized and narrow/wide rendering.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add TUI blocked-task retry for the selected task.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: document the chat/spec-to-task workflow and import format.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Documentation added: README now documents a chat-to-task import workflow for shaping specs in web chat, saving a Markdown task file, dry-running and importing it, opening or refreshing the TUI, running TUI preflight, and starting one bounded TUI pass. It includes a minimal import-file example with summary, acceptance, and verification notes, plus the caution that chat, CLI, and TUI can share task state while concurrent code edits against the same repository should be avoided.
- Verification run: `go test ./...`; `go run ./cmd/revolvr task import --help`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add `revolvr task import <path>` with `--dry-run`.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added a `revolvr task import <path>` command with `--dry-run`. The CLI reads the Markdown file, calls the app-level import operation, prints numbered dry-run rows without creating `.revolvr/`, and prints created task IDs in parsed order for write imports. Existing `task add` and `task list` output remains unchanged.
- Tests added: focused CLI tests for import help, dry-run/no-mutation output, successful ordered import and ID output, parse errors without mutation, and unreadable paths without mutation.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestTaskImport|TestTask(Add|List)'`; `go test ./internal/cli -run 'TestNewRootCommandConstructsExpectedCommands|TestParentCommandHelpOutput'`; `go test ./...`; `go run ./cmd/revolvr task import --help`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to document the chat/spec-to-task workflow and import format.
- Blockers: none.

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
