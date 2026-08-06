---
id: ptc-104-durable-scratch-catalog
status: blocked
workflow: mixed-pass-v1
phase: implement
depends_on: ptc-103-python-exec-broker-integration
---

# Add an explicit durable scratch catalog with tombstones

## Sequence and status

- Sequence: PTC-104 after PTC-103.
- Status: blocked and deferred.
- Phase gate: role-scoped `python_exec` must be implemented, isolated, and
  evidenced before durable scratch is admitted.

## Primary outcome

Add explicit named, versioned, content-addressed task-local scratch entries
that survive container restart without making Python process state canonical.

## Required behavior

- Support bounded text, JSON, bytes, and table entries with stable name,
  version, media type, project/task/run/workspace, creator role/occurrence,
  source trajectory sequence/references, artifact/hash/size, host timestamps,
  supersession, expiry policy, and secret-scan evidence.
- Make create/update/tombstone append-only and atomic with immutable artifact
  and trajectory evidence. Delete creates a tombstone and never erases history.
- Expose only role-scoped brokered put/get/list/tombstone operations; recovery
  reloads explicit entries into a fresh runtime.
- Rank scratch as advisory below accepted task/plan, direct source, host policy,
  verification, audit, and operator decisions.

## Authority and scope boundaries

- Do not persist arbitrary Python objects, globals, interpreter/process
  snapshots, subprocess handles, or pickle graphs.
- No scratch value may mutate or satisfy canonical task, plan, criterion,
  finding, lifecycle, verification, or completion state.
- Add no skills, refinement activation, runtime package installation, network,
  credentials, worker role, Graphiti, or cross-task promotion path.

## Acceptance criteria

- Exact restart fixtures recover named entries from artifacts while proving
  transient globals did not survive.
- Version/supersession/tombstone, stale source/runtime, collision, bounds,
  secret, corrupt/missing artifact, rollback, and concurrent update cases fail
  or reconcile deterministically without history loss.
- Context/ranking tests prove canonical direct evidence always outranks scratch.
- Migration/sqlc, PostgreSQL/artifact/programmatic, and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/scratch ./internal/programmatic ./internal/artifact
go test ./...
git diff --check
```

## Expected completion report

Report schema/catalog/API changes, entry and artifact identities, authority
ranking, restart/tombstone/recovery behavior, prohibited serialization proof,
secret handling, and all tests.
