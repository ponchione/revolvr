# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, record the blocker and stop.
- Add only small, specific, directly discovered follow-up tasks.
- Do not invent broad roadmap work.

## Current Backlog

None.

## Completed Programs

- [x] AW-01 through AW-31 — autonomous workflow contracts, execution,
  persistence, safety, worktree isolation, finalization, archives, queues,
  retention, evidence views, notifications, metrics, and bounded parallelism.
- [x] AUD-01 through AUD-16 — wide-sweep correctness, persistence, process,
  path, configuration, Git, ledger, and cleanup hardening.
- [x] R2-01 through R2-11 — second audit closure for logical ledger authority,
  exclusion, immutable recovery, protected runtime paths, durability, replay,
  CLI, and configuration contracts.
- [x] TS-01 through TS-04 — one shared deterministic scheduler across mixed,
  autonomous, app, CLI, TUI, queue, and run-once surfaces.
- [x] FM-01 — strict canonical frontmatter with inert `x-` extensions.
- [x] OC-01 through OC-02 — operator-checkpoint receipt authority,
  replay-safe fulfillment, scheduling, and visibility.
- [x] AM-01 through AM-02 — deterministic autonomous migration planning,
  atomic application, and restart recovery.
- [x] QA-01 — cross-surface regression closure, operator documentation, and
  read-only Cyber-ARPG readiness assessment.

Detailed behavior is captured in `.agent/DECISIONS.md`; current verification
and readiness evidence is summarized in `.agent/STATE.md`; implementation and
test history is preserved in Git.

## Blocked

None.
