---
id: architecture-020-local-embedding-adapter
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-019-auditor-corrector
---

# Implement the local embedding service adapter

## Sequence and status

- Sequence: `020` of `025`.
- Status: completed.
- Prerequisite: `architecture-019-auditor-corrector`.
- Phase gate: Phase 7 retrieval work starts after the complete core
  implement/verify/audit/correct loop is enforceable without embeddings.

## Primary outcome

Provide a versioned Go adapter to a dedicated local embedding service, with
exact model metadata, bounded batches, health/failure classification, and safe
degraded behavior.

## Required reading

- ADR-007, ADR-008, ADR-017, and ADR-018.
- Specification Sections 7.3, 7.6, 9.20, 14.4-14.6,
  15.5, 27.5, 29 Phase 7, 37.13, 40.12, 42, and 50.

## Existing foundations to inspect

- PostgreSQL `retrieval` schema/pgvector foundation and application config
  conventions.
- `compose/compose.yaml` network/secret style and the OpenAI client's bounded
  HTTP/stream/error patterns where reusable.
- Artifact/event and diagnostic persistence; the adapter does not receive a
  repository mount.

## Starting assumptions

- Embeddings are local and derived; canonical task/run operation must continue
  in a clearly labeled degraded mode when the service is unavailable.
- One project has one active embedding space at a time.
- Model selection is evaluation-driven; no old Sodoryard model is accepted by
  historical inertia.

## Implementation requirements

- Implement the specified `EmbedDocuments`, `EmbedQuery`, and metadata
  responsibility with bounded input count/bytes, timeout, cancellation, and
  dimension/finite-value validation.
- Require exact model name, revision, dimensions, pooling, normalization,
  quantization, and artifact hash; hash this as an embedding-space identity.
- Add dedicated service configuration and Compose wiring with GPU access,
  read-only model storage, internal network only, and no project/database/
  OpenAI-secret/runtime-socket mounts.
- Classify unhealthy, unavailable, malformed response, wrong count/dimension,
  non-finite vector, model metadata drift, timeout, and cancellation without
  corrupting or silently mixing spaces.
- Expose explicit degraded status for callers; do not fabricate vectors or
  fall back to a remote provider.

## Scope boundaries and non-goals

- Do not index source, choose a permanent model without evaluation data, add a
  reranker, use local models for reasoning, or make embeddings canonical.
- Do not implement Graphiti or give the embedding container repository access.
- Do not silently change dimensions/model revision inside an active space.

## Acceptance criteria

- A fake local endpoint proves document/query batching, metadata identity,
  deterministic validation, deadlines, and cancellation without network.
- Every malformed/unavailable/drift case yields typed degraded/failure state
  and stores no mixed-space vector.
- Rendered service configuration has GPU/model access but no project source,
  database/OpenAI credential, host-public port, or runtime socket.
- An explicit local-service smoke command returns metadata and correct vector
  dimensions when the operator supplies the evaluated service image/model.
- Full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go test ./internal/embedding
REVOLVR_POSTGRES_PASSWORD=test-only docker compose -f compose/compose.yaml -f compose/compose.dev.yaml config --quiet
go test ./...
git diff --check
```

Record the separate opt-in GPU smoke command and exact model metadata in the
completion report; deterministic fake-endpoint tests remain mandatory.

## Expected completion report

Report adapter/config/service files, embedding-space metadata/hash, batch
bounds, degraded/error coverage, rendered isolation proof, opt-in GPU smoke
result or explicit omission, dependency decision, and full test results.
