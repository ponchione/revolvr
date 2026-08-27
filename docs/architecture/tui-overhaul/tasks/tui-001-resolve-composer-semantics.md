# TUI-001 — Resolve Composer and Loop-Input Semantics

- Status: Draft; not canonical or runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: none
- Decisions closed: D2 and D5

## Outcome

Define one app-level contract for every plain-text submission state before the
composer is allowed to dispatch text.

## Scope

- Define idle plain text as exactly one of: task draft, reviewed task
  publication, run instruction, or unavailable.
- Define active plain text as current-process steering, persisted later-pass
  input, or unavailable.
- Preserve typed needs-input option identity and confirmation; define a
  free-form answer only if the domain already supports one explicitly.
- For each accepted action, identify the existing app service or specify one
  separate bounded prerequisite task.
- For queued input, define identity, order, persistence, restart, cancellation,
  editing, consumption, and stale-run behavior. Delete queued behavior from the
  plan if those semantics are not accepted.

## Acceptance

- D2 and D5 have one accepted state table covering idle, active, needs-input,
  uninitialized, and unavailable/error conditions.
- Plain text cannot bypass task review/commit, typed question identity, safety,
  scheduler, operation guards, or fresh-session rules.
- Every accepted durable effect names its app/domain authority and recovery
  behavior.
- Missing domain work is a separate prerequisite, not TUI-031 scope.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
rg -n "D2|D5|idle input|active input|needs-input|queued" docs/architecture/tui-overhaul
```

## Not Included

- No composer code, queue implementation, task publication, or typed-answer
  behavior change.
