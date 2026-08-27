# TUI-002 — Choose Transcript and Scrollback Ownership

- Status: Draft; not canonical or runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: none
- Decision closed: D3

## Outcome

Choose which layer owns committed history, active replacement, scrolling, and
reflow so the shell proof has one testable architecture.

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

- D3 selects one approach and rejects the other candidates with concise reasons.
- Each history-related state has one owner; no behavior depends on timestamp or
  rendered-prose matching.
- Resize, replay, scroll/copy, active replacement, and non-TTY output have
  explicit expected behavior.
- The proof scope is bounded to current dependencies and TUI files/tests.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n "D3|scrollback|viewport|reflow|owner" docs/architecture/tui-overhaul
```

## Not Included

- No proof implementation, transcript cell vocabulary, overlay migration, or
  general terminal backend.
