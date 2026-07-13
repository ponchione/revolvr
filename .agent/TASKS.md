# Agent Tasks

## Rules

- Work on the first unchecked task.
- Do exactly one task per fresh loop pass.
- Mark a task complete only after verification passes.
- If blocked, write the blocker under `Blocked` and stop.
- Add new tasks only when they are directly discovered while working on the current task.
- New tasks must be specific, small, and verifiable.
- Do not invent broad roadmap items.

## Current Backlog

- [x] AW-01 — Define and validate autonomous supervisor-decision and audit-finding contracts.
  Scope: implement the foundational structured contracts described by step 1 of `.agent/AUTONOMOUS_WORKFLOW_REFACTOR.md` without wiring runtime behavior. Support deterministic JSON serialization and validation for supervisor actions, worker-profile selection, terminal decisions, audit dispositions/findings, and correction references.
  Acceptance: valid decisions and findings round-trip through JSON; unknown/malformed values, incompatible action/profile pairs, incomplete findings, duplicate finding IDs, and corrections without referenced findings fail clearly; existing `mixed-pass-v1` behavior remains unchanged.
  Verification: add focused table-driven tests; run the focused package tests, `go test ./...`, and `git diff --check`.

- [x] AW-02 — Define durable autonomous task execution-state contracts.
  Scope: extend the isolated autonomous domain with validated JSON types for a durable plan and plan-step status, acceptance criteria and evidence, audit-finding resolution (`open`, `resolved`, `waived`, `superseded`, `invalid`), supervisor-decision references, attempt counters, retry/time budgets, and terminal/needs-input state. Do not add storage or runtime wiring.
  Acceptance: state can represent the complete lifecycle without free-form control fields; IDs and references are stable; invalid transitions, duplicate IDs, incomplete evidence/resolutions, negative budgets, and terminal states with unfinished blocking work fail clearly; AW-01 JSON remains compatible.
  Verification: add table-driven validation and deterministic JSON round-trip tests in `internal/autonomous`; run `go test -count=1 ./internal/autonomous`, `go test -count=1 ./...`, and `git diff --check`.

- [x] AW-03 — Build a deterministic task-dossier projection and manifest.
  Scope: define a pure-input dossier model and stable Markdown renderer for task/spec bytes, execution state, plan, acceptance evidence, recent run summaries, verification, findings, Git snapshot, and repository-guidance sources. Produce a manifest with source identities, hashes, byte sizes, truncation facts, and the final dossier hash; do not read live repositories yet.
  Acceptance: identical inputs produce byte-identical dossier/manifest output; sections have a stable order; absent optional evidence degrades explicitly; bounded history truncation is visible; exact task and guidance source provenance is retained.
  Verification: add snapshot, determinism, source-hash, missing-section, and truncation tests; run the focused package tests, `go test ./...`, and `git diff --check`.

- [x] AW-04 — Assemble task dossiers from repository and runtime evidence.
  Scope: add a read-only assembler that populates AW-03 inputs from the canonical task file, autonomous state, ledger runs/events, receipts, verification evidence, current HEAD/status/diff summary, and applicable `AGENTS.md`/durable guidance. Bound history by explicit policy and return source-specific errors without mutating state.
  Acceptance: assembly is deterministic for a fixed repository/ledger snapshot; task identity and Git evidence cannot be mixed across tasks or HEADs; missing optional history is represented; missing/invalid required evidence blocks before a supervisor call; exact source references flow into the manifest.
  Verification: use temporary Git repos and ledger fixtures for complete, sparse, malformed, wrong-task, and changing-HEAD cases; run focused tests, `go test ./...`, and `git diff --check`.

- [x] AW-05 — Pin and record the intended Codex model, effort, and session mode.
  Scope: expose explicit Codex model and reasoning-effort config, default autonomous role sessions to `gpt-5.6-sol` and `xhigh`, pass `--model`, `-c model_reasoning_effort=...`, and `--ephemeral` to fresh `codex exec` calls, and surface effective values in config check/doctor. Record model, effort, Codex version, effective config hash, and invocation provenance in context/ledger evidence.
  Acceptance: every autonomous role invocation is explicit and reproducible; unsupported/empty settings fail preflight; existing aliases and mixed-pass behavior remain compatible; tests prove argument ordering and evidence recording without invoking live Codex.
  Verification: add config, codexexec, preflight, context, ledger, and CLI tests; run focused packages, `go test ./...`, relevant config/doctor commands, and `git diff --check`.

- [x] AW-06 — Seed repo-authored supervisor, planner, and corrector profiles.
  Scope: add concise but complete profile files and init templates for decision-only supervision, durable planning, and correction from explicit findings/verification failures. Profiles must require structured outputs, fresh-session behavior, evidence use, and scope discipline; supervisor must not edit product code or invoke Codex recursively.
  Acceptance: `revolvr init` seeds all three without overwriting existing files; profile names match AW-01 contracts; instructions distinguish planning, implementation, and correction responsibilities; missing profiles block clearly.
  Verification: add prompt/template/init non-overwrite tests; run `go test ./internal/prompt ./internal/cli ./...` and `git diff --check`.

- [x] AW-07 — Add a fresh, decision-only supervisor execution pass.
  Scope: build a supervisor prompt from an AW-04 dossier, invoke a fresh Codex session with the supervisor profile and a strict output schema, parse and validate exactly one AW-01 decision, and store exact prompt/output/decision artifacts plus ledger events. The pass must be read-only with respect to product source and must not route a worker yet.
  Acceptance: valid decisions are returned with provenance; malformed, extra, mismatched-task, or policy-invalid output blocks safely; any source mutation by the supervisor is detected and refused; no metadata-only Git commit is created.
  Verification: add fake-Codex tests for every decision class, schema/parse failures, wrong task, mutation detection, artifact writes, and ledger evidence; run focused tests and `go test ./...`.

- [x] AW-08 — Add compatible `autonomous-v1` task lifecycle metadata.
  Scope: extend canonical task parsing/updating with an explicit autonomous workflow and minimal harness-owned lifecycle state needed to locate durable autonomous evidence without turning the task spec into a model-authored scratchpad. Preserve immutable identity/spec bytes where required and keep `mixed-pass-v1` behavior available during migration.
  Acceptance: autonomous tasks load/select deterministically; invalid lifecycle metadata fails clearly; status-only retry preserves spec and evidence references; mixed-pass tasks and existing imports remain compatible; switching workflow cannot silently discard active state.
  Verification: add taskfile selection, parsing, atomic update, retry, migration-boundary, and compatibility tests; run `go test ./internal/taskfile ./...` and `git diff --check`.

- [x] AW-09 — Implement pure autonomous routing and completion policy.
  Scope: replace ordered-phase assumptions for `autonomous-v1` with a pure policy engine that validates AW-01 decisions against AW-02 state and legal prerequisites. Enforce mandatory independent audit after implementation/correction, acceptance evidence, verification gates, finding resolution, safe Git state, and terminal action rules without executing Codex or Git.
  Acceptance: every action has explicit legal/illegal state coverage; complete is impossible with failed/missing verification, stale audit, open blocking findings, unmet acceptance, or unsafe Git evidence; document/simplify may be skipped; mixed-pass policy remains unchanged.
  Verification: add exhaustive table-driven transition and completion-gate tests plus invalid/stale evidence cases; run focused policy tests and `go test ./...`.

- [x] AW-10 — Wire one supervisor-directed worker cycle.
  Scope: orchestrate one autonomous cycle that assembles a dossier, obtains and validates a supervisor decision, loads the selected repo-authored worker profile, starts a separate fresh Codex session, captures changes/receipt/verification, and returns updated evidence to supervision. Support one worker action only; do not add automatic correction looping yet.
  Acceptance: decision and worker have distinct run/session identities and artifacts; supervisor remains read-only; worker mutations are scoped; failed/missing decisions never start a worker; meaningful verified changes commit safely; decision-only/no-change evidence does not force a source commit.
  Verification: add end-to-end fake-runner tests for plan, implement, audit, terminal, malformed decision, worker failure, verification failure, and commit failure; run focused tests and `go test ./...`.

- [x] AW-11 — Persist durable plans and acceptance-evidence matrices.
  Scope: let planner/supervisor outputs create and revise AW-02 plans and acceptance criteria while preserving stable IDs and history. Record criterion status (`pending`, `satisfied`, `waived`, `not_applicable`) with typed evidence and validate revisions so completed work cannot silently disappear.
  Acceptance: plan updates are atomic/idempotent; criteria originate from the task/spec or an explicit supervisor refinement; satisfied/waived/not-applicable entries require evidence or rationale; completion policy consumes the matrix; CLI-independent storage survives fresh sessions and crashes.
  Verification: add persistence, revision, stale-write, identity, evidence, crash-reopen, and completion-gate integration tests; run focused tests and `go test ./...`.

- [x] AW-12 — Persist independent audits and finding lifecycles.
  Scope: execute audit in a fresh session with task/spec, diff, tests, and acceptance evidence while minimizing anchoring on worker self-report. Persist AW-01 audit reports and evolve findings through AW-02 resolution states with stable IDs and evidence; never silently drop a prior finding.
  Acceptance: source-changing work requires a newer audit; clean audits cannot erase open findings; resolved/waived/invalid findings require explicit evidence/rationale; blocking findings prevent completion; audit mutation follows an explicit policy and is not mistaken for review evidence.
  Verification: add fresh-audit, stale-audit, repeated/renamed finding, resolution, waiver, clean-with-open-finding, and ledger-reopen tests; run focused tests and `go test ./...`.

- [x] AW-13 — Add tiered verification and controlled flaky-test classification.
  Scope: support ordered structural, focused, task-acceptance, full-suite, and optional race/integration/security tiers. Corrections may use fast tiers, but final completion requires configured final tiers. Allow at most one policy-controlled rerun to classify a suspected flake while retaining both attempts as evidence.
  Acceptance: tier selection and required final gates are deterministic; a flaky pass is not silently treated as clean; timeouts/missing commands remain explicit; command identity/output caps and both rerun results are recorded; existing verification config remains compatible.
  Verification: add tier ordering, focused/final gate, flake pass/fail, timeout, missing-command, cancellation, and receipt/ledger evidence tests; run focused tests and `go test ./...`.

- [x] AW-14 — Add bounded verification/audit correction and re-audit routing.
  Scope: route verification failures and blocking audit findings into a corrector session with exact failure/finding references, then rerun required verification and a new independent audit before returning to completion consideration. Preserve failed-attempt evidence and known-good checkpoints.
  Acceptance: corrections cannot claim unrelated findings; resolved findings cite new evidence; failed correction never advances completion; verification and audit are newer than the correction; clean correction cycles return to supervisor; unsafe dirty state blocks instead of cascading.
  Verification: add fake end-to-end cycles for verification repair, multi-finding repair, partial repair, regression, correction failure, clean re-audit, and cancellation; run focused tests and `go test ./...`.

- [x] AW-15 — Add retry budgets, no-progress detection, and circuit breakers.
  Scope: enforce per-task/per-action attempt, elapsed-time, and optional token budgets. Detect repeated decision/finding/failure signatures, unchanged diff hashes, and materially identical strategies; require a changed strategy before retry and transition to blocked or needs-input when progress stops.
  Acceptance: counters survive restart; limits are deterministic; one transient failure can recover; identical loops terminate; supervisor cannot reset budgets; terminal evidence explains the triggered breaker; unrelated tasks are unaffected when state is clean.
  Verification: add restart, boundary, repeated-signature, changed-strategy, timeout/token, and task-isolation tests; run focused tests and `go test ./...`.

- [x] AW-16 — Make documentation and simplification conditional.
  Scope: let validated supervisor decisions run documentor or simplifier only when evidence identifies relevant work, or record an explicit not-applicable rationale. If either role changes source, require appropriate verification and a fresh audit; if it makes no change, record ledger evidence without a metadata-only source commit.
  Acceptance: clean tasks can skip either/both actions; required user-facing docs cannot be waived without rationale/evidence; cleanup cannot change behavior or bypass audit; no-op passes are auditable and do not clutter Git history.
  Verification: add run, skip, no-op, source-changing, failed-verification, stale-audit, and commit-history tests; run focused tests and `go test ./...`.

- [x] AW-17 — Add structured `needs_input` handling.
  Scope: extend autonomous contracts/state with a terminal-for-now supervisor outcome containing the exact question, blocking reason, mutually exclusive options, a recommendation, and independent work that may continue. Add answer/resume semantics without guessing a product decision.
  Acceptance: unanswered input prevents unsafe task actions; answers are durable and tied to the question/version; stale answers are rejected; clean needs-input tasks can yield the scheduler; CLI/TUI projections can consume the state later.
  Verification: add contract, persistence, stale-answer, resume, clean-yield, unsafe-state, and restart tests; run focused tests and `go test ./...`.

- [x] AW-18 — Isolate tasks in dedicated Git worktrees with checkpoint recovery.
  Scope: create and manage per-task branches/worktrees under local runtime state, record baseline/checkpoint SHAs, execute task mutations only there, and recover idempotently after crashes. Preserve the operator's primary worktree and make failed attempts inspectable without contaminating other tasks.
  Acceptance: creation/reopen/cleanup are safe; user-owned branches/worktrees are never deleted; failed verification can restore a harness-owned checkpoint; ambiguous commits remain inspectable; worktree identity is recorded; concurrent primary-worktree edits do not enter a task accidentally.
  Verification: add real-Git integration tests for create, checkpoint, crash reopen, failed rollback, branch conflict, dirty primary worktree, and cleanup refusal; run focused tests and `go test ./...`.

- [x] AW-19 — Define the unattended-execution safety boundary and preflight.
  Scope: add explicit autonomy modes and preflight checks for writable roots, external sandbox/container expectations, hook trust, command provenance, protected paths, secret redaction, network policy, and dangerous-bypass use. Worktree isolation must be described as Git isolation rather than a security sandbox.
  Acceptance: unsafe configuration blocks before Codex; effective permissions are visible and recorded; logs/artifacts redact configured secrets; tasks cannot expand writable scope through model output; full autonomy requires explicit acknowledgement; existing local dogfood remains configurable.
  Verification: add config/preflight/protected-path/redaction/policy tests and deterministic doctor output; run focused tests, `go test ./...`, config/doctor smoke commands, and `git diff --check`.

- [x] AW-20 — Add transactional terminal finalization and completion capsules.
  Scope: introduce an idempotent `finalizing` boundary that freezes task evidence and generates a human-readable completion capsule from validated plan, acceptance, verification, audit, findings, commits, run IDs, provenance, and timestamps before terminal ledger completion.
  Acceptance: capsules are deterministic and source-linked; incomplete gates cannot finalize; crash/retry does not duplicate or contradict evidence; commit sequences and waived/not-applicable items are explicit; failed capsule generation leaves a resumable state.
  Verification: add golden capsule, missing-gate, crash-between-steps, retry, stale-evidence, and deterministic regeneration tests; run focused tests and `go test ./...`.

- [x] AW-21 — Move terminal tasks into tracked archives with verify and reopen support.
  Scope: atomically move completed/cancelled/superseded/abandoned task specs to `.agent/archive/YYYY/MM/<task-id>/task.md`, store AW-20 `completion.md`, and add archive list/show/verify/reopen operations. Blocked and needs-input tasks remain active.
  Acceptance: archive paths and IDs are collision-safe; the administrative archive commit is identifiable; archive verification cross-checks task/capsule/ledger/commit evidence; reopen preserves history and creates a new active lifecycle; partial moves recover safely.
  Verification: add filesystem/Git/app/CLI tests for every terminal disposition, collision, partial failure, verification mismatch, reopen, and task selection exclusion; run focused tests and `go test ./...`.

- [x] AW-22 — Add run-one-task-until-terminal mode.
  Scope: add an app/CLI/TUI operation that selects or names one task and repeatedly runs fresh supervisor/worker cycles until completed, blocked, needs-input, cancelled, budget-exhausted, or unsafe. Do not spill surplus iterations into another task.
  Acceptance: terminal reason and statistics are deterministic; cancellation is safe; restart resumes durable state; max-cycle fallback remains available; current `--once` and `--max-passes` behavior stays compatible during migration.
  Verification: add fake and integration loops for completion, correction, input, blocker, budget, cancellation, restart, and “never starts next task”; run focused tests and `go test ./...`.

- [x] AW-23 — Add dependency-aware scheduling and supervised child tasks.
  Scope: support stable `depends_on`, priority, tags, optional conflicts, and supervisor-proposed child tasks with parent/evidence links. Select only ready tasks, reject dependency cycles/missing IDs, and require validated scope before persisting child tasks.
  Acceptance: ordering is deterministic; completed dependencies unlock dependents; blocked/needs-input parents have explicit child behavior; child tasks cannot mutate parent identity or invent broad roadmap work; imports and status surfaces preserve dependency metadata.
  Verification: add parser/store/scheduler tests for DAGs, cycles, missing dependencies, priority ties, conflicts, child creation, restart, and compatibility; run focused tests and `go test ./...`.

- [x] AW-24 — Add queue-until-exhausted and daemon-safe continuation.
  Scope: run ready tasks until none remain, continuing past cleanly isolated blocked/needs-input tasks while stopping on unsafe/ambiguous repository state. Add durable queue checkpoints and an optional daemon/watch mode that wakes for new ready task files without overlapping source writers.
  Acceptance: queue stop reasons distinguish drained, waiting dependencies/input, cancelled, budget, and safety stop; crash/restart is idempotent; one task cannot starve the queue; daemon mode debounces changes and respects locks; summaries identify every terminal task.
  Verification: add multi-task, blocked-skip, dependency-wait, unsafe-stop, starvation, cancellation, crash-restart, file-watch, and lock tests; run focused tests and `go test ./...`.

- [x] AW-25 — Add artifact retention, compression, garbage collection, and ledger export.
  Scope: classify pinned versus prunable artifacts, retain evidence referenced by active/blocked/recent/archive capsules, compress eligible old JSONL/stderr, export ledger history before pruning, version event schemas, and add dry-run-first GC plus archive/replay validation.
  Acceptance: dry-run exactly predicts mutations; pinned evidence is never removed; compressed artifacts remain readable/verifiable; export/replay reconstructs terminal evidence; interrupted GC resumes safely; retention is configurable and observable.
  Verification: add age/pin/compression/export/replay/interruption/path-safety tests and CLI dry-run snapshots; run focused tests and `go test ./...`.

- [x] AW-26 — Cache dossier sources by Git SHA and render role-specific projections.
  Scope: cache repository maps/guidance summaries and other immutable context by content/HEAD hash, invalidate precisely on source changes, and render minimal supervisor/planner/implementer/auditor/corrector/documentor/simplifier dossier views while retaining each exact sent payload as an artifact.
  Acceptance: cache hits are byte-equivalent to recomputation; stale inputs cannot leak across HEAD/task/profile; each role gets only required evidence; truncation/token estimates are visible; cache corruption falls back safely without hiding errors.
  Verification: add cache hit/miss/invalidation/corruption, cross-task isolation, role snapshot, and exact-payload retention tests; run focused tests and `go test ./...`.

- [x] AW-27 — Expose autonomous plans, findings, acceptance, and routing explanations through app/CLI.
  Scope: add read-only app projections and CLI commands for task plan, open/resolved findings, acceptance matrix, attempts/budgets, provenance, and “why this next action.” Keep rendering deterministic and raw evidence accessible by reference.
  Acceptance: operators can explain ready/completed/blocked/needs-input tasks without reading JSON; sparse/malformed legacy history degrades safely; commands do not mutate state; archived and active tasks are queryable.
  Verification: add app projection and byte-stable CLI tests for all lifecycle states, missing evidence, archives, and malformed history; run focused tests and `go test ./...`.

- [x] AW-28 — Add autonomous workflow visibility and controls to the TUI.
  Scope: render supervisor decision, current worker, plan progress, acceptance matrix, findings, retry/time budgets, verification tiers, worktree, archive status, and terminal reason. Add safe actions for answering input, cancelling, running a selected task to terminal, and starting/stopping the queue.
  Acceptance: important state is readable without color; long detail scrolls; controls honor app/preflight guards and one-active-run locking; refresh preserves selection; narrow/empty/legacy states remain coherent.
  Verification: add model/render/action/cancellation/resize tests across all lifecycle states plus CLI wiring coverage; run focused tests and `go test ./...`.

- [x] AW-29 — Add external notification hooks for unattended outcomes.
  Scope: provide configurable, bounded hooks for task completed, blocked, needs input, safety stop, queue drained, and daemon failure. Send a stable versioned JSON payload with task/run/archive references while redacting secrets; hook failure must be recorded without corrupting task state.
  Acceptance: event delivery is deduplicated/idempotent across restart; timeouts/output caps apply; unknown events/config fail clearly; retries are bounded; notifications never gain broader execution permissions than configured.
  Verification: add payload golden, redaction, timeout, retry, duplicate, restart, disabled-hook, and failure-isolation tests; run focused tests and `go test ./...`.

- [x] AW-30 — Add autonomous-loop metrics and deterministic evaluation scenarios.
  Scope: project task success, correction cycles, audit findings, no-progress stops, verification flakes, token/duration usage, archive latency, and queue throughput from ledger evidence. Build a fake-Codex evaluation suite covering straight success, correction, clean re-audit, conditional skips, no progress, needs input, blocked-skip, and crash finalization.
  Acceptance: metrics are reproducible from exported history; schemas are versioned; evaluations have deterministic fixtures and explicit expected terminal evidence; no live model call is required for the source-of-truth suite; optional live dogfood remains opt-in.
  Verification: run focused projection/evaluation tests, `go test ./...`, ledger export/replay fixtures, and `git diff --check`.

- [x] AW-31 — Add bounded parallel queue workers after isolation is proven.
  Scope: allow a configurable small number of independent task worktrees to run concurrently only when dependency/conflict analysis permits it. Serialize shared ledger/finalization/archive operations, retain per-task cancellation, and fall back to sequential execution on uncertain overlap.
  Acceptance: conflicting/dependent tasks never overlap; source-writer and archive races are prevented; worker failure/cancellation does not corrupt peers; output remains attributable; max concurrency is enforced; sequential mode remains default.
  Verification: add deterministic concurrency, dependency/conflict, cancellation, crash, ledger ordering, archive race, and fallback tests under `go test -race`; run `go test -race ./...` and `git diff --check`.

- [x] Reconcile ambiguous Git commit outcomes by comparing pre/post-commit HEAD.
  Scope: capture HEAD before staging, resolve it after every commit attempt, retry a failed post-commit lookup once, and classify a changed HEAD as committed even when the commit command or first SHA lookup reports failure. Preserve an explicit indeterminate state when post-commit HEAD cannot be resolved, without restoring stale task phase metadata.
  Acceptance: successful and initial commits still record their SHA; transient post-commit lookup failure recovers the created commit; a commit-command error with an advanced HEAD is recorded as committed; unchanged HEAD remains failed; unavailable post-commit HEAD is reported as indeterminate and leaves the task blocked at the transitioned phase for inspection.
  Verification: add focused `internal/commit` and `internal/runonce` regression tests; run `go test ./internal/commit ./internal/runonce`, `go test -race ./...`, `go vet ./...`, relevant CLI smoke commands, and `git diff --check`.

- [x] Add task workflow/phase metadata parsing and defaults.
  Scope: extend `internal/taskfile` frontmatter parsing for durable workflow metadata without changing runtime behavior yet. Support optional `workflow` and `phase` keys, default missing workflow to `mixed-pass-v1` and missing phase to `implement`, and accept the initial phases `implement`, `audit`, `document`, and `simplify`.
  Acceptance: existing `.agent/tasks/*.md` files without workflow metadata still load as pending implement-phase tasks; invalid workflow or phase values fail clearly; known frontmatter parsing remains deterministic; task selection order is unchanged.
  Verification: add focused `internal/taskfile` tests for defaults, valid explicit metadata, invalid metadata, duplicate keys, and unchanged runnable ordering; run `go test ./internal/taskfile` and `go test ./...`.

- [x] Add a pass-policy model mapping phases to profiles and outcome semantics.
  Scope: introduce a small runtime policy model for `mixed-pass-v1` that maps each phase to a profile, whether no-change success is allowed, and the next durable phase. Keep the model independent from `runonce` orchestration in this slice.
  Acceptance: `implement` maps to `implementer` and requires meaningful changes; `audit` maps to `auditor` and permits no-change success; `document` maps to `documentor` and permits no-change success; `simplify` maps to `simplifier` and permits no-change success; invalid workflow/phase combinations fail with actionable errors.
  Verification: add focused policy tests for every phase, phase order, terminal completion, no-change permissions, and invalid inputs; run the policy package tests and `go test ./...`.

- [x] Teach `runonce` to load the profile for the selected task phase.
  Scope: after selecting the canonical task file, resolve its workflow phase through the pass-policy model and load the mapped repo-authored profile instead of always loading `implementer`. Include workflow/phase/profile identity in task-selection context where it is useful for later audit.
  Acceptance: implement-phase tasks still load `.agent/profiles/implementer.md`; audit and document phases load their existing profiles; missing mapped profiles block before Codex with a clear message; context manifests continue recording the exact profile file used.
  Verification: add `internal/runonce` coverage for implement/audit/document profile selection, missing mapped profile failure, and context manifest source metadata; run `go test ./internal/runonce ./internal/prompt` and `go test ./...`.

- [x] Allow policy-permitted no-change success and durable phase advancement.
  Scope: update `runonce` finalization so successful phases advance the selected task file before the commit gate when policy allows it. Implementation no-change remains blocked; audit/document/simplify may succeed without code changes, advance the task phase or complete the task, and commit the task-file metadata transition.
  Acceptance: implement with no changes still produces `no_changes`; implement with changes advances to audit instead of completing the durable task; audit/document/simplify pass with no code changes can advance cleanly; the final successful phase marks the task completed; ledger events and receipts make the phase outcome auditable.
  Verification: add `internal/runonce` tests for implement no-change refusal, implement-to-audit advancement, no-change audit/document/simplify advancement, final completion, and changed-file capture after task metadata updates; run `go test ./internal/runonce ./internal/app` and `go test ./...`.

- [x] Seed `.agent/profiles/simplifier.md`.
  Scope: add a repo-authored simplifier profile and init template. The profile should direct the agent to reduce unnecessary complexity, duplication, and line count only when meaningful, create helpers only when they reduce real duplication or complexity, and stop cleanly when no simplification is worthwhile.
  Acceptance: `revolvr init` seeds `simplifier.md` without overwriting an existing file; the checked-in profile is concise and aligned with auditor/documentor style; tests cover template content and non-overwrite behavior.
  Verification: update focused prompt/CLI init tests; run `go test ./internal/prompt ./internal/cli` and `go test ./...`.

- [x] Surface task workflow state in CLI, TUI, status, and timeline views.
  Scope: expose task workflow/phase state through app status/task adapters, CLI task/status output, TUI Dashboard/Tasks/Run Detail surfaces, and timeline rows for phase selection or advancement events. Keep raw task files canonical.
  Acceptance: operators can see the current phase, next phase or completion state, and selected profile without opening the task file; empty and legacy task states render coherently; raw ledger events remain available for audit.
  Verification: add focused `internal/app`, `internal/cli`, and `internal/tui` tests for task list/status rendering, TUI task markers, run detail/timeline phase rows, and legacy/default metadata; run `go test ./internal/app ./internal/cli ./internal/tui` and `go test ./...`.

- [x] Document the mixed-pass task workflow.
  Scope: update operator-facing docs to explain durable tasks versus passes, task-file workflow metadata, phase order, profile responsibilities, no-change success for audit/document/simplify, and how Revolvr advances phases.
  Acceptance: README or an appropriate operator doc shows a minimal mixed-pass task file, explains `implement -> audit -> document -> simplify -> completed`, names the four profiles, and clarifies that Revolvr owns phase transitions based on receipts/outcomes.
  Verification: run `go test ./...`, relevant CLI help commands if docs mention them, and `git diff --check`.

- [x] Add a Markdown spec-to-task parser that preserves human-readable acceptance and verification notes.
  Scope: create a small internal parser for a documented Markdown task format without adding dependencies. Support repeated task sections with a required task body and optional summary, acceptance, and verification notes; preserve unknown section text in the generated task body.
  Acceptance: parser returns ordered task specs suitable for `internal/app.AddTask`; empty task text and malformed sections produce clear errors with line context; multiline task text remains readable in Codex prompts.
  Verification: add focused parser tests; run `go test ./internal/taskimport` and `go test ./...`.

- [x] Add an app-level task import and dry-run operation.
  Scope: expose parsed task imports through `internal/app`, with dry-run and write modes. Validate all parsed tasks before writing; write mode creates tasks in input order and returns created IDs.
  Acceptance: dry-run reports the tasks that would be created without mutating `.revolvr/`; write mode persists every valid task in order; parse and validation errors do not partially write tasks.
  Verification: add `internal/app` tests for dry-run, ordered import, validation failure, parse failure, and empty import; run `go test ./internal/app` and `go test ./...`.

- [x] Add `revolvr task import <path>` with `--dry-run`.
  Scope: wire the CLI to the app import operation. Print numbered dry-run rows and created task IDs, while keeping existing `task add` and `task list` output unchanged.
  Acceptance: `--dry-run` does not mutate task state; import creates tasks in parsed order; unreadable files and parse failures return clear command errors.
  Verification: add focused CLI tests for help, dry-run, successful import, parse errors, and unreadable paths; run `go test ./internal/cli -run 'TestTaskImport|TestTask(Add|List)'`, `go test ./...`, and `go run ./cmd/revolvr task import --help`.

- [x] Document the chat/spec-to-task workflow and import format.
  Scope: add README guidance for using web chat to design specs, saving a Markdown task file, dry-running/importing it, refreshing the TUI, and running one pass from the TUI. Include the caution that chat and TUI can share task state, but concurrent code edits against the same repo should be avoided.
  Acceptance: docs include a minimal import-file example with summary, acceptance, and verification notes, plus commands for dry-run, import, TUI refresh, preflight, and run-once.
  Verification: run `go test ./...` and `go run ./cmd/revolvr task import --help`.

- [x] Surface the next runnable task more clearly in the TUI Dashboard and Tasks view.
  Scope: highlight the first pending task, show pending/blocked/completed counts near the task area, and distinguish `ready to run` from `nothing runnable` without relying on color.
  Acceptance: Dashboard shows the next task ID and summary when present; Tasks view marks both the current selection and the next runnable task; uninitialized and empty states still render coherently.
  Verification: add focused `internal/tui` render tests for pending, blocked-only, completed-only, and empty task lists; run `go test ./internal/tui` and `go test ./...`.

- [x] Add TUI blocked-task retry for the selected task.
  Scope: add a Tasks-view action backed by `internal/app.RetryTask`, refresh after success, and display clear inline messages for non-blocked tasks, missing callbacks, and retry errors.
  Acceptance: a blocked selected task can be returned to pending without leaving the TUI; pending and completed selected tasks are not mutated; the footer/help reflects when retry is available.
  Verification: add TUI model tests for successful retry, non-blocked rejection, callback error, and refresh failure; add CLI wiring coverage if command setup changes; run `go test ./internal/tui ./internal/cli ./internal/app` and `go test ./...`.

- [x] Add an app-level run timeline projection from ledger events.
  Scope: build a reusable `internal/app` projection that converts a run history into ordered human-readable timeline rows for prompt creation, Codex start/progress/completion, verification, commit, receipt, and terminal outcome.
  Acceptance: timeline rows include timestamp, phase, status, and concise detail; completed, failed-verification, Codex-failed, blocked, and missing-artifact histories degrade gracefully without panics.
  Verification: add `internal/app` tests for completed, failed verification, Codex failed, blocked, and missing-event histories; run `go test ./internal/app` and `go test ./...`.

- [x] Render the run timeline in CLI `show` and TUI Run Detail.
  Scope: surface the app timeline in `revolvr show <run-id>` and the TUI Run Detail view, while keeping raw event visibility available in the TUI when practical.
  Acceptance: users can understand the run flow without reading raw JSON or ledger payloads; long timelines remain scrollable; CLI output remains deterministic in tests.
  Verification: add focused CLI and TUI tests for timeline rendering and long timeline scrolling; run `go test ./internal/cli ./internal/tui` and `go test ./...`.

- [x] Add a controlled TUI run-next-N flow backed by `internal/app.RunLoop`.
  Scope: add a bounded multi-pass action in the TUI that runs up to a small user-selected pass count, reuses preflight readiness and cancellation controls, and streams pass summaries into the progress pane.
  Acceptance: users can run multiple passes without leaving the TUI; the TUI honors the same stop reasons and guardrails as `run --max-passes`; cancellation reports a clear terminal state and refreshes state.
  Verification: add fake-runner TUI tests for max-pass completion, no-task stop, failure guardrail, blocked stop, and cancellation; run `go test ./internal/tui ./internal/app ./internal/cli` and `go test ./...`.

### Wide-Sweep Audit Follow-up (2026-07-13)

Completion and verification evidence for every item below is preserved in
`.agent/STATE.md` and `.agent/DECISIONS.md`.

- [x] AUD-01 — Close ignored-file gaps in source and verification evidence.
  Scope: detect and classify policy-relevant ignored state at admission, after worker execution, and after verification; prevent unexplained ignored inputs from entering checkpoints or verified revision claims.
  Acceptance: non-allowlisted ignored inputs fail closed or carry explicit safe identity evidence, while harness-owned ignored state remains deterministic and secret contents are never logged.
  Verification: use real Git repositories with nested ignores, worker/verification-created ignored files, symlinks, allowlisted runtime paths, and clean-checkout replay; run focused tests, `go test ./...`, and relevant race tests.

- [x] AUD-02 — Fail closed on source-lock heartbeat failure.
  Scope: propagate source-writer heartbeat/ownership errors into direct and autonomous runs, cancel active work, guard sensitive final boundaries, and retain release errors.
  Acceptance: lease loss prevents later commit, checkpoint, or completion and returns inspectable ownership plus persistence/cancellation evidence without goroutine leaks.
  Verification: add deterministic expiry, token-replacement, I/O, cancellation-race, and release-failure tests; run focused tests, `go test ./...`, and `go test -race ./...`.

- [x] AUD-03 — Terminate complete spawned process trees on cancellation.
  Scope: give commands a supported-platform process boundary, signal descendants gracefully, force-kill after the grace period, and preserve output/result semantics.
  Acceptance: child and grandchild processes cannot survive runner cancellation or mutate the worktree after return; normal and graceful-exit commands remain compatible.
  Verification: add helper-process tests with nested and signal-ignoring descendants plus sentinel writes; run focused tests, `go test ./...`, and `go test -race ./...`.

- [x] AUD-04 — Use safe read semantics for the live SQLite ledger.
  Scope: remove immutable-mode assumptions from live ledger readers, reserve them only for proven frozen copies, and keep dossier, metrics, and export reads transactionally coherent.
  Acceptance: concurrent readers/writers observe consistent snapshots without read APIs creating or mutating ledger files; frozen reads validate their immutability boundary.
  Verification: cover WAL/rollback modes, concurrent commits, reopen, busy/cancellation, sidecar files, and logical export equality; run focused tests and `go test -race ./...`.

- [x] AUD-05 — Preserve long Codex JSONL records without silent corruption.
  Scope: separate bounded previews from authoritative records, preserve complete redacted events up to an explicit record limit, and fail clearly above it.
  Acceptance: valid records larger than 64 KiB stay valid in the artifact, preview truncation is visible, and chunk/UTF-8/secret boundaries are deterministic.
  Verification: test large records, arbitrary chunks, missing newlines, invalid JSON, UTF-8, and redaction; run focused tests and `go test ./...`.

- [x] AUD-06 — Stream and bound Codex JSONL metrics parsing.
  Scope: after AUD-05, replace whole-file usage parsing with one consistent incremental record parser whose memory is bounded by record size.
  Acceptance: large artifacts parse with bounded memory, malformed/oversized records have explicit semantics, cancellation is prompt, and valid receipt totals remain compatible.
  Verification: add large-stream, allocation/benchmark, malformed-record, overflow, cancellation, and unreadable-file tests; run focused tests and `go test ./...`.

- [x] AUD-07 — Atomically publish redacted last-message artifacts.
  Scope: have Codex write to a restricted temporary path, redact into a second safe temporary file, then flush and atomically publish the canonical artifact with recovery cleanup.
  Acceptance: the canonical path never contains unredacted bytes, including at every injected crash/failure boundary, and successful output remains compatible.
  Verification: inject failures around child exit, read, redact, write, sync, rename, and directory sync; inspect canonical/temp paths after failure and restart; run `go test ./...`.

- [x] AUD-08 — Make Git status truncation fail closed.
  Scope: treat truncated machine output as incomplete evidence, use unambiguous NUL-delimited status parsing, and prohibit staging/checkpoint/completion from partial path sets.
  Acceptance: injected or real large-output truncation errors before `git add`; large and hostile-filename repositories either yield a complete set or no mutation.
  Verification: replace fail-open coverage and add real-Git large-status, rename, delete, untracked, newline, and non-UTF-8 cases; run focused tests and `go test ./...`.

- [x] AUD-09 — Stage repository paths with literal Git pathspec semantics.
  Scope: disable pathspec magic for every machine-generated staging path while preserving rename and deletion behavior.
  Acceptance: colon-leading, wildcard-looking, whitespace, newline, and leading-dash filenames stage literally and only the intended tree enters the index.
  Verification: add real-Git staging/tree assertions for hostile filenames, renames, and deletes; run focused tests and `go test ./...`.

- [x] AUD-10 — Preserve notification transition errors during cancellation.
  Scope: retain the last valid journal, join cancellation with persistence failures, and keep history/checkpoint restart authority intact in every cancellation branch.
  Acceptance: no durable failure is discarded or replaced by a zero journal, and restart yields the prior or fully persisted state rather than an inferred transition.
  Verification: inject history, journal, file-sync, directory-sync, and cancellation-timing failures, reopen each store, and run focused tests plus `go test ./...`.

- [x] AUD-11 — Harden runtime persistence against symlinked paths.
  Scope: apply a consistent protected-ancestor/final-path contract to task-run state, task-run locks, and the outer execution lease without beginning broad store deduplication.
  Acceptance: symlinks, wrong file types, unexpected hard links, and unsafe modes are rejected before any outside-root effect; ordinary reopen/recovery stays compatible.
  Verification: test every ancestor/final substitution and assert an outside sentinel remains untouched; run focused tests, `go test ./...`, and relevant race tests.

- [x] AUD-12 — Reject ambiguous YAML and invalid configuration numbers.
  Scope: require one YAML document, preserve numeric field presence, reject zero/negative positive-only values, and prevent seconds-to-duration overflow across all affected fields.
  Acceptance: omissions retain defaults while explicit invalid/trailing values return field-specific errors; valid configurations keep their effective behavior/hash.
  Verification: table-test every affected field and boundary plus second documents and comments; run focused tests, `go test ./...`, `config check`, and `git diff --check`.

- [x] AUD-13 — Remove the ledger snapshot N+1 query pattern.
  Scope: fetch selected runs and ordered events with O(1) SQL statements in one read transaction, without unbounded placeholder lists.
  Acceptance: ordering, `MaxEventID`, malformed payload bytes, cancellation, and returned snapshots remain compatible while query count no longer grows per run.
  Verification: add query-count coverage and large-history benchmarks/fixtures; cover empty events, limits, cancellation, and concurrent visibility; run focused tests and `go test ./...`.

- [x] AUD-14 — Replace flock busy spinning with cancellable backoff.
  Scope: replace repeated `runtime.Gosched` polling with a simple timer/bounded backoff while preserving OS-lock authority and prompt context cancellation.
  Acceptance: sustained contention consumes negligible CPU, cancellation is bounded, and waiters acquire after release without leaks or material uncontended regression.
  Verification: add contention, cancellation, many-waiter, error, and benchmark coverage; run focused tests, `go test ./...`, and relevant race tests.

- [x] AUD-15 — Conditionally consolidate proven persistence primitives.
  Scope: after AUD-11, inventory identical low-level storage mechanics and share only safe-path, locking, immutable-write, atomic-replace/sync, and readback primitives; keep lifecycle state machines separate.
  Acceptance: proceed only with lower net production LOC and equivalent fault/crash semantics; otherwise document the analysis and make no refactor.
  Verification: run every migrated store's fault/crash tests, `go test ./...`, `go test -race ./...`, LOC comparison, and `git diff --check`.

- [x] AUD-16 — Remove only demonstrably dead or no-op code.
  Scope: prove and remove the listed no-op helper, blank import references, unused parameter, and discarded values only when they have no side effect or contract role.
  Acceptance: production LOC decreases with no behavior change; intentional validation/evaluation remains explicit; no cosmetic TUI or API refactor is introduced.
  Verification: run focused touched-package tests, `go test ./...`, `go vet ./...`, and `git diff --check`; make no code change if safety cannot be proven.

### Second Wide-Sweep Audit Follow-up (2026-07-13)

Detailed evidence, failure scenarios, and test guidance are in
`CODEBASE_AUDIT_2026-07-13.md`. Each item below is one bounded follow-up task.

- [x] R2-01 — Replace raw SQLite main-file hashes with WAL-safe logical ledger authority.
- [x] R2-02 — Make artifact-retention exclusion race-free across every competing mutator.
- [ ] R2-03 — Reconstruct and validate GC recovery authority from immutable history and observed effects.
- [ ] R2-04 — Validate child-publication journals and consume one shared verified publication projection.
- [ ] R2-05 — Apply the protected runtime-path contract to queue and child persistence.
- [ ] R2-06 — Validate a contiguous legal queue transition history during recovery.
- [ ] R2-07 — Durably synchronize both sides of retention quarantine renames and cleanup.
- [ ] R2-08 — Unify ledger-export writer and reader record-size contracts.
- [ ] R2-09 — Preserve explicit empty verification-command configuration.
- [ ] R2-10 — Give bare `revolvr run` a real non-placeholder contract.
- [ ] R2-11 — Preserve both GC operation and result-rendering errors in the CLI.

## Completed History

The previous harness, app-boundary, and TUI operator-console tasks were completed on 2026-07-08. Details are preserved in `.agent/STATE.md` and the Git history.

## Blocked

None.
