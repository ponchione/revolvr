# revolvr

`revolvr` is a local Go CLI for running bounded Codex harness passes from
repo-owned Markdown tasks under `.agent/tasks/*.md`. It keeps local runtime
state under `.revolvr/`, launches fresh Codex executions for work passes,
verifies the resulting work, records run history, and commits verified changes.

## Setup

Requirements:

- Go 1.22 or newer
- Git
- Codex CLI available as `codex`

Build or run from the repository root:

```bash
go build ./cmd/revolvr
go run ./cmd/revolvr --help
```

Initialize local runtime state:

```bash
go run ./cmd/revolvr init
```

This creates `.revolvr/` with ledger, run artifact, receipt, and lock state,
and ensures `.agent/tasks/` exists for canonical Markdown tasks. `.revolvr/` is
local runtime state and is ignored by Git. When initialized from a Git worktree,
`init` adds `/.revolvr/` to
`.git/info/exclude` so tracked ignore files do not need to change.

## Tasks

Add work for the harness:

```bash
go run ./cmd/revolvr task add --summary "README docs" "Add concise setup and usage docs"
```

List task files:

```bash
go run ./cmd/revolvr task list
```

If a task is blocked and should be retried:

```bash
go run ./cmd/revolvr task retry <task-id>
```

### Mixed-Pass Task Workflow

A durable task is one canonical Markdown file under `.agent/tasks/*.md`. It
keeps its identity and workflow state as it progresses. A pass (or run) is one
fresh Codex execution for that task, with its own run ID, ledger events,
artifacts, and receipt. One durable task therefore normally spans several
passes.

Minimal canonical task file:

```markdown
---
id: example-task
status: pending
workflow: mixed-pass-v1
phase: implement
---
# Example task

Describe the bounded work here.
```

Omitting `workflow` or `phase` defaults them to `mixed-pass-v1` and
`implement`. The workflow progresses in this exact order:

```text
implement -> audit -> document -> simplify -> completed
```

Revolvr policy selects the repo-authored profile for each phase:

- `implement` uses `implementer` to make the bounded change. It requires
  meaningful changes before the metadata transition; a verified no-change
  implementation pass is refused with `no_changes` and does not advance.
- `audit` uses `auditor` to review correctness, regressions, verification, and
  risks. It may succeed without product or source changes.
- `document` uses `documentor` to update operator-facing documentation when
  needed. It may succeed without product or source changes.
- `simplify` uses `simplifier` to reduce worthwhile complexity or duplication.
  It may succeed without product or source changes and completes the task.

`revolvr init` also seeds the repo-authored `supervisor`, `planner`, and
`corrector` profiles for the future autonomous workflow. They define
decision-only supervision, planning-only output, and finding/failure-scoped
correction, but `mixed-pass-v1` does not select or execute them.

Task frontmatter `profile` is not an operator override. Revolvr derives the
selected profile from `workflow` and `phase` through its pass policy.

Revolvr, not Codex-authored task edits, owns durable phase transitions. After
Codex and verification succeed, Revolvr applies the policy transition to the
task file before the commit gate. Nonterminal transitions keep the task
`pending` at the next phase; a successful `simplify` pass marks it `completed`.
Audit, document, and simplify may advance without source changes because the
task-file metadata transition is durable work included in the commit. A failed
or blocked outcome leaves the original phase unchanged and blocks the task;
`revolvr task retry <task-id>` returns it to `pending` at that same phase.

Receipts, ledger events, and committed task-file transitions make every phase
outcome auditable. Receipt text records the outcome but does not choose the
next phase; Revolvr applies policy to the harness outcome.

Use `revolvr task list` for workflow, phase, profile, and next-state columns,
and `revolvr status` for the next runnable task, its next pass, and recent pass
state. The TUI Dashboard and Tasks views show the current workflow state. Use
`revolvr show <run-id>` or TUI Run Detail to inspect task-selection timeline
metadata and the underlying event sequence.

## Chat-To-Task Imports

Use a web chat or design session to shape a feature into small, bounded tasks,
then save the result as a Markdown import file in the repository. The import
format is intentionally simple: each task starts with a `## Task` heading, has a
required task body, and may include `### Summary`, `### Acceptance`, and
`### Verification` sections. Summary becomes the task summary; acceptance and
verification notes stay in the task text so the Codex pass sees them.

Minimal import file:

```markdown
# Next Revolvr Tasks

## Task
Document the task import workflow in the README.

### Summary
README task import workflow

### Acceptance
- README explains dry-run, import, refresh, preflight, and one-pass TUI use.

### Verification
- go test ./...
- go run ./cmd/revolvr task import --help
```

Save the file somewhere local to the repo, for example
`.agent/imports/next-tasks.md`, then preview it without mutating `.revolvr/`:

```bash
go run ./cmd/revolvr task import --dry-run .agent/imports/next-tasks.md
```

Import the tasks when the dry-run output looks right:

```bash
go run ./cmd/revolvr task import .agent/imports/next-tasks.md
```

Open the TUI:

```bash
go run ./cmd/revolvr tui
```

If the TUI is already open, press `r` to refresh the shared task state. To run a
single pass from the TUI, press `5` to open Preflight, press `p` to run the
readiness check, then press `R` once preflight is ready. To run a bounded
multi-pass loop from the TUI, press `n` to choose 2, 3, or 5 max passes
(default 3), then press `L`; the progress pane shows pass summaries as the loop
runs. The closest CLI equivalents are:

```bash
go run ./cmd/revolvr doctor
go run ./cmd/revolvr run --once
go run ./cmd/revolvr run --max-passes 3
```

Chat sessions, the CLI, and the TUI all read and write the same
`.agent/tasks/*.md` task files, so it is fine to design tasks in chat and
refresh the TUI after importing them. Avoid concurrent code edits against the
same repository while a TUI-started or CLI-started Codex pass is running; keep
one actor responsible for worktree changes at a time.

## Configuration

Configuration is optional and lives at `.revolvr/config.yaml`. Inspect the
effective configuration with:

```bash
go run ./cmd/revolvr config check
```

Example:

```yaml
codex:
  executable: codex
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
      timeout_seconds: 300
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

For Go repositories, the effective default verification command is
`go test ./...` when `verification.commands` is omitted or `null` and no tiered
plan is configured. An explicit `commands: []` disables that synthesized
default and is evaluated by the configured missing-verification and preflight
policies. CLI-initiated
runs default to Codex dangerous bypass/yolo mode for operator-controlled local harness
passes; set `codex.dangerously_bypass_approvals_and_sandbox: false` or
`codex.yolo: false` to disable that default. Every run starts a fresh
`codex exec` session and explicitly passes the effective model,
`model_reasoning_effort`, and `--ephemeral`; persistent and resumed sessions
are not supported. The defaults are `gpt-5.6-sol`, `xhigh`, and ephemeral.

### Unattended safety boundary

`operator_attended` is the compatibility mode for current local dogfood and
mixed-pass runs. It reports dangerous bypass, ambient environment, unknown
network posture, and operator-trusted hooks as explicit operator
responsibilities. A linked task worktree isolates Git/source state only: it
does not constrain filesystem access, child processes, hooks, environment,
network access, or host credentials, and is not a security sandbox.

`fully_unattended` is opt-in and fails closed before a supervisor or worker
starts unless the task has an exact admitted AW-18 workspace and the policy
declares externally attested container/OS isolation, an externally attested
network posture, non-ambient environment handling, trusted or absent hooks,
resolved executable identities, protected harness paths, and configured
secret redaction. Dangerous Codex approval/sandbox bypass remains visible and
requires compatible external isolation.

The unattended acknowledgement is not a boolean. A failed task-bound safety
preflight reports an exact value shaped as
`revolvr-fully-unattended-v1:<policy-sha256>`; the operator must place that
exact value in `autonomy.acknowledgement`. Material permission, workspace,
command, hook, isolation, or network changes produce a different policy hash
and invalidate the old acknowledgement.

Secret redaction names explicit environment variables; their values are read
only at runtime and are not written to effective configuration, hashes,
doctor/config output, Codex output artifacts, progress, or ledger diagnostics.
For example:

```yaml
autonomy:
  redaction:
    environment_variables: [OPENAI_API_KEY, INTERNAL_TOKEN]
```

Revolvr does not itself create or prove a container, OS sandbox, firewall, or
network namespace. Fully unattended declarations therefore require stable
operator-authored attestation evidence; an ambient environment variable alone
is not proof.

### Artifact retention and ledger exports

Retention is harness-authored configuration and is mutation-disabled by
default. The policy is included in the effective configuration hash and shown
by `config check` and `doctor`. Only exact ledger-owned Codex JSONL and stderr
streams are eligible; active-task, nonterminal, recent, archive, completion,
task-run, queue, child, and recovery references pin their transitive run
evidence. Unknown or unsafe stream ownership fails closed.

```yaml
retention:
  schema_version: revolvr-artifact-retention-policy-v1
  mutation_enabled: false
  recent_run_count: 20
  compress_after_seconds: 604800
  prune_after_seconds: 7776000
  minimum_compress_bytes: 65536
  compress_codex_jsonl: true
  compress_codex_stderr: true
  prune_compressed_streams: false
  require_verified_export: true
  max_files_per_operation: 100
  max_bytes_per_operation: 1073741824
  decompression_cap_bytes: 268435456
```

GC is dry-run-first and requires an explicit frozen UTC time. The dry-run does
not create a runtime directory, SQLite sidecar, lock, export, or temporary
file. To apply, repeat the exact authority and supply the printed plan ID:

```bash
go run ./cmd/revolvr artifact gc \
  --operation-id gc-2026-07 \
  --planned-at 2026-07-12T16:00:00Z

go run ./cmd/revolvr artifact gc \
  --operation-id gc-2026-07 \
  --planned-at 2026-07-12T16:00:00Z \
  --apply --plan-id <exact-plan-id>

go run ./cmd/revolvr artifact gc inspect gc-2026-07
go run ./cmd/revolvr artifact gc --operation-id gc-2026-07 --apply --resume
```

A mutating GC transaction excludes autonomous execution, Git administration,
child publication, and control-root or workspace source writers until cleanup
finishes. A source writer already in progress holds shared admission and makes
GC wait; a writer starting during GC waits before publishing lock metadata or
mutating source.

Compression uses deterministic gzip plus a versioned manifest that retains the
original SHA-256, size, and mtime. Receipt validation and Codex-metric recovery
read the admitted compressed form transparently and reject missing, corrupt,
dual, or divergent representations. Pruning is opt-in and requires an exact
immutable ledger export that passes verification and logical replay before any
file enters the operation quarantine. Prune renames are synchronized on both
the source and complete destination-directory chain before journal completion;
quarantine removal synchronizes its parent before the cleaned state is durable.

Resume and inspection reconstruct the latest legal state from contiguous
immutable journal history rather than trusting the mutable checkpoint. Every
claimed completed action is reconciled with its exact filesystem effect, and
recorded prune exports are reverified and replay-validated before recovery can
skip work or report terminal success.

Ledger export is also available independently. It preserves every run field,
global event identity, exact payload bytes, event payload schema label, range,
and a WAL-safe canonical logical source-ledger identity without deleting live
SQLite rows. Each canonical JSON record is limited to 16 MiB excluding its
newline delimiter; exact-limit records verify and replay, while an oversized
task or event stops export before any immutable artifact is published:

```bash
go run ./cmd/revolvr ledger export \
  --operation-id export-2026-07 \
  --exported-at 2026-07-12T16:00:00Z
go run ./cmd/revolvr ledger export verify <export-id>
go run ./cmd/revolvr ledger export replay-validate <export-id>
```

### Autonomous completion capsules

The autonomous runtime now has a bounded terminal-finalization transaction.
After a fresh supervisor has produced an exact `complete` decision, the
finalizer revalidates the current plan, acceptance matrix, final-purpose
verification, independent clean audit, findings, workspace/checkpoint, safety
policy, configuration, attempt state, ordered runs, and commit history. It does
not invoke Codex, rerun verification, audit again, change source, or create a
source commit.

Accepted evidence is frozen before terminal completion. Deterministic artifacts
are stored under the control-root runtime namespace:

```text
.revolvr/autonomous/tasks/<task-id>/completion/completion-evidence.json
.revolvr/autonomous/tasks/<task-id>/completion/completion.md
.revolvr/autonomous/tasks/<task-id>/completion/completion-manifest.json
```

The execution state advances monotonically through admission, capsule
materialization, task status completion, state completion, and terminal ledger
completion. Immutable history precedes every canonical state replacement;
crash retries reuse identical artifacts and ledger evidence and reject changed
material. Configured AW-19 secrets are redacted at all three completion-artifact
boundaries. The capsule is human-readable evidence, while the frozen typed
record, manifest hashes, canonical state, and ledger remain authoritative.

There is intentionally no autonomous finalization CLI/TUI command yet. Once a
terminal lifecycle already exists, the separate archive boundary can move one
exact task into tracked history.

### Tracked terminal task archives

Terminal autonomous tasks use a tracked UTC archive layout:

```text
.agent/archive/YYYY/MM/<task-id>/task.md
.agent/archive/YYYY/MM/<task-id>/completion.md
.agent/archive/YYYY/MM/<task-id>/archive.json
```

`completed` archives require the complete ledger-completed AW-20 state,
frozen evidence, manifest, capsule, workspace/checkpoint, verification, audit,
safety, and terminal ledger identities. The archived `completion.md` is an
exact byte copy of the verified runtime capsule. `cancelled`, `superseded`, and
`abandoned` archives require typed terminal reason, provenance, and time and
explicitly omit completion authority. Blocked and needs-input tasks remain
active and cannot be archived.

Archive creation is one bounded administrative operation. The archive time is
explicit UTC authority for `YYYY/MM`; the operation writes immutable runtime
history before each journal stage, publishes archive files without overwrite,
removes only the exact active task, and creates a path-scoped Git commit with
archive identity lines. A retry rolls forward proven partial effects. Unrelated
dirty or staged paths, active source writers, collisions, symlinks, changed
bytes, and ambiguous Git outcomes fail closed without reset, clean, restore,
stash, or broad deletion.

Read operations are:

```bash
go run ./cmd/revolvr archive list
go run ./cmd/revolvr archive show <archive-id-or-task-id>
go run ./cmd/revolvr archive verify <archive-id-or-task-id>
```

Verification is read-only. It cross-checks the tracked task/capsule/manifest,
terminal state and AW-20 evidence, ledger runs/events, immutable archive
history, administrative commit identity and tree bytes, active-task exclusion,
reopen lineage, and configured-secret absence. A failed named check produces a
nonzero exit.

Archive creation requires explicit harness/operator authority rather than
inferring it from prose or status alone:

```bash
go run ./cmd/revolvr archive create <task-id> \
  --operation-id <operation-id> \
  --disposition completed \
  --reason '<exact terminal reason>' \
  --provenance '<trusted authority>' \
  --terminal-at 2026-07-12T16:00:01Z \
  --archived-at 2026-07-12T16:05:00Z
```

Reopen never changes the terminal record. It first verifies the archive, then
creates a new task ID, a new pending autonomous state, and typed
`reopened_from` lineage naming the immutable archive and archive commit. It
does not run the new task:

```bash
go run ./cmd/revolvr archive reopen <archive-id-or-task-id> \
  --operation-id <operation-id> \
  --new-task-id <new-task-id> \
  --authority '<trusted authority>' \
  --reason '<reason for new lifecycle>' \
  --reopened-at 2026-07-12T17:00:00Z
```

The TUI Workflow view can display this archived evidence through strict
read-only Show semantics. It does not run full archive verification, reopen an
archive, or create an archive automatically.

## Dogfooding

Use this short flow when exercising Revolvr against this repository:

```bash
go run ./cmd/revolvr doctor
go run ./cmd/revolvr task add --summary "README docs" "Add concise setup and usage docs"
go run ./cmd/revolvr run --once
go run ./cmd/revolvr status
go run ./cmd/revolvr show <run-id>
```

`doctor` reports initialized state, the effective Codex model/reasoning/session
settings, the bounded `<codex> --version` result, configured Codex and Git
executables, Git identity, clean worktree state, `.revolvr/` ignore state, and
effective verification coverage. It exits nonzero when a required check fails.
Use `status` to find recent run IDs for `show`.

Note: Real dogfood runs should start from a clean worktree and use `status` and
`show <run-id>` to inspect the recorded result.

For a fuller live check against real Codex, run:

```bash
./scripts/dogfood-live.sh
```

This script is intentionally destructive to local Revolvr runtime state: it
requires a clean source worktree, removes `.revolvr/`, initializes fresh state,
creates one tiny file-update task, runs `revolvr run --once`, and verifies the
receipt, ledger-backed `status`/`show` output, commit SHA, `receipt validate`,
and final clean worktree. It creates one Git commit when the live run passes.

## TUI

Open the interactive terminal UI from the repository root:

```bash
go run ./cmd/revolvr tui
```

The TUI shows the same app-backed state as the CLI: task counts, task details,
recent runs, run diagnostics, artifacts, receipt validation results, preflight
readiness checks, and a live progress pane while a TUI-started operation is
active. Press `6` for the Workflow view. It renders the AW-27 active/archive
projection, including decisions and readiness, plans, acceptance, findings,
attempts and budgets, typed input, verification/audit, workspace/checkpoint,
terminal/archive, provenance, and diagnostics. The detail remains scrollable
at narrow widths; archive evidence is explicitly unverified until the separate
CLI verification boundary runs.

Use number keys to switch views, `j`/`k` or arrow keys to move list selections,
`a` to add a task, `p` in Preflight to check readiness, `R` to run one
mixed-pass pass after preflight is ready, `U` to run the selected
`autonomous-v1` task until a terminal-for-now outcome, `n` to cycle the
mixed-pass loop max through 2/3/5 (default 3), `L` to start that bounded loop,
and `Q` to start a bounded sequential autonomous queue sweep. The TUI control
intentionally stays at one worker; use CLI/config for bounded parallelism. In
Workflow,
`a` opens answer selection only for a current typed needs-input question; no
option or recommendation is preselected, and submission requires an explicit
choice plus confirmation. Press `c` to request cancellation of the active
TUI-started run, loop, task run, or queue, `v` in Run Detail to validate the
loaded receipt, `r` to refresh, and `?` for in-app key help.

Current limitations: the TUI is still a local terminal view over the same
runtime and `.agent/tasks/*.md` task state. It does not verify/create/reopen
archives, install or control the daemon, run retention operations, or change
configuration. Use the CLI for those operations, the non-TUI run alternatives,
and receipt validation outside the loaded run detail.

## Run

Run one selected pending task:

```bash
go run ./cmd/revolvr run --once
```

Run up to a bounded number of passes:

```bash
go run ./cmd/revolvr run --max-passes 3
```

Run one exact autonomous task through fresh supervisor/worker cycles until it
completes, blocks, needs input, is cancelled, exhausts an AW-15 budget, reaches
the caller's cycle fallback, or encounters an unsafe/ambiguous condition:

```bash
go run ./cmd/revolvr run --until-terminal --task <task-id> --max-cycles 50
```

Omitting `--task` selects the next runnable `autonomous-v1` task once and pins
that identity for the complete operation. Autonomous frontmatter may use
comma-separated, source-ordered `depends_on`, `tags`, and `conflicts` values;
items contain no surrounding whitespace. Only pending tasks whose dependencies
are completed are ready. A verified, reconciled completed archive satisfies an
old dependency identity but is never itself selected. Priority remains
ascending, followed by canonical task path and task ID ties. Missing IDs,
duplicate/self edges, active/archive ambiguity, and dependency cycles fail
closed. Explicit task runs apply the same readiness check before workspace or
model work begins.

The supervisor may attach at most four structured `child_tasks` to a `block`
or `needs_input` decision. Revolvr validates cited scope evidence, derives
collision-resistant task IDs, records immutable parent/decision/run lineage,
and publishes pending child state and task bytes through one restartable
operation. A dependent child waits for its parent; explicitly independent work
cannot answer or bypass the parent's blocker. Publication starts no worker,
charges no attempt, and never changes the parent task or state. Incomplete
publication journals fail scheduling closed until exact recovery finishes.
Recovery reconstructs one contiguous immutable transition history and treats
the mutable checkpoint only as a backed cache. Publisher replay and scheduler
admission consume the same validated child-set projection, including exact
operation, parent, decision, proposal, initial artifact identities, and
immutable child lineage.

`--operation-id <id>` supplies a
stable restart identity; repeating the same operation reopens its durable
checkpoint, while changing its pinned task, effective configuration, or cycle
limit is rejected. Every supervisor and worker remains a distinct fresh
ephemeral Codex execution. AW-15 admits attempt-consuming work before its
worker starts, and the loop delegates planning, audit, correction/re-audit,
optional-role persistence, structured input, workspace checkpoints, and final
completion to their existing bounded owners.

The result reports a deterministic stop reason and cumulative statistics.
Blocked and needs-input tasks remain active. A completed result has passed the
transactional completion boundary but is deliberately not archived; archive
creation remains the separate explicit `revolvr archive create` operation.
The loop never spends surplus cycles on another task. `--once` and
`--max-passes` retain their existing mixed-pass meanings and cannot be combined
with `--until-terminal`.

Run every currently ready autonomous task until the queue is
drained, waiting, bounded, cancelled, or stopped by safety evidence:

```bash
go run ./cmd/revolvr run --queue \
  --operation-id <stable-queue-id> \
  --max-tasks 100 \
  --max-cycles 50 \
  --workers 2
```

Queue workers default to `1` and are capped at `4`. The strict config equivalent
is `queue.maximum_workers`; `--workers` overrides it for one queue or daemon
operation and becomes part of that operation's effective-config identity. The
queue rebuilds the exact dependency graph before every admission, walks its
canonical order against the exact occupied set, and durably persists the whole
ordered batch, slot identities, and derived AW-22 operation IDs before starting
any worker. When overlap cannot be proven, it records a sequential fallback.
Dependent or conflicting tasks never share a batch. Completed tasks can unlock
dependents at the next boundary. Clean blocked,
needs-input, task-cancelled, no-progress, budget, and max-cycle outcomes are
recorded and yielded so unrelated ready work can continue; unsafe or ambiguous
state stops the queue immediately. A yielded task is excluded only while its
exact task/state authority is unchanged, preventing a high-priority task from
starving later work without rewriting priority or dependencies.

Queue checkpoints and immutable history live under
`.revolvr/autonomous/queues/<operation-id>/`. Replaying a nonterminal operation
reopens only unresolved exact slots; replaying a terminal operation returns its
ordered prior outcomes without starting work. Recovery derives authority from
one canonically named, contiguous, legal immutable-history chain. The mutable
checkpoint may be missing or exactly behind that chain, but cannot lead or
conflict with it. Canonical outcomes remain in
selection order even when workers finish in another order. Caller cancellation
cancels every active child, waits for cleanup, and reconciles all evidence
before returning. A task-local safe stop does not cancel its peers. Stop reasons
distinguish `drained`, dependency/input/blocked waiting, queue budget,
cancelled, safety, and unsafe/ambiguous state. The outer execution lease still
admits only one direct run or queue coordinator; parallelism exists only among
isolated task workspaces inside that coordinator. Archive mutation refuses
while the coordinator is active, and retention keeps its existing refusal.

Queue and child-publication runtime state uses the shared protected-path
boundary: every ancestor, directory, lock, checkpoint, history entry, temporary
file, and opened descriptor must remain the same safe harness-owned object.
Symlinks, hard links, unsafe modes, wrong file types, or substitutions across
open, link, rename, cleanup, read, and directory-sync boundaries fail closed.

An optional foreground daemon runs the same bounded sweeps after a stable
readiness-authority change:

```bash
go run ./cmd/revolvr run --daemon \
  --operation-id <stable-daemon-id> \
  --max-tasks 100 \
  --max-cycles 50 \
  --workers 2 \
  --max-sweeps 1000 \
  --daemon-poll 1s \
  --daemon-debounce 500ms
```

Daemon mode requires an explicitly acknowledged `fully_unattended` AW-19
configuration. It watches deterministic task/state/archive/child-publication
authority rather than directory mtimes, queue checkpoints, ledger events, or
timestamps, so it does not wake itself. A changed fingerprint must remain
stable across the debounce interval before one new sweep starts. The daemon
holds no source, task-state, Git-admin, child-publication, or autonomous
execution lease while waiting, and caller or signal cancellation interrupts
poll/debounce waits. It remains a foreground process: Revolvr does not detach,
install a service, or expose remote control.

### External outcome notifications

External notification hooks are disabled by default. When enabled, Revolvr
runs one exact executable without a shell and sends a canonical
`revolvr-notification-payload-v1` JSON object on standard input. The allowlist
covers only `task_completed`, `task_blocked`, `task_needs_input`,
`safety_stop`, `queue_drained`, and `daemon_failed`:

```yaml
autonomy:
  redaction:
    environment_variables: [REVOLVR_NOTIFY_TOKEN]

notifications:
  schema_version: revolvr-notification-policy-v1
  enabled: true
  events: [task_completed, task_blocked, task_needs_input, safety_stop, queue_drained, daemon_failed]
  executable: /usr/local/bin/revolvr-notify
  args: [--from-stdin]
  directory: repository_root
  environment_names: [REVOLVR_NOTIFY_TOKEN]
  timeout_seconds: 10
  stdout_cap_bytes: 65536
  stderr_cap_bytes: 65536
  maximum_attempts: 3
  retry_delay_seconds: 2
```

The hook receives only the configured replacement environment; ambient host
variables are not inherited. Every configured environment name must also be
covered by the AW-19 secret-redaction policy, and missing or empty values stop
delivery before the process starts. Executable, ordered arguments, repository
working directory, environment names, timeout, output caps, attempts, and
retry delay are effective configuration authority. Payloads and durable
evidence contain names and redacted bounded text, never configured secret
values.

Delivery intent and exact payload bytes are synchronized under
`.revolvr/autonomous/notifications/<delivery-id>/` before invocation. The
delivery ID and JSON `event_id` are deterministic from the durable source
occurrence and effective hook/config material. Every retry receives
byte-identical JSON and the same delivery ID, allowing receivers to implement
idempotency. A successful local record is never intentionally invoked again.
There is necessarily a crash window after a receiver acts but before local
success is durable, so Revolvr does not claim exactly-once external effects.

Timeouts, nonzero exits, cancellation, output truncation, retries, and final
failure are recorded in immutable transition history and a canonical journal.
Hook failure never changes the task lifecycle, question, terminal reason,
queue result, daemon result, archive, workspace, Git state, or source ledger
authority. Hooks are ordinary explicitly configured external processes, not
Git hooks or a firewall/container sandbox; their filesystem and network access
is whatever the host grants that executable.

Inspect redacted delivery evidence without dispatching or retrying:

```bash
go run ./cmd/revolvr notification list
go run ./cmd/revolvr notification show <delivery-id>
```

A pass selects a runnable task, writes `.revolvr/runs/<run-id>/context.md` plus
its `.revolvr/runs/<run-id>/context.json` manifest, runs Codex with that context
payload, captures artifacts, runs verification commands, records a receipt and
ledger events, and commits the verified result. Failed Codex, verification,
commit, or safety outcomes are recorded without pushing branches. While Codex
runs, `revolvr run` streams concise progress messages to stdout; the full Codex
JSONL and stderr streams remain captured as run artifacts. `run --max-passes`
prints a final loop summary and stops early when failed or blocked passes need
inspection.

### Autonomous dossier cache and role views

Autonomous supervisor and worker calls receive deterministic role-specific
dossiers. The supported views are `supervisor`, `planner`, `implementer`,
`auditor`, `corrector`, `documentor`, and `simplifier`; unsupported
action/profile combinations fail instead of receiving the old broad dossier.
Each manifest names included and omitted sections, exact byte and item counts,
and a deterministic `utf8-bytes-ceil-div-4-v1` token estimate. These estimates
are planning facts, not actual Codex token usage.

Revolvr derives a bounded repository path map from the exact committed Git
tree and caches it under `.revolvr/cache/dossier/v1/<content-key>/`. The key
binds the cache/algorithm version, canonical control and execution roots,
commit and tree identities, bounds, and ordered applicable guidance hashes.
Task state, plans, findings, verification, audit, receipts, worktree status,
profiles, and prompts remain live per-run evidence and are never cached under
HEAD alone. A corrupt entry is never consumed: Revolvr recomputes from Git and
records a nonsecret diagnostic; changed HEAD/tree/guidance/root authority
produces a different key.

Caching never replaces run evidence. Every autonomous invocation retains an
exact role dossier, dossier manifest, complete prompt, profile/schema identity,
and provenance under `.revolvr/runs/<run-id>/`. Cache loss therefore does not
make a recorded run unverifiable. Retention inventory ignores the shared cache
namespace; AW-26 adds no cache eviction or ordinary status/config/TUI cache
population.

## Status, Show, And Receipt Validation

Project autonomous-loop metrics from the live immutable ledger snapshot or an
already verified immutable export:

```bash
go run ./cmd/revolvr metrics show
go run ./cmd/revolvr metrics show --json
go run ./cmd/revolvr metrics show --export <export-id>
```

The versioned projection reports an explicit success numerator/denominator,
typed terminal outcomes, attempts and correction cycles, audit findings and
their explicit dispositions, verification attempts/flaky reruns, recorded
tokens and stable nanosecond durations, exact archive latency, queue throughput,
configured/peak workers, batches, parallel sweeps, and sequential fallbacks.
Missing legacy concurrency evidence is listed as an omission; tokens are
never estimated, a later clean audit never invents finding resolution, and a
flaky classification is not converted into a pass. Logical operation and
occurrence identities deduplicate replay, so equal live and exported history
produces byte-identical canonical JSON. The command is read-only: it does not
run tasks, verification, archive verification, retention, notifications,
Codex, Git, a daemon, or workers.

The source-of-truth AW-30 evaluation suite is
`internal/autonomousmetrics/TestDeterministicEvaluationScenarios`. It uses
fixed UTC clocks and stable typed fake occurrences for straight success,
correction, clean re-audit, conditional skips, no progress, needs input,
blocked queue yield, and crash-finalization replay. It invokes no live model,
network service, notification hook, or repository runtime. Live dogfood remains
the separate opt-in script described below.

Show aggregate task and run state:

```bash
go run ./cmd/revolvr status
```

Inspect one recorded run:

```bash
go run ./cmd/revolvr show <run-id>
```

`show` prints the run summary, timestamps, Codex and verification diagnostics,
commit SHA when present, artifact paths, and event timeline.

Inspect one active autonomous task or tracked archive without changing runtime
state:

```bash
go run ./cmd/revolvr task show <active-task-id-or-archive-id-or-task-id>
go run ./cmd/revolvr task why <active-task-id-or-archive-id-or-task-id>
```

`task show` renders the canonical plan and ordered steps, acceptance matrix,
finding lifecycle, attempt counters and budgets, structured operator input,
verification/audit/workspace/finalization facts, exact provenance references,
and typed omissions. `--json` emits the validated deterministic
`autonomous-task-view-v1` projection. `task why` is the focused routing view:
it distinguishes the latest accepted decision, a still-admitted action,
scheduler readiness, and the next supervisor decision. Revolvr does not infer
that next decision from lifecycle or plan position; when no current admission
exists it reports `undetermined_requires_supervisor`.

Both commands are read-only. Active views bind the canonical task and state
identities and reject a changing snapshot. Archive views use strict archive
`show` evidence but do not silently run full archive verification, so they
report `unverified` until the separate `archive verify` command is used.
Mixed-pass tasks receive an explicit autonomous-evidence-unavailable view.
Configured secret values are redacted from human and JSON output.

Validate one recorded run receipt against the ledger and artifact files:

```bash
go run ./cmd/revolvr receipt validate <run-id>
```

Receipt validation checks the finalized verdict, timestamp, commit SHA,
changed files, verification results, and recorded artifact paths.

## Development Checks

Use the repository checks before changing code:

```bash
gofmt -w <changed-go-files>
go test ./...
go run ./cmd/revolvr --help
go run ./cmd/revolvr config check
go run ./cmd/revolvr status
```

Run the local CLI smoke test without invoking Codex:

```bash
./scripts/smoke-local.sh
```

The smoke test builds a temporary binary and exercises `init`, `task add`,
`task list`, `config check`, and `status` in a temporary workspace.

Run the `run --once` integration smoke test without invoking real Codex:

```bash
./scripts/smoke-run-once-fake-codex.sh
```

This smoke test builds a temporary binary, initializes a temporary Git repo,
points `codex.executable` at a strict fake Codex executable, verifies a
deterministic generated file, checks the completed task/run state, and confirms
the committed receipt and run artifacts.

Run the matching verification-failure smoke test without invoking real Codex:

```bash
./scripts/smoke-run-once-fake-codex-verification-failure.sh
```

This smoke test uses a strict fake Codex executable that makes a deterministic
file change, then intentionally fails local verification. It checks that the run
fails cleanly, the task is blocked, no commit is created, and run diagnostics
and artifacts are still recorded.

Run the opt-in live dogfood check with real Codex:

```bash
./scripts/dogfood-live.sh
```

This resets `.revolvr/`, creates a tiny task, runs one real Codex pass, and
checks the finalized receipt, ledger output, commit, receipt validation command,
and clean-worktree consistency.
