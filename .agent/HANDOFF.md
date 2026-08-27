# Agent Handoff

Updated: 2026-08-27

## Where We Stopped

- `architecture-025-memory-graphiti-phase-gate` completed at `phase: simplify`
  with decision **defer**. Architecture tasks 001-025 are complete.
- The checkout-local runtime contains 22 completed runs from 2026-07-08 through
  2026-07-09. All predate Architecture 021 commit `9fd0de6` and the current
  Architecture 024 TUI commit `2a4dc6f`, so qualifying current-brain/current-
  TUI real-usage history is absent.
- The baseline-metrics gate is supported by the Architecture 021 25-query
  real-project retrieval comparison and Architecture 022 20-occurrence,
  30,371-context-byte deterministic baseline.
- Repeated source-linked entity-alias, temporal-supersession, or cross-document
  multi-hop failures are absent. Smaller existing-lane insufficiency evidence
  is therefore also absent. No comparison prototype is authorized.
- Re-evaluation requires at least ten completed non-fixture current TUI/core-
  loop tasks, the same qualifying failure in at least two distinct tasks with
  exact query/source/context/outcome links, an unsuccessful bounded comparison
  against the smallest applicable existing-lane change, and rerun Architecture
  021/022 baselines.
- `docs/architecture/memory-graphiti-phase-gate.md` consolidates the duplicate
  gate narration while preserving every metric, missing-evidence result,
  re-evaluation trigger, and authority boundary. README, task metadata, and
  durable agent records are current.
- No Go, SQL, dependency, graph/Python implementation, service, database,
  container, schema, adapter, or runtime configuration changed. Canonical
  Go/PostgreSQL ledger and artifact authority remains unchanged.

## Exact Fresh-Session Resume Command

Run this exact command from the repository root. It starts a new pass and does
not resume an old session:

```bash
codex exec -C /home/gernsback/source/revolvr - < /home/gernsback/source/revolvr/.agent/LOOP_PROMPT.md
```

Inside that fresh pass, run the exact read-only selector command
`go run ./cmd/revolvr task list`. The active architecture sequence has no
pending task. Do not reopen Architecture 025, implement a Graphiti prototype,
revive a PTC task, or select the legacy EXT backlog without new authority.

## Verification Results For Architecture 025 Simplify Phase

- `go test ./...` — PASS.
- `git diff --check` — PASS.
- `git diff --name-only` — PASS; documentation and durable metadata only.
- `go run ./cmd/revolvr task list` — PASS before the slice; Architecture 025
  was selected and ready at `phase: simplify`. After the slice it is completed
  and no architecture task is pending.
