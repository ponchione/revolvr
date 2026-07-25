# Attended Developer-Alpha Use

Status: source-built developer preview. This path is for an operator evaluating
the fixed harness in a disposable repository or one with a separately verified
backup and recovery plan. It is not a Level-1 release qualification,
external-use approval, tag, or release procedure. `EXT-20` real-Codex
qualification is incomplete, and RC.1 through RC.5 remain rejected history.

Use only one attended `autonomous-v1` task at a time. Keep the command in the
foreground, use finite bounds, and do not use queue or daemon mode. A linked
task worktree is Git separation, not a security sandbox: the selected Codex
process can still reach whatever the operator's environment, credentials,
hooks, filesystem permissions, and network allow.

## Build the fixed harness

Use a clean `main` checkout whose product source includes remediation commit
`010a8939ef6ad889a34590d05ce0326b6df57571`. From the Revolvr source root:

```bash
git status --short --branch
git merge-base --is-ancestor 010a8939ef6ad889a34590d05ce0326b6df57571 HEAD
git diff --exit-code 010a8939ef6ad889a34590d05ce0326b6df57571..HEAD -- cmd internal go.mod go.sum
mkdir -p ./bin
go build -trimpath -o ./bin/revolvr ./cmd/revolvr
export REVOLVR_BIN="$(pwd -P)/bin/revolvr"
"$REVOLVR_BIN" --version
go version -m "$REVOLVR_BIN"
```

The exact binary location is `<revolvr-source-root>/bin/revolvr`. The version
may report `dev`; that is expected and is one reason this is not a release
artifact. Stop if the checkout is dirty, the ancestry check fails, or the
product-source diff is nonempty.

## Prepare one repository

Start with an operator-owned, non-bare Git worktree with at least one commit,
no active submodules, and a clean index and worktree. Before initialization,
make and verify a repository-specific backup or use a repository that can be
discarded in full.

```bash
export PROJECT_ROOT=/absolute/path/to/disposable-or-recoverable-project
cd "$PROJECT_ROOT"
test "$(git rev-parse --show-toplevel)" = "$(pwd -P)"
test "$(git rev-parse --is-bare-repository)" = false
test -z "$(git submodule status --recursive)"
test -z "$(git status --porcelain=v1 --untracked-files=all)"
umask 0022
"$REVOLVR_BIN" init
find .agent .revolvr -xdev -perm /022 -print
git status --short --branch
```

The `find` command must print nothing. Review the initialized `.agent`
profiles, then commit the intended task-authority files with the project's
normal Git process so the repository is clean before preflight. `.revolvr/` is
ignored local runtime/evidence state and must be included in the backup and
recovery plan even though it is not committed.

Create `.revolvr/config.yaml` using the minimal attended configuration in
[the attended external-project runbook](external-project-runbook.md#configure-attended-execution-and-verification).
Use an absolute Codex path whose exact version and executable hash match
`internal/codexexec/release_manifest.json`, replace the example verification
command with the project's deterministic bounded check, keep
`autonomy.mode: operator_attended`, and keep verification and dirty-worktree
refusals enabled. The configured Codex timeout and each verification timeout
must be finite. Do not put secret values in the file.

Validate the effective configuration before authoring a run:

```bash
"$REVOLVR_BIN" config check
"$REVOLVR_BIN" status
```

`config check` is diagnostic. It prints the effective configuration hash and
finite attended bounds, but only `doctor` establishes current readiness.

## Author and preflight one task

Create and review one task, convert it to the autonomous workflow, and commit
the resulting `.agent` authority before running it:

```bash
"$REVOLVR_BIN" task add "Implement one bounded reviewed change" --summary "Developer-alpha task"
"$REVOLVR_BIN" task list
export TASK_ID=the-id-printed-by-task-add
"$REVOLVR_BIN" task migrate --to autonomous-v1 --dry-run "$TASK_ID"
"$REVOLVR_BIN" task migrate --to autonomous-v1 "$TASK_ID"
"$REVOLVR_BIN" task show "$TASK_ID"
"$REVOLVR_BIN" task why "$TASK_ID"
git add .agent
git diff --cached --check
git commit -m "Add reviewed Revolvr developer-alpha task"
test -z "$(git status --porcelain=v1 --untracked-files=all)"
"$REVOLVR_BIN" doctor --for attended-task
"$REVOLVR_BIN" doctor --for attended-task --task "$TASK_ID"
```

Both doctor commands must end in `Ready: true`. Stop on any warning or
refusal; do not weaken path modes, executable identity, verification, clean
Git, or safety configuration merely to make preflight pass.

## Run one attended operation

Use Bash to allocate and check one collision-resistant stable operation ID,
lower the caller cycle bound for the first evaluation, and remain present for
the whole foreground command:

```bash
export OPERATION_ID="alpha-$(date -u +%Y%m%dT%H%M%SZ)-$$-$RANDOM"
test ! -e ".revolvr/autonomous/task-runs/$OPERATION_ID"
"$REVOLVR_BIN" doctor --for attended-task --task "$TASK_ID"
"$REVOLVR_BIN" run --until-terminal \
  --task "$TASK_ID" \
  --operation-id "$OPERATION_ID" \
  --max-cycles 3
```

Do not run a second task concurrently or modify the control repository/task
workspace while this owns the operation. Send one `Ctrl-C` if you need to
stop, then wait for a typed terminal result. Do not kill the terminal, edit
runtime JSON, remove locks, reset counters, or invent a replacement operation
ID to bypass retained authority.

After the command settles, inspect the summary and durable evidence:

```bash
"$REVOLVR_BIN" status
"$REVOLVR_BIN" task show "$TASK_ID"
"$REVOLVR_BIN" task show "$TASK_ID" --json
"$REVOLVR_BIN" task why "$TASK_ID"
"$REVOLVR_BIN" show "$RUN_ID"
"$REVOLVR_BIN" receipt validate "$RUN_ID"
```

Take `RUN_ID`, the task workspace, and the generated commit from the terminal
summary and task JSON. Review the complete receipt, verification/audit
evidence, commit, and patch before integrating anything:

```bash
git -C "$WORKSPACE" status --short --branch
git -C "$WORKSPACE" show --stat --oneline "$TASK_COMMIT"
git -C "$WORKSPACE" show --format=fuller --find-renames "$TASK_COMMIT"
git worktree list --porcelain
```

Revolvr does not merge or push the generated commit. Integration is a separate
operator Git action only after review and rerunning the project's verification
on the intended destination.

## Stops and recovery

Treat `completed`, `needs_input`, `blocked`, `verification_failed`,
`no_progress`, `safety_stop`, `operation_cancelled`, `max_cycles`, and
`unsafe_or_ambiguous` as distinct retained outcomes. Preserve the old
operation and its `.revolvr` evidence. After ordinary process loss, reissue
the exact original command with the same task, operation ID, configuration,
and cycle bound; it may replay only proven durable work or stop ambiguous.

For `unsafe_or_ambiguous`, do not rerun model work. Start with the read-only
inspection:

```bash
"$REVOLVR_BIN" task recover "$TASK_ID" --operation-id "$OPERATION_ID"
```

Leave the task quarantined unless every reported authority agrees. Exact
confirmed reconciliation, interruption-seam expectations, workspace removal,
and evidence retention are described in
[the attended runbook](external-project-runbook.md#terminal-stops-and-level-1-recovery)
and [recovery matrix](external-recovery.md). Those mechanics do not turn this
developer-alpha path into Level-1 approval.

## What the no-model checks prove

From the Revolvr source root, maintainers can exercise the relevant surfaces
without a real Codex/API call:

```bash
bash scripts/smoke-external-attended.sh
go test -count=1 -v ./internal/app -run '^TestProductionAutonomousHappyPath$'
```

The shell smoke builds a disposable binary, creates a collision-free temporary
Git repository, and checks `--help`, `init`, `config check`, `status`, and
attended `doctor`; its deliberately unlisted fake proves doctor refuses before
Codex execution. The focused Go test builds the repository's strict fake Codex
and completes one production-composition autonomous task in a separate
temporary Git repository, including workspace creation, verification, commit,
audit, finalization, receipt, and ledger evidence.

Together these checks prove local CLI/preflight behavior and the production
task path under deterministic fakes. They do not prove real API acceptance,
real-model quality, long-running recovery, quantitative dogfood thresholds,
or external-project qualification. Those claims remain blocked on unchecked
`EXT-20`.
