# Codebase Audit Problems

Audit date: 2026-07-14

## Summary

The audit found six actionable problems:

| ID | Severity | Problem |
| --- | --- | --- |
| AUDIT-01 | High | Cancellation can outrun source-lease failure publication and mutate a task under lost ownership. |
| AUDIT-02 | High | `allow_pre_existing_dirty` stages and commits pre-existing user changes. |
| AUDIT-03 | High | Core coordination locks bypass the repository's protected runtime-path and opened-file identity contract. |
| AUDIT-04 | Medium | Read-only app commands open the live ledger through the writable, schema-initializing API. |
| AUDIT-05 | Medium | The supported-platform contract is unstated and internally inconsistent; the CLI does not build on Windows. |
| AUDIT-06 | Low | The documented local smoke test fails on the current `task list` header. |

No additional performance or line-count reduction was recorded. The candidates
encountered during the sweep were either insignificant, architecture-sensitive,
or not demonstrably safer than the existing code. They should not be treated as
problems merely to create cleanup work.

## AUDIT-01: Cancellation can hide an asynchronous source-lease failure until after task mutation

Severity: **High**

### Impact

When parent cancellation races with a failing source-lock heartbeat, a mixed
pass can be finalized as an ordinary Codex failure, change the canonical task
from `pending` to `blocked`, and return `codex_failed`. Only the deferred guard
close subsequently discovers and joins the ownership failure. The operation
therefore mutates task and ledger state after its ownership classification has
become uncertain, contrary to the intended fail-closed ownership boundary.

The autonomous-cycle integration contains the same check pattern and can also
retain the earlier outcome classification when the deferred close is the first
place that observes the heartbeat failure.

### Evidence and root cause

- `internal/runonce/runonce.go:971-983` checks `sourceGuard.Failure()`, but if
  no failure has been published yet and `ctx.Err() != nil`, it returns `nil`
  without synchronizing with the in-flight heartbeat or performing a final
  check.
- `internal/runonce/runonce.go:435-445` then enters ordinary Codex-failure
  finalization.
- `internal/runonce/runonce.go:845-880` blocks the selected canonical task for
  every non-committed outcome.
- `internal/runonce/runonce.go:179-192` waits for the guard only in a defer. If
  that close finds the ownership failure, it joins the error but changes the
  outcome only when the outcome is still empty. It cannot undo the task
  mutation already performed by `finish`.
- `internal/autonomouscycle/cycle.go:731-744` has the same
  `Failure`-then-`ctx.Err` early return. Its defer at lines 239-248 changes the
  outcome only when the pre-existing `runErr` is `nil`.
- The repository already has a regression test for the intended behavior:
  `internal/runonce/runonce_test.go:2844-2880`. During this audit, an ordinary
  full `go test -count=1 ./...` run failed that test: the returned error
  contained `context.Canceled`, `lock.ErrOwnershipLost`, and the injected
  persistence error, but the result was `codex_failed` and the task was
  `blocked` instead of the expected `blocked` ownership outcome with the task
  still `pending`. A focused repeated run passed, confirming the result is
  scheduling-dependent rather than a deterministic test expectation error.

### Resolution direction

Make guard-monitor settlement part of terminal classification, before calling
any finalizer that mutates canonical task state. A safe design should:

1. Expose a guard operation that stops and joins the heartbeat monitor without
   releasing the lease, or otherwise synchronizes with an in-flight heartbeat.
2. Preserve ordinary parent cancellation when the heartbeat ended only because
   of that cancellation, while joining any independent heartbeat/persistence
   failure.
3. Make the final ownership decision before task transition, receipt
   finalization, or run completion.
4. Release the lease exactly once after terminal persistence is finished.
5. Apply the same terminal-settlement contract to both `runonce` and
   `autonomouscycle` rather than duplicating the current racy helper.

### Required regression coverage

- Make the existing cancellation/heartbeat test deterministic by controlling
  when the heartbeat returns and when its failure is published.
- Assert the canonical task bytes remain unchanged when ownership failure wins
  or accompanies cancellation.
- Assert the result outcome, terminal ledger evidence, returned error, and
  receipt all agree on the ownership failure.
- Add the equivalent autonomous-cycle race test.

## AUDIT-02: `allow_pre_existing_dirty` commits changes that predate the run

Severity: **High**

### Impact

When `commit.allow_pre_existing_dirty` is enabled, Revolvr can include unrelated
operator edits, staged work, or untracked files in its generated commit. This
breaks commit provenance and can publish user work that the selected task and
Codex pass did not create.

### Evidence and root cause

- Both `gitstate.CaptureDirtyWorktree` and
  `gitstate.CaptureChangedFiles` call the same whole-worktree command,
  `git status --porcelain=v1 -z --untracked-files=all`
  (`internal/gitstate/gitstate.go:73-89` and `156-169`). The latter is not a
  before/after delta; it is the complete dirty set at the time of capture.
- `internal/runonce/runonce.go:802-817` subtracts pre-existing path names only
  when deciding whether the pass produced a meaningful change.
- The commit call at `internal/runonce/runonce.go:530-547` nevertheless passes
  the unfiltered `PostRunChanged` capture.
- `internal/commit/commit.go:107-110` copies every post-run path into
  `Result.ChangedFiles`, and lines 146-147 run `git --literal-pathspecs add --`
  with that complete list. The flag only bypasses the refusal at lines 322-323;
  it does not exclude pre-existing paths.
- `internal/runonce/runonce_test.go:2410-2491` masks the defect by injecting an
  impossible real-world pair of captures: the pre-run capture contains only
  `internal/dirty.go`, while the post-run whole-worktree capture contains only
  `internal/feature.go`. A real post-run `git status` would contain both.
- The autonomous worker already demonstrates the missing concept:
  `internal/autonomouscycle/worker.go:293-311` computes a content-sensitive
  source-snapshot difference, and lines 420-430 plus 660-665 filter the commit
  capture to those observed worker paths.

Simply subtracting filenames is not fully safe. If a file was already dirty and
the model edits that same file, staging the final file also stages the
operator's earlier hunks.

### Resolution direction

Choose one explicit contract:

- Safest and simplest: remove or reject `allow_pre_existing_dirty` for mixed
  passes and retain the clean-worktree requirement.
- If dirty-worktree execution must be supported, isolate the model's work from
  the operator's work (for example, a clean Git worktree/snapshot) and commit
  only a content-sensitive, authority-checked delta. Refuse overlapping paths
  unless the implementation can prove exact hunk ownership.

Do not fix this solely by subtracting `PreRunDirty` path strings from
`PostRunChanged`.

### Required regression coverage

Use a real temporary Git repository rather than fake captures:

1. Commit a baseline.
2. Modify one tracked file before the run.
3. Have the run modify a different file and its task metadata.
4. Enable the dirty-worktree option.
5. Assert the generated commit excludes the pre-existing modification and the
   worktree retains it.
6. Add an overlap case where the operator and model both touch one file; it
   must fail closed unless exact ownership is implemented.

## AUDIT-03: Coordination lock files do not use protected path and inode checks

Severity: **High**

### Impact

A pre-existing symlink or hard link at a predictable lock path can redirect a
lock open to another file. The source-writer path is especially dangerous:
acquisition, heartbeat, and release truncate and rewrite the opened target, so
a link can cause an unrelated writable file outside `.revolvr` to be damaged.

Concurrent pathname replacement can also split contenders across different
inodes. Each process may successfully `flock` a different file while believing
it owns the same logical lock, invalidating source-writer, retention, Git-admin,
notification, or other coordinator exclusion.

### Evidence and root cause

- `internal/lock/source_writer.go:383-449` creates directories with
  `os.MkdirAll` and opens both `artifact-retention.lock` and
  `source-writer.lock` with ordinary `os.OpenFile`. There is no canonical-root,
  ancestor-symlink, `O_NOFOLLOW`, hard-link, mode, or opened/named inode check.
- Source metadata writes and releases truncate the descriptor
  (`internal/lock/source_writer.go:248-289` and `496-514`), making final-component
  link following destructive rather than merely advisory.
- The same final-file omission exists in other cross-process coordinators,
  including `internal/artifactretention/apply.go:500-595`,
  `internal/autonomousworkspace/manager.go:862-883`, and
  `internal/autonomousnotification/delivery.go:323-349`. Some validate parent
  directories, but none proves the final opened descriptor still matches the
  named lock after acquisition.
- This is inconsistent with the repository's established implementation in
  `internal/runtimepath/runtimepath.go`: `CanonicalRoot`, `EnsureDir`,
  `CheckFile`, and `CheckOpenedFile` reject symlinks, aliases, unsafe modes, and
  opened/named identity changes. The protected read helpers use
  `O_NOFOLLOW` and recheck identity around sensitive work.

### Resolution direction

Create one shared hardened flock acquisition primitive and migrate every
predictable harness coordination lock to it. It should:

1. Start from a canonical repository/control root.
2. create and validate directory components without following symlinks;
3. validate an existing final component or allow a missing one;
4. open with no-follow semantics and restrictive mode;
5. validate the opened regular file, link count, mode, and named/opened inode;
6. take the requested shared/exclusive flock; and
7. recheck named/opened identity after successful acquisition and before any
   destructive metadata operation.

Keep platform-specific open/flock details behind build-tagged files. Avoid
implementing subtly different safety checks in each package.

### Required regression coverage

- Final-component symlink to an outside sentinel: acquisition must fail and the
  sentinel bytes must remain unchanged.
- Hard-link alias: acquisition must fail.
- Symlinked ancestor under `.revolvr`: acquisition must fail.
- Rename/substitute the pathname between open and flock: acquisition must fail
  rather than granting a second logical owner.
- Run these cases for the shared primitive and at least one integration path
  for source-writer, retention, Git-admin, and delivery coordination.

## AUDIT-04: Read-only projections initialize or migrate the live ledger

Severity: **Medium**

### Impact

`status`, `show`, receipt validation, and the TUI paths that call the same app
functions can mutate the ledger they are inspecting. They can initialize an
empty file, apply schema migrations, create directories/sidecars, or fail on a
read-only filesystem even though the requested operation is a read projection.
This also means diagnostic inspection can alter damaged or old evidence before
the operator decides how to recover it.

### Evidence and root cause

- `internal/app/app.go:206`, `367`, and `401` use `ledger.Open` in `Status`,
  `ShowRun`, and `ValidateReceipt`.
- `ledger.Open` calls `ensureDBDir` and `StoreWithClock`, which initializes or
  migrates schema (`internal/ledger/store.go:130-149`).
- `ledger.OpenLiveReadOnly` already exists for this exact boundary and promises
  not to create directories, initialize schema, or migrate
  (`internal/ledger/store.go:152-188`). `ListTasks`, metrics, archive
  verification, ledger export, and other read projections already use it.
- `.agent/DECISIONS.md` under “AUD-04 Live Ledger Read Contract” explicitly
  states that a raw live ledger must be opened through `OpenLiveReadOnly`.
- Direct audit reproduction: create an otherwise initialized-looking workspace
  with an empty `.revolvr/ledger.sqlite`, then run `revolvr status`. The command
  succeeded and changed the ledger from 0 bytes (SHA-256
  `e3b0c442...b855`) to 36,864 bytes (SHA-256 `f9a0affd...c32f`).

### Resolution direction

Use `ledger.OpenLiveReadOnly` in all app functions whose contract is inspection
or validation. Centralize the app-level live-reader helper so new projection
functions do not choose the writable opener accidentally. Old, malformed, or
uninitialized ledgers should produce a diagnostic error and remain byte-for-byte
unchanged; schema upgrade belongs to an explicit mutation/recovery operation.

### Required regression coverage

- Snapshot ledger and sidecar names, hashes, sizes, modes, and mtimes around
  `Status`, `ShowRun`, and `ValidateReceipt`.
- Repeat with an empty file and a deliberately old/malformed schema; each read
  command must leave all bytes and filesystem entries unchanged.
- Verify the app functions work when the ledger and its parent are not writable.

## AUDIT-05: Platform support is unstated and the non-Unix fallback cannot make the CLI build

Severity: **Medium**

### Impact

The setup guide lists only Go, Git, and Codex as requirements, so a Windows user
reasonably expects `go build ./cmd/revolvr` to work. It does not. The failure is
particularly confusing because `internal/runner` contains a `!unix` fallback
designed to return a typed unsupported-process-tree error, but the rest of the
CLI cannot compile far enough to use it.

### Evidence and root cause

- `README.md:10-20` gives no operating-system restriction.
- `internal/runner/process_tree_unsupported.go` and its test use a `!unix` build
  tag and intentionally fail closed at runtime with
  `ErrProcessTreeUnsupported`.
- Many other packages use Unix-only `syscall` constants, `Flock`, and `Stat_t`
  directly in untagged files.
- `GOOS=windows GOARCH=amd64 go build ./cmd/revolvr` fails to compile, including
  undefined `syscall.O_NOFOLLOW`, `O_DIRECTORY`, `Stat_t`, `Flock`, and
  `LOCK_*` references in `runtimepath`, `lock`, `autonomousnotification`,
  `autonomousstate`, and other packages.
- Cross-builds for Linux, Darwin, and FreeBSD succeeded during this audit.

### Resolution direction

Decide and enforce one support policy:

- If Revolvr is intentionally Unix-only, say so in setup requirements and use
  explicit build constraints or an intentional unsupported-platform command
  stub so the failure is clear and controlled.
- If Windows is supported, isolate every lock, no-follow open, inode/link, and
  process-tree operation behind platform files and implement equivalent Windows
  semantics. The runner's current `!unix` fail-closed behavior can remain for
  process execution until safe tree control exists, but the overall compile
  contract must be coherent.

Add a CI build matrix that at minimum covers every documented platform.

## AUDIT-06: The documented local smoke test has a stale task-list assertion

Severity: **Low**

### Impact

The primary no-model CLI smoke command documented in the README fails even
though the CLI behavior is valid. This creates a false regression signal and
means the documented development verification sequence cannot currently pass.

### Evidence and root cause

- `README.md:974-981` tells contributors to run `./scripts/smoke-local.sh`.
- `scripts/smoke-local.sh:49` still expects the contiguous header
  `ID<TAB>STATUS<TAB>TASK<TAB>SUMMARY`.
- The current task list inserts workflow, phase, profile, scheduling, dependency,
  and checkpoint columns between `STATUS` and `TASK`.
- Running the script during this audit failed at that exact assertion. Both
  fake-Codex run-once smoke scripts passed.

### Resolution direction and verification

Update the assertion to a stable current contract, such as the leading columns
`ID<TAB>STATUS<TAB>WORKFLOW<TAB>PHASE`, and retain separate assertions for the
task text and summary. Then run `./scripts/smoke-local.sh` end to end. Prefer a
small test of the table schema or shared column definition if future CLI work is
likely to change the header again.

## Audit Scope and Verification Evidence

The sweep covered all production packages under `internal/`, the CLI entry
point, repository scripts, config and operator documentation, cross-package
run/commit/lock/ledger workflows, durable architecture decisions, path and
platform boundaries, and representative tests. The repository contained 60
top-level internal packages, 286 Go files, and 963 Go tests at audit time.

Checks run:

- `gofmt -l` over tracked Go files: clean.
- `git diff --check`: clean before the report was written.
- `go vet ./...`: passed.
- `go test -race -count=1 ./...`: passed.
- `go test -shuffle=on -count=1 ./...`: passed.
- `go test -count=1 ./...`: the initial run failed at the race described in
  AUDIT-01; the final rerun passed, preserving evidence of intermittency.
- `govulncheck ./...`: no reachable vulnerabilities; it reported vulnerable
  symbols in dependencies/modules that this code does not call.
- `go mod verify`: passed.
- Shell syntax checks for tracked shell scripts: passed.
- Root help and `config check`: passed.
- `scripts/smoke-run-once-fake-codex.sh`: passed.
- `scripts/smoke-run-once-fake-codex-verification-failure.sh`: passed.
- `scripts/smoke-local.sh`: failed as described in AUDIT-06.
- Linux, Darwin, and FreeBSD CLI cross-builds: passed.
- Windows CLI cross-build: failed as described in AUDIT-05.

No live Codex run was started by this audit, no dependency was added, and no
commit was created.
