# Agent Loop Setup Notes

## Setup Summary

Created the fresh-session Codex loop framework and initialized local Revolvr runtime state.

## Files Created Or Updated

- `AGENTS.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`
- `.agent/LOOP_PROMPT.md`
- `.agent/SETUP_NOTES.md`
- `agent-one.sh`
- `agent-loop.sh`

## Repository Observations

- Stack: Go 1.22 CLI application.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test ./...`.
- Lint/typecheck command: none configured; use `gofmt -w <changed go files>` and `go test ./...`.
- Runtime initialization command: `go run ./cmd/revolvr init`.
- Local runtime state path: `.revolvr/`.

## How To Run

From the repository root:

```bash
./agent-one.sh
```

For a bounded manual loop:

```bash
./agent-loop.sh 3
```

## Important Reminder

This workflow preserves context refresh by using a brand new `codex exec` run each time. Do not use `codex resume` for this workflow.
