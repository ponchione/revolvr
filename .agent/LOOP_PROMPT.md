# Fresh Codex Loop Pass

You are running one fresh autonomous pass on this repository.

This pass must be treated as a new session. Do not rely on prior chat context. The durable state is stored in repository files.

## Read First

Read these files before making changes:

- `AGENTS.md`
- `.agent/HANDOFF.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`

## Task Selection

1. Run the exact read-only selector command in `.agent/HANDOFF.md` and select
   the first dependency-satisfied pending task in the active architecture
   sequence. Do not treat historical/deferred checklist entries as selectors.
2. Recovery only: if the selector is empty and `.agent/HANDOFF.md` names one
   exact accepted, dependency-satisfied draft, publish that draft and continue
   directly into it in this same pass.
3. If there is no exact recovery draft, record the empty selector or blocker in
   `.agent/STATE.md`, publish nothing, and stop.
4. Do not execute more than one decision, proof, or implementation task.

## Work Rules

1. Restate the selected task briefly.
2. Inspect only the files needed for the task.
3. Make the smallest reasonable change.
4. Preserve existing style and architecture.
5. Do not add dependencies unless the task clearly requires it.
6. Do not commit.
7. Do not use `codex resume`.
8. Do not start a nested Codex run.
9. Respect explicit supersession: do not select terminal `superseded` task
   files or revive a retired roadmap as follow-up work.

## Verification

1. Run the relevant verification commands from `AGENTS.md`.
2. If verification passes, mark the canonical task complete and update its
   concise completion evidence and `.agent/TASKS.md`.
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

## Completion Handoff

After the selected task is terminally complete and verified, publish exactly
one accepted, dependency-satisfied next draft as pending when the plan names an
unambiguous next task. This is completion metadata for the task just finished,
not a second product task or a standalone publication pass. Do not create a
`*_PUBLICATION_PROMPT.md` file.

If the next draft is ambiguous, blocked, or requires an unresolved product
decision, record that blocker and publish nothing. Never bulk-publish the draft
backlog or start the newly pending task in this pass.

## Stop Condition

Stop after one product task and its normal completion handoff. The next fresh
pass implements the newly pending task; it does not republish it.

## Final Response Format

End with:

- Task selected
- Files changed
- Verification run
- Result
- Suggested next task
