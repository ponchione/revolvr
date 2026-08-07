# Agent Handoff

Updated: 2026-08-07

## Where We Stopped

- Architecture 023 is complete in the current uncommitted working tree. The
  canonical foreground queue uses migration 00013 and `internal/queue` to
  persist a UUIDv7 operation, exact ordered task occurrences/effects, pinned
  finite limits, typed yields/stops, cancellation evidence, and a terminal
  marker. It invokes task-009 for every selection, pins one occurrence through
  its terminal-for-now checkpoint, and uses the existing singleton global
  source-mutation lease.
- `direct_tools_v1` and exactly one source-mutating worker are fixed by code
  and database constraints. Instrumented PostgreSQL tests observed peak one;
  deterministic ordering, dependency unlock, unrelated progress, every yield
  and stop class, exact budgets, correction/reverification, and concurrent
  direct-run admission all pass.
- Exact intent precedes selection and every supervisor/worker/verification/
  audit/correction/completion effect. All eight before/after crash boundaries
  replay idempotently. The external-effect/local-completion gap reconciles
  once, terminal replay starts no work, and divergent effect or terminal
  evidence fails closed. Cancellation stops the cooperative active child and
  reconciles workspace, evidence, task result, lease, checkpoint, and terminal
  state.
- CLI/application behavior is `revolvr queue start|status|cancel`. Start
  requires a UUIDv7 and exposes finite task/cycle/token/cost/time bounds;
  status reports pinned configuration, ordered outcomes, usage, peak workers,
  and terminal identity. No daemon, background service, network fetch,
  parallel worker setting, or automatic archive/export/push/deploy/merge path
  was added.
- The Section 23.3 real-project quality threshold remains deliberately unset.
  `deterministic_evaluation_only` is the only gate value and ordinary CLI
  start fails closed before database/worker effects without an injected
  admitted executor. No real-project queue dogfood or invented quality claim
  was made.
- Architecture 017-022 authority remains unchanged, including the exact Qwen
  identity/degraded-no-fallback behavior, exact-first retrieval/context,
  deterministic golden suite, and closed worker modes.
- Verification passed: formatting, sqlc v1.27 generation and stability,
  PostgreSQL queue/evaluation and all focused Architecture 023 scenarios,
  queue race tests, migration 00013 down/up, CLI/application tests and help,
  full `go test ./...`, and `git diff --check`. No dependency changed, the
  disposable PostgreSQL test database was removed after verification, and no
  commit was created.
- Architecture tasks 001-023 and PTC-001 are complete.
  `.agent/tasks/architecture-024-ui.md` is the next and only dependency-
  satisfied pending task. Architecture 025 and all deferred PTC tasks remain
  gated.

## Continue Here

The next fresh pass may select only `.agent/tasks/architecture-024-ui.md`.
Read its ADR/specification requirements and revalidate its phase gate before
making changes. Do not rerun Architecture 023 or begin Architecture 025.

Start one fresh pass from the repository root:

```bash
codex exec 'Read AGENTS.md, README.md, .agent/HANDOFF.md, .agent/TASKS.md, .agent/STATE.md, .agent/DECISIONS.md, .agent/LOOP_PROMPT.md, REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md, and .agent/tasks/architecture-024-ui.md. Confirm architecture 024 is the sole legally selectable pending task and its phase gate is met. Complete only architecture-024-ui, preserve all architecture-017 through architecture-023 authority, run its required verification, update durable state, and stop.'
```

Graphiti remains deferred to the Architecture 025 decision-only gate. The
programmatic workspace, Python execution, skills, refinement, and continual
harness remain blocked behind the complete architecture sequence and their
separate measured/operator gates.
