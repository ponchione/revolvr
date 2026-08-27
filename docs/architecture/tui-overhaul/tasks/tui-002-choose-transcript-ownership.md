# TUI-002 — Choose Transcript and Scrollback Ownership

- Status: Accepted 2026-08-27; decision-only and not runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: none
- Decision closed: D3

## Outcome

Choose which layer owns committed history, active replacement, scrolling, and
reflow so the shell proof has one testable architecture.

## Decision

Use the bounded hybrid recorded in
[D3](../README.md#d3--transcript-and-scrollback-ownership):

- existing app/domain projections remain canonical;
- `internal/tui` retains bounded semantic source cells, one replaceable live
  cell, composer state, overlay state, and a process-local emitted-identity set;
- finalized cell renderings are appended once to normal-screen history through
  the installed Bubble Tea `tea.Println` boundary;
- the terminal or multiplexer owns already-emitted rows, scrolling, selection,
  copying, and any native soft reflow;
- the TUI redraws only the live/composer/overlay frame and never clears or
  reinserts terminal history on resize.

Reject application-driven terminal-history reflow because it needs an
unproven escape/terminal layer. Reject viewport-owned committed history because
it preserves the current application-scrolling limitation instead of making
history natively selectable and copyable.

No app/domain prerequisite, terminal abstraction, dependency, or escape layer
is accepted. TUI-010, TUI-011, and TUI-012 must prove composition, managed-frame
reflow, and settlement before TUI-013 installs the shell; TUI-061 and TUI-062
retain the real-terminal scrollback and restoration matrices.

## Scope

- Compare terminal-native scrollback, viewport-owned history, and the documented
  hybrid against Revolvr's current Bubble Tea program and IO contract.
- Assign ownership for committed source cells, rendered lines, active-cell
  replacement, scroll position, resize reflow, refresh/restart reconstruction,
  copy/paste, and overlay return.
- Define the supported minimum terminal behaviors for plain terminals, tmux,
  and test output buffers.
- Record the exact uncertainty TUI-010 and TUI-011 must prove.

## Acceptance

- D3 selects the bounded hybrid and rejects the other candidates with concise
  reasons.
- The D3 table assigns every history-related state one explicit owner; no
  behavior depends on timestamp or rendered-prose matching.
- Resize, replay, scroll/copy, active replacement, and non-TTY output have
  explicit expected behavior and proof obligations.
- The proof scope stays inside current dependencies and TUI files/tests.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
rg -n "D3|scrollback|viewport|reflow|owner|committed|live cell|tmux|non-TTY" \
  docs/architecture/tui-overhaul .agent/TASKS.md .agent/DECISIONS.md
```

## Not Included

- No proof implementation, transcript cell vocabulary, overlay migration, or
  general terminal backend.
