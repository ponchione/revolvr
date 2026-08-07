# Architecture 021: code indexing, retrieval, and context assembly

## Outcome and authority boundary

Architecture 021 adds a reproducible PostgreSQL FTS/pgvector/structural index,
hybrid retrieval, immutable role-budgeted context packages, and a bounded
read-only host query. All index, vector, relation, and context rows are derived
or frozen evidence. They do not grant task lifecycle, policy, verification,
completion, audit, correction, storage-lifecycle, or canonical-state authority.
Architecture 017 verification evidence, Architecture 018 completion admission,
and Architecture 019 audit/correction remain the only authorities for their
respective decisions.

The implementation indexes exact authoritative semantic units. Embedding input
contains path, symbol, signature, language, line range, content SHA-256,
structural parser/mode, and exact source body. No generated code descriptions
or second generative model are present.

## Schema and build behavior

Migration `00012_retrieval_context.sql` adds reversible tables for embedding
spaces, index builds/state, documents and immutable versions, chunks, symbols,
symbol edges, vectors, typed relations and provenance, context packages, and
context items. PostgreSQL owns `simple` FTS GIN and one 1,024-dimensional
partial HNSW index for the selected Qwen representation. Schema checks reject
every other model name, dimension, pooling, normalization, or quantization;
there are no prior-model vector indexes or query branches. Named sqlc queries
cover every write, activation, exact/lexical/structural/vector read, and context
read. Vectors use explicit text casts in generated queries, so no new Go
pgvector dependency was needed.

The admitted input is an exact Git commit/tree and a path allowlist. Symlinks,
submodules, `.git`, `.revolvr`, vendor, and node_modules content are rejected or
excluded. Parsers cover Go AST declarations/imports/calls, TypeScript and
JavaScript declarations/calls, Python indentation declarations/calls, Markdown
sections, and SQL statements. Malformed or unsupported text uses deterministic
line/byte windows. Limits default to 100,000 files, 4 MiB per file, 32 KiB and
240 lines per chunk.

Full, empty, incremental, rebuild, and embedding-space-switch builds stage a
complete replacement in one transaction. A space switch can change only the
revision/artifact identity of the selected Qwen3-Embedding-0.6B Q8_0
representation; it is not a legacy-model compatibility path. Activation
validates file, chunk, symbol, and vector counts and atomically changes the
active build. Incremental builds reuse unchanged document versions and vectors
only when the exact chunk identity and embedding-space SHA-256 match. An
interrupted committed stage can be replayed by operation ID. A failed
replacement retains the prior active revision/space and returns the project to
`clean`; `failed` means no prior clean index exists. Deleted and renamed files
disappear from new build membership.

## Retrieval and context behavior

`revolvr-hybrid-retrieval-v2` applies the Section 51 scores but first orders by
authority class. Exact task/source/policy/evidence therefore cannot be displaced
by structural, lexical, vector, or model-advisory material. Natural-language
FTS is deterministically compiled to at most 32 OR-connected seven-character
prefix lexemes after a fixed stop-word filter. This measured adjustment bridges
variants such as “persists” and “persistence” while retaining PostgreSQL's
`simple` tokenizer for identifiers. Every lane reports `used`, `empty`,
`omitted`, `stale`, or `degraded`; the relationship graph and reranker remain
explicitly omitted in this baseline.

Context packages sort by authority, score, path, line, and stable identity,
then pack exact inline bytes or resolved immutable artifact/trajectory ranges
within role byte/token budgets. The manifest records included and excluded
candidates, ranking signals, source hashes, revision, embedding space, exact
query-instruction SHA-256, retrieval configuration, byte/token sizes, reference
instructions, omissions, and dossier SHA-256. Unresolved references remain
omissions with their bounded retrieval instruction. Package identity derives
from the complete manifest, and database triggers reject package/item mutation.

The host query has four methods only: manifest, admitted items, exact admitted
artifact range, and reserved trajectory range. Counts and ranges are bounded;
full artifact bytes and SHA-256 are checked before returning a range. It exposes
no database handle or mutation method.

## Real-project evaluation dataset

The frozen fixture manifest is
`internal/retrieval/testdata/architecture-021-real-projects.json`, SHA-256
`4b8bf7283578c9b0b6cda5230f9d4420d473cf03c8901d6c6f9ba4b0ee6deeb2`.
It admits 22 files, 337 chunks, and 239 symbols and contains 25 judged
natural-language queries across these exact sources:

| Project | Commit | Tree | Files / chunks |
| --- | --- | --- | ---: |
| Revolvr | `5a99f1455273b9b1a47560831159205e5773b2b7` | `e777750997a0dd7302ea2e80e1a9c1e5c21040c7` | 12 / 248 |
| PFF | `e5ad44aa5674360bc76ac0fccbbebd666f2544d6` | `7fdf6c614d2c47289d44f50203a4ce4dec303a82` | 5 / 45 |
| Scrapeyard | `f8e1af0244e56d70c39c7383fafd9c97938442eb` | `e6d12313d7ffab25a3702a58c2d47e3cdc141de2` | 5 / 44 |

The comparison threshold, selected from the lexical baseline before model
selection, was vector Recall@5 at least 0.90, vector Recall@10 at least 0.95,
hybrid Recall@5/10 at least 0.95, explicit exact-symbol preservation 1.00,
hybrid p95 below 50 ms, and VRAM below 10 GiB on the local RTX 4090. The common
lexical-only baseline measured Recall@5 0.84, Recall@10 0.96, MRR 0.4657, and
approximately 1.8 ms p95.

## Archived selection evidence

This section is retained only as the immutable A/B evidence required to justify
the selection; the non-selected models have no schema, query, configuration,
or adapter compatibility. Disk was checked before evaluation downloads: 1.1
TiB was free. Artifacts were stored outside Git. After selection, the
Revolvr-owned CodeRank artifacts and TEI image were removed. The prior Nomic
artifact was evaluated read-only from its pre-existing Sodoryard path and was
never copied into or coupled to Revolvr.

| Candidate | Exact revision and artifact | Dimensions / pooling / normalization | License | Pinned evaluation runtime |
| --- | --- | --- | --- | --- |
| Qwen3-Embedding-0.6B-GGUF Q8_0 | revision `370f27d7550e0def9b39c1f16d3fbaa13aa67728`; 639,150,592 bytes; SHA-256 `06507c7b42688469c4e7298b0a1e16deff06caf291cf0a5b278c308249c3e439` | 1,024 / last / L2; Q8_0 | Apache-2.0 | llama.cpp `sha256:8903d304f9cadf35ac881ebf0bb3537426b5b096b63088d0f17b719656b07c20` |
| Nomic CodeRankEmbed | revision `3c4b60807d71f79b43f3c4363786d9493691f8b1`; 546,938,168-byte FP32 safetensors; SHA-256 `827529bcd58aef0d9082e66eeff7e7d53a02f62bd005f841a26b3d3e2fb17ebe`; FP16 evaluation runtime | 768 / CLS / L2 | MIT | TEI `sha256:84bf2441cc388266613ebfe6692799db59f32b844dd95ab998ddee5eba84975a` |
| Legacy Nomic Embed Code GGUF Q8_0 | revision `ff2ddedde976ea623178981f18e36af33c0c2a94`; 7,519,464,768 bytes; SHA-256 `80bbd617cc55bad52ceb0f7dfad5d2f5b8080c61015cad7063ffc438b6561832` | 3,584 / last / L2; Q8_0 | Apache-2.0 | same pinned llama.cpp image |

Primary model sources are the official
[Qwen GGUF repository](https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF),
[CodeRankEmbed repository](https://huggingface.co/nomic-ai/CodeRankEmbed), and
[legacy Nomic GGUF repository](https://huggingface.co/nomic-ai/nomic-embed-code-GGUF).
The CodeRank query instruction was `Represent this query for searching relevant
code: ` (SHA-256 `142a69412548000d1ecc269939858453524546a159f156a51eb1d4a3eea53efb`).
Qwen used `Instruct: Given a natural language query, retrieve relevant source
code that answers the query\nQuery: ` with a real newline (SHA-256
`8a1900345dce8d58adb5671a807e86ed39eeb6da706c491f71fa845c7ed9f59a`).
The local Go evaluation proxy preserved Architecture 020 health/metadata/vector
bounds and rejected drift or non-unit vectors; stock servers received no
metadata or fallback authority.

## Measured results and selection

All measurements are from 2026-08-07 on the same RTX 4090, PostgreSQL/pgvector
database, exact chunks, retrieval-v2 configuration, and 25-query dataset.

| Model | Vector R@5 / R@10 / MRR | Hybrid R@5 / R@10 / MRR | Exact symbol | Hybrid p95 / QPS | VRAM | Build throughput | PostgreSQL growth |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: |
| Qwen3 0.6B Q8_0 | 0.96 / 0.96 / 0.8727 | 0.96 / 1.00 / 0.9444 | 1.00 | 23.80 ms / 69.32 | 4,470 MiB | 59.10 chunks/s | 4,161,536 B |
| CodeRankEmbed | 0.80 / 0.84 / 0.7657 | 0.96 / 0.96 / 0.9225 | 1.00 | 40.52 ms / 50.45 | 756 MiB | 333.51 chunks/s | 4,005,888 B |
| Legacy Nomic code Q8_0 | 0.92 / 0.96 / 0.8767 | 0.96 / 0.96 / 0.9236 | 1.00 | 48.55 ms / 28.96 | 8,720 MiB | 25.91 chunks/s | 9,633,792 B |

Vector Recall@5 by language was:

| Model | Go | TypeScript | Python | Markdown | SQL |
| --- | ---: | ---: | ---: | ---: | ---: |
| Qwen3 0.6B Q8_0 | 0.80 | 1.00 | 1.00 | 1.00 | 1.00 |
| CodeRankEmbed | 0.40 | 1.00 | 1.00 | 1.00 | 0.50 |
| Legacy Nomic code Q8_0 | 0.80 | 1.00 | 1.00 | 1.00 | 0.75 |

Qwen3-Embedding-0.6B Q8_0 is selected as the active Architecture 021 default.
It is the only candidate with vector Recall@5 0.96 and hybrid Recall@10 1.00,
while using about half the legacy VRAM, less than half its PostgreSQL vector
growth, roughly twice its query throughput, and lower latency. CodeRankEmbed's
small footprint and indexing speed did not offset its failure of the vector
recall threshold. Selection never permits automatic download, remote fallback,
metadata drift, or reuse of a wrong source revision/embedding space.

No Recall, MRR, or latency evidence from Sodoryard was reused: its 680 files and
6,557 chunks had no retained quality metrics, only 50 of 147 sampled chunks had
descriptions, descriptions were generic/truncated, and its 529 MiB / 9,178
LanceDB-version history was not imported or coupled.

## Dependencies and reproduction

No Go module dependency was added. PostgreSQL pgvector was already part of the
pinned database image. sqlc vectors remain strings passed through explicit
casts. The selected Qwen artifact remains in the ignored external cache and
uses the digest-pinned llama.cpp runtime. Non-selected Revolvr-owned weights
and the TEI runtime image were removed after the archived measurements were
recorded.

`cmd/revolvr-retrieval-eval` strictly loads the frozen fixture manifest, reads
the exact Git trees, admits only the selected Qwen representation, runs
vector-only, lexical-only, and hybrid suites, and emits model metadata, space
identity, Recall@5/10, MRR, explicit symbol preservation, latency, QPS,
language metrics, VRAM, build throughput, index growth, and project manifests.
The local `cmd/revolvr-embedding-eval-adapter` is bounded, loopback-only, and
also rejects every non-selected representation or query-input policy while
preserving the Architecture 020 health/metadata/drift contract. Retrieval and
evaluation accept only the recorded selected Qwen query-instruction SHA-256.
