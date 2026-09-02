# Agent Tasks

## Active Architecture Sequence

- The current implementation authority is the numbered sequence under
  `.agent/tasks/architecture-001` through `architecture-025`, with the approved
  compatibility task `architecture-016a` inserted between 016 and 017.
- Architecture tasks 001-025 and planning task PTC-001 are completed.
- ADR-025 supersedes the former desktop/Wails/Vue/REST/SSE and custom Python
  harness directions. Revolvr is terminal-first and uses direct tools by
  default.
- `architecture-024-ui` is completed. The Bubble Tea TUI now centers canonical
  run events and provides the composer, typed responses, compact status,
  command discovery, and focused change-summary/evidence/approval views over
  existing application services and dependencies. Its canonical mixed-pass
  metadata is reconciled at the terminal `simplify` phase.
- Architecture 025 completed with **defer**: baseline metrics exist, but
  qualifying current-brain usage, repeated source-linked failures, and
  smaller-fix insufficiency evidence do not. No Graphiti comparison prototype
  or implementation is authorized.
- E0 through E2 are complete. `tui-010-prove-shell-composition`,
  `tui-011-prove-resize-reflow`, `tui-012-prove-active-settlement`, and
  `tui-013-install-terminal-shell` are complete. TUI-020 through TUI-022 are
  complete, TUI-030 and TUI-031 are complete, TUI-032 is the only pending
  canonical task, and all later TUI tasks remain unpublished drafts.
- The 2026-09-02 cross-machine evaluation portability maintenance is complete:
  fixture identities now use Git-stable `0644`/`0755` modes, the golden is
  reconciled, and TUI-032 remains the only pending canonical task.
- The bounded current-state compaction audit is complete. Deferred EXT and
  superseded PTC work remain non-selectable.
- The 2026-08-27 attended-runtime compatibility and receipt-evidence
  reconciliation is complete maintenance, not a new architecture selector. It
  also normalized the two obsolete `complete` task statuses and restored
  canonical graph loading. It does not change the terminal-first reset.
- Architecture 023 was completed without selecting or beginning Architecture
  024 in the same pass.
- The legacy external-readiness backlog below is deferred while this canonical
  architecture sequence is active; its unchecked entries are not the current
  fresh-pass selector.
- PTC-101 through PTC-108B are superseded terminal tasks and are not fresh-pass
  selectors. Do not revive the chain; a future measured direct-tool failure
  may justify one separate small prototype task.

## Rules

- Work on the canonical task selected by `go run ./cmd/revolvr status`.
- Do exactly one product task per fresh loop pass; one next-task publication
  may occur only in its completion handoff.
- Mark a task complete only after verification passes.
- If blocked, record the blocker and stop.
- Add only small, specific, directly discovered follow-up tasks.
- Do not invent broad roadmap work.

## Current Architecture Work

- [x] Architecture 001-016 — completed.
- [x] PTC-001 — amend the active and deferred task specifications.
- [x] Architecture 016a — add programmatic-runtime compatibility seams to the
  completed direct-tool broker.
- [x] Architecture 017 — implement the host-owned verification engine.
- [x] Architecture 018 — implement the evidence model and completion rejection
  gates.
- [x] Architecture 019 — implement the auditor and bounded corrector loop.
- [x] Architecture 020 — implement the local embedding service adapter.
- [x] Architecture 021 — implement code indexing, retrieval, and context
  assembly. Qwen3-Embedding-0.6B Q8_0 is the sole supported vector
  representation; no prior-model runtime/schema/query compatibility remains.
- [x] Architecture 022 — build the deterministic architecture evaluation suite.
  All 20 Section 23.1 scenarios, closed worker-mode refusal, byte-stable
  baselines, recovery, retrieval, safety, and immutable authority contracts
  are recorded under `evals/` and `internal/evaluation`.
- [x] Architecture 023 — implement the bounded sequential queue. The canonical
  PostgreSQL queue is foreground-only, pins `direct_tools_v1`, admits exactly
  one global source-mutating worker, and remains closed to real-project starts
  while the Section 23.3 measured threshold is unset.
- [x] Architecture 024 — refine the existing Bubble Tea TUI toward a
  Codex-like terminal operator workflow: run-event transcript, command
  composer/operator responses, compact status/footer, command discovery, and
  focused change-summary/evidence/approval views. Reuse existing app services
  and keep business logic outside the TUI. **Completed at `simplify`.**
- [x] Architecture 025 — record and simplify the **defer** decision with its
  exact future re-evaluation trigger. No Graphiti comparison prototype or
  implementation was authorized. **Completed at `simplify`.**
- [x] 2026-09-02 evaluation portability maintenance — normalize deterministic
  fixture identities to Git `0644`/`0755` semantics, prove umask-independent
  identity, reconcile the golden hash cascade, and keep semantic outcomes
  unchanged across development machines.
- [x] 2026-08-27 TUI maintenance — collapse the Bubble Tea dashboard into a
  compact transcript-first operator console without changing commands,
  focused views, application services, ledger events, or `app.RunTimeline`.
- [x] 2026-08-27 TUI overhaul planning — capture the current console, proposed
  Codex-shaped transcript application, architectural boundaries, decision
  gates, epics, bounded tasks, acceptance, and verification in
  `docs/architecture/tui-overhaul/`. No implementation task was published.
- [x] 2026-08-27 TUI overhaul task decomposition — replace the two planning
  monoliths with one design/index, eight epic files, and an audited task set.
  Each task owns one decision, proof, component change, focused-view migration,
  or verification pass and states dependencies, scope, acceptance, verification,
  and exclusions. No implementation task was published.
- [x] TUI overhaul design review — review the draft tree under
  `docs/architecture/tui-overhaul/` and resolve D1-D6 before promoting only
  TUI-010. Do not guess through product semantics or duplicate the draft backlog
  here. D1 is accepted: emulate the accepted Codex interaction behavior in the
  existing Go/Bubble Tea TUI; Codex source and snapshots are evidence only. D2
  is accepted: initialized idle plain text opens the existing reviewed Add Task
  draft, every other plain-text state rejects, and typed needs-input remains
  option-only. D5 is accepted: there is no active steering or queued/deferred
  operator input; TUI-041 was removed, TUI-031 was narrowed, and no app/domain
  prerequisite was added. D3 is accepted as a bounded hybrid: `internal/tui`
  retains semantic source/live state, finalized identities append once through
  installed `tea.Println`, terminal history owns emitted rows and native
  reflow/copy, and viewports are overlay-only. No terminal layer, dependency,
  or app/domain prerequisite was added. D4 is accepted: migrate Help, Tasks,
  Runs/Run Detail, Preflight, Workflow, Change Summary, Evidence, Approval,
  then typed needs-input; retain every current key/command entry and page
  rollback until per-view parity, return-state, child-back, and removal gates
  pass. D6 is accepted: one startup-only `session-start` cell per TUI process
  owns product label, inspected absolute project root, and process-start
  initialization; refresh, resize, and overlays never re-emit it, restart does,
  and no Revolvr clear action is added. TUI-005 accepts literal initialized-
  idle, uninitialized, running, completed, failed, cancelled, needs-input,
  overlay, and 40-column source snapshots with exclusive row ownership,
  80-column normal width, 40-column minimum width, and intentional below-
  minimum behavior. E0 is accepted; TUI-010 was published separately and its
  transcript-shell composition proof is complete.
- [x] TUI-010 publication pass — promoted only the accepted
  `docs/architecture/tui-overhaul/tasks/tui-010-prove-shell-composition.md`
  proof into one canonical pending `.agent/tasks` task and the active selector.
  No product code, test, dependency, or terminal behavior changed, and the
  publication pass did not start implementation.
- [x] TUI-010 implementation — proved the accepted session/history append,
  replaceable managed frame, test output, composer input, and normal PTY
  restoration in `internal/tui/model_test.go`. No production shell, dependency,
  or later TUI task was added.
- [x] Current-state compaction audit — compacted `.agent/STATE.md` to current
  Architecture 001-025, ADR-025, verification, blocker, and selector facts.
  No product, architecture, evidence, dependency, runtime, or task-selection
  authority changed.
- [x] TUI-011 publication pass — promoted only the accepted resize/reflow proof
  into one dependency-satisfied pending canonical task. TUI-011 remains
  unstarted; no product code, test, dependency, or terminal behavior changed,
  and all later TUI tasks remain unpublished drafts.
- [x] TUI-011 implementation — proved explicit 80-to-40-to-24-to-80 managed-
  frame reflow, ANSI display-cell bounds, immutable committed source, exact-
  once `session-start`, replaceable live state, and reachable composer state in
  `internal/tui/model_test.go`. Its completion handoff published only TUI-012;
  TUI-013 and later remain unpublished.
- [x] TUI-012 implementation — proved existing Escape/`c` guards, cooperative
  cancellation and delayed `q`/Ctrl-C quit across every active run mode,
  stale-token isolation, distinct terminal outcomes, and append-once final-cell
  settlement in `internal/tui/model_test.go`.
- [x] TUI-013 implementation — installed the proven inline transcript shell,
  added the inspected project root to the status projection, emitted one
  source-backed `session-start`, removed persistent header chrome, and retained
  the dashboard as a managed migration panel without changing current routes,
  callbacks, or guards. Its completion handoff published only TUI-020.
- [x] TUI-020 implementation — defined the seven package-local transcript-cell
  kinds, stable identity and source storage, deterministic styled rendering,
  display-cell width wrapping, and warning-prefixed fallback evidence. Its
  completion handoff published only TUI-021.
- [x] TUI-021 implementation — projected the accepted latest-eight canonical
  run timeline window and typed run result into committed cells, removed the
  duplicated managed-dashboard history, and preserved exact-once startup and
  refresh emission. Its completion handoff published only TUI-022.
- [x] TUI-022 implementation — reconciled replaceable live state with canonical
  committed results through the installed append/ack boundary, stable run and
  operation identities, refresh deduplication, and stale-message guards. Its
  completion handoff published only TUI-030.
- [x] TUI-030 implementation — made the existing composer the primary focus,
  preserved populated input and slash-command paths, and retained modal and
  operation-settlement focus rules. Its completion handoff published only
  TUI-031.
- [x] TUI-031 implementation — routed initialized idle plain text to the
  existing reviewed Add Task flow, rejected every other accepted D2/D5 state
  without an app effect, and preserved active text through settlement. Its
  completion handoff published only TUI-032.
- [ ] TUI-032 implementation — add a compact, filtered slash-command popup
  over the existing command vocabulary and guards. This is the only pending
  canonical task; later TUI tasks remain unpublished.

## Superseded Programmatic Workspace And Continual Harness Sequence

PTC-101 through PTC-108B have `status: superseded` under ADR-025 and are
terminal/non-selectable. Their files retain the former proposals only as
explicitly labeled history. No replacement speculative chain exists.

PTC-101 is superseded because the existing PostgreSQL events/artifacts,
append-only run ledger, run/task event queries, app timeline, receipt
validation, and artifact projections already provide its proposed durable
trajectory value; no concrete missing query is evidenced. PTC-102 through
PTC-108B are superseded because the Python workspace, `python_exec`, scratch,
skills, refinement, and evaluation chain is speculative. Direct tools remain
the default.

## Current Backlog

### Common and Level 1 — Attended Single Task

- [x] EXT-01 — Make shared repository-path admission agree across doctor,
  status, canonical task loading, and the no-model autonomous admission probe.
  - Acceptance: One read-only shared check produces the same safe/refused
    classification for safe, missing, malformed, final-symlink, ancestor-
    symlink, hard-linked, group-writable, and identity-substituted .agent and
    .revolvr paths. The reproduced .agent mode 0775 fixture cannot report
    Ready: true when status or admission refuses it. Every refusal leaves the
    complete repository and outside-sentinel snapshots byte-for-byte and
    metadata-for-metadata unchanged.
  - Verification: Add TestExternalPreflightSharedPathMatrix in internal/app
    and TestDoctorStatusAdmissionAgreeOnUnsafeAgent in internal/cli; run
    go test -count=1 ./internal/app ./internal/cli -run
    'Test(ExternalPreflightSharedPathMatrix|DoctorStatusAdmissionAgreeOnUnsafeAgent)$',
    the same command with -race, then go test -count=1 ./....

- [x] EXT-02 — Add the settled mode-aware, read-only doctor command surface.
  - Acceptance: doctor --for accepts exactly attended-task, queue, and daemon;
    bare doctor is byte-for-byte equivalent to attended-task; an optional exact
    task selector narrows task-bound checks; invalid modes/selector
    combinations fail before commands or writes. Each mode validates the
    canonical task graph and every protected path used before that mode can
    start model work, while execution still rechecks authority.
  - Verification: Add TestModeAwarePreflight and
    TestDoctorForModesAndTaskSelector; run go test -count=1 ./internal/app
    ./internal/cli -run 'Test(ModeAwarePreflight|DoctorForModesAndTaskSelector)$',
    go test -race -count=1 ./internal/app ./internal/cli -run
    'Test(ModeAwarePreflight|DoctorForModesAndTaskSelector)$',
    go run ./cmd/revolvr doctor --help, and go test -count=1 ./....

- [x] EXT-03 — Enforce the initial repository, platform, and verification
  scope at mode-aware preflight and no-model admission.
  - Acceptance: Operator-controlled non-bare Git repositories without active
    submodules are admitted when otherwise safe. Bare repositories and any
    active submodule are refused before model, verification, workspace, ledger,
    or task mutation. Attended-task admits only Linux, macOS, and FreeBSD;
    queue and daemon admit only Linux. Missing verification authority, dirty
    Git authority where cleanliness is required, and an unresolved Git
    executable fail the same shared admission boundary. Attended-task
    effective attempt, action, elapsed, token, cycle, process, output,
    retained-disk, and enabled-notification bounds may use documented defaults
    but are visible in config check/doctor and recorded with run evidence.
  - Verification: Add TestExternalRepositoryShapeAndPlatformMatrix and
    TestAttendedEffectiveBoundsVisibleAndRecorded in internal/app; run go test
    -count=1 ./internal/app -run
    'Test(ExternalRepositoryShapeAndPlatformMatrix|AttendedEffectiveBoundsVisibleAndRecorded)$',
    the same command with -race, GOOS=darwin go test -c ./internal/app,
    GOOS=freebsd go test -c ./internal/app, and go test -count=1 ./....

- [x] EXT-04 — Add release-authored executable identity authority for external
  autonomous admission.
  - Superseded on 2026-08-27: the embedded single-build allowlist was removed.
    Current admission captures and repeatedly verifies the configured path,
    resolved path, SHA-256, and bounded single-line version. Exact identities
    belong in qualification evidence rather than compiled runtime policy.
  - Acceptance: The release manifest lists exact Codex CLI version strings and
    resolved executable SHA-256 values; the first manifest may contain exactly
    one Codex build. Preflight and execution reject an unlisted version,
    different bytes with the listed version, an unresolved executable, or
    identity drift between preflight and invocation. The resolved Git
    executable identity is also shown and recorded. Config check, doctor, run
    provenance, and the effective fingerprint render the same redacted
    identities without treating semantic version ranges as authority.
  - Verification: Add TestExternalExecutableIdentityAdmission in internal/app
    and TestReleaseCodexAllowlist in internal/codexexec; run go test -count=1
    ./internal/app ./internal/codexexec -run
    'Test(ExternalExecutableIdentityAdmission|ReleaseCodexAllowlist)$', the same
    command with -race, and go test -count=1 ./....

- [x] EXT-05 — Build one strict, reusable fake-Codex contract fixture for the
  production autonomous app path.
  - Acceptance: The fixture is a real executable invoked by the ordinary
    runner. It rejects unexpected argv, working directory, schema, environment,
    invocation count, and output sequence; emits deterministic supervisor and
    worker JSON/JSONL/receipt material; requires fresh ephemeral exec; and has
    no model, network, injected StepRunner, or in-process Codex shortcut.
  - Verification: Add TestStrictFakeCodexContract in internal/app and run
    go test -count=1 ./internal/app -run TestStrictFakeCodexContract, the same
    command with -race, go test -count=1 ./internal/codexexec ./internal/runner,
    and go test -count=1 ./....

- [x] EXT-06 — Prove the complete production-composition happy path through
  app.RunTaskUntilTerminal.
  - Acceptance: The test supplies no TaskRunInput.Runner and reaches the real
    productionStepRunner. One strict-fake operation proves workspace creation,
    supervisor decision, worker action, attempt admission/completion,
    verification, run-owned source commit, checkpoint advancement, audit,
    completion authorization, frozen evidence, canonical task/state
    completion, and terminal ledger completion. It asserts exact source, task,
    state, Git, receipt, ledger, workspace, and completion-artifact bytes and
    identities.
  - Verification: Add TestProductionAutonomousHappyPath in internal/app; run
    go test -count=1 ./internal/app -run TestProductionAutonomousHappyPath,
    go test -race -count=1 ./internal/app -run
    TestProductionAutonomousHappyPath, and go test -count=1 ./....

- [x] EXT-07 — Prove production correction, final verification, and clean
  independent re-audit through the same app entry point.
  - Acceptance: A strict-fake operation first records a verification failure
    or blocking audit finding, admits exactly one correction attempt, commits
    only the cited repair, runs a distinct final verification, persists exact
    finding resolution, runs a distinct clean audit, and completes. Attempts,
    commits, verification occurrences, audit runs, receipts, and terminal
    evidence are each present exactly once and use distinct required
    identities.
  - Verification: Add TestProductionAutonomousCorrectionAndReaudit in
    internal/app; run go test -count=1 ./internal/app -run
    TestProductionAutonomousCorrectionAndReaudit, the same command with -race,
    and go test -count=1 ./....

- [x] EXT-08 — Prove the production attended-task terminal-outcome matrix.
  - Acceptance: Separate strict-fake cases exercise needs_input, authorized
    block, verification failure, no_progress, trusted safety refusal, caller
    cancellation, restart of exact durable authority, and maximum_cycle. Each
    case enters through app.RunTaskUntilTerminal without an injected
    StepRunner, asserts the exact stop reason and immutable evidence, and proves
    the absence of every unauthorized worker, verification, commit, task,
    state, or terminal-ledger effect.
  - Verification: Add TestProductionAutonomousTerminalMatrix in internal/app;
    run go test -count=1 ./internal/app -run
    TestProductionAutonomousTerminalMatrix, the same command with -race, and
    go test -count=1 ./....

- [x] EXT-09 — Prove exact task-workspace branch and control-root authority.
  - Acceptance: Every external autonomous task uses one recorded task-scoped
    branch and linked workspace derived from an exact baseline commit. The
    control root, Git common directory, branch/ref, baseline, current HEAD, and
    workspace ownership marker are durable run evidence. Foreign paths, refs,
    registrations, markers, or changed control-root relationships fail before
    source mutation; no ambient operator branch is used as a task workspace.
  - Verification: Add TestExternalTaskWorkspaceAuthority in
    internal/autonomousworkspace; run go test -count=1
    ./internal/autonomousworkspace -run TestExternalTaskWorkspaceAuthority, the
    same command with -race, and go test -count=1 ./....

- [x] EXT-10 — Prove run-owned commit containment and the prohibited Git
  operation boundary.
  - Acceptance: Every generated commit contains exactly the admitted
    operation-owned source delta plus required task metadata and no other
    staged, tracked, untracked, or worktree bytes. Production autonomous code
    never invokes push, merge, rebase, reset, clean, or stash and never mutates
    another linked worktree. A command-spy integration fails on any prohibited
    verb and exact tree comparison proves the admitted commit contents.
  - Verification: Add TestExternalCommitContainsOnlyRunOwnedDelta in
    internal/commit and TestProductionAutonomyForbidsRepositoryIntegrationOps
    in internal/app; run go test -count=1 ./internal/commit ./internal/app -run
    'Test(ExternalCommitContainsOnlyRunOwnedDelta|ProductionAutonomyForbidsRepositoryIntegrationOps)$',
    the same command with -race, and go test -count=1 ./....

- [x] EXT-11 — Close the external Git containment edge-case matrix.
  - Acceptance: Real-Git fixtures cover dirty, staged, ignored,
    linked-worktree, SHA-1, SHA-256, concurrent external-commit, and active-
    submodule cases. Each case either preserves the exact admitted branch,
    index, worktree, and commit authority or stops before publication; active
    submodules always stop. Outside and unrelated-worktree sentinels retain
    exact entries, bytes, modes, targets, link counts, refs, and HEADs.
  - Verification: Add TestExternalGitContainmentMatrix in internal/app; run
    go test -count=1 ./internal/app -run TestExternalGitContainmentMatrix, the
    same command with -race, and go test -count=1 ./....

- [x] EXT-12 — Publish the settled interruption and recovery contract as a
  transition-seam matrix.
  - Acceptance: docs/external-recovery.md enumerates before/during/after
    supervisor, worker, verification, commit, checkpoint, audit, finalization,
    queue reconciliation, notification, and archive publication. Every row
    names durable authority, exact replay behavior, unsafe_or_ambiguous
    quarantine behavior, permitted automatic continuation by readiness level,
    prohibited inference, and the exact operator action when manual recovery
    is required. It preserves the old operation as immutable evidence and
    forbids generic retry from clearing quarantine.
  - Verification: Run rg -n
    'supervisor|worker|verification|commit|checkpoint|audit|finalization|queue reconciliation|notification|archive publication|unsafe_or_ambiguous|immutable|generic retry|Level 1|Level 2|Level 3'
    docs/external-recovery.md and git diff --check; manually cross-check every
    matrix row against .agent/AUTONOMOUS_EXTERNAL_READINESS.md and
    .agent/DECISIONS.md.

- [x] EXT-13 — Add the explicit operator recovery inspection and
  reconciliation command required by Level 1.
  - Acceptance: revolvr task recover <task-id> --operation-id <id> is read-only
    by default, reports task, state, workspace, Git, ledger, receipt, and
    artifact authority, and starts no model or mutation. Reconciliation
    additionally requires --reconcile --confirm-operation <id>, preserves the
    old operation unchanged, and creates a new operation identity only after
    all authorities agree. Existing retry/unblock commands cannot invoke or
    clear this path.
  - Verification: Add TestRecoverAutonomousTaskRequiresExactReconciliation in
    internal/app and TestTaskRecoveryCommand in internal/cli; run go test
    -count=1 ./internal/app ./internal/cli -run
    'Test(RecoverAutonomousTaskRequiresExactReconciliation|TaskRecoveryCommand)$',
    the same command with -race, go run ./cmd/revolvr task --help, and
    go test -count=1 ./....

- [x] EXT-14 — Prove Level-1 task and explicit-administration interruption
  recovery at every production durable transition seam.
  - Acceptance: Deterministic failure injection kills or machine-interrupts the
    production operation before and after supervisor, worker, verification,
    commit, checkpoint, audit, finalization, notification, and archive
    publication. Restart uses the same stable operation or delivery ID for
    exact replay or stops unsafe_or_ambiguous for operator recovery. Across the
    full matrix there is no duplicate commit, attempt charge, notification
    success claim, completed task, terminal ledger event, receipt, completion
    artifact, or archive.
  - Verification: Add TestProductionTaskInterruptionRecoveryMatrix in
    internal/app; run go test -count=1 ./internal/app -run
    TestProductionTaskInterruptionRecoveryMatrix, go test -race -count=1
    ./internal/app -run TestProductionTaskInterruptionRecoveryMatrix, and
    go test -count=1 ./....

- [x] EXT-15 — Make the complete release CI matrix mandatory on the exact
  candidate commit.
  - Acceptance: Remote CI tests the Go 1.22 source floor, go test ./..., the
    production autonomous strict-fake suite, go test -race, go vet, go mod
    verify, both fake-Codex smoke paths, and supported Linux/macOS/FreeBSD
    builds; the unsupported Windows diagnostic stub remains a separate build
    assertion. Required checks cannot be skipped on the release branch or tag,
    and every job reports the exact source commit.
  - Verification: Validate .github/workflows/ci.yml syntax and required job/
    trigger definitions, run every locally reproducible command represented by
    the workflow, and run git diff --check. EXT-19 separately supplies the
    remote execution proof for the exact candidate commit.

- [x] EXT-16 — Write and smoke-test the attended external-project operator
  runbook.
  - Acceptance: docs/external-project-runbook.md covers pinned installation,
    init/path permissions, configuration and verification, attended safety
    responsibilities, task creation/migration/scheduling/checkpoint/input,
    start/monitor/cancel/restart, evidence/receipt/ledger/workspace/completion/
    archive inspection, every Level-1 manual recovery state, review/accept/
    reject/remove of task workspaces without automatic push, export/retention,
    upgrade, and safe runtime-state removal. It names every attended default
    and where its effective value appears in preflight and run evidence. Queue
    and daemon are explicitly unapproved. scripts/smoke-external-attended.sh
    executes every non-destructive command exactly as written in a disposable
    fixture.
  - Verification: Run bash -n scripts/smoke-external-attended.sh,
    bash scripts/smoke-external-attended.sh, go run ./cmd/revolvr --help and
    each referenced subcommand --help, then git diff --check.

- [x] EXT-17 — Add an opt-in Level-1 dogfood evidence collector.
  - Acceptance: scripts/dogfood-external-level1.sh requires a clean disposable
    external repository, exact candidate binary/source commit, exact listed
    Codex identity, and the approved configuration. For each operation it
    records before/after source HEAD/status, outside sentinels, effective
    config and resource bounds, runtime state, ledger validation, receipts,
    task/state history, workspace and completion artifacts, resource use, and
    typed outcome. It never edits runtime state to manufacture recovery and
    emits a hash-verifiable manifest.
  - Verification: Run bash -n scripts/dogfood-external-level1.sh; execute
    scripts/dogfood-external-level1.sh --fixture-only twice and compare
    canonical manifests after removing only declared timestamps; prove
    dirty/non-disposable/wrong-binary/wrong-Codex inputs fail before mutation;
    run git diff --check.

- [x] EXT-18 — Produce a reproducible, versioned Level-1 release candidate.
  - Acceptance: From one clean exact source commit, reproducible instructions
    produce binaries recording Revolvr version, exact patched supported Go
    toolchain, target platform, and source commit. Go 1.22 source-floor tests
    pass. govulncheck ./... reports no reachable standard-library or module
    vulnerability; unreachable findings, if any, are recorded separately.
    Candidate hashes and build instructions are immutable dogfood inputs.
  - Verification: Build the candidate twice in fresh directories and compare
    hashes or document and explain every reproducibility variance; run
    go version, go test -count=1 ./..., go vet ./..., go mod verify,
    govulncheck ./..., supported-platform builds, and verify the embedded
    version/source metadata.

- [x] EXT-19 — Push the exact Level-1 candidate and obtain remote CI evidence.
  - Acceptance: With explicit operator authorization, the complete candidate
    commit is pushed without using Revolvr to push or integrate project work.
    The remote commit equals the dogfood source commit, all EXT-15 required
    checks pass on it, and the CI URL plus job conclusions are recorded in the
    release decision evidence. A local-only or moving branch is insufficient.
  - Verification: Compare git rev-parse HEAD with the remote candidate commit;
    use gh run view for that commit to prove every required job succeeded; hash
    the tested binaries and compare them with EXT-18. Do not mark complete
    without explicit commit/push authorization.

- [ ] EXT-20 — Execute the quantitative Level-1 real-Codex dogfood gate.
  - Current gate (supersedes the historical wrapper gates below): do not create
    another `agent-ext20-*.sh` wrapper and do not require any retired wrapper to
    be present or absent. The reusable workflow implementation stage is
    complete: `scripts/build-level1-candidate.sh` accepts explicit candidate,
    clean source commit/tree, output-root, Go-toolchain, and vulnerability-tool
    authority; runs and records the required floor/current tests, race, module,
    vet, ordinary/verbose vulnerability, supported-target, embedded-metadata,
    complete-manifest, and two-clone reproducibility checks; retains failures;
    and emits an externally hash-bound `candidate-authority.tsv`.
    `scripts/dogfood-external-level1-suite.sh` now requires that authority path
    and SHA-256 in every mode and records them in prepared suite authority. The
    implementation was syntax/help/negative-boundary verified without calling
    Codex. Its first source-bound construction attempt used published commit
    `463f13a7c54698493073f6a8feecdc76a55b2647` and completed the floor/current/
    race/module/vet/vulnerability matrix plus byte-identical supported builds,
    then failed while deleting read-only Go module-cache entries. The EXIT trap
    also lost its function-local status authority, so the retained failed root
    has no typed status. The single repair makes the status trap scope-stable
    and makes only the owned work root writable before deletion; focused probes
    and independent review pass, and the operator explicitly authorized its
    raw-Git publication. Exact local/fetched/public equality passed at clean
    repair commit `5f340a8232a6d1bc9e8fff55fbe0f37ad0957085`. The first new
    source-qualified attempt, RC.14, retained a typed floor-test failure after
    ambient `GOROOT=/usr/local/go` paired its Go 1.22.12 driver with Go 1.26.5
    tools. The single repair attempt unset that ambient override and produced
    independently verified candidate
    `level1-v0.1.0-rc.15-5f340a8232a6`, authority SHA-256
    `07172fbe1f3cc2fd8930da84d71b6e66deadadab4d0cbfdd75cf3018ee7f87bd`.
    Its quantitative gate started in retained root
    `.revolvr/ext20-level1-rc15-quantitative-20260731`, but the first expected
    successful operation stopped `unsafe_or_ambiguous` before source mutation.
    Planning-result application compared the pre-attempt dossier state with
    canonical state after append-only attempt admission/completion, producing
    `dossier sources do not contain exact task/state identities (task=true
    state=false)`. The exact manifest verifies and proves unchanged source
    HEAD, candidate, Codex, configuration, and outside sentinel. RC.15 has
    therefore failed the gate and cannot be retried, relabeled, or used for an
    external-use decision. The single repair attempt passes the exact
    pre-attempt dossier state into planning application, admits only a valid
    predecessor differing by append-only attempt accounting, and rejects all
    other state differences; focused and full Go tests pass. Independent
    review reproduced the failure boundary, accepted the repair, and the
    operator explicitly authorized raw-Git publication. Exact local, fetched,
    raw-Git, and public-REST `main` then matched clean repair commit
    `2be1c7831d5dd84d4871f8c9dca183ba2ec25dd9`. One new source-qualified
    candidate, `level1-v0.1.0-rc.16-2be1c7831d5d`, was constructed and
    independently verified with authority SHA-256
    `a7ac0a73e27e72c77177ae4661ff8f6eee6f587e29f230704972475cccab5ccf`.
    Its complete manifest, floor/current/race/module/vet/vulnerability matrix,
    supported builds, embedded metadata, empty build IDs, and two-clone byte
    comparisons pass. Independent review replayed the workflow and suite-static
    gates, accepted the exact bundle, and the operator explicitly authorized
    raw-Git publication of its durable record. Exact push-triggered CI run
    `30632362941` on source commit
    `2be1c7831d5dd84d4871f8c9dca183ba2ec25dd9` then passed all ten mandatory
    jobs, with every job's exact-source reporting step successful. Post-CI
    candidate, suite-static, manifest, topology, and RC.15-preservation checks
    also passed. Independent operator review repeated those exact remote and
    local assertions and explicitly authorized raw-Git publication of the
    three-file durable record. The fresh RC.16 quantitative gate then started
    in retained root
    `.revolvr/ext20-level1-rc16-quantitative-20260731`. Its first operation
    crossed the repaired planning boundary and admitted an implementer, but
    terminally stopped `unsafe_or_ambiguous` after the implementer created the
    requested source file and followed the prompt's relative receipt path.
    Because Codex runs in the task workspace, that path created an ignored
    workspace `.revolvr` directory; fail-closed source capture rejected it as
    policy-relevant before verification or commit. The exact manifest verifies
    and proves unchanged control HEAD, candidate, Codex, configuration, and
    outside sentinel. No later operation started. RC.16 has therefore failed
    the quantitative gate and cannot be retried, relabeled, or used for an
    external-use decision. The single repair attempt now gives mutable workers
    the exact absolute control-root receipt path while retaining repository-
    relative receipt evidence, and a workspace root-separation regression plus
    focused, race, and full Go tests pass. Follow-up operator review replayed
    the immutable failure/containment evidence, inspected the repair boundary,
    reran focused, race, and full Go tests, and explicitly authorized raw-Git
    publication of the exact six-file repair record. A later source-qualified
    candidate must repeat construction, independent verification, and exact
    remote CI before any new quantitative suite may start. Exact local,
    fetched, raw-Git, and public-REST `main` then matched the clean published
    receipt-root repair commit
    `4cdd716d3bdefd08066fd11e436d326deaf4242c`. One new source-qualified
    candidate, `level1-v0.1.0-rc.17-4cdd716d3bde`, was constructed and
    independently verified with authority SHA-256
    `4909b90eb351fb1eebff7249ae5138ff4af246d92fc100d1d1b8bf807b8f5700`.
    Its complete manifest, floor/current/race/module/vet/vulnerability matrix,
    supported builds, embedded metadata, empty build IDs, and two-clone byte
    comparisons pass. No model call or suite preparation occurred. Exact
    push-triggered CI run `30635879807` on source commit
    `4cdd716d3bdefd08066fd11e436d326deaf4242c` then passed all ten mandatory
    jobs, with every job's exact-source reporting step successful. Post-CI
    candidate, suite-static, manifest, topology, and RC.15/RC.16-preservation
    checks also passed. No model call or suite preparation occurred. What
    remains is a separate fresh collision-free quantitative Level-1 real-Codex
    suite against only this exact RC.17 authority. Follow-up operator review
    of candidate construction replayed both read-only gates, accepted the exact
    bundle and retained test/vulnerability evidence, confirmed RC.15/RC.16
    preservation, and explicitly authorized raw-Git publication of the
    three-file candidate record. Follow-up remote-CI review independently
    confirmed the sole exact-source run, all ten jobs and source-reporting
    steps, replayed post-CI candidate and preservation checks, and explicitly
    authorized raw-Git publication of the three-file remote-CI record.
    The fresh RC.17 quantitative gate then started in retained root
    `.revolvr/ext20-level1-rc17-quantitative-20260731`, suite ID
    `ext20-9c2b181a2a88`. Its first operation completed planning,
    implementation, configured verification, and a clean independent audit,
    but audit application terminally stopped `unsafe_or_ambiguous`: the audit
    cited the sole exact current verification kind/reference while
    independently paraphrasing its descriptive detail, and validation wrongly
    required full-struct equality including that model-authored prose. The
    exact manifest verifies and proves unchanged control HEAD, candidate,
    Codex, configuration, and outside sentinel; all collector ledger/receipt
    validations pass, no later operation started, and no aggregate exists.
    RC.17 has therefore failed the quantitative gate and cannot be retried,
    relabeled, or used for an external-use decision. The single repair attempt
    makes only the audit-report citation check use durable evidence identity
    `(kind, reference)` while exact provenance and resolution-evidence checks
    remain unchanged; a focused regression, focused race test, and the full Go
    suite pass. Follow-up operator review replayed the exact immutable failure
    and containment evidence, inspected the citation/provenance boundary,
    reran focused, package, race, and full Go tests, and explicitly authorized
    raw-Git publication of the six-file repair record. A later source-qualified
    candidate must repeat construction, independent verification, and exact
    remote CI before any new quantitative suite may start. Exact local,
    `origin/main`, raw-Git, and public GitHub `main` then matched the clean
    published audit-citation repair commit
    `0bed41ef930e7db3d0486bc9a82de2b5720fe49f`. One new source-qualified
    candidate, `level1-v0.1.0-rc.18-0bed41ef930e`, was constructed and
    independently verified with authority SHA-256
    `06d8e10e6de5e0ce0774afebc0d49dc543af6334ddba0a175517329729e024ef`.
    Its complete manifest, floor/current/race/module/vet/vulnerability matrix,
    supported builds, embedded metadata, empty build IDs, and two-clone byte
    comparisons pass. The suite's read-only static gate passed without fixture
    preparation or a model call. Exact push-triggered CI run `30641614557` on
    source commit `0bed41ef930e7db3d0486bc9a82de2b5720fe49f` then passed all
    ten mandatory jobs, with every job's exact-source reporting step
    successful. Post-CI candidate, suite-static, manifest, topology, source/
    workflow identity, and RC.15/RC.16/RC.17-preservation checks also passed.
    No fixture, quantitative suite, or model call occurred. Follow-up remote-CI
    review independently confirmed the sole exact-source run, all ten jobs and
    source-reporting steps, replayed all post-CI candidate and preservation
    checks, and explicitly authorized raw-Git publication of the three-file
    remote-CI record. Follow-up operator review of candidate construction
    replayed both read-only gates, accepted the exact bundle and retained test/
    vulnerability evidence, confirmed RC.15/RC.16/RC.17 preservation, and
    explicitly authorized raw-Git publication of the three-file candidate
    record. The fresh RC.18 quantitative gate then started in retained root
    `.revolvr/ext20-level1-rc18-quantitative-20260731`, suite ID
    `ext20-51c9684c419a`. Its first planner copied the exact task origin and
    supervisor-decision artifact into top-level inputs. In plan provenance it
    preserved the task origin and decision reference, but relabeled the
    decision artifact from `file` to `plan` and paraphrased its detail.
    Planning application correctly stopped `unsafe_or_ambiguous` before source
    mutation because plan provenance lacked the exact artifact. The exact
    manifest verifies and proves unchanged control/workspace HEAD, candidate,
    Codex, configuration, and outside sentinel; every collector ledger/receipt
    validation passes, no later operation started, and no aggregate exists.
    RC.18 has therefore failed the quantitative gate and cannot be retried,
    relabeled, or used for an external-use decision. The single repair attempt
    now renders the exact task-origin and supervisor-artifact pair together and
    explicitly requires both objects unchanged in top-level inputs and plan
    provenance; validation remains fail-closed. The focused normal/race tests
    and full Go suite pass. Follow-up operator review replayed the exact
    immutable failure and containment evidence, corrected the durable
    description of the changed kind and detail, inspected the prompt/validator
    boundary, reran focused normal/race, package, and full tests, and explicitly
    authorized raw-Git publication of the six-file repair record. A later
    source-qualified candidate must repeat construction, independent
    verification, and exact remote CI before any new quantitative suite may
    start. One new source-qualified candidate,
    `level1-v0.1.0-rc.19-1285fe1fcdb3`, was constructed and independently
    verified from clean published planner-citation repair commit
    `1285fe1fcdb3da15c28f5dbf45ab98f9167215e6`, with authority SHA-256
    `5c6191a6276c91ba2b802c90c7fbb3270602d750a2c5dd44130a6749d0ef1ffb`.
    Its complete manifest, floor/current/race/module/vet/vulnerability matrix,
    supported builds, embedded metadata, empty build IDs, and two-clone byte
    comparisons pass. The suite's read-only static gate passed without fixture
    preparation or a model call. Follow-up operator review replayed both read-
    only gates; confirmed the exact authority, complete topology, reproducible
    builds, source metadata, retained test/vulnerability evidence, and RC.15–
    RC.18 preservation; and explicitly authorized raw-Git publication of the
    three-file candidate record. Exact push-triggered CI run `30647171845` on
    source commit `1285fe1fcdb3da15c28f5dbf45ab98f9167215e6` then passed all
    ten mandatory jobs, with every job's exact-source reporting step
    successful. Post-CI candidate, suite-static, manifest, topology, source/
    workflow identity, and RC.15/RC.16/RC.17/RC.18-preservation checks also
    passed. No fixture, quantitative suite, or model call occurred. Follow-up
    operator review independently confirmed the sole exact-source run, all ten
    jobs and source-reporting steps, replayed the raw-Git and post-CI candidate
    checks, and explicitly authorized raw-Git publication of the three-file
    remote-CI record. The fresh RC.19 quantitative gate then started in
    retained root `.revolvr/ext20-level1-rc19-quantitative-20260731`, suite ID
    `ext20-56f74f29f1af`. Its first operation completed planning,
    implementation, configured verification, one run-owned commit, checkpoint
    advancement, and a clean independent audit. The audit supplied exact
    evidence that all work was satisfied, but the durable plan steps and
    acceptance criteria remained pending. The supervisor chose `complete`
    instead of routing a planner to reconcile those lifecycle fields, and the
    completion gate correctly stopped `unsafe_or_ambiguous` before
    finalization. The exact manifest verifies and proves unchanged control
    HEAD, candidate, Codex, configuration, and outside sentinel; every
    collector ledger/receipt validation passes, no later operation started,
    and no aggregate exists. RC.19 has therefore failed the quantitative gate
    and cannot be retried, relabeled, or used for an external-use decision.
    The single repair attempt adds an exact harness-level supervisor rule:
    `complete` requires already-terminal durable plan and acceptance state;
    evidence supporting pending lifecycle fields must first route `plan` for
    exact reconciliation. The deterministic completion validator remains
    unchanged and fail-closed. Focused normal/race tests and the full Go suite
    pass. Follow-up operator review replayed the exact immutable failure and
    containment evidence, inspected the final pending lifecycle state and
    prompt/validator boundary, reran focused normal/race, package, and full
    tests, and explicitly authorized raw-Git publication of the six-file
    repair record. A later source-qualified candidate must repeat construction,
    independent verification, and exact remote CI before any new quantitative
    suite may start. The next construction pass created exactly one retained
    candidate, `level1-v0.1.0-rc.20-7826c0fe97bd`, from clean published commit
    `7826c0fe97bd40508553702011475cea8b35e80f`. Construction, the workflow's
    read-only `--verify`, and the suite's read-only `--static` gate passed with
    authority SHA-256
    `6e722fcc7b28cab0af190c9febc5d13d9655adbf1883b954f55f737c5000b627`.
    Independent inspection exhausted this pass's one-repair allowance on two
    operator-authored command mistakes: it first named the manifest
    incorrectly, then invoked the binary with `version` instead of the recorded
    version interface. A fresh read-only review used the actual manifest and
    `--version`; replayed both read-only gates; passed exact topology,
    reproducibility, build-ID, metadata, version, vulnerability, source, and
    RC.15-through-RC.19 preservation assertions; and explicitly authorized raw-
    Git publication of the three-file candidate record. RC.20 is accepted by
    independent verification without rebuild or modification. Exact push-
    triggered CI run `30653427308` on source commit
    `7826c0fe97bd40508553702011475cea8b35e80f` then passed all ten mandatory
    jobs, with every job's exact-source reporting step successful. Post-CI
    candidate, suite-static, strict-manifest, topology, reproducibility,
    build-ID, metadata, vulnerability, source/workflow identity, and RC.15-
    through-RC.19 preservation checks also passed. No fixture, quantitative
    suite, or model call occurred. Follow-up operator review independently
    confirmed the sole exact-source run, all ten jobs and source-reporting
    steps, replayed the raw-Git and post-CI candidate checks, and explicitly
    authorized raw-Git publication of the three-file remote-CI record. A
    separate fresh pass prepared the sole collision-free RC.20 quantitative
    suite at `.revolvr/ext20-level1-rc20-quantitative-20260731`, suite ID
    `ext20-791fbad09b76`. Its first operation completed initial planning,
    implementation, configured verification, one run-owned source commit, and
    checkpoint advancement. A second planner then correctly proposed making
    the existing pending plan steps and acceptance criteria terminal from
    exact current evidence, but planning application stopped
    `unsafe_or_ambiguous` at `acceptance_matrix`: the revision validator
    prohibited any status/evidence change to an existing pending criterion and
    would likewise have prohibited the pending plan-step changes and preserved
    earlier supervisor-origin criteria. The exact manifest verifies and proves
    unchanged control HEAD, outside sentinel, candidate, Codex, and approved
    configuration; every ledger and receipt validation passes, no later
    operation started, and no aggregate exists. RC.20 has therefore failed the
    quantitative gate and cannot be retried, relabeled, or used for an
    external-use decision. The single repair permits only monotonic lifecycle
    progress for existing pending/in-progress plan steps and pending acceptance
    criteria, keeps their stable identity fields exact, preserves earlier
    supervisor-decision origins, and still requires terminal entries to remain
    byte-for-byte unchanged. Focused normal/race tests and the full Go suite
    pass. Follow-up operator review replayed exact candidate, manifest,
    containment, identity, ledger/receipt, token-bound, and historical-
    preservation checks; inspected the final pending state, proposed revision,
    and repaired validator/prompt boundary; reran focused normal/race and full
    tests; and explicitly authorized raw-Git publication of the exact nine-file
    failure-and-repair record. A later source-qualified candidate must repeat
    construction, independent verification, and exact remote CI before a new
    quantitative suite may start.
    Retired wrappers remain recoverable from Git history only. Ignored
    `.revolvr/` evidence is preserved and is not governed by wrapper-retention
    policy.
  - Historical gate record (non-operative): RC.6 and RC.7 remain immutable
    failed live-attempt evidence;
    RC.8 through RC.12 remain immutable failed local-construction evidence and
    every missing former `/tmp` root is terminal lost evidence. The fourth
    independently authored anonymous design passed sequence one, used its one
    neutral repair, then passed sequence two with repaired bytes unchanged. It
    is retained solely in sealed persistent ignored root
    `.revolvr/prospective-builder-revalidation-v4.5pWwTx` and grants only later
    read-only review. Controller verification published the record as
    `bae8ff6b1e5d7e14a9002cd7fbba1ece101dc005`, and every clean published
    preflight diagnostic passed. Fresh read-only review accepted the design
    only for exact builder/construction-launcher publication. The next gate is
    published as `2f21a4399a0a1bc00ceac345e0ebbeac9616d75a` and is
    `agent-ext20-rc12-builder-publication.sh`; its clean non-creating preflight
    passed. That bounded publication pass copied the reviewed draft bytes once
    to the exact ignored mode-`0555` builder
    `.revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh`, published the
    inert mode-`0755` construction-launcher target `agent-ext20-rc12.sh`, and
    preserved the protected parent at tightened mode `0755`. Neither identity
    was executed. Independent controller review accepted and raw Git published
    the record as `b09a1c5d9973f39f2447711a58e03cacf8edf642`; the clean
    construction preflight passed without executing the builder. The operator
    then executed the exact builder once. It terminally stopped before its
    first construction root because the final absent-release-asset loop in
    `verify_remote_collisions` propagated status `1` into `set -e`. All RC.12
    construction outputs remain absent, but the one-shot rule consumes RC.12
    and forbids retry or repair. The sole next gate after controller
    publication is read-only independent failure review via
    `agent-ext20-rc12-construction-failure-review.sh`. Raw Git published that
    record as `cfee541546da35ea60eac102996691f144279e4f`, and its clean
    non-executing preflight passed. RC.12 is terminal and EXT-20 remains
    incomplete. The operator completed its no-argument independent review;
    repository state remained unchanged, and controller replay accepted the
    deterministic pre-root status-propagation failure. A fresh candidate is
    not yet authorized. The sole next gate after raw-Git publication is
    `agent-ext20-rc13-builder-validation.sh`, which may create only a fresh
    anonymous persistent prospective design, two-sequence neutral evidence,
    and one inert later review launcher. It cannot create or execute an RC.13
    builder or candidate. Raw Git published it as
    `9e9e8740686efd17991da38b17fbda1d5eaaff0d`, and its clean non-creating
    preflight passed. The prospective pass created the sole ignored root
    `.revolvr/prospective-builder-validation-v5.tL50Wc`, but sequence one
    failed in neutral cleanup when an EXIT trap outlived function-local probe
    variables. The sole permitted repair removed the exact stranded neutral
    root and corrected that cleanup lifetime. The post-repair sequence passed
    syntax, both semantic publication/cleanup probes, full-context collision
    checks, focused static audit, status-64 exact-self refusal, and forbidden-
    residue checks, then failed available-history preservation because its
    baseline contained one extra final newline. The later failure permits no
    further repair. The root is unsealed rejected evidence; no manifest or
    review launcher was created. Independent controller verification accepted
    the rejection and froze the exact 11-file, 44,298-byte v5 identity. The
    operator's new next-gate direction separately authorizes only
    `agent-ext20-rc13-builder-revalidation-v6.sh`: one independently authored
    persistent v6 design with two complete neutral sequences and explicit
    cleanup-lifetime, canonical-EOF, status-propagation, v5-preservation, and
    accepted-byte gates. It cannot create or execute an RC.13 builder or
    candidate. Raw Git published it with the rejected-v5 record as
    `92eb3d85cad3e78f4e980da1031cca485c8ae8da`, and its clean non-creating
    preflight passed. The separately authorized v6 pass then independently
    authored the sole persistent root
    `.revolvr/prospective-builder-validation-v6.bHfL29`. Before sequence one,
    trap-lifetime and all cleanup variants passed, and the canonical history
    baseline was independently reproduced at exactly 1,662 bytes, 18 lines,
    SHA-256
    `09ee1691d91f1e1e63b83f63e0e3819c7db034c330253e193f9ec8e7797c1dd2`,
    with exactly one final LF and no blank line. Both complete sequences
    reached and passed all 12 gates without using the repair allowance;
    accepted design bytes and the frozen v5 aggregate identities remained
    unchanged before and after both. The v6 root is sealed mode `0500` with 13
    mode-`0444` files, 45,820 bytes, manifest SHA-256
    `ecdd6f9f5a589038754d1bdb8326d5e19a1ea660eb0bb53a17029fa2aa7734be`,
    stream
    `1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f`,
    and inventory
    `0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41`.
    No design full mode or product command ran. Inert
    `agent-ext20-rc13-builder-revalidation-v6-review.sh` is prepared solely
    for later read-only review after controller publication. Independent
    controller inspection accepted and raw Git published that record as
    `bb68b8016646e571bff1711f66dd81ff5ede5e7d`; its clean non-executing
    preflight passed. The operator completed the independent review with no
    state change. Controller inspection then rejected v6 for publication:
    full-role admission wrongly requires tracked review/publication history to
    be absent, exact builder bytes are not bound to the sealed design, and
    stage/final roots are outside the sealing/equality proof. V6 remains sealed
    immutable evidence but cannot become a builder. The sole next gate after
    publication is `agent-ext20-rc13-builder-revalidation-v7.sh`, authorizing
    only one independently authored persistent v7 neutral design with two
    complete sequences and explicit role, self-authority, root-sealing, and
    post-final-retention probes. Raw Git published it with the v6 rejection as
    `4b90b5b511168034a890468a3336b71806c87300`, and its clean non-creating
    preflight passed. RC.13 remains unconsumed, RC.12 remains terminal, and
    EXT-20 remains incomplete. The separately authorized v7 pass created the
    sole ignored root `.revolvr/prospective-builder-validation-v7.hYcT7T` and
    independently authored a fresh full design plus neutral harness. Its first
    preservation bootstrap stopped before publication because a `set -u`
    same-command local expansion referenced `phase` before assignment. The one
    permitted repair retained that bootstrap evidence and split the local
    declarations. Repaired sequence one then passed accepted-byte, terminal-
    history, current-controller/source/tool, syntax, and cleanup-lifetime
    gates. Its success publication probe sealed and copied the candidate root
    with equal root-inclusive inventories, but status `1` followed when
    `assert_distinct_file_inodes` used the same invalid local expansion for
    `rel`. The exact neutral final candidate had appeared, so build, stage,
    final, inventory, log, and terminal-report evidence was correctly retained
    and no cleanup ran. The repair allowance is consumed; sequence one did not
    complete, sequence two did not start, and the unsealed 32-file v7 root is
    rejected terminal evidence. V5, v6, and RC.12 exact snapshots remained
    unchanged. No manifest, v7 review launcher, RC.13 identity, product
    command, remote object, or continuation authority was created. No next
    gate exists without new explicit operator direction; `EXT-20` remains
    incomplete.
  - Acceptance: The exact candidate completes at least 10 real-Codex task
    operations across at least two disposable external repositories, including
    at least five successful source changes and the production scenarios for
    verification failure/correction, needs input, cancellation/restart, and
    safety refusal. There are zero containment violations, duplicate commits
    or attempt charges, lost terminal evidence, manual runtime-state edits, or
    unclassified ambiguous outcomes. Expected typed safe stops retain exact
    recovery evidence.
  - Verification: Validate every EXT-17 manifest/hash; independently total
    repositories, operations, successful changes, and required scenarios;
    compare before/after outside sentinels and Git authorities; run ledger and
    receipt validation for every operation; run
    scripts/dogfood-external-level1.sh --verify-manifest <manifest> for every
    manifest. Any material process, path, persistence, recovery, Codex,
    isolation, or commit change invalidates the affected evidence and requires
    rerun.

- [ ] EXT-21 — Tag and record the first Level-1 external-use decision.
  - Acceptance: With explicit operator authorization, an immutable v0.x.y tag
    names exactly the EXT-18/EXT-20 candidate commit and its tested artifacts.
    Every Release Decision Record field is nonblank, Approved readiness level
    is attended single task only, open exceptions include bounded impact,
    compensating control, owner, and expiry, and the decision remains rejected
    if any Level-1 gate or evidence is missing.
  - Verification: Resolve the tag and compare its commit and artifact hashes
    with CI and dogfood evidence; verify the Codex allowlist and supported OS
    fields; inspect the completed decision record; do not mark complete without
    explicit tag/publish authorization.

### Level 2 — Unattended Sequential Bounded Queue

- [ ] EXT-22 — Add explicit finite operational bounds to effective
  configuration and mode-aware output.
  - Acceptance: Task attempts, every action, elapsed time, model tokens, cycles
    per task, tasks per queue, daemon sweeps, process duration, output bytes,
    retained disk bytes, notification attempts when enabled, and queue workers
    have visible effective values. Queue/daemon require explicit finite
    positive values; missing, zero, negative, unlimited token/time, and hidden
    fallback authority are invalid. Attended defaults remain documented,
    visible, fingerprinted, and recorded with each run. Initial external queue
    authority requires maximum_workers=1.
  - Verification: Add TestExternalOperationalBoundsProjection in internal/app;
    run go test -count=1 ./internal/app ./internal/cli -run
    TestExternalOperationalBoundsProjection, the same command with -race,
    exercise go run ./cmd/revolvr config check and doctor --for for all three
    modes, and run go test -count=1 ./....

- [ ] EXT-23 — Enforce every unattended bound at its owning runtime boundary.
  - Acceptance: Table-driven production tests hit the exact limit for every
    EXT-22 category and prove no next task/action/process/output/notification/
    retention effect starts. Exhaustion publishes one typed terminal or
    terminal-for-now result with exact consumed/limit evidence and cannot be
    bypassed by restart, omission, or config drift. Level-2 admission refuses
    maximum_workers other than 1 even though internal parallel capability
    remains available for later approval.
  - Verification: Add TestUnattendedOperationalBoundEnforcement in
    internal/app; run go test -count=1 ./internal/app -run
    TestUnattendedOperationalBoundEnforcement, the same command with -race, and
    go test -count=1 ./....

- [ ] EXT-24 — Codify and verify the bounded stop/escalation policy.
  - Acceptance: Safety refusal, no progress, exhausted budgets, invalid task
    graph, lost lease, ambiguous Git result, corrupt history, and notification
    failure each map to one documented typed stop, quarantine, continuation, or
    operator-escalation action. Precedence is deterministic, evidence is
    immutable, and no prose/error-string heuristic can broaden authority.
  - Verification: Add TestExternalStopEscalationPolicy in internal/app; run
    go test -count=100 ./internal/app -run TestExternalStopEscalationPolicy,
    go test -race -count=1 ./internal/app -run
    TestExternalStopEscalationPolicy, cross-check the policy table in the
    operator documentation, and run go test -count=1 ./....

- [ ] EXT-25 — Persist automatic task quarantine for unprovable in-flight
  Level-2/3 work.
  - Acceptance: An unprovable supervisor, worker, verification, or Git boundary
    appends immutable unsafe_or_ambiguous operation evidence and a durable
    task-scoped quarantine bound to exact task/state/workspace/Git/config
    authority. Exact idempotent transitions still replay. Restart cannot
    heuristically resume, overwrite the old operation, spend another attempt,
    or let retry/unblock clear quarantine.
  - Verification: Add TestDurableAutonomousTaskQuarantine in
    internal/autonomoustaskrun; run go test -count=1
    ./internal/autonomoustaskrun -run TestDurableAutonomousTaskQuarantine, the
    same command with -race, and go test -count=1 ./....

- [ ] EXT-26 — Exclude unchanged quarantine while allowing unrelated safe
  queue work.
  - Acceptance: The shared scheduler and queue exclude the exact unchanged
    quarantined authority, do not repeatedly select or charge it, and do not
    let it trigger a daemon wake. After quarantine is durable, independent
    ready tasks continue in canonical order; dependency/conflict-related tasks
    remain blocked with exact reasons. Only successful EXT-13 reconciliation
    under a new operation identity removes the exclusion.
  - Verification: Add TestQueueContinuesAfterDurableTaskQuarantine in
    internal/autonomousqueue and TestQuarantineDoesNotSelfWake in
    internal/autonomousdaemon; run go test -count=1 ./internal/autonomousqueue
    ./internal/autonomousdaemon -run
    'Test(QueueContinuesAfterDurableTaskQuarantine|QuarantineDoesNotSelfWake)$',
    the same command with -race, and go test -count=1 ./....

- [ ] EXT-27 — Exercise real production task operations in the sequential
  queue integration path.
  - Acceptance: app.RunQueue uses maximum_workers=1 and the strict fake Codex
    to run real productionStepRunner task operations rather than injected
    terminal results. The test includes independent tasks, one dependency
    unlock after exact completion, deterministic reselection, conflict
    exclusion, cleanup, terminal-for-now yield, and exact ordered queue/task/
    ledger evidence. No parallel batch is admitted.
  - Verification: Add TestProductionSequentialQueueIntegration in internal/app;
    run go test -count=1 ./internal/app -run
    TestProductionSequentialQueueIntegration, go test -race -count=1
    ./internal/app -run TestProductionSequentialQueueIntegration, and
    go test -count=1 ./....

- [ ] EXT-28 — Prove interruption recovery for queue reconciliation.
  - Acceptance: Deterministic kill/restart injection covers before/during/after
    queue selection, task dispatch/return, ordered outcome reconciliation, and
    terminal queue publication. Restart uses the same stable queue/task
    operation identities and never duplicates a queue outcome, commit, attempt
    charge, completed task, or terminal queue event. A quarantined slot remains
    immutable while unrelated admitted work reconciles exactly once.
  - Verification: Add TestExternalAdministrativeInterruptionRecoveryMatrix in
    internal/app; run go test -count=1 ./internal/app -run
    TestExternalAdministrativeInterruptionRecoveryMatrix, the same command with
    -race, and go test -count=1 ./....

- [ ] EXT-29 — Keep attended and fully unattended execution authority
  operationally distinct and bind unattended acknowledgement exactly.
  - Acceptance: A CLI flag cannot turn dangerous approval/sandbox bypass into
    unattended authority. fully_unattended requires the exact effective
    execution/isolation policy acknowledgement and fails when any writable
    root, executable identity, hook policy, environment, credential/redaction,
    network rule, process/resource limit, or container profile identity
    changes. operator_attended remains explicit and cannot satisfy queue/daemon
    admission.
  - Verification: Add TestUnattendedAcknowledgementBindsEffectivePolicy in
    internal/autonomoussafety and TestCLIFlagCannotGrantUnattendedAuthority in
    internal/cli; run go test -count=1 ./internal/autonomoussafety
    ./internal/cli -run
    'Test(UnattendedAcknowledgementBindsEffectivePolicy|CLIFlagCannotGrantUnattendedAuthority)$',
    the same command with -race, and go test -count=1 ./....

- [ ] EXT-30 — Provide the repeatable rootless Linux OCI deployment profile.
  - Acceptance: deploy/rootless-oci contains an engine-independent policy plus
    one tested engine rendering. It mounts only the exact project/control root
    read-write and declared toolchain paths read-only; uses a read-only root
    filesystem, private PID namespace, bounded tmpfs and process/cgroup limits,
    no privilege/capability/device/host-home/engine-socket/agent-socket mounts,
    disabled validated Git hooks, replacement environment, declared redacted
    Codex credential injection, and externally enforced default-deny egress
    limited to the tested Codex endpoint set. Task dependencies are
    pre-provisioned and engine brand is not policy authority.
  - Verification: Run bash -n scripts/check-rootless-oci.sh and
    bash scripts/check-rootless-oci.sh --static; run its documented live
    render/inspect mode to assert every required mount, namespace, capability,
    device, environment, hook, resource, and network property; run the
    unattended runbook smoke without arbitrary task network.

- [ ] EXT-31 — Prove the approved isolation profile against outside access.
  - Acceptance: Under EXT-30, a hostile task/model/verification attempt cannot
    read or mutate an outside-project host sentinel, ambient secret, host home,
    container-engine socket, agent socket, or unrelated writable path.
    Disallowed network endpoints fail while the exact allowed Codex path works.
    Before/after host snapshots and proxy/network logs contain no unauthorized
    access or secret value.
  - Verification: Run bash -n scripts/test-rootless-oci-isolation.sh and
    bash scripts/test-rootless-oci-isolation.sh three times from fresh
    containers; compare complete sentinel snapshots, inspect mounts/
    capabilities/environment/process limits, verify denied and allowed egress
    logs, and scan all Revolvr evidence for the injected secret.

- [ ] EXT-32 — Document and test disk-capacity, retention, and export
  operations for external projects.
  - Acceptance: Guidance derives capacity from explicit output/retained-disk
    bounds and defines warning/refusal thresholds. A tested procedure exports
    and replay-validates ledger evidence, applies retention only from an exact
    plan, and preserves every artifact needed by active tasks, quarantine,
    recovery, receipts, completion, and archives. No completion auto-archives
    and no retention action becomes implicit run authority.
  - Verification: Execute the procedure in a fixture containing active,
    completed, quarantined, and archived work; run ledger export verify and
    replay-validate before/after retention; compare the required-evidence
    manifest; run go test -count=1 ./internal/artifactretention
    ./internal/ledgerexport, the same command with -race,
    go test -count=1 ./..., and git diff --check.

- [ ] EXT-33 — Document and prove at-least-once notification delivery.
  - Acceptance: examples/notification-receiver requires the stable delivery
    idempotency key and demonstrates receiver-side deduplication; the runbook
    documents it. A crash after receiver action but before local success may
    retry with the same key and is never described as exactly once.
    Notification attempts obey EXT-22 bounds, persist failure/escalation
    evidence, redact secrets, and never change the task/queue outcome.
  - Verification: Add TestNotificationAtLeastOnceReceiverIdempotency in
    internal/autonomousnotification; run go test -count=1
    ./internal/autonomousnotification -run
    TestNotificationAtLeastOnceReceiverIdempotency, the same command with
    -race, execute the receiver example through the crash window, and
    git diff --check.

- [ ] EXT-34 — Extend and smoke-test the operator runbook for Level 2.
  - Acceptance: The release-candidate runbook adds OCI profile setup,
    credential/redaction and network policy, explicit finite bounds,
    sequential queue start/monitor/cancel/restart, quarantine inspection and
    EXT-13 recovery, unrelated-work continuation, notification behavior, and
    retention/export. It states maximum_workers=1, no automatic integration,
    no automatic archive, Linux-only unattended support, and daemon remains
    unapproved.
  - Verification: Run bash -n scripts/smoke-external-queue.sh and execute
    scripts/smoke-external-queue.sh in the approved container against a
    disposable candidate fixture; verify every expected safe stop and recovery
    output, and run git diff --check.

- [ ] EXT-35 — Execute the quantitative Level-2 bounded-queue soak.
  - Acceptance: After Level-1 approval, the exact Level-2 candidate first
    repeats EXT-18 reproducible-build/vulnerability evidence and EXT-19 remote
    CI evidence for its own commit. It then completes at least three sequential
    bounded queue operations totaling at least 20 real task operations across
    at least two external repositories. Evidence includes dependency unlock,
    terminal-for-now yield, caller cancellation, queue restart, and unrelated-
    work continuation after one durable quarantine. All runs use EXT-30 and
    maximum_workers=1, with zero containment, duplication, evidence-loss,
    manual-state-edit, or unclassified-ambiguity failures.
  - Verification: Validate and total the immutable dogfood manifests; inspect
    every queue/task/ledger/receipt/quarantine identity and resource bound;
    compare outside sentinels and network/secret logs; run
    scripts/dogfood-external-level2.sh --verify-manifest <manifest> for every
    manifest; rerun affected scenarios after any material authority change.

- [ ] EXT-36 — Tag and record the Level-2 external-use decision.
  - Acceptance: With explicit operator authorization, an immutable v0.x.y tag
    names the exact CI- and soak-tested Level-2 commit and artifacts. The
    decision approves only Linux rootless-OCI sequential bounded queues,
    records the isolation-policy SHA-256 and exact Codex identity, and remains
    rejected if any Level-2 gate or finite bound is missing.
  - Verification: Compare tag, remote CI, build, profile, and soak hashes;
    inspect every nonblank decision field and exception; do not mark complete
    without explicit tag/publish authorization.

### Level 3 — Unattended Daemon

- [ ] EXT-37 — Prove daemon wake authority excludes Revolvr's own mutations.
  - Acceptance: Ledger, queue, receipt, cache, notification, retention, lock,
    and other Revolvr runtime mutations cannot change the stable wake
    fingerprint or start a sweep. A canonical task/state/archive/child
    authority change produces one externally caused stable wake after debounce.
    Quarantined unchanged authority remains excluded and wake evidence is
    bounded.
  - Verification: Add TestDaemonIgnoresSelfMutationsAndWakesOnAuthority in
    internal/autonomousdaemon; run go test -count=20
    ./internal/autonomousdaemon -run
    TestDaemonIgnoresSelfMutationsAndWakesOnAuthority, go test -race -count=1
    ./internal/autonomousdaemon -run
    TestDaemonIgnoresSelfMutationsAndWakesOnAuthority, and
    go test -count=1 ./....

- [ ] EXT-38 — Prove daemon interruption, restart, and bounded-sweep recovery.
  - Acceptance: Deterministic tests interrupt an idle wait, an admitted active
    sweep, post-queue reconciliation, and daemon-wake publication. Restart
    preserves exact queue/task operation identities, starts no duplicate
    effect, respects daemon-sweep/process/task bounds, and either continues
    exact replay or records typed unsafe_or_ambiguous quarantine. Two daemon
    processes cannot own the same authority concurrently.
  - Verification: Add TestProductionDaemonInterruptionRecovery in internal/app;
    run go test -count=1 ./internal/app -run
    TestProductionDaemonInterruptionRecovery, the same command with -race, and
    go test -count=1 ./....

- [ ] EXT-39 — Extend and smoke-test the operator runbook for Level 3.
  - Acceptance: The runbook adds foreground daemon start, monitoring,
    cancellation, clean restart, forced-interruption recovery, idle/wake/sweep
    evidence, finite sweep/resource bounds, and the rule that service-manager
    installation or remote orchestration is outside initial approval. Every
    command is verified against the exact Level-3 release candidate under
    EXT-30.
  - Verification: Run bash -n scripts/smoke-external-daemon.sh and execute
    scripts/smoke-external-daemon.sh through idle, external wake, bounded
    sweep, cancellation, and restart; compare documented output with candidate
    output, and run git diff --check.

- [ ] EXT-40 — Execute the quantitative Level-3 daemon soak.
  - Acceptance: After Level-2 approval, the exact Level-3 candidate first
    repeats EXT-18 reproducible-build/vulnerability evidence and EXT-19 remote
    CI evidence for its own commit. It then runs continuously for at least 72
    hours in the approved container profile, records at least 10 externally
    caused stable wakes, completes two clean daemon restarts, and survives one
    forced interruption during an active sweep. There are zero containment
    violations, duplicate effects/charges, lost terminal evidence, manual
    runtime-state edits, self-caused wakes, or unclassified ambiguity.
  - Verification: Validate the continuous time interval and all immutable wake,
    sweep, queue, task, ledger, receipt, quarantine, restart, resource, network,
    and sentinel evidence; run scripts/dogfood-external-level3.sh
    --verify-manifest <manifest>; independently count thresholds; rerun
    affected scenarios after any material authority change.

- [ ] EXT-41 — Tag and record the Level-3 external-use decision.
  - Acceptance: With explicit operator authorization, an immutable v0.x.y tag
    names the exact CI- and soak-tested Level-3 commit and artifacts. The
    decision approves only the documented Linux rootless-OCI foreground daemon
    profile and names all remaining exclusions; any blank field, expired
    exception, missing bound, or failed soak keeps the decision rejected.
  - Verification: Compare tag, remote CI, build, Codex allowlist, isolation
    policy, runbook, and 72-hour soak identities; inspect the complete decision
    record; do not mark complete without explicit tag/publish authorization.

## Completed Audit Tasks

- [x] AUDIT-FIX-01 — Settle source-lease monitor failures before terminal task,
  receipt, and ledger mutation in mixed and autonomous runs; make the race tests
  deterministic.
- [x] AUDIT-FIX-02 — Remove unsafe mixed-pass dirty-worktree commits or isolate
  and prove the exact run-owned delta with real-Git overlap coverage.
- [x] AUDIT-FIX-03 — Harden source-writer and retention lock files with
  canonical roots, no-follow opens, and named/opened inode checks.
- [x] AUDIT-FIX-04 — Migrate the remaining predictable coordinator locks to the
  same hardened flock primitive and substitution tests.
- [x] AUDIT-FIX-05 — Open all app read projections through the live read-only
  ledger API and prove byte/sidecar immutability.
- [x] AUDIT-FIX-06 — Declare and enforce the supported-platform contract with a
  matching cross-build matrix.
- [x] AUDIT-FIX-07 — Repair and rerun the stale local CLI smoke-test header
  assertion.
- [x] AUDIT-CLOSE-01 — Re-audit every `AUDIT_PROBLEMS.md` finding against the
  committed fixes, run the final verification matrix, and delete the audit
  file only if all findings are resolved.
- [x] AUDIT-R3-00 — Conduct an independent wide-sweep audit and record only
  reproduced or directly evidenced findings in `AUDIT_PROBLEMS.md`.
- [x] AUDIT-R3-01 — Settle runner process groups after successful leader exit
  and prove that no descendant can mutate after return.
- [x] AUDIT-R3-02 — Make initialization and task-directory creation no-follow
  and identity-safe without breaking legitimate linked worktrees.
- [x] AUDIT-R3-03 — Retain SQLite busy evidence across live-reader retries and
  make the cancellation regression deterministic.
- [x] AUDIT-R3-04 — Make task-import and receipt structural parsing ignore
  headings inside Markdown fences.
- [x] AUDIT-R3-05 — Make Git object-ID validation consistent for SHA-1 and
  SHA-256 repositories across workspace, cache, map, and dossier boundaries.
- [x] AUDIT-R3-06 — Accept safe tracked names beginning with `..` while still
  rejecting path traversal.
- [x] AUDIT-R3-07 — Replace map-order-dependent validation diagnostics with
  explicit deterministic ordering.
- [x] AUDIT-R3-08 — Remove the confirmed no-caller wrappers and obsolete
  admitted-cycle orchestration path if it is not intentionally reserved.
- [x] AUDIT-R3-CLOSE-01 — Re-audit AP-01 through AP-08 against committed
  source and regressions, run the final verification matrix, and delete
  `AUDIT_PROBLEMS.md` only when every finding is proven closed.
- [x] AUDIT-R4-00 — Conduct a fresh wide-sweep audit and record only
  reproduced or directly evidenced findings in `AUDIT_PROBLEMS.md`.
- [x] AUDIT-R4-01 — Make the shared runtime-path boundary descriptor-rooted
  across ancestor substitution and migrate canonical autonomous-state
  persistence with the reproduced outside-rename regression.
- [x] AUDIT-R4-02 — Migrate notification intent, payload, history, journal,
  cleanup, sync, and lease checks to the stable runtime-path boundary.
- [x] AUDIT-R4-03 — Repair task-run protected reads and bind history/checkpoint
  publication and cleanup to stable parent and file identities.
- [x] AUDIT-R4-04 — Migrate autonomous archive immutable/mutable storage and
  removal away from its bespoke check-then-use path helpers.
- [x] AUDIT-R4-05 — Bind autonomous finalization artifact publication and
  readback to stable parent, temporary-file, and destination identities.
- [x] AUDIT-R4-06 — Inventory and migrate the remaining authoritative
  `Lstat`-then-by-name evidence readers identified by AP-01.
- [x] AUDIT-R4-07 — Poll for bounded process-group settlement after `SIGKILL`
  and close cancellation/identity-reuse races in the runner.
- [x] AUDIT-R4-08 — Make active TUI quit wait for the matching run, loop,
  task-run, or queue terminal event before exiting.
- [x] AUDIT-VERIFY-01 — Repair the stale missing-checkpoint diagnostic
  assertion left by the protected receipt-read migration.
- [x] AUDIT-R4-09 — Replace recursive map-order Codex usage selection with
  schema precedence and fail-closed ambiguity handling.
- [x] AUDIT-R4-10 — Bind source-snapshot bytes to the opened file identity and
  reject regular-file and symlink ABA substitutions.
- [x] AUDIT-R4-11 — Make action-budget and archive-file first-error diagnostics
  deterministic under multiple simultaneous failures.
- [x] AUDIT-R4-CLOSE-01 — Re-audit AP-01 through AP-06 against committed
  source and regressions, run the final verification matrix, and delete
  `AUDIT_PROBLEMS.md` only when every finding is proven closed.

## Completed Programs

- [x] AW-01 through AW-31 — autonomous workflow contracts, execution,
  persistence, safety, worktree isolation, finalization, archives, queues,
  retention, evidence views, notifications, metrics, and bounded parallelism.
- [x] AUD-01 through AUD-16 — wide-sweep correctness, persistence, process,
  path, configuration, Git, ledger, and cleanup hardening.
- [x] R2-01 through R2-11 — second audit closure for logical ledger authority,
  exclusion, immutable recovery, protected runtime paths, durability, replay,
  CLI, and configuration contracts.
- [x] TS-01 through TS-04 — one shared deterministic scheduler across mixed,
  autonomous, app, CLI, TUI, queue, and run-once surfaces.
- [x] FM-01 — strict canonical frontmatter with inert `x-` extensions.
- [x] OC-01 through OC-02 — operator-checkpoint receipt authority,
  replay-safe fulfillment, scheduling, and visibility.
- [x] AM-01 through AM-02 — deterministic autonomous migration planning,
  atomic application, and restart recovery.
- [x] QA-01 — cross-surface regression closure, operator documentation, and
  read-only Cyber-ARPG readiness assessment.
- [x] AUDIT-2026-07-14 — evidence-based wide-sweep audit recorded in
  `AUDIT_PROBLEMS.md`; six problems found and converted into bounded follow-up
  tasks.

Detailed behavior is captured in `.agent/DECISIONS.md`; current verification
and readiness evidence is summarized in `.agent/STATE.md`; implementation and
test history is preserved in Git.

## Deferred

- The legacy EXT external-readiness backlog remains historical/deferred and is
  not the fresh-pass selector while the active architecture sequence runs.
- Architecture 025 recorded Graphiti deferral. Re-evaluate only on the exact
  evidence trigger in `docs/architecture/memory-graphiti-phase-gate.md`.
