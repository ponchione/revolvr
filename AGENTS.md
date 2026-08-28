# AGENTS.md

## Repository Working Rules

- Work in small, reviewable changes.
- Do exactly one task per autonomous loop run.
- Do not commit unless explicitly asked.
- Do not add dependencies without explaining why.
- Preserve existing style, naming, and architecture unless the task says otherwise.
- Prefer fixing root causes over adding defensive hacks.
- Prefer simple code over clever abstractions.

## Project Shape

- Stack: Go 1.27.0 CLI application.
- Entry point: `cmd/revolvr/main.go`.
- Main packages: `internal/cli`, `internal/runonce`, `internal/ledger`, `internal/taskfile`, `internal/taskmodel`, `internal/codexexec`, `internal/receipt`, `internal/verification`, `internal/commit`, `internal/gitstate`, `internal/runner`.
- Runtime state: `.revolvr/` after `revolvr init`; this directory is local and ignored by Git.

## Verification

Run the relevant checks before finishing a task.

For Go code changes:

- Format changed Go files with `gofmt -w <files>`.
- Run `go test ./...`.

For CLI behavior changes, also run focused commands when practical, such as:

- `go run ./cmd/revolvr --help`
- `go run ./cmd/revolvr config check`
- `go run ./cmd/revolvr status`

There is no separate lint or typecheck command configured. `go test ./...` is the source of truth for compilation and tests.

## Durable Agent State

Use these files as durable memory between fresh Codex sessions:

- `.agent/HANDOFF.md` for the active pause/resume point and exact next command
- `.agent/TASKS.md` for the task backlog
- `.agent/STATE.md` for current status and recent progress
- `.agent/DECISIONS.md` for durable implementation and architecture decisions
- `.agent/LOOP_PROMPT.md` for the reusable one-pass loop prompt

The repository state is the memory. Do not depend on old Codex conversation history.

## Fresh-Session Loop Rule

Every autonomous pass must be a new `codex exec` invocation.

Do not use:

- `codex resume`
- `codex exec resume`
- old session transcripts as required context

Each pass must read the durable state files, execute one bounded decision,
proof, or implementation task, update the durable state files, and stop.

After that task is terminally complete and verified, its completion handoff may
publish exactly one accepted, dependency-satisfied next draft as pending. That
publication is metadata for the completed pass, never a standalone pass. The
next fresh pass implements the pending task instead of republishing it.

Recovery only: if the canonical selector is empty and `.agent/HANDOFF.md`
names one exact accepted, dependency-satisfied draft, publish it and continue
directly into that task in the same pass. If the draft is ambiguous, blocked,
or needs an unresolved product decision, record the blocker and publish
nothing. The canonical selector and dependency checks remain authoritative.
Never bulk-publish drafts or execute more than one product task.
