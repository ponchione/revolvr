# CTUI-020 — Match Initialized Loading and Ready Frames

- Status: Accepted 2026-09-04; unpublished draft
- Source: [Codex TUI baseline](../../codex-tui-baseline.md)
- Depends on: CTUI-010

## Outcome

Implement the exact ordinary initialized loading-to-ready composition with one
losslessly transferred editable draft.

## Scope

- Implement the literal ordinary initialized loading and ready fixtures and
  field mappings already locked by CTUI-001 at 80x24 and its named narrow
  geometry.
- Allow draft editing while loading, gate all unavailable actions, and transfer
  text, cursor, and focus losslessly into ready state.
- Keep ordinary launch inline and free of startup transcript replay.

## Acceptance

- Deterministic 80x24 and named-narrow snapshots match both locked fixtures.
- Delayed startup remains responsive to edits, rejects gated actions, and
  preserves draft text, cursor, and focus exactly once when ready.
- No startup history or alternate-screen transition appears in PTY output.
- The implementation and its focused evidence fit one fresh pass.

## Verification

- Run loading/ready snapshots at both locked geometries and delayed-bootstrap
  transition tests.
- Capture the full loading-to-ready transition in an 80x24 PTY.
- Run `go test ./...`.

## Not Included

- Uninitialized or startup-error presentation, byte-level restoration,
  command discovery, transcripts, focused surfaces, or new dependencies.
- New fixture, field-mapping, or launch-policy decisions.
- Requirements from the retired TUI plan.
