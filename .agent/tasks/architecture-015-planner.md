---
id: architecture-015-planner
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-014-supervisor
---

# Implement versioned planning

## Sequence and status

- Sequence: `015` of `025`.
- Status: pending.
- Prerequisite: `architecture-014-supervisor`.
- Phase gate: planning occurs only for a pinned task/run after an accepted
  supervisor `plan` decision and host policy admission.

## Primary outcome

Turn one admitted planning decision into a validated, immutable plan revision
with ordered steps, acceptance mapping, tests, risks, assumptions, and
evidence provenance.

## Required reading

- ADR-023.
- Specification Sections 9.4, 9.10, 12.2, 13.2, 18.7,
  29 Phase 4, 37.2, 39.1, 44, and 49.

## Existing foundations to inspect

- Task contracts/criteria from `internal/taskintake`, lifecycle state, accepted
  supervisor decisions, the OpenAI client, and artifact/event persistence.
- Existing `internal/autonomousplanning` and `internal/autonomousplanapply`
  for reusable closed-schema and monotonic-step tests only.
- Repository/project maps available from the managed source; semantic retrieval
  is not available until task 021.

## Starting assumptions

- The accepted task contract, not the planner, owns scope and acceptance.
- Plans are immutable revisions; one accepted plan points to ordered mutable
  step state.
- Exact referenced files and repository search can supply initial context
  before vector retrieval exists.

## Implementation requirements

- Add the minimal reversible `plans`, `plan_versions`, and `plan_steps` schema,
  named queries, aggregate constraints, and append-only plan events.
- Define a closed planner output schema with plan identity/revision, ordered
  stable step IDs, task-criterion mapping, expected paths/components, test
  strategy, risks, assumptions, and evidence references.
- Build and hash the Section 13.2 dossier; invoke a fresh tool-free planner and
  retain exact prompt/output provenance.
- Reject missing/duplicate/reordered identities, empty or unbounded steps,
  unmapped criteria, unsupported verification, unresolved placeholders, scope
  expansion, stale source/task/decision, and invented dependencies.
- Persist candidate/accepted revision and events atomically under optimistic
  concurrency; explain material revisions and prevent completed steps from
  silently returning to pending.

## Scope boundaries and non-goals

- Do not implement source mutation, tool calls, task recompilation, acceptance
  changes, semantic retrieval, or model-controlled plan acceptance.
- Do not split or create new canonical tasks; task compilation owns that.
- Do not add speculative steps for later architecture outside this task.

## Acceptance criteria

- A valid plan maps every task criterion and produces stable ordered plan rows
  with exact task/source/decision provenance.
- Malformed/refused output, duplicate steps, missing criteria, stale identity,
  scope expansion, unsupported tests, and placeholder text fail without an
  accepted revision.
- Concurrent acceptance has one winner; forced event failure rolls back the
  plan pointer/rows and retry is idempotent.
- A revision cannot regress completed steps without explicit valid lineage.
- Migration/sqlc, fake-model, PostgreSQL integration, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
env -u OPENAI_API_KEY REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/planner
go test ./...
git diff --check
```

## Expected completion report

Report schema/package changes, plan schema and dossier identity, criterion
mapping, revision/monotonicity rules, malformed/stale/concurrency/rollback
coverage, and all verification results.
