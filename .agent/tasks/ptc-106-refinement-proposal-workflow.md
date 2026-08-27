---
id: ptc-106-refinement-proposal-workflow
status: superseded
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-105-versioned-python-skills
---

# Add the evidence-backed refinement proposal workflow

## Supersession

- Superseded on 2026-08-27 by ADR-025. This task is terminal and not runnable.
- No custom refinement/activation infrastructure is planned for the existing
  direct-tools harness without measured failure evidence and a small
  prototype.

Everything below is retained as historical proposal context, not current
instruction.

## Sequence and status

- Sequence: PTC-106 after PTC-105.
- Status: blocked and deferred.
- Phase gate: versioned skills and their host-owned initial approval/rollback
  boundary must be complete. Missing evidence produces deferral.

## Primary outcome

Allow bounded evidence-backed harness-asset proposals, validation, evaluation,
explicit operator approval/rejection, activation, retirement, and rollback
without giving a proposer activation or canonical-state authority.

## Required behavior

- Admit only closed proposal kinds: supplemental prompt note, Python skill,
  retrieval rule, project memory, verification hint, and task compiler rule.
- Bind problem, proposed exact bytes/patch, scope, source trajectory/artifact/
  task/failure/success evidence, benefit, risk, capability impact, affected
  roles, validation/A-B/rollback plans, and creator model/prompt/schema identity.
- Enforce the explicit lifecycle from proposed through schema/policy/tests/
  evaluation and operator-approved or rejected to active/retired/rolled-back.
- Reject capability escalation, base-policy mutation, broader mounts/network/
  credentials/roles/budgets, verification/audit/completion bypass, hidden
  package addition, missing provenance, or changed evidence.
- Require explicit operator approval for every initial activation. No proposal
  may self-activate; approval/rejection/activation/rollback use host-owned
  canonical transactions and exact replay identities.

## Authority and scope boundaries

- Workers may propose only. They cannot activate, choose their next run's asset
  set, grant canonical authority, install packages, or rewrite/delete evidence.
- Execution/evaluation remains sequential; Python remains rootless-sandboxed;
  `direct_tools_v1` remains the rollback path.
- Do not add Graphiti or infer a Graphiti need from refinement evidence.

## Acceptance criteria

- Each proposal kind passes exact schema/provenance fixtures; forbidden effect,
  escalation, missing evidence, stale identity, and self-activation attempts
  fail before active state.
- Deterministic tests cover validation, test/evaluation failure, concurrent
  approval, explicit operator approval/rejection, exact activation, rollback,
  restart, transaction failure, and idempotent replay.
- Future run/completion provenance includes the exact active asset-set
  manifest, while rejected/retired history remains immutable.
- Migration/sqlc, refinement/skill/evaluation, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/refinement ./internal/skill ./internal/evaluation
go test ./...
git diff --check
```

## Expected completion report

Report proposal/lifecycle/schema changes, provenance and forbidden effects,
operator approval/rejection/activation/rollback transactions, replay/recovery,
active asset-set provenance, rollback path, and tests.
