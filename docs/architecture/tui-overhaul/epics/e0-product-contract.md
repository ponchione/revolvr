# E0 — Settle the Product Contract

- Status: Accepted 2026-08-27; planning record only
- Design authority: [TUI overhaul](../README.md)
- Depends on: none

## Outcome

The operator experience and implementation boundaries are explicit enough that
the terminal-shell proof can be judged without inventing product semantics.
D1-D6 and the TUI-005 source snapshots are accepted; E0 has exited without
publishing or starting an implementation task.

## Tasks

| Task | Single responsibility | Depends on |
| --- | --- | --- |
| [TUI-000](../tasks/tui-000-resolve-source-reuse.md) | resolve Codex source reuse | none |
| [TUI-001](../tasks/tui-001-resolve-composer-semantics.md) | resolve plain-text and loop-input semantics | none |
| [TUI-002](../tasks/tui-002-choose-transcript-ownership.md) | choose transcript and scrollback ownership | none |
| [TUI-003](../tasks/tui-003-accept-overlay-migration.md) | accept the overlay migration order | TUI-002 |
| [TUI-004](../tasks/tui-004-accept-session-header.md) | accept the session-header lifecycle | TUI-002 |
| [TUI-005](../tasks/tui-005-accept-experience-states.md) | accept experience-state snapshots | TUI-000–TUI-004 |

## Boundaries

E0 changes planning, decision, and ADR documents only. It creates no product
code, runtime dependency, canonical task, or domain behavior.

## Exit Gate

- **Passed 2026-08-27.** D1-D6 have accepted answers with no contradiction
  against ADR-025.
- Every required state has an accepted source snapshot with exclusive
  presentation ownership and 80-/40-column geometry.
- No missing app/domain capability or prerequisite task was discovered.
- TUI-010 was the only implementation task eligible for publication. It is now
  published as pending and remains unstarted.
