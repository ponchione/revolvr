# Codex Harness Targets

## Purpose

This repository will house a custom local harness for running bounded Codex
agent loops. The harness should make `codex exec` easier to use repeatedly
without depending on resumed Codex sessions or long-lived chat history.

The core product is not a replacement agent runtime. Codex remains the only
agent executor. This project owns the control plane around Codex:

- selecting one bounded task
- constructing the prompt
- launching a fresh `codex exec`
- capturing Codex JSONL/events/output
- validating the resulting work
- recording a receipt and run ledger
- committing verified changes automatically

## Operating Principles

- Use only local `codex exec` for agent execution.
- Do not use `codex resume` for loop passes.
- Treat each pass as disposable; durable state lives in repo-owned files or a
  local harness ledger.
- Run one selected task per pass.
- The harness, not Codex prose alone, decides whether verification passed.
- Auto-commit only after harness-controlled verification passes.
- Do not push branches or create pull requests in v1.
- Prefer a small Go CLI first; defer TUI and richer UI until the run ledger is
  useful.

## Proposed V1 Shape

Command name is open, but this document uses `revolvr` as the placeholder.

```text
revolvr init
revolvr task add "..."
revolvr task list
revolvr run --once
revolvr status
revolvr show <run-id>
```

Minimal pass flow:

```text
select first runnable task
create run/pass row
build prompt
run codex exec --json
capture JSONL and final response
capture git status
ensure or synthesize receipt
run harness verification commands
if verification passes: git add and git commit
if verification fails: do not commit; record blocker
update task and run state
exit
```

## Recommended Stack

Use Go for the first implementation.

Reasons:

- The harness is a local CLI with subprocess control, git integration, signal
  handling, and eventual TUI needs.
- Sodoryard already has reusable Go utilities for subprocesses, receipts,
  path guarding, run state, and event logging.
- Go keeps the first binary easy to run in bare repos without a Node install.

Initial dependencies should stay small:

- `github.com/spf13/cobra` for CLI command wiring, if command growth justifies it.
- `gopkg.in/yaml.v3` for receipt frontmatter.
- SQLite can be deferred until the first ledger slice if JSONL state is chosen
  for the bootstrap.

## What To Pilfer From Sodoryard

Reference repo: `/home/gernsback/source/sodoryard`.

### 1. Subprocess Runner

Source:

- `/home/gernsback/source/sodoryard/internal/spawn/subprocess.go`
- `/home/gernsback/source/sodoryard/internal/outputcap/buffer.go`

Target:

- `internal/runner`
- `internal/outputcap`

Use for:

- running `codex exec --json`
- running verification commands
- running git commands
- timeout and cancellation handling
- line-level event streaming
- bounded stdout/stderr capture

Adaptations:

- Rename `RunCommand` types away from Sodoryard naming.
- Add JSONL line callback support for Codex stdout.
- Preserve stderr line capture because `codex exec` streams progress there.
- Keep the capped buffer behavior to avoid unbounded logs.

Acceptance criteria:

- Can run a command successfully and capture stdout/stderr.
- Can report non-zero exit codes.
- Can terminate long-running commands on timeout.
- Can emit stdout/stderr lines as events.
- Captured output is capped and reports truncated bytes.

### 2. Receipt Contract

Source:

- `/home/gernsback/source/sodoryard/internal/receipt/types.go`
- `/home/gernsback/source/sodoryard/internal/receipt/parser.go`
- `/home/gernsback/source/sodoryard/internal/receipt/findings.go`
- `/home/gernsback/source/sodoryard/internal/headless/headless.go`

Target:

- `internal/receipt`

Use for:

- a human-readable and machine-parseable record of each Codex pass
- deciding whether a pass was completed, blocked, failed, or needs follow-up
- comparing claimed changed files and validation against harness facts

New schema name:

```text
revolvr.receipt.v1
```

Suggested receipt fields:

```yaml
schema_version: revolvr.receipt.v1
run_id: <id>
pass_id: <id>
task_id: <id>
task: <short task text>
verdict: completed
timestamp: 2026-06-25T00:00:00Z
codex_exit_code: 0
verification_status: passed
commit_sha: <sha or empty>
changed_files: []
verification:
  - command: go test ./...
    exit_code: 0
    status: passed
metrics:
  input_tokens: 0
  output_tokens: 0
  duration_seconds: 0
```

Suggested verdicts:

- `completed`
- `completed_with_concerns`
- `blocked`
- `verification_failed`
- `codex_failed`
- `safety_limit`
- `no_changes`

Required body sections:

- `## Summary`
- `## Changed Files`
- `## Verification`
- `## Concerns`
- `## Next Steps`

Acceptance criteria:

- Parse valid receipts with YAML frontmatter.
- Reject missing required fields.
- Reject unknown verdicts.
- Validate required body sections.
- Synthesize a fallback receipt when Codex does not produce one.
- Rewrite receipt metrics after parsing Codex JSONL usage events.

### 3. Run Ledger

Source:

- `/home/gernsback/source/sodoryard/internal/chain/state.go`
- `/home/gernsback/source/sodoryard/internal/chain/events.go`
- `/home/gernsback/source/sodoryard/internal/db/schema.sql`

Target:

- `internal/ledger`

Use for:

- run history
- task/pass state
- event timeline
- future TUI/status screens

Recommended v1 tables if using SQLite:

```sql
CREATE TABLE runs (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    task TEXT NOT NULL,
    status TEXT NOT NULL,
    summary TEXT,
    started_at TEXT NOT NULL,
    completed_at TEXT,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    codex_exit_code INTEGER,
    verification_status TEXT,
    commit_sha TEXT
);

CREATE TABLE events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id TEXT NOT NULL REFERENCES runs(id),
    event_type TEXT NOT NULL,
    event_data TEXT,
    created_at TEXT NOT NULL
);
```

Defer extra tables until needed. A separate `passes` table is useful only if a
single run can contain multiple Codex passes. V1 should not.

Initial event types:

- `run_started`
- `task_selected`
- `prompt_built`
- `codex_started`
- `codex_json_event`
- `codex_completed`
- `changed_files_captured`
- `receipt_parsed`
- `receipt_synthesized`
- `verification_started`
- `verification_completed`
- `commit_started`
- `commit_created`
- `run_completed`
- `run_failed`

Acceptance criteria:

- Can create a run.
- Can append JSON event payloads.
- Can list recent runs.
- Can show one run with event history.
- Can survive process restart and still report prior run state.

### 4. Changed File Capture

Source:

- `/home/gernsback/source/sodoryard/internal/spawn/spawn_agent.go`
  - `captureChangedFiles`
  - `parseGitStatusChangedFiles`

Target:

- `internal/gitstate`

Use for:

- independent change detection after Codex exits
- comparing actual changed files with receipt claims
- commit staging

Acceptance criteria:

- Runs `git status --short --untracked-files=all`.
- Parses modified, added, deleted, renamed, and quoted paths.
- Sorts and deduplicates paths.
- Records capture errors without crashing unrelated cleanup.
- Provides a clean list of paths for receipts and commits.

### 5. Auto-Commit Gate

Source inspiration:

- Sodoryard's post-step guardrail idea in
  `/home/gernsback/source/sodoryard/internal/spawn/spawn_agent.go`

Target:

- `internal/commit`

Rules:

- Never commit before harness-controlled verification passes.
- Never commit if Codex exits non-zero unless explicitly configured later.
- Never commit if verification commands are missing, unless the user config
  allows manual/no-op verification.
- Never commit unrelated pre-existing dirty files unless the run config allows
  adopting them.
- Commit message should include the task summary and run id.

Suggested commit message:

```text
<task summary>

Run-ID: <run-id>
Task-ID: <task-id>
Verification: passed
```

Acceptance criteria:

- Detect dirty worktree before run starts.
- Record pre-existing dirty files.
- After run, stage only files changed by this run where possible.
- Run configured verification commands after Codex exits.
- Commit only when verification passes.
- Store resulting commit SHA in receipt and ledger.

### 6. Path Guard

Source:

- `/home/gernsback/source/sodoryard/internal/pathguard/pathguard.go`

Target:

- `internal/pathguard`

Use for:

- task file paths
- receipt paths
- artifact paths
- config-relative paths

Acceptance criteria:

- Reject empty paths.
- Reject absolute paths where relative paths are required.
- Reject paths escaping the project root.
- Handle symlink escape attempts by resolving nearest existing path.

### 7. ID Generation

Source:

- `/home/gernsback/source/sodoryard/internal/id/id.go`

Target:

- `internal/id`

Use for:

- run ids
- task ids
- event correlation ids

Acceptance criteria:

- Generate time-sortable IDs.
- Avoid third-party dependency for UUIDs unless needed.
- Provide deterministic test hooks where useful.

### 8. Source Writer Lock

Source:

- `/home/gernsback/source/sodoryard/internal/chain/project_locks.go`

Target:

- `internal/lock`

Use for:

- preventing two harness processes from running mutating Codex passes against
  the same repo at the same time

V1 approach:

- Use a lock file under `.revolvr/locks/source-writer.lock`.
- Store run id, pid, acquired timestamp, heartbeat timestamp, and expiry.
- Refresh heartbeat while `codex exec` is running.
- Allow stale lock replacement after timeout.

Acceptance criteria:

- A second `revolvr run --once` refuses to start while the lock is live.
- A stale lock can be replaced safely.
- Lock is released on normal completion.
- Lock release is attempted on failure and cancellation.

## What Not To Pilfer

Do not copy these Sodoryard systems into v1:

- LLM provider router
- direct Codex provider/API integration
- custom agent loop
- tool registry and tool execution engine
- Shunter project memory
- RAG/code intelligence and LanceDB integration
- web UI and Electron app
- multi-agent roles and orchestrator prompts
- approval system

Those features solve a different problem. `revolvr` should remain a thin,
opinionated control plane around local `codex exec`.

## Bootstrap Task Plan

Each item below should be small enough to drive one future implementation
prompt.

### Task 1: Initialize Go CLI Skeleton

Goal:

- Create a buildable Go module with a minimal `revolvr` CLI.

Suggested prompt:

```text
Initialize this bare repo as a small Go CLI project named revolvr. Add a root
command with version output and placeholder subcommands for init, task, run,
status, and show. Keep the implementation minimal and add tests for command
construction. Do not implement Codex execution yet.
```

Acceptance criteria:

- `go test ./...` passes.
- `go run ./cmd/revolvr --help` works.
- No Codex subprocess is invoked.

### Task 2: Port Subprocess Runner

Goal:

- Add the reusable command runner and capped output buffer.

Suggested prompt:

```text
Port the subprocess runner and capped output buffer patterns from
/home/gernsback/source/sodoryard into this repo. Adapt naming to revolvr.
Add tests for success, non-zero exit, timeout, line callbacks, and output
truncation. Do not add Codex-specific behavior yet.
```

Acceptance criteria:

- Unit tests cover timeout and line flushing.
- Runner returns exit code and captured output.
- No unbounded output capture.

### Task 3: Add Path Guard And IDs

Goal:

- Add foundational path safety and run id generation.

Suggested prompt:

```text
Add internal pathguard and id packages based on the small Sodoryard utilities.
Keep them dependency-light and covered by tests. The path guard must reject
absolute paths and root escapes, including symlink-based escapes.
```

Acceptance criteria:

- Path guard tests cover empty, absolute, `..`, normal, and symlink cases.
- IDs are non-empty and sort by generation time in normal cases.

### Task 4: Add Receipt Package

Goal:

- Define and validate `revolvr.receipt.v1`.

Suggested prompt:

```text
Create an internal receipt package for revolvr.receipt.v1 using the Sodoryard
receipt parser as a starting point. Include typed verdicts, YAML frontmatter
parsing, required body section validation, changed file parsing, verification
command parsing, and fallback receipt formatting. Add focused tests.
```

Acceptance criteria:

- Valid receipts parse.
- Invalid verdicts fail.
- Missing required sections fail.
- Fallback receipt includes summary, changed files, verification, concerns, and
  next steps sections.

### Task 5: Add Run Ledger

Goal:

- Persist run and event history.

Suggested prompt:

```text
Implement a minimal local run ledger for revolvr. Use SQLite unless there is a
strong reason to start with JSONL. Add runs and events storage, recent-run list,
single-run lookup, and append-only event logging. Keep schema small and add
tests around persistence across reopen.
```

Acceptance criteria:

- Can create a run.
- Can append events.
- Can list recent runs.
- Can reopen the ledger and read previous runs.

### Task 6: Add Task Queue

Goal:

- Store and select tasks.

Suggested prompt:

```text
Add a simple task queue for revolvr. It should support adding tasks, listing
tasks, selecting the first pending task for run --once, marking completed, and
recording blockers. Keep task storage compatible with the ledger design.
```

Acceptance criteria:

- Add/list/select/complete/block behavior is tested.
- Selection is deterministic.
- The first pending unblocked task is selected.

### Task 7: Build Prompt Generator

Goal:

- Generate the one-pass Codex prompt.

Suggested prompt:

```text
Create a prompt builder for one fresh Codex pass. It should include the selected
task, repository rules, receipt path, required receipt schema, stop condition,
and instructions not to use codex resume or nested Codex runs. Add snapshot
tests for the generated prompt.
```

Acceptance criteria:

- Prompt includes task id, run id, receipt path, and stop condition.
- Prompt requires one bounded task.
- Prompt asks Codex to write/update the receipt.

### Task 8: Implement Codex Exec Runner

Goal:

- Run `codex exec --json` and capture events.

Suggested prompt:

```text
Implement a Codex runner that invokes local codex exec with --json, explicit
sandbox and approval flags from config, and a generated prompt. Parse stdout
as JSONL events, store raw JSONL to a run artifact, and append summarized
events to the ledger. Do not implement verification or commit yet.
```

Acceptance criteria:

- Uses fresh `codex exec`, not resume.
- Captures stdout JSONL and stderr progress.
- Records thread/turn/item/error events when present.
- Returns exit code, final message if detectable, usage if present, and paths to
  artifacts.

### Task 9: Capture Changed Files

Goal:

- Independently record git changes after Codex exits.

Suggested prompt:

```text
Add git state capture for revolvr runs. Before the Codex pass, record the dirty
worktree. After the pass, record changed files with git status --short
--untracked-files=all. Parse modified, added, deleted, renamed, and quoted
paths. Add tests for the parser.
```

Acceptance criteria:

- Pre-existing dirty files are recorded.
- Post-run changed files are recorded.
- Parser handles rename and quoted paths.

### Task 10: Add Verification Runner

Goal:

- Run harness-controlled checks after Codex exits.

Suggested prompt:

```text
Add verification command support. Load commands from config, run them after
Codex exits, capture exit code/stdout/stderr with capped buffers, write events
to the ledger, and return an aggregate passed/failed status. Do not commit yet.
```

Acceptance criteria:

- Multiple commands run in order.
- First failing command marks verification failed.
- Output is capped and persisted as artifacts or event payloads.
- Missing commands are treated according to explicit config.

### Task 11: Add Auto-Commit Gate

Goal:

- Commit verified work.

Suggested prompt:

```text
Implement the auto-commit gate. After a Codex pass and harness verification,
stage only files changed by the run where possible, create a git commit with a
message containing task id and run id, and store the commit SHA in the ledger
and receipt. Do not push or create PRs.
```

Acceptance criteria:

- No commit is created when verification fails.
- No commit is created when there are no changes.
- Commit SHA is recorded when commit succeeds.
- Commit message includes run id and task id.

### Task 12: Wire `run --once`

Goal:

- Connect task selection, prompt generation, Codex execution, receipt handling,
  verification, and commit.

Suggested prompt:

```text
Wire the revolvr run --once command end to end. It should select the first
pending task, create a run, acquire the source-writer lock, build the prompt,
run codex exec --json, capture changed files, ensure a receipt, run
verification, auto-commit on success, update task status, release the lock, and
print a concise summary.
```

Acceptance criteria:

- End-to-end behavior is covered with fake Codex and fake git command runners.
- The real command does not invoke Codex in tests.
- Lock is released on success and failure.
- Failed verification leaves task blocked or pending according to config.

## Deferred Targets

These are useful but not v1 prerequisites:

- Minimal TUI for run history and live event tailing.
- Configurable prompt templates.
- Rich receipt diff rendering.
- Human approval gates.
- Multi-pass bounded loops with confirmation between passes.
- Cloud or PR integrations.
- Codex SDK or app-server integration.

## Open Design Questions

1. State format: SQLite ledger from the start, or JSONL first with SQLite later?
2. Task format: editable markdown tasks, CLI-managed tasks, or both?
3. Verification policy: require commands always, or allow explicit manual/no-op
   verification for docs-only repos?
4. Dirty worktree policy: refuse to start dirty by default, or allow adopting
   pre-existing changes with an explicit flag?
5. Sandbox policy defaults: `workspace-write` with approval `never`, or a more
   conservative default?

## Suggested First Implementation Prompt

Use this after reviewing and editing the plan:

```text
Read CODEX_HARNESS_TARGETS.md. Implement Task 1 only: initialize this bare repo
as a small Go CLI project named revolvr with placeholder commands. Keep changes
minimal, add command construction tests, run go test ./..., and stop. Do not
implement Codex execution yet.
```
