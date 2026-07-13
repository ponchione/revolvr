# Codebase Audit — 2026-07-13

## Scope and result

This was a read-only wide sweep of the Go implementation, tests, durable
workflow contracts, CLI behavior, persistence boundaries, and configured
verification. No product code was changed.

The audit found eleven actionable issues: five high-priority integrity or
concurrency defects, four medium-priority durability/configuration defects,
and two low-priority CLI defects. They are ordered by risk below. The existing
architecture is heavily tested and generally disciplined; I did not find a
safe broad refactor that should be done merely to reduce line count.

| ID | Priority | Area | Finding |
| --- | --- | --- | --- |
| R2-01 | High | Ledger export / retention | Raw SQLite main-file hashes are not valid logical ledger identities in WAL mode |
| R2-02 | High | Artifact retention | Mutation exclusion is a check-then-release race |
| R2-03 | High | Artifact retention | Resume trusts mutable GC journal claims instead of immutable history/effects |
| R2-04 | High | Child publication | A weakly validated checkpoint can claim completion for the wrong or empty child set |
| R2-05 | High | Runtime persistence | Queue and child stores do not use the established protected-path contract |
| R2-06 | Medium | Queue persistence | Queue recovery selects the largest isolated record without validating a history chain |
| R2-07 | Medium | Artifact retention | Prune renames are not durably synchronized at the destination |
| R2-08 | Medium | Ledger export | The exporter can create records that its verifier refuses to read |
| R2-09 | Medium | Configuration | Explicit `verification.commands: []` is silently treated as omission |
| R2-10 | Low | CLI | Bare `revolvr run` reports an unimplemented placeholder and exits successfully |
| R2-11 | Low | CLI | Artifact-GC result rendering drops or masks errors |

## Findings

### R2-01 — Raw SQLite main-file hashes are not valid logical ledger identities in WAL mode

Evidence:

- `internal/ledgerexport/export.go:174-193` hashes only `ledger.sqlite` before
  and after reading a transactional snapshot.
- `internal/ledgerexport/export.go:435-452` verifies that same main-file hash
  when the high-water mark is unchanged.
- `internal/ledgerexport/export.go:717-730` implements the identity as bytes
  from the main file only.
- `internal/artifactretention/plan.go:125-149` and
  `internal/artifactretention/apply.go:170-191` use the same physical-file
  assumption for GC plans and action revalidation.

In WAL mode, committed logical rows may live in `ledger.sqlite-wal` while the
main file remains byte-identical. Conversely, a checkpoint can change the
main file bytes without changing the logical database at all. The current
identity can therefore miss concurrent WAL changes, accept a same-high-water
logical divergence, or reject a logically identical ledger after a harmless
checkpoint. This contradicts the manifest's claim that `SourceLedger` is exact
source authority.

Recommended resolution:

- Derive authority from the transactional logical snapshot (for example, a
  canonical hash over all run/event fields and exact payload bytes), or create
  and verify a real SQLite snapshot that includes WAL state.
- Treat a raw main-file hash as diagnostic only unless all sidecars and a
  stable checkpoint boundary are part of the identity.
- Add WAL tests with `wal_autocheckpoint=0`, uncheckpointed committed rows, a
  no-op checkpoint, same-high-water data divergence, and concurrent commits.

### R2-02 — Artifact-retention mutation exclusion is a check-then-release race

Evidence:

- `internal/artifactretention/apply.go:448-482` holds the retention and
  autonomous-execution locks, but merely acquires and immediately releases the
  Git-admin and child-publication locks.
- The same function reads source-writer metadata once and does not retain an
  exclusion primitive.
- `internal/autonomouschild/child.go:456-476` and the archive/source-writer
  entry points do not acquire or probe `artifact-retention.lock`.

A child publication, archive operation, or mixed-pass source writer can start
after retention's probe and before a prune. Per-action pin revalidation does
not close the window between the last scan and the rename. GC can therefore
overlap with a new control reference or artifact producer despite the stated
mutual-exclusion contract.

Recommended resolution:

- Establish one documented lock order and retain every relevant exclusion for
  the entire mutating GC transaction, or require every competing entrant to
  acquire/probe the outer retention lock before admission.
- Add deterministic barrier tests that start child publication, archive, and
  source-writer admission immediately after GC's current probe and prove that
  one side blocks before any mutation.

### R2-03 — GC resume trusts mutable journal claims instead of immutable history/effects

Evidence:

- `internal/artifactretention/apply.go:68-78` returns replayed success solely
  from `journal.Stage == "cleaned"`.
- `internal/artifactretention/apply.go:93-121` trusts `CompletedPaths` to skip
  actions and any nonempty `ExportID` to skip export verification.
- `internal/artifactretention/apply.go:130-138` allows destructive prune when
  that unverified `ExportID` is nonempty.
- `internal/artifactretention/apply.go:400-445` validates canonical JSON,
  schema, and operation ID, but does not validate the state machine or
  reconstruct authority from immutable history.

A canonical but corrupted/tampered mutable journal can insert a fake export ID
and bypass the required verified-export gate, or insert action paths in
`CompletedPaths` and skip work that never happened. Immutable history is
written but is not consulted on load.

Recommended resolution:

- Reconstruct the latest valid journal from a contiguous immutable history
  chain and treat the mutable journal as a cache.
- Validate stage/sequence transitions, plan identity, completed-action prefix,
  export identity, cancellation state, and timestamps.
- Before every resumed prune, re-verify/replay the recorded export. Reconcile
  every claimed completed effect against the filesystem before skipping it.
- Test missing/stale/ahead/conflicting checkpoints, fake export IDs, reordered
  or duplicate completed paths, and a `cleaned` claim with unapplied effects.

### R2-04 — Child publication can claim completion for the wrong or empty child set

Evidence:

- `internal/autonomouschild/child.go:173-185` compares only the mutable
  journal's material hash before accepting `StageCompleted`; it then loads the
  journal-supplied `Children` rather than comparing them with the newly
  projected records.
- `internal/autonomouschild/child.go:347-377` reads only schema validity; it
  does not validate operation/parent/decision/proposal identity, stage,
  sequence, child records, canonical bytes, or immutable history.
- `internal/autonomousscheduler/repository.go:48-60` trusts a minimal
  schema/operation/stage projection of that same checkpoint as scheduling
  authority.

For example, changing an admitted checkpoint to `stage: completed` and
`children: []` while retaining the material hash makes `Apply` return replayed
success without publishing the expected children. The scheduler also has no
independent child-set or history check.

Recommended resolution:

- Add a complete `Journal.Validate`, compare the loaded journal exactly with
  the deterministic projected child records and immutable input identities,
  and reconstruct the latest contiguous transition from history.
- Have the scheduler consume a shared validated publication projection rather
  than decode three fields independently.
- Add corruption/recovery tests for every field, empty/substituted children,
  missing/behind/ahead checkpoints, history gaps, and incomplete publication.

### R2-05 — Queue and child stores do not use the protected runtime-path contract

Evidence:

- `internal/autonomousqueue/store.go:37-89` reads checkpoint/history paths with
  `os.ReadFile`/`os.ReadDir` without protected final-file checks.
- `internal/autonomousqueue/store.go:92-178` uses a local symlink-only ancestor
  check around lock and persistence operations.
- `internal/autonomouschild/child.go:347-440` similarly uses `os.MkdirAll`,
  `os.ReadFile`, `os.CreateTemp`, `os.Rename`, and `os.Link` without the
  `internal/runtimepath` identity, mode, hard-link, and opened-file checks.

This is an omission relative to the hardened task-run and outer execution
stores. Final symlinks/hard links, unsafe modes, wrong file types, and
ancestor-substitution races can still cross or corrupt the runtime trust
boundary in queue/child operations.

Recommended resolution:

- Reuse `internal/runtimepath` for queue and child directories, checkpoint,
  history, lock, temp, and cleanup operations. Remove the local weaker path
  helpers if this produces equivalent behavior and a net production-LOC
  reduction.
- Add outside-root sentinel tests for every ancestor/final-file substitution,
  hard links, unsafe modes, and rename/open race boundary.

### R2-06 — Queue recovery does not validate a history chain

Evidence:

- `internal/autonomousqueue/store.go:37-70` accepts the checkpoint plus every
  `.json` history file, sorts them by embedded sequence/stage, and selects the
  largest isolated valid record.
- It does not require canonical history filenames, a contiguous sequence,
  legal transitions, consistent immutable material, or history backing for a
  checkpoint.
- `internal/autonomousqueue/store.go:92-95` permits equal sequences and checks
  only `Operation.Validate`, not a `previous -> next` transition.

A lone terminal checkpoint or injected high-sequence history record can become
authoritative and cause terminal replay/ledger completion without the claimed
transition history. This is the same class of mutable-cache/immutable-history
authority bug already fixed in notification persistence.

Recommended resolution:

- Parse history in canonical filename/sequence order, validate each legal
  transition and immutable material, derive authority from the latest complete
  history entry, and accept the checkpoint only when equal or behind.
- Add tests for missing history, gaps, duplicate sequence/stage, illegal stage
  jumps, checkpoint ahead/behind/conflicting, and foreign canonical JSON.

### R2-07 — Prune renames are not durably synchronized at the destination

Evidence:

- `internal/artifactretention/apply.go:323-384` renames each source into a
  quarantine directory, then syncs only `filepath.Dir(abs)` (the source
  directory).
- The destination directory and newly created destination-parent chain are not
  synchronized before the completed-action journal is published.

Cross-directory rename durability requires both directory sides to be made
durable. A crash can otherwise lose the quarantine entry while the source
removal and later journal transition survive, defeating resumable recovery.

Recommended resolution:

- Sync every affected source and destination directory after renames and
  before persisting the action as complete; sync cleanup parents after
  `RemoveAll` as well.
- Add crash-boundary tests between each rename, each directory sync, journal
  publication, and quarantine cleanup.

### R2-08 — Ledger export can emit records that its verifier refuses to read

Evidence:

- `internal/ledgerexport/export.go:276-310` appends arbitrarily sized run and
  event records to the export stream without a per-record bound.
- `internal/ledgerexport/export.go:596-655` reads records with a hard 16 MiB
  scanner limit.
- Task text and ledger event payloads have no matching 16 MiB admission limit.

An export containing one large task or event can be published successfully
and then fail `Verify`/`ReplayValidate` with `bufio.ErrTooLong`. Because the
export is immutable, retries reproduce the same unusable artifact and a GC
operation requiring it cannot advance.

Recommended resolution:

- Use one explicit writer/reader record contract. Either reject oversize
  records before publishing anything, or replace the scanner with a bounded
  streaming reader whose supported maximum matches admitted ledger data.
- Add exact-limit, limit-plus-one, large task, and large event tests proving
  every successful export verifies and replay-validates.

### R2-09 — Explicit `verification.commands: []` is treated as omission

Evidence:

- `internal/app/config.go:482-495` applies configured commands only when the
  pointer is nonnil *and* the list is nonempty.
- `internal/runonce/runonce.go:668-670` interprets nil commands as permission to
  synthesize the default `go test ./...` command.

The pointer representation preserves omitted-versus-explicitly-empty state,
but `apply` discards that distinction. An operator asking for an empty command
set (to exercise `missing_policy`/preflight behavior) silently receives the Go
default instead.

Recommended resolution:

- When `commands` is present, always replace the base commands, including with
  a nonnil empty slice, and clear any inherited tiered plan as appropriate.
- Test omitted, `null`, empty, and nonempty commands against both nil and
  preconfigured bases.

### R2-10 — Bare `revolvr run` exits successfully with a placeholder

Evidence:

- `internal/cli/root.go:1053-1075` models five mutually exclusive run modes.
- `internal/cli/root.go:1156-1162` falls through to `runPlaceholder` when none
  is selected.
- `internal/cli/root.go:1435-1437` prints “is not implemented yet” and returns
  nil when writing succeeds.
- Focused smoke confirmed `go run ./cmd/revolvr run` exits zero with that text.

This is misleading for a mature command and can make automation treat a
non-operation as success.

Recommended resolution:

- Either make bare `run` the documented one-pass default, or return a clear
  nonzero error requiring one of the mode flags. Remove the now-single-use
  placeholder helper.
- Add exit/output tests for the no-mode case.

### R2-11 — Artifact-GC result rendering drops or masks errors

Evidence:

- `internal/cli/root.go:227-231` discards `writeGCResult` errors during resume.
- `internal/cli/root.go:250-254` returns a rendering error instead of retaining
  both the apply error and rendering error when both occur.

A broken stdout pipe can therefore produce a successful resume exit despite
missing output, or hide the more important mutation failure during apply.

Recommended resolution:

- Return `errors.Join(operationErr, writeErr)` whenever both results exist,
  while still rendering resumable journal evidence when possible.
- Add failing-writer tests for successful and failed apply/resume paths.

## Verification performed

All checks below passed unless explicitly noted:

- `go test -count=1 ./...`
- `go test -race -count=1 ./...`
- `go test -shuffle=on -count=1 ./...`
- `GOTOOLCHAIN=go1.22.12 go test -count=1 ./...`
- `go vet ./...`
- `govulncheck ./...` — no reachable vulnerabilities
- `gofmt -l` — no files reported
- `go mod tidy -diff` — no diff
- `go mod verify`
- `git diff --check`
- CLI help, `config check`, and `status` smokes

The bare `revolvr run` smoke intentionally exposed R2-10; it exited zero and
printed `revolvr run is not implemented yet.`

## Refactoring and efficiency conclusion

No broad package or TUI refactor is recommended from this pass. The safest
line-count reductions are incidental to the fixes above: reuse
`internal/runtimepath` instead of local weaker helpers, share validated
history reconstruction rather than duplicating partial decoders, and remove
the obsolete `runPlaceholder`. Streaming ledger export would also reduce peak
memory, but it should be done only as part of R2-08 so the record contract and
atomic publication semantics remain explicit.
