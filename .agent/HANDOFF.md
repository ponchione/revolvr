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
- Committed and pushed the accepted TUI-004 decision as `3509bb4`
  (`docs: accept TUI session header lifecycle`) with raw Git.
- Created `docs/architecture/tui-overhaul/TUI_005_DECISION_PROMPT.md` as the
  only next-slice prompt after that push. It limits the next fresh pass to the
  experience-state snapshots, preserves D1-D6, deletes itself when consumed,
  and hands off to a separate TUI-010 publication pass without implementing or
  publishing TUI-010.
- TUI-005 is accepted and closes E0. The design authority now contains literal
  initialized-idle, uninitialized, running, completed, failed, cancelled,
  needs-input, Help-overlay, and 40-column source snapshots. Every fact has one
  session/transcript, live, composer, overlay, or transient-footer owner.
- Normal acceptance geometry is 80x24 and the minimum supported geometry is
  40x24. Required safety, cancellation, current-work, terminal-outcome, focus,
  and next-action text never truncates; below 40 columns is explicit best
  effort without application-owned history clear or replay.
- Consumed and removed
  `docs/architecture/tui-overhaul/TUI_005_DECISION_PROMPT.md`. No product code,
  application/domain authority, dependency, production fixture, canonical
  task, commit, push, or TUI-010 implementation changed.
- Created `docs/architecture/tui-overhaul/TUI_010_PUBLICATION_PROMPT.md` as the
  sole next-slice prompt. It allows one separate pass to publish only TUI-010
  as the first canonical implementation task and forbids implementing it in
  that publication pass.
- Committed and pushed TUI-005 plus the initial next-slice prompt as `19d80f8`
  (`docs: accept TUI experience states`) with raw Git. `main` and `origin/main`
  both resolved to `19d80f8977dabae8c3bd1f8a0cf430879147efa8`
  immediately after the push.
- Prepared the TUI-010 publication prompt after that push by pinning it to the
  exact accepted TUI-005 source. This preparation does not publish or start
  TUI-010.
- Published only `.agent/tasks/tui-010-prove-shell-composition.md` as a pending
  mixed-pass proof from the accepted draft. E0 remains accepted, no later TUI
  task was published, and TUI-010 remains unstarted.
- Reconciled only affected planning and durable status, then consumed and
  deleted the TUI-010 publication prompt. No product code, Go test, dependency,
  fixture, callback, domain state, or terminal behavior changed.
- Completed TUI-010 with a test-only proof in `internal/tui/model_test.go`.
  Accepted session/history cells append once through installed `tea.Println`;
  live/composer rows remain managed, two output-buffer types pass, and a real
  PTY restores its normal modes after `q`. No production shell, dependency, or
  later TUI task was added.
- Committed and pushed the TUI-010 publication, proof, and reconciliation as
  `1d959ad` (`test: prove TUI shell composition`) with raw Git. `main` and
  `origin/main` both resolved to
  `1d959ad8bb3315ea1a49c8e9b747945ff68a4c13` immediately after the push.
- Prepared `.agent/STATE_COMPACTION_PROMPT.md` as the sole next-pass prompt.
  It permits only the selected current-state compaction and its normal task and
  handoff metadata.
- Completed the bounded current-state compaction from accepted source
  `1d959ad8bb3315ea1a49c8e9b747945ff68a4c13`. `.agent/STATE.md` now retains
  only current architecture, authority, verification, blocker, and selector
  facts and remains under 200 lines.
- Marked only task `01a043b3-ad2d-7979-8d33-b6875643af8d` complete. No
  canonical task is pending; deferred EXT and superseded PTC work remain
  non-selectable, and no later TUI task was published.

## Exact Read-Only Selector

```bash
go run ./cmd/revolvr status
```

It must report no pending canonical task and must not select deferred EXT or
superseded PTC work.

## Exact Next Command

No next task is authorized. Confirm the empty canonical selector before
publishing any separately approved work:

```bash
go run ./cmd/revolvr status
```

Do not revive deferred EXT/PTC work, publish a later TUI task, or reopen
Architecture 025 without a separately authorized task.

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
- `git show --check --oneline 3509bb4` — PASS.
- `git push origin main` — PASS; `1e57c06..3509bb4  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-005 next-slice prompt scope, relative-link, trailing-whitespace, durable-
  selector, accepted-D1-D6, and no-runnable-task checks — PASS.
- TUI-005 required diff/term, relative-link, changed-scope, accepted-D1-D6,
  owner-annotation, literal-state, 80-/40-column, 33-task, no-published-TUI-010,
  no-product-code/dependency, prompt-deletion, and exact-next-selector checks —
  PASS.
- `git show --check --oneline 19d80f8` — PASS.
- `git push origin main` — PASS; `aadd5f4..19d80f8  main -> main`.
- `git status --short --branch` immediately after push — PASS;
  `main...origin/main` with no worktree changes.
- TUI-010 publication prompt exact-source pin, publication-only scope, no-
  implementation boundary, relative-link, and durable-selector checks — PASS.
- TUI-010 canonical task publication, required-term, relative-link, changed-
  scope, prompt-deletion, no-later-task, and exact-next-selector checks — PASS.
- `gofmt -w internal/tui/model.go internal/tui/model_test.go` — PASS.
- `go test ./internal/tui -run 'TestTranscriptShellProof'` — PASS.
- `go test ./internal/tui` — PASS.
- `go test ./...` — PASS.
- Interactive PTY proof — PASS; committed cells appeared once in order, `q`
  exited cleanly, and Bubble Tea disabled paste/mouse modes and restored the
  cursor. The returned Bash prompt executed `printf 'PROMPT_OK\n'` successfully.
- `go run ./cmd/revolvr task list` — PASS; TUI-010 is completed.
- `go run ./cmd/revolvr status` — PASS; the bounded current-state compaction is
  selected next.
- 169-line state-file, TUI-010 completion, and no-later-TUI-publication checks
  — PASS.
- `git show --check --oneline 1d959ad` — PASS.
- `git push origin main` — PASS; `19d80f8..1d959ad  main -> main`.
- `git status --short --branch` immediately after the push — PASS;
  `main...origin/main` with no worktree changes.
- Current-state compaction prompt source pin, one-task scope, changed-path
  boundary, deletion gate, and durable-selector checks — PASS.
- `go test ./...` — PASS.
- `go run ./cmd/revolvr task list` — PASS; all canonical tasks are terminal.
- `go run ./cmd/revolvr status` — PASS; no pending canonical task is selected.
- Current-state line-limit, exact changed-path, unchanged-boundary,
  trailing-whitespace, and prompt-deletion gates — PASS.

Run the exact read-only selector above before any future task publication.
