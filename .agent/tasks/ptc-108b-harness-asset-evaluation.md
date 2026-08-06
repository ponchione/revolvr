---
id: ptc-108b-harness-asset-evaluation
status: blocked
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-106-refinement-proposal-workflow
---

# Evaluate harness assets and record the activation decision

## Sequence and status

- Sequence: PTC-108B after PTC-106; final recorded supplemental task.
- Status: blocked and deferred.
- Phase gate: exact candidate harness assets, validations/tests, operator
  approval authority, rollback records, fixed paired fixtures, and real
  sequential dogfood evidence must exist. Missing evidence produces deferral.

## Primary outcome

Separately evaluate exact candidate harness-asset sets against the approved
asset-free baseline and record an evidence-backed decision to defer, reject,
retain experimental, or activate a specific pinned set.

## Required behavior

- Hold task/source/model/prompt/policy/runtime/context/fixture/acceptance
  authority equal and compare exact baseline and candidate asset-set hashes.
- Record success, safety, completion, false-completion, token/context, tool/
  Python/repeated-read/retrieval, verification/reuse, correction, time/resource,
  intervention, and runtime metrics plus every omission/failure.
- Validate capability neutrality, source provenance, deterministic tests,
  regression thresholds, reproducibility, and rollback before a candidate can
  be approved.
- Require explicit operator approval for every initial activation even after a
  favorable evaluation; activation names only the exact evaluated hashes.
- Record rollback to `direct_tools_v1` and the exact base
  `programmatic_workspace_v1` asset-free set. Never erase prior activations or
  evaluation evidence.

## Authority and scope boundaries

- No proposal self-activation, invented or incomparable success, automatic
  budget/capability escalation, runtime package installation, credential/
  network expansion, canonical-state authority, or parallel execution.
- A default-mode change, if ever justified, is a separate explicit operator
  decision; this task cannot remove `direct_tools_v1`.
- Programmatic-workspace or continual-harness results are not evidence that
  Graphiti is needed and cannot amend architecture-025's decision gate.

## Acceptance criteria

- Equal baseline/candidate authority and exact asset hashes are proven for
  every pair; contaminated, missing, stale, incomparable, or selectively
  omitted results produce defer/reject.
- Safety/completion/recovery/original-checkout invariants do not regress and
  any benefit is measured against the asset-free PTC-108A baseline.
- The recorded decision cites all fixture/dogfood/approval/rollback evidence;
  absent evidence never becomes positive activation authority.
- Deterministic evaluation reruns, manifest verification, full Go tests, and
  diff checks pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/evaluation ./internal/refinement -count=2
go test ./...
git diff --check
```

Manually resolve every cited dogfood, asset, operator-approval, and rollback
artifact/hash before recording any activation decision.

## Expected completion report

Report exact baseline/candidate sets, paired identities and metrics, safety and
usefulness results, omissions/failures, operator approval, the decision and
rollback path, Graphiti non-inference, and tests.
