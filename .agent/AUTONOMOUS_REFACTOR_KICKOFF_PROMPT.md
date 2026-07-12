# Fresh Autonomous Workflow Refactor Kickoff

You are starting the autonomous-workflow refactor in a brand-new Codex session.
Do not rely on prior chat history or resume an older session. Repository files
are the durable source of truth.

Suggested invocation from the repository root:

```bash
codex exec --model gpt-5.6-sol \
  -c 'model_reasoning_effort="xhigh"' \
  --dangerously-bypass-approvals-and-sandbox \
  - < .agent/AUTONOMOUS_REFACTOR_KICKOFF_PROMPT.md
```

## Read First

Read these files completely before editing:

- `AGENTS.md`
- `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md`
- `.agent/TASKS.md`
- `.agent/STATE.md`
- `.agent/DECISIONS.md`
- `README.md`

Then inspect only the code needed to understand current task workflow,
pass-policy, receipt, ledger, prompt/context, and run-once boundaries. Start
with:

- `internal/passpolicy`
- `internal/taskfile`
- `internal/runonce`
- `internal/prompt`
- `internal/receipt`
- `internal/ledger`

## Objective

Do exactly one bounded task in this session: define and validate the structured
contracts for supervisor decisions and audit findings in the future
`autonomous-v1` workflow.

This is the foundational contract slice from step 1 of
`.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md`. Do not wire the supervisor into
`runonce`, replace `mixed-pass-v1`, add routing behavior, change CLI/TUI output,
or implement later roadmap steps in this session.

## Required Design Properties

Use the smallest package and API that fit the existing architecture. Preserve
current behavior. Add no dependencies.

The contracts must be deterministic, JSON-serializable, and explicitly
validated. They should support at least these concepts without relying on
free-form prose for control flow:

- Supervisor actions: plan, implement, audit, correct, document, simplify,
  complete, and block.
- The selected worker profile, where an action requires one.
- A concise rationale and explicit success criteria for the next action.
- Evidence/input references that identify the durable facts used to decide.
- Audit disposition: clean or changes required.
- Stable audit finding identity, blocking significance, summary, concrete
  evidence, and required correction.
- References from a correction decision to the findings it must resolve.

Validation must reject, at minimum:

- Unknown actions or dispositions.
- Missing task identity or rationale.
- Worker actions without a compatible profile.
- Terminal decisions that incorrectly select a worker profile.
- `changes_required` audits without findings.
- Duplicate or malformed finding IDs.
- Findings without actionable evidence or required correction.
- Correction decisions without referenced findings.

Keep policy separate from model output: model-authored JSON proposes a decision;
Go validation decides whether Revolvr may act on it. Prefer clear enums, small
types, useful error messages, and table-driven tests over an abstract workflow
framework.

## Compatibility Boundary

- Existing `mixed-pass-v1` runtime behavior and tests must remain unchanged.
- Existing receipts and ledger events must continue to parse.
- Do not reinterpret task frontmatter `profile`.
- Do not add runtime fallbacks for malformed supervisor output.
- Do not commit unless the user explicitly asks.

## Verification

Format changed Go files with `gofmt` and run:

```bash
go test ./<new-or-changed-focused-package>
go test ./...
git diff --check
```

Add focused tests covering every supported action/disposition, valid worker and
terminal decisions, finding validation, correction references, duplicate IDs,
JSON round trips, and clear invalid-input errors.

## Durable State

The first unchecked task in `.agent/TASKS.md` is this contract slice. Mark it
complete only after verification passes. Update `.agent/STATE.md` with the task,
files changed, behavior added, verification commands/results, remaining work,
and blockers. Update `.agent/DECISIONS.md` only for decisions actually embodied
in the implementation.

If implementation reveals necessary follow-up slices, add only small,
specific, verifiable tasks that directly follow from this contract. Keep the
nine-step roadmap in `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md` as the broader
direction rather than attempting it all at once.

## Stop Condition

Stop after the contract package and its tests are complete. Do not begin the
task-dossier or supervisor-runtime work. End with:

- Task selected
- Files changed
- Verification run
- Result
- Suggested next task
