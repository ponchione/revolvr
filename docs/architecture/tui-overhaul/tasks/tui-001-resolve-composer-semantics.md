# TUI-001 — Resolve Composer and Loop-Input Semantics

- Status: Accepted 2026-08-27; decision-only and not runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: none
- Decisions closed: D2 and D5

## Outcome

Record one app-level contract for every plain-text submission state before the
composer is allowed to dispatch text.

## Decision

- Initialized idle plain text opens the existing Add Task review with the text
  prefilled. The draft is ephemeral; only explicit confirmation in that review
  calls `app.AddTaskAndCommit`, and Escape cancels without a durable effect.
- Uninitialized, active-operation, unavailable, and error states reject plain
  text, preserve the composer buffer, explain the refusal, and call no app
  service. Whitespace-only input does nothing.
- One-pass runs, bounded loops, autonomous task runs, and autonomous task
  queues accept no current or deferred steering. A composer buffer is not a
  queued item and is never persisted, replayed, or auto-submitted.
- Typed needs-input remains an exclusive option-based overlay. It retains exact
  task, question, revision, content SHA-256, option, and confirmation identity
  through `app.AnswerAutonomousInput`; free-form answers are unavailable.

The complete accepted state table and rationale are in
[D2](../README.md#d2--plain-text-composer-meaning) and
[D5](../README.md#d5--loop-and-queued-input-semantics).

## Scope

- Resolve idle, active, uninitialized, needs-input, unavailable, and error
  submission semantics against current app/domain authority.
- Preserve task review/commit, operation guards, fresh-session execution,
  scheduler authority, and typed question identity.
- Remove unsupported queued-message behavior from the implementation plan.
- Update the design authority, evidence mapping, task graph, and durable state.

## Acceptance

- D2 and D5 have one accepted state table covering idle, active one-pass,
  active loop/task/queue, needs-input, uninitialized, and unavailable/error
  conditions.
- Plain text cannot bypass task review/commit, typed question identity, safety,
  scheduler, operation guards, or fresh-session rules.
- Every accepted durable effect names its app/domain authority and recovery
  behavior.
- No app/domain prerequisite is required; TUI-031 is narrowed to the existing
  reviewed idle task-draft path and unsupported TUI-041 is removed.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
rg -n "D2|D5|idle input|active input|needs-input|queued|plain text" docs/architecture/tui-overhaul
```

## Not Included

- No composer code, queue implementation, task publication, typed-answer
  behavior change, or new application service.
