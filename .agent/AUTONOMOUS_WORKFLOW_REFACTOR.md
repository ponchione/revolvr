# Autonomous Workflow Refactor

## Purpose

Revolvr should be an unattended, self-working, self-correcting task harness. An
operator authors task/spec Markdown files, starts Revolvr in the target repo,
and lets fresh Codex sessions plan, implement, verify, audit, correct, document,
simplify, and complete the work without prescribing a fixed phase itinerary.

The intended Codex model is `gpt-5.6-sol` with `xhigh` reasoning effort for
every role. Revolvr should eventually pin or preflight this invariant rather
than silently depending on ambient user configuration.

## Target Control Loop

```text
task/spec -> supervisor -> selected worker -> verification -> audit
                    ^                              |
                    |--------- correction <-------|

clean audit -> supervisor -> optional docs/cleanup/more work -> final gates -> complete
```

The supervisor is a fresh, decision-only Codex session. It reconstructs a task
dossier from durable repository and runtime evidence, emits a structured next
action, and never invokes Codex recursively or edits product code. Revolvr
validates and executes the decision while retaining authority over safety,
verification, commits, retry limits, legal transitions, and completion.

## Ordered Work Program

The original nine refactor steps and the follow-on operability ideas are
expanded into ordered tasks `AW-01` through `AW-31` in `.agent/TASKS.md`. Each
entry contains scope, acceptance, and verification detail suitable for turning
that one entry into a fresh-session prompt. Do not combine entries in one
autonomous pass.

### Foundation and context

1. `AW-01` structured supervisor-decision and audit-finding contracts.
2. `AW-02` durable plan, acceptance-evidence, finding-resolution, and attempt
   state contracts.
3. `AW-03` deterministic task-dossier projection and manifest.
4. `AW-04` repository-backed dossier assembly from task, ledger, receipt, Git,
   and repository guidance sources.
5. `AW-05` explicit model/effort pinning, ephemeral sessions, and provenance.
6. `AW-06` supervisor, planner, and corrector profile seeding.

### Supervisor and autonomous routing

7. `AW-07` fresh, decision-only supervisor execution pass.
8. `AW-08` `autonomous-v1` task lifecycle metadata and compatibility.
9. `AW-09` pure validated routing and completion policy.
10. `AW-10` supervisor-to-worker single-cycle orchestration.
11. `AW-11` durable planning and acceptance-evidence runtime.
12. `AW-12` independent audit and finding-lifecycle persistence.
13. `AW-13` tiered verification and controlled flaky-test classification.
14. `AW-14` verification/audit correction and re-audit loop.
15. `AW-15` retry budgets, no-progress detection, and circuit breakers.
16. `AW-16` conditional documentation and simplification.
17. `AW-17` structured `needs_input` handling.

### Safe unattended execution and scheduling

18. `AW-18` per-task worktree isolation and checkpoint recovery.
19. `AW-19` unattended-execution safety boundary and preflight.
20. `AW-20` transactional terminal finalization and completion capsules.
21. `AW-21` tracked task archives, verification, and reopen support.
22. `AW-22` run-one-task-until-terminal mode.
23. `AW-23` dependency-aware task and child-task scheduling.
24. `AW-24` run-queue-until-exhausted and daemon-safe continuation.

### Efficiency, retention, and operator experience

25. `AW-25` artifact retention, compression, garbage collection, and ledger
    export/replay validation.
26. `AW-26` Git-SHA context caching and role-specific dossier projections.
27. `AW-27` app/CLI plan, findings, acceptance, and “why next” projections.
28. `AW-28` TUI autonomous-workflow visibility and controls.
29. `AW-29` completion, blocker, input, safety-stop, and queue notifications.
30. `AW-30` autonomous-loop metrics and deterministic evaluation scenarios.
31. `AW-31` bounded parallel task workers after isolation and recovery are
    proven safe.

When a fresh session completes one task, it should update durable state and
recommend the next unchecked `AW-*` entry. It may refine a later entry only
when implementation evidence requires it; it must not silently reorder safety
or correctness prerequisites.

## Required Invariants

- Every Codex work or decision pass starts a fresh `codex exec` session.
- Canonical task Markdown remains the operator-authored goal/specification.
- Revolvr, not model prose, enforces legal actions and terminal conditions.
- Implementation and correction require deterministic verification and an
  independent fresh audit before completion.
- Audit findings route back into correction until clean or safely blocked.
- Supervisor/audit no-change decisions are ledger evidence, not forced
  metadata-only source commits.
- Documentation and cleanup run only when evidence says they are relevant.
- Repeated failure or no progress cannot loop forever.
- One blocked task should not stop unrelated tasks when the repository can be
  left demonstrably clean and safe.

## Completion Standard

A task may complete only when its acceptance criteria have evidence, required
verification passes, the latest independent audit is clean, outstanding
findings are resolved or explicitly dispositioned, Git state is safe, and a
validated supervisor decision recommends completion.
