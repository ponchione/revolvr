---
id: ptc-108a-base-programmatic-workspace-evaluation
status: blocked
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-107-programmatic-context-continuation
---

# Evaluate direct tools against the base programmatic workspace

## Sequence and status

- Sequence: PTC-108A after PTC-107 and before any Python skill implementation.
- Status: blocked and deferred.
- Phase gate: all base runtime, broker, scratch, trajectory, context, recovery,
  and isolation work must be complete; fixed fixtures and at least ten exact
  sequential post-core dogfood tasks must be available. Missing evidence
  produces deferral.

## Primary outcome

Run paired `direct_tools_v1` versus base `programmatic_workspace_v1`
evaluations using the same task, source, model policy, acceptance authority,
and scenario identities, with no self-authored activated Python skills.

## Required behavior

- Pin exact worker/runtime/image/protocol/context/fixture/model/prompt/policy
  identities and a canonical empty/inactive harness-asset-set manifest for both
  sides where applicable.
- Cover deterministic core scenarios plus large-log diagnosis, multi-file
  navigation, stable-failure correction, long-history/context pressure,
  runtime crash/recovery, stale scratch/identity refusal, malicious Python
  isolation, and at least ten real sequential dogfood tasks.
- Record final typed outcome, acceptance evidence, false-completion attempts,
  policy denials, original-checkout identity, context bytes, available input/
  output/reasoning/cached tokens, direct-tool and Python execution counts,
  repeated reads, retrievals, verification executions/reuses, correction
  cycles, wall time, resource use, operator interventions, and protocol/runtime
  failures.
- Produce an evidence-backed decision only to defer, retain experimental base
  mode, or admit PTC-105 skill work. Do not make programmatic mode the default
  and do not activate an asset.

## Authority and scope boundaries

- Use sequential execution only. Python remains inside the disposable rootless
  sandbox with no host/database/runtime/model credentials, network, runtime
  package installation, pickle authority, or canonical-state authority.
- Do not author, activate, simulate, or evaluate self-authored skills or
  refinements in this task. `direct_tools_v1` remains the rollback path.
- Programmatic-workspace evidence is not evidence that Graphiti is needed.

## Acceptance criteria

- Paired runs preserve identical task/acceptance authority and exact fixture
  identity; missing, incomparable, contaminated, or unpinned pairs are rejected.
- Programmatic mode passes every direct-tools safety, completion, recovery, and
  original-checkout invariant with zero new boundary violation or unauthorized
  canonical mutation.
- PTC-105 remains blocked unless exact results demonstrate both safety and a
  useful measured base-workspace outcome. Absent or negative evidence records
  deferral/retain-experimental, never invented success.
- Deterministic paired results and dogfood manifests are independently
  verifiable; full Go tests and diff checks pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/evaluation -count=2
go test ./...
git diff --check
```

Manually verify every cited dogfood manifest, runtime/image identity, paired
authority, safety outcome, and metric before recording the decision.

## Expected completion report

Report all paired scenarios and dogfood identities, empty asset-set proof,
comparability/safety/usefulness metrics, failures and omissions, the exact
defer/experimental/admit-PTC-105 decision, rollback path, and tests.
