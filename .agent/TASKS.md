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

- [ ] Add a multi-view TUI shell with explicit Dashboard, Tasks, Runs, Run Detail, and Help/keys areas.
  Scope: replace the current single-window feel with a view model and predictable navigation keys. Keep the TUI read-only for this task.
  Acceptance: users can switch views without losing loaded state; the footer shows available keys for the active view; empty/uninitialized state still renders coherently.
  Verification: add focused `internal/tui` tests for view switching, help/footer rendering, and resize behavior; run `go test ./internal/tui`, `go test ./internal/cli -run 'TestTUI'`, and `go test ./...`.

- [ ] Add a dedicated TUI Tasks view with selection and task detail rendering.
  Scope: show pending, blocked, and completed tasks in a scannable list or table; allow moving the selection; render details for the selected task including ID, status, summary, task text, blocker, and timestamps when present.
  Acceptance: task details are visible without leaving the TUI; blocked tasks are visually distinguishable; there is no task mutation yet.
  Verification: add focused tests for populated, empty, pending, blocked, and completed task states; run `go test ./internal/tui` and `go test ./...`.

- [ ] Add a TUI task creation flow backed by `internal/app.AddTask`.
  Scope: add an `a` action that opens a task-entry mode with required task text and optional summary, supports submit/cancel, persists the task through `internal/app`, and refreshes the status snapshot after success.
  Acceptance: empty task text is rejected with an inline message; cancel returns to the previous view without writes; successful add selects or surfaces the new pending task.
  Verification: add model tests using stubbed app callbacks for submit, validation, cancel, and refresh; add CLI wiring coverage if command setup changes; run focused tests plus `go test ./...`.

- [ ] Add a dedicated TUI Runs view and richer Run Detail view.
  Scope: list recent runs with status, verification, commit, and summary; allow selecting a run and opening a detail view backed by `internal/app.ShowRun`; show summary, diagnostics, artifacts, changed files, and events in separate sections or tabs.
  Acceptance: run details are useful without falling back to `revolvr show`; long event lists remain scrollable; missing artifacts or warnings are visible.
  Verification: add focused tests for recent-run navigation, detail opening, diagnostics rendering, artifact rendering, and long event output; run `go test ./internal/tui` and `go test ./...`.

- [ ] Add receipt validation status to the TUI Run Detail view.
  Scope: use `internal/app.ValidateReceipt` from the selected run detail, either automatically on open or through a `v` action, and render each validation check result with clear pass/fail messaging.
  Acceptance: a fully valid receipt shows all checks passing; validation failures are visible in the detail view; validation errors do not crash the TUI.
  Verification: add model tests for valid receipts, failed validation checks, missing receipts, and validation callback errors; run focused tests plus `go test ./...`.

- [ ] Move doctor/preflight orchestration behind `internal/app` and add a TUI Preflight view.
  Scope: expose the existing doctor checks as structured app data, keep CLI doctor output unchanged, and render readiness checks in the TUI.
  Acceptance: users can inspect readiness before running Codex; failed checks show enough detail to act on; CLI `doctor` remains byte-for-byte compatible where tests assert output.
  Verification: add `internal/app` tests for preflight snapshots and CLI tests for preserved doctor output; add TUI tests for ready and failed preflight views; run focused tests plus `go test ./...`.

- [ ] Add a nonblocking TUI run-once action with live progress and cancellation.
  Scope: add a guarded run action that starts `internal/app.RunOnce` from a Bubble Tea command, streams Codex progress into a progress/log pane, disables conflicting actions while running, and supports cancellation.
  Acceptance: no run starts when preflight is not ready or another run is active; progress remains visible; completion refreshes status and run details; cancellation reports a clear terminal state.
  Verification: use fake app runners in TUI tests for success, failure, progress events, and cancellation; run `go test ./internal/tui`, `go test ./internal/app`, `go test ./internal/cli`, and `go test ./...`.

- [ ] Polish TUI layout, styling, and documentation for daily use.
  Scope: refine responsive layout, colors, key help, empty states, and README usage notes for `revolvr tui`.
  Acceptance: text does not overlap at narrow terminal widths; important states are readable without color; README explains the TUI capabilities and current limitations.
  Verification: add or update snapshot-style render tests for narrow and wide widths; run `go test ./internal/tui`, `go test ./...`, and `go run ./cmd/revolvr tui --help`.

## Completed History

The previous harness, app-boundary, and initial TUI tasks were completed on 2026-07-08. Details are preserved in `.agent/STATE.md` and the Git history.

## Blocked

None.
