---
id: architecture-019-auditor-corrector
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-018-evidence-model-completion-gates
---

# Implement the auditor and bounded corrector loop

## Sequence and status

- Sequence: `019` of `025`.
- Status: pending.
- Prerequisite: `architecture-018-evidence-model-completion-gates`.
- Phase gate: Phase 6 starts only after deterministic verification and
  completion preflight can reject missing or stale audit evidence.

## Primary outcome

Produce independent evidence-backed audit findings and allow bounded,
finding-scoped correction followed by fresh verification and re-audit.

## Required reading

- ADR-013, ADR-023, and ADR-024.
- Specification Sections 9.13-9.15, 12.5-12.6, 13.4-13.5,
  18.7, 19, 29 Phase 6, 37.9-37.10, 39.3, and 58.5.

## Existing foundations to inspect

- Model client, role prompts, supervisor policy, implementer tool broker,
  workspace snapshots, verification evidence, completion preflight, artifacts,
  and events.
- Existing `internal/autonomousaudit`, `internal/autonomousauditapply`,
  `internal/autonomouscorrection`, and related state/history tests for reusable
  schema and retry behavior only.

## Starting assumptions

- The auditor is a fresh role distinct from the implementer and receives no
  source-mutation tools.
- A correction is authorized by exact open findings or one exact verification
  failure, not by a broad redesign request.
- Every source change makes prior audit evidence stale.

## Implementation requirements

- Add reversible audit-run, finding, disposition, failure-signature, strategy,
  and strategy-outcome schema/queries with immutable occurrence provenance.
- Build/hash the Section 13.4 audit dossier from task/plan/acceptance, exact
  diff/changed symbols, verification, blast radius, and prior findings.
- Validate one closed auditor result: `clean`, `changes_required`, or `blocked`;
  every finding has stable ID, significance, required correction, exact source
  evidence, affected files/symbols, and criterion impact.
- Route optional security/performance/integration/migration/documentation/API
  audits deterministically from task risk and actual changes, not model choice.
- Build the corrector dossier only from exact active failure/findings, current
  source, relevant tests, and prior strategies; execute through the same
  sandbox/tool protections as implementation.
- Normalize failure signatures and strategy fingerprints, reject materially
  repeated failed strategies, enforce budgets/no-progress, then require full
  fresh verification and re-audit of every corrected source revision.
- Persist finding dispositions only with exact resolution/waiver/rejection/
  supersession/staleness authority and evidence.

## Scope boundaries and non-goals

- Do not let an implementer audit itself, let a model waive findings, or accept
  uncited narrative findings.
- Do not correct unrelated code, bypass verification, auto-answer ambiguity,
  or retry indefinitely.
- Do not run every specialist audit for every change.

## Acceptance criteria

- Clean, changes-required, and blocked fixtures route deterministically and
  bind exact task/source/dossier/model evidence.
- Malformed/refused/stale audit output, uncited findings, unknown significance,
  duplicate IDs, and changed source fail without accepted audit authority.
- A correction fixture changes only cited scope, re-verifies, re-audits clean,
  and then satisfies the completion audit gate.
- Repeated strategy, identical diff/failure, no changes/evidence, budget limit,
  cancellation, and correction failure stop with typed outcomes.
- Finding transitions, concurrent disposition, transaction rollback/retry,
  fake-model, sandbox, PostgreSQL, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
env -u OPENAI_API_KEY REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/audit ./internal/correction
go test ./...
git diff --check
```

## Expected completion report

Report schema/role/package changes, audit and finding identities, specialist
routing, correction/reverify/re-audit evidence, malformed/stale/repeat/
no-progress/rollback coverage, and complete test results.
