# Agent State

## Current Focus

`AUDIT-R4-05` is complete. Autonomous finalization completion evidence,
capsule, and manifest publication/replay now retain one descriptor-rooted
repository and parent authority through exclusive publication, cleanup, sync,
and exact readback. The next bounded task is `AUDIT-R4-06`; there are no
blockers.

## Stable Autonomous Finalization Artifact Boundary (2026-07-14)

- `Finalize` binds one `runtimepath.Boundary` for its complete transaction and
  routes frozen completion evidence, `completion.md`, and the completion
  manifest through one `finalizationStorage`. Replay verification uses the
  same retained root authority instead of re-resolving paths independently.
- Immutable completion artifacts are written and synced through an opened
  temporary inode and published with a descriptor-relative exclusive link.
  A racing destination is never overwritten: exact bytes are accepted as
  replay, conflicting or unsafe destinations fail closed.
- Publication retains the opened parent and temporary through post-link
  namespace checks, directory sync, and strict descriptor readback. Cleanup
  removes only an unpublished opened temporary from that parent; once the link
  syscall has completed the final name is recorded before later validation,
  so a published artifact is never treated as cleanup residue.
- Protected descriptor reads replace the finalization-specific parent walker,
  `Lstat`/`ReadFile`, `MkdirAll`, `CreateTemp`, pathname rename/removal,
  directory reopen/sync, and readback helpers. Canonical task and state changes
  remain with their already-hardened `taskfile` and `autonomousstate` owners.
- Permanent regressions reject final symlinks, hard links, unsafe modes,
  competing unsafe destinations, and renamed ancestors at every directory,
  temporary, sync, publication, readback, and cleanup boundary. They prove
  published-vs-unpublished state and exact outside entries, bytes, modes,
  symlink targets, and link counts.
- A full `Finalize` regression substitutes the completion namespace with
  same-named attacker temporary/final files and proves no outside mutation and
  no task, canonical-state, or ledger advancement. Focused adversarial tests
  passed ten repetitions and the package race suite.
- The complete ordinary, shuffled, and race suites, `go vet`, module
  verification, formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus
  Windows-stub cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-06`; blockers: none.

## Stable Autonomous Archive Storage Boundary (2026-07-14)

- `Archive` and `Reopen` bind one `runtimepath.Boundary`, retain the actual
  Git-admin and archived-task state flocks, and route archive/runtime evidence
  through one `archiveStorage`. Locked reopen verification reuses that same
  store and proves the selected archive/task identity did not change during
  admission. Read-only list/show/load/verify operations retain one root
  authority for their complete traversal.
- Archive manifests, archived tasks/capsules, canonical state, frozen evidence,
  journals, history, and reopen records use protected descriptor reads.
  Archive enumeration recursively opens and reads children relative to
  retained directories; the bespoke symlink walker, `Lstat`/by-name reads,
  and `filepath.WalkDir` authority are removed.
- Immutable artifacts/history are written and synced through an opened
  temporary inode, published with a descriptor-relative exclusive link, then
  directory-synced and identity-read back. Mutable journals use retained-temp
  descriptor-relative replacement. Active-task removal unlinks only the
  opened, identity-matched file from its stable parent.
- Every directory/file open, enumeration, file/directory sync, publication,
  removal, cleanup, and readback boundary rechecks the stable namespace and
  held leases. Cleanup removes only an unpublished opened temporary; a
  completed metadata publication is never mistaken for cleanup authority, and
  namespace/lease loss cannot redirect cleanup outside the repository.
- Permanent regressions reject final symlinks, hard links, unsafe modes,
  ancestor replacement at every immutable/mutable/removal metadata boundary,
  enumeration substitution, and held-lock inode replacement. They install
  same-named attacker temporaries and prove exact outside entries, bytes,
  modes, symlink targets, and link counts remain unchanged. A full `Archive`
  regression also proves no active-task or Git-HEAD mutation after rejected
  persistence substitution.
- Focused adversarial tests passed ten repetitions and the package race suite.
  The complete ordinary, shuffled, and race suites, `go vet`, module
  verification, formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus
  Windows-stub cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-05`; blockers: none.

## Stable Exact-Task-Run Persistence Boundary (2026-07-14)

- `RunTaskUntilTerminal` binds one `runtimepath.Boundary`, retains the opened
  operation and history directories, and keeps the actual exclusive
  `lock.Flock` for the complete operation. Admission, cycle, recovery, and
  terminal transitions all use that one store rather than re-resolving paths
  or reducing the lease to an unlock closure.
- Checkpoint/history recovery uses protected descriptor reads and stable
  directory enumeration. The old `CheckFile`/`CheckDir` followed by
  `os.ReadFile`/`os.ReadDir` pattern and its unused standalone load wrapper are
  removed.
- Immutable history now writes and syncs an opened temporary inode, publishes
  it with a descriptor-relative exclusive link, and syncs the retained history
  directory. Mutable `operation.json` retains its opened temporary through
  descriptor-relative replacement, operation-directory sync, and exact-byte
  readback.
- Every create, write/sync, history link, checkpoint replacement, cleanup,
  directory sync, and readback acceptance checks both the stable namespace and
  original operation lease. Cleanup unlinks only the still-opened temporary;
  namespace or lease loss leaves local residue instead of risking an outside
  unlink.
- Permanent end-to-end regressions substitute the operation directory before
  and after history/checkpoint open, publication, sync, cleanup, and readback;
  install same-named attacker temporaries; and prove the complete outside tree
  is unchanged including entries, bytes, modes, symlink targets, and link
  counts. Separate coverage rejects unsafe read evidence and replacement of
  only the held operation-lock inode.
- Focused adversarial tests passed ten repetitions and the package race suite.
  The complete ordinary and race suites, `go vet`, module verification,
  formatting/diff checks, CLI help, and Linux/Darwin/FreeBSD plus Windows-stub
  cross-builds passed. No dependency was added. The next task is
  `AUDIT-R4-04`; blockers: none.

## Stable Notification Persistence Boundary (2026-07-14)

- Notification delivery binds one `runtimepath.Boundary`, retains the opened
  delivery directory, and retains the hardened `lock.Flock` instead of
  converting it to an unlock closure. Every write, immutable link, journal
  replacement, cleanup unlink, sync, and readback acceptance rechecks the
  namespace and the held lease.
- Intent, payload, and transition history now write through opened temporary
  inodes and publish immutably with descriptor-relative exclusive links.
  `journal.json` uses a descriptor-relative atomic replacement. Both paths
  retain the opened temporary identity through post-publication checks and
  safely distinguish a completed metadata syscall from a pre-publication
  failure.
- History creation/opening is relative to the retained delivery directory.
  `Inspect`, `List`, journal reconstruction, and exact-content replay read and
  enumerate through stable directories and protected file descriptors; the
  notification-specific path walkers and `Lstat`/by-name reads are removed.
- Cleanup is fail-closed: it removes only the still-opened temporary from its
  stable parent while the original delivery lease remains valid. Namespace or
  lease loss preserves local residue and cannot unlink an attacker-selected
  outside entry.
- Deterministic before-open, after-open, before/after-publication, sync, and
  cleanup seams exercise intent, payload, history, and journal boundaries.
  Permanent regressions cover final symlinks, hard links, unsafe modes,
  renamed ancestors with same-named attacker temporaries, exact outside-tree
  contents/modes/link counts, and delivery-lock inode replacement.
- Focused tests passed repeatedly and under the race detector. The complete
  ordinary and race suites, `go vet`, module verification, diff/format checks,
  CLI help, and Linux/Darwin/FreeBSD plus Windows-stub cross-builds passed. No
  dependency was added. The next task is `AUDIT-R4-03`; blockers: none.

## Descriptor-Rooted Runtime And Autonomous-State Boundary (2026-07-14)

- `runtimepath.Boundary` binds the initially resolved repository root device
  and inode without retaining or leaking a descriptor. Every operation reopens
  that exact root and traverses descendants with no-follow `openat` calls,
  validating each opened/named identity and safe mode.
- Stable `Directory` and `File` handles now own descriptor-relative create,
  enumerate, exclusive link publication, atomic replacement, unlink, read,
  and directory/file sync. Existing package functions delegate to the same
  boundary rather than performing `Lstat` followed by a full-path operation.
- `autonomousstate.Store` retains one root identity, uses protected reads for
  canonical state and every planning/audit/attempt/input/block/optional-role/
  workspace/finalization history, and uses descriptor-relative immutable and
  state publication. The opened task directory and temporary inode remain
  bound across the locked CAS, rename, sync, and readback.
- Every production canonical-state replacement checks the held state flock
  immediately before publication and before accepting readback. Immutable
  state evidence checks the same lease before and after publication; cleanup
  proceeds only while the lease and stable namespace remain valid.
- The permanent original reproducer moves the real task directory aside,
  installs an outside symlink with attacker-controlled bytes under the same
  temporary basename, and proves replacement returns `ErrUnsafePath` without
  creating outside `state.json` or changing outside entries, contents, modes,
  or link counts. Shared-boundary regressions also cover ancestor replacement,
  cleanup refusal, exclusive link publication, and repository-root identity
  replacement.
- No dependency was added; the implementation uses the already-present
  `golang.org/x/sys/unix` module. Verification passed for focused and complete
  Go suites. Race, vet/module, diff, and supported-platform compile evidence is
  recorded before the implementation commit. The next task is `AUDIT-R4-02`;
  blockers: none.

## Fresh R4 Wide-Sweep Audit (2026-07-14)

- AP-01 is a systemic check/use filesystem gap. A temporary deterministic
  regression used `FailureBeforeStateRename` to replace the task-state
  namespace and proved that `replaceState` created an outside `state.json`
  from attacker-controlled bytes before returning an error. The same root
  pattern affects shared runtime-path creation and multiple state,
  notification, task-run, archive, finalization, and evidence-read owners.
- AP-02 proves the runner returns immediately after sending process-group
  `SIGKILL` without polling for exit. AP-03 proves active TUI `q`/`ctrl+c`
  emits `tea.Quit` before the background operation publishes its terminal
  message or finishes durable cleanup.
- AP-04 was experimentally reproduced: the same event with two nested usage
  objects yielded both token totals across 1,000 parses in each of ten focused
  runs. AP-05 identifies missing opened-file identity in authoritative source
  snapshots. AP-06 identifies two remaining map-order first-error diagnostics.
- Audit-only reproducer tests were removed. No production code or dependency
  changed. No speculative performance or LOC issue was recorded; consolidating
  corrected filesystem ownership is the one substantiated safe simplification
  opportunity.
- Fresh verification passed: ordinary, race-enabled, and shuffled complete Go
  suites; `go vet`; module verification; vulnerability scan with no reachable
  vulnerability; formatting/diff/shell checks; CLI help/config/status; and the
  supported Linux/Darwin/FreeBSD cross-build sample.
- The audit revision is `f9dcd960b3b4`. Follow-up tasks `AUDIT-R4-01` through
  `AUDIT-R4-11` are bounded by owner or behavior; blockers: none.

## Final R3 Audit Closure (2026-07-14)

- AP-01: current runner code inspects and settles the original process group
  after leader exit, refuses reused leader identities, preserves cancellation
  authority, and reports remaining descendants as a lifecycle error. Five
  repetitions each proved redirected natural-exit and cancelled writers cannot
  mutate after return.
- AP-02: current init code canonicalizes one worktree, preflights protected
  components before creation, uses no-follow named/opened identities, validates
  Git-reported admin/common/exclude paths, and accepts only reciprocal linked
  worktrees. Three focused repetitions covered hostile runtime, agent, task,
  profile, ledger, `.git`, and exclude components, post-open replacement, no
  outside mutation, and genuine common-exclude behavior.
- AP-03 through AP-07: twenty busy/cancellation repetitions retained typed
  SQLite evidence and reopened cleanly; fence unit/receipt/CLI regressions
  passed five times; SHA-1/SHA-256 validators and a real SHA-256 dossier passed
  three times; component-aware safe/traversal path and real-Git dossier tests
  passed five times; and all three multi-invalid diagnostic tests passed 100
  package repetitions.
- AP-08: the admitted-cycle file and all six audited symbols remain absent.
  The affected autonomous lifecycle packages passed three times, and complete
  repository compilation proves no consumer remains.
- Final verification passed: the complete suite twice after all production
  fixes, the original AP-03 shuffle seed, the complete race suite, `go vet
  ./...`, `go mod verify`, tracked-Go formatting, `git diff --check`, shell
  syntax, CLI help/config/status, all three documented non-Codex smoke tests,
  Linux/Darwin/FreeBSD amd64 CI-equivalent builds, and the Windows diagnostic
  stub build/message check.
- The first closure run exposed host-umask sensitivity in both fake-Codex smoke
  fixtures: their Git directories inherited mode `0775`, which AP-02 correctly
  rejects. Both scripts now set `umask 0022`; their complete success and
  expected-verification-failure paths then passed without weakening init.
- Files changed in closure: the two fake-Codex smoke scripts, durable agent
  state, and deletion of `AUDIT_PROBLEMS.md`. No dependency was added, nothing
  remains, and there are no blockers.

## Confirmed No-Caller Code Removal (2026-07-14)

- Repository-wide Go references prove `bytesSHA`, `persistTransition`, and
  `replaceFile` had definitions but no calls. Their three thin wrappers were
  removed; active notification paths continue to use the fault-aware
  persistence functions directly.
- `AdmittedCycleConfig`, `AdmittedCycleResult`, and `RunAdmittedCycle` had no
  source, test, documentation, or historical caller. The internal-only
  `admitted_cycle.go` prototype was removed; current app orchestration retains
  the live worker admission and completion path.
- Verification passed: affected packages once, five shuffled repetitions,
  affected-package race tests, `go test -count=1 ./...`, `go vet ./...`,
  `go mod verify`, formatting, `git diff --check`, a repository-wide absence
  check for every deleted symbol, Linux/Darwin/FreeBSD amd64 cross-builds, and
  the Windows diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-CLOSE-01`; blockers:
  none. `AUDIT_PROBLEMS.md` remains until that independent closure pass.

## Deterministic Validation Diagnostics (2026-07-14)

- Source-snapshot hash validation now has explicit `index`, `worktree`, then
  `snapshot` precedence. Dossier-cache root validation explicitly checks the
  control root before the execution root.
- Audit report application collects every open finding omitted from the
  current report, sorts the IDs, and reports the lexically first missing ID.
  Map iteration is no longer diagnostic authority at any of the three audited
  boundaries.
- Each regression supplies multiple simultaneous invalid values and asserts
  one exact error over 100 calls. Focused packages passed ten ordinary and ten
  shuffled repetitions; the exact regressions passed 100 package repetitions,
  and affected-package race tests passed.
- Verification passed: `go test -count=1 ./...`, `go vet ./...`,
  `go mod verify`, formatting, `git diff --check`, Linux/Darwin/FreeBSD amd64
  cross-builds, and the Windows diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-08`; blockers: none.

## Component-Aware Repository Paths (2026-07-14)

- Dossier-cache guidance and Git-tree paths now share one escape check. Only
  exact `..` or a cleaned path beginning with `..` plus the platform separator
  is traversal; a textual `..` prefix inside a legitimate component is inert.
- Existing empty, absolute, normalization, UTF-8, NUL, ordering, duplicate,
  mode/type, and protected `.git`/`.revolvr` checks remain unchanged.
- Unit regressions admit `..foo` and `..well-known/file` through both source
  guidance and repository-map construction. They reject `..`, `../foo`,
  `a/../../b`, an absolute path, `./foo`, `a/../b`, and `a//b` at both
  boundaries.
- A real Git fixture commits both safe names and proves they survive
  `ls-tree -z`, map construction, and complete autonomous dossier assembly.
- Verification passed: focused and complete affected-package tests, shuffled
  affected-package tests, ten race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, formatting,
  `git diff --check`, Linux/Darwin/FreeBSD amd64 cross-builds, and the Windows
  diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-07`; blockers: none.

## SHA-1 and SHA-256 Git Object Identities (2026-07-14)

- `internal/gitoid.Valid` is the sole Git object-ID grammar: exactly 40 or 64
  lowercase hexadecimal characters. Abbreviations, uppercase, non-hex, and
  whitespace-padded values fail closed.
- Task workspace/state, workspace-manager observations, archive and
  finalization evidence, dossier-cache sources, NUL-delimited `git ls-tree`
  records, and repository-map dossier projections all delegate to that shared
  contract. The cache and projection diagnostics now describe both supported
  lengths.
- Regressions validate both SHA-1 and SHA-256 forms and reject invalid
  length/case/hex values at the shared and `ls-tree` boundaries. Workspace,
  cache-source, and dossier-projection tests cover both supported forms.
- A real `git init --object-format=sha256` fixture executed successfully in the
  current environment. Its 64-character HEAD, tree, and `ls-tree -z` object
  IDs survive assembly, cache miss/publication, repository-map construction,
  dossier projection, and cache-hit replay.
- Verification passed: focused and complete affected-package tests, shuffled
  affected-package tests, five race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, formatting,
  `git diff --check`, Linux/Darwin/FreeBSD amd64 cross-builds, and the Windows
  diagnostic-stub cross-build.
- No dependency was added. The next task is `AUDIT-R3-06`; blockers: none.

## Fence-Aware Markdown Structure (2026-07-14)

- `internal/markdown.Fence` is the shared fence state machine for task imports
  and receipts. It recognizes backtick and tilde fences with up to three
  leading spaces, requires a same-marker closing fence at least as long as the
  opener, accepts longer closers and CRLF, and keeps an unclosed fence active
  through EOF.
- Task-import headings are structural only outside a fence. Fence markers and
  contents remain in the imported task text, while a real heading immediately
  after a valid closer is recognized normally.
- Receipt required-section discovery, claim-section extraction, and
  harness-owned section rewriting use the same fence state. Fenced `## Changed
  Files` and `## Verification` examples cannot satisfy required sections,
  redirect claims, or receive harness rewrites; code-block claim extraction
  retains its existing behavior for actual receipt sections.
- Regressions cover both marker types, 0–3-space indentation, shorter inert
  closers, longer valid closers, unclosed fences, CRLF, headings immediately
  after closure, exact fenced-example preservation, and an actual CLI import
  that creates exactly one task from the original reproducer shape.
- Verification passed: focused tests, all affected-package tests, shuffled
  affected-package tests, ten race-enabled focused repetitions,
  `go test -count=1 ./...`, `go vet ./...`, `go mod verify`, root help,
  formatting, and `git diff --check`.
- No dependency was added. The next task is `AUDIT-R3-05`; blockers: none.

## Live-Reader Busy Evidence Retention (2026-07-14)

- A live-read retry now retains the most recent SQLite busy/locked error for
  the full operation. If a later attempt returns cancellation/deadline or the
  context terminates around another error, the read returns its type's zero
  value and joins the context cause with that retained SQLite evidence.
- An ordinary non-context error still returns directly, and a context failure
  with no preceding busy attempt contains no fabricated SQLite evidence. Busy
  retry exhaustion continues to return the latest SQLite error.
- The former 50ms scheduling-dependent regression now captures one real
  rollback-journal busy error, then scripts later deadline, cancellation, and
  no-busy outcomes through the retry operation seam. It checks zero results,
  `errors.Is` context causes, `errors.As` SQLite evidence, latest-only busy
  retention, and successful same-reader and reopened-reader snapshots.
- Verification passed: the focused regression once and twenty consecutive
  times, the original shuffled seed across `./...`, twenty race-enabled
  focused runs, `go test ./...`, `go vet ./...`, formatting, and
  `git diff --check`.
- No dependency was added. The next task is `AUDIT-R3-04`; blockers: none.

## Initialization Filesystem Trust Boundary (2026-07-14)

- CLI state paths canonicalize the worktree once. Init performs a read-only
  preflight of every existing `.revolvr`, `.agent`, profile, task, ledger, and
  Git component before its first repository mutation; symlinks, wrong types,
  group/other-writable paths, hard links, and escaping identities fail closed.
- `runtimepath.OpenFile` is the shared writable-open primitive. It validates
  ancestors and the final component, opens with no-follow/nonblocking flags,
  proves the opened/named regular-file identity, and forbids pre-validation
  truncation. Profiles, the guarded ledger identity, Git exclude updates, and
  new task files use this boundary and recheck identity after writes.
- Git administrative paths come from bounded `git rev-parse` output. The
  reported top-level, Git directory, common directory, and common
  `info/exclude` must agree with the canonical worktree and protected path
  identities. External gitdir files are accepted only for genuine linked
  worktrees with a canonical `worktrees/<name>` admin path and matching
  backlink; forged pointers and per-worktree pseudo-excludes are rejected.
- The exclude updater appends through an already identity-checked descriptor.
  A path replacement after open is detected before use and cannot modify the
  outside replacement target.
- `writeNewTaskFile` validates containment through `runtimepath.EnsureDir`
  before creating missing directories and uses exclusive no-follow creation
  plus file sync and opened/named identity revalidation.
- Regressions cover `.revolvr`, `.agent`, profile/task directories, the final
  profile/task and ledger files, `.git` symlinks and forged gitdir files,
  `info/exclude`, unsafe modes/types, opened-file substitution, and a genuine
  linked worktree. Rejected init fixtures leave both repository and outside
  tree snapshots unchanged.
- Verification passed: five repeated focused runs, focused race tests, package
  tests, `go test -count=1 ./...`, focused `go vet`, tracked-Go formatting,
  `git diff --check`, root help, config check, status, supported Unix
  cross-builds, and the unsupported Windows diagnostic-stub build.
- No dependency was added. The next task is `AUDIT-R3-03`; blockers: none.

## Runner Process-Group Settlement (2026-07-14)

- `runner.Run` now inspects the original process group after `cmd.Wait` on
  every natural-exit path. Remaining descendants receive the existing bounded
  TERM/poll/KILL settlement before the runner returns.
- A leader that exits while descendants remain produces the distinct
  `ErrProcessTreeUnsettled` runner error even when its own exit code is zero;
  cancellation and deadline causes retain their existing result authority.
- Post-exit signalling first checks that no process has reused the reaped
  leader PID. If reuse is observed, the runner fails closed without signalling
  the unrelated group identity.
- Unix regressions use a direct shell child whose background writer redirects
  every inherited stream. Natural leader exit is not reported as success, and
  natural and cancelled runs cannot perform the delayed sentinel mutation
  after runner return.
- Verification passed: runner package test, five repetitions of six focused
  lifecycle tests, runner race test, `git diff --check`,
  `go test -count=1 ./...`, and
  Linux/Darwin/FreeBSD amd64 plus unsupported-Windows-stub cross-builds.
- No dependency was added. The next task is `AUDIT-R3-02`; blockers: none.

## Independent Wide-Sweep Audit (2026-07-14)

- Two high-severity boundary problems were reproduced: successful runner
  leaders can leave mutation-capable descendants alive, and `revolvr init` can
  follow repository-controlled symlinks and write outside the repository.
- A shuffled run and a focused invocation configured for twenty repetitions
  exposed loss of SQLite busy evidence when a live-reader context expires
  during a later retry.
- A CLI fixture proved that a task heading inside a fenced Markdown example is
  persisted as a second task. Receipt section scanners share the same issue.
- Direct code and Git probes found partial SHA-256-repository support, rejection
  of safe names beginning with `..`, three map-order-dependent diagnostics, and
  a small set of confirmed no-caller production code.
- Ordinary tests, race tests, `go vet`, module verification, formatting, shell
  syntax, local smokes, supported cross-builds, and CLI help passed.
  `govulncheck` found no reachable vulnerability. The shuffled/focused ledger
  failure is retained as audit evidence rather than hidden.
- No production code was changed, no dependency was added, and no commit was
  created during the audit.

## Final Audit Closure (2026-07-14)

- Task selected: `AUDIT-CLOSE-01`.
- Re-audit result: the source-lease settlement race, dirty-worktree commit
  escape, predictable coordinator lock identities, writable app ledger reads,
  inconsistent platform contract, and stale smoke header are all closed by
  their committed production boundaries and regression tests.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and deletion of
  `AUDIT_PROBLEMS.md`.
- Focused verification commands run: 25 repeated source-guard/run-once/
  autonomous-cycle race tests; real-Git dirty-worktree refusal; shared,
  source-writer, retention, Git-admin, and delivery lock path/substitution
  tests; immutable app-ledger projection tests; shell syntax and local smoke.
- Final verification commands run: tracked-Go `gofmt -l`, `git diff --check`,
  `go mod verify`, `go vet ./...`, `govulncheck ./...`, syntax checks for every
  tracked shell script, `go test -count=1 ./...`,
  `go test -race -count=1 ./...`, `go test -shuffle=on -count=1 ./...`, root
  help, config check, all three smoke scripts, Linux/Darwin/FreeBSD amd64
  cross-builds, and the Windows diagnostic-stub build/message check.
- Verification result: every focused and final check passed; `govulncheck`
  reported no reachable vulnerabilities. No live Codex execution was started
  through Revolvr.
- What remains: no backlog task. Blockers: none.

## Closed Wide-Sweep Audit (2026-07-14)

- Six evidence-backed problems were found: a cancellation/source-lease race,
  unsafe staging when pre-existing dirty work is allowed, unprotected
  coordinator lock-file identities, writable ledger opens in read projections,
  an unstated/inconsistent non-Unix build contract, and a stale local smoke-test
  assertion.
- No speculative performance or line-count cleanup was recorded.
- Final closure re-audited every finding against production code and focused
  regressions. Twenty-five repeated deterministic race tests passed, including
  canonical task, receipt, ledger, result, and single-release assertions.
- Final ordinary, race-enabled, and shuffled suites passed. The local smoke and
  both fake-Codex run-once smokes passed.
- Linux, Darwin, and FreeBSD builds passed. The intentional Windows diagnostic
  stub built successfully and retained its unsupported-platform message.

## Latest Completed Program (2026-07-14)

- One shared scheduler now owns graph validation, dependency semantics,
  deterministic ready ordering, and selected-next identity for mixed-pass and
  autonomous consumers. App, CLI, TUI, queue, and run-once projections agree
  and invalid graphs never fall back to an arbitrary pending task.
- Canonical task frontmatter is strict. Recognized fields retain their existing
  byte-preserving behavior; unsupported keys fail with exact source locations;
  and inert `x-` extensions survive metadata updates without affecting policy.
- `operator-checkpoint-v1` provides strict, receipt-bound external acceptance
  evidence. Fulfillment is atomic and replay-safe, never invokes Codex or
  commits, and unlocks dependents only through shared scheduler reevaluation.
- Autonomous migration supports deterministic all-or-nothing planning,
  dry-run inspection, locked state-before-task application, immutable recovery
  history, exact replay, orphan adoption, and conflict-safe restart behavior.
- Cross-surface regressions cover scheduler determinism, lifecycle and archive
  dependency states, invalid-graph/no-runner behavior, no-pending fallback, and
  task-list/status/TUI/run-once selection agreement.
- Operator documentation now covers canonical metadata, shared scheduling,
  checkpoint safety, autonomous `needs_input`, and dry-run-first migration and
  recovery.

## Cyber-ARPG Readiness Assessment

The read-only assessment is recorded in
`.agent/CYBER_ARPG_READINESS_ASSESSMENT.md`. It strictly loaded all 446 tasks
and 1,113 dependency edges, found no missing or duplicate identities/edges,
self-edges, cycles, terminal-unsatisfied edges, or scheduler diagnostics, and
selected `m0-architecture-dependency-guard` from two ready tasks. Task list,
status, and TUI agreed. Migration was dry-run only. The target repository's
HEAD, Git status, task/content manifests, and ignored runtime-state manifest
remained unchanged.

## Verification Baseline

- `gofmt` reports no unformatted Go files.
- `go test ./...` passes after app live-ledger projections were consolidated on
  the read-only opener and immutable-filesystem regressions were added.
- `go test -race -count=1 ./...` and
  `go test -shuffle=on -count=1 ./...` pass.
- `go vet ./...`, `go mod verify`, and `govulncheck ./...` pass; no reachable
  vulnerabilities were reported.
- `git diff --check` passes.
- Linux, Darwin, and FreeBSD amd64 CLI cross-builds pass; the unsupported
  Windows diagnostic stub also cross-builds and retains its failure message.
- Root help, config check, the local CLI smoke, and both fake-Codex run-once
  smokes pass.
- No live Codex execution was started through Revolvr during the final
  scheduling/checkpoint/migration campaign, readiness assessment, or audit.

## Durable Documentation

- `README.md`: operator setup, commands, workflows, and safety guidance.
- `AGENTS.md`: repository working and verification rules.
- `.agent/TASKS.md`: current backlog and compact completed-program index.
- `.agent/DECISIONS.md`: durable architecture and implementation decisions.
- `.agent/LOOP_PROMPT.md`: reusable fresh-session pass instructions.
- `.agent/CYBER_ARPG_READINESS_ASSESSMENT.md`: bounded external readiness
  evidence.

Obsolete setup handoffs, design-target drafts, and completed-program kickoff
notes and the resolved wide-sweep audit report were removed after their durable
content was incorporated into the files above. Detailed historical prose
remains available through Git history.

## Current Repository Understanding

- Stack: Go 1.22 CLI application.
- Entry point: `cmd/revolvr/main.go`.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test -count=1 ./...`.
- Formatting: `gofmt` on changed Go files.
- Runtime state: `.revolvr/`, local and ignored by Git.

## Verification Gaps

Remote GitHub Actions execution of the newly linted workflow has not occurred
in a local pass. The matching local test and cross-build matrix passes.

## Notes For Next Fresh Session

- Read `AGENTS.md`, `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md` before acting.
- Do one bounded task, verify it, update durable state, and stop.
- Do not use `codex resume` or depend on an old session transcript.
