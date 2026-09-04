---
id: ctui-020-match-initial-frame
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 3
depends_on: ctui-010-launch-tui-by-default
---

# CTUI-020 — Match Initialized Loading and Ready Frames

- Status: Pending
- Accepted plan: [Codex TUI task plan](../../docs/architecture/codex-tui/README.md)
- Accepted draft: [CTUI-020](../../docs/architecture/codex-tui/tasks/ctui-020-match-initial-frame.md)
- Authoritative contract: [Ordinary initialized launch contract](../../docs/architecture/codex-tui/launch-contract.md)
- Depends on: CTUI-010 (completed)
- Completion handoff: publish only CTUI-025; do not execute it in the same pass

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
