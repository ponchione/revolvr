# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, record the blocker and stop.
- Add only small, specific, directly discovered follow-up tasks.
- Do not invent broad roadmap work.

## Current Backlog

- [x] AUDIT-FIX-01 — Settle source-lease monitor failures before terminal task,
  receipt, and ledger mutation in mixed and autonomous runs; make the race tests
  deterministic.
- [x] AUDIT-FIX-02 — Remove unsafe mixed-pass dirty-worktree commits or isolate
  and prove the exact run-owned delta with real-Git overlap coverage.
- [x] AUDIT-FIX-03 — Harden source-writer and retention lock files with
  canonical roots, no-follow opens, and named/opened inode checks.
- [x] AUDIT-FIX-04 — Migrate the remaining predictable coordinator locks to the
  same hardened flock primitive and substitution tests.
- [x] AUDIT-FIX-05 — Open all app read projections through the live read-only
  ledger API and prove byte/sidecar immutability.
- [x] AUDIT-FIX-06 — Declare and enforce the supported-platform contract with a
  matching cross-build matrix.
- [x] AUDIT-FIX-07 — Repair and rerun the stale local CLI smoke-test header
  assertion.
- [x] AUDIT-CLOSE-01 — Re-audit every `AUDIT_PROBLEMS.md` finding against the
  committed fixes, run the final verification matrix, and delete the audit
  file only if all findings are resolved.
- [x] AUDIT-R3-00 — Conduct an independent wide-sweep audit and record only
  reproduced or directly evidenced findings in `AUDIT_PROBLEMS.md`.
- [x] AUDIT-R3-01 — Settle runner process groups after successful leader exit
  and prove that no descendant can mutate after return.
- [x] AUDIT-R3-02 — Make initialization and task-directory creation no-follow
  and identity-safe without breaking legitimate linked worktrees.
- [x] AUDIT-R3-03 — Retain SQLite busy evidence across live-reader retries and
  make the cancellation regression deterministic.
- [x] AUDIT-R3-04 — Make task-import and receipt structural parsing ignore
  headings inside Markdown fences.
- [x] AUDIT-R3-05 — Make Git object-ID validation consistent for SHA-1 and
  SHA-256 repositories across workspace, cache, map, and dossier boundaries.
- [x] AUDIT-R3-06 — Accept safe tracked names beginning with `..` while still
  rejecting path traversal.
- [x] AUDIT-R3-07 — Replace map-order-dependent validation diagnostics with
  explicit deterministic ordering.
- [x] AUDIT-R3-08 — Remove the confirmed no-caller wrappers and obsolete
  admitted-cycle orchestration path if it is not intentionally reserved.
- [x] AUDIT-R3-CLOSE-01 — Re-audit AP-01 through AP-08 against committed
  source and regressions, run the final verification matrix, and delete
  `AUDIT_PROBLEMS.md` only when every finding is proven closed.
- [x] AUDIT-R4-00 — Conduct a fresh wide-sweep audit and record only
  reproduced or directly evidenced findings in `AUDIT_PROBLEMS.md`.
- [x] AUDIT-R4-01 — Make the shared runtime-path boundary descriptor-rooted
  across ancestor substitution and migrate canonical autonomous-state
  persistence with the reproduced outside-rename regression.
- [x] AUDIT-R4-02 — Migrate notification intent, payload, history, journal,
  cleanup, sync, and lease checks to the stable runtime-path boundary.
- [x] AUDIT-R4-03 — Repair task-run protected reads and bind history/checkpoint
  publication and cleanup to stable parent and file identities.
- [ ] AUDIT-R4-04 — Migrate autonomous archive immutable/mutable storage and
  removal away from its bespoke check-then-use path helpers.
- [ ] AUDIT-R4-05 — Bind autonomous finalization artifact publication and
  readback to stable parent, temporary-file, and destination identities.
- [ ] AUDIT-R4-06 — Inventory and migrate the remaining authoritative
  `Lstat`-then-by-name evidence readers identified by AP-01.
- [ ] AUDIT-R4-07 — Poll for bounded process-group settlement after `SIGKILL`
  and close cancellation/identity-reuse races in the runner.
- [ ] AUDIT-R4-08 — Make active TUI quit wait for the matching run, loop,
  task-run, or queue terminal event before exiting.
- [ ] AUDIT-R4-09 — Replace recursive map-order Codex usage selection with
  schema precedence and fail-closed ambiguity handling.
- [ ] AUDIT-R4-10 — Bind source-snapshot bytes to the opened file identity and
  reject regular-file and symlink ABA substitutions.
- [ ] AUDIT-R4-11 — Make action-budget and archive-file first-error diagnostics
  deterministic under multiple simultaneous failures.

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
- [x] AUDIT-2026-07-14 — evidence-based wide-sweep audit recorded in
  `AUDIT_PROBLEMS.md`; six problems found and converted into bounded follow-up
  tasks.

Detailed behavior is captured in `.agent/DECISIONS.md`; current verification
and readiness evidence is summarized in `.agent/STATE.md`; implementation and
test history is preserved in Git.

## Blocked

None.
