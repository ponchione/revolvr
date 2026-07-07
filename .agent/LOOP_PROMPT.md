# Fresh Codex Loop Pass

You are running one fresh autonomous pass on this repository.

This pass must be treated as a new session. Do not rely on prior chat context. The durable state is stored in repository files.

## Read First

Read these files before making changes:

- `AGENTS.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`

## Task Selection

1. Pick the first unchecked, unblocked task from `.agent/TASKS.md`.
2. If there are no tasks, add a short note to `.agent/STATE.md` and stop.
3. Do not work on more than one task.

## Work Rules

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

## State Updates

Before stopping, update `.agent/STATE.md` with:

- Task selected
- Files changed
- Verification commands run
- Verification result
- What remains
- Any blockers

Update `.agent/DECISIONS.md` only if a durable implementation or architecture decision was made.

If you discover follow-up work directly related to the task, add it to the bottom of `.agent/TASKS.md` as a small, specific, verifiable task.

## Stop Condition

Stop after one task. Do not continue to the next backlog item.

## Final Response Format

End with:

- Task selected
- Files changed
- Verification run
- Result
- Suggested next task
