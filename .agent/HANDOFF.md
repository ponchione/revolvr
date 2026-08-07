# Agent Handoff

Updated: 2026-08-07

## Where We Stopped

- Architecture 021 is complete in the current working tree. Architecture-017
  verification occurrences remain immutable/read-only, architecture-018
  retains exclusive completion/finalization authority, and architecture-019
  retains independent audit/finding/correction authority.
- Migration 00012 and named sqlc queries add immutable/rebuildable embedding
  spaces, index build/state, documents/versions, chunks, symbols/edges,
  pgvector vectors, typed relation provenance, and context package/item rows.
  Empty/full/incremental/rebuild/space-switch builds stage separately and
  activate atomically; an interruption is exact-replayable, and any failed
  replacement retains the prior clean revision/space.
- Go AST, TypeScript/JavaScript, Python, Markdown, and SQL structural parsers
  retain exact path/symbol/signature/body/hash/line/source provenance.
  Malformed or unsupported text uses deterministic bounded fallback chunks.
  No generated description model, LanceDB, Graphiti, Neo4j, learned ranker,
  reranker, Python runtime, or programmatic workspace was added.
- `revolvr-hybrid-retrieval-v2` enforces canonical/exact authority before
  structural, bounded PostgreSQL FTS, and optional vector lanes. Exact and FTS
  remain usable when vectors are missing, stale, unhealthy, or wrong-space;
  vector/graph/reranker omissions and degradation are explicit.
- Context packages are deterministic, role-budgeted, immutable, and manifest
  every included/excluded candidate, provenance, ranking signal, retrieval
  config/query-instruction hash, byte/token size, reference instruction,
  omission, and dossier SHA-256. The host query exposes only manifest,
  admitted-item, exact artifact-range, and reserved trajectory-range reads.
- The 25-query real-project suite indexed 22 files / 337 chunks / 239 symbols
  from exact Revolvr, PFF, and Scrapeyard commits. Qwen3-Embedding-0.6B Q8_0
  was selected over CodeRankEmbed and the legacy Nomic code model: vector
  Recall@5/10 0.96/0.96, hybrid Recall@5/10 0.96/1.00, hybrid MRR 0.9444,
  explicit exact-symbol preservation 1.00, hybrid p95 23.80 ms / 69.32 QPS,
  4,470 MiB VRAM, 59.10 chunks/s, and 4,161,536 bytes PostgreSQL growth.
  Exact artifact/revision/image/query identities and all comparison metrics are
  in `docs/architecture/code-indexing-context-assembly.md`.
- Qwen3-Embedding-0.6B Q8_0 is the only supported vector representation.
  Migration constraints, model admission, persistence, evaluation, and query
  code contain no compatibility branch for the prior model. Only the selected
  1,024-dimensional HNSW index exists. Non-selected Revolvr-owned model caches
  and the TEI image were removed; archived comparison metrics remain solely as
  the required selection evidence. The external Sodoryard reference was not
  modified.
- No Go dependency was added. Reversible migration, pinned sqlc, exact focused
  PostgreSQL/pgvector tests, focused race tests, all repository tests, gofmt,
  Compose rendering, and diff checks pass after the Qwen-only compatibility
  removal. Migration 00012 was verified Up/Down/Up from an empty database, and
  its regression test asserts exactly one selected HNSW lane plus rejection of
  unsupported model metadata. No commit was created.
- Architecture tasks 001-021 and PTC-001 are complete. Architecture 022 is the
  next and only dependency-satisfied pending task. Architectures 023-025 and
  every post-core PTC task remain gated behind their existing chain.

## Continue Here

The next and only task for the next fresh session is
`.agent/tasks/architecture-022-deterministic-eval-suite.md`.

Read the durable state files and task-022 required ADR/specification sections.
Preserve the architecture-021 active embedding identity, index/retrieval/context
authority boundaries, architecture-017 verification, architecture-018
completion, and architecture-019 audit/correction authority. Do not rerun
architecture 021 or begin architecture 023.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, and .agent/tasks/architecture-022-deterministic-eval-suite.md. Confirm architecture 022 is the sole legally selectable pending task. Complete only architecture-022-deterministic-eval-suite, preserve architecture-017 verification, architecture-018 completion, architecture-019 audit/correction, architecture-020 embedding identity/degraded status, and architecture-021 indexing/retrieval/context authority, run its deterministic verification, update durable state, and stop.'
```

Graphiti remains deferred: architecture 025 is a decision-only gate after
successful core-loop dogfooding. Audit/correction completion is not evidence
that Graphiti or the post-core programmatic workspace is needed.
