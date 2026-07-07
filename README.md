# revolvr

`revolvr` is a local Go CLI for running bounded Codex harness passes from a
repo-owned task queue. It keeps durable state under `.revolvr/`, launches fresh
Codex executions for work passes, verifies the resulting work, records run
history, and commits verified changes.

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

This creates `.revolvr/` with task, ledger, run artifact, receipt, and lock
state. The directory is local runtime state and is ignored by Git. When
initialized from a Git worktree, `init` adds `/.revolvr/` to
`.git/info/exclude` so tracked ignore files do not need to change.

## Task Queue

Add work for the harness:

```bash
go run ./cmd/revolvr task add --summary "README docs" "Add concise setup and usage docs"
```

List queued tasks:

```bash
go run ./cmd/revolvr task list
```

If a task is blocked and should be retried:

```bash
go run ./cmd/revolvr task unblock <task-id>
```

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

## Run

Run one selected pending task:

```bash
go run ./cmd/revolvr run --once
```

Run up to a bounded number of passes:

```bash
go run ./cmd/revolvr run --max-passes 3
```

A pass selects a runnable task, writes a prompt, runs Codex, captures artifacts,
runs verification commands, records a receipt and ledger events, and commits the
verified result. Failed Codex, verification, commit, or safety outcomes are
recorded without pushing branches. While Codex runs, `revolvr run` streams
concise progress messages to stdout; the full Codex JSONL and stderr streams
remain captured as run artifacts.

## Status And Show

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
