# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, record the blocker and stop.
- Add only small, specific, directly discovered follow-up tasks.
- Do not invent broad roadmap work.

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

- [ ] EXT-19 — Push the exact Level-1 candidate and obtain remote CI evidence.
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

## Blocked

None.
