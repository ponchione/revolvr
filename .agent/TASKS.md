# Agent Tasks

Updated: 2026-09-04

## Active Codex TUI Sequence

- The accepted task graph and publication order are in
  `docs/architecture/codex-tui/README.md`.
- `ctui-001-lock-launch-contract` is the only canonical pending task.
- CTUI-010 through CTUI-070 remain accepted unpublished drafts. Publish them
  one at a time in numbered order only after the current task completes and
  its direct dependencies are satisfied.
- The former architecture, PTC, EXT, and transcript-first TUI task inventories
  remain retired. Git history is their archive; do not recover or republish
  them.

## Rules

- Execute exactly one canonical task per fresh pass.
- Mark it complete only after its acceptance and verification pass.
- A completion handoff may publish exactly one accepted, dependency-satisfied
  successor; it must not execute that successor in the same pass.
- Do not copy, port, vendor, embed, or depend on Codex source.

## Current Work

- [ ] CTUI-001 — lock the ordinary initialized launch contract from fresh
  fixed-geometry evidence. This is documentation-only and changes no product
  code or dependency.
- Next after verified completion: publish CTUI-010 only.
