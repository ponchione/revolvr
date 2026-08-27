---
id: ptc-105-versioned-python-skills
status: superseded
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-108a-base-programmatic-workspace-evaluation
---

# Add versioned, tested, hash-pinned Python skills

## Supersession

- Superseded on 2026-08-27 by ADR-025. This task is terminal and not runnable.
- A Python skill system is speculative under the direct-tools harness and is
  not planned without measured dogfood evidence and a small prototype.

Everything below is retained as historical proposal context, not current
instruction.

## Sequence and status

- Sequence: PTC-105 only after PTC-108A.
- Status: blocked and deferred.
- Phase gate: do not change this task to pending unless PTC-108A contains exact
  independently verifiable evidence that the base programmatic workspace is
  safe and useful and explicitly admits bounded skill work. Missing,
  inconclusive, or negative evidence produces deferral.

## Primary outcome

Define immutable, versioned, tested, role-scoped Python skills and load only
an exact host-admitted skill-set manifest into the programmatic workspace.

## Required behavior

- Each skill records stable ID/version, content/manifest hashes, source
  evidence, entry point, closed input/output schemas, permitted roles and
  broker capabilities, subprocess policy, runtime/output bounds, pinned image
  dependencies, fixtures/tests, scope, and rollback identity.
- Skills run only inside the existing disposable rootless sandbox and create
  scratch/artifacts only through admitted broker APIs.
- Every run/model/tool/completion/evaluation provenance records the exact
  active skill-set manifest/hash.
- Initial activation of every version requires explicit operator approval
  through host authority. Activation and rollback append new records and never
  rewrite history.

## Authority and scope boundaries

- No runtime package installation, remote fetch/network, host/database/
  runtime/model credentials, arbitrary snapshot/pickle, canonical-state
  mutation, capability escalation, new worker role, or proposal self-activation.
- This task does not implement the general refinement proposal workflow; only
  explicitly task-scoped reviewed skill fixtures may be activated for tests.
- Execution remains sequential and `direct_tools_v1` remains the rollback path.

## Acceptance criteria

- Manifest/hash/version/role/capability tests accept only exact pinned skills
  and reject drift, unknown imports/capabilities, stale image/runtime,
  excessive resources/output, and unapproved activation.
- Skill exceptions are ordinary typed untrusted evidence and cannot mutate
  policy, lifecycle, verification, audit, completion, or active manifests.
- Explicit initial operator-approval and rollback fixtures are transactional,
  attributable, idempotent on exact replay, and fail closed on changed evidence.
- Migration/sqlc, sandbox/programmatic/skill, and full Go tests pass without
  runtime installation or public network.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/skill ./internal/programmatic ./internal/sandbox
go test ./...
git diff --check
```

## Expected completion report

Report the PTC-108A admission evidence, skill/manifest schemas and hashes,
role/capability boundary, pinned dependencies, operator approval/rollback,
failure cases, direct-tools fallback, and all tests.
