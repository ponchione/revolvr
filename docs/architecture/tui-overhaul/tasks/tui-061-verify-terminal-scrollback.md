# TUI-061 — Verify Real-Terminal Scrollback

- Status: Draft; not canonical or runnable
- Epic: [E6 — Harden terminal behavior](../epics/e6-terminal-hardening.md)
- Depends on: [TUI-060](tui-060-lock-geometry-snapshots.md)

## Outcome

Record whether committed transcript history can be navigated, selected, and
copied as designed in each supported real-terminal environment.

## Scope

- Run the accepted history scenario in one plain terminal and tmux.
- Add SSH or another environment only if support is explicit and the test is
  reproducible.
- Verify upward navigation, selection/copy, wrapped-line copying, large-history
  navigation, live-cell replacement, and return from overlays.
- Record terminal names/versions, exact commands, D3 mode, observations, and
  limitations in a repository-owned result document.
- Make only focused fixes required by the accepted supported matrix.

## Acceptance

- Plain-terminal and tmux results are recorded for every scoped behavior.
- Committed history is copyable without duplicate intermediate live states.
- Any terminal-owned reflow limitation matches the accepted D3 contract.
- Each limitation has an operator workaround or explicit unsupported decision.
- Results make no untested claim about SSH or other terminals.

## Verification

```bash
go test ./internal/tui
go run ./cmd/revolvr tui
tmux new-session 'go run ./cmd/revolvr tui'
git diff --check -- docs internal/tui
```

## Not Included

- No process-signal/restoration matrix, broad terminal emulation, or support
  claim for an unreproduced environment.
