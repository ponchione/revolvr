# TUI-004 — Accept the Session-Header Lifecycle

- Status: Accepted 2026-08-27; decision-only and not runnable
- Epic: [E0 — Settle the product contract](../epics/e0-product-contract.md)
- Depends on: [TUI-002](tui-002-choose-transcript-ownership.md)
- Decision closed: D6

## Outcome

Define when the Revolvr session identity appears and which current-context
facts remain outside committed history.

## Decision

Use the startup-only lifecycle recorded in
[D6](../README.md#d6--session-header-lifecycle). Each TUI process commits one
`session-start` cell before its bounded canonical-history replay. The cell owns
only the package-local `Revolvr` label, the inspected absolute repository root,
and the initial `app.StatusResult.Initialized` value. `app.Status` carries the
root returned by `repositorypath.Inspect(...).Root()` as
`app.StatusResult.ProjectRoot`; the TUI does not derive identity from its
ambient directory, a Git remote, or a basename.

Refresh, resize, and overlay open/dismiss retain the same source and emitted
identity and cannot append another session cell. Restart begins a new process-
local emitted set, so it emits one new `session-start` and then replays the
bounded canonical window once. The state fact is explicitly the state at
process start; fresh status still governs current command guards without
rewriting terminal history.

Reject an explicit Revolvr clear action in this overhaul. No clear key,
command, callback, or presentation epoch exists, and an operator clearing
terminal-owned scrollback outside Revolvr causes no application replay. A
future owned clear needs its own bounded decision and task.

Committed transcript, live cell, overlay, composer, and transient-footer facts
have the exclusive owners and sources in D6. No current view, readiness/safety
claim, active operation, task/run identity, workflow, timestamp, version,
count, command hint, or error belongs in the session cell.

The session cell uses typed process-local identity, not time or prose. Startup
inspection/status failure emits nothing; refresh failure retains the last good
projection and reports transiently; Bubble Tea output failure follows normal
program failure/restoration and is never reported as a successful header.

TUI-010, TUI-011, TUI-013, TUI-060, TUI-062, and TUI-070 must prove startup
ordering, refresh/resize/overlay deduplication, restart replay, geometry,
failure restoration, and absence of duplicate persistent chrome before the old
header is removable.

## Scope

- Accept or replace the proposed one-time session transcript cell.
- Define startup, explicit clear, refresh, resize, restart, and overlay behavior
  for the cell.
- Specify its exact source facts, including initialization and project identity.
- Assign active-only context to the live/footer region so it is not duplicated
  in session history.

## Acceptance

- D6 has one lifecycle with no persistent duplicate dashboard header.
- Refresh and resize do not create another session cell.
- Restart emits one new process session cell; explicit Revolvr clear is
  intentionally unsupported and external terminal clearing emits nothing.
- Every displayed fact has one source and one presentation owner.

## Verification

```bash
git diff --check -- docs/architecture/tui-overhaul
rg -n "D6|session header|session cell|session-start|explicit clear" \
  docs/architecture/tui-overhaul
```

## Not Included

- No shell code, general transcript cells, footer redesign, or clear command.
