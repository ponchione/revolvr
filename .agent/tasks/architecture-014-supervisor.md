---
id: architecture-014-supervisor
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-013-openai-structured-output-client
---

# Implement the decision-only supervisor

## Sequence and status

- Sequence: `014` of `025`.
- Status: pending.
- Prerequisite: `architecture-013-openai-structured-output-client`.
- Phase gate: supervisor output is not admitted until fresh model invocations,
  structured-output validation, and lifecycle authority already fail closed.

## Primary outcome

Produce, validate, persist, and policy-route exactly one advisory supervisor
decision for the pinned task/run state without tools or direct mutation.

## Required reading

- ADR-009, ADR-023, and ADR-024.
- Specification Sections 3.1, 9.8-9.9, 12, 13.1, 15,
  18.7, 29 Phase 4, 37.4, 37.6, 39.1, 45, and 46.

## Existing foundations to inspect

- The OpenAI client/model registry from task 013.
- `internal/tasklifecycle`, scheduler-pinned run/task/source identities,
  artifact/event persistence, and budget state.
- Existing `internal/supervisor`, `internal/autonomouspolicy`,
  `internal/autonomouscycle`, and `.agent/profiles/supervisor.md` for reusable
  schemas/tests only; do not carry filesystem authority into PostgreSQL.

## Starting assumptions

- The host assembles a frozen supervisor dossier; the model cannot retrieve or
  mutate source during this role.
- Valid actions are `plan`, `implement`, `correct`, `document`, `simplify`,
  `complete`, `block`, and `needs_input`.
- Verification and finalization remain host operations.

## Implementation requirements

- Define a versioned closed decision schema binding task/version, run, source,
  dossier, prompt, model, policy, and decision hashes.
- Build the bounded Section 13.1 dossier from canonical lifecycle, plan,
  criteria, latest verification/audit, findings, attempts/strategies, budget,
  and high-authority decisions; explicitly record omissions.
- Invoke one fresh, tool-free supervisor call and require exactly one final
  structured decision.
- Validate identity freshness, action-specific required/forbidden fields,
  lifecycle legality, task scope, budget, and policy before accepting it.
- Persist accepted and rejected decisions with full provenance; only trusted
  host code may request the subsequent lifecycle transition.
- Route `complete` only as a proposal to completion preflight, and persist
  typed `block`/`needs_input` data without answering or broadening it.

## Scope boundaries and non-goals

- Do not implement planning, source tools, worker execution, verification,
  audit, correction, or final completion in this task.
- Do not let the supervisor run tools, select another task, update PostgreSQL,
  answer its own question, or relax policy.
- Do not carry model conversation history between decisions.

## Acceptance criteria

- One valid fixture for each action is parsed and routed only when lifecycle
  and identity permit it.
- Multiple actions, unknown fields/actions, malformed/refused output, stale
  task/source/dossier, illegal lifecycle action, exhausted budget, and scope
  broadening are rejected and persisted without state mutation.
- A `complete` proposal cannot bypass missing verification/audit/evidence.
- Fake-model tests require no network and the complete Go suite passes.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
env -u OPENAI_API_KEY go test ./internal/supervisor ./internal/policy
go test ./...
git diff --check
```

## Expected completion report

Report schema/actions, dossier inputs and hash, accepted/rejected persistence,
host-policy routing, stale/malformed/refusal/illegal-action coverage, proof of
no tools/direct mutation, changed files, and test results.
