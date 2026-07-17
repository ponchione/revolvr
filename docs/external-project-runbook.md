# Attended External-Project Operator Runbook

Status: Level-1 release-candidate procedure. This document is not an approval
record. Use it only with an immutable release whose decision record explicitly
approves **attended single-task external use**. The current source tree, a
moving branch, a locally built `dev` binary, or a green local test is not
release authority.

This runbook covers one operator-attended `autonomous-v1` task at a time on
Linux, macOS, or FreeBSD. `run --queue`, `run --daemon`, `doctor --for queue`,
and `doctor --for daemon` are **unapproved**. Do not use them for external
projects until separate Level-2 or Level-3 approval exists.

## Safety model and operator responsibilities

Keep a human operator present from preflight through terminal evidence review.
`operator_attended` deliberately leaves the following with that operator:

- approving the exact task, model-visible repository, verification commands,
  Git hooks, environment, credentials, and network access;
- watching progress and stopping unexpected filesystem, process, credential,
  hook, or network behavior;
- ensuring the control repository and every task workspace remain dedicated
  to Revolvr while an operation is active;
- reviewing every generated commit before integration; and
- preserving evidence and choosing explicit recovery when a result is
  uncertain.

A linked Git worktree separates task source and branches. It is not a security
sandbox and does not confine file reads, child processes, hooks, environment,
credentials, or network traffic. Dangerous Codex approval/sandbox bypass is
enabled by default and is visible in preflight. Disable it in configuration if
that is not acceptable, but do not represent that flag alone as unattended
isolation.

Revolvr does not push, merge, rebase, reset, clean, or stash external project
work. It does not automatically integrate a task commit, remove a task
workspace, retain/prune evidence, or archive a completed task.

## Install one pinned release

Start from the approved release decision record. It must name the immutable
tag, source commit, target platform, exact Revolvr artifact SHA-256, patched Go
toolchain, and the one release-authorized Codex version and executable
SHA-256. Install by hash into an operator-owned directory that is not writable
by other users. Keep the prior approved binary until upgrade validation is
complete.

Run these non-destructive checks and compare every value with that decision
record. `sha256sum` is the Linux spelling; use `shasum -a 256` on macOS or
`sha256 -q` on FreeBSD and compare the same lowercase digest.

```bash
revolvr --version
sha256sum "$(command -v revolvr)"
go version -m "$(command -v revolvr)"
codex --version
sha256sum "$(command -v codex)"
```

Reject `revolvr dev`, an unexpected VCS revision/toolchain, a moving symlink,
an unlisted Codex version, or any digest mismatch. `config check` is diagnostic
and may display an unlisted Codex identity; only a passing attended `doctor`
grants current preflight authority.

## Prepare and initialize the repository

The initial scope is an operator-controlled, non-bare Git worktree with at
least one commit, no active submodules, and a clean index/worktree. Do not use
a bare repository, active submodule, shared writable checkout, or repository
whose control paths contain symlinks, hard-linked protected files, unsafe file
types, or group/other-writable components.

```bash
git rev-parse --show-toplevel
git rev-parse --is-bare-repository
git submodule status --recursive
git status --short --branch
```

The bare result must be `false`, submodule output must be empty, and status
must contain only its branch header. From the exact top level, set a restrictive
creation mask and initialize:

```bash
umask 0022
revolvr init
```

Initialization creates local ignored runtime state under `.revolvr/`, seeds
repository-authored profiles under `.agent/profiles/`, creates
`.agent/tasks/`, initializes the ledger, and adds `/.revolvr/` to the Git
common `info/exclude`. It does not create `.revolvr/config.yaml`, migrate
tasks, commit, or push. Review and commit intended `.agent` additions through
the normal project process.

Directories must not be group/other writable; protected files must be regular,
single-link, non-symlink files without group/other write bits. This diagnostic
prints unsafe writable entries and must print nothing:

```bash
find .agent .revolvr -xdev -perm /022 -print
```

Do not blindly `chmod -R` an unknown tree. First establish ownership and exact
path identity, remove aliases/symlinks through an operator-reviewed procedure,
then correct only the known component. Re-run `init`, `status`, and `doctor`
after any permission repair.

## Configure attended execution and verification

Create `.revolvr/config.yaml` as an owner-controlled regular file. Use the
absolute release-authorized Codex path, the intended Git executable, and
verification commands that are safe to run repeatedly in the isolated task
workspace. A minimal attended configuration is:

```yaml
codex:
  executable: /absolute/path/to/release-authorized/codex
  model: gpt-5.6-sol
  reasoning_effort: xhigh
  ephemeral: true
  dangerously_bypass_approvals_and_sandbox: true
  timeout_seconds: 1800
git:
  executable: git
  timeout_seconds: 30
verification:
  missing_policy: fail
  commands:
    - name: go
      args: ["test", "./..."]
      timeout_seconds: 1800
commit:
  allow_pre_existing_dirty: false
  allow_missing_verification: false
  timeout_seconds: 30
autonomy:
  schema_version: revolvr-autonomous-safety-declaration-v1
  mode: operator_attended
  external_isolation:
    expectation: none
    enforcement: none
  network:
    access: unknown
    enforcement: none
  hooks:
    policy: operator_attended
  environment:
    inherit_host: true
  redaction:
    schema_version: revolvr-secret-redaction-policy-v1
    environment_variables: []
```

Replace the verification command with the project's deterministic, bounded
test entry point. Omitting `verification.commands` in a Go repository
synthesizes `go test ./...`; explicit `commands: []` disables that default and
fails external admission. A task cannot start without verification authority.
Never put secret values in YAML. If a named environment secret is needed, add
only its variable name to `autonomy.redaction.environment_variables`; the
operator remains responsible for the inherited environment and network in
attended mode.

Check the normalized configuration and current attended admission:

```bash
revolvr config check
revolvr doctor --for attended-task
```

`doctor` must end in `Ready: true`. It validates current paths, repository
shape/cleanliness, platform, graph, verification, finite bounds, Codex/Git
executable identities, and mode. It is read-only and is not a lease: execution
rechecks all authority. Narrow it to the exact task immediately before a run:

```bash
revolvr doctor --for attended-task --task "$TASK_ID"
```

### Attended defaults and durable visibility

Level 1 may use finite defaults, but no default is hidden. `config check`
prints the effective configuration SHA-256 and `Attended operational bounds`;
`doctor` prints the same bounds in its operational-bound check. The task-run
operation records an exact copy in
`.revolvr/autonomous/task-runs/<operation-id>/operation.json` and immutable
history, and the task-operation ledger admission/terminal evidence records the
same effective authority.

| Effective attended default | Value | Where to confirm before and after a run |
| --- | --- | --- |
| Total task attempts | 16 | `config check`/`doctor`; task view budgets; task-run operation and ledger evidence |
| Attempts per action | 4 each for `audit`, `correct`, `document`, `implement`, `plan`, `simplify` | Same operational-bound projection and task attempt history |
| Elapsed task budget | 4 hours | Same operational-bound projection and task operation |
| Model token budget | 1,000,000 | Same projection; completion uses recorded receipt token facts, never an estimate |
| Cycles per task | 50 | Same projection and task operation; CLI `--max-cycles` may lower the caller bound but cannot be unlimited |
| Process duration | 30 minutes | Same projection; rises to the largest configured Codex or verification timeout |
| Output per stream | 262,144 bytes (256 KiB) | Same projection plus `Output caps bytes`; rises to the largest configured subprocess stream cap |
| Retained artifacts per operation | 1,073,741,824 bytes (1 GiB) | Same projection and retention operation bounds |
| Notification attempts | 0 while disabled | Same projection; when enabled, exact positive `notifications.maximum_attempts` becomes the bound |

Other attended defaults are `operator_attended`, fresh ephemeral Codex,
model `gpt-5.6-sol`, reasoning `xhigh`, dangerous bypass enabled, sandbox
`workspace-write`, approval policy `never`, Git/commit timeout 30 seconds,
missing verification `fail`, dirty work refused, missing verification refused,
notifications disabled, and retention mutation disabled. All are printed by
`config check`; model/session/safety, executable, verification, repository,
and bounds are repeated by `doctor` and recorded in effective run provenance.

## Author tasks, dependencies, checkpoints, and input

Create one mixed task directly or preview a Markdown import before applying it:

```bash
revolvr task add "Implement the approved change" --summary "Approved change"
revolvr task import import-tasks.md --dry-run
revolvr task import import-tasks.md
revolvr task list
```

Canonical `.agent/tasks/*.md` files are tracked operator authority. Review and
commit task specifications before migration. Dependencies, tags, and conflicts
are source-ordered comma-separated frontmatter identities; missing/duplicate
IDs, self edges, cycles, active/archive ambiguity, and unverified archived
dependencies fail the whole graph closed.

Migration is dry-run first. Apply exactly the reviewed plan, then review and
commit the changed canonical task; `.revolvr` state remains local and ignored.

```bash
revolvr task migrate --to autonomous-v1 --dry-run "$TASK_ID"
revolvr task migrate --to autonomous-v1 "$TASK_ID"
revolvr task list
revolvr task show "$TASK_ID"
revolvr task why "$TASK_ID"
```

An `operator-checkpoint-v1` task is never selected for Codex. Author it and its
closed `operator-checkpoint-receipt-v1` JSON receipt through review, then bind
the exact declared receipt and operator identity:

```bash
revolvr checkpoint fulfill "$CHECKPOINT_TASK_ID" \
  --receipt ".agent/checkpoints/$CHECKPOINT_TASK_ID/receipt.json" \
  --operator "$OPERATOR_ID"
```

Fulfillment is an explicit tracked dependency transition, not autonomous
`needs_input`. For a typed supervisor question, inspect the exact revision and
options with `task show`, start `revolvr tui`, press `6` for Workflow, select
the task, press `a`, explicitly select an option, and press enter twice to
confirm. No recommendation is preselected. The answer and resume are separate
durable transitions; if the TUI says the answer persisted but resume failed,
repeat the same selected answer so the exact persisted authority can resume.
Never edit state/history JSON to answer a question.

## Start, monitor, cancel, and restart one task

Choose and record a unique stable operation ID before starting. Keep the same
task, operation ID, effective configuration, and cycle bound for any exact
reopen of that operation.

```bash
export TASK_ID=the-reviewed-task-id
export OPERATION_ID=l1-2026-07-16-task-001
revolvr doctor --for attended-task --task "$TASK_ID"
revolvr run --until-terminal \
  --task "$TASK_ID" \
  --operation-id "$OPERATION_ID" \
  --max-cycles 50
```

Run in the foreground and keep its streamed progress. From another terminal,
the following reads are safe while the owner is active:

```bash
revolvr status
revolvr task show "$TASK_ID"
revolvr task why "$TASK_ID"
```

To cancel, send one `Ctrl-C` to the foreground command and wait for it to
return a typed `operation_cancelled` result. Do not immediately kill the
terminal or edit locks. A graceful cancellation is a terminal old operation;
after inspection and a fresh passing doctor, continued work uses a new unique
operation ID. Re-running the old ID is only a read/replay of its terminal
evidence.

After process loss or machine interruption, first run the exact original
command with the same operation ID and material. It may replay a proven
idempotent edge or terminalize the operation as `unsafe_or_ambiguous`. Never
replace the ID just to bypass the durable owner. There is no Codex session
resume: every permitted invocation is a fresh ephemeral process.

## Inspect evidence

Capture IDs from the terminal summary and task view. These commands are
read-only:

```bash
revolvr status
revolvr task show "$TASK_ID"
revolvr task show "$TASK_ID" --json
revolvr task why "$TASK_ID"
revolvr show "$RUN_ID"
revolvr receipt validate "$RUN_ID"
revolvr metrics show
revolvr metrics show --json
```

`show` ties the run row to its timeline and artifacts. Receipt validation
compares the receipt with ledger and artifact authority. The JSON task view
contains `workspace.execution_root`, branch/ref, head/source/checkpoint,
verification, audit, completion, attempts/budgets, input, terminal state, and
raw provenance references. The corresponding protected evidence includes:

- `.revolvr/runs/<run-id>/` for exact dossiers, prompts, schemas, Codex JSONL,
  bounded stderr/output, verification and provenance;
- `.revolvr/receipts/<run-id>.md` for the harness-finalized receipt;
- `.revolvr/autonomous/task-runs/<operation-id>/` for checkpoint and immutable
  operation history;
- `.revolvr/autonomous/worktrees/<workspace-id>/` for the linked task source;
- `.revolvr/autonomous/tasks/<task-id>/completion/` for frozen evidence,
  `completion.md`, and the completion manifest after full finalization; and
- `.revolvr/ledger.sqlite` for live logical run/event authority.

Inspect configured notification and tracked archive evidence separately:

```bash
revolvr notification list
revolvr notification show "$DELIVERY_ID"
revolvr archive list
revolvr archive show "$ARCHIVE_SELECTOR"
revolvr archive verify "$ARCHIVE_SELECTOR"
```

Notification delivery is at-least-once: reconcile external receiver action by
the stable delivery ID and do not claim exactly once. Archive show is a bounded
read; verify recomputes complete tracked/runtime/Git/ledger authority and never
repairs it.

## Terminal stops and Level-1 recovery

Treat the command's typed stop reason as authority:

| Stop | Attended action |
| --- | --- |
| `completed` | Validate task, receipt, completion, workspace, Git, and ledger; review the task commit; archive only through a separate explicit command. |
| `needs_input` | Inspect the exact question; answer/resume through the TUI; after resume, start a new unique attended task operation. |
| `blocked` | Preserve the supervisor decision. Do not force the terminal autonomous state with generic retry; archive the terminal disposition or author/reopen a new reviewed task identity. |
| `verification_failed` or failed/flaky evidence | Preserve the exact occurrence and source. Do not relabel it with a later pass. If the operation becomes ambiguous, use task recovery below. |
| `no_progress` or exhausted attempt/time/token budget | Inspect signatures and durable accounting. Do not reset counters; narrow/re-author work through a new reviewed task lifecycle when appropriate. |
| `safety_stop` | Stop. Correct the external path, source, hook, environment, credential, process, or network condition; re-run config and doctor; preserve the old operation. |
| `operation_cancelled` | Inspect the settled terminal old operation. Resume useful work only under a new unique operation after a passing doctor. |
| `max_cycles` | Inspect state and budgets. If the task remains ready, a new attended operation may continue; the old operation remains terminal evidence. |
| `unsafe_or_ambiguous` | Quarantine is mandatory. Use only read-only recovery inspection and, if every authority passes, exact confirmed reconciliation. |

For `unsafe_or_ambiguous`, run the read-only form first:

```bash
revolvr task recover "$TASK_ID" --operation-id "$OPERATION_ID"
```

It reports ordered task, state, workspace, Git, ledger, receipt, and artifact
checks plus an authority SHA-256. If any check fails, leave the task
quarantined and preserve the complete repository/runtime tree for diagnosis.
If all checks pass and the report says reconciliation is eligible, repeat the
exact old ID in both confirmation fields:

```bash
revolvr task recover "$TASK_ID" \
  --operation-id "$OPERATION_ID" \
  --reconcile \
  --confirm-operation "$OPERATION_ID"
```

This preserves the old operation byte-for-byte and returns a new admitted
operation ID. Continue only by running that returned identity with the same
task/configuration/bounds. `task retry`, `task unblock`, a changed old run,
manual state edits, a queue selection, and daemon wake are generic retry and
cannot clear quarantine.

Every Level-1 transition-seam manual state maps to an exact inspection focus:

| Interrupted boundary | Required inspection before reconciliation |
| --- | --- |
| Supervisor | Accepted decision/reference, supervisor run/artifacts, exact unchanged before/after source, and consuming transition |
| Worker | One admitted attempt/charge, worker route/profile/dossier/prompt, settled process, receipt/artifacts, and exact owned source delta |
| Verification | Original plan/purpose/occurrence, ordered command attempts, process settlement/output, recomputed gate, and matching source |
| Commit | Pre/post HEAD, exact parent/tree/message/path delta, task branch/workspace, index/worktree/source, and commit/ledger evidence; never infer from exit zero |
| Checkpoint | Contiguous history, canonical state CAS edge, exact workspace/commit/tree/source, and prior/result checkpoint identity |
| Audit | Raw/canonical report, independent run and verification provenance, history/state edge, and every finding identity/resolution |
| Finalization | Frozen evidence, capsule/manifest, exact task/state stage chain, terminal ledger run/events, checkpointed source, and no missing predecessor |
| Notification | Use `notification show`; reconcile the receiver by stable delivery ID. Notification ambiguity never changes the task outcome. |
| Archive publication | Use archive show/verify, then task recovery. Replay `archive create` only with the original operation and byte-identical material. |

Queue reconciliation is intentionally absent: queue and daemon are not
Level-1 operations. The exhaustive before/during/after rules and prohibited
inferences are in `docs/external-recovery.md`.

## Review, accept, reject, and remove task workspaces

Take `WORKSPACE`, `TASK_COMMIT`, and the full branch ref from the validated
task JSON or recovery output. Before any decision, inspect the exact linked
worktree and commit:

```bash
git -C "$WORKSPACE" status --short --branch
git -C "$WORKSPACE" log --oneline --decorate -n 10
git -C "$WORKSPACE" show --stat --oneline "$TASK_COMMIT"
git worktree list --porcelain
```

The task workspace must be clean at the recorded commit, on the recorded
`refs/heads/revolvr/tasks/...` branch, with a tree equal to task evidence.
Review full diffs, tests, receipts, and findings before integration.

Acceptance is an explicit operator Git action on a clean destination branch.
For example, after independently recording the exact commit:

```bash
git switch the-operator-owned-destination-branch
git status --short
git cherry-pick "$TASK_COMMIT"
```

Resolve any conflict outside Revolvr and rerun project verification. Revolvr
does not push the result; remote publication remains a separate authorized
operator action.

To reject, do not cherry-pick or push. Preserve the task branch and evidence
until the task has an explicit terminal disposition and the required export/
retention decision is complete. A completed but unaccepted result may still be
archived as completed evidence; cancelled/abandoned work requires its exact
terminal reason and provenance, never a rewritten success.

Remove a workspace only after terminal evidence is settled, any needed commit
has been integrated or backed up, and the archive verifies. The ordinary Git
command refuses a dirty worktree; do not add `--force` to bypass that refusal:

```bash
git -C "$WORKSPACE" status --short
git worktree remove "$WORKSPACE"
git worktree list --porcelain
```

Delete the task branch only after the operator proves it is integrated and no
task, recovery, archive, or audit evidence still names it. Prefer retaining an
unmerged rejected branch or an immutable bundle over `git branch -D`.

## Archive terminal tasks

Completion never archives automatically. Supply exact UTC times, stable
operation identity, typed disposition, reason, and trusted provenance:

```bash
revolvr archive create "$TASK_ID" \
  --operation-id "$ARCHIVE_OPERATION_ID" \
  --disposition completed \
  --reason "accepted terminal completion" \
  --provenance "$OPERATOR_ID" \
  --terminal-at "$TERMINAL_AT" \
  --archived-at "$ARCHIVED_AT"
revolvr archive show "$TASK_ID"
revolvr archive verify "$TASK_ID"
```

Replay a partially published archive only with the same operation and exact
material. To create a new lifecycle, first require a passing verify, then use a
new task and operation identity:

```bash
revolvr archive reopen "$ARCHIVE_SELECTOR" \
  --operation-id "$REOPEN_OPERATION_ID" \
  --new-task-id "$NEW_TASK_ID" \
  --authority "$OPERATOR_ID" \
  --reason "reviewed new lifecycle" \
  --reopened-at "$REOPENED_AT"
```

The old archive and task operation remain immutable.

## Export evidence and apply retention deliberately

Export uses a stable operation and frozen UTC time. Record the printed export
ID and manifest outside the runtime tree, then verify and replay-validate it:

```bash
revolvr ledger export \
  --operation-id "$EXPORT_OPERATION_ID" \
  --exported-at "$EXPORTED_AT"
revolvr ledger export verify "$EXPORT_ID"
revolvr ledger export replay-validate "$EXPORT_ID"
revolvr metrics show --export "$EXPORT_ID"
```

Exports preserve logical ledger evidence; they do not automatically include
every task/workspace file. Copy the verified manifest/records and required
task, receipt, run, completion, recovery, archive, and Git evidence into the
operator's immutable evidence store with a separate hash manifest.

Retention mutation is disabled by default. Planning is non-destructive:

```bash
revolvr artifact gc \
  --operation-id "$GC_OPERATION_ID" \
  --planned-at "$GC_PLANNED_AT"
```

Review every action and pin. If policy explicitly enables mutation and the
verified export requirement is satisfied, apply only the exact printed plan:

```bash
revolvr artifact gc \
  --operation-id "$GC_OPERATION_ID" \
  --planned-at "$GC_PLANNED_AT" \
  --apply \
  --plan-id "$GC_PLAN_ID"
revolvr artifact gc inspect "$GC_OPERATION_ID"
```

After cancellation, resume only the admitted journal identity:

```bash
revolvr artifact gc \
  --operation-id "$GC_OPERATION_ID" \
  --apply \
  --resume
```

Never hand-delete a retention quarantine, immutable history, active task,
quarantine/recovery evidence, receipt, completion artifact, or archive.

## Upgrade an attended installation

1. Let the active command settle; confirm no Revolvr or task-workspace writer
   remains.
2. Export, verify, replay-validate, and externally hash the current evidence.
3. Record current Revolvr/Codex/Git versions, executable hashes, effective
   config hash, source HEAD, and task/workspace state.
4. Install the newly approved binary beside the old one and repeat the pinned
   installation checks. Never replace an approved binary through a moving
   symlink.
5. Read release migration notes, run `revolvr init` only for its documented
   idempotent initialization/seed behavior, then run `config check`, general
   attended doctor, exact-task doctor, status, task show, ledger export verify,
   and archive verify as applicable.
6. Keep the old binary and pre-upgrade evidence until one attended smoke passes
   and the release decision's rollback procedure is satisfied. Do not downgrade
   by rewriting newer runtime files or copy `.revolvr` between repositories.

Any material change to paths, executable identity, config, verification,
Codex, Git, recovery, commit, or retention authority invalidates the prior
preflight and requires a new operation.

## Remove runtime state safely

Runtime removal is destructive administration, not task recovery. Do it only
after all operations are terminal, quarantines are reconciled or intentionally
preserved externally, receipts/exports verify, required archives are tracked
and verify, task workspaces are clean and removed with `git worktree remove`,
and evidence has an independently checked external hash manifest. Never remove
`.agent`, tracked archives, task branches, or the Git common directory as part
of runtime cleanup.

First inspect registered worktrees and repository state:

```bash
git worktree list --porcelain
git status --short --branch
revolvr status
```

Then, from the exact canonical repository root, retire `.revolvr` by an atomic
same-filesystem rename. The guards prevent an empty or redirected variable
from becoming deletion authority:

```bash
RUNTIME_DIR="$(pwd -P)/.revolvr"
RETIRED_DIR="$(pwd -P)/.revolvr.retired.$(date -u +%Y%m%dT%H%M%SZ)"
[[ "$RUNTIME_DIR" == "$(pwd -P)/.revolvr" ]]
[[ -d "$RUNTIME_DIR" && ! -L "$RUNTIME_DIR" ]]
mv -- "$RUNTIME_DIR" "$RETIRED_DIR"
[[ -d "$RETIRED_DIR" && ! -L "$RETIRED_DIR" ]]
```

Run repository checks without Revolvr state and verify the external evidence
copy. Only after the retention period and a second exact path check may the
operator remove the retired directory:

```bash
[[ "$RETIRED_DIR" == "$(pwd -P)/.revolvr.retired."* ]]
[[ -d "$RETIRED_DIR" && ! -L "$RETIRED_DIR" ]]
rm -rf -- "$RETIRED_DIR"
```

If any guard, evidence check, archive verification, or Git worktree cleanup
fails, stop and preserve the directory. A later `revolvr init` creates a new
empty runtime identity; it does not recover the removed history.

## Smoke-test contract

`scripts/smoke-external-attended.sh` builds a disposable binary, creates an
external Git fixture, initializes/configures it, exercises task authoring and
migration, executes the non-destructive commands above (including positive and
safe-refusal evidence paths), exports and validates its ledger, plans retention,
checks every referenced subcommand's help, proves no Codex execution occurred,
and exercises guarded runtime retirement. Run it before accepting runbook
changes:

```bash
bash -n scripts/smoke-external-attended.sh
bash scripts/smoke-external-attended.sh
```
