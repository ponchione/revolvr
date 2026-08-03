# ADR-008 — Local Embeddings, Remote Frontier Reasoning

- Status: Accepted
- Date: 2026-08-03
- Source: Section 4, “ADR-008 — Local Embeddings, Remote Frontier Reasoning,” in [`REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`](../../REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md)

## Context

Initial model responsibilities need a clear boundary between remote frontier reasoning and local inference.

## Decision

Use OpenAI remote models for supervisor decisions, planning, implementation reasoning, correction reasoning, independent auditing, and difficult task compilation. Use local models for embeddings and, optionally, reranking, low-risk classification, and entity extraction after evaluation. Local coding models are not a v1 dependency.

## Consequences

- Core reasoning depends on OpenAI remote models while embeddings remain local.
- Local reranking, classification, and entity extraction remain optional and evaluation-gated.
- v1 does not require a local coding model.
