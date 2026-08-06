---
id: architecture-021-code-indexing-context-assembly
status: pending
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-020-local-embedding-adapter
---

# Implement code indexing, retrieval, and context assembly

## Sequence and status

- Sequence: `021` of `025`.
- Status: pending.
- Prerequisite: `architecture-020-local-embedding-adapter`.
- Phase gate: retrieval indexing may consume only a validated embedding-space
  identity and must degrade safely when that optional service is unavailable.

## Primary outcome

Build a reproducible PostgreSQL FTS/pgvector/structural code index and assemble
frozen, provenance-rich, role-budgeted context packages that always prefer
exact sources over fuzzy retrieval.

## Required reading

- ADR-007, ADR-016 through ADR-018, and ADR-023.
- Specification Sections 9.19-9.23, 10.7, 13, 14,
  27.1-27.4, 29 Phase 7 and Phase 8, 37.12-37.13,
  40.9-40.10, 43.5-43.6, 50-53, and NFR-006/NFR-008.
- `REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` Sections 2.2
  and 15 only as subordinate external-reference/query compatibility guidance.

## Existing foundations to inspect

- Registered project/source snapshots, managed Git copies, artifact/event
  storage, task/plan expected paths, role dossiers, and embedding adapter.
- `internal/dossiercache`, `internal/repositorypath`, and exact Git/text search
  helpers for reusable deterministic behavior.
- The existing repository contains no canonical Tree-sitter/pgvector index;
  do not preserve LanceDB or external Sodoryard storage coupling.

## Starting assumptions

- Exact task references, file paths, symbols, and text search outrank semantic
  similarity.
- Indexed/search/graph data is rebuildable derived state tied to project source
  revision and embedding space.
- Initial language priorities are Go, TypeScript/JavaScript, Python, Markdown,
  and SQL; deterministic bounded fallback chunks cover unsupported syntax.

## Implementation requirements

- Add reversible document/version, chunk/embedding-space/vector, symbol/edge,
  relation/provenance, index-state, context-package/item schema and named
  queries with pgvector/FTS indexes appropriate to measured queries.
- Walk only admitted files, respect include/exclude rules, hash bytes, parse
  syntax-aware semantic units, extract symbol/import/call/reference edges, and
  preserve path/language/line/content/source provenance.
- Support full rebuild and changed-file incremental indexing with
  `never_indexed`, `clean`, `dirty`, `building`, and `failed` states; build a new
  embedding space before atomic activation and retain the prior valid space on
  failure.
- Retrieve in the exact order from Section 14.1, combine lexical/vector/
  structural/authority signals transparently (using the Section 51 baseline or
  a measured documented adjustment), and deduplicate by chunk identity.
- Build immutable role-specific dossiers within byte/token budgets, recording
  included/excluded candidates, source hashes/scores, retrieval config,
  embedding space, final size, and dossier SHA-256.
- Represent each context item by exact bounded inline content or an immutable
  content-addressed artifact or trajectory-range reference. Reference forms
  retain exact identity, hash, range/offset, media type, and retrieval
  instructions; unresolved references are omissions or failures, never
  silently summarized content.
- Record a deterministic manifest of every intentionally externalized item,
  including its hash, explicit omission reason, authority/ranking class, and
  bounded retrieval instructions.
- Enforce source precedence: exact accepted task state, exact project files
  and symbols, host policy, and canonical verification/audit/evidence outrank
  model-authored scratch, summaries, compaction, or refinement material.
- Expose a narrow read-only internal host query interface for manifests,
  admitted context items, exact artifact ranges, and reserved trajectory-range
  references. It must be suitable for a later sandbox-brokered client without
  giving that client storage, lifecycle, policy, or canonical-state authority.
- When embeddings are stale/unavailable, keep direct reads, exact symbol/text,
  and FTS usable; label omitted/stale vector and graph lanes explicitly.
- Create real-project retrieval fixtures from Section 50 and measure Recall@5,
  Recall@10, MRR, symbol preservation, latency, VRAM, index size, throughput,
  and language breakdown before choosing the active local model.

## Scope boundaries and non-goals

- Do not add LanceDB, Graphiti, Neo4j, a learned ranker, mandatory reranker,
  graph lifecycle authority, or broad source dumps.
- Do not add a Python API/runtime, `python_exec`, durable scratch store,
  trajectory service, skill/refinement system, or continual harness.
- Do not let retrieved memory override accepted tasks, policy, verification, or
  direct source references.
- Do not reindex unchanged files or silently query vectors from the wrong
  source revision/embedding space.

## Acceptance criteria

- Empty/full/incremental/rebuild and embedding-space-switch integration tests
  produce reproducible rows and no mixed or stale active space.
- Fixture queries preserve exact path/symbol priority and meet recorded
  retrieval thresholds selected from baseline data.
- Malformed source, parser failure, deleted/renamed files, duplicate chunks,
  stale commit, embedding outage/drift, interrupted build, and failed activation
  recover without corrupting the prior clean index.
- Context manifests are deterministic under equal inputs, enforce each role's
  budget, and retain provenance for every inline/reference item, intentionally
  externalized item, retrieval instruction, and omission.
- Host-query tests are read-only, bounded, deterministic, and preserve the
  authority precedence of exact task/source/symbol/policy/evidence over any
  model-authored advisory content.
- Migration/sqlc, PostgreSQL/pgvector, parser, retrieval-quality, degraded-mode,
  and full Go tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/index ./internal/retrieval ./internal/context
go test ./...
git diff --check
```

## Expected completion report

Report schema/index/package changes, supported parsers/fallback, incremental and
space-switch behavior, ranking formula, fixture metrics/model decision,
degraded/recovery cases, context budgets/manifests, dependencies, and tests.
