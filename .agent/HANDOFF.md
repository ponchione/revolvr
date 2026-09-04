# Agent Handoff

Updated: 2026-09-04

## Pause Point

- The old canonical task inventory and the old transcript-first TUI plan have
  been retired.
- The Codex-like replacement plan is accepted in
  `docs/architecture/codex-tui/README.md` and grounded in
  `docs/architecture/codex-tui-baseline.md`.
- `ctui-001-lock-launch-contract` is the only canonical pending task. It is a
  documentation-only decision task; no replacement UI implementation has
  started.
- The next pass must be a fresh thread and execute CTUI-001 only.

## Read-Only Selector

Run:

```bash
go run ./cmd/revolvr status
```

The expected selector is `ctui-001-lock-launch-contract`. Historical run
records may still appear; they are local runtime history, not selectable tasks.

## Resume Rule

- Read `AGENTS.md`, the canonical task, the accepted plan, the research
  baseline, and the durable state files before changing anything.
- Capture fresh ordinary initialized Codex and Revolvr launch evidence at
  80x24 and one named narrow geometry. Lock the literal loading/ready fixtures,
  field mapping, and CLI/TTY/redirected-I/O matrix.
- Change documentation and task metadata only. Do not implement UI behavior,
  add a dependency, revive the retired plan, or execute another task.
- After CTUI-001 is terminally complete and verified, publish CTUI-010 as the
  only next pending task and stop without implementing it.
