# Local Codex Fresh-Session Agent Loop Handoff

## Purpose

You are my local Codex agent operating inside the current repository. Your job is to set up a safe, repeatable agent-loop workflow that preserves the benefits of fresh session context resets.

The core idea is simple:

- Each autonomous work pass must be a brand new Codex session.
- Do not resume prior Codex sessions.
- Do not rely on chat history as durable memory.
- Store durable state inside repository files.
- Do exactly one bounded task per future loop pass.
- Verify the work, update state, and stop.

This setup is meant to avoid long, context-heavy sessions getting weird after hundreds of thousands of tokens. The repo becomes the durable state machine. Codex sessions remain disposable.

## Non-negotiable rules

Follow these rules during this setup run:

1. Do not use `codex resume`.
2. Do not create a workflow that depends on resumed sessions.
3. Do not invoke Codex recursively from inside this setup run.
4. Do not run `./agent-one.sh` during this setup run.
5. Do not commit changes unless I explicitly ask you to.
6. Do not overwrite existing project guidance without preserving it.
7. Keep all generated files small, readable, and easy to edit.
8. Prefer safe defaults over clever automation.

Future loop passes should be launched by me manually with:

```bash
./agent-one.sh
```

Each invocation of `./agent-one.sh` should start a fresh `codex exec` run.

## Desired repository structure

Create or update the repository so it contains:

```text
AGENTS.md
.agent/
  TASKS.md
  STATE.md
  DECISIONS.md
  LOOP_PROMPT.md
  SETUP_NOTES.md
agent-one.sh
agent-loop.sh      # optional, bounded loop with human confirmation
```

If the repository already has any of these files, preserve useful existing content and merge carefully.

## Setup steps to execute now

### 1. Locate the repository root

Find the Git root if this is a Git repository. Work from that root.

Use this behavior:

- If `git rev-parse --show-toplevel` succeeds, use that path.
- If it fails, use the current working directory.
- Do not leave the current project/workspace boundary.

### 2. Inspect the project

Briefly inspect the repository to determine:

- Primary language or stack.
- Build command.
- Test command.
- Lint or typecheck command.
- Important directories.
- Any existing architecture or contributor guidance.

Look for files such as:

```text
package.json
go.mod
*.sln
*.csproj
Cargo.toml
pyproject.toml
requirements.txt
Makefile
README.md
CONTRIBUTING.md
```

Do not spend too long on this. The goal is to produce practical starter instructions, not a perfect architecture review.

### 3. Create `.agent/`

Create the directory:

```bash
mkdir -p .agent
```

### 4. Create or update `AGENTS.md`

If `AGENTS.md` does not exist, create it using the template below.

If `AGENTS.md` already exists, do not overwrite it. Append a clearly marked section named `Agent Loop Addendum` unless the existing content already covers the same rules.

Use this content as the baseline:

```md
# AGENTS.md

## Repository working rules

- Work in small, reviewable changes.
- Do exactly one task per autonomous loop run.
- Do not commit unless explicitly asked.
- Do not add dependencies without explaining why.
- Preserve existing style, naming, and architecture unless the task says otherwise.
- Prefer fixing root causes over adding defensive hacks.
- Prefer simple code over clever abstractions.

## Verification

Run the relevant checks before finishing a task.

Use the project-specific commands discovered from this repository when possible.

Common examples:

- Go: `gofmt` on changed files, then `go test ./...`
- .NET: `dotnet build`, then `dotnet test` if tests exist
- Node/TypeScript: `npm run build`, `npm test`, or `npm run typecheck` if available
- Python: run the existing test command, commonly `pytest`, if configured

If no verification command is obvious, explain what was checked manually and record the gap in `.agent/STATE.md`.

## Durable agent state

Use these files as durable memory between fresh Codex sessions:

- `.agent/TASKS.md` for the task queue
- `.agent/STATE.md` for current status and recent progress
- `.agent/DECISIONS.md` for durable implementation and architecture decisions
- `.agent/LOOP_PROMPT.md` for the reusable one-pass loop prompt

The repository state is the memory. Do not depend on old Codex conversation history.

## Fresh-session loop rule

Every autonomous pass must be a new `codex exec` invocation.

Do not use:

- `codex resume`
- `codex exec resume`
- old session transcripts as required context

Each pass must read the durable state files, do one bounded task, update the durable state files, and stop.
```

After creating or updating `AGENTS.md`, adjust the verification section with actual commands discovered from this repository.

### 5. Create `.agent/TASKS.md`

Create `.agent/TASKS.md` if it does not exist.

If you can identify a few high-confidence small tasks from the repo, add 3 to 7 of them. Good examples include failing tests, obvious TODOs, missing README commands, small type errors, small lint fixes, or missing coverage for a nearby behavior.

If you cannot identify good tasks quickly, use placeholder tasks and tell me to edit them.

Template:

```md
# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, write the blocker under `Blocked` and stop.
- Add new tasks only when they are directly discovered while working on the current task.
- New tasks must be specific, small, and verifiable.
- Do not invent broad roadmap items.

## Backlog

- [ ] Replace this with the first small, verifiable task.
- [ ] Replace this with the second small, verifiable task.

## Blocked

None.
```

### 6. Create `.agent/STATE.md`

Create `.agent/STATE.md` if it does not exist.

Template:

```md
# Agent State

## Current focus

None.

## Last run

No previous autonomous loop run.

## Current repository understanding

- Stack: TBD
- Build command: TBD
- Test command: TBD
- Lint/typecheck command: TBD
- Important directories: TBD

## Verification gaps

None recorded yet.

## Notes for next fresh session

- Read `AGENTS.md` first.
- Read `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md` before making changes.
- Do one task, verify, update state, and stop.
```

Fill in the `Current repository understanding` section based on your quick inspection.

### 7. Create `.agent/DECISIONS.md`

Create `.agent/DECISIONS.md` if it does not exist.

Template:

```md
# Agent Decisions

- The repo itself is the durable state for autonomous loop runs.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
```

Add only meaningful decisions. Do not turn this into a verbose scratchpad.

### 8. Create `.agent/LOOP_PROMPT.md`

Create `.agent/LOOP_PROMPT.md` with this content:

```md
# Fresh Codex Loop Pass

You are running one fresh autonomous pass on this repository.

This pass must be treated as a new session. Do not rely on prior chat context. The durable state is stored in repository files.

## Read first

Read these files before making changes:

- `AGENTS.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`

## Task selection

1. Pick the first unchecked, unblocked task from `.agent/TASKS.md`.
2. If there are no tasks, add a short note to `.agent/STATE.md` and stop.
3. Do not work on more than one task.

## Work rules

1. Restate the selected task briefly.
2. Inspect only the files needed for the task.
3. Make the smallest reasonable change.
4. Preserve existing style and architecture.
5. Do not add dependencies unless the task clearly requires it.
6. Do not commit.
7. Do not use `codex resume`.
8. Do not start a nested Codex run.

## Verification

1. Run the relevant verification commands from `AGENTS.md`.
2. If verification passes, mark the task complete in `.agent/TASKS.md`.
3. If verification fails, make one reasonable repair attempt.
4. If verification still fails, record the blocker in `.agent/STATE.md` and stop.
5. If no automated verification is available, record what manual check was performed and note the verification gap.

## State updates

Before stopping, update `.agent/STATE.md` with:

- Task selected
- Files changed
- Verification commands run
- Verification result
- What remains
- Any blockers

Update `.agent/DECISIONS.md` only if a durable implementation or architecture decision was made.

If you discover follow-up work directly related to the task, add it to the bottom of `.agent/TASKS.md` as a small, specific, verifiable task.

## Stop condition

Stop after one task. Do not continue to the next backlog item.

## Final response format

End with:

- Task selected
- Files changed
- Verification run
- Result
- Suggested next task
```

### 9. Create `agent-one.sh`

Create `agent-one.sh` at the repository root.

Use this script:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [ ! -f ".agent/LOOP_PROMPT.md" ]; then
  echo "Missing .agent/LOOP_PROMPT.md"
  exit 1
fi

echo "Starting one fresh Codex loop pass in: $ROOT"
echo

codex exec \
  --sandbox workspace-write \
  --ask-for-approval never \
  --cd "$ROOT" \
  "$(cat .agent/LOOP_PROMPT.md)"

echo
echo "Codex pass complete. Changed files:"
git status --short 2>/dev/null || true
```

Make it executable:

```bash
chmod +x agent-one.sh
```

This script intentionally starts a fresh `codex exec` run every time.

### 10. Optionally create `agent-loop.sh`

Create this only if it seems useful. It should run a bounded loop with human confirmation between fresh passes.

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

MAX_RUNS="${1:-3}"

for i in $(seq 1 "$MAX_RUNS"); do
  echo "===== Fresh Codex pass $i / $MAX_RUNS ====="
  ./agent-one.sh

  echo
  echo "Review current changes before continuing."
  git status --short 2>/dev/null || true
  echo

  read -r -p "Run another fresh pass? [y/N] " answer
  case "$answer" in
    y|Y|yes|YES) continue ;;
    *) break ;;
  esac
done
```

Make it executable if created:

```bash
chmod +x agent-loop.sh
```

### 11. Create `.agent/SETUP_NOTES.md`

Create a short setup note explaining what you found and what you created.

Template:

```md
# Agent Loop Setup Notes

## Setup summary

Created the fresh-session Codex loop framework.

## Files created or updated

- `AGENTS.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`
- `.agent/LOOP_PROMPT.md`
- `agent-one.sh`
- `agent-loop.sh` if created

## Repository observations

- Stack: TBD
- Build command: TBD
- Test command: TBD
- Lint/typecheck command: TBD

## How to run

From the repository root:

```bash
./agent-one.sh
```

For a bounded manual loop:

```bash
./agent-loop.sh 3
```

## Important reminder

This workflow preserves context refresh by using a brand new `codex exec` run each time. Do not use `codex resume` for this workflow.
```

Fill in the repository observations.

## Final response for this setup run

After you finish creating and updating the files, stop and report:

- Files created
- Files modified
- Project stack detected
- Verification commands detected
- Any uncertainties
- The exact command I should run next

The exact next command should usually be:

```bash
./agent-one.sh
```

Do not run that command yourself during this setup run.
