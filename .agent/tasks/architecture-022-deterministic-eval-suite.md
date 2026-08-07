---
id: architecture-022-deterministic-eval-suite
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-021-code-indexing-context-assembly
---

# Build the deterministic architecture evaluation suite

## Sequence and status

- Sequence: `022` of `025`.
- Status: completed.
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
- `REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` Sections 2.2
  and 21 only as subordinate execution-mode/metrics compatibility guidance.

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
- Add a closed `worker_execution_mode` dimension. `direct_tools_v1` is the only
  implemented and admitted initial mode. Reserve
  `programmatic_workspace_v1` as a future evaluated value, but reject it before
  scenario execution until its runtime is implemented and explicitly
  admitted; never synthesize a success or substitute direct-tool behavior.
- Put mode selection behind a scenario-runner boundary that can apply the same
  immutable task, acceptance, policy, source, and expected-outcome authority to
  either mode once that mode is implemented.
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
- For every scenario occurrence record worker mode, context bytes, available
  input/output/reasoning/cached token counts with explicit omissions when the
  source does not report them, direct-tool count, repeated-read count,
  verification executions and exact reuses, correction cycles, wall time, and
  final typed outcome.
- Make crash injection cover each external-effect boundary from Section 56 and
  prove exact replay is idempotent while divergent evidence fails closed.
- Document the explicit separate live-dogfood command and its required recorded
  identities, but do not run it in ordinary tests.

## Scope boundaries and non-goals

- Do not call live OpenAI, download models/dependencies during the suite, add a
  daemon, start a queue, or loosen production policy for test convenience.
- Do not implement a programmatic workspace, Python runtime, `python_exec`,
  skills, or a simulated `programmatic_workspace_v1` success path.
- Do not treat goldens as verification authority that an implementer may
  rewrite silently; changes require an explained reviewable diff.
- Do not claim quality gates pass until measured baseline evidence supports the
  recorded threshold.

## Acceptance criteria

- One deterministic command executes all 20 required fixture scenarios and
  produces byte-stable canonical results across repeated runs.
- All baseline results name `direct_tools_v1` and carry deterministic metric
  fields/omissions. Selecting the reserved programmatic mode fails with a typed
  not-implemented/not-admitted result before source, model, sandbox, or
  acceptance effects.
- A fake mode-neutral scenario-runner contract proves identical task and
  acceptance authority would be supplied to either admitted mode without
  claiming that the future mode currently runs.
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

Report fixture/scenario files, execution-mode contract and reserved-mode
refusal, all 20 scenario outcomes, repeated-run identity, recorded metric
dimensions/omissions, crash/replay and host-safety coverage, retrieval/core
baseline metrics, explicit live-test omission, and full test results.
