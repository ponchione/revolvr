# Agent Decisions

## AW-31 Bounded Parallel Queue Workers (2026-07-12)

- Queue concurrency policy is strict `autonomous-queue-policy-v1`, included in
  `revolvr-effective-run-config-v5`, defaults to one worker, and has a fixed
  harness cap of four. CLI override is `run --queue/--daemon --workers N`; the
  TUI remains explicitly sequential.
- `internal/autonomousqueue` owns durable batch admission and reconciliation,
  not task lifecycle work. `autonomous-queue-operation-v2` persists a complete
  ordered batch before launch, with monotonic selection sequence, batch/slot,
  exact scheduling fingerprint/authority, task and task-operation identities,
  slot state/outcome, worker bound, peak, batches, and fallback reason.
- Admission retains AW-23 authority: start with canonical ready order and
  re-run the exact snapshot classifier against the occupied IDs after every
  candidate. Dependency/conflict uncertainty never broadens overlap. A missing
  classifier or no provably safe next candidate reduces the batch and records
  a typed sequential fallback.
- Task-operation IDs hash queue operation, selection, batch, slot, task,
  scheduling fingerprint, and worker bound. Stable IDs and result order never
  use goroutine identity, PID, map order, completion timing, or ambient time.
- Workers receive child contexts and exact persisted slot material. The
  coordinator owns queue progress/checkpoint/history/ledger mutation and
  reconciles returned evidence in slot order. Safe local terminal-for-now
  results do not cancel peers. Global cancellation cancels and joins all
  workers. Safety preserves the first canonical slot's authority after all
  already-admitted peers reach their task-run boundary. Panic/ambiguous/runner
  failures become unsafe slot evidence and prevent new admission.
- Restart runs only admitted unresolved slots with their original AW-22 IDs;
  terminal slots and outcomes are never duplicated. Existing v1 sequential
  operations remain readable/replayable and nonterminal pinned selections
  migrate forward at one worker. Queue ledger v3 adds real concurrency facts;
  v2 sequential evidence remains decodable/exportable and is not reinterpreted.
- Lock order is: outer autonomous-execution lease; queue operation lease; then
  worker-owned existing boundaries as needed (Git-admin before task state,
  child publication, or workspace source writer according to their owners).
  The coordinator holds no shared mutex across model, verification, or source
  work. Queue checkpoint/ledger publication happens after worker joins and is
  coordinator-only. Independent SQLite stores use their existing WAL/busy
  ownership; Git/ref/state effects retain existing explicit locks.
- Archive/reopen mutation now nonblockingly takes the outer execution lease
  before Git-admin and task-state, refusing while a direct run or queue is
  active. Retention retains its existing retention lease then nonwaiting outer
  execution probe. Queue execution never implicitly archives or retains.
- Notifications remain post-lease, one terminal queue adaptation with no
  worker dispatch. AW-30 metrics add configured/peak workers, parallel sweeps,
  batches, and fallbacks from queue ledger v3; older queue evidence produces an
  explicit concurrency omission. CLI/TUI render only durable attribution.

## AW-30 Metrics And Deterministic Evaluations (2026-07-12)

- Pure metrics projection lives in isolated `internal/autonomousmetrics` with
  schema `autonomous-loop-metrics-v1`. Its public contract is strict,
  map-free, deterministically ordered, caller-owned, canonical JSON with one
  final newline, and contains source references plus explicit omissions.
- Both sources adapt to `ledger.Snapshot`. Live reads use immutable read-only
  SQLite; `ledgerexport.ReplaySnapshot` verifies the immutable export before
  rebuilding the same logical runs/events. Canonical source identity hashes
  logical evidence rather than its live/export location, so identical history
  produces identical metric bytes.
- Logical terminal task operation ID, attempt ID/kind, audit run, verification
  run/occurrence, archive ID, finalization operation, and queue operation are
  the deduplication keys. Exact duplicates/replay are ignored; materially
  conflicting reuse fails. `no_task` is not a terminal task operation.
- Success numerator is completed terminal task operations; denominator is all
  counted terminal task operations. Raw stop authority is retained while
  rendering completed, blocked, needs-input, safety, budget, no-progress,
  cancelled, max-cycle, and unsafe classes distinctly.
- Actual AW-15 completion tokens and nanosecond durations are counted only when
  recorded. Missing token values are explicit and never estimated. Ledger run,
  task-operation, verification, and queue durations use recorded stable units
  and no ambient `now`.
- Audits are keyed by exact audit run. Findings are introduced once by exact
  report evidence and joined only to explicit durable resolution dispositions;
  a later clean audit never manufactures resolution. Verification ordinary
  attempts, failures/passes, flaky classifications, reruns, timeout,
  cancellation, missing-command, and runner-error facts remain distinct.
- Task-run/queue/archive/finalization owner payloads advance compatibly to v2
  where exact timing/statistics were absent. Tiered verification ledger events
  gain `autonomous-verification-ledger-event-v1`. Relevant older payloads yield
  typed omissions; unknown current schemas/fields/enums fail rather than being
  guessed.
- Archive latency is recorded only from the archive owner's exact matching
  terminal/archive UTC authority. Queue throughput is tasks run divided by
  durable queue nanoseconds, with sweeps, selections, ordered outcomes, stop
  reasons, and drained counts. AW-30 defines no concurrency metric or worker.
- App owns source loading, export verification/replay, configured-secret
  refusal, and pure projection invocation. CLI owns stable `metrics show`,
  `--json`, and `--export` rendering. The path is read-only and cannot dispatch
  notifications, run archive verification, mutate retention, execute workflow
  work, invoke Git/Codex, or populate ordinary status/TUI views.
- The source-of-truth evaluation suite is fixed-time typed fake evidence in
  `internal/autonomousmetrics`; it composes production owner contracts and the
  pure projector without duplicating production execution. It covers exactly
  the eight AW-30 scenarios. Existing owner failure-injection tests remain the
  crash/persistence authority, and live dogfood remains separate and opt-in.
- AW-31 remains the sole owner of bounded parallel workers, overlap analysis,
  concurrency configuration, and parallel scheduling.

## AW-29 External Notification Hooks (2026-07-12)

- Notification delivery is isolated in `internal/autonomousnotification`.
  Canonical source owners remain authoritative; notification code cannot mutate
  tasks, state, questions, queues, daemons, finalization, archives, Git, ledger,
  retention, or scheduling and cannot invoke a model or recurse into itself.
- The closed event enum is exactly `task_completed`, `task_blocked`,
  `task_needs_input`, `safety_stop`, `queue_drained`, and `daemon_failed`.
  Unknown events fail. Task and queue events require exact terminal durable
  operations; typed input additionally requires its AW-17 question history;
  daemon failure requires a durable queue occurrence. Ordinary errors and prose
  never become safety, input, completion, or failure authority.
- Payload schema is `revolvr-notification-payload-v1`: compact canonical JSON
  plus one newline, map-free fields, deterministic event/delivery IDs and UTC
  occurrence, repository/config/policy identities, typed outcome facts, fixed
  reference applicability, sorted omissions, and bounded redacted detail. A
  task completion does not fabricate or verify an archive reference.
- Policy schema is `revolvr-notification-policy-v1`, disabled by default, and
  advances the effective identity to `revolvr-effective-run-config-v4`.
  Enabled authority is a strict event allowlist, one exact executable and
  ordered argv, fixed repository-root working directory, ordered replacement
  environment names, timeout/caps, positive bounded attempts, and bounded
  retry delay. Policy normalization clones caller slices and canonically orders
  events without changing argv/environment order.
- Hook environment values are loaded only at delivery and every environment
  name must be covered by AW-19 redaction. Hooks inherit no ambient environment.
  Configured values and policy material containing them are refused/redacted;
  only names and nonsecret bounds enter config identity and operator output.
- Delivery identity hashes the exact durable source occurrence, event, complete
  redacted payload material, effective config, and hook policy. Storage is
  `.revolvr/autonomous/notifications/<delivery-id>/` with immutable intent and
  payload, immutable history-before-journal transitions, strict readback,
  content conflict refusal, safe path/link/mode checks, and an operation lock.
- Intent and exact payload are durable before invocation. Attempts record exact
  bounded authority, timing, exit/timeout/cancellation/runner classification,
  redacted capped output/error, truncation, retryability, and terminal stage.
  Nonzero/timeout failures alone retry; attempts/delay are bounded and
  cancellation-aware. Valid history can reconstruct a missing/lagging journal;
  interrupted running work consumes an attempt before any restart retry.
- Success is terminal locally. Exact replay after success starts no process;
  terminal failure remains inspectable and warns on replay. Revolvr does not
  promise external exactly-once behavior across a crash after receiver action
  but before synchronized local success; identical retries carry one stable
  receiver idempotency key.
- App adapters run only after the global autonomous-execution lease is released.
  Queue-internal task execution does not notify; the enclosing queue adapts its
  durable ordered task outcomes once. No source/state/Git/admin/child/archive/
  retention lock is held during external execution. Hook failure is persisted
  and observed but source result/error precedence and every lifecycle byte stay
  unchanged.
- `notification list/show` is the only new operator management surface. It is
  redacted, bounded, and read-only; it cannot resend, edit, delete, dispatch, or
  retry. Notification outbox evidence is not inserted into ledger/export or
  AW-25 retention candidates, so those versioned owners require no schema or
  pin-closure change.
- Disabled hooks do no executable lookup, environment load, outbox/lock/temp/
  ledger creation, or process execution during task/queue/daemon work. AW-27
  views and AW-28 TUI refreshes remain read-only and never dispatch.
- AW-29 adds no metric/evaluation projection and no concurrency. AW-30 and AW-31
  retain those respective boundaries.

## AW-28 TUI Autonomous Workflow Visibility And Controls (2026-07-12)

- The TUI's rich workflow detail consumes only `autonomousview.View`; it does
  not own a parallel evidence schema, parse autonomous JSON, infer supervisor
  decisions, or open task/state/archive/ledger stores. App callbacks remain
  the only repository/runtime boundary and CLI only wires those callbacks.
- Workflow is an additive `6` view. Existing Dashboard/Tasks/Runs/Run Detail/
  Preflight/Help rendering and number keys remain compatible. A bounded app
  selector summary joins strict canonical active-task discovery with strict
  archive-manifest discovery so active, archive-only, empty, and mixed-pass
  repositories remain navigable without directory crawling in the TUI.
- Selector controls stay separate from display strings. Exact task/archive IDs
  remain callback authority in model state, while app-redacted labels and the
  redacted AW-27 view are the only rendered identities. Selector discovery is
  observational and never loads state, verifies an archive, creates a cache,
  or opens writable runtime evidence.
- Selector and detail loads are asynchronous Bubble Tea commands identified by
  exact selector plus a monotonic model token. Stale responses cannot replace
  current evidence. Refresh and operation completion reload deliberately, task
  selection is preserved by task ID, and active-to-archive movement follows
  that identity rather than retaining an obsolete list index.
- Workflow rendering uses stable plain-text sections before Lip Gloss styling.
  The existing width-aware wrapper and shared viewport own narrow rendering;
  page/top/bottom keys own long-detail scrolling. Statuses, dispositions,
  budgets, gate facts, archive verification state, and terminal reasons are
  textual rather than color-only.
- Structured input mutation belongs in app `AnswerAutonomousInput`, which
  reloads exact current active task/state/question/option authority, uses AW-17
  CAS operations to persist an explicit operator answer and then resume, and
  redacts every returned error. A persisted answer plus failed resume is a
  typed partial success; retry recognizes and resumes the same exact answer
  without recording a duplicate. The TUI never edits state/task files.
- Input recommendations are evidence only. The Workflow answer control starts
  with selection `-1`, displays every option and recommendation, requires an
  explicit option movement, then requires a separate confirmation enter. Only
  a current typed waiting question on an active task is answerable; legacy,
  archive, stale, missing, answered-with-another-option, or absent authority
  fails closed.
- Task `U` and queue `Q` share the established preflight and one-active-TUI-
  operation state. App/domain revalidation and the global autonomous execution
  lease remain authoritative. Queue defaults match CLI policy (100 tasks, 50
  cycles), uses `app.RunQueue`/AW-24 rather than a model loop, streams typed
  progress/outcomes, and retains replay/stop evidence.
- `c` cancellation only cancels the context created for the current
  TUI-started mixed pass, loop, task run, or queue. Repeated cancellation is
  idempotent, new starts remain refused until the domain returns its typed
  terminal result, and no lifecycle/task/workspace is synthesized or deleted
  by the model.
- Workflow archive reads use strict AW-27 Show and remain explicitly
  `unverified`; AW-28 never invokes archive Verify/create/reopen merely to
  render. It also adds no daemon-service control, notification, metric/
  evaluation, retention action, workspace cleanup, model invocation, network
  request, or parallel worker. AW-29 through AW-31 retain those boundaries.

## AW-27 Read-Only Autonomous Evidence Views (2026-07-12)

- Pure operator projection belongs in isolated `internal/autonomousview`, not
  app orchestration, Cobra callbacks, status/timeline strings, the TUI, or the
  dossier cache. Schema `autonomous-task-view-v1` is strict, map-free,
  deterministic, deep-clones caller evidence, and performs no I/O.
- The app owns exact selector resolution and read-only evidence assembly.
  `ShowAutonomousTask` considers exact active task IDs plus archive/task IDs,
  rejects active/archive and multi-archive ambiguity, and never silently
  chooses one authority.
- Active authority is the duplicate-checked canonical task plus its exact
  `autonomous_state_path` loaded through `autonomousstate`. Optional committed
  audits and the bounded latest decision artifact may enrich the view, but
  task/state identities are reloaded after collection and any changing
  canonical snapshot fails closed.
- `autonomousarchive.LoadEvidence` is the archive-side strict Show extension:
  it reads exact archived task, terminal state, and applicable frozen
  completion evidence by manifest identity. It deliberately does not perform
  full archive Verify and returns no `verified now` claim. Non-completed
  dispositions render AW-20 completion evidence as not applicable.
- Required malformed task/state/archive authority is a hard read failure.
  Optional malformed audit history becomes a typed omission while other valid
  state renders. Malformed latest accepted decision payload makes admission
  unknown; reference presence alone never authorizes a route. Unknown future
  schemas/enums remain strict failures at their canonical owners.
- Plans retain plan ID/revision/predecessor/completed status, source order,
  step status/description/rationale, and typed evidence. Acceptance keeps
  pending/satisfied/waived/not-applicable distinct with rationale/evidence.
  Findings join committed audit identities with durable resolution state and
  never disappear merely because the newest optional history is unavailable.
- Attempts expose total/per-action counts, consecutive failures, exact event
  references, stops/circuit breakers, and distinct unset/limited/unlimited
  retry/elapsed/token/action budgets. Limited remaining values clamp display at
  zero while exhausted/over-limit authority remains explicit; token usage is
  never estimated from dossier size.
- Typed input displays the exact question revision/content identity, ordered
  options, and recommendation as evidence only; it never selects an answer.
  Durable answers retain answer/option/actor provenance, and legacy reason-only
  needs-input remains explicitly non-answerable.
- Why output has four separate fields: latest accepted decision, currently
  admitted action, scheduler readiness, and next supervisor action. Durable
  lifecycle/decision agreement is required for current admission. Otherwise
  `next_supervisor_action` is `undetermined_requires_supervisor`; no plan step,
  lifecycle name, profile, task prose, receipt, or cache entry predicts it.
  Reason ordering is explicit: terminal, finalizing, input, blocked,
  budget/breaker, scheduler, plan/acceptance/finding/verification/audit gates,
  then undetermined-next.
- Ordinary task viewing does not run archive Verify merely to satisfy a
  dependency. Active dependencies may be explained directly; an archived
  dependency remains `archive_dependency_unverified` until the separate
  verification boundary supplies scheduler authority. Views never invent
  conflict occupancy.
- CLI owns rendering through exactly `task show` and `task why`; no per-field
  command family was added. Human output is stable plain text with explicit
  absence/mode words and UTC RFC3339Nano times. `task show --json` is the
  canonical validated projection, not an alternate untyped payload.
- AW-19 redaction remains harness-owned. App loads configured secret values,
  projects typed evidence, then redacts every string in the cloned result and
  all errors before CLI rendering. Secret values never enter human/JSON output,
  diagnostic references, or help fixtures.
- AW-27 viewing creates no ledger, WAL, lock, cache, journal, receipt, export,
  or repair sidecar and acquires no mutation lease. It does not run Git, Codex,
  verification, audit, archive verification, workspace recovery, retention,
  notification, metrics, or parallel workers. AW-26 dossier cache remains
  completely noninteracting.
- Existing task list/status/run show/archive/config/doctor/receipt/TUI output is
  preserved. AW-28 remains the sole owner of TUI evidence screens and controls;
  AW-29 through AW-31 remain notifications, metrics/evaluations, and bounded
  parallelism respectively.

## AW-26 Dossier Cache And Role Projection (2026-07-12)

- Immutable dossier-context caching lives in isolated `internal/dossiercache`,
  never in prompt construction, cycle routing, CLI/TUI, ledger, or retention.
  Namespace/schema are `.revolvr/cache/dossier/v1/<key>/` and
  `revolvr-dossier-cache-v1`; producer algorithm is
  `git-tree-path-map-v1`. Entries are immutable `manifest.json` plus
  `repository-map.md` with synchronized temporary-directory publication.
- A cache key is canonical JSON SHA-256 over schema/algorithm, hashed canonical
  control and execution roots, exact commit/tree IDs, map path/byte bounds, and
  ordered applicable guidance path/SHA-256/byte-size identities. No task ID,
  clock, PID, random value, mtime, cache counter, model output, receipt,
  environment value, or configured secret value is key authority.
- Only exact committed-tree repository-map derivation is cached. Task/spec,
  state, plan, acceptance, findings, input, attempts/budgets, workspace,
  verification, audit, receipts, history, worktree status/diff, profiles,
  safety/config, schemas, routes, and prompts are live. Same reusable map bytes
  therefore cannot carry task/profile/role control evidence across calls.
- Repository mapping consumes only bounded `rev-parse`/`ls-tree -r -z` object
  metadata for the exact commit. It sorts canonical tracked paths, excludes
  `.git`/`.revolvr`, labels conservative file kinds and symlink/submodule
  metadata, and truncates only on whole paths with explicit counts. It runs no
  build, hook, test, container, network request, model, language server, or
  repository binary.
- Cache lookup is strict and read-only. Unsupported/noncanonical JSON,
  unknown fields, source/key/hash/size/count/token disagreement, symlinks,
  hard links, unsafe modes, oversize, partial publication, or unsafe parents
  never yield content. A valid hit is byte-equivalent to recomputation.
  Corruption/unsupported evidence causes safe exact Git recomputation and a
  deterministic diagnostic in run provenance; corrupt evidence is neither
  deleted nor overwritten and never becomes workflow authority.
- Pure role projection remains in `internal/autonomous` and supports exactly
  `supervisor`, `planner`, `implementer`, `auditor`, `corrector`,
  `documentor`, and `simplifier`. Existing validated action/profile pairs map
  to one role; contradictions/future roles fail closed. Schema
  `autonomous-role-dossier-manifest-v1` and policy
  `role-section-matrix-v1` explicitly record every included/omitted section.
- Every role receives exact task, current state, plan, acceptance, Git source,
  guidance, and repository map. Supervisor additionally receives all routing
  gates/history; planner omits current verification/audit; implementer omits
  audit/history; auditor/corrector/documentor/simplifier receive current
  verification and audit/finding authority but omit unrelated run history.
  Exact correction/verification authority continues in the route-specific
  worker prompt. Omission never grants authority.
- Section manifests record exact total/included bytes, whole item counts,
  omission reasons, and deterministic `utf8-bytes-ceil-div-4-v1` estimates;
  final dossier and prompt identities/sizes/estimates are separate. Estimates
  are explicitly not actual Codex usage; receipts/AW-15 remain actual token
  accounting authority.
- Cache use never weakens source freshness. Full content-sensitive source
  snapshots still bracket assembly/supervision and worker admission; assembly
  also reloads task/guidance bytes after cache collection. Changed HEAD/tree,
  guidance, root/worktree, task/state/profile/config/safety, or source evidence
  cannot be admitted from cache presence.
- Every invocation retains an exact per-run dossier and manifest in addition
  to the existing exact prompt/profile/schema/output/provenance artifacts:
  `supervisor-dossier.md`, `supervisor-dossier-manifest.json`,
  `worker-dossier.md`, and `worker-dossier-manifest.json`. Ledger extraction
  and export know these paths. Run provenance binds cache result/key/entry
  manifest, role/policy, dossier, prompt, task/source/profile/config/safety,
  and run identities; it never points only to shared cache bytes.
- AW-25 retention owns only its existing ledger-derived stream candidates and
  ignores `.revolvr/cache`; AW-26 adds no eviction. Ordinary status, show,
  config, doctor, TUI, archive verification, and GC do not populate or repair
  cache entries. No new CLI/TUI evidence screen or configurable budget was
  added; fixed bounded harness policy is sufficient for this slice.
- AW-27 remains the owner of broad app/CLI evidence and why-next projections;
  AW-28 owns TUI workflow controls; AW-29 notifications, AW-30 metrics and
  evaluations, and AW-31 parallel workers remain out of scope.

- The repo itself is the durable state for autonomous loop runs.
- AW-25 policy schema is `revolvr-artifact-retention-policy-v1`; it is strict,
  map-free, harness-authored, and part of `revolvr-effective-run-config-v3`.
  Defaults are mutation disabled, 20 recent runs pinned, seven-day compression,
  90-day prune age, 64 KiB compression threshold, Codex JSONL/stderr classes,
  pruning disabled, verified export required for pruning, 100 files/1 GiB per
  operation, and 256 MiB decompression cap. Secret values and clocks/callbacks
  never enter this identity.
- `internal/artifactretention` exclusively owns candidate inventory, pin
  closure, deterministic plans, compression, GC quarantine, journals, and the
  global retention lease. Candidates come only from exact ledger-owned Codex
  JSONL/stderr paths; tracked tasks/archives, canonical state/history,
  completion, queue/task-run/child/workspace/control files, locks, source, Git,
  and unknown files are never generic age targets.
- Pinning is conservative and transitive over both exact artifact paths and run
  identities found in strict autonomous/archive/recovery JSON. Every active
  task pins its run history; nonterminal/recent runs pin directly; incomplete
  operations pin recovery evidence; cleaned GC history does not permanently
  pin. Missing, unsafe, ambiguous, foreign, symlinked, hard-linked, or changed
  evidence blocks rather than widening deletion authority.
- Compression format is standard-library deterministic gzip with zero/fixed
  headers and adjacent canonical `revolvr-compressed-artifact-v1` manifest.
  Logical identity remains the original path/SHA-256/size. Readers accept
  exactly one original or admitted compressed representation and reject dual,
  corrupt, divergent, oversized, or partial forms. An original must first be
  durably compressed; only a later operation may prune it.
- `internal/ledgerexport` owns schema `revolvr-ledger-export-v1` plus canonical
  JSONL record schema `revolvr-ledger-export-record-v1`. Exports bind immutable
  read-only SQLite identity and high-water range, every run field, global event
  IDs/order/times/types, exact payload bytes, explicit legacy/versioned schema,
  predecessor coverage, and compressed artifact facts. Live ledger rows remain
  authoritative and are not deleted by AW-25.
- Export verification is read-only and checks canonical manifests/records,
  hashes/sizes/counts/range gaps/order, source-ledger coverage, predecessor,
  payload schema agreement, compressed facts, and configured-secret absence.
  Replay reconstructs logical run/event terminal evidence in memory and never
  replaces the ledger, executes work, invokes Git/Codex, or claims pruned source
  artifact bytes were recreated.
- GC planning is the canonical dry-run and must create nothing. A plan binds
  one frozen UTC time, operation/policy/config/repository/ledger/high-water,
  exact inventory/pins, ordered retain/compress/prune actions and expected
  identities/bytes, limits/remaining work, export requirement, and a
  content-derived plan identity. Apply never silently replans a different
  action set and requires the exact printed plan identity.
- GC durability schema is `revolvr-artifact-gc-journal-v1` under
  `.revolvr/retention/gc/<sha256(operation-id)>/`. Synchronized immutable
  history precedes mutable journal replacement. Stage order is admission,
  verified/replay-validated export, deterministic compression, operation-owned
  quarantine, completion, cleanup, then cleaned. Cancellation starts no new
  action and records resumable authority; retries reconcile exact orphan/dual/
  quarantine effects and conflict on changed bytes.
- Retention lease order is outermost, then the existing autonomous-execution
  lease; Git-admin and child-publication locks are only probed and never held
  during scans/export/compression, and live source writers refuse admission.
  Ledger identity/high-water and active/recent/control pins are revalidated
  before every mutation. Ordinary run/status/show/config/doctor/TUI/archive/
  queue operations never trigger retention work.
- App and CLI expose only narrow plan/apply/resume/inspect and ledger
  export/verify/replay operations. `artifact gc` defaults to dry-run and apply
  needs `--apply --plan-id`; there is no `--force`, arbitrary deletion root,
  implicit apply time, broad evidence screen, or TUI control in AW-25.
- AW-24 queue ownership lives in isolated `internal/autonomousqueue`, above the
  pure AW-23 scheduler and exact AW-22 task runner. It owns repeated fresh
  selection, queue-local fairness, durable in-flight recovery, ordered task
  outcomes/statistics, bounds, summaries, and queue stop classification; it
  never performs task effects, archive creation, input answers, cleanup, or
  parallel execution.
- Queue schema `autonomous-queue-operation-v1` lives under
  `.revolvr/autonomous/queues/<operation-id>/`. It binds mode, effective config,
  safety declaration, task bound, sweep/time/sequence/stage, exact scheduling
  fingerprints, deterministic derived AW-22 operation IDs, authority-bound
  exclusions, outcomes, remaining work, and terminal reason. Immutable
  synchronized history precedes atomic checkpoint replacement; exact terminal
  replay starts no task and changed material conflicts.
- Queue fairness never changes canonical scheduling metadata. A safe yielded
  task is excluded only while the SHA-256 authority over its exact task bytes,
  state identity, status, and lifecycle remains unchanged. Fresh deterministic
  scheduler order still governs every nonexcluded ready candidate.
- Queue stop precedence is input waiting, blocked-only waiting, dependency or
  yielded waiting, then drained when no active pending autonomous work remains.
  Queue-owned task bounds produce `budget_exhausted` with remaining-ready
  evidence. Caller cancellation, trusted safety stops, and unsafe/ambiguous
  evidence remain distinct and stop all new starts.
- AW-24 derives each AW-22 ID from queue ID, monotonic selection number, task
  ID, and exact preselection scheduling fingerprint, persists it before start,
  and always reopens that identity after interruption. AW-22 remains a pinned
  single-task primitive and never sees surplus queue budget.
- The global `.revolvr/locks/autonomous-execution.lock` is owned by
  `internal/autonomousexec` at the app/coordinator boundary. Direct AW-22 runs
  and bounded queue sweeps acquire it outermost; the queue calls an explicit
  already-leased app path. Git-admin, task-state, child-publication, workspace
  source-writer, and AW-22 operation owners never acquire it, preventing lock
  inversion and self-deadlock.
- `internal/autonomousdaemon` owns only foreground polling, exact pre/post-sweep
  authority comparison, stable debounce, wake observation, and cancellation.
  It holds no execution/source/state/Git/child lock while waiting and requires
  explicit AW-19 `fully_unattended` authority. The fingerprint covers active
  task bytes, exact autonomous state identities, verified archive authority,
  and completed child publication while excluding queue/ledger self-effects,
  mtimes, and timestamps.
- Queue ledger evidence uses one deterministic run and versioned admitted,
  selection, task-stopped, daemon-wake, and stopped events with exact content
  comparison. The run completes only after the terminal queue checkpoint is
  readable. Wake count/fingerprint are bounded to the next daemon sweep rather
  than retaining unbounded raw filesystem events.
- The AW-24 CLI surface is `run --queue` and foreground `run --daemon`, mutually
  exclusive with mixed-pass and exact-task modes. App owns assembly and typed
  operations; CLI owns validation/rendering. Current TUI behavior is preserved,
  broad controls remain AW-28, and queue concurrency remains exactly one until
  AW-31.
- AW-22 exact-task orchestration lives in `internal/autonomoustaskrun`; its
  operation lease is separate from Git-admin, state, and source-writer locks,
  and no other owner may acquire it, so bounded effect owners can retain their
  established lock orders without inversion.
- `autonomouscycle` exposes one nil-compatible pre-worker admission callback
  only after the route and admission source are validated. AW-15 admission is
  performed there; worker execution is never charged after it starts.
- AW-22 pins one active `autonomous-v1` task per versioned durable operation
  under `.revolvr/autonomous/task-runs/<operation-id>/`; immutable history is
  authoritative when it is newer than the synchronized mutable checkpoint,
  and a separate operation lease never nests inside Git-admin, state, or
  workspace source-writer locks.
- Attempt-consuming correction/document/simplify decisions use the same AW-10
  pre-worker admission seam as ordinary work. The already-admitted one-shot
  result is then continued only by `autonomouscorrection` or
  `autonomousoptional`, which retain final verification, finding resolution,
  fresh re-audit, attempt completion, and optional occurrence ownership.
- An explicit supervisor block is a canonical one-way transition owned by
  `internal/autonomousblock`: it re-evaluates the pure route, writes immutable
  `autonomous-block-transition-v1` history before exact state CAS, and
  publishes blocked task status without running a worker or charging an
  attempt.
- Autonomous task-run ledger summaries use one deterministic operation run
  plus versioned admitted/cycle-started/cycle-completed/restarted/stopped
  events. Exact retry deduplicates those summaries; completed loop evidence is
  written only after the bounded terminal owner has succeeded.
- AW-22 completion is terminal-but-unarchived. The loop invokes AW-20 for a
  validated `complete` route but never infers AW-21 archive time, provenance,
  or administrative-commit authority; archive remains a separate command.
- Each loop pass starts as a fresh `codex exec` session.
- Do not use resumed Codex sessions for this workflow.
- Work is limited to one small, verifiable task per pass.
- Existing project patterns should win over new abstractions unless a task explicitly asks for a change.
- Tests, build output, and typechecks are the source of truth.
- `.revolvr/` is local runtime state for the Revolvr CLI and is ignored by Git.
- CLI-initiated harness runs default to Codex dangerous bypass/yolo mode for unattended autonomy; repo config can disable it with `codex.dangerously_bypass_approvals_and_sandbox: false` or `codex.yolo: false`.
- `revolvr init` keeps runtime state local by adding `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, avoiding tracked `.gitignore` changes.
- Dogfood readiness is exposed as a top-level `revolvr doctor` command so preflight checks stay separate from config inspection.
- `revolvr run` streams summarized Codex progress events to stdout while keeping raw Codex JSONL and stderr as run artifacts; console output should stay human-readable and artifact-backed.
- `revolvr receipt validate <run-id>` treats the ledger run row plus run events as the source of truth for finalized receipt validation, and recorded artifact paths must exist on disk.
- `revolvr task retry <task-id>` is the preferred blocked-task recovery command; it reuses the same blocked-to-pending task update as `task unblock` so task identity and prior run history remain intact.
- `revolvr run --max-passes` must print one concise final loop summary and stop before a failed dirty pass or blocked outcome can cascade into blocking unrelated tasks; only clean failed outcomes may repeat, and those stop after two consecutive failures.
- The real-Codex live dogfood check is an opt-in script (`scripts/dogfood-live.sh`) because it removes local `.revolvr/` runtime state and creates a Git commit; it must require a clean source worktree before resetting state.
- Read-only CLI inspection should move orchestration into `internal/app` while keeping exact command rendering in `internal/cli`; `status` and `show` are the first commands following this boundary.
- Receipt validation orchestration now follows the same boundary: `internal/app` resolves state, loads run history, and invokes receipt validation, while `internal/cli` keeps exact command rendering and failed-check exit formatting.
- Task command orchestration follows the same boundary: `internal/app` owns task add/list/retry/unblock state resolution, task-file access, and blocked-to-pending transitions, while `internal/cli` keeps the exact command output formatting.
- Run orchestration follows the same boundary: `internal/app` owns run config loading, single-pass execution, loop stats, stop reasons, outcome errors, and `run --max-passes` guardrails; `internal/cli` keeps exact run summary, progress, and loop summary rendering through callbacks.
- Charm TUI dependencies are pinned to the newest stable tagged releases that preserve the repository's Go 1.22 module baseline: Bubble Tea `v1.3.4`, Bubbles `v0.20.0`, and Lip Gloss `v1.1.0`. Newer Bubble Tea and Bubbles tags require Go 1.23 or 1.24, so they are intentionally avoided until the project raises its Go floor.
- `revolvr tui` follows the app boundary: `internal/cli` loads `internal/app.Status` and passes that snapshot to `internal/tui`, while `internal/tui` owns Bubble Tea program execution and rendering.
- TUI run controls follow the same app-boundary callback pattern as other TUI actions: `internal/cli` wires `RunOnce` to `internal/app.RunOnce`, while `internal/tui` owns readiness guards, one-active-run state, progress rendering, cancellation, and post-run refresh/detail loading.
- The TUI shell uses an explicit view model with number-key navigation: `1` Dashboard, `2` Tasks, `3` Runs, `4` Run Detail, and `?` Help. Switching views must preserve the loaded status snapshot, selected run, and opened run detail unless state is refreshed to uninitialized.
- TUI layout renders plain text first, wraps header/content/key-help lines to the current terminal width, and only then applies semantic Lip Gloss styles. Status words and markers such as `OK`, `FAIL`, `PASS`, `Status: failed`, and `! blocked` remain in the text so important states are readable without color; recent-run rows switch to a compact layout below 72 columns.
- The TUI Tasks view derives list rows and selected-task details from `internal/app.StatusResult.Tasks`, uses its own in-memory selection index, and makes blocked tasks visibly distinct with a text marker so the status is clear without relying on terminal color.
- TUI write actions use explicit model callbacks that are wired by `internal/cli` to `internal/app`; task creation calls `internal/app.AddTask`, then refreshes through `internal/app.Status`, switches to the Tasks view, and selects the added task from the refreshed snapshot when present.
- The TUI Runs and Run Detail views stay read-only and app-boundary-backed: run lists use `internal/app.Status` recent runs, detail opening uses `internal/app.ShowRun`, and detail diagnostics are derived from ledger events into separate Summary, Diagnostics, Changed Files, Artifacts, and Events sections. Missing artifact paths, missing changed-file events, receipt warnings, and long event lists must remain visible inside the TUI without requiring `revolvr show`.
- TUI receipt validation stays app-boundary-backed and manually triggered from Run Detail with `v`: `internal/cli` wires the callback to `internal/app.ValidateReceipt`, while `internal/tui` stores and renders the latest validation result for the loaded run with explicit `PASS`/`FAIL` check lines and non-crashing error display for missing or unreadable receipts.
- Doctor/preflight orchestration now belongs to `internal/app.Preflight`: it returns structured readiness checks while preserving the existing doctor check order and detail strings. `internal/cli` renders the established `revolvr doctor` output unchanged, and the TUI uses an app-wired Preflight callback for the `5 Preflight` view plus `p` reruns.
- Next-phase development treats Markdown task import as the bridge from external design/spec chat to Revolvr task files. Initially, acceptance and verification notes should be preserved in task text plus summary rather than expanding task frontmatter; add structured task fields only after the plain-text import flow proves insufficient.
- The initial Markdown task import parser lives in `internal/taskimport` and recognizes repeated `## Task` or `## Task: summary` sections, optional `### Summary`, `### Acceptance`, and `### Verification` subsections, and unknown subsections that are preserved in task text. It returns a local `{Task, Summary}` spec instead of importing `internal/app`, so the next app-level import operation can consume it without a package cycle.
- App-level task import is split into parser exposure and execution: `internal/app.ParseTaskImport` wraps `internal/taskimport`, `ImportTasksFromMarkdown` is the convenience parse-and-import path, and `ImportTasks` accepts already parsed tasks for dry-run/write modes. Dry-run and empty parsed imports must not create task files or `.revolvr/`; write mode validates every task before creating files so persisted list order matches input order.
- CLI task import stays thin and app-boundary-backed: `revolvr task import <path>` reads the Markdown file in `internal/cli`, delegates parsing/importing to `internal/app.ImportTasksFromMarkdown`, prints numbered dry-run rows from parsed task descriptions, and prints created task IDs in parsed order for write imports. Existing `task add` and `task list` rendering remains unchanged.
- README is the operator-facing reference for the chat-to-task import workflow: it documents the minimal Markdown import shape, dry-run/import commands, TUI refresh/preflight/run-once keys, and the rule to avoid concurrent code edits in the same worktree while a harness pass is running.
- TUI next-runnable visibility is derived from the existing `internal/app.StatusResult.Tasks` snapshot rather than new task-store fields: the Dashboard and Tasks view count pending, blocked, and completed tasks locally, treat the first pending task as the next runnable task, and render explicit `Runnable: ready to run` / `Runnable: nothing runnable` text plus a plain `next` list marker so the state is clear without color.
- TUI blocked-task retry follows the existing app-boundary callback pattern: `internal/cli` wires `internal/app.RetryTask` into `internal/tui`, while the TUI validates the selected task status before calling the callback, refreshes after successful retry, keeps the selected task stable by ID, and reports non-blocked selections, missing callbacks, retry errors, and refresh failures inline.
- Run profiles are repo-authored Markdown files under `.agent/profiles/<name>.md`, with `implementer` as the default profile name. Runtime fallback to hardcoded profile text is intentionally avoided; missing, empty, or unsafe profile files should block clearly before Codex starts. Embedded profile text may exist only as `revolvr init` seed/template content.
- Exact context payloads are retained as first-class run artifacts for audit and reuse: each run writes the full Markdown context payload plus `context.json` manifest before Codex starts. Future summaries or compression may be added as additional artifacts, but they must not replace the exact payload that was sent to Codex.
- `.revolvr/runs/<run-id>/context.md` and `.revolvr/runs/<run-id>/context.json` are the only supported run context artifacts; alternate context artifact names and ledger keys are not supported.
- `.agent/tasks/*.md` are the canonical human-authored task specs. Runtime initialization creates task-file directories and ledger state only.
- `internal/runonce` consumes only canonical `.agent/tasks/*.md` task files through `internal/taskfile`; when no pending file task exists, it returns `no_task`. It resolves the selected task's workflow and phase through `internal/passpolicy.Lookup`, loads the mapped repo-authored run profile, and records the selected workflow, phase, and profile name in the task-selected ledger event. Successful runs keep `selected_task` manifest metadata from the exact pre-run task bytes, then after Codex and verification pass they apply the policy transition before the commit gate: implement advances to pending audit only when pre-metadata changed files exist, audit/document/simplify may advance with no source changes when policy permits, and simplify completes the task. Changed files are recaptured after task-file metadata updates so commits and receipts include the durable transition. Blocking/failed outcomes mark selected task files `blocked` without advancing phase; if the commit gate fails after a metadata update, the task is blocked at its original phase. Task frontmatter `profile` remains reserved for a future slice.
- Mixed-pass workflow development treats a durable task and an execution pass as different concepts: one `.agent/tasks/*.md` task can move through multiple phases while each phase still runs as a fresh Codex session.
- Task workflow state belongs in the canonical task file. The initial workflow metadata shape is `workflow: mixed-pass-v1` plus `phase: implement|audit|document|simplify`, defaulting to `mixed-pass-v1` and `implement` for existing task files that omit the keys.
- Revolvr owns phase transitions through a pass-policy model, not through Codex-written task edits. Policy maps phases to profiles and outcome semantics: `implement` uses `implementer` and requires meaningful changes; `audit` uses `auditor`, `document` uses `documentor`, and `simplify` uses `simplifier`, with those non-implementation phases allowed to succeed without code changes when verification and receipts are otherwise acceptable.
- The pass-policy model lives in `internal/passpolicy`. `Lookup(workflow, phase)` is the integration point for later runtime slices and returns profile name, no-change success permission, next phase, and terminal completion state while reusing `internal/taskfile` workflow and phase constants.
- Successful phase advancement must be durable and auditable by updating the selected task file before the commit gate when a commit is appropriate. A task should stay pending while more phases remain and should only become completed after the final successful phase.
- Auto-commit outcome classification uses pre/post-commit HEAD evidence, not the commit command exit alone. Revolvr captures HEAD before staging, resolves it after every commit attempt, retries one failed post-commit lookup, and records an advanced HEAD as committed with its SHA. If post-commit HEAD remains unavailable, the result is explicitly `indeterminate`; runonce blocks and restages the transitioned task snapshot rather than restoring original-phase bytes that may already have been committed.
- App-level read visibility for tasks now comes from canonical `.agent/tasks/*.md` files through `internal/taskfile`: `internal/app.ListTasks` and `internal/app.Status(...).Tasks` adapt each `taskfile.Task` into the `internal/taskmodel.Task` rendering shape using task-file ID, full Markdown context body, H1 title as summary, and preserved task-file status. The adapter resolves workflow and phase through `internal/passpolicy.Lookup` and exposes the mapped run profile plus next phase or `completed`; CLI and TUI renderers consume those display fields without interpreting policy. Ledger/recent-run loading in `app.Status` remains unchanged.
- App-level task write operations also use canonical `.agent/tasks/*.md` files. `internal/app.AddTask` and write-mode `ImportTasks` create pending Markdown task files with generated IDs, `status: pending` frontmatter, H1 titles derived from summary or task text, and task text as the body; dry-run imports and validation/parse failures must not mutate `.agent/tasks/` or `.revolvr/`. `RetryTask` and `UnblockTask` find file-backed tasks by ID and only update `blocked` tasks to `pending`.
- Run timeline projection starts at the app boundary: `internal/app.RunTimeline` converts `ledger.RunWithEvents` into ordered `RunTimelineRow` values with timestamp, phase, status, and concise detail while keeping CLI/TUI rendering for a later slice. Event rows preserve ledger order and `CreatedAt`; missing start/terminal history falls back to the ledger run row, and malformed payloads produce generic rows instead of panics.
- Run timeline rendering is user-facing in both read surfaces: `revolvr show <run-id>` prints a tab-delimited `Timeline:` table immediately after run summary fields and before artifacts/diagnostics/events, while TUI Run Detail prints compact single-line timeline rows after `Summary` and before `Diagnostics`. Both surfaces use `internal/app.RunTimeline`; missing timeline row timestamps render as `none`, and raw TUI `Events` remain visible below artifacts for audit/debug detail.
- TUI bounded multi-pass runs use the existing app-boundary callback pattern: `internal/cli` wires `internal/tui.RunLoop` to `internal/app.RunLoop` with the same configured single-pass runner, while the TUI owns readiness checks, one-active-run state, cancellation, progress rendering, and post-run refresh/detail opening. The durable keybindings are `R` for single pass, `n` to cycle loop max passes through 2/3/5 with default 3, and `L` to start the bounded loop.
- Future `autonomous-v1` model output contracts live in `internal/autonomous` as plain JSON-tagged structs whose `Validate` methods apply Revolvr-owned policy before the output may drive work. Worker actions select one exact role profile (`planner`, `implementer`, `auditor`, `corrector`, `documentor`, or `simplifier`); `complete` and `block` are terminal and select none. Decisions and audits cite typed durable evidence with a reference and concrete detail. Audit findings use explicit `blocking`/`non_blocking` significance plus lower-case kebab-case IDs, and correction decisions are checked against the findings in a validated `changes_required` audit report. This package is not wired into `mixed-pass-v1` runtime behavior.
- Durable `autonomous-v1` execution snapshots use schema identity `autonomous-execution-state-v1`. Each plan revision has a new lower-case kebab-case ID, a monotonic revision number, and an explicit predecessor link; ordered step slices are canonical. Acceptance and finding dispositions reuse AW-01 evidence and finding-ID contracts, while compact decision references reuse AW-01 action/profile compatibility and retain decision, run, artifact, task, and timestamp provenance. Retry/count and elapsed-time budgets use explicit `unset`, `limited`, or `unlimited` modes, with durations encoded as nanoseconds and exact over-limit consumption retained for later AW-15 policy. Contract transition validation protects stable identities and irreversible terminal states only; routing, evidence freshness, completion gates, persistence, and explicit reopen policy remain outside AW-02.
- Autonomous task dossiers are a pure projection in `internal/autonomous`: callers supply exact task/guidance bytes and typed state, audit, verification, recent-run, and Git evidence; the builder performs no live discovery or I/O. Canonical Markdown section order is identity, task/spec, state, plan, acceptance, verification, audit/resolutions, recent runs, Git, guidance, then omission/truncation facts. Recent runs sort newest-first by supplied start time with run ID as the tie-breaker, and a zero history limit includes no runs.
- Task-dossier manifests use schema `autonomous-task-dossier-manifest-v1`. Raw task/guidance sources hash their exact supplied bytes; typed sources hash compact `encoding/json` bytes over structs or deterministically ordered slices; recent-history provenance always hashes the complete ordered history even when rendering is truncated. Manifest JSON is indented with a final newline, while its required hash describes only the final Markdown dossier to avoid recursive self-hashing.
- Direct `.agent/tasks/AGENTS.md` is reserved for applicable nested repository guidance and is not a task document. Every other direct `.md` file under `.agent/tasks/` remains subject to strict task parsing, duplicate-ID detection, and deterministic discovery.
- Autonomous dossier assembly lives in `internal/autonomousassembly` as a read-only boundary over `internal/taskfile`, immutable ledger access, exact receipts, bounded Git commands, and repository guidance. Selected-task history is filtered before its explicit collection limit and Git evidence is accepted only when HEAD remains stable across the complete collection window.
- Codex invocation configuration extends the shared app/runonce path with one canonical YAML shape: `codex.model`, `codex.reasoning_effort`, and `codex.ephemeral`. Missing keys default to `gpt-5.6-sol`, `xhigh`, and `true`; explicit empty model/effort values and non-ephemeral sessions are invalid, reasoning effort is limited to `minimal`, `low`, `medium`, `high`, or `xhigh`, and model validation stays structural rather than using a guessed allowlist.
- Effective run configuration provenance uses schema `revolvr-effective-run-config-v1`: SHA-256 covers compact `encoding/json` bytes for a map-free normalized projection of working directory, Codex/Git/verification/commit/source-lock settings and ordered command/argument slices. Process-local callbacks, stores, clocks, per-run identity, and artifact paths are deliberately excluded, and projection construction copies caller-owned slices.
- Canonical Codex invocation provenance is `codexexec.InvocationProvenance`, prepared before execution from the exact effective config and shared unchanged by `context.json`, `context_built`, and `codex_started`. It records the configured executable, bounded `<executable> --version` result, model, effort, ephemeral/session mode, effective-config schema/hash, absolute working directory, and exact fresh `codex exec` argv; `resume` is forbidden.
- Canonical autonomous role profile names are `supervisor`, `planner`, and `corrector`. `planner` and `corrector` exactly match the AW-01 worker-profile values; `supervisor` is an orchestrator role and is intentionally not a `WorkerProfile` value.
- The supervisor profile has decision-only, read-only authority. It may recommend exactly one structured next action from durable evidence but may not mutate repository/runtime evidence, commit, execute a worker, or take over Revolvr-owned safety, verification, transition, retry, or terminal-state policy.
- The planner profile has planning-only authority. It may return a structured new or revised durable plan grounded in supplied evidence, but it may not implement, verify, persist the plan, mutate task/runtime state, or route work.
- The corrector profile has finding/failure-scoped mutation authority. It may change only source/tests required by explicit verification failures or cited audit finding IDs and report the evidence; finding disposition, re-verification, re-audit, commits, retries, and overall completion remain Revolvr-owned.
- `internal/prompt.DefaultRunProfileTemplates` remains the canonical init seeding source, while `.agent/profiles/<name>.md` remains the runtime source loaded by `prompt.LoadRunProfile`. Init normalizes a template to one final newline only when creating a missing file and never reconciles or overwrites an existing repository-authored profile.
- Fresh supervisor execution lives in isolated `internal/supervisor`, not `runonce`, CLI, TUI, or worker lifecycle code. Its package-level API requires caller-supplied task identity, validated dossier, optional current audit, writable ledger, exact AW-05 invocation inputs, and injected command/snapshot dependencies; autonomous task discovery and selection remain deferred to AW-08.
- The AW-07 supervisor output schema is a deterministic standard-library projection of the single AW-01 `SupervisorDecision` contract. Codex receives it through a typed repository-contained `codexexec` output-schema path, while strict unknown-field decoding, an explicit EOF check, `SupervisorDecision.Validate`, exact task identity, and `ValidateCorrectionDecision` remain authoritative after execution.
- Supervisor raw output and canonical decision output are distinct artifacts: `supervisor-output.json` preserves exact output-last-message bytes for every produced response, while `supervisor-decision.json` is deterministic validated AW-01 JSON with a final newline and is materialized only after exact decoding and policy validation succeed.
- Supervisor source-read-only enforcement uses a source-writer lock plus validated content-sensitive before/after Git snapshots of HEAD, index, tracked content/modes/types, deleted paths, and non-ignored untracked content. Only `.revolvr/` runtime artifacts are excluded. Any mutation or capture uncertainty rejects the decision and is preserved as evidence; Revolvr never commits, resets, cleans, restores, or reverts the detected work.
- One supervisor invocation has its own ledger run. `completed` means exactly one decision was accepted, including a structurally valid `complete` or `block` recommendation; it never means the durable task completed. Rejected output, invocation failure, timeout, mutation, or capture uncertainty fails the supervisor run with verification `not_run`, empty commit SHA, no receipt, and no commit event.
- AW-07 returns a validated decision and a compact AW-02-compatible `DecisionReference` but does not persist it. Revolvr retains worker routing, verification, legal-transition, retry, commit, finding-disposition, completion-gate, and terminal-task authority for later slices.
- Canonical autonomous task lifecycle identity is `workflow: autonomous-v1`. Missing workflow remains a compatibility alias for `mixed-pass-v1`, and mixed-pass remains the default for task-file creation, app add, and Markdown import.
- The only AW-08 task-to-state reference is `autonomous_state_path: .revolvr/autonomous/tasks/<task-id>/state.json`. It is required for autonomous tasks, forbidden for mixed-pass tasks, must exactly identify the owning task namespace, and is validated without requiring or creating the referenced file. Existing symlink components are rejected so an apparently correct path cannot redirect into another namespace or outside the repository.
- Workflow lifecycle metadata is mutually exclusive. `mixed-pass-v1` keeps its ordered `implement|audit|document|simplify` phase and existing profile-frontmatter compatibility; `autonomous-v1` has no ordered phase and rejects a nonempty task profile because future worker selection belongs to validated supervisor routing. `internal/passpolicy` remains mixed-pass-only.
- Runnable task selection is workflow-explicit through `taskfile.ListRunnableForWorkflow` and `taskfile.SelectNextForWorkflow`. Compatibility `ListRunnable` and `SelectNext` mean the default mixed-pass workflow, and current `runonce` names that workflow directly. Every selector still validates all canonical task files and duplicate IDs before filtering, then orders eligible pending tasks by priority and filename.
- Status-only task updates preserve all non-status raw bytes when frontmatter already exists, including autonomous workflow/state references, unknown metadata, H1/specification content, CRLF/LF choice, and missing final newline. Metadata updates validate the complete resulting document before the existing atomic replacement; the APIs do not expose workflow or state-reference replacement, so retry cannot silently migrate a task or discard lifecycle evidence.
- AW-08 defines metadata, parsing, updating, read projection, and selection boundaries only. It does not create/load/persist AW-02 state, invoke the AW-07 supervisor, route workers, apply completion policy, add ledger evidence, execute verification, create receipts/commits, or add autonomous CLI/TUI execution behavior.
- Pure `autonomous-v1` routing policy lives in isolated `internal/autonomouspolicy`. Its sole public evaluation entry point is `Evaluate(Input) (Route, error)`, consuming AW-01 decisions, AW-02 decision references/state, and explicit transient evidence while returning only a compact worker/complete/block authorization. It does not belong in the ordered mixed-pass `internal/passpolicy` package.
- Autonomous policy source identity is an exact 64-character lower-case SHA-256 compatible with `gitstate.SourceSnapshot.SnapshotSHA256`, not HEAD or human-readable worktree status. Callers separately classify source safety as `safe`, `unsafe`, or `unknown`; all worker and completion routes require `safe`, while `block` remains available for unsafe or unknown classifications. An optional latest source mutation records same-task run/decision/action/resulting-revision provenance and nil means no source-mutating worker run exists in the supplied evidence history.
- Policy verification evidence reuses `autonomous.VerificationSummary` inside a source-revision envelope and requires nonempty run and occurrence identities plus typed evidence. It is fresh only when status is `passed` and its exact source revision equals the current revision; missing, failed, malformed, and stale verification fail closed wherever verification is a gate.
- Policy audit evidence embeds the exact validated `autonomous.AuditReport` and records audit run, worker profile, source revision, and consumed verification run/occurrence identities. A usable audit must come from `auditor`, cover the current source, consume the current fresh passed verification occurrence, and use a run distinct from the latest source-mutating worker run. Clean disposition is additionally required for document, simplify, and complete; correct requires `changes_required` and `ValidateCorrectionDecision`.
- Autonomous lifecycle admission is explicit: `pending` admits only plan and block; `ready` may consider every action subject to its gates; planning/working/verifying/auditing/correcting reject concurrent routing; needs_input rejects routing until AW-17 resume semantics; finalizing rejects routing until AW-20; completed/blocked/cancelled reject every new decision without an explicit reopen operation.
- Plan authorizes planner work without verification/audit and permits later revisions, but cannot bypass current unresolved blocking findings. Implement requires an actionable pending/in-progress step in a valid non-completed plan and cannot bypass any current unresolved changes-required finding. Audit requires current passed verification. Correct may select a deterministic unresolved subset of current changes-required findings and cannot select terminally resolved/waived/superseded/invalid findings. Document and simplify are optional independent routes after fresh clean gates and have no required order. Block requires the AW-01 typed decision evidence but no worker, verification, audit, or safe-source classification.
- Complete is authorization only and requires ready lifecycle, safe current source, a completed all-terminal plan, at least one nonpending structurally valid acceptance criterion, current passed verification, a linked fresh independent clean audit, and no open durable finding resolution. Existing AW-02 state-transition validation remains responsible for preventing durable plans, acceptance criteria, and finding resolutions from disappearing between snapshots; AW-09 evaluates the validated current snapshot and never constructs or persists a completed state.
- AW-09 policy performs no I/O, Git/Codex/verification commands, clock/random access, ledger/task/state persistence, task transition, receipt, commit, or runtime orchestration. It does not interpret attempt budgets, needs-input questions, optional-role outcomes, or retries, and it is not wired into supervisor, runonce, taskfile, app, CLI, or TUI behavior. `mixed-pass-v1` lookup/order and explicit runonce selection remain unchanged for later AW-10 integration.
- One autonomous supervisor-directed runtime cycle lives in isolated `internal/autonomouscycle`. Its only orchestration entry point is `Run(context.Context, Config) (Result, error)`; it composes AW-04, AW-07, AW-09, worker execution, receipt, verification, and commit evidence for one task without becoming an app, CLI, TUI, queue, daemon, or general mutable state runner.
- AW-10 receives one explicit validated `autonomous.ExecutionState` plus typed transient source-safety, latest-mutation, verification, and audit evidence. It deep-clones state for dependency calls and never loads, creates, migrates, updates, or returns a durable state snapshot. Canonical task loading is read-only and accepts only a pending exact `autonomous-v1` task with its AW-08 state reference; task and state metadata remain unchanged.
- `autonomouspolicy.ValidateEvidence` is the narrow structural pre-supervisor validation boundary for transient AW-09 evidence. It validates task/source/verification/audit envelopes without authorizing an action; `Evaluate` remains the sole route authorization and the cycle calls it exactly once after the supervisor and lock-held source admission.
- AW-10 brackets dossier assembly with validated full content-sensitive `gitstate.SourceSnapshot` values and refuses any same-HEAD, staged, tracked, deleted, untracked, mode, symlink, index, or HEAD drift without silently reassembling. The accepted supervisor's before/after snapshots must match that exact full dossier source baseline and remain read-only.
- The supervisor keeps its AW-07 internal source-writer lock. AW-10 never calls `supervisor.Run` while holding another source lock; after supervision it acquires a separate cycle/worker source-writer lock, rechecks the full source snapshot, evaluates policy under that lock, and holds it with heartbeat coverage through worker, change capture, verification, commit, and final capture.
- Full source snapshots and `CompareSourceSnapshots` remain the authoritative race/mutation record. The canonical AW-09/verification freshness identity is the pure `gitstate.PolicySourceRevision` content-tree SHA-256: it hashes non-missing path/type/mode/size/content entries but excludes HEAD, index placement, and missing tombstones. This keeps edits, additions, deletions, renames, modes, and symlinks content-sensitive while making staging and committing identical verified bytes administratively stable.
- A verification occurrence is always labeled with the exact pre-commit policy source revision it actually tested. After a successful commit, AW-10 captures the final full source and requires its policy revision to equal that verified revision; otherwise it returns a commit-freshness failure and never relabels the earlier verification as current.
- Supervisor and worker identities and artifacts are separate. The cycle supplies its own supervisor run/decision IDs to AW-07; only a validated worker route generates a distinct worker run ID. Worker prompt, provenance, output, source evidence, JSONL/stderr, receipt, ledger run, and artifact paths are isolated under the worker identity, and both Codex provenance records require fresh ephemeral `exec` argv with no `resume`.
- Worker profile selection comes only from the validated AW-09 route and exact repo-authored `.agent/profiles/<name>.md` content loaded through `prompt.LoadRunProfile`; autonomous task frontmatter cannot override it. The worker prompt embeds the exact dossier, decision, reference, route, profile, admitted source, artifact identities, and current evidence, forbids nested Codex/routing/commits/task-state mutation, and gives correctors an exclusive cited-finding authority projection.
- Plan and audit workers are source read-only. Any full snapshot mutation is failed evidence that Revolvr preserves without verification, commit, reset, clean, restore, checkout, or automatic correction. Implement, correct, document, and simplify may change source, but HEAD/task/state mutation, administrative-only Git changes, source-capture uncertainty, or changes outside exact observed path evidence stop before verification/commit.
- AW-10 treats receipt prose as advisory. It preserves exact worker output and original valid receipt claims, records mismatch warnings, and rewrites verdict, timestamp, changed files, verification, commit, and metrics from harness observations. Missing, malformed, and wrong-identity receipts use deterministic `receipt.FormatFallbackReceipt`; decision-only/read-only/no-change evidence remains in ledger/artifacts without forcing a source commit.
- Only a successful authorized source-changing worker with meaningful captured content changes receives one configured verification run and one harness-generated occurrence ID. Missing commands fail commit admission even when the existing verification package's configured missing-command policy reports pass. Failure, timeout, runner/capture/ledger error, or verification source mutation preserves evidence and starts no corrector, rerun, tier, retry, or second worker.
- AW-10 delegates commit staging, refusal classification, command evidence, pre/post-HEAD reconciliation, and ledger commit recording to `internal/commit`. It filters the post-worker capture to paths actually changed during this worker window so unrelated pre-existing work is not staged; refusal, failure, indeterminate HEAD, final capture uncertainty, and freshness mismatch remain explicit returned outcomes.
- Complete and block are transient AW-09 authorizations only in AW-10. They create no worker run/session, verification occurrence, receipt, commit, completed/blocked task status, terminal execution state, or `LatestDecision` update. Exactly one worker is the hard maximum for worker routes, and AW-10 contains no retry, automatic correction, re-audit, or repeated supervision loop.
- AW-10 has no CLI, TUI, app, `runonce`, task-selection loop, or mixed-pass integration. `runonce` continues to call `SelectNextForWorkflow(..., mixed-pass-v1)`, and `internal/passpolicy` remains exclusively the ordered mixed workflow; AW-11 and AW-12 own durable plan/acceptance and audit/finding persistence.
- AW-11 structured planner-output ownership lives in isolated `internal/autonomousplanning`. Schema `autonomous-planning-output-v1` contains the complete proposed current plan and acceptance matrix plus typed task, decision, worker, planner-profile, dossier, raw-output, and source provenance. Its standard-library decoder rejects unknown fields and requires exact EOF; receipt prose and arbitrary Markdown never become planning state.
- The canonical AW-08 state path remains `.revolvr/autonomous/tasks/<task-id>/state.json`, encoded directly as the current validated AW-02 `ExecutionState` with indented deterministic JSON and one final newline. AW-11 introduces no wrapper or migration and gives dossier/policy consumers the exact reopened state.
- Immutable planning transition history uses schema `autonomous-planning-transition-v1` under `.revolvr/autonomous/tasks/<task-id>/history/planning/`. A filename combines the zero-padded 20-digit resulting revision with SHA-256 of the operation ID, making ordering deterministic without exposing an unsafe operation string and making same-operation collisions explicit.
- State, history, planner output, dossier, profile, decision, task source, and source revision identities use exact byte sizes and lower-case SHA-256 values. Each history record retains exact previous/result state identities, plan identities and values, complete previous/result acceptance matrices, change kind/time, operation/application identities, and the full admitted planner provenance needed to reconstruct a prior revision.
- Planning persistence writes and synchronizes canonical validated planner output and immutable history before preparing and atomically replacing current state. A crash before replacement leaves the previous state authoritative and may leave only an identical orphan record; retry validates and reuses it. A committed state therefore always has required immutable evidence, while replay reopens, validates, and re-synchronizes the evidence/state directories after uncertain post-rename outcomes.
- `autonomousstate.Store` owns canonical loading and compare-and-swap persistence. It validates the canonical autonomous task and exact namespace, rejects existing symlink components and noncanonical/malformed/wrong-task state, serializes writers with a persistent per-task file lock, compares the caller's absent/existing plus hash/size expectation before history work and immediately before rename, uses same-directory temporary files with file flush, atomic rename, strict readback, and practical directory synchronization, and never silently changes missing/existing state expectations.
- Planning operation idempotency is content-addressed by an application SHA-256 over the exact expected state and admitted planner/cycle evidence. Replaying one operation ID with identical material returns the already committed state/history without another record; an identical orphan is reconciled; the same operation ID with materially different content returns an explicit conflict rather than overwriting evidence.
- An initial plan is accepted only from a policy-admissible pending state without a plan, at revision 1 with no predecessor, at least one ordered step, and no newly completed/skipped work. A deliberate ready-state revision requires a new plan ID, revision exactly plus one, and the exact current predecessor; stable step IDs cannot change meaning, terminal step status/evidence/rationale and compatible order cannot change, prior terminal work cannot disappear, and new terminal work is rejected.
- Acceptance criterion identity is stable by criterion ID and immutable exact requirement, with duplicate IDs and duplicate normalized requirements rejected. Existing criteria cannot disappear or be replaced under renamed IDs. A criterion source must be either the exact hashed canonical task artifact and text grounded in task bytes, or the exact current supervisor decision artifact and an exact cited supervisor success criterion; planner invention does not create task or supervisor authority.
- Acceptance transitions retain prior complete criterion values. Pending has no disposition evidence or rationale; satisfied requires validated typed evidence; waived and not-applicable require concrete rationale and retain any supplied validated evidence; nonpending criteria cannot return to pending. A revision may add a newly grounded criterion but cannot mutate or erase earlier acceptance evidence.
- A successfully applied initial plan moves `pending` to `ready`; a valid deliberate revision keeps `ready`. Both preserve task/schema identity, findings, attempts, budgets, and unrelated state, perform no retry accounting, and persist the exact current supervisor `DecisionReference` as `LatestDecision`. AW-02 `ValidateExecutionStateTransition` remains the final general transition authority.
- AW-10 remains a transient, generic one-cycle boundary. Only its plan/planner route receives the distinct planner JSON schema and raw-output artifact path; it still neither parses nor persists a plan. Isolated `autonomousplanapply.ApplyPlanningResult` later admits only a successful exact read-only AW-10 planner result with distinct valid supervisor/worker runs, matching decision/dossier/profile/source/output artifacts, fresh ephemeral invocation, no source mutation, and no synthesized verification or commit.
- AW-09 remains pure and performs no loading. A reopened AW-11 state is passed explicitly to `autonomouspolicy.Evaluate`; implement routing consumes the persisted plan, completion consumes the persisted matrix, and all existing verification/audit/finding/source freshness and independence gates remain unchanged.
- AW-11 does not persist audit reports, findings, or finding-resolution transitions; AW-12 owns those operations. It also adds no retries, corrections, verification tiers, needs-input semantics, terminal finalization, task-frontmatter updates, or generalized transaction database.
- AW-11 has no app, CLI, TUI, task-selection, or `runonce` integration. `internal/passpolicy` remains exclusively `mixed-pass-v1`, `Lookup(autonomous-v1, ...)` remains invalid there, and `internal/runonce` continues to explicitly select `mixed-pass-v1`.
- AW-12 structured auditor-output ownership lives in isolated `internal/autonomousaudit`. Schema `autonomous-audit-output-v1` contains one complete AW-01 `AuditReport` plus exact task, audit decision, auditor worker, auditor profile, dossier, source, verification occurrence, latest source mutation, and raw-output provenance. Strict standard-library decoding rejects unknown fields and requires exact EOF; receipt prose and arbitrary Markdown are never control evidence.
- Only the AW-10 `audit -> auditor` route receives `auditor-output-schema.json` and writes `auditor-output.raw.json`; the exact schema path is supplied to fresh ephemeral Codex. The cycle remains transient and read-only, performs no parsing/persistence, verification, or commit for audit, and leaves planner schema behavior unchanged while all other AW-12 worker actions remain schema-free.
- Canonical AW-02 state remains unwrapped deterministic JSON at `.revolvr/autonomous/tasks/<task-id>/state.json`. AW-12 adds no mutable current-audit projection. Exact current audit authority is reconstructed from immutable history only when a transition's resulting state hash and size match canonical state.
- Immutable audit and finding-transition history uses schema `autonomous-audit-transition-v1` under `.revolvr/autonomous/tasks/<task-id>/history/audit/`. Filenames combine a zero-padded 20-digit global audit-transition sequence, the typed transition kind, and SHA-256 of the operation ID. Audit revisions increment only for recorded audits; finding transitions retain the introducing/current audit revision and exact report/policy projection.
- Every audit history record retains the exact audit decision/reference, supervisor artifact, auditor run/profile, dossier identity, audited source revision, passed verification run/occurrence, latest source-mutation identity, task/raw/canonical artifacts, report, policy evidence, previous/resulting state identities, complete previous/resulting resolution slices, and any finding transition evidence/rationale/authority/target.
- Audit and planning persistence share the exact existing per-task `state.lock`. Audit persistence performs canonical task/path/symlink validation and exact hash/size CAS before immutable work and immediately before state rename. Canonical audit output and history are synchronously created before same-directory temporary state replacement; atomic rename, directory sync, and strict readback follow.
- A pre-state crash may leave orphan canonical/history files, but they never become current. Committed-history reconstruction walks backward from canonical state across both audit and planning transition identities, excluding orphans from durable finding identity. Identical retry reuses an orphan's sequence/revision; a post-rename state always has reconstructible immutable evidence.
- Audit operation idempotency is content-addressed over exact expected state plus admitted cycle/raw/canonical evidence. Exact replay returns the existing state/history, while one operation ID with different material returns `ErrOperationConflict`. Stale state produces `ErrStaleWrite`; typed application results retain concrete failure stage and reason.
- Audit admission requires the exact successful AW-10 worker route, task/action/auditor profile, supervisor decision/reference, completed worker ledger run, fresh ephemeral schema invocation, readable matching task/dossier/profile/schema/raw artifacts, unchanged source/task/state, no synthesized verification or commit, and exact current passed verification/source evidence. Supervisor, auditor, verification, and latest source-mutating run identities must be distinct where applicable; these independence relationships are revalidated from persisted history.
- Finding IDs are durable identities. A reused ID keeps significance and deterministic normalized summary/correction meaning; prior evidence is an exact retained prefix and only deterministic appends are allowed. An equivalent new ID is a probable rename while the old finding is open; open findings cannot disappear; terminal IDs cannot reopen or be reused; terminal recurrence needs a new identity.
- Clean audit behavior is fail-closed when any finding remains open: silence never resolves, waives, supersedes, invalidates, deletes, or renames durable history. Clean reports may coexist with historical terminal resolutions, which remain visible in state and dossiers.
- Finding resolution is an explicit separate AW-12 application. Resolved, waived, superseded, and invalid all require typed evidence; waived/invalid require rationale; supplied resolution verification must be passed/current and retained exactly; correction decisions must cite the finding; supersession requires a different known noncontradictory target and is acyclic. No reopen semantics are added.
- Successful audit persistence keeps lifecycle `ready`, preserves plans, acceptance, attempts, budgets, and unrelated state, appends open entries only for newly admitted findings, and stores the exact audit decision as `LatestDecision`. Resolution normally remains `ready` and updates `LatestDecision` only when an explicit supplied authority reference exists.
- Reopen strictly validates every persisted JSON envelope and required artifact identity, reparses raw and canonical auditor output, requires canonical bytes, and rejects raw/canonical/history disagreement. Reopened `autonomouspolicy.AuditEvidence` is passed explicitly to pure AW-09; reopened reports and state are passed explicitly to input-driven AW-03/AW-04 dossier assembly.
- AW-12 provides persistence APIs only. It does not execute a corrector, verification, automatic re-audit, or repeated supervisor loop; AW-14 owns that orchestration. It adds no AW-13 verification tiers/flaky reruns and no app, CLI, TUI, task-frontmatter, `passpolicy`, or `runonce` integration.
- AW-13 tier-plan ownership lives in isolated `internal/autonomousverification`; `internal/verification` remains the low-level bounded flat command runner. The autonomous package owns strict versioned plan/result/gate contracts, pure selection, exact identities, one-operation orchestration, controlled flaky classification, deterministic artifact encoding, and legacy projections, but owns no task state, audit persistence, correction, commit, or lifecycle transition.
- Canonical tier-kind order is `structural`, `focused`, `task_acceptance`, `full_suite`, `race`, `integration`, then `security`. Plans require unique lower-case kebab-case tier IDs, unique kinds, canonical order, declared command order, typed flags/policies, repository-contained command directories, and no public map contract. Caller slices are copied before selection or execution.
- Verification purpose is explicit: `fast` selects only explicitly enabled structural/focused/task-acceptance tiers and is evidence only for those checks; `final` selects every explicit final tier and requires at least one configured required final tier. Optional final tiers run only when enabled. A final gate requires an ordinary passed overall operation and ordinary passed evidence for every configured required tier.
- Existing flat command configuration maps deterministically to one `legacy-flat` tier of kind `full_suite`, required and enabled for final verification with reruns disabled. Omitted tier config continues to execute the exact existing flat runner. Supplying flat and tiered material together is an error. Empty legacy command YAML retains existing defaults rather than becoming an ambiguous tier plan.
- Plan identity is SHA-256 plus byte size over validated indented canonical plan JSON with one final newline. Effective configuration hashing additionally covers tier IDs/kinds/order, fast/final/required flags, rerun policy, exact ordered command fields, and global default timeout/caps. Command material identity separately binds plan hash, purpose, tier, position, executable, argv, normalized directory, ordered env, and effective timeout/caps.
- Attempt identity is caller-injected and distinct from the verification occurrence. Attempt evidence is immutable and append-only: attempt number, command identity, times/duration, exit and classification, timeout/cancellation/runner error, capped stdout/stderr, and truncation counts remain visible. A second attempt never overwrites the first.
- Tiered execution is fail-fast. Commands run in declared order, tiers run in canonical selected order, and the first nonpassing command/tier prevents later commands/tiers. Tier start/completion evidence is retained for everything reached, and cancellation starts nothing further.
- A flaky-classification rerun is permitted only for the first deterministic ordinary command failure whose tier explicitly uses `once_to_classify_flaky`; the entire operation has a one-rerun maximum. Missing commands, timeout, cancellation/context cancellation, runner errors, invalid configuration, ledger/artifact failures, and already-rerun work are ineligible. No third or recursive attempt exists.
- Fail/fail is failed and retains both attempts. Fail/pass is `flaky` and retains both attempts. Flaky is intentionally not passed because a nondeterministic required check is not trustworthy final evidence; it fails aggregate compatibility projection, final-gate satisfaction, receipt overall status, and commit admission.
- Missing selected commands, ordinary failures, timeout, cancellation, runner errors, output truncation, ledger failure, artifact failure, invalid configuration, and verification source mutation remain explicit rather than becoming a generic blocked verification result. Truncation is evidence and does not alone turn an otherwise successful command into failure.
- Final-gate validation recomputes authority from the plan identity, purpose, required/selected/executed tier slices, required outcomes, missing required tiers, and overall outcome. Consumers do not trust a standalone `final_satisfied` boolean. Legacy evidence without a tier projection remains compatible through the documented adapter.
- AW-10 purpose selection is `final` for implement and source-changing document/simplify, and explicitly configured `fast` for correct. Plan/audit read-only workers, failed workers, and no-change workers run no verification. One policy rerun remains part of one verification operation and never starts another worker or cycle.
- AW-10 commits only after an ordinary pass for the selected purpose and unchanged post-verification source evidence. Fast correction evidence may admit that correction's commit but never claims the final gate; flaky, failed, missing, timeout, cancelled, errored, artifact/ledger-failed, or source-mutating verification never commits.
- AW-09 remains pure. `autonomouspolicy.VerificationEvidence` consumes a validated gate projection and the compact summary can retain the exact typed result. Audit/document/simplify/complete require current final-gate evidence when tiered; fast-only, unsatisfied, malformed, stale, or wrong-identity evidence rejects. Tier selection and flake classification never occur inside `Evaluate`.
- AW-12 auditor provenance retains exact tier gate/result material alongside existing run/occurrence/source identity. Strict raw/canonical/history parsing and reopen validation reject fast-only, flaky, failed, malformed, or provenance-mismatched tier evidence without changing audit/finding storage layout or identity rules.
- Tiered verification artifacts live at `.revolvr/runs/<worker-run-id>/verification.json`. Ledger events retain plan/purpose/selection, tier order, command identities, attempts, rerun authorization, classification, final gate, and artifact identity; artifact extraction exposes the path after reopen. Dossiers render a bounded typed projection including both flaky attempts and explicit failure facts. `revolvr.receipt.v1` stays unchanged and represents attempts as ordered verification entries; distinct outcomes for the same command are not deduplicated.
- AW-13 adds no automatic correction, final re-verification, re-audit, repeated supervision, retry budget, or task-state transition; AW-14 owns those loops. It adds no CLI/TUI autonomous execution and no `runonce` autonomous path. `internal/passpolicy` remains exclusively `mixed-pass-v1`, while `runonce` explicitly refuses tiered autonomous plans instead of changing mixed-pass behavior.
- AW-14 bounded correction orchestration lives in isolated `internal/autonomouscorrection`. One call may run exactly one correction cycle, one distinct final verification, explicit resolutions for only corrector-claimed cited findings, one fresh audit cycle, and one audit application; it contains no retry or recursive supervision loop.
- Correction authority is an exact union. Existing audit corrections retain `finding_ids`; verification-triggered correction uses `autonomous.VerificationFailureTarget` with exact task, run, occurrence, source revision, failed status, and typed evidence. A correction decision cannot combine or substitute those authority kinds, and pure AW-09 policy compares verification authority to the exact supplied failed occurrence.
- Correctors now return strict schema `autonomous-correction-output-v1`. The result binds task/worker/decision/authority, partitions cited findings into resolved and remaining IDs, and carries typed evidence. It cannot claim an uncited finding, and a verification repair cannot invent an audit finding. Failed and partial outcomes remain explicit control evidence.
- Corrector execution remains one ordinary AW-10 source-changing cycle with fast verification and commit admission. Correct receives a corrector-only schema/artifact, but `autonomouscycle.Run` still performs exactly one supervisor decision and at most one worker; it does not run final verification, resolve findings, or re-audit itself.
- The AW-14 coordinator captures and returns an exact pre-correction full source checkpoint and rejects dirty or ambiguous source before another writer. It never resets, cleans, checks out, restores, rolls back, or destructively rewrites user work.
- Final verification has its own ledger run and occurrence, targets the committed corrected source, starts after correction completion, and must be a validated ordinary final-purpose pass satisfying every configured required tier. Any other outcome or source mutation stops before audit while retaining the attempt.
- Finding resolution remains exclusively through `autonomousauditapply.ApplyFindingResolution`. Resolution evidence combines exact structured correction evidence/artifact, the exact correction decision/reference, and the new final verification; partial repair leaves other findings open.
- Re-audit is a separate fresh AW-10 auditor cycle on the corrected source and final verification occurrence. Its supervisor/auditor identities must differ from correction supervisor/corrector/final-verification identities, and it starts after final verification. Persistence remains through `autonomousauditapply.ApplyAuditResult` and immutable `autonomousstate` history. Clean or findings-bearing persisted re-audits both return control to supervision and never directly complete the task.
- AW-14 adds no app, CLI, TUI, task-frontmatter, `runonce`, or `passpolicy` path. `autonomouspolicy.Evaluate` remains pure, and AW-15 retains all retry-budget, no-progress, repeated-signature, strategy-change, elapsed/token-limit, and circuit-breaker behavior.
- AW-15 ownership model: a new isolated `internal/autonomousattempt` boundary authoritatively admits and completes exactly one caller-selected action above the existing one-shot cycle/correction APIs. Admission is compare-and-swap persisted before the injected runner starts and charges the total and exact action attempt once; completion uses the admitted resulting state as its compare-and-swap expectation and applies trusted harness duration and receipt token facts once. The supervisor supplies no budget fields and cannot write accounting state.
- AW-15 limits are explicit caller authority. The first clean attempt may bind caller-supplied unset/limited/unlimited total, per-action, elapsed, and token limits into state; every later call must supply the exact same modes and limits. Limited budgets admit only while `consumed < limit`; equality is exhausted for attempt counts, elapsed time, and tokens. Consumption is monotonic, and missing token facts under a limited token budget are a typed fail-closed stop rather than guessed usage.
- AW-15 durable evidence uses append-only typed attempt events in canonical state plus one immutable compare-and-swap history record per admission/completion/breaker operation under the task namespace. Exact task/action/attempt/decision/run/occurrence/source identities, outcome, trusted duration/token facts, evidence references, canonical decision/failure/finding signatures, and canonical structured strategy signature remain restart/replay inputs. Operation IDs are idempotent and content-addressed; stale writers cannot double-charge.
- AW-15 progress detection is exact and deterministic. Signature helpers hash validated structured decisions, exact verification-failure occurrences, and stable ordered open-finding/report identities. Strategy signatures hash normalized typed approach/technique/target material while excluding run IDs, timestamps, formatting-only whitespace, and incidental explanatory prose. A caller-owned repeated-signature threshold is persisted with the limits; a transient first failure may retry, but threshold repetition terminates, and an explicit no-progress event requires a materially different strategy before another admission.
- AW-15 task isolation follows the existing canonical task namespace and shared per-task `state.lock`. A circuit breaker appends typed reason, budget snapshot, triggering attempt/signature references, and immutable history evidence, then transitions only that task through existing state validation to `blocked`. It cannot complete a task, resolve findings, erase evidence, or start a later stage.
- AW-16 optional-role ownership lives in isolated `internal/autonomousoptional`. A trusted, versioned `OptionalRoleAssessment` binds one documentor or simplifier decision to exact task/state/source/final-verification/audit evidence. Documentation obligations and simplification targets are structured harness evidence with exact repository-relative target paths; supervisor prose cannot invent or waive them, and every observed changed path must remain within selected target authority.
- Documentation and simplification remain independent and unordered. `not_applicable` is a typed decision-only occurrence that requires exact `no_relevant_work` evidence and rationale and consumes no AW-15 attempt. It is not absence of an attempt, task-frontmatter state, an acceptance disposition, or a receipt claim. Existing completion still requires neither optional role by default.
- Executed optional roles use one AW-15 admission/completion around one ordinary AW-10 role cycle and, only after a committed source change, at most one fresh independent audit cycle plus the existing AW-12 audit application. The nested audit is a stage of the one charged operation. No-op remains AW-15 `no_progress`, runs no verification/audit, creates no commit, and retains exact worker/receipt/ledger/artifact/current-gate evidence.
- Source-changing optional roles rely on AW-10 final-purpose verification and exact commit admission. The coordinator requires a newer independent audit for the same source and verification occurrence before success; simplification additionally retains behavior-preservation evidence. It never resolves or otherwise dispositions audit findings, and both clean and findings-bearing audits return to supervision.
- Optional-role occurrences are append-only in canonical state and immutable schema `autonomous-optional-role-transition-v1` history under each task namespace. Persistence shares `state.lock`, exact CAS, history-before-state ordering, strict readback, and content-addressed replay. Audit reconstruction traverses planning, audit, attempt, and optional-role edges so evidence-only transitions do not hide the latest audit; policy still rejects stale source or verification identities.
- Optional-role relevance identity canonicalizes exact evidence and selected IDs while excluding supervisor IDs, timestamps, formatting, and rationale-only changes. The full occurrence separately retains the exact validated decision/reference and rationale, so materially conflicting operation reuse still fails closed.
- AW-17 models operator input as a distinct terminal-for-now supervisor action, `needs_input`, rather than block rationale. The action selects no worker and forbids strategy, correction authority, and success criteria. Its typed question contains exact task/question/revision/content identity, question and blocking reason, stable mutually exclusive options, one offered recommendation and rationale, typed evidence, and optional independent-work declarations.
- Needs-input content authority is SHA-256 over compact deterministic JSON containing every control-relevant question field except the hash itself. A supervisor may omit the hash so the harness assigns it deterministically during strict parsing; a supplied hash must match. Answers and resumes use only exact typed task/question/revision/content/option/answer identities, never labels, array indexes, recommendation prose, or rationale as control authority.
- Independent work under needs-input is a projection, not a route. It is valid only for compatible read-only plan/planner or audit/auditor declarations whose ordered `independent_of_option_ids` exactly names every offered option and whose inputs are typed evidence. AW-17 never executes such work; later scheduling may consider it only after the pure clean-yield gate succeeds.
- Canonical execution state keeps append-only input lifecycle evidence as ordered question, answer, and resume records while `NeedsInputDetail` holds only the exact current question identity. A question records source revision, decision provenance, suspension time, resume lifecycle, and explicit predecessor identity. An answer records stable answer ID, one offered option, operator provenance/evidence, and harness time. Resume records bind that exact answer and restores only the question's recorded pending/ready lifecycle. Questions/answers/resumes never consume or reset AW-15 accounting.
- Input persistence lives in isolated `internal/autonomousinput` over `internal/autonomousstate` schema `autonomous-input-transition-v1`. It uses the canonical task namespace and shared `state.lock`, exact state hash/size CAS, immutable history before atomic canonical-state replacement, synchronized files/directories, strict readback, content-addressed replay/conflict behavior, and before/after-rename recovery. Committed audit reconstruction traverses input edges alongside planning, audit, attempt, and optional-role edges.
- `autonomouspolicy.Evaluate` remains pure and returns a distinct no-worker needs-input route only from pending/ready safe state. Suspended state admits no ordinary route before exact answer plus explicit resume. The separate pure future-scheduler projection returns typed clean/unsafe results for task/state/question/provenance/source/in-flight gates; dirty, unknown, stale, malformed, legacy, or ambiguous state never yields. No scheduler, CLI, or TUI control surface was added.
- Legacy minimal `needs_input: {reason: ...}` execution state remains readable and is labeled non-answerable in dossiers. It cannot fabricate question IDs, revisions, options, recommendations, content identities, answers, or resume authority, so input application fails closed until a typed question is durably recorded.
- AW-18 uses `autonomous.TaskWorkspace` as the typed control/execution-root authority. It records canonical absolute control root, execution worktree, Git common directory, control-root ownership marker, deterministic harness branch, exact baseline/current/checkpoint identities, retained refs, and recovery status; immutable ownership cannot change and checkpoint/retained evidence cannot regress.
- Workspace IDs hash canonical control root, Git common directory, and task ID. Harness worktrees live only under `.revolvr/autonomous/worktrees/<workspace-id>` and refs under `refs/heads/revolvr/tasks/`; a canonical synchronized control-root marker is required before any existing ref/path/registration may be recovered. Naming conventions or path presence never prove ownership.
- Workspace preparation uses an exact commit object, never primary index/filesystem bytes. Marker-before-ref and ref-before-registration states are recoverable; foreign branches, worktrees, paths, symlinks, markers, common directories, or `.git` links are conflicts. Unknown advanced HEADs are retained under immutable `refs/revolvr/retained/` inspection refs rather than reset or guessed current.
- The known-good checkpoint begins at the baseline and advances only from an exact reconciled clean commit/tree/source identity with operation provenance. Restoration first retains failed staged/tracked/untracked bytes in an immutable commit/ref, then may reset/clean only the twice-revalidated owned execution worktree to the exact durable checkpoint. Ignored files that cannot be safely retained block restoration.
- Workspace transitions use `autonomous-workspace-transition-v1` immutable history and the existing per-task `state.lock`, exact state CAS, history-before-state atomic replacement, strict readback, replay/conflict/stale classification, and committed-audit graph traversal. A control-root global Git-admin flock serializes ref/worktree mutation; workspace source locks remain control-root state while naming the exact protected execution root.
- Autonomous source execution is fail-closed and root-separated. `autonomouscycle.Run` requires a validated workspace matching durable state. Canonical tasks, profiles/guidance, state/history, ledger, receipts, and artifacts remain at the control root; source snapshots, Codex work, changed-file capture, verification, commits, correction, and optional-role mutations use the execution root. Codex and tiered verification carry a separate control-root artifact root. Mixed-pass behavior remains unchanged.
- AW-19 autonomy declarations use schema `revolvr-autonomous-safety-declaration-v1` with explicit `operator_attended` and `fully_unattended` modes. Operator-attended is the compatibility default for mixed-pass and current dogfood; it renders dangerous bypass, ambient environment, operator-attended hooks, and unknown network posture as operator responsibilities. Fully unattended is never inferred or defaulted and requires an exact task/workspace-bound preflight plus policy acknowledgement.
- Effective run configuration is now `revolvr-effective-run-config-v2`; its deterministic projection includes the complete secret-name-only autonomy declaration. Secret values never enter effective configuration or its hash. The policy acknowledgement is `revolvr-fully-unattended-v1:<policy-sha256>` and authorizes the nonrecursive material policy identity; changing workspace, permissions, commands, roots, external isolation, network, hooks, environment, or redaction policy invalidates it.
- `internal/autonomoussafety` owns pure typed safety contracts and bounded host preflight. It consumes the exact ready/restored AW-18 workspace and current source identity, derives harness/model writable roots and protected path classes, resolves executable hashes/argv/directories/environment/timeouts/caps, inspects effective `core.hooksPath` without running or modifying hooks, distinguishes unknown/denied/restricted/unrestricted network posture, requires stable external attestations for full autonomy, and states that worktrees are Git/source isolation rather than a security sandbox.
- Autonomous safety preflight runs inside `autonomouscycle` after canonical task/workspace/source/config authority is known and before dossier assembly or supervisor Codex. Unsafe or mismatched authority returns `safety_preflight_failed` and starts no supervisor, worker, verification, or commit. When AW-15 has already durably admitted the enclosing operation, the safety stop remains charged once and is classified as trusted safety-stop evidence; no duplicate admission occurs.
- The same safety-policy SHA-256 and complete redacted policy/preflight projection are recorded in supervisor and worker provenance and ledger preparation evidence. Model-authored changed paths are checked against protected Git administration, task specs, profiles, guidance, state/history, ownership, locks, ledger, and config authority before verification or commit; task/model output never supplies commands, environment, writable roots, hooks, network policy, or bypass authority.
- `internal/redact` provides dependency-free deterministic longest-first configured-secret replacement using explicit environment-variable names. Values are loaded only at runtime, short/missing/duplicate/malformed sources fail closed, and persistent Codex JSONL/stderr/final-output, returned output/errors, progress, and ledger summaries receive the redacted form plus source/match facts. Autonomous command results are redacted before verification/Git/commit evidence, and fully unattended execution can replace rather than inherit the ambient process environment through the bounded runner.
- AW-20 terminal ownership lives in isolated `internal/autonomousfinalization`; it consumes one already-authorized complete decision and performs no Codex, worker, verification, audit, correction, optional-role, commit, restore, archive, or scheduling work. Pure completion authority is recomputed through `autonomouspolicy.Evaluate`, and a caller-supplied bounded live-evidence revalidator must confirm the frozen workspace/source/safety authority before admission and before task terminalization.
- Canonical state uses `autonomous-finalization-state-v1` with monotonic `admitted`, `capsule_materialized`, `task_completed`, `state_completed`, and `ledger_completed` stages. Immutable operation, run, frozen-evidence, original/completed-task, capsule, manifest, and harness-time identities cannot be rewritten; legacy completed snapshots without AW-20 detail remain readable, while a real `finalizing` snapshot requires it.
- Frozen authority is map-free schema `autonomous-completion-frozen-evidence-v1`. It binds exact canonical task/spec and state bytes, complete decision/reference/route, source and final-purpose tier gate, independent clean audit, plan/acceptance/findings/optional-role/attempt evidence, exact ready/restored checkpointed workspace, safety policy/preflight/config, ordered commit/run evidence, provenance, and harness times. Full state/task hashes are recomputed, no attempt may remain in flight, and final HEAD must equal the checkpoint and final commit evidence.
- Completion artifacts live only under `.revolvr/autonomous/tasks/<task-id>/completion/` as `completion-evidence.json`, deterministic `completion.md`, and `completion-manifest.json`. Writes are immutable, same-directory temporary-file/file-sync/rename/directory-sync operations with containment, symlink-parent refusal, exact readback/hash/size checks, collision refusal, and configured-secret redaction. Schema `autonomous-completion-capsule-manifest-v1` records ordered source identities and explicit omissions without recursive self-hashing.
- AW-20 changes the canonical autonomous task status from `pending` to `completed`; AW-21 still owns tracked archive movement and reopen. The expected completed task projection is frozen and validated before admission, so a crash after the task rename cannot authorize changed task bytes. No metadata-only source commit is created.
- Transaction order is frozen artifact, admitted history/state, exact finalization run/prepared evidence, capsule and manifest, materialized history/state, exact task-status transition, task-completed history/state, completed lifecycle/terminal history/state, exact terminal ledger event/run completion, then ledger-completed history/state. Every state replacement uses `autonomous-finalization-transition-v1` under the shared per-task `state.lock`, history-before-state CAS, atomic replacement, strict readback, replay/conflict/stale classification, and committed-audit graph traversal. Once durable progress begins, reconciliation uses a non-cancelled context.
- Finalization ledger events are versioned exact payloads for prepared, capsule-materialized, state-terminal, and terminal-completed effects. Retry reads and compares the deterministic run/event/completion evidence before writing; identical effects replay, different effects conflict, and terminal ledger completion can never precede the capsule/manifest it cites.
- AW-21 archive ownership lives in isolated `internal/autonomousarchive`. Its bounded create/list/show/verify/reopen operations do not invoke Codex, verification commands, audit, correction, optional roles, workspace restoration/cleanup, a scheduler, or a repeated task loop. Active task discovery remains exclusively `.agent/tasks/*.md`; archive discovery is an independent strict scan of `.agent/archive/YYYY/MM/<task-id>/archive.json`.
- Archive disposition authority is schema `autonomous-task-archive-authority-v1` and covers exactly `completed`, `cancelled`, `superseded`, and `abandoned`. Task status and autonomous lifecycle contracts represent those four terminal outcomes; blocked and needs-input remain active and are rejected. Non-completed admission requires matching terminal task/state disposition and reason plus explicit trusted provenance/time, and explicitly omits AW-20 completion authority.
- Completed archive admission requires AW-20 `ledger_completed` state, exact completed task bytes, canonical terminal state, frozen evidence, completion manifest/capsule, source/workspace/checkpoint, final verification, independent clean audit, safety policy, and exact terminal finalization ledger event/run identities. Archived `completion.md` is a byte-identical copy of the verified AW-20 capsule; AW-21 never regenerates it.
- Archive IDs are deterministic SHA-256-derived identities over task, operation, disposition, and one explicit UTC archive time. That time alone selects `.agent/archive/YYYY/MM/<task-id>/`; task/capsule/manifest paths and archive/task IDs are collision-fail-closed, and strict archive scanning rejects duplicates, foreign files/directories, traversal, symlinks, unsafe permissions, and hard-link surprises.
- Tracked schema `autonomous-task-archive-manifest-v1` binds task/disposition/reason/provenance/terminal/archive times, original and archived task identity, canonical state, applicable AW-20 artifacts/finalization/ledger identity, expected tracked paths, and explicit omissions. It intentionally does not self-reference the administrative commit; the reconciled SHA lives in immutable runtime history and exact archive ledger evidence.
- Archive transactions take the control-root Git-admin lock before the per-task `state.lock`, reject live control/workspace source writers, and validate active task/state/artifacts/ledger/Git/index/worktree/operation authority before admission. Each monotonic stage writes synchronized immutable `autonomous-task-archive-transition-v1` history before atomic mutable journal replacement, then publishes task/capsule/manifest, removes only the exact active task, reconciles the administrative commit, and completes exact ledger evidence. After admission, roll-forward uses a non-cancelled context; exact retry reuses effects and conflicting partial evidence remains inspectable.
- Administrative archive commits are path-scoped and carry `Archive-Operation`, `Archive-ID`, `Task-ID`, `Disposition`, and `Terminal-Identity` lines. Pre/post HEAD reconciliation supports unborn repositories, retries HEAD lookup, accepts an advanced verified HEAD despite command failure, and rejects indeterminate outcomes. Commit verification checks the exact addition/deletion set and exact archive bytes; unrelated staged, dirty, untracked, conflicted, or source-worktree paths are never absorbed.
- Archive verification is read-only and returns ordered named checks. It uses immutable SQLite access and read-only Git object commands to cross-check archive path/date/schema, task bytes/metadata, terminal state, AW-20 evidence/capsule/manifest, finalization ledger evidence, immutable archive history, archive ledger run/events, administrative commit message/tree/bytes, active-task exclusion/reopen lineage, and configured-secret absence without repair or sidecar creation.
- Reopen uses a new task ID and schema `autonomous-reopen-lineage-v1`; terminal archives and old runtime history remain immutable. A verified archive produces one new pending autonomous task/state with only id/status/state-reference metadata rewritten and exact spec/unknown metadata/line endings preserved. Lineage binds archive/task/disposition/commit, operator authority/reason, operation, and UTC time. The new task addition receives its own path-scoped administrative commit, exact replay recovers partial publication, conflicting or second reopen attempts fail, and no task execution starts.
- AW-23 scheduling metadata uses source-ordered comma-separated scalar lists: `depends_on`, `tags`, and `conflicts`. Items are stable nonblank safe identities without comma or surrounding whitespace; duplicates and self-dependencies are invalid. Source order is identity-significant and is preserved rather than silently sorted. Child tasks additionally carry exact `parent_task_id`, proposal/decision/run IDs, evidence tokens, and `parent_behavior`; status-only updates and archive/reopen projections do not rewrite those bytes.
- `internal/autonomousscheduler` is the pure graph/readiness owner. Repository loading parses and duplicate-checks every active task before filtering, and exact archive evidence is a separate typed input. Missing dependencies, duplicate identities, active/archive coexistence, unverified archive authority, and deterministic directed cycles fail closed. Completed active tasks and verified/reconciled completed AW-21 archives satisfy edges; every other lifecycle/disposition waits, and archives are never runnable nodes.
- Ready order preserves the established lower-integer-first priority direction, then canonical source path, then task ID. A conflict applies against exact occupied identities when either task names the other or both share a stable key. AW-23 remains sequential and creates no parallel-worker or fairness policy.
- New AW-22 operations apply scheduler admission before workspace/model work. Omitted task IDs select once; explicit IDs must classify ready. A previously admitted operation's durable pinned task remains stronger than changed ambient graph/priority/archive state, so restart never reselects and one operation never advances to a second task.
- Supervisor child proposals remain part of the existing no-worker `block` or `needs_input` decision rather than adding a queue action. The strict schema permits at most four children and bounds canonical proposal bytes, titles, scopes, criteria, evidence, and graph lists. Material scope evidence must exactly occur in accepted decision inputs. Dependent children name their parent edge; independent children cannot name, answer, or bypass the parent. Publication consumes no AW-15 worker attempt.
- `internal/autonomouschild` owns deterministic ID derivation, exact child task/state projection, graph-impact validation, and restartable publication. Initial state is pending with immutable `autonomous-child-lineage-v1`; ordinary state transitions cannot attach, remove, or rewrite that lineage. Parent bytes/state/history are hash-revalidated and never written. Active task specs remain ordinary uncommitted canonical task files in this slice; AW-23 does not infer Git administrative commit authority.
- Child publication takes `.revolvr/locks/child-publication.lock` without holding the AW-22 operation lease, Git-admin lock, a parent/child state lock, or a workspace source-writer lock. It writes immutable history before each mutable journal stage, publishes every child state before task bytes, refuses collisions, and makes incomplete publication authority a scheduler error until exact roll-forward. Three content-compared supervisor-run ledger events are replay-safe. Configured secret values are rejected before persistent child task/state evidence.
- App status and task projections carry dependency, tag, conflict, parent, readiness, and deterministic next-autonomous facts. CLI task list exposes narrow scheduling columns, status names next autonomous readiness, and TUI `U` uses the same readiness result. `runonce` and `passpolicy` remain exclusively mixed-pass and do not consult the autonomous graph.
