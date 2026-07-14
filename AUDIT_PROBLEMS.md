# Codebase Audit Problems

Audit date: 2026-07-14
Audited revision: `f9dcd960b3b4`

## Summary

This audit found six substantiated problems. The most serious are a systemic
check/use gap in harness-owned filesystem state, incomplete process-group
settlement after `SIGKILL`, and TUI shutdown that does not join an active run.
Two focused audit-only regressions were added temporarily, executed, and
removed: one reproduced an outside-root autonomous-state mutation, and one
observed two different token totals from the same usage event.

The ordinary, race-enabled, and shuffled Go suites all pass. These findings
are therefore gaps in the behaviors currently covered by tests, not a claim
that the existing suite is failing.

## AP-01 — Harness-owned filesystem operations are not bound to a stable namespace

Severity: high
Areas: `internal/runtimepath`, `internal/autonomousstate`,
`internal/autonomousnotification`, `internal/autonomoustaskrun`,
`internal/autonomousarchive`, `internal/autonomousfinalization`, and several
evidence readers

### Evidence

The shared `runtimepath` package checks path components with `Lstat`, then uses
the full pathname again:

- `internal/runtimepath/runtimepath.go:43-65` checks and creates directories by
  name one component at a time.
- `internal/runtimepath/runtimepath.go:152-176` checks a file and then calls
  `os.OpenFile` by name. `O_NOFOLLOW` protects only the final component; it
  does not prevent an ancestor from being replaced between the check and open.
- `internal/runtimepath/runtimepath.go:182-210` and `:216-244` similarly open
  files and directories after separate pathname checks. The later opened/name
  identity check detects many substitutions, but a creating open may already
  have added an outside-root directory entry before that detection.
- The package has no descriptor-relative rename/publication primitive, so
  callers fall back to `os.Rename`, `os.Link`, and `os.Remove` by pathname.

Several authoritative stores either inherit that gap or bypass the stronger
helpers entirely:

- `internal/autonomousstate/store.go:429-452` performs `Lstat` followed by
  `os.ReadFile`; `:455-498` uses `os.ReadDir`/`os.ReadFile` after `safePath`;
  `:501-545` publishes immutable evidence with a by-name exclusive open; and
  `:550-616` creates a temporary file and later renames it by pathname without
  revalidating the parent or temporary-file identity. The shared helper is
  used by planning, audit, attempt, input, block, optional-role, workspace,
  and finalization state transitions.
- `internal/autonomousnotification/store.go:311-319` checks a file and then
  reopens it by name; `:326-391` performs immutable publication, replacement,
  cleanup, and directory sync by name. `ensureSafeDirectory` and
  `ensureSafeExistingParents` at `:409-460` are also check-then-use walkers.
  `internal/autonomousnotification/delivery.go:324-346` converts the hardened
  delivery lease to an `unlock` closure, so persistence cannot call
  `Flock.Check` immediately before metadata mutation.
- `internal/autonomoustaskrun/store.go:42-104` calls `CheckDir`/`CheckFile` and
  then `os.ReadDir`/`os.ReadFile`, contrary to the package's documented
  protected-read contract. Its checkpoint and history publication at
  `:122-257` also uses by-name temp creation, rename, cleanup, and sync.
- `internal/autonomousarchive/storage.go:42-229` duplicates a symlink walker,
  `Lstat`/`ReadFile`, temp/link or temp/rename publication, cleanup, and sync
  instead of using one stable directory authority.
- `internal/autonomousfinalization/coordinator.go:458-557` uses another
  bespoke parent walker plus by-name read, `MkdirAll`, temp creation, rename,
  and readback for terminal completion evidence.
- The same unsafe `Lstat`-then-`ReadFile` shape exists in authoritative or
  operator-visible readers including
  `internal/autonomousauditapply/apply.go:442-446`,
  `internal/autonomousplanapply/apply.go:489-506`,
  `internal/operatorcheckpoint/receipt.go:185-195`,
  `internal/ledgerexport/export.go:802-815`,
  `internal/codexexec/last_message.go:208-221`, and
  `internal/app/autonomous_view.go:315-325`.

This was reproduced against `autonomousstate.Store.replaceState` using the
existing `FailureBeforeStateRename` seam:

1. Prepare a valid current state so the locked CAS read succeeds.
2. At `FailureBeforeStateRename`, move the real task-state directory aside.
3. Create an outside directory containing attacker-controlled bytes under the
   same temporary basename, then replace the canonical task-state directory
   with a symlink to that outside directory.
4. Allow the fault hook to return `nil`.

`os.Rename(tempPath, statePath)` resolved both names through the replacement
symlink and created the outside `state.json` from the attacker-controlled
temporary file. `replaceState` later returned an error, but the outside-root
mutation had already occurred. Holding the old `state.lock` inode does not
protect a replaced directory namespace.

This is a same-account concurrent-filesystem integrity issue, not a privilege
escalation or a claim that Revolvr is a security sandbox. It nevertheless
violates the repository's explicit no-follow, unchanged-outside-sentinel, and
durable-evidence identity contracts.

### Impact

- Canonical state, history, notification intent/journal, task-run recovery,
  archive evidence, or finalization evidence can be read from or published to
  a different inode than the one that was validated.
- A failed operation can still create, replace, link, or remove an entry
  outside the canonical repository root.
- Replay and CAS decisions can be made from substituted evidence.
- A held advisory lock can remain valid on an obsolete inode while mutation
  proceeds through a replacement namespace.

### Required correction

Fix the root cause before patching individual `Lstat` calls:

1. Extend the filesystem boundary with descriptor-rooted traversal and
   metadata operations. On supported Unix platforms this should use stable
   directory descriptors and `*at`/equivalent no-follow operations (or an
   equally strong mechanism), including create, enumerate, rename/link,
   unlink, and directory sync.
2. Bind every publication to the validated parent and opened temporary inode;
   validate the lease and namespace immediately before destructive metadata
   operations and again before accepting readback.
3. Migrate each affected store and reader; remove the duplicated bespoke path
   walkers once their callers use the shared boundary. This should safely
   reduce code rather than add more scattered defensive checks.
4. Add deterministic substitution hooks at before-open, after-open,
   before-publication, after-publication, and cleanup boundaries. Cover final
   symlinks, ancestor symlinks, hard links, unsafe modes, renamed ancestors,
   and unchanged outside contents, entries, modes, and link counts.
5. Add an end-to-end regression for every durable owner listed above. The
   autonomous-state reproduction should become a permanent regression and
   must fail before any outside entry is created.

## AP-02 — Process-tree termination returns immediately after sending `SIGKILL`

Severity: high
Area: `internal/runner/runner.go:202-229`

### Evidence

`terminateProcessTreeWithSignal` polls after `SIGTERM`, but when the grace
timer expires it calls `signal(true)` and returns immediately at line 226.
On Unix, `signal(true)` is `syscall.Kill(-pid, SIGKILL)`
(`internal/runner/process_tree_unix.go:17-28`). A successful `kill` syscall
only queues/delivers the signal; it does not wait until all members of the
process group have exited.

Current tests wait well after `Run` returns and usually observe that the
kernel has completed termination, but they do not prove settlement at the
return boundary. The earlier successful-leader fix therefore closes the
pre-`SIGKILL` path but still leaves a post-`SIGKILL` window.

### Impact

`runner.Run` can return while a force-killed descendant still exists. A caller
may then finalize a receipt, release a source lease, update task/ledger state,
or start another operation before the old process group is actually gone.
The return value does not establish the lifecycle guarantee that descendants
cannot mutate after return.

### Required correction

- After `SIGKILL`, continue bounded polling until the original process group
  is gone. Use a distinct bounded kill-settlement deadline so the runner cannot
  hang indefinitely.
- Preserve signal and inspection errors. If settlement cannot be proven,
  return an explicit unsettled-process-tree error rather than success plus the
  original cancellation alone.
- Reconcile process-group identity reuse on both natural-exit and cancellation
  races before signalling or polling an identity that may no longer belong to
  this command.
- Add a deterministic TERM-ignoring descendant regression that blocks its
  final exit long enough to assert that `Run` does not return first, followed
  by an unchanged-sentinel assertion beginning immediately at return.

## AP-03 — Quitting the TUI abandons active-run settlement

Severity: high
Area: `internal/tui/model.go:229-254`, `:1164-1351`, `:1389-1400`

### Evidence

When a run is active, `q` and `ctrl+c` call `requestRunCancel()` and
immediately return `tea.Quit` (`updateActiveRunKeys`, lines 1393-1396).
`RunStatus` returns as soon as Bubble Tea exits; there is no outer join on the
run goroutine.

The run-once, loop, exact-task, and queue commands all execute their domain
operation in a goroutine and publish a terminal `runOnceDoneMsg` only after
the operation and status refresh complete (`:1172-1185`, `:1212-1227`,
`:1239-1273`, and `:1305-1351`). Immediate quit stops consuming that terminal
message and allows the CLI process to exit before domain cleanup finishes.
Existing cancellation tests cover the `c` key, which stays in the model and
drains the terminal message; the quit test covers only an idle model.

### Impact

An operator can exit before process-tree termination, source-lease release,
receipt/ledger/task finalization, queue reconciliation, and status refresh.
Because program exit terminates Go goroutines, even requesting context
cancellation is not sufficient evidence that cleanup ran.

### Required correction

- During an active run, make `q`/`ctrl+c` request cancellation and enter a
  `quit-after-settlement` state. Do not emit `tea.Quit` until the matching
  terminal message has been applied.
- Alternatively, have `RunStatus` own and join the active operation outside
  Bubble Tea before returning; the model approach is likely smaller because
  it already has the event stream and terminal message.
- Ensure terminal publication cannot deadlock when cancellation has stopped
  progress publication.
- Add active-run tests for all four modes. Block domain cleanup, press `q`,
  prove no quit command is returned, release cleanup, process the terminal
  message, and only then expect `tea.Quit`.

## AP-04 — Nested Codex usage selection depends on Go map iteration order

Severity: medium
Area: `internal/receipt/metrics.go:330-371`

### Evidence

Known top-level shapes have explicit precedence, but the fallback
`nestedUsageMap` ranges a `map[string]any` and returns the first recursively
found usage object. Go deliberately randomizes map iteration, so an event with
multiple nested usage-shaped objects has no stable authority.

A temporary focused test parsed the same in-memory event 1,000 times. One
nested object reported `input_tokens: 1`; another reported
`input_tokens: 101`. Both totals were observed in every one of ten test runs.

This is actual result nondeterminism, not only unstable diagnostic wording.
The selected metrics flow into `codexexec.Result.Usage`, receipts, and
autonomous token accounting (`internal/autonomousattempt/execution.go:197-219`).

### Impact

Identical Codex JSONL can produce different receipts and token-budget
consumption across runs. In ambiguous input, one parse may admit another
attempt while another may exhaust a budget.

### Required correction

- Define a schema-based precedence for supported Codex event shapes. Do not
  use arbitrary recursive map traversal as control authority.
- If multiple otherwise valid candidates remain, fail closed with a typed
  ambiguity diagnostic rather than silently choosing one. Merely sorting keys
  makes the bug repeatable but does not establish that the chosen object is
  semantically correct.
- Add exact tests for each supported shape plus a multi-candidate event
  repeated enough to prove stable precedence or stable ambiguity failure.

## AP-05 — Source snapshots validate the pathname, not the file that was hashed

Severity: medium
Area: `internal/gitstate/source_snapshot.go:499-549`

### Evidence

For a regular file, `captureSourceEntry`:

1. calls `os.Lstat(absPath)`;
2. calls `os.Open(absPath)`, which follows a replacement final symlink;
3. hashes the opened descriptor;
4. calls `os.Lstat(absPath)` again; and
5. compares only the two pathname observations.

It never calls `file.Stat` and never proves that the opened descriptor is the
same inode as either pathname observation. An A-to-B-to-A substitution around
the open therefore lets the snapshot record A's metadata/path with B's bytes.
The symlink branch similarly calls `Readlink` without a post-read identity
check.

Full source snapshots are documented and consumed as the authoritative race
and mutation record for supervisor, worker, verification, and finalization
freshness. The missing descriptor identity check weakens that guarantee.

### Impact

A concurrent source replacement can produce a valid-looking policy source
revision for bytes that were never the contents of the recorded path/inode.
Routing, verification freshness, and commit admission can then compare against
incorrect source evidence.

### Required correction

- Open regular files without following the final component, immediately
  `fstat` them, and require opened identity/type/mode/size to match the initial
  pathname observation. Recheck both descriptor and pathname after hashing.
- Apply an equivalent before/after identity rule to symlink targets.
- Add deterministic seams around `Lstat`, open/readlink, and post-read checks;
  cover final-symlink replacement, inode replacement, and A-to-B-to-A
  substitution.

## AP-06 — Map order still controls two first-error diagnostics

Severity: low
Areas: `internal/autonomous/state.go:1655-1674`,
`internal/autonomousarchive/git.go:188-196`

### Evidence

`validateAttemptTransition` builds `previousBudgets` as a map, then returns on
the first changed or missing action budget encountered while ranging that map.
If two budgets are invalid, the reported action varies across calls.

`verifyCommit` likewise ranges `expectedFiles` and returns on the first missing
or byte-mismatched file. A commit with multiple bad expected files therefore
has nondeterministic command order and first-error text.

These are smaller than AP-04 because the accepted/rejected result does not
change, but they contradict the deterministic-diagnostic contract used by the
rest of the codebase and make tests and operator triage less reproducible.

### Required correction

- Validate action budgets in their canonical slice order or via sorted action
  keys.
- Validate archive files using sorted expected paths (the function already
  constructs a sorted path list).
- Add multi-invalid tests that assert the exact same first error over repeated
  calls.

## Verification performed

The following checks passed on the audited revision:

- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go test -shuffle=on -count=1 ./...`
- `go vet ./...`
- `go mod verify`
- `govulncheck ./...` (no reachable vulnerabilities; unreachable findings
  remain in dependencies/packages)
- tracked-Go formatting check and `git diff --check`
- shell syntax checks for tracked `*.sh`
- CLI help, `config check`, and `status`
- Linux 386/arm64, Darwin arm64, and FreeBSD arm64 cross-builds

The two temporary reproducer tests described above were removed after use.
No production code or dependency was changed by the audit. No standalone
performance regression or safe additional line-count reduction was found;
the clearest safe LOC reduction is to replace the duplicated filesystem
walkers with one corrected shared boundary while resolving AP-01.
