---
id: architecture-017-verification-engine
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-016a-programmatic-compatibility-seams
---

# Implement the host-owned verification engine

## Sequence and status

- Sequence: `017` of `025`.
- Status: completed.
- Prerequisite: `architecture-016a-programmatic-compatibility-seams`.
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
- The superseded
  `REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` Sections 2.2
  and 14 as subordinate fingerprint/reuse requirements.

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
- Compute one canonical exact execution fingerprint for each admitted gate.
  At minimum it covers verifier protocol, implementation, and configured parser
  versions; project identity; task version; verification-plan version and
  hash; candidate source tree; ordered argv and working directories; declared
  environment names and nonsecret values; verifier/worker image digest;
  sandbox profile hash; project environment contract; every admitted
  verification-authority file; scripts, fixtures, and goldens; and the exact
  output policy.
- Look up only terminal results by exact fingerprint before execution. When
  policy permits reuse, create a new occurrence linked to the original
  execution; never rewrite, erase, or retimestamp the original result.
- Make reuse outcomes explicit. In particular, an exact reused failure is
  `unchanged_failure_reused` and remains a failure. Cancelled, incomplete,
  infrastructure-failed, or ambiguous results are not reusable unless a later
  closed policy explicitly admits that exact typed outcome.
- Record original execution time and reuse occurrence time separately; a
  reused result is never described as freshly executed.
- Invalidate reuse after any material fingerprint input changes. No partial,
  heuristic, source-only, or prose-based cache match is authority.
- Add an explicit policy option requiring fresh final verification even when
  an equal reusable terminal result exists. Initial completion-purpose Tier 4
  policy remains fresh and source-bound.
- Run baseline when configured, then focused/project/risk checks as admitted,
  and apply the explicit freshness policy to the Tier 4 check against the exact
  candidate source.
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
- Equal complete inputs produce the same fingerprint; database lookup, cache
  selection, original-result linkage, reuse occurrence, and both timestamps
  are deterministic and exact.
- Fresh verifier isolation prevents reliance on implementer-local state.
- Baseline differential fixtures classify new/resolved/unchanged/flaky
  outcomes without false pass.
- Tampered tests/scripts/config/goldens, stale source/environment, missing
  commands, timeout, cancellation, malformed structured output, and artifact
  failure stop or escalate exactly as policy states.
- Transaction failure leaves no partial accepted verification and retry does
  not erase prior occurrences.
- Database, cache, invalidation, reuse, and final-freshness fixtures prove exact
  hits and misses, a new occurrence per reuse, unchanged failure preservation,
  exclusion of cancelled/incomplete/infrastructure/ambiguous results, and
  forced execution when final-freshness policy is enabled.
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

Report schema/package changes, tier plan and pinned identities, fingerprint
components and canonicalization, exact lookup/reuse/invalidation/freshness
behavior, fresh-sandbox proof, differential/tamper/timeout/malformed/rollback
coverage, artifact evidence, and all test results.
