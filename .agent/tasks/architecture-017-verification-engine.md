---
id: architecture-017-verification-engine
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-016-tool-broker-implementer
---

# Implement the host-owned verification engine

## Sequence and status

- Sequence: `017` of `025`.
- Status: pending.
- Prerequisite: `architecture-016-tool-broker-implementer`.
- Phase gate: Phase 5 verification runs only after a candidate source snapshot
  is frozen and the implementer sandbox can no longer alter it.

## Primary outcome

Execute the admitted verification plan in a fresh verifier sandbox and persist
source/environment-bound results that models cannot redefine or self-certify.

## Required reading

- ADR-024.
- Specification Sections 9.12, 12.4, 17.7, 18,
  29 Phase 5, 37.8, 39.1, 40.8, 44.4, 52-55,
  58, and 60.

## Existing foundations to inspect

- Pinned task acceptance/verification plan, workspace source/diff identity,
  sandbox runtime, tool command evidence, artifacts, and events.
- Existing `internal/verification`, `internal/autonomousverification`, and
  `internal/runonce` for useful command/result behavior only; adapt it to the
  canonical PostgreSQL and fresh-sandbox boundaries.
- Project environment data captured by project registration; add only the
  minimum contract fields required by this task.

## Starting assumptions

- Verification authority is accepted before implementation and versioned with
  the run.
- Tier 0 baseline may be absent only when policy explicitly permits it; final
  clean verification is never the implementer process.
- A timeout, missing check, stale source, or environment mismatch is not pass.

## Implementation requirements

- Add reversible verification-run/check schema, named queries, status/result
  types, and atomic result/event persistence.
- Pin the ordered Tier 0-4 plan, command argv, environment names, working
  directory, source commit/tree, image digest, sandbox profile, and plan hash.
- Run baseline when configured, then focused/project/risk checks as admitted,
  and a fresh Tier 4 check against the exact candidate source.
- Capture bounded raw stdout/stderr artifacts and parse structured Go test JSON,
  JUnit/JSON, or equivalent only when configured; raw evidence remains
  authoritative.
- Compare baseline/candidate into new, resolved, unchanged, and flaky outcomes;
  preserve every occurrence and never convert a later pass into erasure.
- Detect changes to commands, scripts, CI, fixtures/goldens, generated
  validators, environment contract, and other verification authority; apply
  reject/dual-run/escalate policy from task authority.

## Scope boundaries and non-goals

- Do not let the implementer select final checks or mark criteria/task complete.
- Do not run an auditor, correct code, waive failures, or hide pre-existing
  failures.
- Do not hard-code language-specific scanners globally; project configuration
  owns optional risk checks.

## Acceptance criteria

- Results bind exact source, task/run/plan, image/profile, command, timestamps,
  exit/timeout, parsed data, and raw artifacts.
- Fresh verifier isolation prevents reliance on implementer-local state.
- Baseline differential fixtures classify new/resolved/unchanged/flaky
  outcomes without false pass.
- Tampered tests/scripts/config/goldens, stale source/environment, missing
  commands, timeout, cancellation, malformed structured output, and artifact
  failure stop or escalate exactly as policy states.
- Transaction failure leaves no partial accepted verification and retry does
  not erase prior occurrences.
- PostgreSQL, sandbox fixture, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/verification ./internal/sandbox
go test ./...
git diff --check
```

## Expected completion report

Report schema/package changes, tier plan and pinned identities, fresh-sandbox
proof, differential/tamper/timeout/malformed/rollback coverage, artifact
evidence, and all test results.
