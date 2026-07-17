# Autonomous External-Project Readiness

Status: working draft

Decision: **not yet approved for autonomous use in external projects**
Last updated: 2026-07-15

## Purpose

This document defines the minimum evidence required before Revolvr may run
autonomously in a project other than its own development repository. It is a
release gate, not a feature wishlist. An item belongs here only when its
absence could make an unattended run unsafe, unrecoverable, misleading, or
operationally untrustworthy.

The initial target is a local repository controlled by the operator. Hosted
service operation, remote orchestration, multi-tenant execution, and automatic
push or merge are outside this gate.

## Readiness Levels

Revolvr has three distinct operating levels. Approval at one level does not
imply approval at the next.

1. **Attended single task**: an operator starts one exact autonomous task,
   watches the run, and can respond to a safe stop.
2. **Unattended bounded queue**: Revolvr runs a bounded set of already-admitted
   tasks without an operator watching each cycle.
3. **Unattended daemon**: Revolvr waits for authority changes and starts new
   bounded queue sweeps without an operator initiating each sweep.

The current code is closest to level 1. Levels 2 and 3 remain unapproved until
every applicable gate below has passing evidence.

## Fixed Decisions For Task Decomposition

The following policy decisions are settled. Follow-up task authors should
implement and verify them rather than reopen them unless direct contradictory
evidence is discovered.

### Approval sequence and initial scope

- External-project approval is incremental: level 1 first, then level 2, then
  level 3. A single release may not skip a level.
- The first external-project release targets operator-controlled Git
  repositories. Bare repositories and repositories with active Git submodules
  are unsupported and must be refused by preflight.
- Level 1 retains the existing Linux, macOS, and FreeBSD application support.
  Levels 2 and 3 initially support Linux only because their approved external
  isolation profile is Linux-specific.
- The first level 2 approval is sequential: `maximum_workers` must equal `1`.
  Parallel external-project execution is a later, separately approved
  capability and is not required for the first bounded-queue release.
- Revolvr never pushes, merges, rebases, resets, cleans, or stashes external
  project work. It produces reviewable task-workspace commits; integration
  into an operator branch remains an explicit external action.
- Completion does not automatically archive a task. Archive creation remains
  a separate explicit administrative operation.

### Interrupted in-flight work

- Revolvr will not attempt to infer or automatically resume an in-flight model
  cycle whose exact external effects cannot be proven.
- Durable transitions that are already exact and idempotent continue to replay
  normally. An unprovable in-flight supervisor, worker, verification, or Git
  boundary quarantines only the affected task and records terminal
  `unsafe_or_ambiguous` evidence for that operation.
- A queue or daemon may continue unrelated tasks only after the quarantine is
  durable and the scheduler excludes the unchanged quarantined authority.
  It must not repeatedly wake or spend attempts on that task.
- Recovery requires a new explicit operator command. It must inspect and
  reconcile task, state, workspace, Git, ledger, receipt, and artifact
  authority; preserve the old operation as immutable evidence; and create a
  new operation identity only after reconciliation succeeds. Existing generic
  retry commands must not silently clear autonomous quarantine.
- Level 1 may stop and wait for this operator action. Levels 2 and 3 must
  quarantine automatically and continue or wait deterministically without
  manual runtime-file edits.

### Initial unattended isolation profile

- The first approved unattended deployment is a rootless OCI container on
  Linux. Container-engine brand is not release authority; the enforced
  properties below are.
- Mount only the exact project/control root read-write plus explicitly required
  read-only toolchain material. Do not mount the host home directory, parent
  source tree, container-engine socket, SSH/GPG agent sockets, unrelated
  credentials, or additional writable host paths.
- Use a read-only container root filesystem, a private PID namespace, a
  bounded temporary filesystem, no privileged mode, no added capabilities,
  no host devices, and a process/cgroup limit sufficient for bounded
  descendant cleanup.
- Disable Git hooks for the admitted repository by exact configuration and
  validate that policy before model or verification work.
- Replace the environment with an explicit allowlist. The Codex credential is
  injected only under a declared name covered by Revolvr redaction; ambient
  host environment variables are not inherited.
- Default-deny egress is externally enforced. The initial profile permits only
  the exact Codex/OpenAI endpoint set required by the tested CLI through an
  operator-controlled proxy or equivalent allowlist. Task and verification
  dependencies must be pre-provisioned; arbitrary task-requested network
  access is outside the initial approval.
- The release test must prove that an outside-project host sentinel, ambient
  secret, container-engine socket, and disallowed network endpoint remain
  unavailable. Configuration declarations without these external tests do not
  satisfy the isolation gate.

### Preflight contract

- Add mode-aware read-only preflight using `doctor --for
  attended-task|queue|daemon`; an optional exact task selector may further
  narrow task-bound checks. Bare `doctor` remains the general attended-task
  preflight for compatibility.
- Preflight validates every locally knowable prerequisite used before the
  requested mode starts model work, including task graph, path modes and
  identities, Git shape and cleanliness, supported platform, Codex/Git
  executable identity, verification presence, finite unattended bounds, and
  isolation acknowledgement.
- Execution always rechecks authority. Preflight is a current snapshot, not a
  transferable lease or a promise that later external commands will succeed.
- `doctor`, `status`, and no-model admission must agree on shared authority;
  mode-specific checks may add stricter failures but may not weaken shared
  checks.

### Codex and build compatibility

- Each Revolvr release supports an explicit allowlist of tested Codex CLI
  version strings and resolved executable SHA-256 identities. Autonomous
  external-project admission refuses an unlisted version or executable.
- The first release may list exactly one Codex CLI build. Adding a version
  requires the production fake-Codex contract suite plus one isolated live
  dogfood scenario; semantic version-range assumptions are not sufficient for
  a pre-1.0 CLI.
- Go 1.22 remains the source-language compatibility floor and is tested in CI.
  Release binaries are built with a currently supported, patched Go toolchain,
  record that exact toolchain version, and have no reachable standard-library
  or module vulnerability reported by the release scan.
- An approved release is an immutable `v0.x.y` tag and exact source commit with
  reproducible build instructions. A moving branch or unpushed local commit is
  never installation authority.

### Unattended budgets and policy

- Levels 2 and 3 require explicit finite positive limits for task attempts,
  each action, elapsed time, model tokens, cycles per task, tasks per queue,
  daemon sweeps, process duration, output bytes, retained disk bytes, and
  notification attempts when notifications are enabled.
- Unlimited token or time budgets are invalid for unattended admission. No
  hidden hard-coded fallback may convert a missing unattended bound into
  authority.
- Every bound is part of the effective configuration fingerprint and is shown
  by `config check` and mode-aware `doctor`.
- Level 1 may use documented defaults, but the effective values must still be
  visible and recorded with run evidence.

Level 1 uses these finite attended defaults: 16 task attempts; 4 attempts for
each of `audit`, `correct`, `document`, `implement`, `plan`, and `simplify`; 4
hours elapsed time; 1,000,000 model tokens; 50 cycles per task; 30 minutes per
Codex or verification process; 256 KiB per output stream; and 1 GiB of retained
artifacts per operation. The effective process and output bounds rise to the
largest configured subprocess timeout and stream cap. When notifications are
enabled, their required positive `maximum_attempts` is the effective
notification bound; disabled notifications have a bound of zero.

### Required soak thresholds

- **Level 1:** at least 10 real-Codex task operations across at least two
  external repositories, including five successful source changes and the
  production scenarios for verification failure/correction, needs input,
  cancellation/restart, and safety refusal.
- **Level 2:** after level 1 approval, at least three sequential bounded queue
  operations totaling at least 20 task operations across at least two external
  repositories. Evidence must include dependency unlock, terminal-for-now
  yield, caller cancellation, queue restart, and unrelated-work continuation
  after one quarantined task.
- **Level 3:** after level 2 approval, at least 72 continuous hours in the
  approved container profile, at least 10 externally caused stable wakes,
  two clean daemon restarts, and one forced interruption during an active
  sweep.
- All levels require zero containment violations, duplicate commits or attempt
  charges, lost terminal evidence, manual runtime-state edits, or unclassified
  ambiguous outcomes. Expected typed safe stops do not fail the soak when
  their evidence and recovery behavior match policy.
- Any material change to process lifecycle, path authority, persistence,
  recovery, Codex invocation, isolation, or commit behavior invalidates the
  affected soak evidence and requires the relevant scenarios to be rerun.

## Absolute Go/No-Go Gates

### 1. Exercise the production autonomous composition end to end

- [ ] Add an integration harness that enters through the real app or CLI
  autonomous command and uses the production `productionStepRunner` wiring.
  Supplying a replacement `autonomoustaskrun.StepRunner` does not satisfy this
  gate.
- [ ] Use a strict fake Codex executable to prove the complete happy path:
  workspace creation, supervisor decision, worker action, attempt admission
  and completion, verification, source commit, checkpoint advancement, audit,
  completion authorization, frozen evidence, canonical task/state completion,
  and terminal ledger completion.
- [ ] Prove the production correction and clean re-audit path.
- [ ] Prove `needs_input`, authorized block, verification failure, no-progress,
  safety stop, cancellation, and maximum-cycle outcomes through the same
  production entry point.
- [ ] Assert exact source, task, state, Git, receipt, ledger, workspace, and
  artifact outcomes after every terminal case.
- [ ] Add a queue integration that runs real production task operations rather
  than injected terminal results, including at least one dependency unlock.
  A safe parallel batch belongs to the later parallel-execution approval, not
  the initial sequential level 2 gate.

Required evidence:

- A deterministic integration test or smoke script committed to the repository.
- Ordinary and race execution of that test in CI.
- No live model or network dependency in the mandatory CI form.

### 2. Make preflight authority agree with execution authority

- [ ] `revolvr doctor`, `status`, task loading, and autonomous admission must
  agree on whether repository and harness paths are safe.
- [ ] Add a regression for the reproduced case where `doctor` reports
  `Ready: true` while `status` rejects a group-writable `.agent` directory.
- [ ] Preflight must validate the canonical task graph and the protected paths
  required by the requested run mode without mutating the repository.
- [ ] A passing preflight must not guarantee that model work will succeed, but
  it must guarantee that every locally checkable admission prerequisite used
  before model start currently passes.

Required evidence:

- Cross-surface tests for safe, missing, malformed, symlinked, aliased,
  group-writable, and substituted task/runtime paths.
- A fresh initialized fixture in which `doctor`, `status`, and a no-model
  admission probe all agree.

### 3. Define and prove interruption recovery

- [ ] Write the supported recovery contract for interruption before, during,
  and after supervisor, worker, verification, commit, checkpoint, audit,
  finalization, queue reconciliation, notification, and archive publication.
- [ ] Implement the settled quarantine contract for an in-flight task cycle
  that stops as `unsafe_or_ambiguous` without exact reconciliation.
- [ ] Level 1 must expose the explicit operator recovery procedure. Levels 2
  and 3 must durably quarantine the task, prevent unchanged reselection and
  self-wake, and permit unrelated safe work to continue.
- [ ] Prove recovery after process kill and machine-style interruption at every
  durable transition seam used by the production integration path.
- [ ] Prove that recovery never duplicates a commit, attempt charge, external
  notification success, completed task, or queue outcome.

Required evidence:

- Failure-injection matrix with restart from the same stable operation ID.
- An operator runbook for every state that intentionally requires human action.

### 4. Require real isolation for unattended execution

- [ ] Keep `operator_attended` and `fully_unattended` operationally distinct.
  Dangerous Codex approval/sandbox bypass must not become unattended authority
  merely because a command-line flag was supplied.
- [ ] Implement and document the settled rootless Linux OCI isolation profile:
  filesystem/process boundaries, default-deny egress, replacement environment,
  disabled hooks, executable identities, and secret injection/redaction.
- [ ] Prove the fully-unattended acknowledgement is bound to the exact effective
  isolation and execution policy and becomes invalid when material authority
  changes.
- [ ] Run an adversarial test showing that the model cannot read or mutate a
  protected outside-project sentinel under the approved deployment profile.
- [ ] Run an adversarial test showing that disallowed network access and ambient
  secrets are unavailable under that profile.

Revolvr's linked Git worktree and Codex workspace sandbox are not sufficient
external isolation. Until a deployment profile supplies these controls, only
operator-attended use is approved.

### 5. Prove branch, commit, and repository containment

- [ ] Autonomous work must occur on an exact disposable or task-scoped branch
  and linked workspace whose control-root relationship is recorded.
- [ ] Prove that no run pushes, merges, rebases, resets, cleans, stashes, or
  mutates unrelated worktrees unless a future explicit workflow separately
  authorizes that operation.
- [ ] Prove every generated commit contains only the admitted run-owned delta
  and required task metadata.
- [ ] Prove dirty, staged, ignored, linked-worktree, SHA-256 Git, and concurrent
  external-commit cases either preserve exact authority or stop before
  publication. Active submodules are initially unsupported and must be
  refused before publication.
- [ ] Document how an operator reviews, accepts, rejects, and removes a task
  workspace after every terminal outcome.

### 6. Establish release and CI authority

- [ ] Push the complete candidate revision and run remote CI on that exact
  commit. Local-only commits are not release evidence.
- [ ] Gate the supported Go floor, `go test ./...`, production autonomous
  integration, `go test -race`, `go vet`, `go mod verify`, fake-Codex smoke
  tests, and supported-platform builds.
- [ ] Run `govulncheck ./...` and record any unreachable findings separately
  from reachable vulnerabilities. A reachable vulnerability blocks release.
- [ ] Publish a versioned build or reproducible build procedure that records
  Revolvr version, Go version, target platform, and source commit.
- [ ] Publish the exact tested Codex CLI version/executable allowlist and test
  invocation/output-schema behavior against every listed identity.
- [ ] Tag the first approved external-use release. An untagged moving `main`
  branch is not sufficient authority for unattended installation.

### 7. Complete external-project dogfood and soak

- [ ] Run the candidate against a disposable external fixture using the real
  Codex CLI and the exact approved configuration.
- [ ] Meet the settled level-specific repository, operation, scenario, and
  duration soak thresholds.
- [ ] Exercise a successful change, verification failure, correction, needs
  input, cancellation, and restart without hand-editing runtime state.
- [ ] After level 1 passes, run a bounded queue with independent and dependent
  tasks. Confirm deterministic selection, conflict exclusion, cleanup, and
  exact terminal evidence.
- [ ] Before level 3 approval, operate the foreground daemon through repeated
  idle/wake/sweep cycles and at least one process restart.
- [ ] Record source HEAD/status, outside-project sentinels, runtime state,
  ledger validation, receipts, task/state history, artifacts, and resource use
  before and after every scenario.

Dogfood evidence must come from the release candidate. Historical runs made
before material process, path, or persistence hardening do not close this gate.

### 8. Set bounded operational policy

- [ ] Supply explicit task, action, elapsed-time, token, cycle, queue-task,
  worker, output, disk-retention, and notification-retry bounds for unattended
  use. Defaults must be documented and visible in `config check`.
- [ ] Define the stop/escalation policy for safety, no progress, exhausted
  budgets, invalid task graphs, lost leases, ambiguous Git outcomes, corrupt
  history, and notification failure.
- [ ] Ensure a daemon cannot wake itself from its own ledger, queue, receipt,
  cache, notification, or retention mutations.
- [ ] Provide disk-capacity guidance and a tested retention/export procedure
  that preserves evidence required for active work and recovery.
- [ ] Define notification delivery as at-least-once with receiver-side
  idempotency; do not claim exactly-once external effects.

### 9. Provide an external-project operator runbook

- [ ] Installation and version pinning.
- [ ] Repository initialization and required path permissions.
- [ ] Configuration, verification commands, isolation profile, secrets, and
  network policy.
- [ ] Task creation, migration, scheduling, checkpoint, and input workflows.
- [ ] Starting, monitoring, cancelling, and restarting single runs, queues, and
  the daemon.
- [ ] Inspecting receipts, ledger history, metrics, workspaces, completion
  capsules, notifications, and archives.
- [ ] Recovering every intentional manual-stop state.
- [ ] Accepting or rejecting generated commits without pushing automatically.
- [ ] Exporting evidence, applying retention, upgrading, and removing Revolvr
  runtime state safely.

The runbook must use commands verified against the release candidate rather
than architecture-only descriptions.

## Initial Known Blockers

| Blocker | Severity | Current evidence | Closure evidence |
| --- | --- | --- | --- |
| Production autonomous app composition is uncovered | Release blocker | Production step/coordinator helpers currently report 0% statement coverage | Production-entry integration matrix passes ordinarily and under race |
| Current audit/hardening revision is local-only | Release blocker | Local `HEAD` is ahead of remote `main`; remote CI has not evaluated it | Exact candidate commit is pushed and all required CI gates pass |
| Doctor can report ready when status refuses `.agent` permissions | High | Reproduced with `.agent` mode `0775` | Cross-surface regression and consistent refusal/readiness result |
| In-flight cycle restart has no durable task quarantine and explicit recovery command | High for levels 2–3 | Restart intentionally stops `unsafe_or_ambiguous`, but unchanged authority has no complete external-use quarantine workflow | Durable exclusion, unrelated-work continuation, immutable old operation, and explicit reconciliation/new-operation evidence |
| No current live autonomous dogfood evidence | Release blocker | Historical ledger proves mixed-pass runs only | Candidate external-project scenario matrix and soak record |
| Approved external isolation profile does not exist | Release blocker for levels 2–3 | Revolvr validates declarations but does not create a host/container/network boundary | Repeatable deployment profile plus outside-sentinel, network, and secret tests |

## Work That Is Not an Initial Release Blocker

The following may improve the project, but should not delay the first attended
external-project trial unless new evidence connects them to correctness or
safety:

- General refactoring of large CLI, TUI, or state files.
- Binary-size reduction.
- TUI control for archive, retention, daemon installation, or configuration.
- Higher queue parallelism than the existing bounded maximum.
- Broad performance tuning before real autonomous metrics identify a bottleneck.
- Hosted control plane, remote API, automatic push, or automatic merge.

## Recommended Closure Order

1. Fix preflight/execution readiness agreement.
2. Build the production autonomous fake-Codex integration harness.
3. Define interruption recovery and add its failure-injection matrix.
4. Push the candidate and strengthen mandatory CI gates.
5. Create the attended external-project runbook and isolation profile.
6. Complete attended external-project dogfood.
7. Approve level 1 or record exact remaining blockers.
8. Complete bounded-queue recovery and soak before approving level 2.
9. Complete isolation and daemon soak before approving level 3.

## Release Decision Record

For each proposed external-use release, record:

- Candidate source commit:
- Version/tag:
- Supported Go version(s):
- Supported Codex CLI version(s):
- Supported operating systems:
- Approved readiness level:
- Isolation profile and policy SHA-256:
- CI run/reference:
- Dogfood evidence reference:
- Open exceptions and approving authority:
- Decision: approved / rejected
- Decision time (UTC):

No blank field is implicit approval. Exceptions must identify their bounded
impact, compensating control, owner, and expiration or removal condition.
