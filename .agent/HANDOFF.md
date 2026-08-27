# Agent Handoff

Updated: 2026-08-27

## Where We Stopped

- Added `/.reference/` to the root ignore rules and shallow-cloned the official
  `openai/codex` repository to `.reference/codex` at
  `8228e9b867251f544a5e0c6c80bb5ebc9d5446a1`; the reference TUI is in
  `codex-rs/tui`.
- Fixed TUI task creation without weakening preflight: it refuses a dirty
  worktree, creates the canonical task, and path-scoped commits only that new
  file. CLI task add/import remains review-and-commit.
- Added a real-Git regression proving the new task is the only committed path,
  the worktree is clean, and attended preflight admission succeeds.
- Collapsed the dashboard into a transcript-first operator console: one-line
  header, latest-run operator status plus semantic timeline, concise idle and
  uninitialized states, persistent `› / for commands` affordance, and a
  two-line-at-most narrow dashboard footer with active cancellation visible.
- Removed dashboard task counts, latest/recent run summaries, and raw events;
  those projections remain in Tasks, Runs, Run Detail, Workflow, and Help.
- Compacted dashboard activity to the latest eight meaningful semantic
  transitions, omitting low-level Codex lifecycle noise and the duplicated
  full task body; Run Detail retains the complete timeline.
- Removed redundant Transcript/Activity labels and dashboard UUID rows, and
  shortened file lists, commit hashes, receipt paths, and warning prefixes so
  the 80-column operator view remains readable without wrapped log walls.
- Simplified shared terminal styles to default/bold/dim text plus ANSI cyan,
  green, and red. Keyboard actions, focused views, scrolling, refresh,
  commands, typed responses, safety state, and cancellation are unchanged.
- Decomposed the draft TUI overhaul plan under
  `docs/architecture/tui-overhaul/`: `README.md` holds the product/design
  authority and index, `epics/` has one file for each of eight epics, and
  `tasks/` now has 33 audited, bounded task files after D5 removed TUI-041.
- Split composite work into separate product decisions, shell resize and
  settlement proofs, independently reachable focused-view migrations,
  scrollback and terminal-lifecycle checks, and documentation and final
  closeout tasks. Every task states dependencies, scope, acceptance,
  verification, and exclusions.
- The draft does not publish runnable `.agent/tasks`. TUI-000 and D1 are
  accepted: Revolvr will reimplement the accepted Codex interaction behavior
  in its existing Go/Bubble Tea TUI; the pinned Codex source and snapshots are
  behavioral evidence only, and no implementation source may be copied or
  ported. ADR-025 remains unchanged.
- Completed the pinned Codex TUI behavioral study and added exactly four
  evidence documents under `docs/architecture/tui-overhaul/reference/`:
  the reference index/task lookup, interaction model, terminal mechanics, and
  current Revolvr mapping. The consumed study prompt was removed.
- The references trace shell ownership, transcript settlement/replay, composer
  and interrupt behavior, overlays/questions/approvals, terminal
  scrollback/reflow/restoration, styles, tests, snapshots, and current Go seams.
  Every retained TUI-010 through TUI-072 proof/implementation task has a
  relevant link or an explicit no-useful-analog entry.
- TUI-001 is accepted and closes D2/D5. Initialized idle plain text opens the
  existing reviewed Add Task draft; uninitialized, active, and unavailable
  text rejects without an app effect; typed needs-input remains exact option-
  only input. Active steering and queued/deferred operator messages are not
  supported.
- TUI-031 is narrowed to the reviewed idle task-draft route, TUI-041 is removed,
  and no app/domain prerequisite was added. D3 is accepted as a bounded hybrid:
  `internal/tui` owns semantic source/live state, finalized identities append
  once through installed `tea.Println`, terminal history owns emitted rows and
  native reflow/copy, and viewports are overlay-only. D4 is accepted as the
  strict Help, Tasks, Runs/Run Detail, Preflight, Workflow, Change Summary,
  Evidence, Approval, typed-needs-input migration order.
- Committed the task-publication fix, console presentation/test update,
  planning tree, behavioral study, and durable-state snapshot as `24be655`
  (`feat: prepare transcript-first TUI overhaul`) and pushed `main` to
  `origin/main` with raw Git.
- Consumed and removed `docs/architecture/tui-overhaul/TUI_001_DECISION_PROMPT.md`.
  No product code, runnable task, or commit changed in the TUI-001 pass.
- Committed and pushed the accepted TUI-001 decision as `f4e7ebf`
  (`docs: resolve TUI composer semantics`) with raw Git.
- Created `docs/architecture/tui-overhaul/TUI_002_DECISION_PROMPT.md` as the
  only next-task prompt. It constrains the next fresh pass to the D3 ownership
  decision and deletes itself when consumed.
- TUI-002 is accepted and closes D3. Terminal-native application reflow and
  viewport-owned committed history are rejected. TUI-010/TUI-011/TUI-012 retain
  the append, managed-reflow, and settlement proofs; TUI-061/TUI-062 retain the
  real-terminal scrollback and lifecycle matrices. No product code, dependency,
  app/domain prerequisite, runnable task, terminal backend, or escape layer was
  added.
- Consumed and removed `docs/architecture/tui-overhaul/TUI_002_DECISION_PROMPT.md`.
  No product code or runnable task changed in the TUI-002 pass.
- Committed and pushed the accepted TUI-002 decision as `1f1fc1c`
  (`docs: choose TUI transcript ownership`) with raw Git.
- TUI-003 is accepted and closes D4. Each focused workflow migrates only after
  the preceding parity gate; every current key/command entry is retained and
  cuts over with its command/key pair. Page presentation remains rollback-only
  until TUI-070 proves all E5 parity, E6 geometry, exact return/child-back, and
  no-page-only-behavior criteria.
- `internal/tui.StatusModel` owns root overlay focus, local state, and saved
  composer/source return state while transcript/live meaning updates beneath
  it. Runs detail and typed needs-input are the only explicit child states; no
  general overlay stack, callback, domain-state, runtime dependency, D6,
  app/domain prerequisite, product code, or runnable task was added by the
  decision pass.
- Committed and pushed the accepted TUI-003 decision as `1e57c06`
  (`docs: accept TUI overlay migration`) with raw Git.
- Created `docs/architecture/tui-overhaul/TUI_004_DECISION_PROMPT.md` as the
  only next-slice prompt after that push. It limits the next fresh pass to D6,
  requires reconciliation and verification, deletes itself when consumed, and
  hands off to TUI-005 without implementing or publishing TUI-010.
- TUI-004 is accepted and closes D6. Each TUI process emits one committed
  `session-start` before bounded replay, sourced from the local product label,
  inspected absolute repository root, and process-start initialization.
  Refresh, resize, and overlay transitions never re-emit it; restart does.
- Revolvr adds no clear action: external terminal/multiplexer clearing is not
  observed and causes no application replay. The accepted presentation-owner,
  stable-identity, failure, geometry, restoration, and dashboard-removal proof
  gates are reconciled across the design, epics, tasks, references, and durable
  state. No product code, callback, domain state, dependency, or runnable task
  changed.
- Consumed and removed
  `docs/architecture/tui-overhaul/TUI_004_DECISION_PROMPT.md`.

## Exact Next Command

Run one fresh Codex pass for TUI-005:

```bash
codex exec "Execute TUI-005 from docs/architecture/tui-overhaul/tasks/tui-005-accept-experience-states.md as one bounded documentation-only decision pass. Read AGENTS.md, the durable state, and the TUI-overhaul design/reference files first; accept only the experience-state snapshots, do not change product code, publish TUI-010, commit, or continue past TUI-005."
```

Do not create another TUI-004 prompt or start TUI-010 in that pass.

## Verification

- `gofmt -w internal/tui/model.go internal/tui/model_test.go
  internal/tui/architecture_024_test.go internal/tui/checkpoint_test.go
  internal/cli/root_test.go` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
- `go run ./cmd/revolvr tui --help` — PASS.
- `git diff --check` — PASS.
- `git diff --name-only` — PASS; the approved snapshot contains only the
  reviewed product, test, planning, reference, and durable-state files.
- Real 80x24 PTY render — PASS; compact status and activity fit above the
  composer/footer without wrapped activity rows or the prior event wall.
- TUI-000 decision checks — PASS; D1, ADR-025, and durable decisions agree that
  Codex references are behavioral evidence only and implementation remains
  native Go/Bubble Tea with no source copying or porting.
- Codex behavioral reference gate — PASS; pinned identity, path-scoped diff,
  trailing-whitespace, marker, and pin checks passed.
- Reference link/range audit — PASS; every relative Markdown target exists and
  every cited Codex/Revolvr line range is within its named local source file.
- Scope audit for the behavioral study — PASS; exactly four reference files
  were created, its prompt was deleted, and no product source, ADR, dependency,
  D2-D6 decision, or task selector was changed by that study pass.
- `git show --check --oneline 24be655` — PASS.
- `git push origin main` — PASS; `f6f1db5..24be655  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-001 `git diff --check`, required D2/D5 `rg`,
  `git diff --name-only 24be655`, and `git status --short` — PASS.
- TUI-001 relative Markdown link, 33-task count, changed-path scope, accepted-
  decision consistency, and rejected-behavior task audits — PASS.
- `test ! -e docs/architecture/tui-overhaul/TUI_001_DECISION_PROMPT.md` — PASS.
- `git show --check --oneline f4e7ebf` — PASS.
- `git push origin main` — PASS; `24be655..f4e7ebf  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-002 path-scoped `git diff --check`, required D3 `rg`,
  `git diff --name-only f4e7ebf`, and `git status --short` — PASS.
- TUI-002 relative Markdown link, changed-path scope, ownership-table,
  accepted-decision consistency, and rejected-owner task audits — PASS.
- `test ! -e docs/architecture/tui-overhaul/TUI_002_DECISION_PROMPT.md` — PASS.
- `git show --check --oneline 1f1fc1c` — PASS.
- `git status --short --branch` after the TUI-002 push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-003 path-scoped `git diff --check` and required D4 overlay/migration/
  parity/return-state/Run-Detail term audit — PASS.
- TUI-003 relative Markdown link, changed-scope, exact dependency-chain,
  retained-entry, page-removal, accepted-D4/open-D6, 33-task, no-runnable-task,
  no-product-code, and status checks — PASS.
- `git show --check --oneline 1e57c06` — PASS.
- `git push origin main` — PASS; `1f1fc1c..1e57c06  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-004 next-slice prompt scope, relative-link, trailing-whitespace, durable-
  selector, and no-runnable-task checks — PASS.
- TUI-004 path-scoped `git diff --check` and required D6 session/startup/
  refresh/resize/restart/clear/overlay/source/owner/identity/failure/removal term
  audits — PASS.
- TUI-004 relative-link, changed-scope, accepted-D6, unchanged-D1-D5,
  33-task, no-published-TUI-010, no-product-code/runtime-dependency,
  prompt-deletion, and TUI-005-selector checks — PASS.

Run the exact next command above for TUI-005.
