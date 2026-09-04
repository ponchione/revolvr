# Codex TUI Draft Task Plan

- Status: Accepted 2026-09-04

This index links the [research baseline](../codex-tui-baseline.md), the
[accepted ordinary launch contract](launch-contract.md), and every task in the
accepted Codex-like TUI sequence. CTUI-001 completed the decision gate;
CTUI-010 is the only canonical pending task. Every later task remains an
accepted unpublished draft.

Repository fresh-pass rules apply: one fresh pass executes one bounded task,
and completion may publish exactly one accepted, dependency-satisfied successor
as described by `AGENTS.md`. The numbered table is also publication order; when
multiple drafts are on the dependency frontier, publish only the earliest
numbered unpublished draft.

## Dependency graph

```text
CTUI-001 <- none
CTUI-010 <- CTUI-001
CTUI-020 <- CTUI-010
CTUI-025 <- CTUI-001
CTUI-030 <- CTUI-020, CTUI-025
CTUI-040 <- CTUI-030
CTUI-045 <- CTUI-040
CTUI-050 <- CTUI-045
CTUI-055 <- CTUI-030
CTUI-060 <- CTUI-055
CTUI-065 <- CTUI-050, CTUI-060
CTUI-070 <- CTUI-065
```

CTUI-010 consumes the accepted launch contract without reopening its route,
stream, field, loading, omission, or ordinary initialized fixture decisions.
Uninitialized and startup-error fixtures remain exclusively CTUI-025 work.

## Drafts

| # | Task | Outcome | Direct blockers |
|---:|---|---|---|
| 1 | [CTUI-001](tasks/ctui-001-lock-launch-contract.md) | Lock initialized launch, fixtures, and I/O contract | None |
| 2 | [CTUI-010](tasks/ctui-010-launch-tui-by-default.md) | Implement shared launch and early TUI ownership | CTUI-001 |
| 3 | [CTUI-020](tasks/ctui-020-match-initial-frame.md) | Implement locked initialized loading and ready frames | CTUI-010 |
| 4 | [CTUI-025](tasks/ctui-025-lock-startup-branch-contracts.md) | Lock startup branch contracts | CTUI-001 |
| 5 | [CTUI-030](tasks/ctui-030-keep-startup-branches-inside-shell.md) | Implement uninitialized and error transitions | CTUI-020, CTUI-025 |
| 6 | [CTUI-040](tasks/ctui-040-prove-terminal-lifecycle.md) | Prove core terminal lifecycle | CTUI-030 |
| 7 | [CTUI-045](tasks/ctui-045-prove-exceptional-render-lifecycle.md) | Prove exceptional cleanup and redraw stability | CTUI-040 |
| 8 | [CTUI-050](tasks/ctui-050-append-post-launch-results.md) | Append one finalized result | CTUI-045 |
| 9 | [CTUI-055](tasks/ctui-055-lock-command-discovery-contract.md) | Lock command discovery | CTUI-030 |
| 10 | [CTUI-060](tasks/ctui-060-match-command-discovery.md) | Implement command discovery | CTUI-055 |
| 11 | [CTUI-065](tasks/ctui-065-lock-first-focused-surface.md) | Lock one focused surface and viewport policy | CTUI-050, CTUI-060 |
| 12 | [CTUI-070](tasks/ctui-070-match-first-overlay.md) | Implement the locked focused surface | CTUI-065 |

Later UI surface tasks require fresh side-by-side evidence rather than invented
fixtures, interactions, or viewport policy. Interactive-child handoff is absent
from this graph because no current Revolvr product path demonstrates it.
