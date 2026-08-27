# TUI-000 — Resolve Codex Source Reuse

- Status: Accepted 2026-08-27; decision-only and not runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: none
- Decision closed: D1

## Outcome

Record one unambiguous policy for how the pinned Codex checkout may influence
the Revolvr TUI implementation.

## Decision

Reimplement the accepted Codex interaction behavior in Revolvr's existing
Go/Bubble Tea TUI. The pinned Codex checkout and its snapshots are behavioral
acceptance evidence only: do not copy, port, vendor, depend on, or distribute
Codex implementation source. Preserve Revolvr branding, domain semantics,
application-service boundaries, and installed terminal dependencies.

This retains ADR-025 unchanged. Because no Codex source material may be copied
or distributed, this task creates no attribution, NOTICE, licensing,
Go/Rust-boundary, or upgrade-ownership follow-up.

## Scope

- Choose behavioral reimplementation or copying/porting source.
- If behavioral reimplementation is selected, retain ADR-025's no-copy
  boundary and classify local Codex files as behavioral acceptance evidence.
- If copying/porting is selected, amend ADR-025 and record Apache-2.0
  attribution, NOTICE, distribution, upgrade ownership, and the Go/Rust
  boundary before implementation is promoted.
- Update D1 and any affected durable decision record with the accepted choice.

## Acceptance

- D1 has one accepted answer and does not contradict ADR-025.
- Every local Codex reference is behavioral evidence, never permitted source
  material.
- Any required license/architecture follow-up is complete, not deferred into a
  TUI implementation task.

## Verification

```bash
git diff --check -- docs/adr .agent/DECISIONS.md docs/architecture/tui-overhaul
rg -n "D1|Codex source|copy|port|reimplement" \
  docs/adr .agent/DECISIONS.md docs/architecture/tui-overhaul
```

## Not Included

- No source copying, code port, dependency, UI change, or snapshot acceptance.
