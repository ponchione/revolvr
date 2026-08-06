---
id: ptc-101-trajectory-manifest-query-service
status: blocked
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-025-memory-graphiti-phase-gate
---

# Add the append-only trajectory manifest and bounded query service

## Sequence and status

- Sequence: PTC-101, first post-core supplemental task.
- Status: blocked and deferred.
- Prerequisite: architecture tasks 001-025 are complete.
- Phase gate: do not change this task to pending or select it until exact
  initial core-loop dogfooding evidence has been recorded and verified. Missing
  evidence produces deferral, never invented success.

## Primary outcome

Normalize the complete ordered run trajectory over existing canonical
events/artifacts, publish deterministic manifests, and expose bounded read-only
query projections without creating a second canonical state system.

## Required behavior

- Include every material context, model, direct-tool, host decision/denial,
  source capture, verification/reuse, audit/correction, artifact, policy, and
  lifecycle occurrence with a trusted-host monotonically increasing sequence.
- Publish versioned manifests with run/source identities, exact first/last
  sequence, counts, ordered entry IDs/hashes, referenced artifact IDs/hashes,
  context hashes, model identities, harness-asset-set identity, and manifest
  hash.
- Provide bounded list/get/search/range interfaces whose indexes are
  rebuildable projections and never override canonical bytes or ordering.
- Make architecture-018 completion provenance able to resolve the exact final
  manifest and fail closed on missing, stale, divergent, or incomplete
  coverage.
- Preserve current-state tables plus append-only events under ADR-019; do not
  introduce full event sourcing.

## Authority and scope boundaries

- Execution remains sequential. The host owns sequence, hashes, lifecycle,
  source, evidence, and completion; workers gain only bounded read access.
- Add no Python runtime, scratch catalog, skills, refinement activation,
  Graphiti, worker canonical-state authority, or model-authored ordering.
- Do not grant host, database, runtime, model credential, network, or package
  installation capability to a worker.

## Acceptance criteria

- Equal canonical entries produce byte-identical manifests; changed order,
  content, range, artifact, source, or asset identity changes the manifest.
- Gap, duplicate, stale, unresolved-artifact, wrong-run/source, and incomplete
  final-range fixtures fail closed.
- Bounded queries return exact sequence/reference evidence, enforce limits, and
  remain read-only; rebuilding indexes produces identical results.
- PostgreSQL/artifact integration, deterministic manifest/query, recovery, and
  full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/trajectory ./internal/artifact
go test ./...
git diff --check
```

## Expected completion report

Report the dogfood gate evidence, schema/projection changes, sequence and
manifest contracts, query bounds, completion resolution, corruption/recovery
coverage, unchanged authority boundaries, and all test results.
