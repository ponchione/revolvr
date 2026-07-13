# Agent State

## Current Focus

The second 2026-07-13 wide-sweep audit is registered as R2-01 through R2-11 in
`.agent/TASKS.md`. R2-01 through R2-07 are complete; R2-08 is the next bounded
follow-up. The detailed report remains `CODEBASE_AUDIT_2026-07-13.md` until all
eleven items are complete. The prior AUD-01 through AUD-16 queue and the
ordered AW-01 through AW-31 autonomous workflow program remain complete and
published.

## R2-07 Completion (2026-07-13)

- Selected task: R2-07 — make retention quarantine renames and removal durable
  on every affected directory side. No R2-08 ledger-export size work or later
  audit item was started.
- After a prune moves any original, gzip, or manifest representation, GC now
  synchronizes the source directory and every destination directory from the
  quarantine leaf through the operation directory before publishing action
  completion. This makes both rename sides and newly created parent entries
  durable.
- A filesystem-ahead retry no longer returns merely because quarantine evidence
  already exists. It repeats all source/destination directory syncs before it
  may advance the journal, closing the crash-after-last-rename window.
- After quarantine `RemoveAll`, GC synchronizes the operation directory before
  publishing the `cleaned` journal. A crash after removal or its parent sync
  resumes safely from the durable `completed` state.
- Deterministic apply failure points cover both representation renames, the
  source sync, each destination-chain sync, action/completed journal
  publication, quarantine removal, cleanup-parent sync, and cleaned-journal
  publication. Every injected boundary resumes to verified cleaned state.
- Files changed: `internal/artifactretention/{apply.go,durability_test.go}`;
  `README.md`; and `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: focused artifact-retention tests; the complete
  durability/crash matrix repeated three times; `go test -race -count=1
  ./internal/artifactretention`; `go test -count=1 ./...`; `go vet ./...`; and
  `git diff --check`.
- What remains: R2-08 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-06 Completion (2026-07-13)

- Selected task: R2-06 — make queue recovery authority a contiguous legal
  immutable-history chain. No R2-07 durability work or later audit item was
  started.
- Queue history is sorted and consumed only by exact canonical
  `<sequence>-<stage>.json` names. It must begin with a pristine admission at
  sequence zero and remain gap-free, duplicate-free, and free of foreign
  entries.
- Every adjacent record now validates exact sequence progression,
  nondecreasing time, immutable operation/configuration material, legal queue
  stage and evidence changes, terminal finality, and the one supported v1-to-v2
  migration. Parallel batches retain their legal repeated `task_stopped`
  transitions, each backed by exactly one newly reconciled slot and outcome.
- Immutable history is the authority. A missing checkpoint and an exact stale
  checkpoint recover to the latest history record; a lone, ahead, unbacked, or
  conflicting checkpoint fails closed. Persistence validates the complete
  existing chain and current predecessor before publishing the next record,
  and equal-sequence transitions are rejected.
- Files changed: `internal/autonomousqueue/{store.go,history_test.go,
  path_safety_test.go,run_test.go}`; `README.md`; and
  `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: focused queue tests; R2-06 corruption/recovery tests
  repeated 10 times; `go test -race -count=1 ./internal/autonomousqueue`;
  `go test -count=1 ./...`; `go vet ./...`; and `git diff --check`.
- Coverage includes absent history, a lone terminal record, sequence gaps,
  duplicate sequences, illegal stage jumps, changed immutable material,
  foreign canonical JSON, filename/content mismatch, missing/equal/stale/ahead/
  conflicting checkpoints, equal-sequence persistence, protected rename fault
  tests, legacy replay/migration, and parallel crash recovery.
- What remains: R2-07 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-05 Completion (2026-07-13)

- Selected task: R2-05 — apply the established protected runtime-path contract
  to autonomous queue and child-publication persistence. No R2-06 history-chain
  semantics or later audit item was started.
- `runtimepath` now provides identity-checked protected file reads, directory
  enumeration, opened-directory checks, and directory sync. File/directory
  descriptors must match their safe named component before and after use;
  final symlink races use no-follow opens, and missing paths remain explicit.
- Queue inspect, recovery, history creation/readback, checkpoint temp/rename,
  directory sync, and operation locks now use canonical roots plus complete
  ancestor, type, mode, link-count, named/opened identity, and final-file
  checks. The local symlink-only walker and duplicate canonical-root helper are
  removed. Checkpoints and immutable records are revalidated at every open and
  rename boundary, and temp cleanup refuses a substituted parent.
- Child publication uses the same contract for its global lock, publication
  directories, immutable histories, mutable checkpoints, initial child states,
  state readback, temp/link/rename operations, and directory sync. The shared
  R2-04 authority loader uses protected checkpoint/history directory and file
  handles, so scheduling cannot consume an unsafe alias.
- Files changed: `internal/runtimepath/{runtimepath.go,runtimepath_test.go}`;
  `internal/autonomousqueue/{run.go,store.go,path_safety_test.go}`;
  `internal/autonomouschild/{child.go,child_test.go,path_safety_test.go}`;
  `internal/autonomouschildpublication/publication.go`; `README.md`; and
  `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: focused runtime-path, queue, child-publication, child,
  scheduler, task-run, and app tests; path-substitution tests repeated 10 times;
  affected-package race tests; `go test -count=1 ./...`; `go vet ./...`; and
  `git diff --check`.
- Coverage checks every queue/publication namespace ancestor; symlink, hard
  link, directory, FIFO, wrong-type, and unsafe-mode final files; checkpoint
  and immutable-history reads; opened lock replacement; checkpoint rename and
  immutable link replacement; and parent substitution during temp cleanup.
  Every case preserves exact outside sentinel bytes, mode, link count, and
  directory contents.
- What remains: R2-06 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-04 Completion (2026-07-13)

- Selected task: R2-04 — make child-publication recovery and scheduling use
  one complete validated authority. No later R2 item was started.
- New shared `internal/autonomouschildpublication` contracts validate every
  journal and child-record field, deterministic child IDs and canonical paths,
  nonempty strictly ordered child sets, exact stage/sequence pairs, UTC
  creation time, and immutable transition authority. Canonical strict JSON and
  full history wrappers are required.
- Load reconstructs the latest state from a contiguous four-stage immutable
  history. Missing checkpoints and exact backed stale checkpoints recover;
  ahead, conflicting, unbacked, noncanonical, gapped, or authority-divergent
  state fails closed. The mutable operation journal is only a cache.
- Publisher replay recomputes the exact deterministic tasks, initial states,
  records, and material hash from the current input. It compares every journal
  authority field before replay or continuation and re-reads exact state/task
  bytes before claiming task publication or completion. A completed checkpoint
  can no longer substitute or erase children while retaining a material hash.
- The scheduler no longer decodes three checkpoint fields. It consumes the
  shared history-backed projection and binds each active child to exact
  publication membership, task/state paths, parent/decision/proposal identity,
  immutable `ChildOf` lineage, supervisor run/evidence/behavior metadata, and
  the deterministic initial state identity. Missing lineage or incomplete
  publication is never schedulable; evolved state remains supported because
  lineage is transition-immutable.
- Files changed: new `internal/autonomouschildpublication` package and tests;
  `internal/autonomouschild/{child.go,child_test.go}`;
  `internal/autonomousscheduler/repository.go`; `README.md`; and
  `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: focused publication, child, scheduler, task-run, and app
  tests; corruption/recovery tests repeated 10 times; affected-package race
  tests; `go test -count=1 ./...`; `go vet ./...`; and `git diff --check`.
  Coverage includes every journal/child field, immutable-field equality,
  empty/substituted/duplicate/reordered children, missing/stale/ahead/
  conflicting checkpoints, history gaps and divergence, noncanonical data,
  partial publication, missing state, stripped lineage, and scheduler recovery
  from missing or stale checkpoints.
- What remains: R2-05 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-03 Completion (2026-07-13)

- Selected task: R2-03 — reconstruct and validate artifact-GC recovery
  authority from immutable history and observed effects. No later R2 item was
  started.
- GC load now reconstructs the authoritative state from exact, contiguous
  immutable snapshots beginning with admission. A missing checkpoint is
  recoverable; a stale checkpoint is accepted only when byte-equivalent to its
  backing history entry. Abandoned regular atomic-write temporaries are not
  committed authority; ahead, conflicting, unbacked, gapped, or other foreign
  history/checkpoint state fails closed.
- Every snapshot and transition validates the schema, operation and exact
  plan, sequence, nondecreasing UTC timestamp, legal stage and cancellation
  state, immutable export identity, and the exact mutating-action prefix.
  Persistence validates both states and publishes history before refreshing
  the mutable checkpoint cache.
- Apply and inspect reconcile every claimed compression or prune with the
  planned source identity and exact observed representation. Terminal cleanup
  may be physically ahead of a `completed` snapshot, but a `cleaned` claim
  requires an absent quarantine; unapplied or conflicting effects are never
  skipped or reported as replayed.
- Every recorded prune export is cryptographically verified and logically
  replay-validated again, then bound to the exact plan operation, time, policy,
  bounds, high-water mark, and WAL-safe logical source identity before prune
  can resume or terminal replay can succeed.
- Files changed: `internal/artifactretention/{apply,journal}.go`, retention and
  journal recovery tests, `README.md`, and
  `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: focused artifact-retention tests; recovery corruption
  tests repeated 10 times; race-enabled artifact-retention and ledger-export
  suites; `go test -count=1 ./...`; `go vet ./...`; artifact-GC CLI help; and
  `git diff --check`.
  Coverage includes missing/stale/ahead/conflicting checkpoints, abandoned
  history temporaries, missing and gapped history, reordered/duplicate
  completed paths, cancellation/timestamp/stage violations, fake and
  valid-wrong-authority exports, unapplied cleaned claims, positive terminal
  replay, and tampered terminal exports.
- What remains: R2-04 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-02 Completion (2026-07-13)

- Selected task: R2-02 — make artifact-retention exclusion race-free across
  every competing mutator. No later R2 item was started.
- Mutating GC now creates and retains all four admission locks for the entire
  transaction in the documented order: artifact retention, nonwaiting
  autonomous execution, nonwaiting Git administration, then nonwaiting child
  publication. Partial acquisition releases in reverse order. An absent inner
  lock file therefore cannot turn a probe into an unprotected race window.
- Every direct/control-root and autonomous-workspace source writer now holds a
  shared control-root artifact-retention gate for its complete lease. GC takes
  that gate exclusively before inner locks. Shared admission preserves
  independent workspace concurrency; release drops the gate even when source
  metadata cleanup fails, while GC's locked fallback scan rejects still-live
  control-root or workspace metadata.
- Deadlock avoidance is explicit: retention is GC's only waiting acquisition;
  inner coordinator/admin/publication locks are nonwaiting. Thus an existing
  autonomous owner that reaches source admission causes GC to release its
  outer gate before the source writer proceeds.
- Files changed: `internal/artifactretention/apply.go` and retention tests;
  `internal/lock/source_writer.go` and tests; child-publication and archive
  admission tests; `README.md`; and `.agent/{TASKS,STATE,DECISIONS}.md`.
- Verification passed: affected-package baseline and focused tests; the new
  barrier/admission tests repeated 10 times; `go test -count=1 ./...`; focused
  race tests for retention, locks, execution, child, archive, mixed-pass,
  supervisor, and autonomous-cycle packages; `go vet ./...`; and `git diff
  --check`.
- Coverage acquires the complete GC lease set as the deterministic barrier,
  then proves archive execution admission fails, Git-admin and child locks
  remain contended, and source admission times out without publishing
  metadata. Actual archive and child operations retain task/HEAD/journal state
  while blocked. Reverse-direction tests prove both control and workspace
  writers exclude GC, legacy/live metadata remains a fallback, independent
  workspace writers coexist, and all admissions proceed after release.
- What remains: R2-03 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## R2-01 Completion (2026-07-13)

- Selected task: R2-01 — replace raw SQLite main-file hashes with WAL-safe
  logical ledger authority. No later R2 item was started.
- A shared `revolvr-ledger-logical-snapshot-v1` identity now hashes a canonical
  length-prefixed projection of the high-water mark, every ordered run field,
  and every event ID/run/type/time plus byte-exact payload. Explicit presence
  markers distinguish absent optional values, and time locations normalize to
  the same UTC instant.
- Ledger exports tag new source identities with that logical schema and bind
  the tag through the recomputed export authority ID. Untagged existing v1
  manifests remain readable, but their physical SQLite hash is diagnostic
  only; it is never accepted as same-high-water logical authority.
- GC plans bind the same tagged logical identity. Planning compares coherent
  snapshots on both sides of inventory while separately retaining file-inode
  safety checks; apply recomputes the logical identity before every mutation.
  SQLite checkpoints therefore do not invalidate unchanged authority, while
  same-high-water row or exact-payload divergence does.
- Files changed: `internal/ledger/snapshot_identity.go` and tests;
  `internal/ledgerexport/export.go` and tests;
  `internal/artifactretention/{apply,compression,plan}.go` and retention tests;
  `README.md`; `.agent/{TASKS,STATE,DECISIONS}.md`; and the registered audit
  report.
- Verification passed: focused tests for `internal/ledger`,
  `internal/ledgerexport`, and `internal/artifactretention`; WAL export tests
  repeated 20 times; WAL retention tests repeated 20 times; `go test -count=1
  ./...`; `go test -race -count=1` for all three touched packages; `go vet
  ./...`; and `git diff --check`. New coverage proves uncheckpointed committed
  rows, physical-byte-changing checkpoints, same-high-water WAL divergence,
  concurrent commits, every logical field, and byte-exact payload identity.
- Verification note: one earlier race run hit an existing timing-sensitive
  busy-reader assertion; its exact test then passed 10 race repetitions and
  the final touched-package race suite passed cleanly.
- What remains: R2-02 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## Second Wide-Sweep Audit (2026-07-13)

- Selected task: perform a new wide-sweep audit after completion of the prior
  AUD-01 through AUD-16 queue. No product-code fix was started.
- Files changed: `CODEBASE_AUDIT_2026-07-13.md`, `.agent/TASKS.md`, and
  `.agent/STATE.md`.
- Result: confirmed 11 actionable findings: five high-priority integrity or
  concurrency defects, four medium-priority durability/configuration defects,
  and two low-priority CLI defects. The highest-risk themes are WAL-safe ledger
  identity, retention exclusion/recovery, and child/queue persistence
  authority. No broad speculative refactor was recommended.
- Verification passed: `go test -count=1 ./...`; `go test -race -count=1
  ./...`; `go test -shuffle=on -count=1 ./...`; Go 1.22 compatibility tests;
  `go vet ./...`; `govulncheck ./...` with no reachable vulnerabilities;
  formatting, tidy, module, and diff checks; and CLI help/config/status smokes.
- Targeted observation: bare `revolvr run` exited zero and printed an
  unimplemented placeholder, confirming R2-10.
- What remains: R2-01 through R2-11, one bounded task per fresh pass.
- Blockers: none.

## Wide-Sweep Audit Queue (2026-07-13)

- The audit itself was read-only. It found 4 high-priority correctness and
  integrity risks, 8 medium-priority fault-path/configuration risks, and 4
  safe or conditional efficiency/cleanup opportunities.
- Work is deliberately split into 16 bounded items. Correctness and evidence
  integrity precede performance, deduplication, and line-count cleanup.
- AUD-06 depends on AUD-05's authoritative JSONL record contract. AUD-15
  depends on AUD-11's filesystem hardening and must be skipped if it cannot
  prove both semantic equivalence and a net production-LOC reduction.
- Audit verification passed: `go test -count=1 ./...`, `go test -race -count=1
  ./...`, shuffled tests, Go 1.22 compatibility tests, `go vet ./...`,
  `govulncheck ./...` with no reachable vulnerabilities, module verification,
  tidy/format/diff checks, and CLI help/config/status smokes.
- There are no recorded blockers. Each fresh autonomous pass must take exactly
  one `AUD-*` item and update the durable records before stopping.

## AUD-01 Completion (2026-07-13)

- Selected task: AUD-01 — close ignored-file gaps in source and verification
  evidence. No later audit item was started.
- Source snapshots now inventory ignored paths with NUL-delimited Git output,
  classify safe relative paths without reading contents or following symlinks,
  and fail with `ErrPolicyRelevantIgnored` for every non-allowlisted ignored
  source or verification input.
- The only ignored-state allowance is explicit control-repository
  `AllowHarnessRuntime` authority for safe `.revolvr` directories/regular
  files. Autonomous task worktrees do not receive that allowance. Tracked or
  unignored `.revolvr` paths remain ordinary source evidence rather than being
  excluded by pathname.
- Cycle captures enforce the invariant before supervision/model admission,
  after worker execution, after verification, and after commit. Workspace
  reopen/checkpoint and finalization revalidation use the same capture, so an
  unexplained ignored input cannot advance a checkpoint or completion claim.
  Existing cleanup refusal retains its typed `ErrCleanupRefused` authority.
- Policy source revisions now normalize regular-file permissions to executable
  bits. Full snapshots still detect all permission changes, while identical
  committed bytes replay to the same policy revision across checkout umasks.
- Real-Git coverage includes ignored regular files, collapsed and empty
  directories, nested ignore rules, newline-containing names, symlinks,
  explicit harness runtime state, tracked/unignored `.revolvr` paths,
  pre-existing/worker-created/verification-created inputs, checkpoint refusal,
  secret-safe diagnostics, and clean-clone replay.
- Verification passed: focused affected-package tests; `go test -count=1
  ./...`; and `go test -race -count=1` for `internal/gitstate`,
  `internal/autonomousworkspace`, `internal/autonomouscycle`,
  `internal/autonomouscorrection`, `internal/supervisor`, and `internal/app`.
- Remaining audit work: AUD-02 through AUD-16. Blockers: none.

## AUD-02 Completion (2026-07-13)

- Selected task: AUD-02 — fail closed on source-lock heartbeat failure. No
  later audit item was started.
- A shared source-lease guard now owns the heartbeat monitor, serializes
  heartbeat checks, records the first ownership failure, cancels active work,
  and joins heartbeat and release errors without discarding the primary
  operation error.
- Direct runs and autonomous cycles use the guard's context for model and
  verification work and synchronously prove ownership around verification,
  task transitions, commit, and terminal publication. Autonomous ownership
  loss returns a source-changed result so checkpoint and finalization cannot
  advance.
- Expired leases cannot be revived or released as if they were still owned.
  Token replacement and lock persistence failures are reported as inspectable
  ownership loss while retaining their underlying errors.
- Deterministic tests cover heartbeat I/O and permission failures, expiry,
  replacement-owner tokens, cancellation races, commit prevention, release
  failure, prompt worker cancellation, and healthy monitor shutdown.
- Verification passed: repeated focused lock, run-once, and autonomous-cycle
  tests; affected-package tests; `go test -count=1 ./...`; `go test -race
  -count=1 ./...`; formatting; and `git diff --check`.
- Remaining audit work: AUD-03 through AUD-16. Blockers: none.

## AUD-03 Completion (2026-07-13)

- Selected task: AUD-03 — terminate complete spawned process trees on
  cancellation. No later audit item was started.
- Every production command still converges on `internal/runner`, which now
  starts commands in dedicated process groups on Unix. Cancellation sends
  `SIGTERM` to the captured group, polls during the configured grace interval,
  and sends `SIGKILL` to that same group when descendants remain.
- The runner owns cancellation independently of `exec.CommandContext`, waits
  for its termination monitor before returning, and retains bounded
  stdout/stderr draining, timeout classification, caller-cancellation errors,
  exit-code behavior, and graceful-exit output.
- Platforms without the Unix process-group contract fail before process start
  with typed `ErrProcessTreeUnsupported` evidence. They do not silently fall
  back to unsafe direct-child-only termination.
- Helper-process integration tests exercise a root, child, and grandchild;
  delayed sentinel mutations; inherited output pipes; signal-ignoring
  descendants; bounded force termination; repeated cancellation; graceful
  exit; and an unrelated sibling process.
- Verification passed: ten repeated focused runs; three repeated runner race
  runs; Go 1.22 runner tests; Darwin, FreeBSD, Windows, and Plan 9 runner
  cross-compilation; focused vet; `go test -count=1 ./...`; `go test -race
  -count=1 ./...`; formatting; and `git diff --check`.
- Remaining audit work: AUD-04 through AUD-16. Blockers: none.

## AUD-04 Completion (2026-07-13)

- Selected task: AUD-04 — use safe read semantics for the live SQLite ledger.
  No later audit item was started.
- Raw database callers now use the explicitly live `OpenLiveReadOnly` API with
  SQLite `mode=ro`, ordinary locking/change detection, query-only enforcement,
  and no immutable promise. Immutable evidence remains the independently
  verified ledger export/replay format rather than a raw live database handle.
- Full snapshots, task dossier history, and individual run/event projections
  read all related rows in one read transaction. Dossier, metrics, export,
  archive, retention, and scheduling callers therefore receive coherent
  snapshots while concurrent writers continue safely.
- Live-reader lock contention uses short SQLite busy slices with bounded retry
  up to the existing five-second limit. Caller cancellation interrupts between
  slices promptly and preserves both context and SQLite busy evidence.
- Mutation methods on a live read-only store fail immediately with typed
  `ErrReadOnly` evidence instead of reaching SQLite or a nil writer clock.
- WAL and rollback-journal tests prove same-reader freshness and coherent
  atomic run/event versions under concurrent commits. Read access creates no
  database or sidecar files and changes no durable database/WAL bytes. SQLite
  may update transient reader marks in an existing in-process `-shm` index;
  tests require its presence, size, and mode to remain bounded while preserving
  the locking protocol.
- Verification passed: repeated WAL/rollback focused tests; repeated affected
  package race tests; Go 1.22 affected-package tests; focused vet; `go test
  -count=1 ./...`; `go test -race -count=1 ./...`; formatting; and `git diff
  --check`.
- Remaining audit work: AUD-05 through AUD-16. Blockers: none.

## AUD-05 Completion (2026-07-13)

- Selected task: AUD-05 — preserve long Codex JSONL records without silent
  corruption. AUD-06 metrics parsing was not started.
- Runner stdout now has independent paths for capped in-memory capture,
  bounded human line previews, and an authoritative byte sink. Preview lines
  remain at most 64 KiB including a visible truncation marker, emit once per
  oversized logical line, and resynchronize at its newline without converting
  discarded fragments into new lines.
- `internal/jsonl` owns the shared streaming record contract: records are
  reconstructed across arbitrary chunks, a final unterminated record is
  emitted deterministically, and the documented hard limit is 1 MiB excluding
  the newline delimiter. `ErrRecordTooLarge` and `RecordTooLargeError` retain
  the rejected record number and limit without retaining its content.
- Codex stdout artifacts consume the authoritative stream. Each complete
  record is redacted as one value before persistence or JSON parsing, so split
  secrets and UTF-8 remain deterministic. Records over 64 KiB are complete;
  records over 1 MiB fail the invocation and artifact with typed evidence and
  are never partially persisted as fabricated JSONL. Existing invalid-JSON
  diagnostics and bounded returned stdout remain intact.
- Coverage includes record sizes around 64 KiB and the 1 MiB hard boundary,
  multiple large records, actual process-pipe fanout, arbitrary chunks, split
  newlines/secrets/UTF-8, final records without newlines, invalid JSON, typed
  writer failure propagation, record-by-record artifact decoding, and preview
  resynchronization.
- Verification passed: focused `internal/jsonl`, `internal/runner`,
  `internal/codexexec`, and `internal/runonce` tests; `go test -count=1 ./...`;
  `go test -race -count=1 ./...`; formatting; and `git diff --check`.
- Remaining audit work: AUD-06 through AUD-16. Blockers: none.

## AUD-06 Completion (2026-07-13)

- Selected task: AUD-06 — stream and bound Codex JSONL metrics parsing. AUD-07
  last-message publication was not started.
- `internal/jsonl.ReadRecords` now consumes arbitrary readers in fixed 32 KiB
  chunks, checks cancellation between reads and records, reconstructs final
  unterminated records, and enforces the shared AUD-05 1 MiB record limit.
  Memory is therefore bounded by one record rather than total artifact size.
- `internal/receipt` exposes reader- and file-based metrics parsing and receipt
  rewriting. Production Codex finalization, mixed-pass receipt parsing, and
  autonomous worker receipt parsing all use the file stream; no production
  metrics caller loads the JSONL artifact with `os.ReadFile` or a retention
  read cap. Byte-slice APIs remain compatibility wrappers over the same parser.
- Metrics diagnostics are explicit and non-destructive. Malformed records
  report first record/count while retaining partial totals only as diagnostic
  output; individual and aggregate integer overflow report exact record/field;
  oversized records retain `jsonl.ErrRecordTooLarge`; and open/read failures
  retain typed `ErrCodexJSONLSource` evidence. Receipt rewrite refuses partial
  metrics and returns the original parsed receipt bytes unchanged.
- File parsing owns and closes its descriptor. A cancellation watcher closes a
  blocked source so cancellation returns promptly. Codex classifies source
  access failure as `ArtifactError`, while cancellation, malformed input,
  overflow, and other optional metrics degradation remain `ParseError`; the
  authoritative redacted JSONL is never rewritten or truncated.
- Coverage includes 64 one-megabyte-shaped records, an exact maximum-size
  unterminated record, shared oversize errors, malformed middle records,
  individual and aggregate numeric overflow, blocked-reader cancellation and
  closure, missing/unreadable sources, unchanged malformed artifacts, bounded
  read requests, and valid receipt compatibility.
- A one-shot generated benchmark processed 2,148,532,224 bytes (more than 2
  GiB) in about 0.52 seconds with 9,539,744 bytes allocated. The ordinary 16
  MiB allocation benchmark used 1,732,824 bytes, guarding against whole-file
  buffering regressions.
- Verification passed: repeated shuffled focused tests; focused race tests;
  Go 1.22 focused tests; focused `go vet`; both generated benchmarks;
  `go test -count=1 ./...`; `go test -race -count=1 ./...`; formatting; and
  `git diff --check`.
- Remaining audit work: AUD-07 through AUD-16. Blockers: none.

## AUD-07 Completion (2026-07-13)

- Selected task: AUD-07 — atomically publish redacted last-message artifacts.
  AUD-08 Git-status truncation was not started.
- Codex now receives a deterministic restricted raw staging path beside the
  canonical last-message artifact; the canonical path is never present in the
  child argv. Exact invocation provenance records that actual staging path,
  while result and ledger artifact metadata continue to name the canonical
  destination.
- Preparation removes and directory-syncs any stale canonical, raw temporary,
  or redacted temporary, then precreates the child target as a regular `0600`
  file. This gives restart cleanup a deterministic orphan contract and keeps
  stale output from being mistaken for the current invocation.
- Publication validates the raw file and its restricted mode, redacts the
  complete value, writes a second same-directory temporary, sets the protected
  `0644` published mode, syncs and closes it, atomically renames it, removes the
  raw temporary, verifies the canonical type/mode, and syncs the parent
  directory. Handled failures remove and directory-sync temporary artifacts.
- Compatibility is explicit: an absent or zero-byte child output remains
  missing; returned messages remain trimmed; without a redactor the canonical
  file preserves the child's exact bytes; with a redactor it retains the
  prior normalized message-plus-newline form. Existing canonical output is
  replaced at invocation start.
- Coverage injects failures after child completion and around raw read,
  redaction, temporary write, file sync, rename, and directory sync. Every
  pre-rename failure leaves the canonical path absent; a post-rename directory
  sync failure may leave it present but only with redacted bytes. Tests also
  cover a real configured secret, `0600` raw and `0644` canonical modes,
  unsafe raw-mode rejection, orphan restart cleanup, missing output, byte and
  parsing compatibility, and removal of both temporary paths.
- Verification passed: repeated focused `internal/codexexec` tests; focused
  `internal/codexexec`, `internal/runonce`, `internal/autonomouscycle`, and
  `internal/supervisor` tests and race tests; Go 1.22 full tests; `go vet
  ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; shuffled
  full tests; formatting; and `git diff --check`.
- Remaining audit work: AUD-08 through AUD-16. Blockers: none.

## AUD-08 Completion (2026-07-13)

- Selected task: AUD-08 — make Git-status truncation fail closed. AUD-09
  literal pathspec staging was not started.
- Dirty and changed-file capture now invokes the stable machine contract
  `git status --porcelain=v1 -z --untracked-files=all`. Its strict parser
  preserves arbitrary non-NUL filename bytes, treats the first rename/copy
  path as the destination and the following field as the source, validates
  status/framing, and returns a deterministic byte-sorted unique path set.
- Command failure, timeout, nonzero exit, stdout truncation, stderr truncation,
  or malformed porcelain now makes the capture incomplete. The bounded raw
  preview and truncation counts remain diagnostic evidence, but entries,
  paths, dirty files, and changed files are all empty; no caller can mistake a
  parsed prefix for an authoritative set.
- The preflight worktree check consumes the same capture contract. Autonomous
  workspace ignored-path inspection now uses porcelain-v1 `-z` and the shared
  parser, so newline and raw-byte names are unambiguous. Autonomous archive Git
  results now treat truncation as failure for status and every other parsed
  command, including special unborn-HEAD and missing-object branches.
- Commit refuses captures carrying any integrity error before resolving HEAD,
  staging, or committing, and does not repeat partial paths in its result.
  Existing run-once, autonomous-cycle, correction, workspace, and application
  gates already refuse `CaptureError`; focused integration and race tests prove
  verification, checkpoint, transition, staging, and completion cannot advance
  through incomplete status evidence.
- Real-Git coverage uses 4,000 long untracked paths to exceed the former 256
  KiB cap. At that cap the result is a bounded truncation failure with zero
  paths; at 2 MiB repeated captures produce the exact same complete 4,000-path
  set. A commit-gate fixture proves truncated real output leaves HEAD and the
  index unchanged and invokes no Git command inside the commit component.
- Hostile-path coverage includes modification, deletion, staged rename source
  and destination, untracked newline names, leading/trailing whitespace,
  ignored newline names, embedded quotes/tabs/arrows, and non-UTF-8 filename
  bytes on supported filesystems. Clean and ordinary small captures retain
  their existing path/kind behavior.
- Verification passed: ten repeated focused `internal/gitstate` and
  `internal/commit` runs; focused affected-package tests and race tests; Go
  1.22 full tests; `go vet ./...`; `go test -count=1 ./...`; `go test -race
  -count=1 ./...`; shuffled full tests; CLI help/config/status smokes;
  formatting; and `git diff --check`.
- Remaining audit work: AUD-09 through AUD-16. Blockers: none.

## AUD-09 Completion (2026-07-13)

- Selected task: AUD-09 — stage repository paths with literal Git pathspec
  semantics. AUD-10 notification cancellation handling was not started.
- Every exact machine-generated staging list now invokes Git as
  `git --literal-pathspecs add -- ...`: the primary auto-commit gate,
  autonomous archive staging, and run-once recovery when a blocked task must
  be restaged after a failed commit. Pathless whole-tree `git add -A` remains
  unchanged because it carries no generated pathspec.
- Commit path collection now treats filenames as opaque nonempty strings.
  Deduplication and byte sorting remain deterministic, but leading/trailing
  spaces, tabs, and newlines are no longer trimmed into a different filename.
  Recovery logic recognizes the global Git option when determining whether a
  failed commit had successfully staged changes.
- Real-Git commit coverage captures a complete hostile-name set, creates
  matching decoys after capture, and inspects both `git diff --cached
  --no-renames -z` and `git ls-files -z` immediately after the production add
  and before commit. It proves literal handling for `:name`, a `:(glob)`-like
  name, wildcard characters, spaces, tabs, newlines, leading/trailing
  whitespace, a leading dash, deletion, and rename source/destination paths.
  The final commit tree is exactly the staged tree and the decoys remain
  untracked. Autonomous archive staging has a separate real-Git exact-set
  test, while the existing real-Git failed-commit test exercises restored-task
  restaging.
- Verification passed: focused `internal/commit`,
  `internal/autonomousarchive`, and `internal/runonce` tests; `go test -count=1
  ./...`; `go vet ./...`; a repeated full `go test -race -count=1 ./...`; CLI
  help/config/status smokes; formatting; and `git diff --check`. The first
  full race attempt hit the existing ledger busy-reader test's 250 ms deadline
  under repository-wide race load; that test then passed ten race-enabled
  repetitions in isolation and the complete race suite passed on rerun.
- Remaining audit work: AUD-10 through AUD-16. Blockers: none.

## AUD-10 Completion (2026-07-13)

- Selected task: AUD-10 — preserve notification transition errors during
  cancellation. AUD-11 runtime-path hardening was not started.
- Notification transitions now retain the prior valid journal on every
  pre-history failure. When immutable history was published before a later
  file, checkpoint-replacement, or directory-sync failure, the transition
  rereads the store and returns the valid history-derived newer journal with
  the persistence error. No failed transition replaces known journal evidence
  with a zero value.
- Immutable history publication removes and directory-syncs an incomplete
  final history file after write, file-sync, or close failure. Once a complete
  history file reaches the directory-sync boundary it remains restart
  authority, preserving the established history-before-checkpoint order.
  Journal checkpoint failures likewise reconcile from immutable history, and
  temporary checkpoint files are removed before reconciliation.
- Every cancellation exit now joins the original cancellation with transition
  persistence failure: cancellation before lookup, cancellation after lookup
  but before dispatch, hook cancellation during terminal attempt persistence,
  and cancellation during retry delay. A successful persistence path returns
  the original cancellation sentinel unchanged, preserving operator-visible
  behavior.
- Deterministic fault injection covers history writes, history and journal
  file syncs, both history and journal directory syncs, and journal replacement
  across pre-dispatch, in-delivery terminal, and retry-delay cancellation.
  Each case asserts the returned result, raw checkpoint sequence, immutable
  history sequence, reopened store authority, absence of checkpoint temporary
  files, joined error identities, and successful delivery from a clean
  restart. Separate timing coverage proves ordinary cancellation remains
  resumable in every branch.
- Verification passed: repeated and race-enabled focused
  `internal/autonomousnotification` tests; focused `internal/app` integration
  tests; Go 1.22 focused tests; `go test -count=1 ./...`; `go test -race
  -count=1 ./...`; `go vet ./...`; formatting; and `git diff --check`.
- Remaining audit work: AUD-11 through AUD-16. Blockers: none.

## AUD-11 Completion (2026-07-13)

- Selected task: AUD-11 — harden runtime persistence against symlinked paths.
  AUD-12 configuration parsing was not started.
- A small `internal/runtimepath` safety helper now owns the shared trust
  contract below a canonical repository root. It creates directories one
  component at a time and uses `Lstat` to reject symlinks, non-directory
  ancestors, and group/world-writable directory modes while naming the unsafe
  repository-relative component without following it.
- Protected final files must be regular, not group/world writable, and have
  exactly one hard link. Sensitive opens also prove that descriptor metadata
  is safe and `os.SameFile` matches the named final component before use. The
  same checks run again after lock acquisition and around checkpoint rename
  boundaries to narrow path-substitution windows within the existing
  path-based filesystem architecture.
- Task-run inspection, immutable history creation/readback, mutable checkpoint
  creation/rename/readback, and `operation.lock` now use the contract. The
  outer `autonomous-execution.lock` uses it for every ancestor and the final
  lease before and after `flock`. Existing roots may still be named through a
  repository-root symlink because root authority is canonicalized once; no
  symlink is allowed below that boundary.
- Checkpoint temporary cleanup first validates the still-named temp file. If
  the operation directory was replaced before rename, cleanup leaves the
  original temp in the displaced harness directory rather than following the
  substituted parent and unlinking an outside bait file. Final and parent
  components are revalidated immediately before rename and the checkpoint and
  directory are revalidated again afterward.
- Table-driven tests substitute symlinks at every task-run state ancestor,
  task-run lock ancestor, outer-lease ancestor, and all three final file
  families. Additional cases cover a regular file where a directory is
  expected, directory where a file is expected, FIFO, unexpected hard links,
  unsafe file and directory modes, final replacement before rename, and whole
  operation-directory replacement before cleanup. Every case compares an
  outside sentinel's entries, bytes, mode, and link count before and after.
- Existing task-run exact replay, immutable-history crash recovery, queue
  crash consistency, lock contention/reacquisition, archive, and application
  integrations remain green. The queue store was not changed or weakened.
- Verification passed: ten race-enabled repetitions of `internal/runtimepath`,
  `internal/autonomoustaskrun`, and `internal/autonomousexec`; focused queue,
  archive, and application integrations; Go 1.22 affected-package tests; `go
  test -count=1 ./...`; `go test -race -count=1 ./...`; `go vet ./...`;
  formatting; and `git diff --check`.
- Remaining audit work: AUD-12 through AUD-16. Blockers: none.

## AUD-12 Completion (2026-07-13)

- Selected task: AUD-12 — reject ambiguous YAML and invalid configuration
  numbers. AUD-13 ledger query work was not started.
- Config loading now validates the YAML stream before typed decoding: exactly
  one document is required, an empty or structured second document is rejected,
  and only legal trailing whitespace/comments may follow. The existing
  `KnownFields` decode remains authoritative for the one admitted document.
- All Codex, Git, verification-command, commit, and output numeric overrides
  now preserve omission separately from explicit values. Explicit YAML nulls
  and empty numeric values are rejected rather than collapsing to omission.
  Decoder type/range failures are annotated with their exact dotted/indexed
  YAML field path.
- Positive-only timeouts, output caps, attempt/worker bounds, and retention
  operation caps reject zero and negative values. Intentional nonnegative
  retention ages/counts/sizes and notification retry delay continue to accept
  zero but reject negatives. Existing notification and queue upper bounds are
  reported against their exact config fields.
- Every seconds field decodes as `int64` and is checked against the largest
  whole-second value representable by `time.Duration` before multiplication.
  Notification domain limits remain unchanged and are applied after the
  conversion-safety bound.
- Table-driven tests exercise loader and `config check` errors for every
  affected flat/tiered field with zero, negative, string, null, integer decode
  overflow, maximum valid duration, and first duration overflow values. They
  also cover omissions, an empty second document, a structured second
  document, legal document markers/comments, and stable effective behavior and
  hashing for valid configuration.
- Verification passed: `go test ./internal/app`; `go test ./...`; `go test
  -race ./internal/app`; `go run ./cmd/revolvr config check` with the existing
  effective SHA-256 unchanged at
  `87b4ec3dfa23880d41065622c6c2509c1d9ad93e4b810e71c2039f869652a613`;
  formatting; and `git diff --check`.
- Remaining audit work: AUD-13 through AUD-16. Blockers: none.

## AUD-13 Completion (2026-07-13)

- Selected task: AUD-13 — remove the ledger snapshot N+1 query pattern.
  AUD-14 lock-wait work was not started.
- `ReadSnapshot` now loads ordered runs and all joined events with at most two
  SELECT statements inside the same read transaction. The task-filtered recent
  history reader uses the same two-query shape: its second statement repeats
  the exact task/order/limit selection as a bounded derived table and joins
  events through it, avoiding both per-run queries and an ID placeholder list.
- Run rows are materialized directly in their public order. One run-ID-to-index
  map is used only for lookup while query-ordered event rows append directly to
  each returned run; map iteration never determines output. No second event
  collection duplicates the returned payloads in memory.
- Snapshot events retain ascending global IDs within each run, exact malformed
  `event_data` bytes, nil payloads, and nil event slices for runs without
  events. `MaxEventID` is still the maximum joined event ID. Empty selections
  skip the unnecessary second statement while remaining O(1).
- An instrumented wrapper around the existing SQLite driver proves exactly two
  SELECT calls for one and 1,105 selected runs and for task-history limits of
  1, 1,001, and 2,000. The 1,105-run fixture crosses SQLite's traditional
  variable limit and proves that no placeholder expansion is involved.
- Exact and large fixtures cover tied run ordering, interleaved event IDs,
  missing events, malformed payloads, maximum event identity, repeat
  determinism, limit boundaries, and cancellation during the second query
  without a partial result. The AUD-04 live-reader test now also reads full
  snapshots while a writer advances 120 atomic versions in both DELETE and WAL
  modes, proving compatible transactional visibility.
- `BenchmarkReadSnapshot` covers 10 and 2,000 runs with three events per run
  and reports allocations as well as time. Its one-iteration verification
  completed both sizes successfully.
- Verification passed: five repetitions of `go test ./internal/ledger`;
  focused race and concurrency repetitions; `go test ./...`; `go test -race
  ./internal/ledger`; `go vet ./internal/ledger`; the small/large benchmark
  smoke with `-benchtime=1x -benchmem`; formatting; and `git diff --check`.
- Remaining audit work: AUD-14 through AUD-16. Blockers: none.

## AUD-14 Completion (2026-07-13)

- Selected task: AUD-14 — replace flock busy spinning with cancellable
  backoff. The conditional AUD-15 persistence-consolidation analysis was not
  started.
- The one shared `flockContext` path still performs an immediate nonblocking
  exclusive OS `flock`, so every uncontended autonomous-state transition has
  no timer or scheduling overhead. A busy result now waits on a deterministic
  exponential timer sequence of 1, 2, 4, 8, 16, then at most 20 milliseconds
  between attempts instead of calling `runtime.Gosched` in a tight loop.
- Caller cancellation is checked before each flock syscall and selected while
  every retry timer is live. The cancellation branch stops and, when needed,
  drains its timer before returning the exact context error; no timer survives
  the call. Non-contention flock errors still return unchanged.
- Real-file tests prove exclusive cross-open-file authority, refusal before
  release, acquisition within the bounded retry interval after release, prompt
  cancellation, reuse of the same waiter file after cancellation, and exact
  `EBADF` propagation from a closed descriptor.
- A sustained 400-millisecond contention test pins one scheduler processor and
  compares process CPU time to wall time with a broad CI tolerance that the
  former spin loop exceeds. A 24-waiter real-flock test releases one holder,
  observes every waiter acquire and unlock before its deadline, and collects
  every goroutine result.
- `BenchmarkFlockContext` covers both paths. The verification run measured the
  uncontended path at zero allocations and approximately 241 ns/op, while the
  contended-deadline path waited approximately 2.12 ms/op rather than consuming
  that interval as CPU. These measurements are diagnostic, not runtime policy.
- Verification passed: five repetitions of `go test
  ./internal/autonomousstate`; the focused contention/CPU suite; `go test -race
  ./internal/autonomousstate`; `go test ./...`; `go vet
  ./internal/autonomousstate`; the benchmark with `-benchtime=100x -benchmem`;
  formatting; and `git diff --check`.
- Remaining audit work: AUD-15 and AUD-16. Blockers: none.

## AUD-15 Completion (2026-07-13)

- Selected task: AUD-15 — conditionally consolidate proven persistence
  primitives. AUD-16 dead/no-op-code cleanup was not started.
- The inventory found one exact shared contract: planning, attempt, audit,
  block, finalization, input, optional-role, and workspace commits all replace
  the same canonical autonomous state with a same-directory temporary, file
  sync, locked CAS recheck, atomic rename, directory sync, and strict readback.
  Existing helpers already own safe paths/directories, protected lock opening,
  and immutable exact writes.
- `Store.replaceState` now owns only that atomic replacement sequence and its
  generic fault points. Every owner still performs its own lifecycle
  validation, history ordering and identity construction, transition-specific
  readback predicate, disposition, and recovery decisions at the call site.
- The inventory deliberately excluded autonomous queue and task-run storage.
  They have different history/checkpoint authority, recovery and injection
  ordering, and task-run additionally carries AUD-11 protected-path
  revalidation and cleanup. No generic repository or state-machine framework
  and no dependency were added.
- Focused shared-primitive coverage exercises every temporary-write, file-sync,
  pre-rename, rename, post-rename, directory-sync, and readback fault boundary;
  verifies the authoritative pre/post-rename state, cleanup and retry; and
  simulates mutation between the temporary write and locked CAS recheck. The
  previously missing generic rename/directory-sync/readback injection coverage
  is therefore uniform across all eight owners.
- Production `internal/autonomousstate` Go code decreased from 4,346 to 3,937
  physical lines, a net reduction of 409. The eight owner call sites are each
  one explicit replacement call followed by their original lifecycle-specific
  readback checks.
- Verification passed: the focused shared fault/crash tests; uncached tests for
  every `internal/autonomous...` package; `go test ./... -count=1`; `go test
  -race ./... -count=1`; formatting; LOC comparison; production-diff
  inspection; and `git diff --check`.
- One preceding full race attempt hit the unrelated ledger cancellation test's
  timing branch: it returned the deadline without the expected SQLite busy
  wrapper. Five focused race repetitions passed, as did the subsequent clean
  repository-wide race run; no ledger code was changed in AUD-15.
- Remaining audit work: AUD-16. Blockers: none.

## AUD-16 Completion (2026-07-13)

- Selected task: AUD-16 — remove only demonstrably dead or no-op code. No
  unrelated cleanup or new backlog item was started.
- The safety preflight's `validationCopy` block was wholly behavior-neutral:
  the copy was never observed and its only condition tested the nonempty schema
  literal assigned in the same function. The block and its misleading comment
  were removed without replacing them with another abstraction.
- `fmt` in artifact compression and `encoding/json` in artifact apply were
  imported only through blank package references. Both imports and anchors
  were removed. Autonomous queue's private `classifyEmpty` had one caller and
  never read its `outcomes` parameter, so the parameter and argument were
  removed directly.
- The finalization task and metrics map-key tail assignments were dead after
  their values had already served their real work. The redundant audit-root
  blank assignment was removed while retaining the later root-dependent
  artifact read. Notification `Inspect` still validates intent, payload, and
  journal identity before projecting a summary, but its unused intent return is
  now discarded at binding. Planning-result apply still executes and its error
  remains authoritative while the unused result is discarded at binding.
- No public API, persistence order, validation call, state mutation, dependency,
  or TUI behavior changed. Production Go code under `internal` decreased from
  59,976 to 59,956 physical lines, a net reduction of 20.
- Verification passed: focused uncached tests for `internal/autonomoussafety`,
  `internal/artifactretention`, `internal/autonomousqueue`,
  `internal/autonomousfinalization`, `internal/autonomousnotification`,
  `internal/autonomousmetrics`, `internal/autonomousauditapply`, and
  `internal/app`; `go test ./... -count=1`; `go vet ./...`; `go test -race
  ./... -count=1`; formatting; candidate search; LOC comparison; and `git diff
  --check`.
- AUD-01 through AUD-16 are complete, all queue-exit checks pass, no directly
  discovered correctness regression needs a follow-up, and no audit work
  remains. Blockers: none.

## Final Publication (2026-07-12)

- The operator authorized a final full documentation/hygiene sweep followed by
  one raw-Git commit and push of the complete AW-30/AW-31 worktree.
- Live README guidance now distinguishes terminal replay from unresolved-slot
  recovery and documents default/capped CLI, daemon, and TUI worker behavior.
- Historical dated records remain unchanged where their former forward-looking
  statements accurately describe those earlier task boundaries.
- No temporary/editor residue or unrelated untracked file was present; every
  untracked source/test file belonged to AW-30 metrics.
- Final validation: `git diff --check`, `go test -count=1 ./...`, the previously
  completed full `go test -race -count=1 ./...`, focused vet, and CLI smokes all
  passed before publication.

## AW-31 Completion (2026-07-12)

- Selected task: AW-31 — add bounded parallel queue workers after isolation
  was proven. No unrelated follow-up task was started.
- Configuration/operator contract: new strict `autonomous-queue-policy-v1`
  defaults `maximum_workers` to 1 and caps it at 4. It is part of
  `revolvr-effective-run-config-v5`. Config rejects unknown/schema-invalid,
  zero, negative, and excessive values; CLI `run --queue/--daemon --workers N`
  rejects invalid and duplicate occurrences. TUI `Q` intentionally pins one
  worker. Worker material participates in queue replay compatibility and task
  operation identity.
- Durable coordinator: queue operations advance to
  `autonomous-queue-operation-v2` with ordered batches, contiguous selection
  sequences, deterministic slots, exact task-operation identities, slot
  admitted/terminal state, per-slot outcomes, worker bound, peak activity,
  batch counts, and typed sequential fallback. Complete batches are persisted
  before any goroutine starts. Queue checkpoint/history and queue ledger writes
  remain coordinator-only and history-before-checkpoint.
- Admission: each boundary loads one exact scheduler snapshot in canonical
  order. The first ready task is selected, then every additional candidate is
  reclassified by AW-23 against the exact occupied set containing earlier batch
  selections. Dependency and symmetric/shared-key conflict authority therefore
  prevents overlap. Missing overlap classifier or inability to prove another
  admission records `overlap_authority_unavailable` or
  `no_additional_safe_candidate` and conservatively reduces the batch.
- Execution and failure semantics: only the bounded persisted batch receives
  goroutines and each worker gets queue operation, selection, batch, slot,
  task, and exact AW-22 operation identities. Results reconcile in slot order,
  independent of completion timing. Safe task-local stops preserve peers.
  Queue cancellation cancels all child contexts, waits for every return, and
  persists all outcomes before returning. Safety lets already-admitted peers
  reach their bounded task-run boundary, preserves their evidence, and then
  stops new admission. Coordinator-owned runner panics become typed unsafe
  slot evidence without losing peer evidence.
- Recovery/compatibility: exact retry starts only unresolved slots. Already
  reconciled slots and canonical outcomes are retained; AW-22 replay remains
  the authority for a worker whose task completed before queue reconciliation.
  Existing terminal v1 sequential operations replay as one-worker evidence,
  nonterminal v1 operations migrate forward with their pinned task operation,
  and changed worker material is refused. Queue ledger advances compatibly to
  v3 while v2 remains decodable; ledger export accepts both.
- Shared effects and locks: one outer autonomous-execution lease still admits
  only a direct run or one queue coordinator. Workers use distinct AW-18
  workspaces/source-writer locks; existing Git-admin, state, child-publication,
  and SQLite owner boundaries serialize their shared effects without holding a
  coordinator mutex across model/verification/source work. Archive/reopen now
  take a nonwaiting outer execution lease before Git-admin then task-state, so
  they refuse while a queue/direct coordinator is active. Retention keeps its
  existing retention-then-nonwaiting-execution refusal. Notifications still
  dispatch once from the terminal queue after outer lease release.
- Evidence/rendering: queue ledger schema v3 records worker bounds, ordered
  slots, fallbacks, statistics, and ordered outcomes. AW-30 metrics add maximum
  configured workers, peak active workers, parallel sweeps, batches, and
  fallbacks while marking legacy concurrency omissions; live/export logical
  equality is preserved. CLI summaries identify selection/batch/slot/task/task
  operation and worker statistics. TUI progress can render durable slots.
- Tests added/updated: strict config default/bounds/hash/unknown fields; CLI
  flag propagation/duplicates/summary and daemon propagation; limits 1/2/4;
  deterministic selection/outcome order with inverted completion; dependencies
  and shared conflicts; conservative fallback; cancellation cleanup; panic peer
  preservation; crash after batch admission and after first reconciliation;
  exact unresolved-slot replay; worker replay mismatch; terminal legacy replay;
  ledger deduplication/ordering; metrics concurrency; outer lease nonblocking
  probe; archive refusal; TUI sequential default; and full race execution.
- Verification passed: required focused non-race matrix; `go test -count=1
  ./...`; required focused race matrix; `go test -race -count=1 ./...`; focused
  changed-package `go vet`; `git diff --check`; root/run/metrics help, config
  check, and status smokes. No live model/Codex, hook, network request, daemon,
  archive/retention mutation, queue against this repository, dependency,
  commit, push, reset, clean, restore, checkout, or stash was used.
- Remaining work: none in the AW-01 through AW-31 program.
- Blockers: none.

## AW-30 Completion (2026-07-12)

- Selected task: AW-30 — add autonomous-loop metrics and deterministic
  evaluation scenarios. AW-31 parallel workers were not started.
- Projection ownership: new dependency-free `internal/autonomousmetrics` owns
  strict map-free `autonomous-loop-metrics-v1`, canonical JSON, validation,
  deterministic ordering, logical occurrence deduplication/conflict refusal,
  explicit source references, and legacy/partial omission diagnostics. It
  performs no filesystem, SQLite, Git, model, workflow, notification,
  retention, archive-verification, daemon, or scheduling work.
- Authority and compatibility: metrics consume one `ledger.Snapshot` logical
  shape. `ledgerexport.ReplaySnapshot` first verifies an immutable AW-25 export
  and reconstructs that same public shape. A content-derived logical source
  identity makes equal live and exported histories byte-identical. Relevant
  legacy events remain readable as explicit omissions; unknown current
  schemas/fields/enums, malformed occurrences, duplicate conflicts, and
  corrupted exports fail clearly.
- Versioned owner evidence: task-run and queue ledger events advance to strict
  v2 payloads with exact UTC operation times, full statistics, ordered queue
  outcomes, and bounded task attempt/finding/breaker/finalization evidence.
  Archive v2 events add exact matching terminal/archive times; finalization v2
  adds admitted/terminal authority. Tiered verification now emits versioned v1
  start/tier/rerun/completion events and retains ordinary failures/passes,
  flaky classifications, reruns, timeout/cancellation/missing/runner facts.
  Existing v1 archive/finalization verification remains compatible.
- Metrics semantics: terminal task-operation IDs are the outcome unit and
  `no_task` is omitted; replay/duplicate copies do not enlarge counts. Success
  is completed operations over all counted terminal operations, with explicit
  numerator/denominator. Safety, budget, no-progress, cancellation, max-cycle,
  and unsafe outcomes remain distinct. Attempts retain task/run/occurrence and
  recorded token/duration references; missing tokens are counted, never
  estimated. Audit introductions and explicit resolution dispositions remain
  separate, so clean re-audit cannot erase or resolve history. Archive latency
  uses exact matching owner times only. Queue throughput is sequential tasks
  run over durable nanoseconds; no concurrency metric exists.
- Read-only app/CLI surface: app `ShowMetrics` opens the live ledger immutable
  and read-only or verifies/replays one export, loads configured redaction
  values only to reject secret-bearing output, and invokes the pure projector.
  CLI adds `metrics show`, `--json`, and `--export`; human output uses stable
  ordering, explicit denominators/units/omissions, and no ambient clock. Tests
  prove no SQLite WAL/SHM or runtime entry is created. Status, AW-27 views,
  AW-28 TUI refresh, AW-29 notification outbox, retention, and archives remain
  noninteracting.
- Deterministic evaluation: the no-model source suite covers exactly straight
  success, correction with retained finding evidence, clean re-audit with
  explicit resolution, conditional document/simplify skips, repeated-signature
  no progress, exact needs-input terminal evidence, blocked queue yield followed
  by unrelated completion, and crash-finalization duplicate replay. Fixtures
  use fixed UTC times, stable safe IDs, typed owner payloads, exact action order,
  explicit terminal evidence, and no Codex/network/hook/daemon/retention/
  archive mutation or parallel worker. Existing AW-20 failure-injection tests
  remain the production crash-boundary authority; metrics assert one logical
  completion on replay.
- Tests added: strict/canonical projection decoding, unknown fields/schemas,
  caller ownership, all task stop classes and denominator, duplicate/conflict
  handling, attempt/token/duration, correction/audit/finding/resolution,
  real fake-runner flaky fail/pass/rerun evidence, finalization/archive latency,
  queue throughput, live/export byte equality, corrupted export refusal,
  read-only app behavior, human/JSON/help CLI output, and all eight scenarios.
- Verification passed: required baseline focused matrix; baseline and final
  `go test -count=1 ./...`; final required focused package matrix including
  `internal/autonomousmetrics`; focused changed-package `go vet`; `git diff
  --check`; root/metrics help, metrics human/JSON, config check, and status
  smokes. No live Codex/model, notification receiver, network service, Git
  hook, daemon service, archive/retention mutation, source task, dependency,
  commit, push, reset, clean, restore, checkout, stash, or parallel worker ran.
- Remaining work: AW-31 exclusively owns bounded parallel queue workers.
- Blockers: none.

## AW-29 Completion (2026-07-12)

- Selected task: AW-29 — add configurable, bounded external notification hooks
  for unattended terminal outcomes.
- Ownership and contracts: new dependency-free `internal/autonomousnotification`
  owns strict `revolvr-notification-policy-v1`, canonical map-free
  `revolvr-notification-payload-v1`, deterministic event/delivery identity,
  durable intent/payload/journal/history, bounded execution/retry/restart, and
  read-only inspection. It owns no task lifecycle, scheduling, finalization,
  archive, safety classification, ledger authority, metrics, or concurrency.
- Events and source authority: the exact allowlist is `task_completed`,
  `task_blocked`, `task_needs_input`, `safety_stop`, `queue_drained`, and
  `daemon_failed`. App adapters consume only terminal AW-22 task-run operations,
  terminal AW-24 queue operations, typed AW-17 question history, and bounded
  foreground daemon results backed by an exact durable queue occurrence.
  Legacy reason-only input and daemon failures without durable occurrence
  evidence are refused. Queue task outcomes reuse their exact task-operation
  adapters, so direct, queue, daemon, and replay paths derive the same identity.
- Payload: canonical compact JSON with one newline includes stable event and
  delivery IDs, exact UTC source occurrence, hashed repository identity,
  effective-config and hook-policy identities, typed subject/outcome/stop,
  bounded redacted detail, fixed applicable/inapplicable task/task-run/worker/
  question/queue/daemon/archive/safety/terminal/evidence references, and sorted
  omission facts. Newly completed tasks explicitly omit archive authority.
- Configuration: strict top-level `notifications` YAML is disabled by default
  and part of `revolvr-effective-run-config-v4`. Enabled policy requires an
  explicit event allowlist, exact executable/ordered argv, fixed
  `repository_root` directory, replacement environment names, positive bounded
  timeout/caps/attempts, and bounded retry delay. Unknown fields/events,
  duplicates, unsupported directories, empty executable, invalid bounds, and
  hook environment names outside AW-19 redaction coverage fail closed.
- Permissions and secrets: hooks run through `runner.Command` with no shell,
  canonical JSON stdin, repository-root directory, exact argv, `ReplaceEnv`,
  and only configured environment values. Runtime rejects missing/empty secret
  sources and policy material containing a configured secret. Payload strings,
  captured output/errors, resolved authority display, app diagnostics, and CLI
  evidence are redacted; config/doctor expose names/counts and never values.
- Durable delivery: `.revolvr/autonomous/notifications/<delivery-id>/` contains
  immutable `intent.json`, exact `payload.json`, synchronized immutable
  transition history, canonical replaceable `journal.json`, and a delivery
  lock. Paths reject traversal, unsafe evidence, hard links, writable modes,
  and symlinked runtime namespaces. Payload is durable before invocation;
  history precedes journal replacement; exact material replays and conflicts
  fail explicitly; valid history ahead of a missing/lagging journal is
  reconstructed read-only and rolled forward by the next delivery transition.
- Retry/restart: success is terminal and never intentionally starts another
  process. Nonzero exits and timeouts retry deterministically up to at most five
  configured total attempts; missing executable/secret, invalid material,
  unsafe paths, and conflicts are nonretryable. Cancellation starts no new
  attempt and interrupts runner/delay. An interrupted running attempt is
  recorded as retryable before a bounded retry. Every attempt reuses identical
  payload bytes and delivery ID. External exactly-once is not claimed across
  receiver-success/local-crash; receivers can deduplicate the stable key.
- Integration and locks: direct task and queue wrappers release the global
  autonomous-execution lease before dispatch. Queue-internal task runs do not
  dispatch; the enclosing durable outcomes adapt once after lease release.
  Daemon sweeps dispatch after their queue lease, and daemon failure adaptation
  does not reinterpret cancellation, normal sweep limits, or safety as failure.
  No source/state/Git/admin/child/archive/retention lock is held during a hook.
- Failure isolation and operator surface: notification success/failure is
  journaled and reported through a panic-isolated app observer; CLI renders
  bounded warnings after the unchanged source result. Hook failure cannot
  modify task/state/question/workspace/checkpoint/queue/daemon/archive/Git or
  source ledger authority and never hides the source error. `notification
  list/show` is strict redacted read-only inspection and never retries.
- Compatibility: disabled policy performs no notification lookup, secret load,
  directory/lock/ledger/temp creation, or process start. Existing task/queue/
  daemon, mixed-pass, archive, retention/export, AW-27 views, AW-28 TUI refresh,
  config/status/show behavior remains compatible aside from the intentional
  effective-config v4 identity and additive config/doctor/help output. Outbox
  evidence is deliberately separate from ledger/export/retention schemas.
- Tests added: six-event payload golden hashes, determinism, strict decode,
  redaction, caller-slice ownership, policy defaults/validation/hash sensitivity,
  disabled no-op, exact stdin/argv/directory/replacement environment, payload-
  before-process, timeout/nonzero retry/exhaustion, cancellation, byte-identical
  retry, terminal replay, running/history-ahead restart, symlink refusal,
  configured-secret absence, task safety failure isolation, lease release,
  source byte preservation, app replay deduplication, CLI warning/list, and
  read-only noncreation.
- Verification passed: required AW-28 baseline; required AW-29 focused package
  matrix; repeated focused notification/app/CLI/config tests; `go test -count=1
  ./...`; `go vet ./internal/app ./internal/cli ./internal/runonce
  ./internal/autonomousnotification`; `git diff --check`; root/run/notification
  help, notification list, config check, and status smokes. Doctor rendered the
  deterministic disabled notification fact and failed only the established
  accumulated dirty-worktree check.
- No live Codex/model, real receiver/webhook/network request, repository/Git
  hook, daemon service, archive/retention mutation, metric/evaluation, parallel
  worker, dependency, commit, push, reset, clean, restore, checkout, or stash
  was used.
- Remaining work: AW-30 owns metrics and deterministic evaluations. AW-31 owns
  bounded parallel workers. Neither was started.
- Blockers: none.

## AW-28 Completion (2026-07-12)

- Selected task: AW-28 — add autonomous workflow visibility and safe controls
  to the TUI.
- Information architecture: added additive `6 Workflow` navigation while
  preserving Dashboard, Tasks, Runs, Run Detail, Preflight, Help, Add Task,
  and existing number-key behavior. Tasks can open their exact workflow
  evidence directly; a bounded app selector list includes active tasks and
  tracked archives, including archive-only repositories.
- Projection ownership/rendering: Workflow consumes only the injected AW-27
  `autonomousview.View`. Stable plain-text sections render identity/lifecycle,
  decision/current worker/readiness/why, plan, acceptance, findings,
  attempts/budgets, typed input, verification/audit, workspace/checkpoint,
  terminal/archive, provenance/raw references, and diagnostics. It does not
  parse state JSON or open stores. Explicit none/not-available/unlimited/
  unverified facts remain readable without color.
- Async/selection behavior: selector and evidence loads are Bubble Tea
  commands bound to exact selectors and monotonic request tokens. Stale
  responses are ignored, errors remain visible, explicit refresh refetches,
  selection is preserved by task identity, and an active task transitioning
  into an archive follows the same task ID to the archive selector. Long
  detail uses the shared viewport with page/top/bottom movement and coherent
  narrow wrapping.
- App boundaries: new read-only `ListAutonomousTaskSelectors` performs strict
  task/archive discovery without loading state, verifying archives, or
  creating runtime evidence; display labels and errors use configured-secret
  redaction. New `AnswerAutonomousInput` reloads exact current task/question
  authority, calls canonical AW-17 answer then resume operations with exact
  identity/CAS/provenance, and returns typed partial success when answer
  persistence succeeds but resume fails. Exact retry resumes that existing
  answer without duplication.
- Safe controls: `U` continues to run only the exact selected ready active
  pending autonomous task. `Q` starts the existing bounded sequential AW-24
  queue with 100-task/50-cycle defaults. Both require established preflight,
  share the TUI one-active-operation state, compose the app/global execution
  lease, stream typed progress, retain final stop evidence, refresh status and
  workflow evidence, and use idempotent `c` cancellation that waits for the
  typed terminal result.
- Input control: only current typed questions expose `a Answer`; legacy input,
  archives, missing callbacks, and stale/already-resumed authority fail closed.
  Every ordered option and the recommendation are visible, no option is
  preselected, and submission requires an explicit selection plus a second
  confirmation. TUI operator provenance is explicit. Answer-persisted/
  resume-failed state is shown rather than guessed away.
- Archive/read-only/redaction: Workflow archive display uses strict AW-27 Show
  and says `unverified`; it never runs Verify, create, reopen, finalization,
  workspace cleanup, retention, daemon installation, model work, notification,
  metrics, or parallel execution. Read-only selector/view tests preserve
  bytes, mtimes, directory entries, and cache absence. Raw control selectors
  are not rendered; redacted labels/projections are the display authority.
- Tests added: active/archive selector discovery and redaction; exact answer,
  stale refusal, answer/resume success, persisted-answer recovery; projection
  loading/error-token behavior, active-to-archive preservation, every lifecycle
  label, comprehensive evidence rendering, narrow/resize/scroll behavior,
  explicit option/confirmation semantics, queue progress/overlap/cancellation,
  and CLI callback wiring. Existing TUI snapshots and mixed-pass controls stay
  byte-compatible outside the additive Workflow/help surfaces.
- Files changed for AW-28: `internal/app/autonomous_input.go` and tests;
  `internal/tui/model.go` plus `autonomous_test.go`; `internal/cli/root.go` plus
  `tui_autonomous_test.go`; `README.md`; durable state/handoff/decision files;
  and the consumed AW-28 kickoff prompt deletion.
- Verification passed: mandated AW-27 baseline; required AW-28 focused package
  suite; `go test -count=1 ./...`; `go vet ./internal/app ./internal/tui
  ./internal/cli`; `git diff --check`; root/TUI/task-show help, config check,
  and status smokes. No live Codex/model, network, hook, daemon, archive
  mutation/verification, retention mutation, notification, metric/evaluation,
  parallel worker, commit, push, reset, clean, restore, checkout, or stash ran.
- Remaining work: AW-29 owns external notifications. AW-30 metrics/evaluations
  and AW-31 bounded parallel workers remain untouched.
- Blockers: none.

## AW-27 Completion (2026-07-12)

- Selected task: AW-27 — expose autonomous plans, acceptance, findings,
  attempts/budgets, provenance, and routing explanations through read-only app
  and CLI projections.
- Projection ownership: new dependency-free `internal/autonomousview` owns the
  strict map-free `autonomous-task-view-v1` contract, deep-cloned pure
  projection, deterministic canonical JSON, typed diagnostics, configured
  string redaction, and explicit why-reason precedence. It performs no
  filesystem, Git, SQLite, clock, environment, command, cache, model, or
  mutation work.
- Projection contents: identity/source kind, exact task/state/archive
  identities, lifecycle summary, ordered plan steps, acceptance dispositions,
  audit finding introduction/current audit/resolution facts, total/per-action
  attempts, unset/limited/unlimited retry/elapsed/token/action budgets, attempt
  references and stops, typed/legacy input, current verification/audit,
  workspace/checkpoint, terminal/finalization/archive facts, provenance/raw
  references, and bounded omissions are represented without public maps.
- Why semantics: the view distinguishes `latest_decision`,
  `currently_admitted_action`, `scheduler_readiness`, and
  `next_supervisor_action`. A current admission is shown only when exact typed
  decision evidence and the durable in-flight/terminal lifecycle agree.
  Otherwise the result is `undetermined_requires_supervisor`; lifecycle or plan
  position never predicts the next decision. Terminal archives/tasks report
  no next supervisor action.
- Active selection and snapshot: app `ShowAutonomousTask` duplicate-checks all
  canonical active tasks and strict archive manifests, rejects active/archive
  and multi-archive ambiguity, loads only exact `autonomous-v1` task/state
  authority through `taskfile`/`autonomousstate`, reconstructs committed audit
  history read-only, bounds and strictly reads the latest decision artifact,
  then rechecks task/state/audit/decision identities before returning. A
  changing snapshot fails instead of mixing evidence.
- Archive selection: `autonomousarchive.LoadEvidence` extends strict `Show`
  with exact archived task, terminal state, and completed-only frozen-evidence
  loading. It does not run full `Verify`; views say `unverified` and
  `archive_verification_not_run`. Cancelled/superseded/abandoned archives
  explicitly mark completion evidence not applicable. Completed archives can
  project the frozen exact decision, final verification, and clean audit.
- Sparse/malformed behavior: required canonical task/state/archive bytes fail
  closed. Missing plans, audits, verification, decisions, workspaces, input,
  and provenance remain explicit. Malformed optional committed audit history
  degrades to a diagnostic while valid state still renders; a malformed latest
  accepted decision payload makes the route unknown and cannot admit work.
  Mixed-pass tasks return a clear autonomous-evidence-unavailable view.
- Scheduler explanation: ordinary active views project pending/lifecycle and
  active dependency readiness. They never silently treat strict archive Show
  as verified dependency authority; an archived dependency remains waiting
  with `archive_dependency_unverified` until the separate full verification
  boundary runs. No conflict occupancy or worker is invented by viewing.
- CLI surface: `revolvr task show <active-task-id-or-archive-selector>` renders
  one stable plain-text evidence report; `--json` emits the validated canonical
  projection. `revolvr task why <selector>` renders the focused routing/gate
  explanation. Output uses stable section/order, UTC RFC3339Nano exact times,
  explicit none/unset/unlimited/unverified/not-applicable wording, no color,
  width, locale, relative time, or ambient clock.
- Read-only/redaction: viewing acquires no mutation lease, opens no writable
  ledger, creates no task/state/archive/cache/lock/WAL/receipt/export/sidecar,
  and never invokes archive verification, retention, workspace recovery, Git,
  Codex, verification, audit, notification, metrics, or parallel work.
  Existing AW-19 configured environment secrets are loaded at the app boundary
  and removed from human and JSON projections and returned errors.
- Compatibility: existing task list/status/run show/archive show/verify,
  config/doctor/receipt/TUI and mixed-pass behavior remain unchanged. AW-26
  cache entries are neither populated nor repaired; before/after smokes proved
  `.revolvr/cache/dossier` stayed absent. No dependency or TUI workflow control
  was added.
- Tests added: strict/deterministic projection JSON, unknown fields, caller
  immutability, typed input without recommendation selection, lifecycle why
  states, plan/acceptance/finding/budget facts, redaction, active read-only tree
  identity, malformed optional audit history, malformed latest decision,
  cancellation, hard canonical-state failure, mixed-pass absence, cancelled
  archive omissions/unverified status, active/archive ambiguity, byte-stable
  human/why output, JSON CLI output, all lifecycle rendering labels, and help.
- Verification passed: required AW-26 baseline; required broad AW-27 focused
  packages; repeated `go test -count=1 ./...`; focused changed-package `go
  vet`; `git diff --check`; root/task/show/why help, config check, and status
  smokes; and explicit dossier-cache noncreation proof.
- No live Codex/model, network request, hook, dependency addition, source task
  execution, archive verification/reopen/create, workspace recovery, retention
  mutation, notification, metric/evaluation, parallel worker, commit, push,
  reset, clean, restore, checkout, or stash was used.
- Remaining work: AW-28 owns TUI autonomous workflow visibility and controls.
  AW-29 notifications, AW-30 metrics/evaluations, and AW-31 bounded parallel
  workers remain untouched.
- Blockers: none.

## AW-26 Completion (2026-07-12)

- Selected task: AW-26 — cache immutable dossier context by exact Git/source
  identity and render role-specific supervisor and worker projections.
- Cache ownership: new dependency-free `internal/dossiercache` owns schema
  `revolvr-dossier-cache-v1`, algorithm `git-tree-path-map-v1`, strict typed
  source/key/entry manifests, deterministic repository-map rendering, lookup,
  immutable synchronized publication, and deterministic corruption evidence.
  Entries live only under `.revolvr/cache/dossier/v1/<key>/` and contain
  `manifest.json` plus `repository-map.md`.
- Cache identity and source classification: keys bind canonical hashed control
  and execution roots, exact commit and tree object IDs, producer/schema,
  path/byte bounds, and ordered applicable guidance path/content/size facts.
  Only committed-tree repository-map material is cached. Canonical task/state,
  plan/acceptance/findings/input/attempt/budget/workspace evidence,
  verification/audit/receipts/history, dirty status/diff, profiles, safety,
  config, schemas, routes, and prompts remain live per-call evidence.
- Repository map: assembly uses bounded read-only `git rev-parse` and
  `git ls-tree -r -z` output, never ambient mtimes or worktree content. Paths
  are canonical and sorted; `.git` and `.revolvr` are excluded; Go,
  documentation, configuration, command, and generic facts are conservative;
  symlink and submodule entries are labeled metadata-only. Whole-path caps and
  exact total/included/omitted/truncated facts are visible.
- Cache behavior: cold misses recompute and publish; valid hits return exact
  cached bytes; identical publication replays; changed HEAD/tree, guidance,
  bounds, algorithm, worktree/root identity produce another key. Strict reads
  reject nonregular, symlinked, hard-linked, unsafe-mode, oversized,
  noncanonical, unknown-field, key/source/hash/size/count/token-inconsistent,
  partial, or unsupported entries. Corrupt/unsupported bytes are never used;
  safe Git recomputation proceeds with the original deterministic diagnostic
  retained in the role manifest, without overwriting/deleting the corrupt
  entry.
- Source freshness: existing full source snapshots still bracket the entire
  assembly/supervision window. Assembly additionally reloads canonical task
  and applicable guidance bytes after Git/cache collection, and refuses
  same-HEAD task/guidance mutation. Dirty/untracked/status/diff and workspace
  facts remain live, so a cache hit never authorizes source, task, state,
  profile, safety, config, or route admission.
- Role projection: pure `autonomous.ProjectTaskDossier` and
  `ReprojectTaskDossier` cover exactly supervisor, planner, implementer,
  auditor, corrector, documentor, and simplifier. Existing action/profile
  pairs map fail-closed to one role. Exact task/state/plan/acceptance/Git/
  guidance/map evidence remains in every view; verification, audit/finding,
  recent-history, and legacy projection sections follow an explicit tested
  role matrix with named deterministic omissions. Caller evidence is cloned.
- Size/token facts: schema `autonomous-role-dossier-manifest-v1` records each
  section's exact total/included bytes and items, omissions/reasons, final
  dossier hash/size, and deterministic `utf8-bytes-ceil-div-4-v1` estimates.
  The manifest states that estimates are not actual Codex usage. Exact prompt
  byte size and estimate are recorded separately in supervisor/worker
  provenance; AW-15 receipt accounting remains actual usage authority.
- Exact sent evidence: supervisor runs retain `supervisor-dossier.md` and
  `supervisor-dossier-manifest.json`; workers retain `worker-dossier.md` and
  `worker-dossier-manifest.json`, alongside the existing exact full prompt,
  profile, output schema, output, source, stream, receipt, and provenance
  artifacts. Worker prompt construction validates that dossier role/hash/size
  exactly match the admitted route. Ledger artifact extraction and export now
  preserve the dossier paths; provenance embeds cache result/key/entry
  manifest identity, role/policy, dossier facts, prompt facts, task/source/
  profile/config/safety/run identities.
- Retention/redaction/observability: AW-25 inventory remains ledger-derived and
  does not classify or evict the shared cache namespace. Runs remain
  inspectable/exportable after cache loss because exact dossiers are copied
  into each run. No status/show/config/doctor/TUI/archive/GC read populates or
  repairs the cache; smoke tests proved config/status leave it absent. No new
  operator config was required because projection/cache bounds are fixed
  harness policy in this slice. README documents the behavior and limits.
- Tests added/extended: deterministic key/map/order/classification/bounds;
  miss/hit/replay/HEAD and guidance invalidation; corruption recomputation;
  control-root and cancellation isolation; role snapshots/matrix/unknown
  combinations/caller immutability/cross-task isolation; same-HEAD guidance
  mutation; exact supervisor/worker dossier artifacts, manifest identities,
  prompt inclusion, and ledger extraction/export compatibility.
- Verification passed: required AW-25 baseline; required broad AW-26 focused
  packages; repeated `go test -count=1 ./...`; focused `go vet`; `git diff
  --check`; CLI help, config check, and status smokes; explicit proof ordinary
  config/status reads did not create `.revolvr/cache/dossier`.
- No dependency, live Codex/model, network request, hook, arbitrary repository
  binary, source-task mutation, archive/reopen, cache eviction, notification,
  metric/evaluation, parallel worker, commit, push, reset, clean, restore,
  checkout, or stash was used.
- Remaining work: AW-27 owns broad app/CLI plan/findings/acceptance/why-next
  evidence views. AW-28 TUI workflow controls, AW-29 notifications, AW-30
  metrics/evaluations, and AW-31 parallel workers remain untouched.
- Blockers: none.

## AW-25 Completion (2026-07-12)

- Selected task: AW-25 — artifact retention, compression, garbage collection,
  and deterministic ledger export/replay validation.
- Ownership: new `internal/artifactretention` owns the versioned retention
  policy, exact stream inventory, transitive pin closure, deterministic typed
  plans, deterministic gzip compatibility, mutation lease, quarantine, and
  restartable synchronized GC journal/history. New `internal/ledgerexport`
  owns read-only SQLite snapshot export, immutable canonical JSONL/manifest
  publication, verification, and logical replay; live ledger row deletion was
  deliberately not added.
- Policy/config: schema `revolvr-artifact-retention-policy-v1` is harness-only,
  map-free, strict YAML and part of `revolvr-effective-run-config-v3` identity.
  Defaults disable mutation, pin 20 recent runs, compress after seven days,
  retain for 90 days, admit only Codex JSONL/stderr, require verified export
  for pruning, and bound operations to 100 files/1 GiB. Config check and doctor
  expose the effective nonsecret policy; negative, overflowed,
  contradictory, unknown, and destructive-without-export values fail.
- Inventory/pins: candidates originate only from exact ledger artifact events
  under `.revolvr/runs/<run-id>/`, with regular-file/mode/link/path, SHA-256,
  byte-size, and frozen-mtime checks. Active task identity conservatively pins
  every associated run; nonterminal and recent runs pin directly; canonical
  autonomous/archive/queue/task-run/child/completion/recovery JSON references
  pin exact paths and referenced run IDs transitively. Incomplete GC recovery
  evidence pins while cleaned operations do not. Unknown stream-shaped files,
  ambiguous ownership, missing required representations, symlinks, hard links,
  unsafe modes, traversal, and changing ledger/files fail closed.
- Compression/read compatibility: eligible old original streams are replaced
  only by standard-library deterministic gzip with fixed headers plus
  `revolvr-compressed-artifact-v1` manifest recording original/compressed
  SHA-256 and size and original mtime. Compression is bounded, cancellable,
  atomic/synchronized, verifies compressed and decompressed readback, rejects
  corrupt/dual/divergent forms, and reconciles an interrupted exact dual
  publication. Receipt validation and worker JSONL metric recovery transparently
  consume the admitted logical representation without changing ledger paths.
- Export/replay: `revolvr-ledger-export-v1` manifests and canonical
  `revolvr-ledger-export-record-v1` JSONL preserve every run field, exact global
  event ID/type/time, byte-exact base64 payload, explicit legacy/versioned
  payload schema, range/high-water/source-ledger identities, counts/hashes,
  predecessor lineage, and verified compressed-representation facts. Export
  uses immutable SQLite reads, detects source changes and configured secrets,
  recovers identical orphan stream publication, and never prunes SQL rows.
  Verify checks canonical paths/types/links, source coverage, stream hashes,
  counts/range gaps/order/payload schemas/predecessors/secrets and compressed
  facts. Replay reconstructs exact logical histories, terminal status, ordered
  events, artifacts, verification/commit/finalization/archive/task-run/queue
  payload evidence without executing work or mutating the live ledger.
- GC transaction: dry-run requires one operation ID and frozen UTC time and
  performs no runtime creation or SQLite write. The plan binds policy/config,
  repository/ledger/high-water, ordered pins/actions, source and expected gzip
  identities, exact byte counts/bounds, export requirement, and content-derived
  plan identity. Apply requires that exact ID, recomputes authority before
  admission, takes the outer retention and autonomous-execution leases, probes
  active Git-admin/child/source writers, and revalidates ledger/high-water and
  current pins before every action. Stage order is admitted, verified/replayed
  export, compression, quarantine prune, completed journal, bounded cleanup,
  cleaned journal. Immutable synchronized history precedes journal replacement;
  cancellation records a resumable result and exact retries reconcile without
  duplicate export/compression/quarantine/deletion.
- App/CLI/docs: app exposes plan/apply/resume/inspect and export/verify/replay
  APIs. CLI adds default-dry-run `artifact gc`, explicit exact-plan apply,
  inspect/resume, and `ledger export`, `verify`, and `replay-validate`. Output is
  deterministic and narrow; no TUI retention controls or AW-27/AW-28 evidence
  screens were added. README documents safe defaults, policy, operations, and
  compatibility.
- Tests added: policy/default/hash/config parsing; deterministic empty and
  candidate plans; active/recent pins; unknown-file refusal; deterministic and
  cancelled gzip; compressed reads; exact apply/replay; interrupted dual
  publication recovery; cancelled journal resume; compression-before-prune;
  verified export before quarantine; compressed-reference export; immutable
  deterministic export; exact legacy payloads; tamper/secret refusal; logical
  terminal replay; doctor/config visibility; CLI dry-run/no-write/apply guard
  and export verify/replay snapshots.
- Verification passed: required AW-24 baseline; focused AW-25 packages; repeated
  `go test -count=1 ./...`; focused `go vet`; `git diff --check`; artifact/ledger
  CLI help; config check; and status. No live model, network, hook, source task,
  archive/reopen, notification, cache, metric, parallel worker, current-repo GC
  mutation, commit, push, reset, clean, restore, checkout, or stash was invoked.
- Remaining work: AW-26 exclusively owns Git-SHA dossier caching and
  role-specific projections. AW-27/AW-28 retain broad evidence screens/TUI
  controls; AW-29 notifications, AW-30 metrics/evaluations, and AW-31 bounded
  parallelism remain untouched.
- Blockers: none.

## AW-24 Completion (2026-07-12)

- Selected task: AW-24 — queue-until-exhausted and daemon-safe continuation.
- Queue ownership: new `internal/autonomousqueue` owns a versioned sequential
  queue operation above AW-23 scheduling and AW-22 exact-task execution. It
  persists exact mode/config/safety/bounds/times/sweep, fresh scheduling
  fingerprints, derived task-operation IDs, in-flight selection, ordered task
  outcomes, authority-bound exclusions, statistics, remaining work, and typed
  stop evidence under `.revolvr/autonomous/queues/<operation-id>/`.
- Durability/recovery: synchronized immutable history is written before every
  mutable checkpoint replacement with canonical JSON, contained non-symlink
  paths, exclusive history, atomic same-directory rename, directory sync, and
  strict readback. Newer valid history recovers a lagging checkpoint. Selection
  is durable before AW-22 starts; in-flight restart reopens only the same
  derived operation; exact terminal replay starts no work; changed operation
  material, malformed/divergent state, or conflicting task results fail closed.
- Fresh selection/fairness: every boundary reloads duplicate-checked active
  tasks/states, completed verified/reconciled archives, and complete child
  publication authority, then uses the scheduler's pure canonically ordered
  classification. Safe completed/blocked/input/task-cancelled/budget/
  no-progress/max-cycle stops are recorded and yield to unrelated work.
  Queue-local exclusions bind the exact task/state/status/lifecycle authority
  and vanish only after material change, preventing starvation without changing
  priority, dependencies, status, budgets, or task bytes.
- Stop classification: deterministic precedence distinguishes `drained`,
  `waiting_dependencies`, `waiting_input`, `waiting_blocked`, queue-owned
  `budget_exhausted`, `cancelled`, `safety_stop`, and
  `unsafe_or_ambiguous`. Unsafe results or uncertain runner errors stop before
  another task. Summaries retain every terminal task and its AW-22 operation.
- Dependency/child/archive behavior: completion unlocks dependency chains and
  diamonds on the next exact graph; newly published independent children can
  enter later selections; dependent children remain waiting; completed archive
  authority unlocks but is never selected. Incomplete child publication,
  malformed graph/archive/state, missing state, or disappeared pinned authority
  is queue-fatal. Queue execution never archives, reopens, answers input,
  retries/unblocks, restores/cleans, or mutates scheduling metadata.
- Locks: new `internal/autonomousexec` is the outer one-active autonomous
  execution lease shared by direct AW-22 app runs and queue sweeps. The queue's
  internal AW-22 path does not reacquire it, avoiding self-deadlock. Queue
  operation leases are outer to task execution and never acquired by
  Git-admin, state, child-publication, or source-writer owners. Daemon waiting
  holds none of those locks.
- Daemon: new `internal/autonomousdaemon` provides foreground injected polling
  and stable debounce only. Explicit fully-unattended AW-19 authority is
  required. Exact pre/post-sweep fingerprints preserve events arriving during
  a sweep; transient/equivalent samples start no duplicate sweep; cancellation
  interrupts poll/debounce; safety/ambiguity stops; and an explicit maximum
  bounds sweeps. Wake count/fingerprint are stored with the next durable queue
  sweep and ledger evidence. No dependency or platform-specific watcher was
  added.
- Ledger/redaction: deterministic queue ledger runs content-compare and
  deduplicate admitted, selection, task-stopped, daemon-wake, and stopped
  events. Terminal ledger completion follows readable terminal checkpoint
  persistence. Configured-secret redaction covers stop details, outcomes,
  checkpoints/history, ledger payloads, progress, app results, and CLI output;
  progress callback panics are isolated.
- App/CLI/TUI: app exposes one-shot queue and foreground daemon operations,
  reusing production AW-22 assembly without CLI recursion. CLI adds mutually
  exclusive `run --queue` and `run --daemon`, stable operation ID, positive
  task/cycle/sweep/poll/debounce bounds, and deterministic ordered summaries.
  Mixed-pass `--once`/`--max-passes`, exact `--until-terminal`, archive,
  status/show/receipt/config/doctor, and current TUI `U`/`R`/`L` behavior remain
  compatible. Broad TUI queue controls remain AW-28; concurrency remains AW-31.
- Tests added: independent multi-task order; blocked/input safe skip; waiting
  precedence; dependency diamond unlock; child appearance between selections;
  starvation prevention; queue budget; safety/unsafe/cancellation; exact
  derived operation recovery after selection-history crash; terminal replay and
  operation conflict; queue/driver lock cancellation; ledger deduplication;
  secret redaction; daemon attended refusal, stable debounce, transient events,
  event-during-sweep, cancellation, safety stop; scheduling fingerprint
  exclusion of queue/ledger self-effects; app composition; and CLI flags/
  summaries.
- Verification passed: required AW-23 baseline; focused AW-24 package suite;
  repeated `go test -count=1 ./...`; focused `go vet`; `git diff --check`;
  `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr config check`; and
  `go run ./cmd/revolvr status`.
- No live Codex/model session, network request, hook, dependency, commit, push,
  destructive Git command, archive mutation, workspace cleanup, notification,
  retention operation, or parallel task execution was invoked.
- Remaining work: AW-25 owns retention/compression/GC and ledger export/replay.
  AW-26 and later retain caching, broad evidence screens/TUI controls,
  notifications, metrics/evaluations, and parallel workers.
- Blockers: none.

## AW-23 Completion (2026-07-12)

- Selected task: AW-23 — dependency-aware scheduling and supervised child
  tasks.
- Canonical metadata: taskfile now strictly projects and parses source-ordered
  comma-separated `depends_on`, `tags`, and `conflicts`, plus immutable child
  parent/proposal/decision/run/evidence/behavior lineage. Status updates,
  finalization projections, archive/reopen byte preservation, ordinary add,
  and Markdown import retain the metadata; unknown frontmatter and original
  line endings/final-newline behavior remain byte-preserved.
- Scheduler: new pure `internal/autonomousscheduler` validates one exact
  duplicate-checked active snapshot plus caller-supplied verified/reconciled
  archive evidence. Missing/self/duplicate edges, cycles, duplicate IDs, and
  active/archive ambiguity fail closed with deterministic evidence. Only
  completed active or completed archived identities unlock dependents;
  archived identities are never selections. Readiness distinguishes waiting,
  blocked, needs-input, conflict, wrong-workflow, and nonpending states.
- Ordering/conflicts: ready tasks retain ascending priority semantics, then
  canonical source path and task ID. Conflicts are symmetric when either task
  names the other or both share a stable conflict key, and apply only against
  exact occupied identities in this sequential slice.
- AW-22 composition: omitted-task admission selects once through the scheduler
  above `autonomoustaskrun`; explicit new operations receive the same gate
  before workspace/model work. Existing durable operation identity bypasses
  ambient reselection on restart. The pinned loop still returns after its one
  task's terminal/max-cycle/budget/input/safety outcome and never consumes a
  second task.
- Child contract: block/needs-input decisions may carry at most four bounded
  structured child proposals whose exact material evidence is a subset of the
  accepted supervisor inputs. Stable keys, distinct normalized scopes,
  bounded title/scope/criteria/lists, dependencies/tags/conflicts, and typed
  parent behavior are required; broad roadmap/command/permission/network/
  environment/sandbox authority is rejected. Dependent children name the
  parent edge; independent children cannot name or bypass it.
- Child publication: new `internal/autonomouschild` derives deterministic
  collision-resistant IDs, canonical task/state paths, initial pending state,
  and `autonomous-child-lineage-v1`. One global child-publication lock protects
  a versioned journal with immutable history-before-journal stages. State is
  published before task visibility; incomplete journals make scheduling fail
  closed; exact retry rolls forward, changed operation material conflicts, and
  user-owned collisions are never overwritten. Parent task/state/history are
  read and hash-revalidated but never mutated.
- Evidence/accounting/safety: child publication starts no worker, verification,
  commit, finalization, archive, nested Codex, or AW-15 attempt. Three
  idempotently reconciled supervisor-run ledger events cover proposal admitted,
  children published, and completion. Configured secret values are refused
  before child task/state persistence.
- App/CLI/TUI: app projections expose dependency/tag/conflict/parent metadata,
  deterministic autonomous readiness, and next-autonomous identity. Task list
  adds narrow scheduling columns; status reports next autonomous readiness;
  TUI detail shows scheduling facts and `U` refuses a selected non-ready task.
  Mixed-pass selection and `--once`/`--max-passes` remain unchanged.
- Tests added: strict metadata round-trip/CRLF/no-final-newline/status
  preservation; import preservation; DAG/diamond-style readiness, completed
  archive, missing/duplicate/ambiguous/cycle diagnostics, priority ties,
  lifecycle reasons, and conflicts; explicit AW-22 dependency admission and
  pinned one-task behavior; child schema bounds/authority; deterministic
  creation, parent immutability, state lineage, ledger deduplication,
  crash/restart, changed-operation conflict, partial-publication fail-closed,
  and secret refusal; app and TUI admission coverage.
- Verification passed: required AW-22 baseline; required focused AW-23 package
  suite; `go test -count=1 ./...`; focused `go vet`; `git diff --check`; and
  non-model CLI help/config/status smoke commands.
- Remaining work: AW-24 owns queue-until-exhausted, daemon/watch continuation,
  fairness, and moving to a second ready task. AW-25 and later retain retention,
  caching, broad evidence screens, notifications, metrics, and concurrency.
- Blockers: none.

## AW-22 Completion (2026-07-12)

- Selected task: AW-22 — run one selected autonomous task until terminal.
- Ownership and identity: `internal/autonomoustaskrun.RunTaskUntilTerminal`
  owns only the exact-task loop, typed stop/result/statistics contract, durable
  operation checkpoint/history, replay, and operation lease. Explicit task ID
  or deterministic next-autonomous selection occurs once; the exact task,
  original task/state identities, workspace/checkpoint, effective config,
  max-cycle mode/value, evidence, counters, and stop are pinned. It never
  selects a replacement task after completion, block, input, cancellation,
  budget/max-cycle exhaustion, safety stop, or archive movement.
- Durability and locks: operation schema `autonomous-task-run-operation-v1`
  lives under `.revolvr/autonomous/task-runs/<operation-id>/`. Immutable
  synchronized history precedes mutable checkpoint replacement; recovery
  reads the newest valid history when it is ahead of the checkpoint. Exact
  replay is idempotent, changed task/config/limit conflicts, and an ambiguous
  in-flight external cycle stops without starting work. The separate operation
  lease is outermost and never acquired by Git-admin/state/source owners.
- Cycle/admission order: every cycle reconstructs current durable authority
  and calls bounded `autonomouscycle` once. Its nil-compatible `BeforeWorker`
  seam runs only after exact decision/policy/source admission and performs
  AW-15 CAS admission before any worker. Ordinary plan/implement/audit work is
  completed/accounted once, then persisted through existing owners before the
  next fresh supervisor cycle.
- Correction and optional roles: admitted `correct` results continue through
  `autonomouscorrection` for fast verification, distinct final verification,
  exact finding resolution, independent re-audit, and audit persistence before
  AW-15 completion. Admitted document/simplify results continue through the
  new `autonomousoptional.Continue` boundary for no-op/source-change
  accounting, final verification, fresh audit, occurrence persistence, and
  exact replay; it never starts or charges the already-run worker again.
- Terminal owners: AW-17 persists needs-input without an attempt; new
  `internal/autonomousblock` re-evaluates the exact block route, records
  immutable `autonomous-block-transition-v1` evidence, CASes the state to
  blocked, and publishes blocked task status; AW-20 alone performs completion
  finalization through ledger completion. Completed results are deliberately
  terminal-but-unarchived because AW-21 authority remains a separate explicit
  operation.
- Workspaces/safety: the app prepares or reopens the exact AW-18 workspace,
  uses it for every cycle, and advances only verified committed checkpoints.
  AW-19 preflight and redaction remain authoritative. Operation checkpoint,
  history, ledger, progress, result, CLI, and TUI summaries receive redacted
  detail; caller cancellation stops new work and already-admitted accounting
  uses non-cancelled persistence without rewriting task cancellation.
- Limits/statistics: default CLI/TUI max cycles is 50; API callers may select a
  positive limited value or unlimited mode. Equality stops before cycle N+1
  and never alters AW-15 budgets. Ordered durable statistics include cycle and
  supervisor counts, attempts, action counts, verification, audit, correction,
  optional-role, commit, and checkpoint totals.
- Ledger evidence: one deterministic operation ledger run records versioned
  admitted, cycle-started, cycle-completed, restarted, and stopped summaries.
  Exact retry deduplicates events and terminal run completion; loop completion
  is never claimed before AW-20 finalization.
- App/CLI/TUI: app exposes the typed operation; CLI adds
  `run --until-terminal [--task] [--operation-id] [--max-cycles]`, rejects
  incompatible mode flags and nonpositive limits, and renders deterministic
  stop/evidence statistics. TUI `U` pins the selected pending autonomous task,
  reuses preflight/one-active-run/cancel/progress/refresh behavior, and never
  starts the next row. Existing `run --once`, `run --max-passes`, archive
  commands, and mixed-pass routing remain unchanged.
- Tests: fake/temp-repository coverage includes completion, correction,
  needs-input, explicit block, every projected budget/no-progress/safety stop,
  cancellation before and during a cycle, max-cycle equality, exact replay,
  immutable-history crash recovery, ambiguous in-flight refusal, selected-task
  pinning, second-task byte/state isolation, loop-ledger deduplication,
  redaction, renderer panic isolation, block CAS/replay/conflict, optional
  pre-admitted continuation, CLI flag/stat rendering, TUI pin/cancel/refresh,
  and pre-worker admission ordering. Existing owner packages retain deeper
  finalization/archive/workspace/safety/correction/optional failure matrices.
- Verification passed: required AW-21 baseline; required focused AW-22 package
  suite including `internal/autonomoustaskrun` and `internal/autonomousblock`;
  repeated `go test -count=1 ./...`; focused `go vet` for every AW-22 changed
  package; `git diff --check`; `go run ./cmd/revolvr run --help`;
  `go run ./cmd/revolvr config check`; and read-only `go run ./cmd/revolvr
  status`.
- No live Codex/model session, network request, hook, dependency addition,
  commit, push, destructive Git command, workspace cleanup, archive mutation,
  queue, scheduler, daemon, notification, metric framework, or concurrency was
  used or added.
- Remaining work: AW-23 owns dependency-aware scheduling and supervised child
  tasks. AW-24 and later retain queues/daemon, retention, projections,
  notifications, metrics/evaluations, and concurrency.
- Blockers: none.

## Dogfood Timestamp Verification

- 2026-07-08T13:04:17Z live run `019f41d3-9120-7a77-92fd-d799f76ba000`: verifies receipt timestamp finalization after the prior fix by writing the receipt with the prompt-provided stale timestamp.

## Last Run

Task completed on 2026-07-12:

- Selected task: AW-21 — move terminal tasks into tracked archives with verify and reopen support.
- Files changed: added `internal/autonomousarchive` contracts, storage, transaction coordinator, scoped Git reconciliation, read-only verification, reopen coordinator, and focused real-Git/ledger tests; added app and CLI archive operations/tests; extended autonomous lifecycle/reopen lineage and taskfile terminal status/reopen publication contracts; added archive/reopen ledger event types; updated README and durable `.agent` state; removed the consumed AW-21 kickoff prompt after final verification.
- Ownership and entry points: `autonomousarchive.Archive`, `List`, `Show`, `Verify`, and `Reopen` are isolated bounded operations. They never invoke Codex, a supervisor/worker, verification command, audit, correction, optional roles, source restoration/cleanup, a scheduler, or a loop. Ordinary `taskfile` list/find/select remains active-only under `.agent/tasks/*.md`; archive discovery scans only the canonical tracked archive hierarchy.
- Dispositions and authority: schema `autonomous-task-archive-authority-v1` covers exactly completed, cancelled, superseded, and abandoned. Task/state contracts now represent those terminal outcomes; blocked and needs-input remain active and fail archive admission. Non-completed outcomes require matching terminal task/state disposition and exact reason plus trusted provenance and terminal time; their manifest explicitly omits completion authority.
- Completed admission: completed tasks require finalization stage `ledger_completed`, exact completed task and canonical terminal-state identities, AW-20 frozen evidence/capsule/manifest, final source/workspace/checkpoint, final-purpose verification, independent clean audit, safety policy, and exact terminal finalization ledger event/run evidence. `completion.md` is copied byte-for-byte from the verified AW-20 runtime capsule and is never rerendered.
- Identity/layout/manifest: archive IDs are deterministic SHA-256-derived values over task/operation/disposition/frozen UTC archive time. That UTC time alone selects `.agent/archive/YYYY/MM/<task-id>/`; tracked files are `task.md`, `archive.json`, and completed-only `completion.md`. Schema `autonomous-task-archive-manifest-v1` binds operation/archive/task/disposition/reason/provenance/terminal/archive times, original/archived task, state, applicable AW-20 and ledger identities, expected paths, and explicit omissions. The administrative commit SHA is recorded non-recursively in runtime history and ledger evidence.
- Path/collision behavior: exact replay is allowed only for identical operation/content. Different archive/task/operation/lineage content conflicts. Strict list/show rejects duplicate archive/task IDs, malformed date depth, foreign files/directories, traversal, absolute paths, symlinks, unsafe file modes, hard-link counts, wrong types, and different pre-existing target bytes. Active selection never sees archived files.
- Locks and transaction order: archive takes control-root `.revolvr/locks/git-admin.lock` before the task's canonical `state.lock`, rejects live control-root or workspace source writers, then validates task/state/finalization/artifacts/ledger/Git/index/worktree authority. Each stage writes synchronized immutable `autonomous-task-archive-transition-v1` history before atomic journal replacement. Order is admitted; archive files published; exact active task removed; administrative commit reconciled; archive ledger completed. After admission, persistence uses a non-cancelled context and retry rolls forward exact partial effects without reset, clean, restore, stash, broad deletion, or overwrite.
- Git administration: archive commits contain only the exact tracked archive additions and optional active-task deletion and carry `Archive-Operation`, `Archive-ID`, `Task-ID`, `Disposition`, and `Terminal-Identity` lines. Reopen commits contain only the new active task. Pre/post HEAD comparison, one HEAD retry, unborn repositories, advanced-HEAD command failure, indeterminate outcomes, exact commit-tree paths/bytes, and unrelated dirt/staging refusal are enforced.
- Ledger/runtime evidence: archive creation uses one exact run and idempotent versioned prepared/files-published/active-removed/commit-reconciled/completed events. Runtime journals/history remain directly addressable after active-task removal and no longer depend on `autonomousstate.canonicalTask`. Material event/run reuse conflicts; successful retry creates no duplicate history, events, commits, or archive entry.
- Read-only verification: ordered named checks cross archive schema/path/date/task bytes and terminal metadata; canonical terminal state; AW-20 frozen/capsule/manifest/final source/workspace/checkpoint/verification/audit/safety/ledger evidence; archive journal/history/ledger; administrative commit identity/tree/bytes; active-task exclusion/reopen lineage; and configured-secret absence. It opens SQLite immutable/read-only and does not repair, normalize, stage, commit, create sidecars, update ledger, or change file mtimes.
- Reopen semantics: reopen first verifies the archive immediately before admission and always creates a new task ID. Schema `autonomous-reopen-lineage-v1` in the new pending state binds archive/task/disposition/archive commit, operator/harness authority, reason, operation, and time. Only task id/status/state reference change; exact spec, unknown metadata, CRLF/LF, and no-final-newline behavior are preserved. State/task publication and the path-scoped reopen commit recover idempotently; a second/conflicting reopen fails; the immutable archive remains canonical and no run starts.
- Redaction: configured secret values are rejected before tracked task/capsule/manifest publication and checked again by read-only verification. AW-20 remains responsible for redacting completion artifacts; AW-21 never rewrites exact task or capsule bytes to conceal a leak.
- Tests added: deterministic UTC paths/archive IDs and strict list/show; completed AW-20 byte-copy and evidence verification; cancelled/superseded/abandoned archives; blocked rejection; exact active selection exclusion; crash after active removal and roll-forward with single ledger effects; operation conflicts; secret and symlink refusal; tamper detection; read-only HEAD/status/ledger/mtime preservation; reopen/new pending lifecycle/lineage/selection/replay/second-reopen refusal; partial reopen state/task recovery; app list/show; CLI list/show/help/explicit-UTC validation.
- Verification run: preserved required AW-20 baseline; `gofmt -w` on every AW-21 Go file; required focused package suite including `internal/autonomousarchive`; `go test -count=1 ./...`; `go vet` across every changed Go package; `git diff --check`; `go run ./cmd/revolvr archive --help`; `go run ./cmd/revolvr archive list`; archive verify/reopen help smoke commands.
- Verification result: all focused real-Git/filesystem/ledger/app/CLI tests, the complete repository suite, changed-package vet, diff validation, and non-model CLI smoke commands passed. No live Codex/model, hook, network request, dependency, push, force, destructive Git command, source-worktree cleanup, scheduler, terminal loop, or TUI archive operation was used.
- Remaining work: AW-22 owns the run-one-task-until-terminal loop. AW-21 added no dependency scheduling, queue/daemon, retention/GC/export, dossier caching, broad autonomous CLI/TUI evidence screens, notifications, metrics/evaluations, or concurrency.
- Blockers: none.

Task completed on 2026-07-12:

- Selected task: AW-20 — add transactional terminal finalization and completion capsules.
- Files changed: added `internal/autonomousfinalization` contracts, deterministic capsule/manifest rendering, immutable artifact handling, and bounded coordinator; added finalization detail/stage validation under `internal/autonomous`; added `autonomous-finalization-transition-v1` history/CAS persistence under `internal/autonomousstate` and extended committed-audit traversal; added finalization ledger event types and a pure task metadata projection helper; added focused domain, golden, gate, ordering, redaction, crash/retry, stale-evidence, ledger-deduplication, and integration tests; updated `README.md` and durable `.agent` state; removed the consumed AW-20 kickoff prompt after verification.
- Ownership and entry point: `autonomousfinalization.Finalize` consumes one exact frozen `complete` authorization and never starts Codex, a worker, verification, audit, correction, optional roles, a source commit, restoration, archive movement, or a loop. It recomputes `autonomouspolicy.Evaluate` from the exact complete decision/reference/state/source/final verification/audit and requires a caller-owned bounded live evidence revalidator before admission and again before task terminalization.
- Frozen evidence: schema `autonomous-completion-frozen-evidence-v1` binds the original and projected-completed task bytes, canonical state hash/size, decision/reference/route, current source, exact ready/restored checkpointed workspace, safety policy/preflight/config, completed plan, acceptance dispositions, final-purpose tiered verification, independent clean audit, finding and optional-role histories, completed attempt/budget evidence, ordered commits/runs/provenance, and harness admission/terminal times. Exact state/task hashes are recomputed; stale identity, fast/flaky/missing verification, findings-bearing/stale audit, pending acceptance, open attempts, workspace/checkpoint/source/policy drift, and malformed commit/run evidence fail closed.
- Lifecycle and persistence: `autonomous-finalization-state-v1` advances monotonically through `admitted`, `capsule_materialized`, `task_completed`, `state_completed`, and `ledger_completed`. `ExecutionState.Validate` requires detail for `finalizing`, permits legacy completed snapshots without it, requires AW-20 completed snapshots to have reached state completion, and prevents immutable authority/artifact/task/time rewrites or regression. Each canonical replacement has an immutable finalization history record written first under the shared task `state.lock`, exact state CAS, synchronized atomic rename, strict readback, replay/conflict/stale handling, and committed-audit graph continuity.
- Artifacts and capsule: immutable `completion-evidence.json`, deterministic human-readable `completion.md`, and `completion-manifest.json` live under `.revolvr/autonomous/tasks/<task-id>/completion/`. Canonical contained paths, non-symlink parents, same-directory temporary files, bounded modes, file/directory sync, atomic rename, exact hash/size readback, collision refusal, deterministic source ordering, explicit waiver/not-applicable/finding/optional/commit/run facts, omission records, and nonrecursive manifest hashing are enforced. AW-19 configured-secret redaction is applied to all three persistent artifacts.
- Task/state/ledger choice and order: AW-20 changes only the canonical autonomous task status from pending to completed and freezes/validates the exact projected bytes; archive movement remains AW-21. Ordering is frozen artifact; admitted state; exact finalization run/prepared evidence; capsule/manifest; materialized state; task status; task-completed state; completed lifecycle/terminal detail; exact terminal event/run completion; ledger-completed state. No metadata-only source commit is made. Durable reconciliation uses a non-cancelled context, identical operation/stage/artifact/run/event effects replay, and conflicting reuse fails closed.
- Tests added: finalization-detail lifecycle/monotonicity/immutability; byte-for-byte capsule golden and deterministic rerender; missing/pending/stale/fast/unsafe gates; explicit waiver/not-applicable and reconciled ordered commit rendering plus duplicate refusal; exact state/task/workspace/safety/config authority; crash after capsule rename with resumable admitted state; stale live evidence refusal after freeze; retry through task/state/ledger completion; exact terminal replay with one event of each type; and configured-secret removal from frozen evidence, capsule, and manifest.
- Verification run: preserved AW-19 focused baseline; `gofmt -w` on all AW-20 Go files; required focused package suite including `internal/autonomousfinalization`; `go test -count=1 ./...`; `go vet ./internal/autonomous ./internal/autonomousstate ./internal/autonomousfinalization ./internal/autonomouspolicy ./internal/taskfile ./internal/ledger`; `git diff --check`.
- Verification result: all focused tests, the complete repository suite, focused vet checks, and diff validation passed. No live Codex/model, verification command outside tests, hook, network request, dependency, source mutation/commit, destructive Git operation, archive, autonomous CLI/TUI run, scheduler, or loop was used.
- Remaining work: AW-21 owns tracked terminal archives, archive verification, and reopen support. AW-20 added no archive paths/moves, run-until-terminal loop, scheduler/queue, retention, CLI/TUI autonomous evidence surface, notification, metrics, daemon, or concurrency behavior.
- Blockers: none.

Task completed on 2026-07-12:

- Selected task: AW-19 — define the unattended-execution safety boundary and preflight.
- Files changed: added `internal/autonomoussafety/contracts.go`, `preflight.go`, and focused tests; added dependency-free `internal/redact`; extended app/runonce strict config parsing and `revolvr-effective-run-config-v2` fingerprinting; extended deterministic config/doctor projections and tests; integrated safety authority, protected-path admission, provenance, environment projection, and redaction into `autonomouscycle`, `supervisor`, `codexexec`, `runner`, and AW-15 outcome classification; updated `README.md`, `scripts/dogfood-live.sh`, and durable `.agent` state; removed `.agent/AW_19_KICKOFF_PROMPT.md` after final verification.
- Modes and acknowledgement: `operator_attended` is the explicit compatibility default for existing mixed-pass/local dogfood and keeps dangerous bypass/network/environment/hook responsibility visible. `fully_unattended` is never inferred and requires a ready/restored exact AW-18 task workspace, externally attested container/OS isolation, explicit externally attested network posture, non-ambient environment handling, trusted/absent hooks, resolved command provenance, redaction readiness, and exact `revolvr-fully-unattended-v1:<policy-sha256>` acknowledgement. Material policy changes invalidate stale acknowledgement.
- Roots and protected authority: preflight consumes exact canonical control/execution/common-dir/owner/branch/checkpoint/current-source workspace evidence and derives harness-only run/receipt/lock/state-history roots plus the model source root. It rejects missing, symlinked, duplicate/overlapping, moved, or noncanonical roots. Protected classes cover Git common/worktree administration, ownership markers, locks, state/history, ledger, task specs, profiles, repository guidance, and safety config. Model changes to protected paths stop before verification/commit; task/model output is not a source of commands, environment, roots, network, hooks, or bypass permissions.
- External isolation, network, and hooks: worktree evidence explicitly says Git/source isolation is not a security sandbox. Full autonomy requires stable operator-authored external-isolation/network attestations and rejects unknown network posture or ambient environment. Hook preflight resolves effective `core.hooksPath`, bounds/truncation-checks Git evidence, inventories executable regular hooks without running or modifying them, rejects symlinks/outside paths for full autonomy, and requires exact trusted path/content identities or an actually empty/disabled hook set.
- Command/environment provenance: Codex, Git, and every flat/tiered verification command are resolved to executable content identities with exact argv, canonical directory, ordered configured environment, timeout, and output caps. Fully unattended execution can replace the ambient environment with an explicit allowed-name projection; operator-attended compatibility retains current inheritance. Commands remain bounded non-shell `runner.Command` values from harness/config authority.
- Redaction: configured secret sources are environment-variable names only; values never enter effective config, hashes, config/doctor output, or policy provenance. Deterministic longest-first replacement covers overlapping/repeated values and is applied to Codex JSONL/stderr/final-output artifacts, returned stdout/stderr/errors, progress, and ledger summaries; redaction source/match facts and raw/redacted sizes remain visible. The autonomous command-runner projection redacts verification/Git/commit evidence before downstream persistence.
- Runtime ordering and provenance: `autonomouscycle.Run` performs the task/workspace/source/config-bound safety preflight before dossier assembly, supervisor, or worker Codex. Failure returns typed `safety_preflight_failed` and starts no supervisor/worker/verification/commit. If an AW-15 caller already durably admitted the enclosing operation, the stop is charged once as safety evidence. Supervisor and worker invocation/provenance retain the same safety-policy SHA-256 and full redacted policy/preflight projection; protected path checks run before verification/commit.
- Tests added/updated: strict mode/policy/config unknown-value handling; effective-hash sensitivity; missing/stale/valid acknowledgement; external/network/environment combinations; workspace/source identity; symlinked roots; protected task/profile/guidance/Git paths; default/disabled hooks and hook drift; executable provenance and caps; environment replacement; configured-secret overlap, error/output/artifact/ledger redaction; deterministic config/doctor visibility; safety failure before any Codex; and identical supervisor/worker policy identity. Existing AW-17 input, AW-18 workspace/checkpoint, AW-01–AW-16 autonomous, and mixed-pass behavior remain covered by the complete suite.
- Verification run: AW-18 focused baseline; `gofmt -w <all changed Go files>`; required focused package suite including `internal/autonomoussafety` and `internal/redact`; repeated `go test -count=1 ./...`; focused `go vet` for all changed packages; `git diff --check`; `go run ./cmd/revolvr config check`; expected-nonzero `go run ./cmd/revolvr doctor` on the intentionally dirty accumulated worktree, which rendered the new safety check correctly and failed only worktree cleanliness.
- Verification result: all focused tests, the complete repository suite, focused vet checks, config smoke, and diff validation passed. Doctor produced deterministic safety visibility and the expected dirty-worktree nonzero result. No live model/Codex, hook, verification command during preflight, container, network request, dependency addition, destructive Git operation, commit, or autonomous CLI/TUI run was used.
- Remaining work: AW-20 owns transactional terminal finalization/completion capsules. AW-19 added no finalization/archive, repeated supervision loop, scheduler/queue, autonomous CLI/TUI controls, retention, notification, metrics, daemon, or concurrency behavior.
- Blockers: none.

Task completed on 2026-07-12:

- Selected task: AW-18 — isolate autonomous tasks in dedicated Git worktrees with checkpoint recovery.
- Files changed: added typed workspace/checkpoint/recovery contracts in `internal/autonomous/workspace.go`; extended execution-state validation and deterministic dossier rendering; added `internal/autonomousworkspace` Git administration, reconciliation, restoration, cleanup, and state-application boundaries; added immutable `autonomous-workspace-transition-v1` history and canonical-state compare-and-swap persistence under `internal/autonomousstate`; extended committed-audit reconstruction; separated control and execution roots through dossier assembly, supervisor, one-cycle worker, correction, optional-role, Codex artifact, and tiered-verification paths; added control-root workspace-scoped source locks; added focused contract, persistence, runtime, lock, and real-Git integration tests; updated durable `.agent` state and removed the consumed AW-18 kickoff prompt.
- Workspace authority: `autonomous.TaskWorkspace` records exact task/workspace IDs, canonical absolute control/execution/common-dir/marker paths, deterministic harness branch ref, baseline/current HEAD/tree/source identities, known-good checkpoint sequence and provenance, retained failed/ambiguous refs, recovery evidence, status, and harness times. Immutable ownership fields cannot change across execution-state transitions; checkpoint/retained evidence cannot regress or disappear.
- Deterministic ownership: workspace IDs hash canonical control root, Git common directory, and task ID. Branches are `refs/heads/revolvr/tasks/<validated-task>-<collision-resistant-id>` and worktrees live only at `.revolvr/autonomous/worktrees/<workspace-id>`. A canonical control-root ownership marker is written and synchronized before ref/worktree creation. Names, paths, branch prefixes, directories, `.git` files, or registry entries alone never grant ownership.
- Creation/reopen recovery: preparation captures an exact commit baseline and creates a linked worktree from Git objects, never from the primary index/filesystem. Primary modified, staged, deleted, untracked, and concurrently edited bytes remain outside the task checkout. Exact marker/ref/registry/`.git` link/common-dir/branch/HEAD/tree/source evidence is revalidated. Marker-before-ref and ref-before-registration interruption windows recover idempotently; foreign branches, registered worktrees, unregistered paths, symlinked namespaces, and marker mismatches conflict without adoption or deletion.
- Checkpoints and ambiguity: the baseline is checkpoint sequence one. `ReconcileCommit` accepts only an exact caller-observed distinct post-commit HEAD, preserving the established pre/post-HEAD rule; unknown advances receive immutable `refs/revolvr/retained/<workspace>/ambiguous-*` refs. A clean reconciled source can advance the known-good checkpoint with exact commit/tree/source/provenance evidence. Command, exit, bounded output/truncation, directory, and harness timestamps remain observable.
- Failed restoration and cleanup: restoration first revalidates exact ownership and refuses an active source writer or ignored files that cannot be safely retained. It captures staged/tracked/untracked failed bytes in a synthetic immutable commit/ref, then and only then runs narrow `reset --hard <checkpoint>` plus `clean -fd` inside the owned execution worktree and verifies exact post-restore HEAD/tree/source cleanliness. Cleanup revalidates twice under the Git-admin lock and refuses dirty, ignored, locked, busy, advanced, ambiguous, mismatched, or retained-evidence workspaces; it removes only the exact registration and exact expected harness ref. User-owned refs/worktrees/files are never deleted.
- Persistence and locks: `autonomousworkspace.Apply` records already-observed Git transitions through the shared per-task `state.lock`. Workspace history uses exact state SHA-256/byte-size CAS, immutable history-before-state replacement, same-directory temporary state files, flush/rename/directory sync/readback, replay/conflict/stale-write classification, and after-rename recovery. Committed audit-history traversal now includes workspace edges. Git administration uses a control-root global `git-admin.lock`; source writers use control-root `.revolvr/locks/workspaces/<workspace-id>/source-writer.lock` metadata naming the exact execution root. The order is Git-admin lock before workspace inspection/mutation; source execution never takes the Git-admin lock.
- Runtime root split: canonical task/spec, profiles/guidance, state/history, ledger, receipts, schemas, and artifacts stay control-rooted. Dossier Git evidence, supervisor source snapshots, Codex working directory, changed-file capture, verification, commits, correction final verification, and optional-role source work use the admitted execution root. Codex and tiered verification have explicit control-root artifact roots. `autonomouscycle.Run`/`RunInWorkspace` fail closed without a validated workspace matching durable state; needs-input remains no-worker/no-mutation and mixed-pass behavior is unchanged.
- Tests added: deterministic workspace JSON/dossier projection; contract/transition validation; state CAS/replay/conflict/stale/after-rename recovery; committed-audit traversal; real-Git exact-baseline create with modified/staged/deleted/untracked/concurrent primary edits; two-task isolation; owned creation-window recovery; branch/registered-worktree/symlink conflicts; restart reopen; successful commit reconciliation/checkpoint advancement; ambiguous advanced-HEAD retention; failed dirty/untracked state retention and exact restore; dirty/ignored/locked/active-writer/evidence-losing cleanup refusal; exact cleanup; workspace-scoped source-lock placement; and one-cycle control/execution/artifact routing.
- Verification run: `gofmt -w <all changed Go files>`; AW-17 baseline focused tests; `go test -count=1 ./internal/autonomous ./internal/autonomousstate ./internal/gitstate ./internal/lock ./internal/commit ./internal/autonomouscycle ./internal/autonomousattempt ./internal/autonomouscorrection ./internal/autonomousoptional ./internal/autonomousinput ./internal/autonomouspolicy ./internal/autonomousverification ./internal/autonomousworkspace`; `go test -count=1 ./...`; `go vet ./internal/autonomous ./internal/autonomousstate ./internal/autonomousworkspace ./internal/autonomousassembly ./internal/autonomouscycle ./internal/autonomousattempt ./internal/autonomouscorrection ./internal/autonomousoptional ./internal/autonomousinput ./internal/autonomouspolicy ./internal/autonomousverification ./internal/codexexec ./internal/supervisor ./internal/lock ./internal/commit ./internal/gitstate`; `git diff --check`.
- Verification result: all focused real-Git/unit/runtime tests, the complete repository suite, focused vet checks, and diff validation passed. No live Codex/model, nested session, autonomous CLI/TUI/runonce execution, dependency addition, or Git commit was used. Destructive Git commands ran only inside temporary test repositories and only after exact harness-worktree ownership validation.
- Remaining work: AW-19 owns unattended permission/sandbox/preflight, protected paths, secret redaction, network policy, and dangerous-bypass acknowledgement. AW-18 adds no security sandbox, finalization/archive, repeated supervisor loop, scheduler/queue, retention, notifications, metrics, concurrency, or operator-facing workspace command.
- Blockers: none.

Task completed on 2026-07-12:

- Selected task: AW-17 — add structured `needs_input` handling.
- Files changed: extended `internal/autonomous` supervisor/state/dossier contracts and tests; added isolated `internal/autonomousinput`; added `internal/autonomousstate/input_history.go`, `input_store.go`, and focused tests; extended pure `internal/autonomouspolicy`, supervisor schema/parser/prompt/execution tests, one-cycle terminal routing, attempt decision signatures, committed audit-history traversal, canonical supervisor profile/template/tests, and durable `.agent` state/handoff files; removed `.agent/AW_17_KICKOFF_PROMPT.md` after completion.
- Supervisor contract: added terminal-for-now `needs_input` with no worker, strategy, correction authority, or success criteria. Its task-bound question has a stable lower-case ID, positive revision, exact question and blocking reason, at least two distinct stable option IDs/meanings, one recommendation referencing an offered option plus rationale, typed evidence, and a deterministic SHA-256 over all control-relevant content. The parser deterministically assigns a missing content hash and rejects a supplied mismatch. Independent work is optional and limited to exact read-only plan/planner or audit/auditor declarations that name every offered option in canonical order; it is a projection, never a route.
- Durable lifecycle: canonical execution state now retains immutable ordered question, answer, and resume records plus an exact current-question identity. Answers bind task, question ID/revision/content SHA-256, one offered option ID, stable answer ID, operator provenance/evidence, and harness-supplied timestamp. Resume binds the exact durable answer and restores only the recorded pending/ready lifecycle. Superseding questions retain the prior identity and use consecutive revisions for the same question ID. Unknown, wrong-task, stale, changed-content/option, superseded, contradictory, double-answer, and double-resume operations fail closed. Exact operation replay is idempotent; material reuse conflicts. Legacy `{\"reason\": ...}` snapshots remain readable/renderable but cannot authorize answer or resume.
- Persistence and recovery: schema `autonomous-input-transition-v1` stores one immutable question/answer/resume transition per operation under `.revolvr/autonomous/tasks/<task-id>/history/input/`. It validates canonical task/path ownership, shares `state.lock`, uses exact SHA-256/byte-size CAS twice, writes and synchronizes immutable history before same-directory temporary state replacement, atomically renames, synchronizes directories, and strictly reads back. Failures before rename leave only a non-authoritative orphan reusable by exact retry; failures after rename reopen as committed replay. Committed audit reconstruction now traverses planning, audit, attempt, optional-role, question, answer, and resume transitions, so input evidence never hides the latest committed audit.
- Safety and routing: pure policy admits a valid `needs_input` decision only from pending/ready with safe source evidence and returns a distinct no-worker route. A suspended task rejects all ordinary routing until an exact answer and explicit resume. `EvaluateNeedsInputYield` returns typed clean/unsafe results for task/state/question/provenance/source/in-flight gates; dirty, unknown, mismatched, stale, malformed, legacy, or source-mutating-in-flight state cannot yield. `EvaluateIndependentWork` exposes only a previously validated read-only declaration and starts nothing. The one-cycle coordinator returns `needs_input_authorized` without worker, verification, audit, commit, task/state persistence, or recommendation selection. Answer/resume persistence does not create attempts or change AW-15 counters/budgets.
- Dossier and compatibility: deterministic dossiers render current and prior unanswered, answered, resumed, or superseded questions; exact options and recommendation; independent work; answer provenance; and legacy non-answerable state. Existing AW-01 through AW-16 decision JSON and zero-input execution-state bytes remain compatible through an omitted empty input projection. Mixed-pass behavior, `passpolicy`, `runonce`, task Markdown, verification, audit/finding ownership, optional roles, CLI, and TUI remain unchanged.
- Tests added/updated: deterministic question/decision JSON, deterministic supervisor schema, parser-assigned identity, every malformed field composition, invalid/duplicate/ambiguous options and recommendations, exact/dependent/unsafe independent work, all-action compatibility, no-worker cycle behavior, unanswered route refusal, clean/dirty/unknown/in-flight/stale/malformed/provenance yield gates, answer persistence/provenance, unknown/wrong-task/stale/changed/superseded/contradictory answers, question/answer/resume replay and conflicts, explicit/double resume, append-only rewrite/disappearance rejection, unrelated evidence/budget preservation, legacy fail-closed behavior, restart, failures before/after state rename, and audit reconstruction through all three input transitions.
- Verification run: `gofmt -w <all changed Go files>`; `go test -count=1 ./internal/autonomous ./internal/autonomouspolicy ./internal/autonomousstate ./internal/autonomousinput ./internal/supervisor ./internal/autonomouscycle ./internal/autonomousattempt ./internal/prompt`; `go test -count=1 ./...`; `go vet ./internal/autonomous ./internal/autonomousinput ./internal/autonomouspolicy ./internal/autonomousstate ./internal/supervisor ./internal/autonomouscycle ./internal/autonomousattempt ./internal/prompt`; `git diff --check`.
- Verification result: all focused tests, the complete repository suite, focused vet checks, and diff validation passed. No live model/Codex, nested session, CLI/TUI autonomous execution, destructive Git command, dependency addition, or commit was used.
- Remaining work: AW-18 is the next unchecked task and owns per-task Git worktree isolation and checkpoint recovery. AW-17 added no worktree, safety-preflight, finalization/archive, repeated loop, scheduler/queue, notification, CLI, or TUI surface.
- Blockers: none.

Task completed on 2026-07-11:

- Selected task: AW-16 — make documentation and simplification conditional.
- Files changed: added `internal/autonomous/optional_role.go` and focused tests; added the isolated `internal/autonomousoptional` application/coordinator package and tests; added `internal/autonomousstate/optional_role_history.go` and `optional_role_store.go`; extended autonomous state validation, dossier rendering, audit-history traversal, attempt-history loading, policy completion tests, ledger event kinds, optional-role worker prompt authority, the supervisor/documentor/simplifier profiles and profile templates; updated `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`; removed `.agent/AW_16_KICKOFF_PROMPT.md` after verification and handoff.
- Public contracts and ownership: versioned map-free `OptionalRoleAssessment`, `OptionalRoleEvidence`, `OptionalRoleOccurrence`, gate, and worker-evidence contracts distinguish `run`, `not_applicable`, `no_change`, and `source_changed`. The isolated coordinator owns exactly one optional-role assessment and operation; AW-09 remains pure, AW-10 still runs one supervisor decision and at most one worker, AW-12 remains the only audit/finding persistence authority, and AW-15 remains the only attempt/budget authority.
- Relevance and not-applicable authority: structured evidence is bound to exact task, canonical state hash, source revision, current final verification occurrence, and current audit. Documentation can run only for exact task/spec, acceptance, plan, audit, or user-facing documentation evidence; simplification can run only for exact complexity, duplication, or maintainability evidence. Every runnable target has a clean repository-relative path, and source changes outside selected target files/directories are rejected. `not_applicable` requires exact `no_relevant_work` evidence plus rationale and fails closed if any role obligation/target is present. Supervisor rationale, IDs, timestamps, or formatting do not create relevance; relevance identity canonicalizes evidence and excludes incidental supervisor metadata.
- Skip semantics: `not_applicable` is a separately persisted decision-only disposition and does not consume an AW-15 attempt. Documentation and simplification remain independent with no fixed order. Both explicit skips are compatible with the existing completion policy, which still requires neither role by default.
- No-op evidence and Git behavior: a worker no-op is one admitted and completed AW-15 attempt with the existing `no_progress`/changed-strategy behavior. The occurrence retains exact decision, assessment, attempt, worker, profile, dossier, receipt, ledger, source, and prior final verification/audit gate identities. It runs no verification or audit and creates no commit or task-frontmatter transition.
- Source-changing behavior: an admitted role uses exactly one ordinary AW-10 cycle. AW-10 remains responsible for source snapshots, dirty-state refusal, role-scoped changes, final-purpose verification, exact commit admission, and no-change classification. After a committed change, the coordinator runs at most one newer independent audit on the exact verified/committed source and persists it only through `autonomousauditapply.ApplyAuditResult`. Missing/failed/flaky/timed-out verification, commit refusal/failure/indeterminate status, stale/failed/malformed/non-independent audit, cancellation, unsafe source, or stale state stops later stages and records no successful optional-role occurrence. Simplifier source changes additionally require retained behavior-preservation evidence. Findings-bearing audits return to supervision and are never silently resolved by this coordinator.
- AW-15 integration: every executed document/simplify operation is admitted before its bounded runner and completed exactly once after its worker/audit stages. Nested audit work is part of the one charged operation, not another action attempt. Task/action/time/token/repetition limits, cancellation persistence, no-progress strategy requirements, exact CAS, and task isolation remain enforced. Skip accounting is explicitly separate and counter-free.
- Persistence, restart, and replay: canonical state retains append-only per-task optional-role occurrences. Immutable schema `autonomous-optional-role-transition-v1` uses the existing per-task `state.lock`, exact previous/result state identities, history-before-state writes, atomic replacement, strict readback, content-addressed operation identity, stale-writer rejection, and exact replay. Coordinator replay returns the existing occurrence without rerunning a worker or duplicating its ledger event. Audit reconstruction now traverses committed planning, audit, attempt, and optional-role transitions, allowing later evidence-only state changes without hiding the latest audit; source/verification freshness is still checked by policy. Older source-bound occurrences remain visible and become stale naturally after source drift.
- Dossier/profile/ledger behavior: dossiers render every optional-role occurrence and its current gate/attempt/worker/commit identities. A new `optional_role_disposition` ledger event records skip/no-op/source-change evidence. Supervisor instructions state that roles are independent and evidence-conditional; documentor/simplifier profiles and worker prompts restrict changes to exact selected targets and require honest no-op reporting.
- Tests added: independent role skips and either-order completion; required documentation cannot be waived; rationale-only/generic cleanup rejection; stable relevance identity; skip/no-op/source-change contracts; changed-path authority; documentor and simplifier no-op accounting/evidence; source-changing final verification/commit/fresh-audit composition; behavior-preservation evidence; verification/commit stops; failed/stale/non-independent audits; cancellation and AW-15 task/action budget stops; persistence CAS/replay/conflict/source drift/task isolation; audit reopen through attempt/optional transitions; coordinator replay; dossier rendering; and existing completion/mixed-pass compatibility through the full suite.
- Verification passed: `gofmt -w` on every changed Go file; `go test -count=1 ./internal/autonomous ./internal/autonomousoptional ./internal/autonomouspolicy ./internal/autonomouscycle ./internal/autonomousattempt ./internal/autonomouscorrection ./internal/autonomousverification ./internal/autonomousaudit ./internal/autonomousauditapply ./internal/autonomousstate ./internal/supervisor ./internal/ledger ./internal/prompt`; `go vet ./internal/autonomous ./internal/autonomousoptional ./internal/autonomouspolicy ./internal/autonomouscycle ./internal/autonomousattempt ./internal/autonomousauditapply ./internal/autonomousstate ./internal/supervisor ./internal/ledger ./internal/prompt`; `go test -count=1 ./...`; `git diff --check`.
- Verification result: all focused tests, the complete repository suite, focused vet checks, and diff validation passed. No live Codex/model, live supervisor/worker, `revolvr run`, nested execution, destructive Git operation, dependency addition, or commit was used.
- Remaining work: AW-17 owns structured questions, answers, and resume behavior. AW-18 still owns worktrees and recovery. AW-16 adds no autonomous CLI/TUI/runonce path, scheduler, retry loop, automatic correction, or fixed document/simplify itinerary.
- Blockers: none.

Task completed on 2026-07-11:

- Selected task: AW-15 — add retry budgets, no-progress detection, and circuit breakers.
- Files changed: added `internal/autonomousattempt/contracts.go`, `control.go`, `execution.go`, and `control_test.go`; added `internal/autonomousstate/attempt_history.go` and `attempt_store.go`; extended `internal/autonomous/contracts.go`, `state.go`, `dossier.go`, and added `attempt_state_test.go`; extended `internal/autonomousstate/store.go`; extended `internal/supervisor/prompt.go`, `schema.go`, and `prompt_schema_test.go`; updated the canonical supervisor profile and template in `.agent/profiles/supervisor.md` and `internal/prompt/profile.go`; updated `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`; removed the consumed `.agent/AW_15_KICKOFF_PROMPT.md` after completion.
- Ownership boundary: isolated `internal/autonomousattempt` admits, invokes, and completes exactly one caller-selected operation above the existing one-shot AW-10 cycle and AW-14 correction coordinator. `Execute` has no retry or recursive supervision loop; `CycleOperation`/`CorrectionOperation` are one-call adapters. Admission compare-and-swap persists before the injected runner starts, and every rejected admission starts no runner.
- Budget authority and boundary semantics: caller-owned `Limits` bind only on a clean zero-attempt state or must exactly match already persisted modes and limits. `unset`, `limited`, and `unlimited` remain distinct. A limited budget admits only while `consumed < limit`; equality is exhausted for total attempts, exact per-action attempts, elapsed time, and tokens. Admission increments total/retry and exact action counters once; successful completion never erases cumulative consumption. Per-action exhaustion persists a typed nonterminal action stop, so another action remains admissible.
- Trusted consumption: elapsed time comes only from the injected harness clock around one runner call. Tokens come from exact Codex execution usage adapters; every started model invocation must have unambiguous metrics. Missing or negative metrics with a limited token budget append the completion evidence and fail closed as `missing_trusted_metrics` without inventing consumption. Cancellation uses a non-cancelled persistence context after admission so the attempt, trusted duration/tokens, and cancellation breaker survive.
- Durable attempt evidence: `ExecutionState.Attempts` now carries monotonic counters, action budgets/stops, a global transition sequence, append-only admission/completion events, repeated-signature policy, changed-strategy requirement, and retained last failure. Events bind task/action/attempt/decision/run/occurrence identity, before/known-after source revisions, outcome, trusted duration/token facts, evidence, and canonical signatures. Typed circuit-breaker detail retains reason, trigger attempts/signature, exact budget snapshot, and evidence; terminal blocks keep the same evidence in `TerminalDetail`.
- Persistence and replay: schema `autonomous-attempt-transition-v1` records every admitted/completed/breaker transition under `.revolvr/autonomous/tasks/<task-id>/history/attempts/` using the existing per-task `state.lock`, exact previous/result state identities, immutable operation/application identity, synchronized history-before-state writes, atomic state replacement, and strict readback. Exact replay returns without charging again; stale writers fail; an identical orphan history record left before state replacement is reused on retry; a post-rename replay recognizes the immutable event in a newer current state. State transition validation prevents counter/consumption/sequence decreases, limit changes after admission, event/action-stop rewrites, and failure-evidence disappearance.
- Signature and strategy semantics: decision signatures hash validated structured task/action/profile/correction/success/input material while excluding rationale-only prose. Verification signatures bind exact failed run/occurrence/source/status/classification/tier/command/evidence. Open-finding signatures bind sorted finding IDs plus immutable finding/report and independent audit occurrence identities. Structured strategy hashes normalize case/whitespace, sort techniques/targets, and exclude run IDs/timestamps/rationale; worker admission requires the caller strategy to match the validated supervisor strategy exactly. A first ordinary failure can retry; exact repetition at the caller-owned threshold blocks. A source-writing success with unchanged exact source revision becomes no progress and requires a materially changed strategy; retrying the identical strategy blocks before a runner.
- Circuit behavior and isolation: typed reasons cover task/action/elapsed/token exhaustion, repeated signature, unchanged source, identical strategy, missing metrics, stale evidence, cancellation, unsafe source, and accounting safety. Unsafe/unknown source blocks before the one-operation runner. Task namespaces and locks isolate counters/signatures; tests exhaust task A while task B remains clean. Breakers deep-clone and preserve existing plans, acceptance, findings, decisions, correction/verification/audit evidence, and never complete a task or disposition a finding.
- Dossier/supervisor behavior: dossiers conditionally render action limits/stops, attempt events, strategy requirements, signatures, metrics, and circuit-breaker budget/evidence. `SupervisorDecision` has an optional backward-compatible validated structured strategy contract; AW-15 worker admission requires it, terminal decisions forbid it, and the deterministic supervisor schema/prompt/profile explain that strategy and all budget/counter authority remain harness-owned.
- Tests added: unset/unlimited/zero-limited modes; below/at/above total boundaries; per-action isolation; stale CAS; transient failure then success; decision/verification/open-finding repetition; independent audit occurrence identity; unchanged source; identical versus changed strategy; elapsed/token exact boundaries; missing/negative metrics; malformed signature evidence; overflow; cancellation; unsafe source; runner non-start; restart/replay; orphan-history recovery; cross-task isolation; state monotonicity; dossier breaker rendering; prior AW-14 evidence preservation; structured strategy validation; and trusted cycle usage adaptation.
- Verification passed: `gofmt -w` on every changed Go file; `go test -count=1 ./internal/autonomousattempt ./internal/autonomous ./internal/autonomouscycle ./internal/autonomouscorrection ./internal/autonomouspolicy ./internal/autonomousverification ./internal/autonomousaudit ./internal/autonomousauditapply ./internal/autonomousstate ./internal/supervisor ./internal/ledger ./internal/prompt`; `go test -count=1 ./...`; `go vet ./internal/autonomous ./internal/autonomousattempt ./internal/autonomousstate ./internal/supervisor ./internal/prompt`; `git diff --check`.
- Verification result: all focused tests, the complete repository suite, vet checks, and diff validation passed. No live Codex/model, `revolvr run`, live supervisor, live worker, destructive Git operation, dependency addition, or commit was used.
- Remaining work: AW-16 may add conditional documentor/simplifier routing while preserving AW-15 admission/accounting. AW-15 deliberately adds no autonomous CLI/TUI/runonce path, structured AW-17 questions/resume, AW-18 worktrees/recovery, queue, daemon, or scheduler behavior.
- Blockers: none.

Task completed on 2026-07-11:

- Selected task: AW-14 — add bounded verification/audit correction and re-audit routing.
- Files changed: added `internal/autonomous/correction.go` and `correction_test.go`; added `internal/autonomouscorrection/coordinator.go` and `coordinator_test.go`; extended `internal/autonomous/contracts.go` and tests; narrowly extended `internal/autonomouscycle/artifacts.go`, `cycle.go`, `cycle_test.go`, `types.go`, `worker.go`, and `worker_prompt.go`; extended `internal/autonomouspolicy/policy.go` and tests; extended `internal/supervisor/execution.go`, `parser.go`, and `schema.go`; strengthened `.agent/profiles/corrector.md` and its canonical template in `internal/prompt/profile.go`; updated `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`.
- Public correction contracts: `autonomous.VerificationFailureTarget` identifies one exact failed verification task/run/occurrence/source/status/evidence tuple. `SupervisorDecision` now admits exactly one correction authority: the existing compatible `finding_ids` shape or one typed `verification_failure`. `ValidateVerificationCorrectionDecision` rejects invented, stale, or materially different failure targets while existing audit-finding correction JSON remains compatible.
- Structured corrector evidence: schema `autonomous-correction-output-v1`, `CorrectionOutput`, `ParseCorrectionOutput`, `MarshalCorrectionOutput`, and `CorrectionOutputSchema` bind the output to task/worker/decision plus the exact authority. Audit outputs must partition every cited ID exactly once into resolved/remaining sets and cannot name uncited findings; verification repair cannot claim an audit finding. Failed outputs cannot claim resolution, and partial outputs retain explicit remaining IDs and typed evidence.
- AW-10 extension: correctors now receive a corrector-only output schema and canonical `corrector-output.raw.json`. Worker prompts project either only the exact cited current findings or only the exact failed occurrence. Correctors still run one fast tiered verification inside the single cycle, commit only after an ordinary fast pass, never claim the final gate, and retain all existing failure/no-change/commit evidence.
- Bounded orchestration: new `autonomouscorrection.Run` composes exactly one correction cycle, one distinct final verification run/occurrence, one sequence of explicit cited-finding resolutions, one fresh audit cycle, and one existing audit application. It has no recursive call, retry budget, changed-strategy logic, no-progress detector, or circuit breaker.
- Admission and safety: the coordinator compare-and-swap loads exact current state, captures a full known-good source checkpoint, rejects any pre-correction dirty/capture ambiguity before a source writer, and admits only the current failed occurrence or explicitly selected open blocking findings from the current committed audit. The correction decision and structured output must preserve that exact authority.
- Ordering and freshness: successful correction must produce committed source changes and ordinary passed fast verification. A distinct ledger-backed final verification starts strictly after the completed correction, uses the corrected source revision, must validate as ordinary `final`/all-required-tier passed evidence, and is bracketed by unchanged full source snapshots. The fresh auditor run must be distinct from both supervisors, the corrector, and final verification, start strictly after final verification, and inspect the same corrected revision.
- Resolution and persistence: only corrector-claimed cited findings are resolved through `autonomousauditapply.ApplyFindingResolution`. Each resolution retains the exact correction decision/reference, structured corrector artifact, corrector evidence, and new final-verification evidence. Partial correction leaves uncited/unresolved findings open. Re-audit persistence remains exclusively in `autonomousauditapply`/`autonomousstate`, so earlier failed audits, findings, correction runs, verification attempts, resolutions, and reports remain immutable and reopenable. A clean persisted re-audit returns `returned_to_supervisor`; it never marks the task complete.
- Stop behavior: corrector error/timeout/cancellation, malformed output, reported failure, no changes, fast-verification failure, or commit failure starts no final verification. Failed/missing/flaky/timed-out/cancelled/invalid or source-mutating final verification starts no audit. Cancellation and dirty-state stops start no later stage.
- Tests added: exact verification-failure authority; corrector prompt retention; structured authority partitioning; full verification repair ordering; multiple-finding resolution; partial repair; uncited claim refusal; corrector failure/timeout/cancellation/malformed/no-change/commit failure; final regression/missing/flaky/timeout/cancellation/source mutation; clean independent re-audit ordering; and unsafe dirty refusal. Existing AW-09 purity, AW-10 one-worker behavior, AW-12 state/audit persistence, AW-13 flat/tier compatibility, prompt seeding, and mixed-pass behavior remain covered by the full suite.
- Verification passed: `gofmt -w` on every changed Go file; `go test -count=1 ./internal/autonomous ./internal/autonomouscycle ./internal/autonomouspolicy ./internal/autonomousverification ./internal/autonomousaudit ./internal/autonomousauditapply ./internal/autonomousstate ./internal/autonomouscorrection ./internal/supervisor ./internal/autonomousassembly`; `go test -count=1 ./internal/prompt`; `go test -count=1 ./...`; `go vet ./internal/autonomous ./internal/autonomouscorrection ./internal/autonomouscycle ./internal/autonomouspolicy ./internal/supervisor ./internal/prompt ./internal/autonomousverification ./internal/autonomousaudit ./internal/autonomousauditapply ./internal/autonomousstate ./internal/autonomousassembly`; `git diff --check`.
- Verification result: all commands passed. No live Codex, `revolvr run`, supervisor, worker, or nested model invocation was used; no commit or destructive Git operation was performed.
- Remaining work: AW-15 owns retry budgets, repeated-signature/no-progress detection, elapsed/token limits, changed-strategy requirements, and circuit breakers. AW-14 deliberately performs only one authorized correction sequence.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-13 — add tiered verification and controlled flaky-test classification.
- Files changed: added `internal/autonomousverification/contracts.go`, `execution.go`, `contracts_test.go`, and `execution_test.go`; narrowly extended `internal/verification/verification.go`; `internal/autonomouscycle/artifacts.go`, `cycle.go`, `cycle_test.go`, `types.go`, and `worker.go`; `internal/autonomouspolicy/policy.go` and `policy_test.go`; `internal/autonomousaudit/contracts.go`, `schema.go`, and `contracts_test.go`; `internal/autonomousauditapply/apply.go`; `internal/autonomousstate/audit_history.go`; `internal/autonomous/dossier.go` and `dossier_test.go`; `internal/autonomousassembly/assembly.go`; `internal/receipt/fallback.go` and `receipt_test.go`; `internal/ledger/events.go`, `artifacts.go`, and `artifacts_test.go`; `internal/app/config.go`, `config_test.go`, and `preflight.go`; `internal/cli/config.go`; `internal/runonce/runonce.go`, `effectiveconfig.go`, and `effectiveconfig_test.go`; plus `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`.
- Public tier-plan API: `autonomousverification.Plan`, `Tier`, `TierKind`, `Purpose`, `RerunPolicy`, `PlanIdentity`, `TierSelection`, `Validate`, `DecodePlan`, `MarshalPlan`, `Identity`, `Select`, `AdaptLegacy`, and `ClonePlan` define a strict map-free versioned contract. JSON decoding rejects unknown fields, trailing/multiple values, malformed enums/IDs, duplicates, unsafe directories, invalid command/env material, contradictory flags, and noncanonical ordering; canonical JSON has one final newline and hashes exact canonical bytes.
- Public execution/evidence API: `autonomousverification.Execute(context.Context, Config)`, `Result`, `GateEvidence`, `TierResult`, `CommandResult`, `CommandIdentity`, `Attempt`, `Artifact`, `MarshalResult`, `DecodeResult`, `Result.Validate`, `GateEvidence.Validate`, and `CommandMaterialSHA256` expose deterministic orchestration over injected clocks, attempt IDs, command runners, ledgers, and artifact writers. Results retain task/run/occurrence/source identity, plan schema/hash/size, purpose, complete selected/executed/required projections, exact effective command material, all attempts, output/truncation facts, timing, classification, aggregate legacy projection, failure stage/reason, and artifact identity.
- Tier ordering: validation requires exactly canonical kind order `structural`, `focused`, `task_acceptance`, `full_suite`, `race`, `integration`, `security`, with unique tier IDs and kinds and stable lower-case kebab-case IDs. Commands remain in declared order, and execution is fail-fast within the first tier that cannot pass.
- Fast versus final selection: fast selects only explicitly enabled structural/focused/task-acceptance tiers and can never satisfy the final gate. Final selects every `run_for_final` tier and requires at least one `required_for_final` tier; every required tier must be selected, executed, and ordinarily passed, and the overall selected operation must pass. Optional final tiers run only when explicitly enabled and also fail the operation if selected and unsuccessful.
- Legacy mapping: omitted tier configuration preserves the existing flat `verification.commands` path and low-level `verification.Run` behavior. `AdaptLegacy` maps a caller-supplied flat list to one `legacy-flat` `full_suite` tier that is required and selected for final verification with reruns disabled. Explicit flat plus tiered configuration fails rather than merging. Empty legacy YAML command lists retain the prior defaults. `runonce` remains `mixed-pass-v1`, continues its flat runner, and explicitly refuses a tiered autonomous plan rather than silently changing phase behavior.
- Command and attempt identity: command SHA-256 binds plan identity, purpose, tier ID/kind, zero-based position, command name, exact argv, normalized repository-contained directory, ordered environment additions, effective timeout, and effective stdout/stderr caps. Every attempt has a distinct injected ID, number, unchanged command identity, start/end/duration, exit/status, timeout/cancellation/runner-error facts, exact capped output, and truncated-byte counts; result validation rejects identity drift, malformed attempt order, more than two attempts, or more than one rerun operation-wide.
- Flaky rerun behavior: only the deterministic first ordinary failed command in a selected tier with `once_to_classify_flaky` may run once more. Missing commands, timeouts, cancellation, context cancellation, runner errors, invalid configuration, ledger/artifact failures, and already-consumed reruns are ineligible. Fail/fail remains failed with both attempts; fail/pass becomes explicit `flaky` with both attempts. Flaky never becomes passed, never satisfies a required final tier, never passes the aggregate projection, and never reaches AW-10 commit admission.
- Explicit failure behavior: missing selected commands, ordinary failure, timeout, cancellation, runner error, truncation, ledger failure, artifact failure, malformed configuration, and source mutation remain distinct result/outcome or cycle stages. Cancellation starts no later command. Verification source snapshots still surround the complete operation, so mutation during either attempt blocks. No correction, final re-verification, audit, second worker, task-state mutation, or task lifecycle transition occurs in the runner.
- Final-gate projection: `GateEvidence.Validate` recomputes final satisfaction from purpose, overall outcome, configured required tiers, selected/executed tiers, required outcomes, and missing required tiers instead of trusting a boolean. Legacy evidence with no tier projection remains compatible; typed fast, failed, flaky, missing, stale, wrong-identity, and malformed evidence fails closed.
- AW-10 integration: source-changing implement/document/simplify routes select final verification; correct selects the explicitly configured fast plan. Planner/auditor, failed workers, and no-change workers still run no verification. One tiered verification operation is executed per eligible worker; its one optional command rerun is internal to that operation. The result is returned beside the existing `verification.Result`, written deterministically to `.revolvr/runs/<worker-run-id>/verification.json`, projected into receipts/ledger, checked against post-verification source state, and admitted to commit only for an ordinary pass appropriate to the selected purpose.
- AW-09 integration: `autonomouspolicy.VerificationEvidence` carries an optional validated tier gate while the compact verification summary may carry the exact tiered result. `Evaluate` remains pure and performs no I/O, selection, execution, or mutation. Audit/document/simplify/complete require passed current source evidence and, when tiered, exact final-purpose/final-gate evidence; fast-only, missing, failed, flaky, malformed, stale, and wrong task/run/occurrence evidence reject while the documented legacy adapter remains valid.
- AW-12 integration: structured auditor provenance retains the exact optional gate and tiered result through strict raw/canonical/history JSON. The auditor schema admits typed tier provenance while Go decoding remains unknown-field strict. Audit admission, apply, and history reopen reject fast-only or unsatisfied final evidence; existing verification run/occurrence/source and independence checks, raw/canonical/history agreement, audit layout, finding identity, and resolution behavior remain unchanged.
- Dossier, receipt, ledger, and artifact integration: compact dossier summaries now render purpose, overall/final-gate status, canonical tier order, tier outcomes, command identities, both flaky attempts, timeout/cancellation, exit codes, and truncation counts without discovering state. Assembly accepts caller-supplied current verification and can reconstruct tier evidence from typed ledger completion events. `revolvr.receipt.v1` remains unchanged; ordered duplicate-command entries now retain distinct failed/passed attempts, overall flaky status remains failed, and agent prose cannot override harness evidence. Ledger adds typed tier start/completion and rerun-authorization events, retains complete results in `verification_completed`, and extracts `VerificationEvidencePath` after reopen.
- Configuration/effective identity: strict YAML supports optional tier IDs/kinds, fast/final flags, rerun policy, ordered commands, argv, directory, ordered env, per-command timeout, and per-command caps. Unknown keys still fail. Flat+tiered conflicts fail. Tier material and all effective/default verification timeout/cap settings participate in `revolvr-effective-run-config-v1` hashing; injected clocks/runners/writers/IDs do not. Legacy config-check output remains on the existing flat format; explicit tier config adds deterministic tier details.
- Purity and persistence boundaries: plan validation/selection is pure; autonomous verification owns transient execution and its run-scoped artifact only; AW-09 remains pure; dossier assembly remains read-only; AW-12 persistence remains in its existing stores; no verification scratch data was added to `ExecutionState`; no task metadata, audit/finding lifecycle, correction orchestration, CLI/TUI autonomous execution, or generalized persistence moved into the runner.
- Mixed-pass compatibility: `internal/passpolicy` production code is unchanged and remains exclusively `mixed-pass-v1`; all four phase lookup tests pass; `Lookup(autonomous-v1, ...)` still rejects; `internal/runonce` still explicitly selects `mixed-pass-v1` and uses the flat low-level verifier. No live Codex, `revolvr run`, supervisor, worker, or nested model invocation was used.
- Tests added: exhaustive tier kinds/order/selection/identity/strict-decoding/caller-ownership/legacy adaptation; deterministic passes/fail-fast/missing/timeout/cancellation/runner/truncation/ledger/artifact behavior; fail/fail and fail/pass reruns and ineligible reruns; command identity/defaults; fast/final cycle purpose, one-operation execution, artifact persistence, flaky commit refusal, two receipt attempts, conflicting configuration; pure policy final-gate cases; audit final/fast/flaky provenance; dossier flaky rendering; ledger artifact extraction; tiered YAML/effective hashing; and receipt duplicate-command attempt preservation.
- Verification passed exactly: `gofmt -w` on changed Go files; `go test -count=1` separately for `./internal/autonomousverification`, `./internal/verification`, `./internal/autonomouscycle`, `./internal/autonomouspolicy`, `./internal/autonomousaudit`, `./internal/autonomousauditapply`, `./internal/autonomousstate`, `./internal/autonomous`, `./internal/autonomousassembly`, `./internal/receipt`, `./internal/ledger`, `./internal/app`, `./internal/cli`, `./internal/passpolicy`, and `./internal/runonce`; `go vet` on all changed packages; `go test -count=1 ./...`; `git diff --check`; and `go run ./cmd/revolvr config check`.
- Verification result: all commands passed. Config check retained the flat `go test ./...` verification command and produced effective-config SHA-256 `8a883ecb9e33feb8c927615bab895eada01d8c3ac0f7c85c7b456593a1eff62a` for the current repository config.
- Remaining work: AW-14 owns bounded correction after verification/audit failure, final verification after correction, and independent re-audit. AW-13 deliberately does not start any of those loops.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-12 — persist independent audits and finding lifecycles.
- Files changed: added `internal/autonomousaudit/contracts.go`, `parser.go`, `schema.go`, `validation.go`, and `contracts_test.go`; added `internal/autonomousauditapply/apply.go` and `apply_test.go`; added `internal/autonomousstate/audit_history.go`, `audit_store.go`, and `audit_store_test.go`; narrowly extended `internal/autonomousstate/store.go`, `internal/autonomouscycle/artifacts.go`, `worker.go`, `worker_prompt.go`, and `cycle_test.go`; strengthened `.agent/profiles/auditor.md`; updated `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`.
- Public audit API: `autonomousaudit.AuditOutput` and typed `AuditProvenance`, dossier/profile/source-mutation identities, `ParseAuditOutput`, `MarshalAuditOutput`, and `AuditOutputSchema` define the strict auditor-only envelope; `ApplyReport` is the pure audit/finding-admission boundary; `ResolutionRequest` and `ApplyResolution` are the pure explicit open-to-terminal transition boundary.
- Public state API: `autonomousstate.AuditHistoryRecord`, transition kinds, resolution transition records, audit snapshots/history snapshots, `LoadAuditOperation`, `LoadAuditHistory`, `LoadCommittedAuditHistory`, `LoadCurrentAudit`, `ReplayAudit`, and `CommitAudit` provide strict immutable history, exact current-evidence reopening, replay, shared-lock CAS persistence, and typed observed results without wrapping canonical state.
- Public application API: `autonomousauditapply.ApplyAuditResult(context.Context, ApplyConfig)` consumes only one successful exact AW-10 audit/auditor route; `ApplyFindingResolution(context.Context, ResolutionConfig)` persists one explicit terminal finding transition. Both return task/operation/kind/disposition, exact previous/current state identities, audit revision, report/policy evidence, raw/canonical/history identities, finding/resolution counts, newly opened IDs, resulting state, and concrete failure stage/reason.
- Auditor-output contract and schema: schema `autonomous-audit-output-v1` is deterministic, auditor-only, strict about unknown fields and exactly one JSON value, requires one complete AW-01 report and typed audit provenance, and preserves exact raw output separately from canonical indented JSON with one final newline. Audit report inputs must retain every exact current verification evidence reference; receipt prose and Markdown never become the report.
- AW-10 integration: only `audit -> auditor` now joins the existing planner route in receiving a distinct output schema. It writes `auditor-output-schema.json` and `auditor-output.raw.json`, passes the exact schema path to fresh ephemeral Codex, embeds exact verification/source/decision/dossier/profile/latest-mutation provenance in the prompt, remains read-only, synthesizes no verification or commit, and still neither parses nor persists the audit. Planner behavior remains unchanged and implementer/corrector/documentor/simplifier remain schema-free in AW-12.
- Identity and independence validation: application requires a successful authorized worker route, exact task/action/profile/decision/reference, distinct supervisor/auditor/verification/latest-mutating runs where applicable, completed worker ledger evidence, exact fresh invocation/schema/raw/profile/dossier/task/state/source evidence, passed current verification with exact run/occurrence, unchanged task/state/source, and no auditor verification or commit. Persisted history independently revalidates those relationships on reopen.
- Canonical state and history layout: current state remains strict unwrapped AW-02 JSON at `.revolvr/autonomous/tasks/<task-id>/state.json`; audit/finding transitions use `.revolvr/autonomous/tasks/<task-id>/history/audit/<20-digit-sequence>-<kind>-<sha256(operation-id)>.json`; canonical auditor output remains `.revolvr/runs/<auditor-run-id>/auditor-output.canonical.json`. No mutable current-audit pointer was added.
- Reopen behavior: current audit authority requires a strict immutable transition whose resulting state hash/size exactly matches canonical state. Reopen verifies task/profile/supervisor/raw/canonical artifacts, reparses raw and canonical envelopes, requires canonical bytes, and rejects any raw/canonical/history disagreement. Committed-history traversal follows both audit and planning state identities backward from canonical state so orphan files do not affect durable finding identity; ordinary load performs no writes.
- Audit behavior: successful audit persistence preserves schema/task/plan/acceptance/attempts/budgets and unrelated fields, keeps lifecycle `ready`, stores the exact audit decision as `LatestDecision`, appends open resolutions only for newly admitted findings, and retains every terminal resolution. Clean reports with an open durable finding fail closed rather than implicitly resolve or erase it; clean reports coexist with historical terminal findings.
- Finding identity: reused IDs must preserve significance plus normalized lower-case/whitespace-collapsed summary and required-correction meaning; earlier evidence must remain an exact prefix and new evidence may append deterministically. Equivalent new IDs are rejected as probable renames while the prior finding is open; open findings cannot disappear; terminal IDs cannot reopen or be reused; recurrence requires a new ID.
- Finding resolutions: `resolved`, `waived`, `superseded`, and `invalid` all require typed evidence. Waived and invalid require concrete rationale; resolved may retain an exact correction decision/reference citing the finding and validates any supplied verification as passed/current with exact evidence; superseded requires a different known current target, rejects absent/invalid/superseded targets, self-links, and cycles. Every transition starts from an exact open finding and cited current audit revision, preserves unrelated state, normally remains `ready`, and has no generalized reopen semantics.
- Atomicity and compare-and-swap: audit and planning writers share the existing per-task `state.lock`; exact expected state hash/size is checked before immutable work and immediately before rename. Canonical output and immutable transition history are flushed before same-directory temporary state replacement, followed by atomic rename, directory sync, and strict readback. Existing path/symlink/canonical-state validation remains authoritative.
- Idempotency and crash recovery: operation application hashes bind exact caller expectation and admitted evidence. Exact replay returns the committed state/history without duplicates; reused operation IDs with different material conflict. A crash after immutable history but before state replacement leaves an orphan that is never current; identical application retry reuses its sequence/revision and commits it. Post-rename recovery reopens reconstructible history; injected failures cover canonical output, audit history, state temporary write/sync/rename/readback, and shared-writer serialization.
- Dossier and policy integration: reopened reports remain caller-supplied to the existing read-only AW-03/AW-04 dossier boundary while finding resolutions come from reopened state. Reopened changes-required evidence authorizes only exact open finding correction; reopened clean evidence satisfies current independent audit gates; open findings still block completion; stale verification/source and non-independent evidence fail before persistence or on strict reopen. AW-09 remains pure and performs no loading.
- Compatibility and boundaries: canonical task metadata/status, planning semantics, general supervisor behavior, CLI/TUI/app behavior, `internal/passpolicy`, the explicit `mixed-pass-v1` selection in `runonce`, receipts, verification, and commit behavior remain unchanged. AW-12 adds no automatic correction/re-audit loop, verification tiers/flaky reruns, retries, needs-input, finalization, worktrees, or live model calls.
- Tests added: strict clean/changes-required parsing, canonical schema/JSON, malformed/multiple/unknown output, provenance/identity rejection, stable/reused/renamed/disappearing findings, every terminal resolution and invalid transition, exact AW-10 auditor schema behavior, fake-cycle admission and rejection, clean/completion and changes-required/correction policy compatibility, dossier reopening, store recreation, CAS/replay/conflict, shared-lock concurrency, orphan reconciliation, artifact mismatch detection, and injected crash points. All tests use structured fixtures, temporary repositories, fake cycle/ledger/source evidence, fake times/IDs, and injected filesystem failures; no live model was invoked.
- Verification passed exactly: `gofmt -w` on all changed Go files; `go vet ./internal/autonomousaudit ./internal/autonomousauditapply ./internal/autonomousstate ./internal/autonomouscycle`; `go test -count=1` separately for `./internal/autonomousaudit`, `./internal/autonomousauditapply`, `./internal/autonomousstate`, `./internal/autonomouscycle`, `./internal/autonomouspolicy`, `./internal/autonomous`, `./internal/autonomousassembly`, `./internal/supervisor`, `./internal/ledger`, `./internal/taskfile`, `./internal/passpolicy`, and `./internal/runonce`; `go test -count=1 ./...`; `git diff --check`; and `go run ./cmd/revolvr config check`.
- Remaining work: AW-13 owns verification tiers and controlled flaky reruns; AW-14 owns automatic correction and re-audit orchestration. Later tasks retain retry/no-progress, optional roles, needs-input, worktrees, finalization, and unattended loops.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-11 — persist durable plans and acceptance-evidence matrices.
- Files changed: added `internal/autonomousplanning/contracts.go`, `parser.go`, `schema.go`, and `contracts_test.go`; added `internal/autonomousstate/history.go`, `store.go`, and `store_test.go`; added `internal/autonomousplanapply/apply.go` and `apply_test.go`; narrowly extended `internal/autonomouscycle/artifacts.go`, `types.go`, `worker.go`, `worker_prompt.go`, and `cycle_test.go` for planner-only structured output; updated `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`.
- Public planning API: `autonomousplanning.PlanningOutput` is the typed complete-plan/complete-matrix envelope; `ParsePlanningOutput`, `MarshalPlanningOutput`, and `PlanningOutputSchema` provide strict decoding, canonical encoding, and a deterministic planner-only schema; `CanonicalTaskOrigin` constructs the exact hashed task-source authority; `ApplyProposal` is the pure initial/revision transition boundary over AW-02 validation.
- Public state API: `autonomousstate.New` creates a repository-rooted store; `Load` returns a typed `Snapshot`; `LoadPlanningOperation` and `ReplayPlanning` reopen immutable operation evidence; `CommitPlanning` performs the compare-and-swap transition; `MarshalState`/`DecodeState` and `MarshalPlanningHistory`/`DecodePlanningHistory` define the strict deterministic encodings. Typed expected-state, history, artifact/state/plan identity, acceptance-count, commit-disposition, result, and injected failure-point contracts expose observed evidence without public maps or callbacks.
- Public application API: `autonomousplanapply.ApplyPlanningResult(context.Context, Config) (Result, error)` is the only AW-11 coordinator. It consumes one successful exact AW-10 planner result plus the caller's expected state, optional initial state, operation identity, and creation time, and returns created/updated/replayed disposition, exact current state, state/history identities, plan revisions, acceptance counts, planner evidence identities, and a concrete failure stage/reason.
- Planner output contract and schema: schema `autonomous-planning-output-v1` requires one exact task, one complete AW-02 plan revision, one complete acceptance matrix, typed inputs, and exact decision, worker run, planner profile, dossier, source revision, raw-output path, and profile identity. The decoder rejects empty, malformed, unknown-field, trailing, and multiple JSON values. Canonical validated output is indented deterministic JSON with one final newline and remains distinct from readable exact raw output.
- Canonical state and history layout: current state remains the unwrapped strict AW-02 `ExecutionState` at `.revolvr/autonomous/tasks/<task-id>/state.json`. Immutable versioned history uses `.revolvr/autonomous/tasks/<task-id>/history/planning/<20-digit-result-revision>-<sha256(operation-id)>.json`, and canonical planner output remains under the worker run as `planner-output.canonical.json`; history retains exact previous/result state identities, plans and matrices, decision/run/profile/dossier/source/task/raw/canonical identities, change kind, operation/application identity, and creation time.
- Identity admission: the application boundary requires a pending canonical `autonomous-v1` task; a successful `RouteKindWorker` plan/planner authorization; distinct supervisor and worker runs; accepted supervisor decision/reference; completed fresh ephemeral worker evidence; exact planner-only schema argv/artifact; exact readable hash/size-matching decision, task, dossier, profile, raw output, and admitted source identities; read-only source snapshots; and no verification or commit. Wrong action/profile/task/run/decision/artifact/dossier/source evidence fails before persistence.
- Initial planning behavior: a pending state without a plan accepts only revision 1 with no predecessor, at least one ordered stable step, no newly terminal work, and a complete grounded matrix. Success preserves schema, task, attempts, budgets, findings, and unrelated state; records the exact current `LatestDecision`; and moves the state to `ready` without changing task frontmatter or attempt/retry accounting.
- Revision behavior: deliberate revision requires a new plan ID, revision exactly plus one, and `supersedes_plan_id` equal to the current plan. Reused step IDs retain meaning, status, evidence, and rationale; terminal steps remain represented in compatible order; new terminal work is rejected; completed/skipped evidence cannot regress; prior criteria remain byte-equivalent; and new grounded criteria may be added. AW-02 transition validation remains authoritative.
- Acceptance matrix behavior: criterion IDs and normalized requirements are unique; reused IDs keep an immutable requirement and complete prior disposition; existing criteria cannot disappear or return to pending. Pending criteria have no disposition evidence/rationale; satisfied criteria require typed evidence; waived and not-applicable criteria require rationale and retain any valid evidence. Every source is exactly either the hashed canonical task artifact with a requirement present in task bytes or the current supervisor decision artifact with an exact cited success criterion; uncited planner inventions and ambiguous duplicate requirements fail.
- Atomicity and stale writes: the store validates the canonical repository/task namespace and rejects symlink components, malformed/noncanonical state, unsafe paths, missing-vs-existing expectation mismatches, and stale hash/size identities. A persistent per-task file lock serializes writers; the expected state is checked before immutable writes and immediately before replacement. Files use same-directory temporaries, file flush, atomic rename, strict readback, and directory synchronization; temporary failures never become canonical state.
- Idempotency and recovery: immutable canonical output and planning history are written and synchronized before current state becomes authoritative. A crash may leave an identical orphan history record, which retry reconciles; a committed state is never missing required history. Replaying an identical operation/application returns the existing committed result without duplicate history, while reusing an operation ID for different material fails. Reopen replay revalidates artifacts and re-synchronizes required directories, including recovery after an injected post-rename synchronization/readback failure.
- AW-10 integration: only planner routes receive `autonomousplanning.PlanningOutputSchema` through `codexexec` and write `planner-output.raw.json`; other worker actions retain their existing output behavior. AW-10 still returns transient evidence and never parses or persists state. AW-11 verifies that evidence afterward and persists through the separate application boundary; worker failures, no output, mutations, non-plan routes, terminal routes, and mismatched evidence create no state/history and no source commit.
- Completion-policy integration: reopened persisted initial state authorizes a legal implement route through the existing pure AW-09 policy. Reopened complete plans retain satisfied evidence and waived/not-applicable rationale; pending criteria still prevent completion; existing verification, audit, finding, source-freshness, and independence gates remain unchanged. Policy performs no state I/O.
- Purity and compatibility: planning proposal logic is pure, persistence is isolated in `autonomousstate`, and coordination is isolated in `autonomousplanapply`; `autonomouscycle` did not become a mutable state store. No AW-12 audit/finding persistence, retry/no-progress behavior, task lifecycle update, CLI/TUI/app wiring, or live Codex call was added. `internal/passpolicy` remains exclusively `mixed-pass-v1`, `Lookup(autonomous-v1, ...)` still fails, and `internal/runonce` still explicitly selects `mixed-pass-v1`.
- Tests added: strict/deterministic planner parsing/schema and identity cases; initial/revision/terminal-step and complete-matrix rules; every acceptance status and origin; strict state load/path/symlink/read-only behavior; deterministic persistence/reopen; CAS/concurrent-writer/idempotency/conflict behavior; 12 injected crash points and orphan reconciliation; exact AW-10 planner admission/rejection; raw/canonical readability; AW-09 implement/completion compatibility; and caller-input immutability.
- Verification passed exactly: `gofmt -w internal/autonomousplanning/*.go internal/autonomousstate/*.go internal/autonomousplanapply/*.go internal/autonomouscycle/*.go`; `go test -count=1` separately for `./internal/autonomousplanning`, `./internal/autonomousstate`, `./internal/autonomousplanapply`, `./internal/autonomouscycle`, `./internal/autonomouspolicy`, `./internal/autonomous`, `./internal/autonomousassembly`, `./internal/supervisor`, `./internal/ledger`, `./internal/taskfile`, `./internal/passpolicy`, and `./internal/runonce`; `go vet ./internal/autonomousplanning ./internal/autonomousstate ./internal/autonomousplanapply ./internal/autonomouscycle`; `go test -count=1 ./...`; `git diff --check`; and `go run ./cmd/revolvr config check`.
- Remaining work: AW-12 owns durable independent audit reports, findings, and finding-resolution transitions; later tasks retain verification tiers, correction/re-audit loops, retry/no-progress semantics, optional-role outcomes, needs-input, worktrees, finalization, and unattended operation.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-10 — wire one supervisor-directed worker cycle.
- Files changed: added `internal/autonomouscycle/artifacts.go`, `cycle.go`, `cycle_test.go`, `types.go`, `worker.go`, `worker_prompt.go`, and `worker_receipt.go`; narrowly extended `internal/gitstate/source_snapshot.go` and its tests with the canonical policy source revision; exported structural transient-evidence validation from `internal/autonomouspolicy/policy.go` with focused tests; updated `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`.
- Public cycle API: added isolated `autonomouscycle.Run(context.Context, Config) (Result, error)`. `Config` takes one exact task ID, one validated supplied AW-02 `ExecutionState`, typed AW-09 source safety/latest-mutation/verification/audit evidence, AW-04 history and guidance policy, explicit AW-05 Codex and effective-config provenance, verification/commit/lock settings, writable ledger access, clock/ID generation, and injectable task, dossier, supervisor, policy, profile, Codex, snapshot, Git-capture, verification, commit, lock, and command dependencies. `Result` contains authorization and observed dossier, supervisor, worker, artifact, source, receipt, verification-occurrence, commit, and concrete failure evidence; it does not return an updated state, persistence callback, or future command.
- Task/state/evidence validation: the cycle requires an exact nonblank task identity, validates and deep-clones the supplied execution state, requires task/state agreement, loads the canonical task, accepts only pending `autonomous-v1` metadata with its canonical state reference, rejects mixed-pass profile/phase routing, validates typed AW-09 transient evidence before supervision through `autonomouspolicy.ValidateEvidence`, and validates all explicit ephemeral Codex, effective-config, Git, verification, commit, lock, dependency, and repository-relative command-path inputs before the supervisor. Missing explicit tasks return `no_autonomous_task_or_state`; zero/malformed/wrong-task state and evidence fail closed.
- Dossier and supervisor flow: content-sensitive snapshots bracket exactly one injected/default `autonomousassembly.Assemble` call; any capture uncertainty or full source drift stops without reassembly. The cycle calls the existing `supervisor.Run` boundary once with an explicitly generated supervisor run ID and decision ID, exact dossier/audit/config/ledger evidence, and the injected command/snapshot dependencies. It revalidates the returned decision/reference, exact dossier provenance, supervisor-only profile, explicit fresh invocation, read-only full snapshots, and exact dossier-source baseline; supervisor preparation, invocation, decision, mutation, ledger, or evidence failures never reach policy or a worker.
- Supervisor/worker separation: a worker run ID is generated only for an authorized worker and must differ from the supervisor run ID. Supervisor and worker prompts, provenance, outputs, source evidence, JSONL/stderr, receipts, ledger runs, and artifact paths use separate `.revolvr/runs/<run-id>/` identities. Both invocation envelopes require one fresh ephemeral `codex exec` and forbid `resume`; no second worker ID or invocation is requested.
- Source locking and race prevention: the supervisor retains its existing internal source-writer lock. After it returns, AW-10 acquires a new cycle/worker source-writer lock, captures and validates the exact current full snapshot against the dossier and supervisor source, evaluates policy under that lock, and holds it across worker execution, source/change capture, verification, commit, and final capture with heartbeat/release handling. The cycle never nests `supervisor.Run` under the worker lock and never resets, cleans, restores, checks out, reverts, or hides changed source.
- Stable source identity and commit freshness: full `SourceSnapshot` equality, including HEAD/index/worktree entries, remains authoritative for assembly, supervisor, admission, worker, and verification race/mutation evidence. New pure `gitstate.PolicySourceRevision` hashes the current non-missing content tree (path/type/mode/size/content) while excluding HEAD, index placement, and missing tombstones, so edits/additions/deletions/renames/modes/symlinks change the policy identity but staging and committing the same verified bytes do not. Verification is tied to the exact pre-commit content revision; a final post-commit snapshot must reproduce that revision or the cycle reports a commit-freshness failure instead of relabeling stale verification.
- Policy integration: after lock-held admission the cycle builds the exact `autonomouspolicy.Input` from requested task, validated decision/reference, cloned state, current captured policy revision, typed safety/latest mutation, and current verification/audit evidence. It calls the injected/default `autonomouspolicy.Evaluate` exactly once and validates that the returned route preserves task, decision, action, profile, and source identity. Rejection returns preserved supervisor/source evidence without worker, verification, commit, task, or state mutation.
- Terminal routes: `complete` and `block` return their validated AW-09 authorization and current final source identity with no worker ID/run/session, verification occurrence, receipt, commit, task status change, execution-state change, terminal state creation, or `LatestDecision` persistence. Unsafe and unknown typed source classifications remain valid only for the policy-authorized `block` safe stop.
- Worker prompt/profile behavior: worker routes load only `.agent/profiles/<policy-profile>.md` through `prompt.LoadRunProfile`; task-frontmatter overrides are impossible for validated autonomous tasks and wrong/missing profiles block before worker ledger/Codex creation. The deterministic prompt embeds the exact repo-authored profile, dossier, decision, decision reference, route, source revision, run/output/receipt identities, current evidence, explicit no-nested-Codex/no-routing/no-commit/no-task-state-mutation rules, and action authority. Corrector prompts add an exclusive projection of only the decision-cited finding IDs and exact finding evidence; the full dossier remains the immutable context.
- Per-action mutation authority: plan/planner and audit/auditor are source read-only; any full snapshot change is an unauthorized outcome, retained without verification, commit, or automatic revert. Implement/correct/document/simplify may change source; exact worker path differences come from full observed snapshots and are cross-checked against typed changed-file capture. HEAD changes, task-file changes, canonical autonomous-state-path changes, administrative-only Git changes, capture uncertainty, and pre-existing dirty state under the existing disallow policy stop before verification/commit. Pre-existing paths are not added to the commit capture unless the worker actually changed them.
- Receipt and artifact handling: worker prompt, provenance, exact final output, source evidence, Codex JSONL/stderr, profile hash/path/size, and receipt use hashed typed artifacts. Valid same-run receipts retain original claims for mismatch warnings, then `receipt.RewriteHarnessFields` replaces verdict/timestamp/changed files/verification/commit/metrics from observed facts. Missing, malformed, or wrong-identity receipts deterministically use `receipt.FormatFallbackReceipt` and retain the parse reason. Worker claims never drive source, verification, or commit decisions; no-change/read-only/terminal evidence is ledger/artifact evidence and never forces a source commit.
- Verification behavior: only a successful source-changing worker with meaningful observed content changes runs the existing verification runner, exactly once. AW-10 creates an exact occurrence ID and returns an AW-09-compatible envelope with run/task/status/evidence and the exact content revision verified. Runner errors, failed/timed-out commands, missing commands under either existing missing-command policy, source mutation during verification, malformed capture, and ledger errors prevent commit; no corrector, rerun, tier, retry, re-audit, or second worker starts.
- Commit behavior: AW-10 calls `internal/commit` only after an authorized source-changing worker succeeds, meaningful exact changes are captured, and verification passes. It supplies the existing pre-run dirty evidence, a worker-difference-filtered changed capture, explicit missing-verification refusal, ledger recorder, and configured Git bounds. Refusal, command failure, runner error, indeterminate post-commit HEAD, final capture failure, and final content-freshness mismatch return concrete failed evidence. Successful commits return the commit SHA, pre/post HEAD evidence, final full snapshot, stable final policy revision, and the unchanged verification revision.
- Purity and compatibility boundaries: the cycle never writes task status/workflow/phase/profile/state metadata, `LatestDecision`, plans, acceptance criteria, audits, finding dispositions, terminal state, archives, or metadata-only commits. It adds no retries, correction/re-audit loop, verification tiers, needs-input, worktrees, CLI/TUI/app integration, queue/daemon behavior, or model invocation in tests. `internal/runonce` remains the existing mixed runner and still explicitly selects `mixed-pass-v1`; `internal/passpolicy` remains the unchanged four-phase ordered policy and still rejects `autonomous-v1`.
- Tests added: fake-driven coverage includes exact autonomous task/state/evidence validation, missing task/state, mixed-pass rejection, malformed paths/config, stable and changing dossier windows, supervisor failure/missing-invalid decision/source mismatch/source and durable-state mutation, admission race, exactly-one policy evaluation/rejection, unsafe/unknown block, every action/profile, distinct fresh supervisor/worker identities and artifacts, wrong/missing profiles, exact correction scope, terminal non-mutation, planner/auditor mutation, worker failure, task/state immutability, pre-existing work, no destructive Git commands, exact changed paths/capture failure, passed/failed/timed-out/missing/error verification, no second worker, valid/missing/malformed receipts and claim disagreement, no-change/read-only no-commit behavior, commit success/refusal/failure/runner error/indeterminate/freshness mismatch, ledger errors, deterministic projections, and worker artifact readability after ledger reopen. Gitstate tests prove content changes affect the policy revision while stage/commit and committed deletion bookkeeping do not.
- Verification run: `gofmt -w internal/autonomouscycle/*.go internal/autonomouspolicy/policy.go internal/autonomouspolicy/policy_test.go internal/gitstate/source_snapshot.go internal/gitstate/source_snapshot_test.go`; `go test -count=1 ./internal/autonomouscycle`; `go test -count=1 ./internal/autonomouspolicy`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./internal/autonomousassembly`; `go test -count=1 ./internal/supervisor`; `go test -count=1 ./internal/codexexec`; `go test -count=1 ./internal/gitstate`; `go test -count=1 ./internal/verification`; `go test -count=1 ./internal/receipt`; `go test -count=1 ./internal/commit`; `go test -count=1 ./internal/ledger`; `go test -count=1 ./internal/taskfile`; `go test -count=1 ./internal/passpolicy`; `go test -count=1 ./internal/runonce`; `go test -count=1 ./...`; `go vet ./internal/autonomouscycle`; `git diff --check`; `go run ./cmd/revolvr config check`.
- Verification result: every focused package, the complete repository suite, focused vet, diff validation, and non-model config smoke check passed. No live Codex model, supervisor pass, worker pass, nested Codex, `revolvr run`, verification workflow against the repository, task/state transition, source commit, reset/clean/restore/checkout, or destructive command was invoked.
- Remaining work: begin AW-11 in a new fresh session to persist validated plan revisions and acceptance-evidence matrices. AW-12 and later still own durable audits/findings, verification tiers, correction/re-audit loops, retry/no-progress policy, conditional-role outcomes, needs-input, isolation, finalization, and unattended loops.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-09 — implement pure autonomous routing and completion policy.
- Files changed: `internal/autonomouspolicy/policy.go`, `internal/autonomouspolicy/policy_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Public policy API: added isolated `autonomouspolicy.Evaluate(Input) (Route, error)`. `Input` carries the requested task, validated AW-01 `SupervisorDecision`, AW-02 `DecisionReference` and `ExecutionState`, exact source evidence, and optional verification/audit evidence. `Route` is a compact immutable-by-value authorization containing worker/complete/block kind, task and decision IDs, action, exact worker profile, and current source revision; it contains no commands, callbacks, transitions, or persistence instructions.
- Evidence and freshness contract: current source identity is an exact lower-case SHA-256 compatible with `gitstate.SourceSnapshot.SnapshotSHA256`, paired with typed `safe|unsafe|unknown` safety and optional latest same-task source mutation run/decision/action/resulting-revision provenance. Verification reuses `autonomous.VerificationSummary` and adds the exact verified source revision; policy evidence requires nonempty verification run/occurrence identity and typed evidence. Audit embeds the exact validated `AuditReport` and adds audit run, auditor profile, audited source revision, and the consumed verification run/occurrence identity. No transient freshness fact was added to task Markdown or AW-01/AW-02 persisted JSON.
- Identity and provenance policy: evaluation calls AW-01/AW-02 validators, requires exact requested/decision/reference/state/evidence task agreement, exact decision/reference action and profile agreement, valid evidence status/profile/safety/source identities, and rejects materially changed reuse or replay of a decision ID already present as the latest or a finding-resolution reference. It never updates `ExecutionState.LatestDecision`.
- Lifecycle admission: pending admits only plan and block; ready may consider all eight actions; planning, working, verifying, auditing, and correcting reject concurrent routing; needs_input and finalizing reject routing; completed, blocked, and cancelled reject every new route without explicit reopen semantics.
- Action rules: plan requires planner and can create or revise a plan without verification/audit but cannot bypass a current unresolved blocking finding. Implement requires implementer plus a valid current plan with a pending/in-progress step and rejects completed/all-terminal plans or unresolved changes-required findings. Audit requires auditor, safe source, and fresh passed verification. Correct requires corrector, fresh current independent changes-required audit evidence, `ValidateCorrectionDecision`, known finding IDs, and only unresolved selected findings; deterministic subsets are allowed. Document and simplify independently require their exact profiles plus current passed verification and a linked fresh independent clean audit. Block has no worker and remains legal from pending/ready under safe, unsafe, or unknown safety when its typed decision inputs are valid.
- Verification, audit, and independence: passed verification is fresh only for an exact current source-revision match. An audit is usable only when it came from `auditor`, covers that same current source, consumed the exact current verification run/occurrence, has a nonempty run ID, and is distinct from the latest source-mutating worker run. Missing, failed, malformed, stale, differently linked, non-auditor, non-independent, and changes-required evidence fail closed where a clean gate is required.
- Completion gates: complete is authorization only and requires ready lifecycle, safe known source, a completed plan with every step terminal, at least one acceptance criterion, no pending criterion, AW-02-valid satisfied/waived/not-applicable evidence and rationale, fresh passed verification, a linked fresh independent clean audit, and no open finding resolution. Resolved, waived, superseded, and invalid findings remain explicitly representable and accepted; current changes-required evidence is rejected. The policy does not create a completed state/task, event, artifact, receipt, capsule, archive, or commit.
- Optional roles and safety: completion requires neither documentor nor simplifier attempts, and there is no document-before-simplify order. Every worker route and complete requires caller-classified safe source evidence; block is the sole unsafe/unknown safe-stop exception. Policy never parses Git status prose or treats dirty as inherently unsafe.
- Purity and determinism: the package imports only standard-library helpers and `internal/autonomous`; it performs no filesystem, Git, ledger, taskfile, command, clock, random-ID, Codex, verification, or persistence operation. Repeated equal inputs return equal routes/errors, public outputs contain no mutable slices/maps, deterministic validation order is explicit, and tests compare deep input JSON before/after successful and rejected evaluation.
- Mixed-pass compatibility: `internal/passpolicy` remains the unchanged four-phase `mixed-pass-v1` policy and still rejects `autonomous-v1`; `internal/runonce` still explicitly selects `taskfile.WorkflowMixedPassV1`. No taskfile, app, CLI, TUI, supervisor, receipt, ledger, commit, or runtime behavior changed.
- Tests added: exhaustive tables cover common identity/structural/provenance validation, unknown/malformed inputs, decision reuse/replay, deterministic non-mutation, all eight actions across every lifecycle, all six exact worker profiles, unsafe/unknown source handling, initial/revision planning, implementation plan states, verification freshness, audit source/link/profile/independence/disposition, correction subsets and every finding disposition, optional document/simplify gates and ordering independence, every completion prerequisite/failure mode, and block admission/mutation behavior. Tests use only in-memory values and invoke no command, repository, ledger, `.revolvr/`, or model fake.
- Verification run: `gofmt -w internal/autonomouspolicy/policy.go internal/autonomouspolicy/policy_test.go`; `go test -count=1 ./internal/autonomouspolicy`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./internal/passpolicy`; `go test -count=1 ./internal/taskfile`; `go test -count=1 ./internal/runonce`; `go test -count=1 ./internal/autonomousassembly`; `go test -count=1 ./internal/supervisor`; `go test -count=1 ./...`; `git diff --check`; `go run ./cmd/revolvr config check`.
- Verification result: all focused policy/domain/compatibility tests, autonomous assembly/supervisor fake tests, the full repository suite, diff validation, and non-model configuration smoke check passed. No live Codex model, supervisor or worker session, verification workflow, repository mutation command, state/task persistence, receipt, or commit was invoked.
- Remaining work: begin AW-10 in a separate fresh session to consume the pure authorization while orchestrating exactly one supervisor-directed worker cycle. AW-11 and later still own durable plan/acceptance/audit/finding state, verification tiers, correction loops, retry budgets, optional-role outcomes, needs-input, finalization, and archival.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-08 — add compatible `autonomous-v1` task lifecycle metadata.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/autonomous_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/passpolicy/policy_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Autonomous metadata contract: added canonical workflow identity `autonomous-v1` and typed `taskfile.Task.AutonomousStatePath`. A valid autonomous task has `workflow: autonomous-v1`, no mixed-pass `phase`, no task-level profile override, and exactly `autonomous_state_path: .revolvr/autonomous/tasks/<task-id>/state.json`. Existing and newly created/imported tasks without a workflow remain `mixed-pass-v1` and continue defaulting a missing phase to `implement`.
- Workflow-specific validation: mixed-pass tasks retain the established four phases and profile-frontmatter compatibility but reject autonomous state metadata. Autonomous tasks require their canonical same-task state reference and reject phase/profile routing metadata. Duplicate recognized keys, unknown workflows, cross-workflow metadata, empty/absolute/traversing/outside/wrong-task state paths, and malformed files fail with source-path context.
- State-path safety and non-creation: autonomous state references are exact normalized repository-relative paths under the owning task namespace. Validation reuses repository path safety and rejects existing symlink components, including external escapes and redirects into another task namespace. Parsing, listing, selecting, retrying, and snapshot updating never create `.revolvr/` or the referenced state file and never load, validate, overwrite, or delete AW-02 state.
- Selection boundaries: added `ListRunnableForWorkflow` and `SelectNextForWorkflow`; both validate the requested workflow, parse every canonical task, preserve duplicate-ID/malformed-file failures, filter only pending tasks, and retain priority-first plus filename-tie-break ordering. Compatibility `ListRunnable`/`SelectNext` explicitly mean default mixed-pass selection, and `runonce` now names `mixed-pass-v1` directly so earlier/higher-priority autonomous tasks cannot be sent to the ordered-phase runner. App read surfaces project autonomous workflow identity without inventing a phase/profile/next state and continue marking only the current mixed-pass runner's next task.
- Status update and retry preservation: the existing atomic task writer now edits raw line records instead of normalizing the complete document. Status-only autonomous retry changes only the status line while preserving workflow, state reference, priority, unknown frontmatter, H1/specification bytes, original CRLF/LF convention, and no-final-newline bodies. Repeated retry remains non-applicable, phase updates against autonomous tasks fail before replacement, rejected updates leave original bytes intact, and snapshot updates retain the snapshot's identity/workflow/state evidence under the existing documented snapshot semantics.
- Migration safeguards: mixed-pass metadata cannot be interpreted as autonomous and autonomous metadata cannot fall back to mixed-pass. Existing status/phase update APIs cannot change workflow or remove/replace an autonomous state reference; no general workflow-switch or force-discard API was added, and no runtime evidence is deleted.
- Creation/import and mixed-pass compatibility: `taskfile.Create`, app add, and Markdown import remain pending mixed-pass creation paths with missing-workflow/missing-phase compatibility defaults; dry-run import remains non-mutating. `internal/passpolicy` still supports only the ordered `mixed-pass-v1` phase policy and explicitly rejects `autonomous-v1`. No supervisor/dossier call, AW-02 persistence, decision persistence, ledger event, Codex/worker execution, verification, receipt, commit, task completion, CLI command, or TUI lifecycle behavior was added.
- Tests added: valid/default workflow parsing; every current mixed phase; autonomous status/priority/no-phase projection; duplicate/missing/empty/cross-workflow lifecycle metadata; absolute/traversing/outside/wrong-task state references; external and cross-namespace symlinks; deterministic coexisting selection; non-pending exclusion; priority and filename ordering; duplicate IDs; malformed-file blocking; no runtime-state creation; CRLF/no-final-newline retry; unknown-frontmatter/spec preservation; retry idempotence; rejected phase update atomicity; snapshot lifecycle preservation; app projection/retry; add/import defaults; pass-policy rejection; and current runonce isolation. All command/model paths use package fakes where execution behavior is tested.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/autonomous_test.go internal/app/app.go internal/app/app_test.go internal/passpolicy/policy_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test -count=1 ./internal/taskfile`; `go test -count=1 ./internal/app`; `go test -count=1 ./internal/passpolicy`; `go test -count=1 ./internal/runonce`; `go test -count=1 ./internal/autonomous ./internal/autonomousassembly ./internal/supervisor`; `go test -count=1 ./...`; `git diff --check`; `go run ./cmd/revolvr config check`.
- Verification result: all focused tests, autonomous compatibility tests, the full repository suite, diff validation, and non-model config smoke check passed. No live Codex model, supervisor pass, worker pass, verification workflow, receipt, commit, task transition beyond temporary test fixtures, or autonomous state file was started.
- Remaining work: begin AW-09 in a separate fresh session; legal autonomous routing/completion gates, supervisor/worker orchestration, state persistence, correction loops, and autonomous CLI/TUI presentation remain unwired.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-07 — add a fresh, decision-only supervisor execution pass.
- Files changed: `internal/supervisor/execution.go`, `internal/supervisor/parser.go`, `internal/supervisor/prompt.go`, `internal/supervisor/schema.go`, `internal/supervisor/execution_test.go`, `internal/supervisor/prompt_schema_test.go`, `internal/codexexec/codexexec.go`, `internal/codexexec/invocation.go`, `internal/codexexec/codexexec_test.go`, `internal/gitstate/source_snapshot.go`, `internal/gitstate/source_snapshot_test.go`, `internal/ledger/events.go`, `internal/ledger/artifacts.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Supervisor pass API: added isolated `internal/supervisor.Run`, which accepts explicit repository/task/dossier/audit/ledger identities, exact Codex model/effort/session/sandbox/approval/version/effective-config provenance, injected Codex and Git runners, source snapshotter, clock, and IDs. It loads only the repo-authored `supervisor` profile, returns a validated AW-01 decision plus AW-02-compatible decision provenance, and never discovers a task, persists execution state, routes a worker, verifies, commits, or mutates a task file.
- Prompt and schema behavior: added deterministic prompt rendering that embeds the exact loaded supervisor profile and exact validated AW-04 dossier Markdown with task/schema/hash/size identity plus direct decision-only, exact-one-object, no-mutation, no-worker, no-nested-Codex, and Revolvr-authority instructions. Added a deterministic standard-library JSON Schema with one final newline, strict top-level/evidence properties, every AW-01 action and action/profile pair, terminal no-worker rules, nonblank criteria/evidence, and correction finding constraints.
- Codex invocation behavior: extended `internal/codexexec` with one typed optional output-schema path. It is repository-contained, must name a nonempty regular file, appears exactly once in fresh `codex exec` argv, remains absent from existing invocations, and flows through exact AW-05 invocation provenance alongside explicit model, effort, `--ephemeral`, sandbox/approval behavior, absolute working directory, output-last-message path, Codex version, and effective-config schema/hash. No resume or generic extra-argument injection was added.
- Exact parsing and AW-01 validation: the output-last-message file remains the exact raw output artifact. Strict decoding rejects empty/missing output, malformed/fenced/prose-wrapped/multiple JSON values, unknown properties, unknown enum values, missing fields, invalid action/profile combinations, terminal workers, invalid criteria/evidence, invalid/duplicate/misplaced finding IDs, and task mismatches. Go calls `SupervisorDecision.Validate`; correction additionally requires a valid same-task `changes_required` audit and calls `ValidateCorrectionDecision` against exact known findings.
- Source-mutation detection: added content-sensitive `gitstate` source snapshots covering stable HEAD, index records, tracked filesystem content/mode/type, deleted paths, and non-ignored untracked content while excluding only `.revolvr/`. Snapshots and their hashes validate before use. The pass holds the existing source-writer lock across the Codex window, captures immediately before and after Codex even on invocation failure/timeout, rejects capture uncertainty/truncation, records concrete before/after/path evidence, detects changes to already-dirty paths, and never commits, resets, cleans, restores, reverts, or otherwise hides a mutation.
- Artifact and provenance behavior: each supervisor run uses stable `.revolvr/runs/<run-id>/` paths for `supervisor-prompt.md`, `supervisor-output-schema.json`, exact `supervisor-output.json`, successful canonical `supervisor-decision.json`, `supervisor-provenance.json`, `supervisor-source.json`, rejection diagnostics, `codex.jsonl`, and `codex.stderr`. Raw and canonical decisions remain distinct; materialized artifact paths carry SHA-256 and byte size; the provenance artifact retains the dossier manifest, profile identity/path/hash/size, exact invocation, and expected artifact paths.
- Ledger behavior: added minimal `supervisor_prepared`, `supervisor_decision_validated`, `supervisor_decision_rejected`, and `supervisor_source_mutation_detected` events while retaining `codex_started`/`codex_completed` and ordinary run terminal events. Accepted decisions complete only the supervisor ledger run; every malformed, invalid, mismatched, mutated, timed-out, failed, or uncertain pass fails that run. Verification remains `not_run`, commit SHA remains empty, no receipt or commit event is created, and even an accepted `complete` action does not complete the durable task.
- Compatibility preserved: no supervisor entry point was added to CLI, TUI, app, task selection, `runonce`, or `mixed-pass-v1`; current worker profiles/phases, profile loading/init behavior, AW-01/AW-02 contracts, AW-03/AW-04 assembly, AW-05 provenance, AW-06 profiles, receipts, verification, and commits remain unchanged. Existing Codex argv without a schema is byte-for-byte covered by prior exact-argv tests.
- Tests added: deterministic prompt/schema structure; exact profile/dossier inclusion; typed output-schema invocation and path refusal; all eight valid actions; raw/canonical artifacts and hashes; invocation/dossier/profile provenance; exact accepted/rejected/mutation event sequences; ledger reopen; every required malformed/invalid decision class; missing profile/invalid dossier/audit/provenance; ledger failures; clean source behavior; tracked, staged, untracked, deleted, renamed, already-dirty, and HEAD mutation; capture failure/truncation/invalid snapshots; mutation plus valid output; mutation plus Codex failure; nonzero exit; and timeout. Every Codex path uses an injected fake command runner.
- Verification run: inspected `codex exec --help` and confirmed `--output-schema` without starting a model; `gofmt -w` on all AW-07 Go files; `go test -count=1 ./internal/supervisor ./internal/codexexec ./internal/gitstate ./internal/ledger ./internal/autonomous ./internal/autonomousassembly`; `go test -count=1 ./internal/passpolicy`; `go test -count=1 ./internal/runonce ./internal/prompt`; `go test -count=1 ./...`; `git diff --check`; `go run ./cmd/revolvr config check`.
- Verification result: all focused tests, mixed-pass compatibility tests, the full repository suite, diff validation, and non-model config smoke check passed. No live model, nested Codex invocation, worker execution, verification pass, task transition, receipt, or commit was started.
- Remaining work: begin AW-08 in a separate fresh session; autonomous task lifecycle metadata/selection, AW-09 routing/completion gates, worker execution, state persistence, correction loops, and autonomous CLI/TUI presentation remain unwired.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-06 — seed repo-authored supervisor, planner, and corrector profiles.
- Files changed: `.agent/profiles/supervisor.md`, `.agent/profiles/planner.md`, `.agent/profiles/corrector.md`, `internal/prompt/profile.go`, `internal/prompt/profile_test.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Supervisor profile contract: added the canonical decision-only, read-only `supervisor` profile. It treats the supplied dossier/evidence as complete context, emits exactly one harness-schema-conforming next-action decision with exact action/profile compatibility and evidence, refuses unsupported completion claims, cites correction finding IDs, distinguishes missing from negative evidence, and cannot edit files/state/evidence, commit, execute workers, route safety policy, invoke nested Codex, or resume sessions.
- Planner profile contract: added the canonical AW-01 worker profile `planner`. It builds or revises ordered durable plans from supplied task/state/acceptance/guidance evidence, preserves task scope plus plan revision/predecessor and stable lower-case kebab-case ID relationships, defines reviewable observable steps and verification/audit needs, makes missing inputs and uncertainty explicit, and cannot implement, edit, verify, persist state, route work, invoke nested Codex, or resume sessions.
- Corrector profile contract: added the canonical AW-01 worker profile `corrector`. It may change only source/tests needed for explicit verification failures or cited audit finding IDs plus a harness-designated response/receipt, preserves unrelated user work and architecture, requires root-cause repair and relevant verification evidence, reports partial or blocked corrections honestly, and cannot broaden into unrelated cleanup/docs/simplification/roadmap work, disposition findings, route workers, claim overall completion, invoke nested Codex, or resume sessions.
- Template ordering and init behavior: `prompt.DefaultRunProfileTemplates` remains the single deterministic init registry and now orders `supervisor`, `planner`, `implementer`, `auditor`, `corrector`, `documentor`, and `simplifier`. The existing init loop seeds normalized final-newline files only when missing; custom files, including differing line endings or no final newline, remain byte-identical, mixed existing/missing sets create only missing profiles, and repeated init leaves all profiles unchanged.
- Structured-output and fresh-session rules: all three profiles require fresh-session, evidence-grounded work, exact harness-supplied structured output, no prior transcript reliance, no nested Codex invocation, and no resume behavior. Each profile states its mutation and authority boundary directly; no runtime output contract, parser, persistence, execution, or routing behavior was added.
- Compatibility preserved: `DefaultRunProfileName` remains `implementer`; `mixed-pass-v1` still maps only implement, audit, document, and simplify to its established profiles and phase sequence. Existing profile loading/trimming/path validation/missing/empty errors, init output/directories/Git-local ignore behavior, existing four templates, AW-01/AW-02 contracts, AW-03/AW-04 dossier behavior, AW-05 invocation provenance, receipts, verification, commits, CLI, and TUI behavior remain unchanged.
- Tests added: deterministic unique template order and default identity; exact planner/corrector agreement with AW-01 worker-profile values; semantic supervisor/planner/corrector contract assertions; checked-in/template byte equality and successful loading; actionable missing-file errors for every new profile; fresh init seeding and template contents; byte-preserving custom-profile non-overwrite; mixed existing/missing files; and repeated-init profile stability. Existing pass-policy coverage confirms the established `mixed-pass-v1` mapping.
- Verification run: `gofmt -w internal/prompt/profile.go internal/prompt/profile_test.go internal/cli/root_test.go`; `go test -count=1 ./internal/prompt`; `go test -count=1 ./internal/cli`; `go test -count=1 ./internal/prompt ./internal/cli ./internal/passpolicy`; `go test -count=1 ./...`; `git diff --check`; `go run ./cmd/revolvr config check`.
- Verification result: all focused tests, the full repository suite, diff validation, and non-model config smoke check passed. CLI init tests exercised fresh, mixed, custom, and repeated initialization in temporary workspaces. No live Codex model, nested Codex command, autonomous role, or task execution was invoked.
- Remaining work: begin AW-07 in a separate fresh session; supervisor execution, schema generation/parsing, autonomous routing, worker execution, persistence, and lifecycle presentation remain unwired.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-05 — pin and record the intended Codex model, effort, and session mode.
- Files changed: `internal/app/config.go`, `internal/app/config_test.go`, `internal/app/preflight.go`, `internal/app/app_test.go`, `internal/cli/config.go`, `internal/cli/root_test.go`, `internal/cli/doctor_test.go`, `internal/codexexec/codexexec.go`, `internal/codexexec/invocation.go`, `internal/codexexec/version.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/runonce/effectiveconfig.go`, `internal/runonce/effectiveconfig_test.go`, `internal/runonce/runonce_test.go`, `internal/prompt/context.go`, `internal/prompt/prompt_test.go`, `internal/ledger/artifacts_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Config defaults and validation: the shared app/runonce configuration path now defaults Codex to executable `codex`, model `gpt-5.6-sol`, reasoning effort `xhigh`, and required ephemeral sessions. Pointer-backed YAML fields distinguish omission from explicitly empty model/effort values; empty or structurally malformed models, empty/unknown efforts outside `minimal|low|medium|high|xhigh`, `ephemeral: false`, unknown YAML fields, and conflicting bypass aliases fail before commands run. Existing omitted-key configs and the `dangerously_bypass_approvals_and_sandbox`/`yolo` aliases remain compatible.
- Exact invocation flags and ordering: dangerous-bypass runs use `exec --json --model <model> -c model_reasoning_effort=<effort> --ephemeral --dangerously-bypass-approvals-and-sandbox --cd <absolute-workdir> [--output-last-message <absolute-path>] -`. Non-bypass runs retain global `--ask-for-approval <policy>` before `exec`, replace the bypass flag with `--sandbox <mode>` after the pinned session flags, and otherwise keep the same order. Every value remains a separate argv element; model/effort/ephemeral occur exactly once, stdin ends with `-`, and no argv includes `resume`.
- Codex version discovery: `internal/codexexec.DiscoverVersion` runs only `<configured-executable> --version` through the injectable bounded runner, with timeouts and stdout/stderr caps. It requires a successful nonempty single-line result and reports timeouts, execution errors, nonzero exits, truncation, and malformed output. Runtime and doctor/preflight use this same parser; no model is invoked for discovery or config checking.
- Effective-config hashing: schema `revolvr-effective-run-config-v1` serializes a map-free compact JSON projection containing normalized working directory, Codex model/session/sandbox/approval/bypass/timeouts/caps, Git settings, ordered verification policy/commands, commit policy, and source-writer lock policy. SHA-256 covers the compact bytes; callbacks, open stores, clocks, per-run IDs, and artifacts are excluded, and caller-owned slices are copied.
- Context and ledger provenance: one typed `codexexec.InvocationProvenance` records executable, actual version, model, effort, ephemeral/session mode, effective-config schema/hash, absolute working directory, and exact argv. The same value is written into `context.json`, the `context_built` event, and the extended `codex_started` event; the manifest is written before Codex starts. Existing context task/profile source hashes, artifact paths, and ledger artifact extraction remain unchanged.
- Config-check and doctor behavior: `revolvr config check` deterministically renders model, effort, ephemeral session mode, effective-config schema, and SHA-256 without executing Codex. Doctor preserves the existing readiness sequence while adding effective model/effort/session and bounded version checks; invalid config stops before command execution, and version failures make readiness false with actionable detail.
- Compatibility preserved: `mixed-pass-v1` selection and phase advancement, repo-authored profile selection, receipts, verification, commits, CLI/TUI run controls, dangerous-bypass/yolo behavior, fresh `codex exec`, and existing artifact decoding remain intact. The strict success smoke assertion was aligned with the established implement-to-audit pending transition; no AW-06 profiles, autonomous routing, supervisor execution, persistence, resume support, or lifecycle UI was added.
- Tests added: focused configuration defaults/overrides/error cases; exact bypass/non-bypass and optional-artifact argv slices; every bounded version-discovery outcome; deterministic effective-config bytes/hash, material changes, process-local exclusions, and slice ownership; deterministic context provenance and source hashes; manifest/context/codex event agreement; extra-field artifact extraction; preflight/doctor version/config failures; and strict fake-Codex version/model/effort/ephemeral/no-resume parsing.
- Verification run: `gofmt -w` on every changed Go file; `go test -count=1 ./internal/app`; `go test -count=1 ./internal/codexexec`; `go test -count=1 ./internal/prompt`; `go test -count=1 ./internal/ledger`; `go test -count=1 ./internal/runonce`; `go test -count=1 ./internal/cli`; `go test -count=1 ./...`; `git diff --check`; `codex --version`; `codex exec --help`; `go run ./cmd/revolvr config check`; expected-failure `go run ./cmd/revolvr doctor`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`.
- Verification result: all focused tests, the full repository suite, diff validation, CLI/config inspection, shell syntax checks, and both fake-model smoke scripts passed. Doctor correctly discovered `codex-cli 0.144.1` and showed the intended model/effort/session; it was not ready only because the implementation worktree contains the known preserved AW-01–AW-04 and AW-05 changes. No real Codex task was started.
- Remaining work: begin AW-06 in a separate fresh session; supervisor execution, autonomous routing/storage, resume support, and lifecycle presentation remain unwired.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-04 — assemble task dossiers from repository and runtime evidence.
- Files changed: `internal/autonomous/dossier.go`, `internal/autonomous/dossier_test.go`, `internal/autonomousassembly/assembly.go`, `internal/autonomousassembly/assembly_test.go`, `internal/ledger/store.go`, `internal/ledger/ledger_test.go`, `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Assembly and read-only evidence behavior: added `internal/autonomousassembly.Assemble`, which validates caller-supplied AW-02 state and optional AW-01 audit evidence, loads the canonical task and exact source bytes, queries an explicitly bounded selected-task history window, retains exact receipt bytes, derives current verification evidence, captures Git and applicable guidance, and feeds the pure AW-03 projection without writing repository, runtime, ledger, caller, or Git state. Missing optional history and a missing ledger remain explicit without creating `.revolvr/`; present ledgers open through an immutable read-only path without schema initialization, migrations, parent-directory creation, or journal sidecars.
- Identity and stable-HEAD safeguards: requested task identity is cross-checked against state, audit, task files, ledger runs, identity-bearing events, receipts, commits, and verification evidence. Receipt paths are repository-contained through symlink-aware path resolution. Git HEAD is captured before and after task/history/receipt/guidance/status/diff collection; changing HEAD fails with both identities, so status and diff evidence belong to one stable HEAD window.
- History, receipt, verification, Git, and guidance behavior: ledger history filters by task before applying the collection limit, sorts newest-first with ascending run ID as the timestamp tie-breaker, probes one bounded older item, and performs no query for a zero limit. Events are ordered without mutating caller slices; malformed irrelevant payload bytes remain available while identity/provenance payloads are validated strictly. Receipt task/run/pass/commit/verification conflicts fail with source context. Git commands are read-only, timeout/output bounded, and truncation-aware. Applicable root-to-task `AGENTS.md` plus configured guidance is path-sorted, exact-byte preserving, and represented with task, receipt, history-window, Git, and guidance provenance in the dossier manifest.
- Root causes resolved: task discovery now reserves exact direct `AGENTS.md` files for nested guidance while continuing to parse every other direct Markdown file as a task, preserving deterministic lookup, duplicate-ID errors, and malformed-task failures. The zero-history test now compares slice contents so an empty derived slice does not fail solely because it is non-nil.
- Tests added or corrected: temporary Git/ledger fixtures cover complete and sparse assembly, task/state/audit/run/event/receipt provenance failures, task-filtered bounded history, zero-query history, deterministic ordering, malformed irrelevant versus identity-bearing events, caller-owned state/audit/event/payload non-mutation, receipt traversal and symlink escapes, exact manifest provenance, guidance failures, malformed and read-only ledgers, bounded Git commands, and synthetic/real changing-HEAD cases. Focused taskfile coverage proves list/find/select/create and mixed-pass phase behavior beside nested `AGENTS.md`, while malformed real task files remain errors; read-only ledger coverage verifies unchanged database bytes and directory entries.
- Verification run: `gofmt -w internal/autonomous/contracts.go internal/autonomous/contracts_test.go internal/autonomous/state.go internal/autonomous/state_test.go internal/autonomous/dossier.go internal/autonomous/dossier_test.go internal/autonomousassembly/assembly.go internal/autonomousassembly/assembly_test.go internal/ledger/store.go internal/ledger/ledger_test.go internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go`; `go test -count=1 ./internal/autonomousassembly`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./internal/ledger`; `go test -count=1 ./internal/taskfile`; `go test -count=1 ./...`; `git diff --check`.
- Verification result: all commands passed.
- Remaining work: begin AW-05 in a separate fresh session; no supervisor execution, autonomous routing/storage, Codex model/session pinning, CLI/TUI presentation, or AW-05 behavior was added in AW-04.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-03 — build a deterministic task-dossier projection and manifest.
- Files changed: `internal/autonomous/dossier.go`, `internal/autonomous/dossier_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Dossier contracts added: introduced a pure `TaskDossierInput`/`TaskDossier` projection over exact task/spec bytes, one validated AW-02 `ExecutionState`, optional AW-01 audit evidence, a minimal verification summary, deterministic recent-run summaries with an explicit limit, a caller-supplied Git snapshot, and exact repository-guidance sources. The builder performs no file, repository, ledger, receipt, Git, clock, network, prompt, or execution work and does not mutate caller-owned values.
- Markdown contract added: the stable section order is Dossier Identity, Canonical Task/Spec, Current Autonomous State, Current Plan, Acceptance Criteria, Verification, Audit and Finding Resolutions, Recent Runs, Git Snapshot, Repository Guidance, then Omissions and Truncation. Raw task and guidance content retains its supplied bytes with one deterministic structural newline boundary; optional sections render explicit absence statements.
- Manifest contract added: schema `autonomous-task-dossier-manifest-v1` records task identity, final dossier SHA-256/byte size, ordered source records, raw-source exact byte hashes and sizes, compact-JSON hashes and sizes for typed sources, included-byte counts for raw sources, item counts for recent history, truncation flags, and ordered omission/truncation facts. Typed hashes use `encoding/json` compact JSON over structs or deterministically ordered slices; the manifest itself uses indented JSON plus a final newline and does not self-hash.
- Truncation behavior added: recent runs sort newest-first by supplied `started_at` with ascending run ID as the tie-breaker; zero explicitly includes none; the newest bounded prefix is rendered without reordering the caller slice; omitted history is visible in Markdown and manifest counts; the source hash always covers the complete deterministically ordered history.
- Tests added: fixed full-dossier Markdown and manifest byte snapshots; stable section/source/guidance/run ordering; repeated rendering and manifest determinism; input slice and byte-buffer non-mutation; exact raw and canonical typed provenance; final dossier hash/size; trailing-newline and CRLF distinctions; sparse input; clean and changes-required audits; actionable validation errors; and below/exact/above/zero history limits with stable retained selection and full-history hashes.
- Verification run: `gofmt -w internal/autonomous/dossier.go internal/autonomous/dossier_test.go`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./...`; `go vet ./...`; `git diff --check`.
- Verification result: all commands passed.
- Remaining work: begin AW-04 in a new fresh session to assemble these pure inputs from repository and runtime evidence; no live assembly, storage, routing, prompt execution, CLI/TUI behavior, or runtime orchestration was added in AW-03.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: AW-02 — define durable autonomous task execution-state contracts.
- Files changed: `internal/autonomous/state.go`, `internal/autonomous/state_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Contracts and validation added: introduced the versioned `ExecutionState` snapshot with revision-linked durable plans and ordered steps, evidence-backed acceptance criteria, audit-finding resolution lifecycle, compact supervisor decision/run/artifact provenance, total and per-action attempt accounting, explicit unset/limited/unlimited retry/time/token budgets, autonomous lifecycle states, and minimal needs-input/terminal details. Validation reuses AW-01 evidence, finding-ID, action, and worker-profile rules; enforces unique stable IDs, nested task identity, disposition-specific evidence/rationale, deterministic plan constraints, nonnegative accounting, explicit budget semantics, and structurally coherent completed/terminal snapshots. Minimal transition validation preserves stable record identity and prevents terminal step, finding, and task-state reversals while leaving routing, evidence freshness, completion policy, and reopen behavior to later tasks.
- Tests added: table-driven coverage for every new enum value and unknown rejection; valid and invalid plan revisions/steps; acceptance dispositions; finding resolution dispositions; decision provenance and profile compatibility; zero, maximum, over-limit, negative, duplicate, and unknown accounting values; active, needs-input, and terminal aggregates; nested task/decision identity conflicts; allowed and forbidden minimal transitions; deterministic representative JSON round trips; and exact AW-01 JSON fixture compatibility.
- Verification run: `gofmt -w internal/autonomous/state.go internal/autonomous/state_test.go`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./...`; `git diff --check`; `go vet ./...`.
- Verification result: all commands passed.
- Remaining work: begin `AW-03` in a new fresh session; no dossier rendering, persistence, runtime routing, CLI/TUI behavior, or orchestration was added in AW-02.
- Blockers: none.

Planning expanded on 2026-07-10:

- Selected task: integrate the agreed autonomy, efficiency, accuracy, archival, recovery, operability, and evaluation additions into the autonomous workflow program.
- Files changed: `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Program added: the roadmap now orders `AW-01` through `AW-31` across foundation/context, supervisor routing, planning and acceptance evidence, independent audit and correction, tiered verification, bounded retries, conditional documentation/cleanup, needs-input handling, per-task worktrees, safety preflight, transactional finalization, tracked archives, task/queue terminal loops, dependencies, daemon continuation, artifact retention, context caching, CLI/TUI visibility, notifications, metrics/evaluations, and eventual bounded concurrency.
- Prompt readiness: every unchecked task has explicit scope, acceptance criteria, and verification guidance so a fresh-session prompt can be generated from one task without relying on chat history.
- Ordering rationale: correctness contracts and evidence precede runtime routing; correction and completion gates precede unattended loops; isolation/safety and archival precede queue/daemon operation; optimization, observability, and parallelism follow proven sequential behavior.
- Verification run: roadmap/task/state inspection; confirmed `AW-02` is the first unchecked task; `git diff --check`.
- Verification result: passed.
- Remaining work: execute one unchecked `AW-*` task per fresh session, beginning with `AW-02`.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: define and validate autonomous supervisor-decision and audit-finding contracts.
- Files changed: `internal/autonomous/contracts.go`, `internal/autonomous/contracts_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior added: introduced isolated, JSON-serializable `internal/autonomous` contracts for supervisor decisions, durable evidence references, audit reports, and audit findings. Go validation now recognizes all eight supervisor actions, enforces exact worker-profile compatibility for worker actions and no profile for terminal actions, requires rationale/evidence/success criteria where applicable, validates clean versus changes-required audit semantics, requires explicit blocking significance and actionable finding corrections, enforces stable lower-case kebab-case finding IDs, rejects duplicates, and verifies correction references against a changes-required audit report.
- Compatibility preserved: no `runonce`, `mixed-pass-v1`, task frontmatter, receipt, ledger, CLI, or TUI behavior changed; no dependencies were added.
- Tests added: table-driven coverage for every action and disposition, valid worker and terminal decisions, evidence and finding validation, malformed and duplicate finding IDs, correction references, and deterministic JSON round trips.
- Verification run: `gofmt -w internal/autonomous/contracts.go internal/autonomous/contracts_test.go`; `go test -count=1 ./internal/autonomous`; `go test -count=1 ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: continue with the task-dossier slice from step 2 of the autonomous workflow roadmap in a new fresh session; supervisor runtime and routing remain unwired.
- Blockers: none.

Planning completed on 2026-07-10:

- Selected task: capture the autonomous workflow refactor roadmap and create a fresh-session kickoff prompt.
- Files changed: `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md`, `.agent/AUTONOMOUS_REFACTOR_KICKOFF_PROMPT.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Planning added: a concise nine-step roadmap for replacing the fixed mixed-pass pipeline with a supervisor-directed, self-correcting `autonomous-v1` control loop, including required safety and completion invariants.
- Kickoff added: a detailed fresh-session prompt scoped to the first foundational slice—structured supervisor-decision and audit-finding contracts with validation and tests, without runtime routing changes.
- Verification run: Markdown inspection; `git diff --check`.
- Verification result: passed.
- Remaining work: execute the first unchecked contract task using the kickoff prompt, then continue the roadmap in bounded fresh sessions.
- Blockers: none.

Task completed on 2026-07-10:

- Selected task: reconcile ambiguous Git commit outcomes by comparing pre/post-commit HEAD.
- Files changed: `internal/commit/commit.go`, `internal/commit/commit_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the commit gate now captures HEAD before staging, resolves HEAD after every commit attempt, retries a failed post-commit lookup once, and treats an advanced HEAD as committed even if the commit command or first lookup reported failure. Initial commits remain supported through an explicit unborn-HEAD result. If post-commit HEAD remains unavailable, the commit result is `indeterminate`; runonce blocks and restages the policy-transitioned task state instead of restoring potentially stale original-phase bytes. Terminal events record pre/post HEAD evidence and whether the lookup was retried.
- Tests added: focused fake-runner coverage for transient lookup recovery, commit-command failure with an advanced HEAD, an unavailable post-commit HEAD, and an unborn initial HEAD; a real-Git regression injects a transient lookup failure after an actual commit; runonce coverage verifies indeterminate outcomes block the transitioned phase and record terminal evidence.
- Tests/verification run: `go test -count=1 ./internal/commit ./internal/runonce`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `go vet ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr task list`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: passed.
- Remaining work: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: document the mixed-pass task workflow.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Documentation added: README now distinguishes a canonical `.agent/tasks/*.md` durable task from each fresh pass/run, shows a minimal `mixed-pass-v1` task, documents `implement -> audit -> document -> simplify -> completed`, summarizes the four policy-selected profiles and their no-change rules, states that task frontmatter `profile` is not an override, explains Revolvr-owned pre-commit phase advancement and same-phase failure retry, and points operators to the CLI and TUI workflow inspection surfaces. It also clarifies that receipts and ledger events audit policy-driven harness outcomes rather than choosing the next phase.
- Tests/verification run: `go test ./...`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr task list`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: passed.
- Remaining work: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface task workflow state in CLI, TUI, status, and timeline views.
- Files changed: `internal/taskmodel/task.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/app/timeline.go`, `internal/app/timeline_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: app task adaptation now resolves each file-backed task through `internal/passpolicy.Lookup` and exposes workflow, current phase, mapped run profile, and next phase or `completed` on `taskmodel.Task`; lookup failures return a task-specific error. `revolvr task list` now prints stable workflow/phase/profile/next columns, and `revolvr status` prints a concise next-task and next-pass summary when a pending task exists while preserving the empty-task output. The TUI Dashboard and Tasks views now show the next task's workflow state, every task row shows phase/profile/next, and selected task detail shows workflow/phase/profile/next. `task_selected` timeline rows append available workflow/phase/profile payload metadata while legacy payload formatting and raw TUI event rows remain intact.
- Tests/verification run: `gofmt -w internal/taskmodel/task.go internal/app/app.go internal/app/app_test.go internal/app/timeline.go internal/app/timeline_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/app ./internal/cli ./internal/tui`; `go test ./...`; `go run ./cmd/revolvr task list`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to document the mixed-pass task workflow.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: seed `.agent/profiles/simplifier.md`.
- Files changed: `.agent/profiles/simplifier.md`, `internal/prompt/profile.go`, `internal/prompt/profile_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `prompt.DefaultRunProfileTemplates` now includes the repo-authored `simplifier` profile. `revolvr init` seeds `.agent/profiles/simplifier.md` alongside `implementer.md`, `auditor.md`, and `documentor.md` when missing, while preserving any existing simplifier profile file. The checked-in profile instructs agents to simplify only when meaningful, preserve behavior, avoid clever abstractions, create helpers only for real duplication or complexity reduction, and stop cleanly when no worthwhile simplification exists.
- Tests/verification run: `gofmt -w internal/prompt/profile.go internal/prompt/profile_test.go internal/cli/root_test.go`; `go test ./internal/prompt ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to surface task workflow state in CLI, TUI, status, and timeline views.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: allow policy-permitted no-change success and durable phase advancement.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: successful runonce passes now apply the selected task's `internal/passpolicy.Policy` after Codex and verification succeed and before the commit gate. Implement passes still need pre-metadata changed files and now advance the durable task to `phase: audit` with `status: pending`; audit advances to `phase: document`, document advances to `phase: simplify`, and simplify marks the task `completed`. Audit/document/simplify can succeed with no source changes because the task-file metadata update becomes the committed durable change. Changed files are recaptured after metadata updates so commits, receipts, and terminal ledger events include the task-file transition. Failed/blocking outcomes do not advance phase; if a commit fails after a metadata update, the task is marked blocked at its original phase.
- Tests/verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go`; `go test ./internal/runonce ./internal/taskfile`; `go test ./internal/app`; `go test ./internal/passpolicy`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to seed `.agent/profiles/simplifier.md`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: teach `runonce` to load the profile for the selected task phase.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/runonce` now resolves the selected task file's `workflow` and `phase` with `internal/passpolicy.Lookup` and loads the mapped repo-authored profile before building the context bundle. Default and explicit implement phases load `implementer`, audit loads `auditor`, document loads `documentor`, and an unseeded simplify phase blocks before Codex with the existing missing-profile failure path. The task-selected ledger event now records workflow, phase, and profile name for audit; context manifests continue recording the exact loaded profile path, SHA-256, and byte size. Successful runs still mark the selected task file `completed`; no phase advancement or no-change success policy was added.
- Tests/verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/runonce ./internal/prompt`; `go test ./internal/passpolicy ./internal/taskfile`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to allow policy-permitted no-change success and durable phase advancement.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a pass-policy model mapping phases to profiles and outcome semantics.
- Files changed: `internal/passpolicy/policy.go`, `internal/passpolicy/policy_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/passpolicy.Lookup` for `mixed-pass-v1`, mapping `implement -> audit -> document -> simplify -> completed` with profile names `implementer`, `auditor`, `documentor`, and `simplifier`; `implement` disallows no-change success, later phases allow it, and `simplify` is terminal with no next phase. Unsupported workflows and phases now return clear lookup errors from the policy package. No runtime behavior changed in `runonce`.
- Tests/verification run: `gofmt -w internal/passpolicy/policy.go internal/passpolicy/policy_test.go`; `go test ./internal/passpolicy`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to teach `runonce` to load the profile for the selected task phase.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add task workflow/phase metadata parsing and defaults.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `.agent/tasks/*.md` parsing now exposes `workflow` and `phase` on `taskfile.Task`, defaults missing workflow to `mixed-pass-v1`, defaults missing phase to `implement`, accepts `implement`, `audit`, `document`, and `simplify`, validates invalid workflow/phase values clearly, and treats duplicate `workflow`/`phase` as duplicate known frontmatter keys. Status-only task updates continue preserving workflow/phase frontmatter.
- Tests/verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go`; `go test ./internal/taskfile`; `go test ./...`; `git diff --check`.
- Verification result: passed.
- Remaining work: next unchecked backlog item is to add a pass-policy model mapping phases to profiles and outcome semantics.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: seed the mixed loop pass progression backlog for durable task phases.
- Files changed: `.agent/TASKS.md`, `.agent/DECISIONS.md`, `.agent/STATE.md`.
- Backlog added: small ordered slices for task workflow/phase metadata, pass-policy modeling, phase-based profile loading, no-change success with durable phase advancement, `simplifier` profile seeding, workflow state visibility, and mixed-pass documentation.
- Decision update: durable tasks and passes are distinct; workflow state lives in canonical task files; Revolvr owns phase transitions through policy; audit/document/simplify may succeed without code changes when policy permits; successful advancement should update the task file before the commit gate when appropriate.
- Verification run: `git diff --check`.
- Verification result: passed.
- What remains: next unchecked backlog item is to add task workflow/phase metadata parsing and defaults.
- Blockers: none.

Audit completed on 2026-07-09:

- Selected task: audit obsolete migration surface after moving to file-backed tasks, repo-authored profiles, current context artifacts, and fresh Codex sessions.
- Files changed: `README.md`, `CODEX_AGENT_LOOP_HANDOFF.md`, `CODEX_HARNESS_TARGETS.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`.
- Obsolete surface removed: current operator/developer docs, TUI empty-state text, and CLI test helper names no longer describe tasks as a queue; archived setup/design docs now point readers to the current `.agent/tasks/*.md`, `.agent/profiles/*.md`, `context.md`/`context.json`, and fresh `codex exec` architecture.
- Audit result: no tracked runtime code remains for `internal/taskqueue`, `.revolvr/tasks.sqlite`, `prompt_path`, `prompt.md`, `prompt_built`, or historical context artifact compatibility. Valid SQLite usage is limited to the run ledger at `.revolvr/ledger.sqlite`; ledger artifact decoding only accepts `context_payload_path` and `context_manifest_path` for run context artifacts; app, CLI, TUI, and runonce task paths use `internal/taskfile` plus `internal/taskmodel`.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; stale-term `rg` sweeps; `go list ./...`; `bash -n scripts/smoke-local.sh scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `git diff --check`; `go test ./...`; temp-worktree `revolvr init` smoke confirming no `.revolvr/tasks.sqlite`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `./scripts/smoke-local.sh`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: remove obsolete SQLite task queue and historical prompt artifact compatibility before publishing.
- Files changed: `AGENTS.md`, `CODEX_AGENT_LOOP_HANDOFF.md`, `CODEX_HARNESS_TARGETS.md`, `README.md`, `.agent/DECISIONS.md`, `.agent/STATE.md`, `internal/taskmodel/task.go`, `internal/taskqueue/store.go`, `internal/taskqueue/store_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/state.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/ledger/events.go`, `internal/ledger/artifacts.go`, `internal/ledger/artifacts_test.go`, `internal/app/timeline.go`, `scripts/smoke-local.sh`, `scripts/dogfood-live.sh`.
- Behavior changed: initialization no longer creates or requires `.revolvr/tasks.sqlite`; the old SQLite task queue package has been removed; task display/status data now uses `internal/taskmodel` while canonical task storage remains `.agent/tasks/*.md`.
- Compatibility removed: ledger artifact decoding no longer accepts `prompt_path`, `EventPromptBuilt` has been removed, and timeline/context artifact rendering now only uses `context_built` plus `context_payload_path` / `context_manifest_path`.
- Verification run: `gofmt -w internal/taskmodel/task.go internal/app/app.go internal/app/app_test.go internal/cli/state.go internal/cli/root.go internal/cli/root_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go internal/tui/model.go internal/tui/model_test.go internal/ledger/events.go internal/ledger/artifacts.go internal/ledger/artifacts_test.go internal/app/timeline.go`; `go test ./internal/taskmodel ./internal/ledger ./internal/app`; `go test ./internal/cli ./internal/runonce ./internal/tui`; `go list ./...`; `go test ./...`; `bash -n scripts/smoke-local.sh scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `git diff --check`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: update operator-facing README TUI documentation for bounded multi-pass loops.
- Files changed: `README.md`, `.agent/STATE.md`.
- Documentation changed: README now documents `n` loop max-pass cycling, `L` bounded loop start, cancellation for an active run or loop, and progress-pane pass summaries. The chat-to-task workflow still recommends a single `R` pass for one bounded pass and now also mentions the optional TUI loop; CLI `run --max-passes` remains documented as the non-TUI equivalent.
- Verification run: `go test ./...`; `go run ./cmd/revolvr tui --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
- Files changed: `internal/app/run.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: the TUI now keeps `R` as single-pass run-once and adds a bounded loop action on `L` backed by the app-wired `internal/app.RunLoop`. The loop pass count defaults to 3 and cycles through 2/3/5 with `n`; active loops reuse the existing preflight blocker, cancellation key `c`, progress pane, status refresh, and latest-run detail opening.
- Progress behavior changed: the existing `Run Progress` pane now distinguishes loop mode, shows max passes, pass counts, completed/failed-or-blocked/no-task/consecutive-failure stats, stop reason, latest run ID, Codex progress, and per-pass summaries streamed through `RunLoopInput.OnPass`. Cancellation cancels the loop context, refreshes status, and renders `cancelled` with `context_cancelled` when applicable.
- CLI wiring changed: `revolvr tui` now passes a `RunLoop` callback into `internal/tui` that calls `app.RunLoop` with the same configured runner used by CLI/TUI run once, plus progress and pass callbacks. `app.RunLoop` now reports `context_cancelled` when the runner returns because the loop context was cancelled inside a pass.
- Tests added/updated: focused TUI model coverage for loop max-pass completion, no-task stop, repeated failure guardrail, blocked stop, cancellation, and pass-count cycling; CLI TUI wiring coverage now verifies the `RunLoop` callback; app coverage now verifies runner-context cancellation stop reason.
- Verification run: `gofmt -w internal/app/run.go internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: render the run timeline in CLI `show` and TUI Run Detail.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: `revolvr show <run-id>` now prints a `Timeline:` section immediately after run summary fields and before artifacts, diagnostics, and raw events. The section uses `internal/app.RunTimeline` with deterministic tabular columns for timestamp, phase, status, and detail, uses CLI time formatting for real timestamps, and renders missing row timestamps as `none`.
- TUI behavior changed: Run Detail now renders a `Timeline` section after `Summary` and before `Diagnostics` using the same app projection and compact single-line rows. Raw `Events` remain visible below artifacts, and long timeline/event detail remains scrollable through the existing viewport.
- Tests added/updated: CLI `show` exact-output coverage now includes timeline rows for persisted run start/completion, artifact, diagnostic, warning, and sparse histories, plus an empty timeline helper check. TUI coverage now proves Run Detail includes timeline rows while keeping raw event rows, renders an empty timeline clearly, and scrolls through a long timeline while still reaching raw events.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add an app-level run timeline projection from ledger events.
- Files changed: `internal/app/timeline.go`, `internal/app/timeline_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`, `.agent/TASKS.md`.
- Behavior changed: added `internal/app.RunTimeline`, which projects a `ledger.RunWithEvents` history into reusable app-level `RunTimelineRow` values with timestamp, phase, status, and concise detail. The projection preserves ledger event order, uses event `CreatedAt` timestamps, covers run start, task selection, context, Codex start/progress/completion, changed files, receipts, verification, commit, and terminal outcome rows, and keeps the slice app-only for later CLI/TUI rendering.
- Fallback behavior added: when start or terminal events are missing, the projection uses the ledger run row for started/completed timestamps, run status, summary, verification status, Codex exit code, and commit SHA. Malformed, missing, or partial event payloads return generic deterministic rows instead of panicking.
- Tests added: focused `internal/app` coverage for completed, verification-failed, Codex-failed, blocked/pre-run failure, sparse missing-event fallback, and malformed payload histories with exact expected row slices.
- Verification run: `gofmt -w internal/app/timeline.go internal/app/timeline_test.go`; `go test ./internal/app`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to render the run timeline in CLI `show` and TUI Run Detail.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: migrate app-level task write operations to canonical `.agent/tasks/*.md`.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model_test.go`, `README.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/app.AddTask`, write-mode `ImportTasks`, `RetryTask`, and `UnblockTask` now operate on canonical Markdown task files under `.agent/tasks/` instead of mutating SQLite task rows. New task files are written with `id` plus `status: pending` frontmatter and an H1 title derived from the provided summary or first task line. Retry/unblock find file-backed tasks by ID and only transition `blocked` tasks back to `pending`, preserving clear missing-task and non-blocked-task errors.
- Taskfile helpers added: `internal/taskfile.Create` writes canonical pending task files with generated IDs, `FindByID` resolves frontmatter or filename-derived task IDs and reports duplicates, and `UpdateBlockedToPending` reuses status-frontmatter updates for blocked-to-pending transitions.
- CLI/TUI behavior preserved: `task add` keeps the concise confirmation output based on the original task text and summary, while `task add` followed by `task list`, write-mode `task import`, TUI add refresh, and retry/unblock refresh paths now expose file-backed task changes through the existing `taskqueue.Task` adapter shape. `task import --dry-run` remains non-mutating, and validation/parse failures do not create `.agent/tasks/` or `.revolvr/`.
- Documentation changed: README now describes `.agent/tasks/*.md` as the shared task state and `.revolvr/` as local runtime state with transitional task database infrastructure.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/app/app.go internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model_test.go`; `go test ./internal/taskfile`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface canonical `.agent/tasks/*.md` tasks through app status, CLI task list/status, and existing TUI task views.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/app.ListTasks` and `internal/app.Status(...).Tasks` now load direct Markdown task files through `internal/taskfile` instead of reading SQLite queue rows. File tasks are adapted into the existing `taskqueue.Task` shape with task-file ID, full Markdown body as `Task`, H1 title as `Summary`, and task-file status preserved, including `running`.
- Status behavior preserved: `app.Status` still uses the existing initialized-state check and still loads recent runs plus latest run events from the ledger exactly as before; only the task slice source changed.
- CLI/TUI changed: `revolvr status`, `revolvr task list`, and `revolvr tui` task/dashboard views now surface file-backed tasks through the existing render paths. `task add`, `task import`, `task retry`, and `task unblock` remain transitional SQLite-backed write operations in this slice, so tests that cover those writes inspect the legacy queue directly instead of expecting app status refreshes to expose SQLite-only rows.
- Tests updated: app coverage now proves file-backed status/list conversion, deterministic taskfile order, full Markdown task bodies, H1 summaries, status preservation, and zero timestamp/blocker metadata. CLI coverage now counts file-backed tasks in status, lists file-backed tasks in filename order, and verifies TUI Dashboard/Tasks rendering from app status for pending, blocked, completed, and next-runnable file tasks while keeping write callback tests scoped to SQLite writes.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root_test.go`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: fix file-backed runonce integration so fake-Codex smoke scripts pass and successful task status updates do not leave the worktree dirty.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `scripts/dogfood-live.sh`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: successful runonce passes now keep the selected task manifest source metadata based on exact pre-run file bytes, wait until Codex and verification have passed, update the selected Markdown task to `status: completed`, recapture changed files, and pass that recaptured list to the commit runner so the task status flip is committed with the successful run. The pre-status changed-file capture still gates `no_changes`, so a task-status-only mutation does not turn an empty Codex pass into a success. Failed and blocking outcomes continue to mark selected task files `blocked` without committing.
- Script changes: the fake-Codex success and verification-failure smokes now create committed canonical `.agent/tasks/*.md` task files instead of using `revolvr task add`; they assert run artifacts and receipts, selected-task manifest source metadata from the pending file bytes, `completed` on success, and `blocked` on verification failure without relying on SQLite task counts. `scripts/dogfood-live.sh` now writes and commits a canonical live dogfood task file before the run and expects the successful run commit to include both the requested dogfood file and the task status update.
- Tests added/updated: runonce coverage now proves the successful commit changed-file capture includes the completed selected task file, a real-Git second pending file task can run after a successful first task without leftover dirty status, and smoke-shaped task files produce `selected_task` manifest metadata from exact pre-run bytes.
- Verification run: `gofmt -w internal/runonce/runonce.go internal/runonce/runonce_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/taskfile`; `go test ./internal/runonce`; `go test ./internal/prompt`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: wire runonce to select canonical file-backed Markdown tasks from `.agent/tasks/*.md`.
- Files changed: `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/runonce` now selects the next pending Markdown task via `internal/taskfile.SelectNext`, ordered by priority then filename, and returns `OutcomeNoTask` when no pending task files exist without falling back to SQLite queue rows.
- Context/ledger changed: selected run fields use `taskfile.Task.ID`, `ContextBody`, and `Title`; prompt input now passes `prompt.SourceContent{Path: task.SourcePath, Content: task.SourceBytes}` so `context.json` records selected-task path, SHA-256, and byte size from the exact pre-run file bytes. Run profile selection remains the default `implementer` profile only.
- Task status changed: `internal/taskfile.UpdateStatus` can replace, insert, or add deterministic `status` frontmatter while preserving other frontmatter/body content as much as practical. Committed runs mark the selected Markdown task `completed`; blocking/failed terminal outcomes mark it `blocked`; no-task and failures before task selection do not mutate task files.
- Transitional note: the old SQLite task queue package, app/CLI task commands, and compatibility result fields remain in place, but runonce no longer consumes SQLite pending tasks.
- Tests added/updated: taskfile status replacement/insertion/frontmatter-add coverage; runonce coverage for file priority/filename selection, exact selected-task manifest metadata from pre-run bytes, committed-to-completed mutation, failed/blocking-to-blocked mutation, second run after completion returning no-task, pre-selection lock failure leaving files pending, and SQLite pending tasks being ignored when no task file exists.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/taskfile`; `go test ./internal/runonce`; `go test ./internal/prompt`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: introduce file-backed Markdown task specs under `.agent/tasks/` and selected-task source metadata in the context manifest.
- Files changed: `internal/taskfile/taskfile.go`, `internal/taskfile/taskfile_test.go`, `internal/cli/state.go`, `internal/cli/root_test.go`, `internal/prompt/prompt.go`, `internal/prompt/context.go`, `internal/prompt/prompt_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/taskfile` for dependency-free loading of direct `.agent/tasks/*.md` files with optional YAML-ish `id`, `profile`, `status`, and `priority` frontmatter, required non-empty H1 titles, status/profile/path validation, exact source bytes, SHA-256 and byte-size helpers, all-file listing, runnable pending ordering by numeric priority then filename, and select-next behavior.
- Init changed: `revolvr init` now creates `.agent/tasks/` when missing, leaves it empty by default, and preserves existing task Markdown files across repeated init runs.
- Context manifest changed: `prompt.Input` can now carry a selected task source path plus exact source bytes, and `context.json` records `selected_task` path, SHA-256, and byte size from those exact bytes when provided. The existing task-text fallback remains for the transitional queue-backed path.
- Transitional note: this pass did not remove or rewrite the existing SQLite queue-backed runtime selection path; the new file-backed loader is the canonical task-file foundation for the next runtime wiring step.
- Tests added/updated: focused taskfile loader/parser tests for valid files, missing H1, invalid status, unsafe profile, path containment, direct-file loading, deterministic runnable ordering, select-next behavior, and exact source hash/byte size; CLI init coverage for task directory creation, empty default contents, and idempotent non-overwrite; prompt manifest coverage for selected task file source metadata.
- Verification run: `gofmt -w internal/taskfile/taskfile.go internal/taskfile/taskfile_test.go internal/prompt/context.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/cli/state.go internal/cli/root_test.go`; `go test ./internal/taskfile`; `go test ./internal/prompt`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: make run profiles file-backed and repo-authored.
- Files changed: `.agent/profiles/implementer.md`, `.agent/profiles/auditor.md`, `.agent/profiles/documentor.md`, `internal/prompt/profile.go`, `internal/prompt/profile_test.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/prompt/context.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/cli/state.go`, `internal/cli/root_test.go`, `internal/cli/doctor_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: run profiles are now loaded from `.agent/profiles/<name>.md` relative to the repository root. The default runtime profile name remains `implementer`, but missing, empty, or unsafe profile names now fail clearly instead of falling back to embedded text.
- Init changed: `revolvr init` seeds `implementer.md`, `auditor.md`, and `documentor.md` under `.agent/profiles/` without overwriting existing files; `.revolvr/` state and local Git exclude behavior remain intact.
- Context bundle changed: `context.md` renders the loaded profile markdown as the `## Run Profile` body, and `context.json` records the run profile source path plus SHA-256 and byte size for the profile content used in the run.
- Tests added/updated: focused profile-loader coverage for success, default-name loading, missing files, empty files, and unsafe names; runonce coverage for file-backed profile content and manifest metadata plus missing-profile blocking before Codex; CLI init coverage for profile seeding and non-overwrite behavior; doctor/smoke tests updated for repo-authored profile files.
- Verification run: `gofmt -w internal/cli/doctor_test.go internal/cli/root_test.go internal/cli/state.go internal/prompt/context.go internal/prompt/profile.go internal/prompt/profile_test.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: finish context-bundle migration cleanup in smoke scripts and operator-facing docs.
- Files changed: `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `scripts/dogfood-live.sh`, `README.md`, `internal/ledger/ledger_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: fake-Codex smoke checks and the live dogfood script now assert `.revolvr/runs/<run-id>/context.md` and `.revolvr/runs/<run-id>/context.json` instead of obsolete `prompt.md`, validate the main context Markdown sections, validate manifest keys for payload path and SHA-256, and check CLI `show` output for both context artifact paths.
- Documentation changed: README now describes run passes as writing `context.md` plus `context.json` and sending that context payload to Codex.
- Verification run: `gofmt -w internal/ledger/ledger_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh scripts/smoke-run-once-fake-codex-verification-failure.sh scripts/dogfood-live.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/ledger`; `go test ./internal/cli`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: introduce an auditable run context bundle as the canonical prompt architecture.
- Files changed: `internal/prompt/context.go`, `internal/prompt/profile.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/ledger/events.go`, `internal/ledger/artifacts.go`, `internal/ledger/artifacts_test.go`, `internal/receipt/validation.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/app/app_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: each run now writes an exact Markdown context payload at `.revolvr/runs/<run-id>/context.md` and an auditable JSON manifest at `.revolvr/runs/<run-id>/context.json` before Codex starts. The manifest records run/task/profile identity, payload path, payload SHA-256 and byte size, generation time, and source entries for the selected task and run profile. Ledger run artifacts now expose `context_payload_path` and `context_manifest_path`, while old `prompt_path` events still decode as legacy context payloads.
- Runtime flow preserved: `codex exec` still receives the full uncompressed Markdown context payload in a fresh session; no resume flow or context compression was added.
- Tests added/updated: prompt context-manifest hash/byte-size and JSON tests, runonce context-bundle artifact coverage proving the manifest references the exact bytes sent to Codex, ledger artifact context-path coverage plus legacy `prompt_path` fallback, and updated CLI/TUI/app artifact fixtures and rendering expectations.
- Verification run: `gofmt -w internal/app/app_test.go internal/cli/root.go internal/cli/root_test.go internal/ledger/artifacts.go internal/ledger/artifacts_test.go internal/ledger/events.go internal/prompt/context.go internal/prompt/profile.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/receipt/validation.go internal/runonce/runonce.go internal/runonce/runonce_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./internal/ledger`; `go test ./internal/cli`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: begin the new prompt architecture with the smallest safe implementation step by adding run profiles and richer prompt assembly.
- Files changed: `internal/prompt/profile.go`, `internal/prompt/prompt.go`, `internal/prompt/prompt_test.go`, `internal/runonce/runonce_test.go`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/prompt` now has a `RunProfile` type and built-in default profile named `implementer`. Prompt artifacts now render predictable sections in order: `# Revolvr Codex Pass`, `## Run Profile`, `## Selected Task`, `## Repository Rules`, `## Artifact Paths`, `## Required Receipt Schema`, and `## Stop Condition`.
- Runtime flow preserved: `runonce` still selects the next task from the current queue, writes `.revolvr/runs/<run-id>/prompt.md`, and invokes fresh `codex exec` through the existing path without resume or task-storage migration.
- Tests added/updated: prompt builder snapshot/order coverage for the new profile section, default profile coverage when no explicit profile is provided, default rules/artifacts/receipt/stop-condition assertions, and a runonce test that reads the written prompt artifact and checks the default profile section.
- Verification run: `gofmt -w internal/prompt/profile.go internal/prompt/prompt.go internal/prompt/prompt_test.go internal/runonce/runonce_test.go`; `go test ./internal/prompt`; `go test ./internal/runonce`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is still to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add TUI blocked-task retry for the selected task.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Tasks view now supports `u` retry for the selected blocked task. The action calls the app-wired retry callback, refreshes the status snapshot after success, keeps the retried task selected, and reports inline notices for successful retry, non-blocked selections, missing callbacks, retry callback errors, and refresh failures. The Tasks footer shows `u Retry` only when the selected task is blocked and retry/refresh callbacks are available, while Help documents the retry key.
- CLI wiring changed: `revolvr tui` now passes `internal/app.RetryTask` into the TUI action callbacks.
- Tests added: focused TUI model coverage for successful blocked-task retry, pending/completed non-blocked rejection without mutation, missing retry/refresh callbacks, retry callback error, and refresh failure after retry. CLI TUI wiring coverage now verifies the retry callback can return a blocked task to pending through the command setup.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/tui ./internal/cli ./internal/app`; `go test ./...`; `go run ./cmd/revolvr tui --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add an app-level run timeline projection from ledger events.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the Dashboard and Tasks view now show pending, blocked, and completed counts alongside explicit runnable state text. When a pending task exists, both views show `Runnable: ready to run` and the next task ID plus summary/task text; otherwise they show `Runnable: nothing runnable` and `Next task: none`. The Tasks list marks the current selection with `>` and the first pending task with a plain `next` marker, so selection and run readiness are visible without depending on color.
- Tests added: focused `internal/tui` render coverage for pending, blocked-only, completed-only, and empty queues, plus updated dashboard/task snapshots for uninitialized and narrow/wide rendering.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add TUI blocked-task retry for the selected task.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: document the chat/spec-to-task workflow and import format.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Documentation added: README now documents a chat-to-task import workflow for shaping specs in web chat, saving a Markdown task file, dry-running and importing it, opening or refreshing the TUI, running TUI preflight, and starting one bounded TUI pass. It includes a minimal import-file example with summary, acceptance, and verification notes, plus the caution that chat, CLI, and TUI can share task state while concurrent code edits against the same repository should be avoided.
- Verification run: `go test ./...`; `go run ./cmd/revolvr task import --help`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add `revolvr task import <path>` with `--dry-run`.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added a `revolvr task import <path>` command with `--dry-run`. The CLI reads the Markdown file, calls the app-level import operation, prints numbered dry-run rows without creating `.revolvr/`, and prints created task IDs in parsed order for write imports. Existing `task add` and `task list` output remains unchanged.
- Tests added: focused CLI tests for import help, dry-run/no-mutation output, successful ordered import and ID output, parse errors without mutation, and unreadable paths without mutation.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestTaskImport|TestTask(Add|List)'`; `go test ./internal/cli -run 'TestNewRootCommandConstructsExpectedCommands|TestParentCommandHelpOutput'`; `go test ./...`; `go run ./cmd/revolvr task import --help`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to document the chat/spec-to-task workflow and import format.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add an app-level task import and dry-run operation.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added app-level Markdown task import parsing and import execution. `internal/app.ParseTaskImport` exposes normalized parsed tasks, `ImportTasksFromMarkdown` parses and imports in one call, and `ImportTasks` supports dry-run and write modes over parsed tasks. Dry-run returns the tasks that would be created without opening or mutating `.revolvr/`; write mode validates every task before opening the store, creates tasks in input order, and returns created IDs.
- Tests added: focused app tests for dry-run reporting without state mutation, ordered write/import ID returns, validation failure without partial writes, parse failure without partial writes, and empty parsed imports without state creation.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go`; `go test ./internal/app`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add `revolvr task import <path>` with `--dry-run`.
- Blockers: none.

Task completed on 2026-07-09:

- Selected task: add a Markdown spec-to-task parser that preserves human-readable acceptance and verification notes.
- Files changed: `internal/taskimport/parser.go`, `internal/taskimport/parser_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `internal/taskimport`, a dependency-free Markdown parser for repeated `## Task` sections. It returns ordered task specs with `Task` and `Summary`, preserves acceptance and verification notes in task text, keeps unknown subsections in task text, supports explicit task body subsections, and reports parse errors with line context.
- Tests added: focused parser tests for ordered repeated tasks, multiline task body readability, explicit task body sections, preserved acceptance/verification/unknown notes, empty task text, malformed pre-task sections, duplicate known sections, and empty input.
- Verification run: `gofmt -w internal/taskimport/parser.go internal/taskimport/parser_test.go`; `go test ./internal/taskimport`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add an app-level task import and dry-run operation.
- Blockers: none.

Planning update on 2026-07-09:

- Selected task: clean the completed TUI backlog out of the active task list and seed next-phase development tasks from the current design discussion.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: none; durable planning state only.
- Task list changed: `.agent/TASKS.md` now starts with a Markdown spec-to-task import workflow, then TUI next-task visibility, blocked-task retry, human-readable run timelines, and a bounded multi-pass TUI flow.
- Runtime queue changed: preserved the existing `.revolvr/` completed run history and added 9 new pending tasks in the same order as `.agent/TASKS.md`.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: first unchecked backlog item is to add a Markdown spec-to-task parser.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: polish TUI layout, styling, and documentation for daily use.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: TUI rendering now wraps plain content, header lines, and key help against the active terminal width before applying semantic Lip Gloss styles. Narrow layouts use a compact recent-run list, long values wrap instead of spilling, empty states distinguish unavailable runtime state from absent tasks/runs, and important states remain visible through words like `Status: failed`, `PASS`, `FAIL`, `OK`, and `! blocked`.
- Documentation added: README now includes a `revolvr tui` section covering views, key actions, live progress, receipt validation, preflight checks, and current limitations that still require the CLI.
- Tests added: snapshot-style wide and narrow TUI render coverage with max-width assertions for narrow output.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go run ./cmd/revolvr tui --help`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a nonblocking TUI run-once action with live progress and cancellation.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI now exposes an uppercase `R` run-once action guarded by the latest ready preflight result, tracks one active run at a time, streams Codex progress into a persistent Run Progress pane, hides or blocks conflicting actions while running, and supports `c` cancellation through the run context. Run completion refreshes the status snapshot, selects and loads the completed run detail when a run ID exists, and reports terminal states for success, failure, no-task, and cancellation.
- CLI wiring changed: `revolvr tui` now passes a `RunOnce` callback that invokes `internal/app.RunOnce` with the same fakeable runner hook used by `revolvr run --once`.
- Tests added: focused TUI model coverage for preflight and active-run guards, progress streaming, successful completion refresh, failed outcomes, and cancellation. CLI TUI wiring coverage now verifies the run callback and progress callback are available.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to polish TUI layout, styling, and documentation for daily use.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move doctor/preflight orchestration behind `internal/app` and add a TUI Preflight view.
- Files changed: `internal/app/preflight.go`, `internal/app/app_test.go`, `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: existing doctor checks now run through `internal/app.Preflight`, which returns structured readiness checks and preserves the same check order, status labels, and detail strings. `revolvr doctor` remains a thin CLI renderer over that app result. The TUI now has a `5 Preflight` view with a `p` rerun action, displays ready/failed/error preflight states, and shows each check detail inline.
- Tests added: app-level preflight snapshot tests for ready and failed checks, a deterministic byte-for-byte CLI doctor output test, CLI TUI wiring coverage for the preflight callback, and TUI model tests for ready and failed preflight views.
- Verification run: `gofmt -w internal/app/preflight.go internal/app/app_test.go internal/cli/doctor.go internal/cli/doctor_test.go internal/cli/root.go internal/cli/root_test.go internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/app -run 'TestPreflight'`; `go test ./internal/cli -run 'TestDoctor|TestTUIRunnerReceivesRefreshOpenAndAddActions'`; `go test ./internal/tui`; `go test ./internal/app`; `go test ./internal/cli -run 'TestDoctor|TestTUI'`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a nonblocking TUI run-once action with live progress and cancellation.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add receipt validation status to the TUI Run Detail view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Run Detail view now exposes a `v` validation action backed by a `ValidateReceipt` callback wired from `revolvr tui` to `internal/app.ValidateReceipt`. Run Detail renders a dedicated Receipt Validation section showing not-run, passed, failed, and error states, and each returned validation check is shown with explicit `PASS` or `FAIL` messaging. Opening or reloading a run resets stale validation state for that run.
- Tests added: focused TUI model coverage for fully valid receipts, failed validation checks, missing receipt errors, and validation callback errors. CLI TUI wiring coverage now verifies the ValidateReceipt callback is present and works against a ledger-backed valid receipt.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `git diff --check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched because it waits for terminal input.
- What remains: next unchecked backlog item is to move doctor/preflight orchestration behind `internal/app` and add a TUI Preflight view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a dedicated TUI Runs view and richer Run Detail view.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Runs view now lists recent runs with status, verification status, commit SHA, and summary. Opening a selected run still uses the app-backed `ShowRun` callback and now renders Run Detail as separate Summary, Diagnostics, Changed Files, Artifacts, and Events sections. Run Detail supports explicit `home`/`end` jumps alongside viewport scrolling, artifact sections show missing paths, changed-file capture gaps are visible, and receipt warnings are surfaced in diagnostics.
- Tests added: focused TUI model coverage for recent-run navigation, opening the selected run detail, diagnostics and receipt warning rendering, artifact and missing-artifact rendering, and scrolling long event output.
- CLI test update: the existing TUI CLI snapshot assertion now expects the richer recent-run row format.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all final commands passed. An initial `go test ./...` run failed only because `internal/cli/root_test.go` still expected the old TUI run row text; the expectation was updated and the full suite then passed.
- What remains: next unchecked backlog item is to add receipt validation status to the TUI Run Detail view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a TUI task creation flow backed by `internal/app.AddTask`.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI now supports an `a` action that opens Add Task mode with task text and summary fields. Empty task text is rejected inline without calling app callbacks, `esc` cancels back to the previous view without writing, and successful submit calls the app-backed add callback, refreshes status, switches to Tasks, and selects the added task when it is present in the refreshed snapshot.
- CLI wiring changed: `revolvr tui` now passes `internal/app.AddTask` through `internal/tui.RunOptions` alongside the existing status refresh and run open callbacks.
- Tests added: focused TUI model coverage for empty validation, cancel without writes, submit with add/refresh callback order, trimmed task and summary input, and selecting the new pending task. CLI TUI wiring coverage now verifies refresh, open, and add callbacks through the command setup.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a dedicated TUI Runs view and richer Run Detail view.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a dedicated TUI Tasks view with selection and task detail rendering.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: the TUI Tasks view now keeps an independent selected task, supports `j/k` and arrow-key movement, renders pending, blocked, and completed tasks in a scannable list, and shows an inline detail section for the selected task with ID, status, summary, task text, blocker, and present timestamps. Blocked tasks use a visible `! blocked` list marker without relying on color.
- Tests added: focused TUI coverage for populated task lists, empty task state, pending task details, blocked task details, and completed task details.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go test ./internal/tui`; `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a TUI task creation flow backed by `internal/app.AddTask`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a multi-view TUI shell with explicit Dashboard, Tasks, Runs, Run Detail, and Help/keys areas.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/tui.StatusModel` now has an explicit read-only view model for Dashboard, Tasks, Runs, Run Detail, and Help. Number keys switch views, Runs keeps selection/open actions, Run Detail preserves loaded details while switching away and back, and the shell renders header/footer key help with resize-aware content sizing.
- Tests added: focused TUI coverage for explicit shell rendering, refresh with view switching, loaded run detail preservation, Help/footer rendering, and narrow resize behavior. CLI TUI tests now size the model before snapshot assertions.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run TestTUI`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched because it waits for terminal input.
- What remains: next unchecked backlog item is to add a dedicated TUI Tasks view with selection and task detail rendering.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: clean the completed backlog out of the active task list and seed a detailed next-phase TUI backlog.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Task list changed: `.agent/TASKS.md` now keeps completed history as a short pointer to `.agent/STATE.md` and Git history, while the active backlog focuses on TUI workflow work: multi-view shell, Tasks view, task creation, Runs/detail views, receipt validation in details, preflight view, nonblocking run-once with cancellation, and layout/documentation polish.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: commit the planning-state update, then seed the runtime queue before using `revolvr run --max-passes`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add basic TUI actions for refresh, opening selected run details, and quit, without starting real Codex runs yet.
- Files changed: `internal/tui/model.go`, `internal/tui/model_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `internal/tui.StatusModel` now supports refresh, recent-run selection, opening selected run details, returning from details, and quit actions. `revolvr tui` passes read-only callbacks backed by `internal/app.Status` and `internal/app.ShowRun`; it still does not start Codex or invoke run orchestration.
- Tests added: focused TUI coverage for refresh reloading a status snapshot, selecting/opening a run detail view, and quit command behavior. Focused CLI coverage verifies the TUI runner receives refresh/open callbacks and that the run hook is not invoked.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/tui`; `go test ./internal/cli -run 'TestTUI'`; `go test ./internal/tui ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched as a smoke command because it waits for terminal input.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add `revolvr tui` showing task counts, latest run summary, and recent runs from `internal/app`.
- Files changed: `internal/tui/model.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr tui`, which loads the current `internal/app.Status` snapshot and opens the Bubble Tea status model. The command renders uninitialized state, task counts, latest run summary fields, and recent runs through `internal/tui`.
- Tests added: focused CLI coverage for command discovery/help, uninitialized TUI snapshots without creating runtime state, and populated TUI snapshots backed by app-loaded tasks and recent runs.
- Verification run: `gofmt -w internal/tui/model.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'Test(NewRootCommandConstructsExpectedCommands|RootHelpWorks|TUI)'`; `go test ./internal/tui`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr tui --help`; `go run ./cmd/revolvr config check`; `git diff --check`.
- Verification result: all commands passed. The interactive `revolvr tui` session itself was not launched as a smoke command because it waits for terminal input.
- What remains: next unchecked backlog item is to add basic TUI actions for refresh, opening selected run details, and quit, without starting real Codex runs yet.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add stable Charm dependencies for Bubble Tea, Bubbles, and Lip Gloss, and create a minimal `internal/tui` model that renders a static app status snapshot in tests.
- Files changed: `go.mod`, `go.sum`, `internal/tui/model.go`, `internal/tui/model_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Dependencies added: direct Charm requirements for Bubble Tea `v1.3.4`, Bubbles `v0.20.0`, and Lip Gloss `v1.1.0`. These are the newest stable tagged versions found that keep compatibility with the repo's `go 1.22` directive; newer Bubble Tea and Bubbles releases require Go 1.23+ or 1.24+.
- Behavior changed: added `internal/tui.StatusModel`, a Bubble Tea model backed by a Bubbles viewport that renders an `internal/app.StatusResult` snapshot for initialized and uninitialized state. It currently supports static rendering, terminal resize messages, viewport updates, and quit keys without adding a CLI command.
- Tests added: focused `internal/tui` coverage for uninitialized output and an initialized static snapshot with task counts, latest run details, and recent runs.
- Verification run: `gofmt -w internal/tui/model.go internal/tui/model_test.go`; `go mod tidy`; `go test ./internal/tui`; `go test ./...`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add `revolvr tui` showing task counts, latest run summary, and recent runs from `internal/app`.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move run once and run loop orchestration behind `internal/app`, preserving CLI output and `run --max-passes` guardrails.
- Files changed: `internal/app/config.go`, `internal/app/run.go`, `internal/app/app_test.go`, `internal/cli/config.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr run --once` and `revolvr run --max-passes` now call `internal/app` for run config loading, pass execution, loop stats, stop reasons, outcome errors, and max-pass guardrail decisions. CLI rendering stays in `internal/cli`, preserving run summaries, Codex progress lines, and final loop summary output. `config check` and `doctor` now share the app-owned run config loader.
- Tests added: focused `internal/app` coverage for run config loading, progress callback wiring, invalid config short-circuiting, repeated-failure guardrails, immediate stop for blocked or dirty failed outcomes, and config-error loop stats. Focused CLI coverage was added for `run --max-passes` config-error summary output.
- Verification run: `gofmt -w internal/app/config.go internal/app/run.go internal/app/app_test.go internal/cli/config.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/app -run 'TestRun(Once|Loop)'`; `go test ./internal/cli -run 'TestRun(Once|MaxPasses)'`; `go test ./internal/app ./internal/cli`; `go test ./...`; `git diff --check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add stable Charm dependencies for Bubble Tea, Bubbles, and Lip Gloss, and create a minimal `internal/tui` model that renders a static app status snapshot in tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move task add/list/retry orchestration behind `internal/app`, update CLI task commands to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `internal/cli/state.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr task add`, `revolvr task list`, `revolvr task retry`, and the shared `task unblock` recovery path now call `internal/app` task operations for state resolution, store access, persistence, and blocked-to-pending transitions. CLI rendering stays in `internal/cli`, preserving existing output.
- Tests added: focused `internal/app` coverage for task add/list persistence, trimmed input, retrying blocked tasks, missing task IDs, missing tasks, and non-blocked task rejection.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go internal/cli/state.go`; `go test ./internal/app -run 'TestTask(Add|Operations|List|Retry)'`; `go test ./internal/cli -run 'TestTask(Add|List|Retry|Unblock)'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move run once and run loop orchestration behind `internal/app`, preserving CLI output and `run --max-passes` guardrails.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: move receipt validation orchestration behind `internal/app`, update CLI `receipt validate` to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr receipt validate <run-id>` now uses `app.ValidateReceipt` for state resolution, ledger lookup, and receipt validation. CLI rendering and the failed-check command error remain in `internal/cli`, preserving existing output.
- Tests added: focused `internal/app` coverage for consistent receipt validation, failed checks returned as a result rather than a command error, empty run IDs, uninitialized state, and missing runs.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go`; `go test ./internal/app`; `go test ./internal/cli -run 'TestReceiptValidate'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr receipt --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr receipt validate 019f42a2-5584-7eff-827d-f4420b4e2000`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move task add/list/retry orchestration behind `internal/app`, update CLI task commands to use it without changing output, and add focused tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: introduce `internal/app` with read-only `Status` and `ShowRun` operations, update CLI `status` and `show` to use it without changing output, and add focused tests.
- Files changed: `internal/app/app.go`, `internal/app/app_test.go`, `internal/cli/root.go`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr status` and `revolvr show <run-id>` now load their read-only snapshots through `internal/app`; CLI rendering remains in `internal/cli` and existing output is unchanged.
- Tests added: focused `internal/app` coverage for uninitialized status reads, populated status snapshots with latest run events, persisted show history, missing runs, and uninitialized show reads.
- Verification run: `gofmt -w internal/app/app.go internal/app/app_test.go internal/cli/root.go`; `go test ./internal/app`; `go test ./internal/cli -run 'TestStatus|TestShow'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr show 019f42a2-5584-7eff-827d-f4420b4e2000`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to move receipt validation orchestration behind `internal/app`, update CLI `receipt validate` to use it without changing output, and add focused tests.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: seed the durable backlog with a TUI-readiness sequence for an `internal/app` service boundary and a future Bubble Tea frontend.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Tasks added: extract read-only `internal/app` status/show operations; move receipt validation into app; move task add/list/retry into app; move run once/loop orchestration into app; add stable Charm dependencies and a minimal `internal/tui` model; add `revolvr tui` dashboard; add basic TUI refresh/open/quit actions.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: commit these planning-state updates, then run a bounded multi-pass dogfood loop starting with the first unchecked backlog item.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a live dogfood verification script or README checklist that resets runtime state, queues a tiny task, runs once, and verifies receipt, ledger, commit, receipt validation, and clean-worktree consistency.
- Files changed: `scripts/dogfood-live.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Script added: `scripts/dogfood-live.sh` is an opt-in real-Codex dogfood check. It requires `codex`, Git identity, and a clean source worktree; builds a temporary `revolvr` binary; removes `.revolvr/`; initializes fresh runtime state; writes a one-command `go test ./...` verification config; queues a tiny single-file task; runs `revolvr run --once`; and checks the final receipt, ledger-backed `status` and `show` output, commit SHA, `receipt validate`, and final clean worktree.
- Documentation added: README Dogfooding and Development Checks sections now point to the live dogfood script and warn that it resets local `.revolvr/` state and creates a commit on success.
- Verification run: `bash -n scripts/dogfood-live.sh`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go test ./...`.
- Verification result: all commands passed. The live script itself was not executed in this pass because the selected run rules prohibit launching nested Codex runs.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add safer `run --max-passes` loop guardrails for repeated failures or blocked tasks, and show a concise final loop summary.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `run --max-passes` now always prints one final `Loop summary` line for no-task, max-pass, failed, blocked, runner-error, context, and config-error exits. The bounded loop stops immediately after blocked outcomes or failed outcomes that report changed files/capture errors, and clean repeated failed outcomes trip a two-pass failure guardrail.
- Tests added: focused CLI coverage for final summaries on no-task and max-pass exits, repeated clean failures, blocked outcomes, and failed dirty passes.
- Documentation added: README run docs now note the final loop summary and early stop behavior for failed or blocked passes.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestRunMaxPassesStopsAfterNoTask|TestRunMaxPassesStopsAfterRepeatedFailuresWithSummary|TestRunMaxPassesStopsAfterBlockedOutcomeWithSummary|TestRunMaxPassesStopsAfterFailedPassWithChangedFiles|TestRunMaxPassesCapIsHonored'`; `go test ./internal/cli`; `git diff --check`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr run --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a live dogfood verification script or README checklist that resets runtime state, queues a tiny task, runs once, and verifies receipt, ledger, commit, and clean-worktree consistency.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add focused failure-recovery CLI support for blocked tasks with `revolvr task retry <task-id>`.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr task retry <task-id>` as the recovery-oriented command for blocked tasks. It reuses the existing blocked-to-pending store update, rejects missing or non-blocked tasks, clears current blocker fields, preserves the same task ID/text/summary/created timestamp, and leaves the existing `task unblock` command available.
- Tests added: focused CLI coverage for command discovery/help, successful retry making the same blocked task selectable by `run --once`, completed-task rejection, and missing-task rejection.
- Documentation added: README task queue recovery example now uses `task retry <task-id>`.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli -run 'TestNewRootCommandConstructsExpectedCommands|TestParentCommandHelpOutput|TestTaskRetryMakesBlockedTaskRunnableForRunOnce|TestTaskRetryDoesNotRevertCompletedTask|TestTaskRetryMissingTaskReturnsClearError|TestTaskUnblockDoesNotRevertCompletedTask|TestTaskUnblockMissingTaskReturnsClearError'`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr task --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr task retry --help`; `git diff --check`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add safer `run --max-passes` loop guardrails for repeated failures or blocked tasks, and show a concise final loop summary.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: add a first-class receipt validation command that checks a run receipt against ledger completion time, commit SHA, changed files, verification results, and artifact existence.
- Files changed: `internal/receipt/validation.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: added `revolvr receipt validate <run-id>`, which loads the run ledger history, parses the recorded receipt, compares receipt identity, finalized timestamp, commit SHA, changed files, verification results, and recorded artifact paths, prints per-check results, and exits nonzero when validation fails.
- Tests added: focused CLI coverage for a fully consistent receipt and for mismatched timestamp, changed files, verification results, and a missing artifact.
- Documentation added: README now documents `receipt validate <run-id>` in the status/show inspection flow.
- Verification run: `gofmt -w internal/receipt/validation.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/receipt`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr receipt --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`; final `go test ./...`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add focused failure-recovery CLI support for blocked tasks, starting with one command to retry or unblock a blocked task safely.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: expand `revolvr status` to show latest run summary, verification status, commit SHA, and artifact path hints when a run exists.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `status` now loads the latest run's event history and prints the latest summary, verification status, commit SHA, and artifact paths after the existing latest run line. Missing latest-run fields render as `none`; artifact paths reuse the same order as `show`.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a first-class receipt validation command.
- Blockers: none.

Task completed on 2026-07-08:

- Selected task: document the next harness-usefulness improvements in the durable task backlog for continued dogfooding.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; durable planning state only.
- Tasks added: richer latest-run `status` output; receipt validation command; blocked-task retry/unblock support; `run --max-passes` guardrails and loop summary; live dogfood verification script/checklist.
- Verification run: not run; durable planning state only.
- Verification result: not run.
- What remains: start the first unchecked backlog item with a fresh dogfood pass.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: fix finalized receipt timestamps so the harness overwrites stale agent-authored timestamps with the run completion time.
- Files changed: `internal/receipt/update.go`, `internal/receipt/receipt_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: receipt finalization now writes the harness completion timestamp into parsed receipts, and run completion uses the same timestamp for the ledger and final receipt.
- Verification run: `gofmt -w internal/receipt/update.go internal/receipt/receipt_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./internal/receipt`; `go test ./internal/runonce`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `git diff --check`.
- Verification result: all commands passed.
- What remains: commit if requested, then run another real dogfood pass to confirm receipt timestamps in the live path.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add one concise README Dogfooding note that real dogfood runs should start from a clean worktree and use `status`/`show` to inspect the result.
- Files changed: `README.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: Dogfooding now explicitly notes that real runs should start from a clean worktree and inspect recorded results with `status` and `show <run-id>`.
- Verification run: not run; documentation-only change and the Revolvr harness owns pass verification.
- Verification result: not run.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: resolve the dogfood run diagnostics found after the README dogfooding pass: stale receipt body facts, false `.agent/STATE.md` changed-file mismatch warning, zero Codex usage metrics, and missing live `run` progress output.
- Files changed: `internal/receipt/claims.go`, `internal/receipt/metrics.go`, `internal/receipt/update.go`, `internal/receipt/receipt_test.go`, `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: final receipt rewrites now refresh harness-owned `Changed Files` and `Verification` body sections to match finalized frontmatter; dotfile path claims keep their leading `.`; Codex usage parsing continues past malformed JSONL fragments when a later valid usage event exists; `revolvr run --once` and `run --max-passes` stream summarized Codex progress to stdout before the final summary.
- Verification run: `gofmt -w internal/receipt/claims.go internal/receipt/metrics.go internal/receipt/update.go internal/receipt/receipt_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/receipt`; `go test ./internal/codexexec`; `go test ./internal/cli`; `go test ./internal/runonce`; `bash -n scripts/smoke-local.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: commit the completed repair slice.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a README Dogfooding section with the commands for `doctor`, `task add`, `run --once`, `status`, and `show`.
- Files changed: `README.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: grouped the dogfooding flow into one README section with commands for preflight, queueing a task, running one pass, checking status, and showing a recorded run.
- Verification run: not run; documentation-only change and the Revolvr harness owns pass verification.
- Verification result: not run.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a `revolvr doctor` dogfood preflight for Codex, Git identity, clean worktree, runtime ignore state, and verification readiness.
- Files changed: `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr doctor` now reports initialized state, effective config loading, configured Codex executable availability, configured Git executable availability, Git identity, clean worktree state, `.revolvr/` Git ignore readiness, and effective verification command readiness. It exits nonzero with `doctor: preflight failed` when required checks are not ready.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/doctor.go internal/cli/root_test.go internal/cli/doctor_test.go`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; expected-failure `go run ./cmd/revolvr doctor` on the dirty implementation checkout, asserting `Dogfood preflight:`, `Ready: false`, and `doctor: preflight failed`; final `go test ./internal/cli` after cleanup.
- Verification result: all commands passed, with the doctor command failing only in the expected pre-commit dirty-worktree check.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: make `revolvr init` locally ignore `.revolvr/` in Git worktrees so fresh dogfood repos do not start dirty.
- Files changed: `internal/cli/state.go`, `internal/cli/root_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr init` now idempotently adds `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, leaving non-Git directories alone. The fake-Codex smoke tests no longer create a tracked `.gitignore` and assert that post-init Git status stays clean.
- Verification run: `gofmt -w internal/cli/state.go internal/cli/root_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` verification failure path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex-verification-failure.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, has fake Codex create `generated.txt` and a matching failure receipt, intentionally fails verification with `test -f required.txt`, checks the run failure summary, confirms the task is blocked, confirms no commit is created, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` success path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, verifies `generated.txt`, runs `revolvr run --once`, checks completed task/run status, confirms a commit, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Files changed: `scripts/smoke-local.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-local.sh` builds a temporary `revolvr` binary, runs `init`, `task add`, `task list`, `config check`, and `status` in a temporary workspace, and asserts expected outputs without invoking Codex.
- Verification run: `bash -n scripts/smoke-local.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: root README covering setup/build, `init`, task queue commands, optional `.revolvr/config.yaml`, `config check`, `run --once`, `run --max-passes`, `status`, `show`, and development checks.
- Verification run: `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: expand `revolvr config check` output to show effective verification command details, not only the command count.
- Files changed: `internal/cli/config.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `config check` now prints one deterministic detail row per effective verification command after `Verification command count`, including command index, name, args, and optional dir/timeout fields. No detail rows are printed when the effective command list is empty.
- Verification run: `gofmt -w internal/cli/config.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI; the targeted smoke-test note/script task also remains.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add Codex yolo/dangerous bypass support for autonomous harness runs, fix fresh-session wrapper flags, and update focused tests.
- Files changed: `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/cli/config.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `agent-one.sh`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: CLI-initiated harness runs now default to Codex dangerous bypass/yolo mode and pass `--dangerously-bypass-approvals-and-sandbox` instead of separate sandbox/approval flags when enabled. Config supports `codex.dangerously_bypass_approvals_and_sandbox` and `codex.yolo` aliases, including explicit `false` to disable the default. `agent-one.sh` now uses the dangerous bypass flag with valid `codex exec` ordering.
- Verification run: refreshed Codex manual with `node /home/gernsback/.codex/skills/.system/openai-docs/scripts/fetch-codex-manual.mjs`; `gofmt -w internal/cli/root.go internal/cli/config.go internal/cli/root_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config`; `bash -n agent-one.sh agent-loop.sh`; `codex exec --dangerously-bypass-approvals-and-sandbox --help`; `codex exec --yolo --help`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous task completed on 2026-07-07:

- Selected task: replace bare parent command placeholder output for `revolvr task` and `revolvr config` with normal help output, and update focused CLI tests.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: bare `revolvr task` and `revolvr config` now show Cobra help instead of placeholder "not implemented yet" output.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr task`; `go run ./cmd/revolvr config`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous setup performed on 2026-06-29:

- Initialized local Revolvr runtime state with `go run ./cmd/revolvr init`.
- Created fresh-session agent loop setup files.
- Did not run `agent-one.sh`; that would invoke a nested Codex session.

## Current Repository Understanding

- Stack: Go 1.22 CLI application using Cobra, YAML config, and SQLite.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test ./...`.
- Lint/typecheck command: none configured; use `gofmt -w <changed go files>` and `go test ./...`.
- Important directories: `cmd/revolvr`, `internal/cli`, `internal/runonce`, `internal/ledger`, `internal/taskqueue`, `internal/codexexec`, `internal/receipt`, `internal/verification`, `internal/commit`, `internal/gitstate`, `internal/runner`.
- Runtime state: `.revolvr/`, created by `revolvr init` and ignored by Git.

## Verification Gaps

- No separate lint command is configured.

## Notes For Next Fresh Session

- Read `AGENTS.md` first.
- Read `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md` before making changes.
- Do one task, verify, update state, and stop.
- Do not run nested Codex from inside another active Codex session.
