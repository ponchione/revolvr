---
id: architecture-022-deterministic-eval-suite
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-021-code-indexing-context-assembly
---

# Build the deterministic architecture evaluation suite

## Sequence and status

- Sequence: `022` of `025`.
- Status: pending.
- Prerequisite: `architecture-021-code-indexing-context-assembly`.
- Phase gate: queue autonomy and Graphiti consideration remain disabled until
  deterministic core-loop and retrieval failure scenarios are executable and
  baseline results are recorded.

## Primary outcome

Create source-of-truth fixture repositories and deterministic fakes that
exercise the complete bounded control loop, safety boundary, recovery, and
retrieval behavior without live models or network services.

## Required reading

- Specification Sections 9.25, 19, 21.3, 22, 23,
  29 Phase 10 entry concerns, 30, 31, 37-39, 50, 56, 58, and 60.

## Existing foundations to inspect

- Every canonical package produced by tasks 004-021 and its focused tests.
- Existing `internal/autonomousmetrics/evaluation_test.go`, application
  production-path fake tests, and smoke scripts for reusable deterministic
  clocks/model/process patterns only.
- `evals/` should be created just in time; do not duplicate package unit tests
  without an end-to-end scenario need.

## Starting assumptions

- Deterministic fixtures use fixed UTC clocks, stable IDs, local temporary Git
  repositories, fake OpenAI/embedding/runtime endpoints, and controlled
  PostgreSQL state.
- Live OpenAI dogfood is separate, explicit, and never the source of truth for
  this suite.
- Quality thresholds are recorded from baseline evidence rather than invented
  before measurement.

## Implementation requirements

- Add fixture repositories, scenario inputs, expected canonical events/state,
  and golden evidence under `evals/` with one documented runner/test entrypoint.
- Cover all Section 23.1 scenarios: straight success; compile/test correction;
  audit correction; ambiguity; missing/cyclic dependency; scope/protected-path
  violation; repeated strategy; no changes; test tampering; mid-run source
  change; cancellation; crashes during state/external effects; stale index;
  missing embeddings; sandbox timeout; and network-denied dependency install.
- Assert task/run/plan/criterion/finding/workspace/sandbox/verification/audit/
  completion state, event order, artifacts, hashes, stop reason, lease cleanup,
  and original-checkout identity for each relevant scenario.
- Include retrieval quality fixtures and deterministic context-manifest checks;
  record baseline metrics and omissions without estimating model tokens/cost.
- Make crash injection cover each external-effect boundary from Section 56 and
  prove exact replay is idempotent while divergent evidence fails closed.
- Document the explicit separate live-dogfood command and its required recorded
  identities, but do not run it in ordinary tests.

## Scope boundaries and non-goals

- Do not call live OpenAI, download models/dependencies during the suite, add a
  daemon, start a queue, or loosen production policy for test convenience.
- Do not treat goldens as verification authority that an implementer may
  rewrite silently; changes require an explained reviewable diff.
- Do not claim quality gates pass until measured baseline evidence supports the
  recorded threshold.

## Acceptance criteria

- One deterministic command executes all 20 required fixture scenarios and
  produces byte-stable canonical results across repeated runs.
- Every false-completion, unsafe host access, transaction rollback, crash
  recovery, and degraded retrieval case stops with the expected typed outcome.
- No scenario requires a live model, public network, ambient credential, or
  operator home data.
- Retrieval and core-loop baseline metrics are stored with exact fixture and
  implementation identities and are suitable for the queue/Graphiti phase
  gates.
- The complete Go suite and `git diff --check` pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/evaluation -count=2
go test ./...
git diff --check
```

## Expected completion report

Report fixture/scenario files, all 20 scenario outcomes, repeated-run identity,
crash/replay and host-safety coverage, retrieval/core baseline metrics,
explicit live-test omission, and full test results.
