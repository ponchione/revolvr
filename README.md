# revolvr

`revolvr` is a local Go CLI for running bounded Codex harness passes from
repo-owned Markdown tasks under `.agent/tasks/*.md`. It keeps local runtime
state under `.revolvr/`, launches fresh Codex executions for work passes,
verifies the resulting work, records run history, and commits verified changes.

## Setup

Requirements:

- Go 1.22 or newer
- Git
- Codex CLI available as `codex`

Build or run from the repository root:

```bash
go build ./cmd/revolvr
go run ./cmd/revolvr --help
```

Initialize local runtime state:

```bash
go run ./cmd/revolvr init
```

This creates `.revolvr/` with ledger, run artifact, receipt, and lock state,
and ensures `.agent/tasks/` exists for canonical Markdown tasks. `.revolvr/` is
local runtime state and is ignored by Git. When initialized from a Git worktree,
`init` adds `/.revolvr/` to
`.git/info/exclude` so tracked ignore files do not need to change.

## Tasks

Add work for the harness:

```bash
go run ./cmd/revolvr task add --summary "README docs" "Add concise setup and usage docs"
```

List task files:

```bash
go run ./cmd/revolvr task list
```

If a task is blocked and should be retried:

```bash
go run ./cmd/revolvr task retry <task-id>
```

### Mixed-Pass Task Workflow

A durable task is one canonical Markdown file under `.agent/tasks/*.md`. It
keeps its identity and workflow state as it progresses. A pass (or run) is one
fresh Codex execution for that task, with its own run ID, ledger events,
artifacts, and receipt. One durable task therefore normally spans several
passes.

Minimal canonical task file:

```markdown
---
id: example-task
status: pending
workflow: mixed-pass-v1
phase: implement
---
# Example task

Describe the bounded work here.
```

Omitting `workflow` or `phase` defaults them to `mixed-pass-v1` and
`implement`. The workflow progresses in this exact order:

```text
implement -> audit -> document -> simplify -> completed
```

Revolvr policy selects the repo-authored profile for each phase:

- `implement` uses `implementer` to make the bounded change. It requires
  meaningful changes before the metadata transition; a verified no-change
  implementation pass is refused with `no_changes` and does not advance.
- `audit` uses `auditor` to review correctness, regressions, verification, and
  risks. It may succeed without product or source changes.
- `document` uses `documentor` to update operator-facing documentation when
  needed. It may succeed without product or source changes.
- `simplify` uses `simplifier` to reduce worthwhile complexity or duplication.
  It may succeed without product or source changes and completes the task.

Task frontmatter `profile` is not an operator override. Revolvr derives the
selected profile from `workflow` and `phase` through its pass policy.

Revolvr, not Codex-authored task edits, owns durable phase transitions. After
Codex and verification succeed, Revolvr applies the policy transition to the
task file before the commit gate. Nonterminal transitions keep the task
`pending` at the next phase; a successful `simplify` pass marks it `completed`.
Audit, document, and simplify may advance without source changes because the
task-file metadata transition is durable work included in the commit. A failed
or blocked outcome leaves the original phase unchanged and blocks the task;
`revolvr task retry <task-id>` returns it to `pending` at that same phase.

Receipts, ledger events, and committed task-file transitions make every phase
outcome auditable. Receipt text records the outcome but does not choose the
next phase; Revolvr applies policy to the harness outcome.

Use `revolvr task list` for workflow, phase, profile, and next-state columns,
and `revolvr status` for the next runnable task, its next pass, and recent pass
state. The TUI Dashboard and Tasks views show the current workflow state. Use
`revolvr show <run-id>` or TUI Run Detail to inspect task-selection timeline
metadata and the underlying event sequence.

## Chat-To-Task Imports

Use a web chat or design session to shape a feature into small, bounded tasks,
then save the result as a Markdown import file in the repository. The import
format is intentionally simple: each task starts with a `## Task` heading, has a
required task body, and may include `### Summary`, `### Acceptance`, and
`### Verification` sections. Summary becomes the task summary; acceptance and
verification notes stay in the task text so the Codex pass sees them.

Minimal import file:

```markdown
# Next Revolvr Tasks

## Task
Document the task import workflow in the README.

### Summary
README task import workflow

### Acceptance
- README explains dry-run, import, refresh, preflight, and one-pass TUI use.

### Verification
- go test ./...
- go run ./cmd/revolvr task import --help
```

Save the file somewhere local to the repo, for example
`.agent/imports/next-tasks.md`, then preview it without mutating `.revolvr/`:

```bash
go run ./cmd/revolvr task import --dry-run .agent/imports/next-tasks.md
```

Import the tasks when the dry-run output looks right:

```bash
go run ./cmd/revolvr task import .agent/imports/next-tasks.md
```

Open the TUI:

```bash
go run ./cmd/revolvr tui
```

If the TUI is already open, press `r` to refresh the shared task state. To run a
single pass from the TUI, press `5` to open Preflight, press `p` to run the
readiness check, then press `R` once preflight is ready. To run a bounded
multi-pass loop from the TUI, press `n` to choose 2, 3, or 5 max passes
(default 3), then press `L`; the progress pane shows pass summaries as the loop
runs. The closest CLI equivalents are:

```bash
go run ./cmd/revolvr doctor
go run ./cmd/revolvr run --once
go run ./cmd/revolvr run --max-passes 3
```

Chat sessions, the CLI, and the TUI all read and write the same
`.agent/tasks/*.md` task files, so it is fine to design tasks in chat and
refresh the TUI after importing them. Avoid concurrent code edits against the
same repository while a TUI-started or CLI-started Codex pass is running; keep
one actor responsible for worktree changes at a time.

## Configuration

Configuration is optional and lives at `.revolvr/config.yaml`. Inspect the
effective configuration with:

```bash
go run ./cmd/revolvr config check
```

Example:

```yaml
codex:
  executable: codex
  dangerously_bypass_approvals_and_sandbox: true
  timeout_seconds: 1800
git:
  executable: git
  timeout_seconds: 30
verification:
  missing_policy: fail
  commands:
    - name: go
      args: ["test", "./..."]
      timeout_seconds: 300
commit:
  allow_pre_existing_dirty: false
  allow_missing_verification: false
  timeout_seconds: 30
```

For Go repositories, the effective default verification command is
`go test ./...` when no verification commands are configured. CLI-initiated
runs default to Codex dangerous bypass/yolo mode for unattended local harness
passes; set `codex.dangerously_bypass_approvals_and_sandbox: false` or
`codex.yolo: false` to disable that default.

## Dogfooding

Use this short flow when exercising Revolvr against this repository:

```bash
go run ./cmd/revolvr doctor
go run ./cmd/revolvr task add --summary "README docs" "Add concise setup and usage docs"
go run ./cmd/revolvr run --once
go run ./cmd/revolvr status
go run ./cmd/revolvr show <run-id>
```

`doctor` reports initialized state, configured Codex and Git executables, Git
identity, clean worktree state, `.revolvr/` ignore state, and effective
verification coverage. It exits nonzero when a required check fails. Use
`status` to find recent run IDs for `show`.

Note: Real dogfood runs should start from a clean worktree and use `status` and
`show <run-id>` to inspect the recorded result.

For a fuller live check against real Codex, run:

```bash
./scripts/dogfood-live.sh
```

This script is intentionally destructive to local Revolvr runtime state: it
requires a clean source worktree, removes `.revolvr/`, initializes fresh state,
creates one tiny file-update task, runs `revolvr run --once`, and verifies the
receipt, ledger-backed `status`/`show` output, commit SHA, `receipt validate`,
and final clean worktree. It creates one Git commit when the live run passes.

## TUI

Open the interactive terminal UI from the repository root:

```bash
go run ./cmd/revolvr tui
```

The TUI shows the same app-backed state as the CLI: task counts, task details,
recent runs, run diagnostics, artifacts, receipt validation results, preflight
readiness checks, and a live progress pane while a TUI-started run or loop is
active. Use number keys to switch views, `j`/`k` or arrow keys to move list
selections, `a` to add a task, `p` in Preflight to check readiness, `R` to run
one pass after preflight is ready, `n` to cycle loop max passes through 2/3/5
(default 3), `L` to start the bounded run loop, `c` to request cancellation of
an active run or loop, `v` in Run Detail to validate the loaded receipt, `r` to
refresh, and `?` for in-app key help. Loop runs show pass summaries in the
progress pane.

Current limitations: the TUI starts at the current terminal size and wraps
content for narrow terminals, but it is still a local terminal view over the
same runtime and `.agent/tasks/*.md` task state. It can add tasks, retry blocked
tasks, run one pass, run a bounded multi-pass loop, cancel an active TUI-started
pass or loop, refresh state, open run details, and validate receipts; use the
CLI for configuration changes, the non-TUI `run --max-passes` loop alternative,
and receipt validation outside the loaded run detail.

## Run

Run one selected pending task:

```bash
go run ./cmd/revolvr run --once
```

Run up to a bounded number of passes:

```bash
go run ./cmd/revolvr run --max-passes 3
```

A pass selects a runnable task, writes `.revolvr/runs/<run-id>/context.md` plus
its `.revolvr/runs/<run-id>/context.json` manifest, runs Codex with that context
payload, captures artifacts, runs verification commands, records a receipt and
ledger events, and commits the verified result. Failed Codex, verification,
commit, or safety outcomes are recorded without pushing branches. While Codex
runs, `revolvr run` streams concise progress messages to stdout; the full Codex
JSONL and stderr streams remain captured as run artifacts. `run --max-passes`
prints a final loop summary and stops early when failed or blocked passes need
inspection.

## Status, Show, And Receipt Validation

Show aggregate task and run state:

```bash
go run ./cmd/revolvr status
```

Inspect one recorded run:

```bash
go run ./cmd/revolvr show <run-id>
```

`show` prints the run summary, timestamps, Codex and verification diagnostics,
commit SHA when present, artifact paths, and event timeline.

Validate one recorded run receipt against the ledger and artifact files:

```bash
go run ./cmd/revolvr receipt validate <run-id>
```

Receipt validation checks the finalized verdict, timestamp, commit SHA,
changed files, verification results, and recorded artifact paths.

## Development Checks

Use the repository checks before changing code:

```bash
gofmt -w <changed-go-files>
go test ./...
go run ./cmd/revolvr --help
go run ./cmd/revolvr config check
go run ./cmd/revolvr status
```

Run the local CLI smoke test without invoking Codex:

```bash
./scripts/smoke-local.sh
```

The smoke test builds a temporary binary and exercises `init`, `task add`,
`task list`, `config check`, and `status` in a temporary workspace.

Run the `run --once` integration smoke test without invoking real Codex:

```bash
./scripts/smoke-run-once-fake-codex.sh
```

This smoke test builds a temporary binary, initializes a temporary Git repo,
points `codex.executable` at a strict fake Codex executable, verifies a
deterministic generated file, checks the completed task/run state, and confirms
the committed receipt and run artifacts.

Run the matching verification-failure smoke test without invoking real Codex:

```bash
./scripts/smoke-run-once-fake-codex-verification-failure.sh
```

This smoke test uses a strict fake Codex executable that makes a deterministic
file change, then intentionally fails local verification. It checks that the run
fails cleanly, the task is blocked, no commit is created, and run diagnostics
and artifacts are still recorded.

Run the opt-in live dogfood check with real Codex:

```bash
./scripts/dogfood-live.sh
```

This resets `.revolvr/`, creates a tiny task, runs one real Codex pass, and
checks the finalized receipt, ledger output, commit, receipt validation command,
and clean-worktree consistency.
