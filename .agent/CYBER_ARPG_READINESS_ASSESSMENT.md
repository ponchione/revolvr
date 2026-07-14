# Cyber-ARPG Readiness Assessment

Assessment date: 2026-07-14

Target: `/home/gernsback/source/cyber-arpg`

## Boundary and canonical discovery

This was a read-only assessment. It started no Codex process, selected no task
for execution, fulfilled no checkpoint, applied no migration, and wrote no
file in the target repository. The assessment binary was built from this
Revolvr worktree at `/tmp/revolvr-qa`; captures and an independently initialized
copy of the task directory were written only under `/tmp`.

The target's canonical `.agent/tasks` path is a pre-existing symlink to
`../tasks`. Its resolved target is `/home/gernsback/source/cyber-arpg/tasks`,
which remains inside the repository root. Revolvr's canonical path guard
accepted that layout and strictly loaded every Markdown task. A plain `find`
that does not follow symlinks can misleadingly report no files under
`.agent/tasks`; the canonical loader is the authoritative discovery check.

The target also had pre-existing ignored `.revolvr` state dated 2026-07-13.
Read-only status and TUI projection left both its file hashes and its path,
type, size, and modification-time listing byte-identical.

## Strict load and graph result

| Check | Result |
| --- | ---: |
| Canonical tasks loaded | 446 |
| Pending `mixed-pass-v1` tasks | 446 |
| Dependency edges | 1,113 |
| Missing task IDs / dependency targets | 0 |
| Duplicate task IDs | 0 |
| Duplicate dependency edges | 0 |
| Self-dependency edges | 0 |
| Dependency cycles | 0 |
| Terminal-unsatisfied edges | 0 |
| Ready tasks | 2 |
| Waiting on dependencies | 444 |
| Scheduler diagnostic rows | 0 |

Strict canonical loading would have failed on malformed frontmatter or invalid
list syntax. The shared graph projection would have emitted a diagnostic and
selected nothing for missing or duplicate IDs/edges, self-edges, or cycles.
The successful 446-row projection with no diagnostics therefore establishes
the zero counts above; the readiness projection separately establishes that
there are no terminal-unsatisfied edges.

The deterministic ready order is:

1. `m0-architecture-dependency-guard` — priority 10, selected next for
   `mixed-pass-v1`.
2. `m0-content-id-types` — priority 80, ready but not selected.

The selected next task is therefore
`m0-architecture-dependency-guard` (`mixed-pass-v1`, `implement`,
`implementer`). No file-order fallback was involved.

## Read-surface agreement

The target's real `task list` and `status` commands both selected
`m0-architecture-dependency-guard`. A bounded terminal capture of the real TUI
rendered the same next-task ID. The task-list output was also reproduced
byte-for-byte from an independently initialized `/tmp` copy of the exact 446
task files; target and copy both had SHA-256
`f12dfec4cf931e9bdffec9a7712855f71bcd458b7ba72d67011127c7b2155944`.
Target and copy status output likewise matched byte-for-byte with SHA-256
`f922a8c15a641ac54d09545085e6af1aa6a146b6503bd33f2d04f7cbf0cda253`.

No run command was issued against Cyber-ARPG. Revolvr's focused cross-surface
regression uses a fake Codex runner to prove that task list, status, TUI, and
the real one-pass selector consume the same selected identity, while the
invalid-graph regression proves that a runner is not called when the shared
graph is invalid.

## Autonomous migration dry-run

The only migration command was:

```bash
/tmp/revolvr-qa task migrate --to autonomous-v1 --all --dry-run
```

It produced schema `autonomous-migration-plan-v1`, exactly 446 deterministic
projection rows, and the explicit result `no files written`. Output SHA-256 was
`770baee074fa37b4fd684396fa60ff3b8bad57e2f52e910059fff213d6864cf9`.
No apply command was run.

All tasks are structurally eligible because they are pending mixed-pass tasks
at the implement phase with no autonomous lineage or state. That is not an
operational reason to migrate them. This is a dependency-heavy campaign—444
of 446 tasks currently wait on another task—so it should remain
`mixed-pass-v1` unless an independent, task-specific reason justifies the
supervisor/worker autonomous lifecycle. A bulk dry-run is useful readiness
evidence, not approval to bulk apply.

## Recorded commands and unchanged identities

The assessment used these non-model commands from the target root:

```bash
/tmp/revolvr-qa task list
/tmp/revolvr-qa status
/tmp/revolvr-qa tui             # bounded capture; no run key was sent
/tmp/revolvr-qa task migrate --to autonomous-v1 --all --dry-run
git rev-parse HEAD
git status --porcelain=v1 -z --untracked-files=all
find tasks -maxdepth 1 -type f -name '*.md'
```

Before/after target identities were unchanged:

- HEAD: `20f9adee81b95aa60b3bac81a851a63686e39170`
- exact NUL-delimited Git status SHA-256:
  `d4c2c9b04b008f47f2d75b8d95c57bab2dc7be9711aa5c95ede5035c22976398`
- ordered task-content manifest SHA-256:
  `180f1d3f4e2572f6d495ee924e9c73f17e79729be6b046da24f49cb3f2f13910`
- ordered task-name manifest SHA-256:
  `4431f1bd772a814ea64d9e15a739a29ec1cafa728d2631258341df3e38e9aa97`
- task count: 446

The target was already dirty before assessment (including the untracked task
campaign); this pass neither cleaned nor changed that state.
