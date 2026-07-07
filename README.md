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
state. The directory is local runtime state and is ignored by Git.

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
recorded without pushing branches.

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
