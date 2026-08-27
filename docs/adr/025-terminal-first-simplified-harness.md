# ADR-025 — Terminal-First Product and Simplified Harness

- Status: Accepted
- Date: 2026-08-27
- Supersedes: ADR-020, ADR-021, and the programmatic-workspace roadmap

## Context

Revolvr already has a Bubble Tea operator interface, shared Go application
services, direct Codex/tool execution, canonical PostgreSQL state, append-only
events, receipts, and content-addressed artifacts. A second desktop/web surface
or speculative Python harness would duplicate working boundaries without
evidence that they are insufficient.

## Decision

- Revolvr is terminal-first. No desktop GUI, Wails/Vue frontend, embedded web
  server, or local REST/SSE interface is planned.
- Extend the existing Bubble Tea TUI and reuse the existing application
  services and installed dependencies. Business logic stays outside the TUI.
- Codex CLI interaction patterns are inspiration only. Revolvr will not clone,
  vendor, port, or depend on Codex source.
- Keep the "brain" concept narrowly defined as durable project knowledge,
  relationships, retrieval, prior evidence, and provenance-bearing context
  assembly.
- Canonical truth remains in Revolvr's existing Go/PostgreSQL ledger and
  artifact model. The brain and every optional retrieval lane are subordinate
  projections or context.
- Graphiti remains optional and evidence-gated. It is deferred unless real
  usage proves a concrete retrieval gap that the existing brain cannot meet.
- Direct tool execution remains the default harness. Do not build custom
  Python environments, `python_exec`, scratch runtimes, Python skill systems,
  or refinement infrastructure without measured dogfood evidence that direct
  tools fail and a small prototype demonstrating the need.

## Consequences

- Architecture 024 refines the existing TUI and is the next implementation
  task.
- Architecture 025 is an evidence-only decision gate; it does not implement or
  plan Graphiti.
- PTC-101 through PTC-108B are superseded and non-runnable.
- ADR-020 and ADR-021 remain as architectural history, not current direction.
