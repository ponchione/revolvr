# Architecture 025: memory and Graphiti evidence gate

Date: 2026-08-27

Decision: **defer** Graphiti and do not authorize a comparison prototype.

## Gate review

`go run ./cmd/revolvr task list` selected the ready Architecture 025 document
phase after its Architecture 024 dependency completed.

- **Current-brain usage — absent.** The checkout has 22 completed run
  directories and matching receipts dated from `2026-07-08T14:42:14Z` through
  `2026-07-09T11:17:14Z`. `revolvr status` reports latest run
  `019f4692-ad3c-7cc1-a7a8-910e3f276000` and a capped 20-run recent projection.
  Every run predates Architecture 021 commit `9fd0de6` and Architecture 024 TUI
  commit `2a4dc6f`, leaving zero real runs through the current brain or TUI.
  The TUI projects canonical events but creates no separate session evidence,
  so these runs also cannot establish use of its current transcript, focused
  evidence, or context-assembly surfaces.
- **Existing-lane baselines — supported.** The Architecture 021 fixture freezes
  25 judged queries across three projects, 22 files, 337 chunks, and 239
  symbols. It records lexical Recall@5/10 `0.84/0.96`, selected-Qwen vector
  Recall@5/10 `0.96/0.96`, hybrid Recall@5/10 `0.96/1.00`, hybrid MRR `0.9444`,
  exact-symbol preservation `1.00`, and hybrid p95 `23.80 ms`. The Architecture
  022 golden records 20 deterministic `direct_tools_v1` occurrences, 30,371
  context bytes (`1,464` minimum, `1,575` maximum), 62 tool uses, six repeated
  reads, 13 verification executions, one exact reuse, four correction cycles,
  and five retrieval fixtures with Recall@5/10, MRR, and exact-symbol
  preservation `1.00`. Its quality threshold remains unset and live dogfood is
  explicitly omitted, so it is regression evidence rather than real usage.
- **Repeated source-linked failures — absent.** Repository and runtime searches
  found no entity-alias, temporal-supersession, or cross-document multi-hop
  retrieval failure. Those categories appear only in this gate; each
  Architecture 021 fixture expects one source target, and the 22 old runs'
  prompts, receipts, last messages, and stderr record no qualifying failure.
- **Smaller-fix insufficiency — absent.** With no qualifying failure, no smaller
  fix has been attempted. Exact path/symbol/text, one-hop structural, FTS,
  vector, typed-relation, ranking, and context-budget lanes have not been shown
  insufficient for a concrete case.

The three absent gates require defer; no comparison prototype is authorized.

## Exact re-evaluation trigger

Re-evaluate only when one repository-owned evidence set contains all of the
following:

1. At least ten completed, non-fixture real tasks run through the current
   TUI/core loop at or after source commit `2a4dc6f`, each binding its exact
   task, run, source revision, retrieval query/results, context-package or
   dossier identity, and typed outcome.
2. At least two of those runs, across distinct tasks, record the same concrete
   failure involving entity aliases, temporal supersession, or cross-document
   multi-hop retrieval. Each occurrence must link the query, expected sources,
   returned sources/context, and downstream outcome.
3. A bounded comparison against the smallest applicable existing-lane change
   demonstrates that exact path/symbol/text, typed relational data,
   structural/FTS/vector ranking, or context assembly cannot resolve those
   same cases acceptably.
4. The Architecture 021 retrieval metrics and Architecture 022 context metrics
   are rerun as the unchanged baseline for the evidence set.

Only then may a new task authorize a separate bounded comparison prototype.
Any future graph projection remains optional, derived, removable, and
subordinate to the canonical Go/PostgreSQL ledger, artifacts, verification,
audit, and completion authority.
