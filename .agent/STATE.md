# Agent State

## Architecture 015 Versioned Planner Complete (2026-08-06)

- Task selected: `.agent/tasks/architecture-015-planner.md`, the sole task
  named by the architecture handoff. Architecture tasks 001-014 remain
  complete; task 016 was not started. No commit was created in this pass.
- Added reversible migration `db/migrations/00008_plans.sql`, named sqlc
  queries, and regenerated PostgreSQL code for `core.plans`, immutable
  `core.plan_versions`, ordered `core.plan_steps`, accepted-plan pointers,
  optimistic aggregate versions, and append-only candidate/acceptance events.
  Database triggers keep plan-version content immutable and prevent completed,
  skipped, or in-progress step state from regressing.
- Added `internal/planner` as the versioned planning boundary. It requires the
  exact scheduler-pinned task/version, run, project source, accepted task-014
  supervisor `plan` decision, and trusted host-policy route. It builds and
  hashes the bounded `revolvr-planner-dossier-v1` Section 13.2 projection from
  the task contract, architecture constraints, project map, module
  relationships, conventions, prior decisions, acceptance requirements,
  baseline verification, and prior plan, with explicit semantic-retrieval,
  conversation-history, broad-source, and absent-section omissions.
- The closed `revolvr-planner-output-v1` schema and prompt bind the exact task,
  task version, run, source, supervisor decision, dossier, prompt, response
  schema, model policy, host policy, plan, plan version, revision, and parent
  revision identities. The task-013 model boundary is invoked exactly once as
  a fresh, stateless, tool-free structured-output call. The planner package
  has no ambient credential, transport, tool, source-write, or task-intake
  capability.
- Host validation accepts 1-64 stably ordered steps only. Every canonical
  criterion is mapped exactly once in canonical order; step dependencies must
  name earlier steps; expected paths must remain beneath canonical task paths;
  test strategies must exactly match task-owned verification; and all evidence
  references must exist in the frozen dossier. Unknown fields, malformed or
  refused output, duplicates, reordering, invented criteria/dependencies,
  placeholders, unsupported tests, scope expansion, and stale identities fail
  before any candidate can be accepted.
- Plan revisions preserve the complete existing step prefix and its content.
  Existing steps require exact prior-plan/step/status lineage; status changes
  require dossier-backed transition evidence; new steps are appended pending;
  completed/skipped steps never regress. Material revisions carry a bounded
  explanation.
- Candidate insertion, ordered step insertion, plan-pointer acceptance, and
  both append-only events run in one PostgreSQL transaction when a host accepts
  a fresh candidate. Acceptance is an explicit `trusted_host` operation absent
  from model output. Optimistic concurrency gives concurrent acceptors one
  winner; an exact operation replay is idempotent. Candidate rows retain exact
  dossier, prompt, schema, model/host policies, request, completed model
  evidence, raw output, and canonical output.
- Focused fake-model tests prove valid exact identity/mapping, malformed,
  refusal, duplicate/unknown fields, duplicate/reordered steps, missing
  criteria, invented dependencies, placeholders, unsupported verification,
  scope expansion, model-output identity staleness, task/source/supervisor/
  dossier drift, monotonic revision lineage, and absence of API-key, external
  network, tools, hidden session, source mutation, or task recompilation.
- PostgreSQL tests used isolated Compose project `revolvr-architecture015` on
  loopback port `55433`. Migrations 1-8 applied, and migration 00008 passed
  down/up twice (including its immutable/monotonic triggers). Fixtures used
  fresh UUIDs and cleaned only exact planner-test rows. Tests proved stable
  ordered rows and complete provenance, one concurrent acceptance winner,
  forced final-event failure rolling back the plan pointer, plan, version,
  steps, and events, successful retry, exact replay, and database-level
  completed-step non-regression. The exact Compose containers, networks, and
  disposable PostgreSQL volume were removed after verification.
- Dependency decision: the standard library plus existing `pgx`, `sqlc`, UUID,
  task-013 model, task-014 supervisor/policy, and JSON Schema foundations were
  sufficient. No dependency was added; `go.mod` and `go.sum` are unchanged.
- Files changed: `db/migrations/00008_plans.sql`, `db/queries/core.sql`,
  generated `internal/storage/postgres/core.sql.go` and `models.go`, the new
  `internal/planner` package and tests, the task-015 status file,
  `.agent/STATE.md`, and `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was not
  changed because the implementation follows ADR-023 and the canonical task
  015 boundary without a new durable architecture choice.
- Required verification passed: `test -z "$(gofmt -l cmd internal)"`,
  `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml`,
  `env -u OPENAI_API_KEY REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go
  test ./internal/planner` with the isolated URL, `go test ./...`, and
  `git diff --check`. No live model or external network call was made.
- Result: **PASS**. Architecture tasks 001-015 are complete; tasks 016-025
  remain. There are no blockers. The next task is
  `architecture-016-tool-broker-implementer`.

## Architecture 014 Decision-Only Supervisor Complete (2026-08-06)

- Task selected: `.agent/tasks/architecture-014-supervisor.md`, the sole task
  named by the architecture handoff. Architecture tasks 001-013 remain
  completed; task 015 was not started. No commit was created in this pass.
- Added `internal/supervisor` v1 contracts that build and hash a bounded,
  frozen Section 13.1 dossier from the pinned task/version, run, source,
  lifecycle, plan, criteria, latest verification and audit, findings,
  attempts, strategies, budget, high-authority decisions, workspace
  reconciliation, and artifact manifest. The dossier always records explicit
  omissions for broad raw source, unrelated code, and conversation history,
  plus absence evidence for optional canonical sections.
- Added the closed `revolvr-supervisor-decision-v1` schema and strict parser.
  The decision binds the exact task version, run, source, dossier, prompt,
  response schema, model policy, host policy, and decision identities and
  admits exactly `plan`, `implement`, `correct`, `document`, `simplify`,
  `complete`, `block`, or `needs_input`. Duplicate JSON keys, unknown fields or
  actions, malformed output, refusal, stale identity, invalid evidence, and
  action-specific required/forbidden field violations fail closed.
- The runtime reads canonical state before and after the one fresh task-013
  invocation, requires the frozen identity and dossier to remain exact, and
  invokes the model boundary once with a one-attempt retry policy, strict
  structured output, no tools, and no prior conversation state. It never reads
  `OPENAI_API_KEY` or creates a transport itself.
- Added `internal/policy` as the deterministic trusted host-routing boundary.
  It validates lifecycle legality, remaining supervisor and downstream worker
  budgets, allowed/excluded path scope, plan/correction authority, and exact
  verification, independent audit, criteria, finding, workspace, and artifact
  evidence for completion. Worker actions become typed worker requests;
  `complete` becomes only a completion-preflight proposal; and `block` and
  `needs_input` remain typed advisory routes. This package performs no I/O and
  cannot transition lifecycle or mutate PostgreSQL.
- Accepted and rejected decisions are passed to an injected recorder with full
  task/run/source, dossier, prompt, schema, model-policy, host-policy, expected
  request, actual invocation, raw output, parsed decision, observed dossier,
  and route provenance. Model-controlled response bytes are persisted as
  opaque bytes so malformed rejected output remains exact and the complete
  record remains serializable. The supervisor package contains no PostgreSQL
  write adapter by design; task 014 defines the persistence contract without
  expanding into task 015's schema or lifecycle work.
- Focused fake-model tests prove one valid fixture for every admitted action;
  exact schema/dossier/prompt/model/policy/decision identity; accepted and
  rejected record persistence; host routing without direct state mutation;
  rejection of duplicate/unknown actions and fields, malformed output,
  refusal, stale task/source/dossier identity, illegal lifecycle, exhausted
  budget, scope broadening, and incomplete completion evidence; and absence of
  tools, PostgreSQL mutation, hidden session state, API-key use, and network.
- Dependency decision: existing standard-library facilities, task-013 model
  contracts, lifecycle contracts, and the existing JSON Schema dependency are
  sufficient. No dependency was added and `go.mod`/`go.sum` were unchanged.
- Files changed: `internal/policy/supervisor.go`,
  `internal/policy/supervisor_test.go`,
  `internal/supervisor/dossier_v1.go`,
  `internal/supervisor/decision_v1.go`,
  `internal/supervisor/decision_runtime_v1.go`,
  `internal/supervisor/decision_v1_test.go`, the task-014 status file,
  `.agent/STATE.md`, and `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was not
  changed because implementation follows the accepted ADR/specification
  boundary without introducing a new durable architecture choice.
- Required verification passed: `test -z "$(gofmt -l cmd internal)"`,
  `env -u OPENAI_API_KEY go test ./internal/supervisor ./internal/policy`,
  `go test ./...`, and `git diff --check`.
- Result: **PASS**. Architecture tasks 001-014 are complete; tasks 015-025
  remain. There are no blockers. The next task is
  `architecture-015-planner`.

## Architecture 013 OpenAI Structured-Output Client Complete (2026-08-06)

- Task selected:
  `.agent/tasks/architecture-013-openai-structured-output-client.md`, the sole
  task named by the architecture handoff. Architecture tasks 001-012 remain
  completed; task 014 was not started. No commit was created in this pass.
- Added `internal/model` as the trusted-process OpenAI Responses API boundary.
  `Prepare` validates and immutably pins the logical request, task, run, and
  source identities; model, reasoning effort, and maximum output; exact prompt
  version/hash; canonical response-schema version/hash/name; timeout and byte
  bounds; and normalized retry policy/hash before any transport begins.
- Current official OpenAI documentation and the `/v1/responses` OpenAPI 3.1
  document (specification version `2.3.0`) were checked before implementation.
  Requests use `POST /v1/responses`, `stream: true`, `store: false`,
  `reasoning.effort`, `max_output_tokens`, and strict Structured Outputs at
  `text.format = {type: json_schema, name, schema, strict: true}`. No
  `conversation` or `previous_response_id` is sent. Each transport attempt has
  a unique deterministic `X-Client-Request-Id` derived from the pinned logical
  request ID.
- Typed SSE processing enforces an increasing event sequence and bounded total
  stream and diagnostic bytes. Deltas and refusal fragments are redacted,
  bounded diagnostics only; the exact nested response from
  `response.completed` is canonical. The host requires a completed, non-stored,
  stateless response with matching metadata, validates the final JSON against
  the pinned strict schema, and then compares its exact `revolvr_identity`
  envelope to the pinned request/task/run/source/prompt/schema identity.
- Results distinguish success, safety refusal, malformed final JSON,
  schema-invalid output, stale identity, oversized stream, timeout,
  cancellation, retryable transport failure, retryable service failure,
  nonretryable failure, and exhausted retries. Only pinned transient statuses,
  admitted typed service codes, and admitted transport failures are retried;
  semantic failures, refusals, quota/spend failures, and incomplete responses
  are never reclassified as transient transport failures.
- The exact completed response is returned for semantic evidence unless it
  contains a configured secret, in which case it is withheld. Returned usage
  records input/output/total, cached, cache-write, and reasoning tokens;
  latency records local total and available server timestamps; service evidence
  records OpenAI request/response IDs, resolved model, and tier. Cost is
  explicitly unavailable because the Responses API does not report it.
- The API credential is accepted only into private client memory, is never a
  request/result field, and the credential-bearing client refuses JSON
  serialization. Request bodies/evidence are secret-scanned before transport;
  errors, typed diagnostics, refusal text, request IDs, service evidence, and
  string rendering are redacted. Secret-bearing completed responses are not
  returned.
- Added focused loopback `httptest` coverage for exact request and strict-schema
  transmission, completed-response authority over partial deltas, usage/cache/
  latency/service evidence, fresh-call isolation, refusal, malformed JSON,
  schema mismatch, stale identity, oversized streams, timeout, cancellation,
  transient HTTP/typed-stream/transport retries, nonretryable and quota errors,
  retry exhaustion, unique retry request IDs, and secret-sentinel absence from
  every recorded or returned diagnostic surface. Tests use a fake credential,
  never read `OPENAI_API_KEY`, and make no live or external network call.
- Dependency decision: added `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3`
  because strict validation of caller-supplied versioned Draft 2020-12 JSON
  Schemas is security-critical and is not safely reproducible with a small
  handwritten validator. Its use is bounded to schema compilation and final
  value validation; format assertions are enabled and external schema loading
  is rejected. `github.com/dlclark/regexp2 v1.11.0` is its checksum-recorded
  transitive dependency. No other dependency was added.
- Files changed: `go.mod`, `go.sum`, `internal/model/contracts.go`,
  `internal/model/client.go`, `internal/model/stream.go`,
  `internal/model/client_test.go`, the task-013 status file, and
  `.agent/STATE.md`, `.agent/HANDOFF.md`, and `.agent/DECISIONS.md`.
- Required verification passed: `test -z "$(gofmt -l cmd internal)"`,
  `env -u OPENAI_API_KEY go test ./internal/model`, `go test ./...`, and
  `git diff --check`. Additional checks passed:
  `env -u OPENAI_API_KEY go test -race -count=1 ./internal/model`,
  `go vet ./internal/model`, `go mod verify`, and `go mod tidy -diff`.
- Result: **PASS**. Architecture tasks 001-013 are complete; tasks 014-025
  remain. There are no blockers. The next task is
  `architecture-014-supervisor`.

## Architecture 012 Managed Workspace Lifecycle Complete (2026-08-06)

- Task selected: `.agent/tasks/architecture-012-workspace-lifecycle.md`, the
  sole task named by the architecture handoff. Architecture tasks 001-011
  remain completed; task 013 was not started.
- Implementation commit:
  `2db60eeb54cc8971015e59053652755d793012af` (`Add managed workspace lifecycle`).
- Added reversible migration `db/migrations/00007_workspaces.sql`, named sqlc
  queries, regenerated `internal/storage/postgres/core.sql.go` and
  `models.go`, and added `internal/workspace` manager, Git, lifecycle,
  candidate, cleanup, persistence, types, and focused test files.
- PostgreSQL is canonical for the explicit `planned`, `creating`, `ready`,
  `active`, `frozen`, `reconciling`, terminal, and `cleaned` states. A separate
  planned/applied operation journal binds branch creation, worktree creation,
  capture/artifact creation, candidate commit, and worktree cleanup to stable
  operation and material hashes before external effects are accepted.
- Creation pins the scheduler's exact run/task/project-source commit and tree
  to `refs/heads/revolvr/workspaces/<workspace-uuid>` and
  `<managed-workspace-root>/<workspace-uuid>`. Exact retries adopt matching
  Git/filesystem effects; pre-existing paths/refs, reused identities,
  divergent heads/trees, missing registrations, symlinks, or changed
  device/inode identities return typed conflicts without broad deletion.
- Trusted Git execution uses a replacement environment, disables system and
  global configuration, fixes `HOME` to a nonexistent path, disables hooks,
  prompting, credentials, file transport, and GPG signing, and uses fixed
  Revolvr commit identity. The sandbox binding contains only the symbolic
  managed source mounted read-write at `/workspace`; the original checkout
  and managed bare repository are forbidden host paths and no runtime socket
  is exposed.
- Host capture persists actual porcelain status, an ordered path manifest, a
  content-addressed binary/full-index diff artifact and SHA-256, and a
  single-parent candidate commit/tree whose message binds run, task,
  workspace, and operation. Completion, cancellation, and failure retain
  events, operations, branch/commit, and diff evidence while removing only
  the exact registered worktree.
- Focused PostgreSQL/Git fixtures cover the happy path, path and branch
  collisions, scheduler source drift, operation identity reuse, symlink
  substitution, repository and inherited-global hook attempts, cancellation,
  command timeout, durable cleanup failure plus exact retry, divergent cleanup
  replay, and injected crashes after branch, worktree, and candidate commit
  creation. Original checkout commit/tree, status, source snapshot, index,
  worktree bytes, canonical path, and root device/inode are identical before
  and after; crash recovery produces exactly one candidate commit.
- PostgreSQL verification used isolated Compose project
  `revolvr-architecture012` on loopback port `55432`. Migrations 1-7 applied;
  migration 00007 also passed down/up. Only architecture-012 fixture rows in
  that isolated database were cleared during diagnosis; unrelated data was
  not touched. The Compose containers and networks were stopped and removed
  after verification; the related PostgreSQL volume was retained.
- Verification passed: `test -z "$(gofmt -l cmd internal)"`, repeatable
  `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f
  db/sqlc.yaml`, `REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test
  ./internal/workspace ./internal/project` against the isolated PostgreSQL
  service, `go test -race -count=1 ./internal/workspace`, `go test ./...`, and
  `git diff --check`.
- Result: **PASS**. Architecture tasks 001-012 are complete; tasks 013-025 are
  pending. There are no blockers. The next task is
  `architecture-013-openai-structured-output-client`.

## Repository Audit Cleanup Complete (2026-08-04)

- Task selected: execute every actionable item in `AUDIT-FINDINGS.md`; the
  active architecture sequence was not advanced.
- Removed seven inert release-candidate workflows and 18 unreachable internal
  functions, applied only the Go 1.26 `slicescontains`, `minmax`, and
  `mapsloop` fixes, cleared the five named analyzer findings, and restored
  canonical Go module metadata.
- Commits: `150de11` (`Remove obsolete release candidate workflows`),
  `c6fa279` (`Remove unreachable internal helpers`), `80993e2` (`Use standard
  library collection helpers`), `3c6995d` (`Clear static analysis leftovers`),
  and `8756122` (`Tidy Go module metadata`).
- Verification passed: `bash -n` for every tracked shell script, repeated
  `go test ./...`, `go test -race -count=1 ./...` after the production
  reductions, `deadcode@v0.48.0 -test ./...`, the exact requested `go fix`
  diff check, the named Staticcheck diagnostic check, `go mod tidy -diff`,
  `go mod verify`, and `git diff --check`.
- Result: **PASS**. All five audit findings are complete. The next architecture
  task remains `architecture-012-workspace-lifecycle`; there are no blockers.

## Architecture 011 Sandboxd Complete (2026-08-04)

- Task selected: `.agent/tasks/architecture-011-sandboxd.md`, the sole task
  named by the architecture handoff. Architecture tasks 001-010 remain
  completed; task 012 was not started.
- Implementation commit:
  `6d1c72edd34ccc7c9d1968a6390249fdec36fdac` (`Add rootless sandbox runtime`).
- The implementation adds the specified `SandboxRuntime`
  create/exec/stop/inspect/remove boundary, a Docker CLI adapter that requires
  positive rootless-daemon evidence, typed strict-profile refusal without
  `runsc`, hardened direct create arguments, exact-label orphan reconciliation,
  descriptor identity rechecks, bounded lifecycle/output evidence, a framed
  local Unix socket server, and `cmd/revolvr-sandboxd`.
- Unit and fake-runtime coverage proves hardened argument translation,
  rootful/strict refusal, exact-owner reconciliation, deterministic evidence,
  timeout/cancellation cleanup, filesystem substitution refusal, malformed
  protocol refusal, client-disconnect cleanup, and mode-0600 socket creation.
- `dockerd-rootless-setuptool.sh check --force` passed. A disposable Docker
  29.7.1 rootless daemon used an isolated runtime/data root and pinned
  `alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce`.
  The live gate proved non-root execution, no host-home or runtime-socket
  access, no network under `none`, read-only-root enforcement, bounded tmpfs,
  deterministic timeout/cancellation cleanup, exact orphan reconciliation,
  and zero retained managed containers. The daemon and disposable root were
  removed after verification.
- The real daemon exposed two Docker CLI compatibility facts: Docker 29 emits
  lowercase not-found diagnostics and `docker ps --quiet` abbreviates IDs.
  The shared classifier now compares case-insensitively and reconciliation
  requests `--no-trunc`, retaining full create/reconcile identity.
- Passed verification: `test -z "$(gofmt -l cmd internal)"`, `go test
  ./internal/sandbox ./cmd/revolvr-sandboxd`, the opt-in real rootless
  `TestRootlessRuntimeSecurityProfile`, `go test -race ./internal/sandbox`,
  `go test ./...`, `go run ./cmd/revolvr-sandboxd --help`, and
  `git diff --check`.
- Files changed: `cmd/revolvr-sandboxd/main.go`, the task-011 runtime/server/
  test files under `internal/sandbox`, and this task's durable state files.
- Result: **PASS**. Architecture tasks 001-011 are complete; tasks 012-025 are
  pending. There are no blockers. The next task is
  `architecture-012-workspace-lifecycle`, which was not started in this pass.

## Architecture 010 Sandbox Specification Validator Complete (2026-08-04)

- Task selected:
  `.agent/tasks/architecture-010-sandbox-specification-validator.md`, the sole
  task named by the architecture handoff. Architecture tasks 001-009 were
  preserved as completed foundations; sandboxd/task 011 was not started.
- Implementation commit:
  `c3e74e0278ef725318ba4919ba91cf90c58a416b` (`Add sandbox request validator`).
- Added `revolvr-sandbox-request-v1` and a read-only `internal/sandbox`
  validator. Trusted policy pins the exact project/task/run/role, approved
  image reference and lowercase digest, permitted profiles/networks,
  environment names, maximum resources, forbidden host paths, and symbolic
  source registry. Requests contain no arbitrary host paths or OCI flags.
- Normalized specification identity hashes compact canonical JSON containing
  schema and scheduler identities, role, approved image, runtime profile,
  direct argv, target-sorted resolved mounts with canonical managed root/path,
  file type and device/inode identity, default-deny network, all resource
  bounds, and name-sorted explicit environment values. Input maps/slices are
  cloned and equivalent ordering/default forms hash identically.
- Descriptor-bound `internal/runtimepath` identity accessors let mount
  resolution reject path escapes, symlink components/substitution, unsafe
  modes, wrong types, and hard-linked files while retaining the validated
  device/inode evidence. Admission requires exactly one read-write
  `/workspace`; context and cache mounts are approved and read-only, with
  source/target overlap rejected.
- Strict is the default policy profile, compatible requires explicit policy
  authority, and diagnostic also requires attended authority. Network
  defaults to `none`, open network requires attended diagnostic authority, and
  verifier network is always `none`. Environment replacement accepts only
  allowlisted names and rejects host home/runtime configuration, SSH agent,
  PostgreSQL/OpenAI, cloud, and GitHub credential names.
- Tests cover deterministic normalization/hash, valid JSON and attended
  profiles, malformed/duplicate/unknown input, raw privilege/namespace/
  capability/device/socket fields, unknown images and versions, identity
  drift, command/environment bounds, every nonpositive and over-policy
  resource, network escalation, traversal, symlinks, hard links, wrong types,
  unsafe modes, source/target overlap, host-home/runtime-socket exposure, and
  mutable input detachment.
- Verification passed: formatting check, focused task-required sandbox tests,
  `go test ./...`, and `git diff --check`.
- Files changed: `internal/sandbox/validator.go`,
  `internal/sandbox/validator_test.go`, the two descriptor identity accessors
  in `internal/runtimepath/boundary.go`, and this task's durable state files.
- Result: **PASS**. Architecture tasks 001-010 are complete; tasks 011-025 are
  pending. There are no blockers. The next task is `architecture-011-sandboxd`.

## Architecture 009 PostgreSQL Scheduler And Leases Complete (2026-08-04)

- Task selected: `.agent/tasks/architecture-009-scheduler-leases.md`, the sole
  task named by the architecture handoff. Architecture tasks 001-008 were
  preserved as completed foundations; task 010 was not started.
- Implementation commit:
  `d190de9916a6b70df100c345f1165337f8097bd9` (`Add PostgreSQL scheduler leases`).
- Added reversible migration `00006_scheduler_leases.sql`, named sqlc queries,
  regenerated PostgreSQL code, and `internal/scheduler`. The canonical v1
  lease is the persistent singleton `global-source-mutation-v1`; a caller-
  supplied UUIDv7 run ID, exact accepted task version/aggregate, project source
  commit/tree, and coordinator identity are pinned atomically with the task's
  `pending` to `admitted` transition and both append-only events.
- Selection is priority ascending, task creation time, then stable task UUID.
  The projection rejects duplicate or ambiguous task identities, missing,
  duplicate, self, stale, cyclic, or dependency/conflict-overlap edges,
  unsupported lifecycle state, ambiguous project source authority, and an
  active task without matching run/lease authority. Waiting dependencies,
  terminal-unsatisfied dependencies, unresolved conflicts, operator
  checkpoints, and unhealthy/missing project source state never select.
- Admission reprojects under the locked lease row and compares every selected
  identity before writing. Equal concurrent admissions produced exactly one
  run/lease/task/event winner and one typed lease-busy loser. A forced failure
  on the final admission event rolled back run, lease, task, and both events;
  an exact retry then succeeded once. Exact admission replay is idempotent.
- Release requires the pinned task, source, admission events, lease owner, and
  resolved task state to agree. Restart reconciliation reports consistent
  active authority without mutation, releases exact resolved authority once,
  returns idle idempotently, and refuses foreign or unresolved ownership.
- Verification passed: `gofmt`, repeatable sqlc v1.27.0 generation, Goose
  migration up/down/up against isolated PostgreSQL 18/pgvector, focused
  scheduler/lifecycle/storage integration tests, scheduler `-race`,
  `go test -count=1 ./...` with PostgreSQL integration enabled, and
  `git diff --check`. The isolated Compose project and its disposable volume
  were removed after verification.
- Files changed: `db/migrations/00006_scheduler_leases.sql`,
  `db/queries/core.sql`, generated files under `internal/storage/postgres`,
  `internal/scheduler`, and the two new terminal-status constants in
  `internal/tasklifecycle/state.go`, plus this task's durable state files.
- Result: **PASS**. Architecture tasks 001-009 are complete; tasks 010-025 are
  pending. There are no blockers. The next task is
  `architecture-010-sandbox-specification-validator`.

## RC.20 Quantitative Gate Failed; Pending-Lifecycle Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to preparation
  and execution of one collision-free quantitative Level-1 real-Codex suite
  against exact RC.20 authority SHA-256
  `6e722fcc7b28cab0af190c9febc5d13d9655adbf1883b954f55f737c5000b627`.
  This was the sole task. `EXT-20` remains unchecked.
- Read-only admission passed at clean synchronized local, `origin/main`, and
  live remote `main` commit `5fbc82c5dbe61abcedf17dc79cdcf8062fbc599d`.
  Candidate workflow `--verify`, suite `--static`, authority identity, source
  ancestry, disk, collision absence, and RC.15-through-RC.19 root-preservation
  checks passed before preparation. Preparation installed isolated exact Codex
  `0.144.4` and created two clean disposable repositories without a model call
  at `.revolvr/ext20-level1-rc20-quantitative-20260731`, suite ID
  `ext20-791fbad09b76`.
- Live operation `ext20-791fbad09b76-01` completed initial planning,
  implementation, configured verification, one run-owned source commit, and
  checkpoint advancement. The next supervisor correctly chose `plan` because
  the durable plan and acceptance matrix were still pending. The planner
  returned a completed successor plan with exact terminal evidence and kept
  the task-origin criteria exact, but application stopped
  `unsafe_or_ambiguous`: `existing criterion "exact-output-content" changed
  requirement, origin, status, evidence, or rationale`.
- The root cause was an internal planning-contract contradiction. The
  supervisor and planner prompts required evidence-backed pending-to-terminal
  reconciliation and the core state transition accepted it, while
  `autonomousplanning.ApplyProposal` prohibited every change to an existing
  pending plan step or acceptance criterion. Its origin check would also have
  rejected an unchanged existing criterion sourced from an earlier supervisor
  decision.
- The single repair now allows existing pending/in-progress plan steps to
  progress monotonically while preserving IDs/descriptions, and allows pending
  acceptance criteria to receive supported terminal dispositions while
  preserving IDs/requirements/origins. Existing terminal steps and criteria
  must remain exactly unchanged, in-progress steps cannot regress, new entries
  cannot begin terminal, and new criteria still require the exact current task
  or supervisor origin. The worker prompt states these same boundaries.
- The retained manifest SHA-256 is
  `a4b21d8bce1aae9d0491f5e7b125190e8cf95f784c0e72eb1a591e11ebeb02b4`;
  terminal-operation SHA-256 is
  `107603722ffa30753a96aa31c150bb916f9b520e69ab72420c3ba15e40924385`.
  Direct manifest verification, exact before/after sentinel and control-HEAD
  checks, candidate/Codex/configuration checks, all collector ledger/receipt
  validations, clean disposable control repositories, and RC.15-through-RC.19
  root preservation pass. Exactly one operation and manifest exist; operations
  2-11 and the aggregate do not exist. RC.20 and this suite are immutable and
  cannot be retried, modified, relabeled, removed, or used for approval.
- The terminal evidence also records 1,032,842 consumed model tokens against
  the effective 1,000,000-token bound after the final planner call. Planning
  application failed first, so this candidate does not prove how the next
  admission would classify that overrun. The repair does not alter or evade
  the recorded bound; a later fresh candidate gate must prove bounded progress.
- Files changed: `internal/autonomousplanning/contracts.go`,
  `internal/autonomousplanning/contracts_test.go`,
  `internal/autonomousplanapply/apply_test.go`,
  `internal/autonomouscycle/worker_prompt.go`,
  `internal/autonomouscycle/cycle_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained RC.20 suite root. No dependency, commit, push, tag, release,
  external-use decision, remote ref, candidate, or historical evidence changed.
- Follow-up operator review replayed candidate `--verify`, suite `--static`,
  collector manifest verification, strict full-file hashes, exact terminal
  operation, containment, identity, ledger/receipt, token-bound, and
  RC.15-through-RC.20 preservation assertions. It inspected the pending durable
  state, exact evidence-backed proposed revision, failing validator boundary,
  and repaired validator/prompt contract. Focused normal/race tests, the full
  Go suite, and `git diff --check` pass. The operator explicitly authorized
  raw-Git commit and push of this exact nine-file failure-and-repair record.
- Verification commands: required durable-state reads; clean/local/tracked-
  remote/live-remote source checks; candidate `--verify`; suite `--static` and
  `--prepare`; the explicitly confirmed suite `--live`; direct collector
  `--verify-manifest`; exact operation-count, aggregate-absence, sentinel,
  control-HEAD, disposable-repository, candidate/Codex/configuration, ledger-
  export/replay, receipt, and historical-root checks; `gofmt`; focused normal
  and race tests for `internal/autonomousplanning`,
  `internal/autonomousplanapply`, and `internal/autonomouscycle`;
  `go test -count=1 ./...`; and `git diff --check`.
- Verification result: **RC.20 QUANTITATIVE GATE FAILED; SOURCE REPAIR
  VERIFICATION AND INDEPENDENT REVIEW PASSED**. After publication, what remains
  is a new source-bound candidate, exact remote CI, and a fresh quantitative
  suite in separate passes. Release progression is blocked on those gates;
  there is no implementation-test blocker.

## RC.20 Exact-Source Remote CI Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to the required
  remote-CI evidence gate for exact RC.20 source commit
  `7826c0fe97bd40508553702011475cea8b35e80f`. This was the sole task; no
  quantitative suite was prepared and no model call occurred. `EXT-20`
  remains unchecked.
- Clean local `main`, refreshed `origin/main`, raw-Git remote `main`, and the
  public GitHub commit API agreed at published candidate-record commit
  `5c1c46572184e8106ba2c7e63ee6ef89dfb689d8`. The public source commit and
  local object both resolve to tree
  `8583930512aefa07fb281cb5daaea3af49061fda`; that source is an ancestor of
  the published candidate record, and its candidate-workflow bytes equal the
  current workflow and recorded SHA-256
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
- Public GitHub Actions REST returned exactly one push-triggered `CI` run for
  the source SHA: run `30653427308`, number `150`, attempt `1`, branch `main`,
  status `completed`, conclusion `success`, created
  `2026-07-31T18:00:22Z`, and updated `2026-07-31T18:03:35Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30653427308`.
- The run contained exactly the ten mandatory unique jobs. Every job had the
  exact RC.20 source SHA, completed successfully, and had one successful
  `Report exact source commit` step:
  - `91231913549` — Go 1.22 source floor and tests
  - `91231913444` — Production autonomous strict-fake suite
  - `91231913220` — Race tests
  - `91231913333` — Vet and module verification
  - `91231913401` — Fake-Codex success smoke
  - `91231913379` — Fake-Codex verification-failure smoke
  - `91231913274` — Build linux/amd64
  - `91231913377` — Build darwin/amd64
  - `91231913376` — Build freebsd/amd64
  - `91231913501` — Build Windows diagnostic stub
- Post-CI verification replayed the reusable candidate workflow's read-only
  `--verify` mode and the dogfood suite's read-only `--static` mode against
  candidate-authority SHA-256
  `6e722fcc7b28cab0af190c9febc5d13d9655adbf1883b954f55f737c5000b627`.
  Direct strict complete-manifest verification passed. The bundle remained
  exactly 48 regular single-link files and 137,838,214 bytes, with no symlink,
  hard link, or residual `.work` root. All three build pairs remained byte-
  identical, all six inspected build IDs remained empty, embedded source
  metadata and `revolvr 0.1.0` output remained exact, retained vulnerability
  evidence still reports zero affected vulnerabilities, and the Linux
  candidate remained
  `1b63c3735339a4f9aa775b06a2d84eaad7ff9031f1461259409734a8601232e2`.
  RC.15 through RC.19 candidate authorities and failed-operation manifests
  also remained exact and immutable.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`. `.agent/DECISIONS.md` is unchanged because no durable
  implementation or architecture decision changed. No product, dependency,
  candidate, historical evidence, remote ref, tag, workflow, suite root,
  release, or external-use decision changed.
- Follow-up operator review independently confirmed the sole exact-source
  public Actions run, all ten required job identities, and exactly one
  successful source-reporting step per job; replayed raw-Git source identity,
  candidate `--verify`, suite `--static`, strict manifest, topology,
  reproducibility, metadata, empty build IDs, version, vulnerability, and
  RC.15-through-RC.19 preservation assertions. Every assertion and
  `git diff --check` passed without suite preparation or a model call. The
  operator explicitly authorized raw-Git commit and push of this exact
  three-file remote-CI record.
- Verification commands: required durable-state reads; clean worktree and
  local/fetched/raw-Git/public commit and tree checks; exact GitHub Actions
  run-count, run-identity, job-set, job-source, and source-reporting-step
  assertions through the public REST API; candidate workflow `--verify`;
  suite `--static`; direct strict `sha256sum --check --strict`; candidate
  authority, manifest, workflow, topology, artifact-pair, build-ID, embedded-
  metadata, version, and vulnerability-evidence checks; RC.15/RC.16/RC.17/
  RC.18/RC.19 preservation checks; and `git diff --check`. The unavailable
  `gh` executable was replaced by the public REST API without changing scope
  or evidence.
- Verification result: **PASS for the RC.20 exact-source remote-CI gate**.
  What remains is a separate fresh pass preparing and executing one new
  collision-free quantitative Level-1 real-Codex suite against only this exact
  RC.20 authority. There are no blockers for that next gate.

## RC.20 Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to construction
  and independent verification of exactly one new source-qualified candidate
  from the clean published plan-reconciliation repair. This was the sole task;
  `EXT-20` remains unchecked.
- Admission passed at clean local, `origin/main`, and live remote `main` commit
  `7826c0fe97bd40508553702011475cea8b35e80f`, tree
  `8583930512aefa07fb281cb5daaea3af49061fda`. Workflow bytes at the commit and
  in the worktree both hash to
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
  Exact Go 1.22.12, Go 1.26.5, and govulncheck 1.1.4 executable identities,
  available disk, candidate-path collision absence, and RC.15 through RC.19
  candidate/failure evidence all matched their recorded authority.
- The sole new output root is
  `.revolvr/release-candidates/level1-v0.1.0-rc.20-7826c0fe97bd`.
  Construction passed and emitted candidate-authority SHA-256
  `6e722fcc7b28cab0af190c9febc5d13d9655adbf1883b954f55f737c5000b627`,
  bundle-manifest SHA-256
  `134bab9aa2a7649bad7fd7362d382bfecf364b26cf02626f7a55e547dde42cf6`,
  and Linux candidate SHA-256
  `1b63c3735339a4f9aa775b06a2d84eaad7ff9031f1461259409734a8601232e2`.
  The workflow's separate read-only `--verify` and the dogfood suite's
  read-only `--static` gate also passed without fixture preparation or a model
  call.
- Direct strict manifest verification passed. The bundle has 48 regular
  single-link files, 137,838,214 bytes, no symbolic links, no hard links, and
  no residual `.work` root. Linux, Darwin, and FreeBSD pass-one/pass-two
  artifacts are byte-identical at SHA-256 values `1b63c373...`, `2c6e6dd9...`,
  and `790fc3fd...`; all inspected build IDs are empty. Embedded Linux metadata
  records Go 1.26.5, exact source commit `7826c0fe...`, and an unmodified VCS
  tree.
- Verification commands: required durable-state reads; clean local/origin/live-
  remote source, source tree, workflow, tool, version, disk, collision, and
  historical-preservation admission; reusable workflow `--build` and separate
  `--verify`; suite `--static`; strict manifest, topology, pair-equality,
  build-ID, embedded-metadata, version, and vulnerability inspection. The
  first independent command incorrectly looked for `bundle-manifest.sha256`;
  the one reasonable repair used the actual `SHA256SUMS` file and passed
  manifest/topology/pair/build-ID/metadata checks, but then incorrectly invoked
  the candidate as `revolvr version`, which exited with `unknown command`.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`, plus the ignored retained RC.20 candidate root. No
  dependency, product source, commit, push, tag, release, external-use
  decision, quantitative suite, model call, remote ref, or historical evidence
  changed. `.agent/DECISIONS.md` is unchanged because no durable implementation
  or architecture decision was made.
- Follow-up read-only review used the authority's recorded `--version`
  interface and the actual `SHA256SUMS`; replayed candidate `--verify` and
  suite `--static`; and independently checked the exact authority/manifest,
  48-file/137,838,214-byte topology, all three byte-identical artifact pairs,
  empty build IDs, embedded metadata, `revolvr 0.1.0` output, zero affected
  vulnerabilities, raw-Git source identity, and RC.15/RC.16/RC.17/RC.18/RC.19
  preservation. All assertions and `git diff --check` passed without modifying
  RC.20, preparing a suite, or starting a model call. The operator explicitly
  authorized raw-Git commit and push of the exact three-file candidate record.
- Verification result: **PASS for RC.20 candidate construction and independent
  verification**. The prior command mistakes were review-harness mistakes, not
  candidate failures. What remains after publication is exact-source remote CI
  for source commit `7826c0fe...`; a fresh quantitative suite remains a later
  separate gate. There are no blockers for remote CI.

## RC.19 Quantitative Gate Failed; Plan-Reconciliation Prompt Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its remaining
  quantitative Level-1 real-Codex gate against exact RC.19 authority SHA-256
  `5c6191a6276c91ba2b802c90c7fbb3270602d750a2c5dd44130a6749d0ef1ffb`.
  This was the sole task; `EXT-20` remains unchecked.
- Admission passed at clean synchronized `main` commit
  `4f2a2276eaff5ae8f3799253af3082756561ed6f`, with exact RC.19 candidate
  authority and RC.15/RC.16/RC.17/RC.18 candidate/failure identities
  preserved. Static verification and preparation installed isolated exact
  Codex `0.144.4` and created two disposable repositories without a model call
  at `.revolvr/ext20-level1-rc19-quantitative-20260731`, suite ID
  `ext20-56f74f29f1af`.
- Live operation `ext20-56f74f29f1af-01` completed planning, implementation,
  configured verification, one run-owned source commit, checkpoint
  advancement, and a clean independent audit. The audit cited exact evidence
  showing all three plan steps and all acceptance requirements were satisfied,
  but their canonical durable statuses remained pending. The next supervisor
  chose `complete`; the completion gate correctly stopped
  `unsafe_or_ambiguous` because step `create-result-file` was still pending.
- The retained manifest SHA-256 is
  `1bb053862b869e564376c128538d199fea775b78a3793f5383226e481f4f4f13`;
  terminal-operation SHA-256 is
  `8013a58b306cf351b4299bb578953a9a524b490278c517590daf6ddb58a83d09`.
  Direct manifest verification, exact outside-sentinel/control-HEAD checks,
  candidate/Codex/configuration checks, every collector ledger/receipt
  validation, and RC.15/RC.16/RC.17/RC.18 preservation checks pass. Exactly
  one operation and manifest exist; no later operation or aggregate exists.
- The one reasonable repair attempt changes only the harness-authored
  supervisor prompt. It says `complete` is valid only when the current durable
  state already marks the plan completed, every step terminal, and every
  acceptance criterion terminally dispositioned; evidence supporting pending
  lifecycle fields must route `plan` first for exact reconciliation. The
  deterministic completion validator remains unchanged and fail-closed.
- Files changed: `internal/supervisor/prompt.go`,
  `internal/supervisor/prompt_schema_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained RC.19 suite root. No dependency, commit, push, tag, release,
  external-use decision, candidate, remote ref, or historical evidence changed.
- Verification commands: required durable-state reads; clean Git, disk,
  collision, candidate, historical-preservation, Bash syntax, candidate
  `--verify`, and suite `--static` admission; suite `--prepare`; the explicitly
  confirmed suite `--live`; direct collector `--verify-manifest`; exact
  sentinel, source, candidate/Codex/configuration, operation-count,
  aggregate-absence, ledger-export, replay, receipt, and historical-
  preservation checks; `gofmt`; focused normal and race
  `TestBuildPromptIsDeterministicAndIncludesExactInputs`; and
  `go test -count=1 ./...`. Two initial read-only admission assertions used
  abbreviated historical hashes and a superseded clean-head identity; the
  corrected exact admission passed before preparation.
- Follow-up operator review replayed candidate `--verify`, suite `--static`,
  prepared-authority and strict-manifest verification; confirmed the terminal
  operation, clean audit, passed verification, one source commit/checkpoint,
  unchanged control HEAD/sentinel/candidate/Codex/configuration, operation
  count, aggregate absence, collector checks, and RC.15/RC.16/RC.17/RC.18
  preservation; and inspected the final dossier's zero terminal steps and six
  pending criteria alongside the rejected `complete` decision. The prompt-only
  repair directly requires a planner reconciliation before completion while
  leaving the deterministic validator unchanged. `gofmt`, focused normal/race,
  package, full-suite, and `git diff --check` all pass. The operator explicitly
  authorized raw-Git commit and push of the exact six-file repair record.
- Verification result: **RC.19 QUANTITATIVE GATE FAILED; SOURCE REPAIR
  VERIFICATION PASSED**. RC.19 and its suite are immutable and cannot be
  retried, removed, modified, relabeled, or used for external approval.
  Independent review passed and publication is authorized. What remains after
  publication is a new source-bound candidate, exact remote CI, and a fresh
  collision-free quantitative suite in separate passes. Release progression
  is blocked on those gates; there is no implementation-test blocker.

## RC.19 Exact-Source Remote CI Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to the required
  remote-CI evidence gate for exact RC.19 source commit
  `1285fe1fcdb3da15c28f5dbf45ab98f9167215e6`. This was the sole task; no
  quantitative suite was prepared and no model call occurred. `EXT-20`
  remains unchecked.
- Clean local `main`, refreshed `origin/main`, raw-Git remote `main`, and the
  public GitHub commit API agreed at published candidate-record commit
  `a595decc31ac9e8472f3794e7dff499388aebc5c`. The public source commit and
  local object both resolve to tree
  `ca363a63ce2491a7280a5b4d73a38241017b0340`; that source is an ancestor of
  the published candidate record, and its candidate-workflow bytes equal the
  current workflow and recorded SHA-256
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
- Public GitHub Actions REST returned exactly one push-triggered `CI` run for
  the source SHA: run `30647171845`, number `147`, attempt `1`, branch `main`,
  status `completed`, conclusion `success`, created
  `2026-07-31T16:27:10Z`, and updated `2026-07-31T16:30:53Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30647171845`.
- The run contained exactly the ten mandatory unique jobs. Every job had the
  exact RC.19 source SHA, completed successfully, and had one successful
  `Report exact source commit` step:
  - `91211298742` — Go 1.22 source floor and tests
  - `91211298773` — Production autonomous strict-fake suite
  - `91211298907` — Race tests
  - `91211298747` — Vet and module verification
  - `91211298759` — Fake-Codex success smoke
  - `91211298665` — Fake-Codex verification-failure smoke
  - `91211298838` — Build linux/amd64
  - `91211298886` — Build darwin/amd64
  - `91211298721` — Build freebsd/amd64
  - `91211298771` — Build Windows diagnostic stub
- Post-CI verification replayed the reusable candidate workflow's read-only
  `--verify` mode and the dogfood suite's read-only `--static` mode against
  candidate-authority SHA-256
  `5c6191a6276c91ba2b802c90c7fbb3270602d750a2c5dd44130a6749d0ef1ffb`.
  Direct complete-manifest verification passed. The bundle remained exactly
  48 regular single-link files and 137,829,370 bytes, with no symlink, hard
  link, or residual `.work` root. All three build pairs remained byte-
  identical, build IDs remained empty, embedded source metadata remained
  exact, and the Linux candidate remained
  `029692ea14b28a0c3a20f58f744e2afe17b921c35382367acf43510d1defce2d`.
  RC.15, RC.16, RC.17, and RC.18 candidate authorities and failed-operation
  manifests also remained exact and immutable.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was unchanged because no durable
  implementation or architecture decision changed. No product, dependency,
  candidate, historical evidence, remote ref, tag, workflow, suite root,
  release, or external-use decision changed.
- Verification commands: required durable-state reads; clean worktree and
  local/fetched/raw-Git/public commit and tree checks; exact GitHub Actions
  run-count/run-identity/job-set/job-source-step assertions through the public
  REST API; candidate workflow `--verify`; suite `--static`; direct strict
  `sha256sum --check --strict`; candidate authority, manifest, artifact,
  workflow, file, byte, link, work-root, build-ID, embedded-metadata, and
  version checks; RC.15/RC.16/RC.17/RC.18 preservation checks; and `git diff
  --check`. The first preservation command's final display-only glob used an
  invalid Bash range after all explicit assertions passed; the one repair
  attempt reran the complete check with explicit paths and passed.
- Follow-up operator review independently queried the public Actions REST API
  and confirmed the sole exact-source run, exact ten-job set, and exactly one
  successful source-reporting step per job. Raw-Git source identity, candidate
  `--verify`, suite `--static`, strict manifest, topology, reproducibility,
  embedded metadata, empty build IDs, version output, and RC.15/RC.16/RC.17/
  RC.18 preservation all passed again, as did `git diff --check`. No suite or
  model operation occurred. The operator explicitly authorized raw-Git commit
  and push of the exact three-file remote-CI record.
- Verification result: **PASS for the RC.19 exact-source remote-CI sub-gate**.
  Independent review passed and publication is authorized. What remains after
  publication is a separate fresh pass preparing and executing a new collision-
  free quantitative Level-1 real-Codex suite against only this exact RC.19
  authority. There is no blocker for the quantitative gate.

## RC.19 Candidate Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to construction
  and independent verification of exactly one new source-qualified candidate
  from the clean published planner-citation repair. This was the sole task; no
  quantitative-suite preparation or live Codex started and `EXT-20` remains
  unchecked.
- Admission passed at clean local, `origin/main`, and remote `main` commit
  `1285fe1fcdb3da15c28f5dbf45ab98f9167215e6`, tree
  `ca363a63ce2491a7280a5b4d73a38241017b0340`. Workflow bytes at that commit
  and in the worktree both hash to
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
  The exact admitted tools remained Go 1.22.12 SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The sole new output identity is
  `.revolvr/release-candidates/level1-v0.1.0-rc.19-1285fe1fcdb3`.
  Construction and separate read-only workflow verification passed. Exact
  candidate-authority SHA-256 is
  `5c6191a6276c91ba2b802c90c7fbb3270602d750a2c5dd44130a6749d0ef1ffb`,
  bundle-manifest SHA-256 is
  `3c9b67673b9661f08250b8a6290300b13bba28203a71b06ebf9193eec3d976a7`,
  and Linux candidate SHA-256 is
  `029692ea14b28a0c3a20f58f744e2afe17b921c35382367acf43510d1defce2d`.
- Linux, Darwin, and FreeBSD pass-one/pass-two SHA-256 values are respectively
  `029692ea14b28a0c3a20f58f744e2afe17b921c35382367acf43510d1defce2d`,
  `e4f303d4822794e4f152f1d2c2c963fe4832867284a95e620300220a794546ca`,
  and `28bc1afc6c5d7a0792ef78cacfac6c482a5b4af22567944a88234d3cd61e2bef`;
  every pair is byte-identical. The bundle has 48 regular single-link files,
  137,829,370 bytes, no symbolic links, and no residual `.work` root.
- Go 1.22/current ordinary tests, current race tests, module verification,
  vet, ordinary and verbose vulnerability scans, supported builds, embedded
  metadata checks, empty build IDs, strict complete-manifest verification,
  and both independent build comparisons passed. Govulncheck retained the
  uncalled Windows-only module finding `GO-2026-5024`; it found no reachable
  or imported-package vulnerability. The dogfood suite's read-only static gate
  also passed without fixture preparation or a model call.
- RC.15, RC.16, RC.17, and RC.18 candidate authorities still hash to their
  recorded `07172fbe...`, `a7ac0a73...`, `4909b90e...`, and `06d8e10e...`
  identities, and their failed quantitative-operation manifests still hash to
  their recorded `97804009...`, `e1f926a6...`, `93665a34...`, and
  `6f217cbc...` identities.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`, plus the ignored retained RC.19 candidate root.
  `.agent/DECISIONS.md` is unchanged because no durable implementation or
  architecture decision changed. No dependency, product source, commit, push,
  tag, release, external-use decision, historical evidence, or remote ref was
  changed.
- Verification commands: required durable-state reads; clean worktree and
  exact local/origin/remote source checks; source tree, workflow, tool,
  version, disk, collision, and historical-preservation admission; the
  reusable candidate workflow `--build` and separate `--verify` modes; the
  dogfood suite `--static` mode; strict `sha256sum --check --strict`;
  independent topology, artifact-pair, embedded-metadata, build-ID,
  vulnerability, version-output, and historical-preservation checks; and
  `git diff --check`.
- Follow-up operator review replayed both read-only gates; independently
  checked the exact authority and strict complete manifest, 48-file/
  137,829,370-byte topology, all three byte-identical build pairs, empty build
  IDs, embedded source metadata, version output, retained test/vulnerability
  evidence, and RC.15/RC.16/RC.17/RC.18 preservation; and refreshed raw-Git
  local/origin/remote source identity. All checks passed, including `git diff
  --check`; no candidate, suite, model call, dependency, or historical evidence
  changed. The operator explicitly authorized raw-Git commit and push of the
  exact three-file candidate record.
- Verification result: **PASS for the RC.19 candidate construction and
  independent-verification sub-gate**. What remains is a separate fresh pass
  obtaining and recording exact-source remote-CI evidence for source commit
  `1285fe1f...`; only after that passes may another fresh pass prepare and
  execute a collision-free quantitative Level-1 suite against this exact
  RC.19 authority. There are no blockers for the remote-CI gate.

## RC.18 Quantitative Gate Failed; Planning-Citation Prompt Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its remaining
  quantitative Level-1 real-Codex gate against exact RC.18 authority SHA-256
  `06d8e10e6de5e0ce0774afebc0d49dc543af6334ddba0a175517329729e024ef`.
  This was the sole task; `EXT-20` remains unchecked.
- Admission passed at clean `main` commit
  `b1f62297614ffcc53719c6a98fd000641c485009`. Static verification and
  preparation installed isolated exact Codex `0.144.4` and created two clean
  disposable repositories without a model call at
  `.revolvr/ext20-level1-rc18-quantitative-20260731`, suite ID
  `ext20-51c9684c419a`.
- Live operation `ext20-51c9684c419a-01` completed supervisor routing and one
  read-only planner attempt, then terminally stopped `unsafe_or_ambiguous`
  before implementation, verification, audit, or commit. The planner copied
  the exact canonical task origin and supervisor-decision artifact into
  top-level `inputs`; in `plan.provenance` it copied the task origin and
  decision reference exactly but changed the decision artifact kind from
  `file` to `plan` and paraphrased its detail. Planning application correctly
  rejected the non-exact citation with `plan provenance must contain the exact
  task source and supervisor decision artifact`.
- The retained manifest SHA-256 is
  `6f217cbcb02ed9ae24d41a5adf348a2abd495201831e0ef2a9bce448760fe3d4`;
  terminal-operation SHA-256 is
  `b66695df34fcd4af95cf650705ad451ac54564df55068e4174f830d2c53b5ca2`.
  Direct manifest verification, exact outside-sentinel comparison, all
  collector ledger/receipt validations, and unchanged candidate, Codex,
  approved configuration, control HEAD, and workspace HEAD checks pass.
  Exactly one operation and one manifest exist; no later operation or aggregate
  exists. RC.15, RC.16, RC.17, and RC.18 candidate/failure authorities remain
  exact and immutable.
- The one reasonable repair attempt changes only planner prompt construction.
  It renders the exact canonical task-origin and supervisor-decision artifact
  together, requires both evidence objects field-for-field in top-level
  `inputs` and `plan.provenance`, and explicitly forbids relabeling the decision
  artifact from `file` to `plan`. The host validator remains unchanged and
  fail-closed; no model output is normalized or repaired by the harness. The
  production-cycle regression asserts both the exact rendered pair and the
  explicit identity rule.
- Files changed: `internal/autonomouscycle/worker_prompt.go`,
  `internal/autonomouscycle/cycle_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained RC.18 suite root. No dependency, commit, push, tag, release,
  external-use decision, candidate, remote ref, or historical evidence changed.
- Verification commands: required durable-state reads; clean Git, collision,
  disk, candidate, historical-preservation, Bash syntax, and suite `--static`
  admission; suite `--prepare`; the explicitly confirmed suite `--live`;
  direct collector `--verify-manifest`; exact sentinel, source/workspace,
  candidate/Codex/config, operation-count, aggregate-absence, ledger-export,
  replay, receipt, and RC.15/RC.16/RC.17 preservation checks; `gofmt`; focused
  normal and race `TestRunEveryWorkerActionUsesExactProfileAndOneFreshWorker`;
  `go test -count=1 ./...`; and `git diff --check`.
- Follow-up operator review independently replayed the read-only static,
  strict-manifest, prepared-suite, terminal-operation, sentinel, control/
  workspace HEAD, candidate/Codex/configuration, operation-count, aggregate-
  absence, collector, and RC.15/RC.16/RC.17 preservation checks. Inspection of
  the raw planner output corrected the durable failure description: the plan-
  provenance copy changed both kind and detail while preserving the reference.
  The prompt-only repair already requires all three fields unchanged and leaves
  exact host validation untouched. `gofmt`, focused normal/race tests, the
  package and full Go suites, and `git diff --check` all pass. The operator
  explicitly authorized raw-Git commit and push of the exact six-file record.
- Result: **RC.18 QUANTITATIVE GATE FAILED; SOURCE REPAIR VERIFICATION PASSED**.
  RC.18 and its suite are immutable and cannot be retried, removed, modified,
  relabeled, or used for external approval. Independent review passed and
  publication is authorized. What remains after publication is construction
  and verification of a new source-bound candidate, exact remote CI, and a new
  collision-free quantitative suite in separate fresh passes. Release
  progression is blocked on those gates; there is no implementation-test
  blocker.

## RC.18 Exact-Source Remote CI Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to the required
  remote-CI evidence gate for exact RC.18 source commit
  `0bed41ef930e7db3d0486bc9a82de2b5720fe49f`. This was the sole task; no
  quantitative suite was prepared and no model call occurred. `EXT-20`
  remains unchecked.
- Clean local `main`, refreshed `origin/main`, raw-Git remote `main`, and the
  public GitHub commit API agreed at published candidate-record commit
  `cbd54e2dac36a0ca7ba1254be036237118a7e27f`. The public source commit and
  local object both resolve to tree
  `cfb4852f07ba4e7759d94cc2890fbaa2c47bec0f`; that source is an ancestor of
  the published candidate record, and its candidate-workflow bytes equal the
  current workflow and recorded SHA-256
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
- Public GitHub Actions REST returned exactly one push-triggered `CI` run for
  the source SHA: run `30641614557`, number `144`, attempt `1`, branch `main`,
  status `completed`, conclusion `success`, created `2026-07-31T15:08:39Z`,
  and updated `2026-07-31T15:11:22Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30641614557`.
- The run contained exactly the ten mandatory unique jobs. Every job had the
  exact RC.18 source SHA, completed successfully, and had one successful
  `Report exact source commit` step:
  - `91192694341` — Go 1.22 source floor and tests
  - `91192694318` — Production autonomous strict-fake suite
  - `91192694293` — Race tests
  - `91192694141` — Vet and module verification
  - `91192694223` — Fake-Codex success smoke
  - `91192694253` — Fake-Codex verification-failure smoke
  - `91192694190` — Build linux/amd64
  - `91192694262` — Build darwin/amd64
  - `91192694192` — Build freebsd/amd64
  - `91192694250` — Build Windows diagnostic stub
- Post-CI verification replayed the reusable candidate workflow's read-only
  `--verify` mode and the dogfood suite's read-only `--static` mode against
  candidate-authority SHA-256
  `06d8e10e6de5e0ce0774afebc0d49dc543af6334ddba0a175517329729e024ef`.
  Direct complete-manifest verification passed. The bundle remained exactly
  48 regular single-link files and 137,820,906 bytes, with no symlink, hard
  link, or residual `.work` root. All three build pairs remained byte-
  identical, build IDs remained empty, embedded source metadata remained
  exact, and the Linux candidate remained
  `e484b428981cd3e619ae923bc6bac8baa52352ef701a3212b473519ebf4b22ca`.
  RC.15, RC.16, and RC.17 candidate authorities and failed-operation manifests
  also remained exact and immutable.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was unchanged because no durable
  implementation or architecture decision changed. No product, dependency,
  candidate, historical evidence, remote ref, tag, workflow, suite root,
  release, or external-use decision changed.
- Verification commands: required durable-state reads; clean worktree and
  local/fetched/raw-Git/public commit and tree checks; exact GitHub Actions
  run-count/run-identity/job-set/job-source-step assertions through the public
  REST API; candidate workflow `--verify`; suite `--static`; direct strict
  `sha256sum -c`; candidate authority, manifest, artifact, workflow, file,
  byte, link, work-root, build-ID, embedded-metadata, and version checks;
  RC.15/RC.16/RC.17 preservation checks; and `git diff --check`.
- Follow-up operator review independently queried GitHub's public REST API,
  confirmed the sole exact-source run and all ten job identities with exactly
  one successful source-reporting step each, and replayed the read-only
  workflow, suite-static, strict manifest, reproducibility, embedded-metadata,
  empty-build-ID, topology, source/workflow identity, and RC.15/RC.16/RC.17
  preservation checks. All passed, including `git diff --check`; no suite or
  model operation occurred. The operator explicitly authorized raw-Git commit
  and push of the exact three-file remote-CI record.
- Result: **PASS for the RC.18 exact-source remote-CI sub-gate**. What remains
  after authorized publication is a separate fresh pass preparing and
  executing a new collision-free quantitative Level-1 real-Codex suite against
  only this exact RC.18 authority. There is no blocker for that next gate.

## RC.18 Candidate Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to construction
  and independent verification of exactly one new source-qualified candidate
  from the clean published audit-citation repair. This was the sole task; no
  quantitative-suite preparation or live Codex started and `EXT-20` remains
  unchecked.
- Admission passed at clean local, `origin/main`, raw Git, and public GitHub
  `main` commit `0bed41ef930e7db3d0486bc9a82de2b5720fe49f`, tree
  `cfb4852f07ba4e7759d94cc2890fbaa2c47bec0f`. Workflow bytes at that commit
  and in the worktree both hash to
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
  The exact admitted tools remained Go 1.22.12 SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The sole new output identity is
  `.revolvr/release-candidates/level1-v0.1.0-rc.18-0bed41ef930e`.
  Construction and separate read-only workflow verification passed. Exact
  candidate-authority SHA-256 is
  `06d8e10e6de5e0ce0774afebc0d49dc543af6334ddba0a175517329729e024ef`,
  bundle-manifest SHA-256 is
  `f52ef75540e5a29700398ad70fe1eab2435906b4d858e614a758cbd05d56ef8b`,
  and Linux candidate SHA-256 is
  `e484b428981cd3e619ae923bc6bac8baa52352ef701a3212b473519ebf4b22ca`.
- Linux, Darwin, and FreeBSD pass-one/pass-two SHA-256 values are respectively
  `e484b428981cd3e619ae923bc6bac8baa52352ef701a3212b473519ebf4b22ca`,
  `315bfaa98d3816dd3ebca0b7815e10fa9c0dffbf241ef4c075f37e4055f727ee`,
  and `42d8b5a53161c1b96bf5babf020ec2b8be6619c829c18e6a7daff2bcee6f7fd2`;
  every pair is byte-identical. The bundle has 48 regular single-link files,
  137,820,906 bytes, no symbolic links, and no residual `.work` root.
- Go 1.22/current ordinary tests, current race tests, module verification,
  vet, ordinary and verbose vulnerability scans, supported builds, embedded
  metadata checks, empty build IDs, strict complete-manifest verification,
  and both independent build comparisons passed. Govulncheck retained the
  uncalled Windows-only module finding `GO-2026-5024`; it found no reachable
  vulnerability. The dogfood suite's read-only static gate also passed without
  fixture preparation or a model call.
- RC.15, RC.16, and RC.17 candidate authorities still hash to their recorded
  `07172fbe...`, `a7ac0a73...`, and `4909b90e...` identities, and their failed
  quantitative-operation manifests still hash to their recorded `97804009...`,
  `e1f926a6...`, and `93665a34...` identities.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`, plus the ignored retained RC.18 candidate root.
  `.agent/DECISIONS.md` is unchanged because no durable implementation or
  architecture decision changed. No dependency, product source, commit, push,
  tag, release, external-use decision, historical evidence, or remote ref was
  changed.
- Verification commands: required durable-state reads; clean worktree and
  exact local/origin/raw-Git/public source checks; source tree, workflow, tool,
  version, disk, and collision admission; the reusable candidate workflow
  `--build` and separate `--verify` modes; the dogfood suite `--static` mode;
  strict `sha256sum --check --strict`; independent topology, artifact-pair,
  embedded-metadata, build-ID, vulnerability, and historical-preservation
  checks; and `git diff --check`.
- Follow-up operator review independently replayed workflow `--verify` and
  suite `--static`; checked strict complete-manifest verification, exact 48-
  file/137,820,906-byte topology, all three build pairs, embedded source
  metadata, empty build IDs, retained test and vulnerability evidence, exact
  local/origin/raw-Git/public source and workflow identity, and RC.15/RC.16/
  RC.17 preservation; and passed `git diff --check`. No fixture, suite, or
  model operation occurred. The operator explicitly authorized raw-Git commit
  and push of the exact three-file candidate record.
- Result: **PASS for the RC.18 candidate construction and independent-
  verification sub-gate**. What remains is a separate fresh pass obtaining and
  recording exact remote-CI evidence for source commit `0bed41ef...`; only
  after that passes may another fresh pass prepare and execute a collision-free
  quantitative Level-1 suite against this exact RC.18 authority. There is no
  blocker for the remote-CI gate.

## RC.17 Quantitative Gate Failed; Audit Citation Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its remaining
  quantitative Level-1 real-Codex gate against exact RC.17 authority SHA-256
  `4909b90eb351fb1eebff7249ae5138ff4af246d92fc100d1d1b8bf807b8f5700`.
  This was the sole task; `EXT-20` remains unchecked.
- Admission passed at clean local and `origin/main` commit
  `316697f01392d5e6d3b4cc347f2e22496e1dc3a6`. Static verification passed,
  preparation installed isolated exact Codex `0.144.4`, and two disposable
  repositories were created without a model call at
  `.revolvr/ext20-level1-rc17-quantitative-20260731`, suite ID
  `ext20-9c2b181a2a88`.
- Live operation `ext20-9c2b181a2a88-01` completed exact planning,
  implementation, configured verification, and a clean independent audit with
  three admitted and three completed attempts. The auditor cited the sole
  current verification evidence kind/reference exactly and independently
  described its meaning, but audit application stopped
  `unsafe_or_ambiguous`: `report inputs must cite every exact current
  verification evidence reference`.
- The root cause is an audit-contract mismatch. The prompt requires the report
  to cite every exact verification **reference**, while `AuditOutput.Validate`
  compared the complete `EvidenceReference`, including model-authored `detail`
  prose. The exact provenance envelope was already copied and validated
  separately. Thus an accurate paraphrase of a non-authoritative description
  rejected an otherwise exact reference.
- The retained manifest SHA-256 is
  `93665a348d761f0a6c041ff16e67e68921a84bed071f7f3f945bda43181673a0`;
  terminal-operation SHA-256 is
  `e0112df81b9b9fef4b81136b9524bec31acb8c97995146f2eb1020c77a5189d8`.
  Direct manifest verification, exact outside-sentinel comparison, all
  collector ledger/receipt validations, and unchanged candidate, Codex,
  configuration, and control HEAD checks pass. Exactly one operation manifest
  exists and no aggregate report exists; no later operation started. RC.15,
  RC.16, and RC.17 recorded authorities and failed manifests remain exact.
- The one reasonable repair attempt changes only the audit-report verification
  citation check to match durable identity `(kind, reference)`. It still
  rejects a changed kind or reference. Exact model provenance is still checked
  against host authority at audit apply, and correction-resolution evidence
  retains full-struct equality.
- Files changed: `internal/autonomousaudit/contracts.go`,
  `internal/autonomousaudit/contracts_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained RC.17 suite root. No dependency, commit, push, tag, release,
  external-use decision, candidate, remote ref, or historical evidence changed.
- Verification commands: required durable-state reads; clean Git/candidate and
  preservation checks; suite `--static`; suite `--prepare`; the explicitly
  confirmed suite `--live`; direct collector `--verify-manifest`; exact
  sentinel comparison; collector ledger/receipt validation; operation and
  aggregate counts; focused autonomous-audit tests; focused race test;
  `gofmt`; `go test -count=1 ./...`; and `git diff --check`.
- Follow-up operator review independently replayed the retained manifest,
  terminal-operation, sentinel, prepared-suite, ledger/receipt, candidate/
  Codex/config, operation-count, and RC.15/RC.16/RC.17 preservation checks;
  confirmed the report/provenance citation shared exact `(kind, reference)`
  and differed only in explanatory detail; reviewed the unchanged full-
  provenance and resolution-evidence boundaries; formatted both changed Go
  files; and reran focused, package, race, and full Go tests plus `git diff
  --check`. All passed. The operator explicitly authorized raw-Git commit and
  push of the exact six-file repair record.
- Result: **RC.17 QUANTITATIVE GATE FAILED; SOURCE REPAIR VERIFICATION PASSED**.
  RC.17 and its suite are immutable and cannot be retried, removed, relabeled,
  or used for external approval. Independent review passed and publication is
  authorized. What remains after publication is construction and verification
  of a new source-bound candidate, exact remote CI, and a new collision-free
  quantitative suite in separate fresh passes. There is no test blocker;
  release progression is blocked on those gates.

## RC.17 Exact-Source Remote CI Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to the required
  remote-CI evidence gate for exact RC.17 source commit
  `4cdd716d3bdefd08066fd11e436d326deaf4242c`. This was the sole task; no
  quantitative suite was prepared and no model call occurred. `EXT-20`
  remains unchecked.
- Clean local `main`, refreshed `origin/main`, raw-Git remote `main`, and the
  public GitHub commit API agreed at published candidate-record commit
  `7d4f652af96279597a2eb2717d151c724501326b`. The public source commit and
  local object both resolve to tree
  `3c688dafc1cbf57b59a9bf1e3beb13d327b65396`; that source is an ancestor of
  the published candidate record, and its candidate-workflow bytes equal the
  current workflow and recorded SHA-256
  `2a7ff48266b9bf3601b7f05ffef58fd36db0033a2afe2537c46043a3e69472e9`.
- Public GitHub Actions REST returned exactly one push-triggered `CI` run for
  the source SHA: run `30635879807`, number `141`, attempt `1`, branch `main`,
  status `completed`, conclusion `success`, created
  `2026-07-31T13:47:28Z`, and updated `2026-07-31T13:53:03Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30635879807`.
- The run contained exactly the ten mandatory unique jobs. Every job had the
  exact RC.17 source SHA, completed successfully, and had one successful
  `Report exact source commit` step:
  - `91173232198` — Go 1.22 source floor and tests
  - `91173232243` — Production autonomous strict-fake suite
  - `91173232212` — Race tests
  - `91173232207` — Vet and module verification
  - `91173232313` — Fake-Codex success smoke
  - `91173232239` — Fake-Codex verification-failure smoke
  - `91173232261` — Build linux/amd64
  - `91173232371` — Build darwin/amd64
  - `91173232238` — Build freebsd/amd64
  - `91173232254` — Build Windows diagnostic stub
- Post-CI verification replayed the reusable candidate workflow's read-only
  `--verify` mode and the dogfood suite's read-only `--static` mode against
  candidate-authority SHA-256
  `4909b90eb351fb1eebff7249ae5138ff4af246d92fc100d1d1b8bf807b8f5700`.
  Direct complete-manifest verification passed. The bundle remained exactly
  48 regular single-link files and 137,821,257 bytes, with no symlink, hard
  link, or residual `.work` root. The Linux candidate remained
  `40d374ff2842e9a695e801d30686fbda4fff55cd9b2f4fc3f7d74765b951cb2a`.
  RC.15 and RC.16 candidate authorities and failed-operation manifests also
  remained exact and immutable.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was unchanged because no durable
  implementation or architecture decision changed. No product, dependency,
  candidate, historical evidence, remote ref, tag, workflow, suite root,
  release, or external-use decision changed.
- Verification commands: required durable-state reads; clean worktree and
  local/fetched/raw-Git/public commit and tree checks; exact GitHub Actions
  run-count/run-identity/job-set/job-source-step assertions through the public
  REST API; candidate workflow `--verify`; suite `--static`; direct strict
  `sha256sum -c`; candidate authority, manifest, artifact, workflow, file,
  byte, link, and work-root checks; RC.15/RC.16 preservation checks; and
  `git diff --check`.
- Follow-up operator review independently queried GitHub's public REST API,
  confirmed the sole exact-source run and all ten job identities with exactly
  one successful source-reporting step each, and replayed the read-only
  workflow, suite-static, strict manifest, topology, source/workflow identity,
  and RC.15/RC.16 preservation checks. All passed, including `git diff
  --check`; no suite or model operation occurred. The operator explicitly
  authorized raw-Git commit and push of the exact three-file remote-CI record.
- Result: **PASS for the RC.17 exact-source remote-CI sub-gate**. What remains
  is a separate fresh pass preparing and executing a new collision-free
  quantitative Level-1 real-Codex suite against only this exact RC.17
  authority. There is no blocker for that next gate.

## RC.17 Candidate Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to construction
  and independent verification of exactly one new source-qualified candidate
  from the clean published control-root receipt repair. This was the sole task;
  no live Codex or quantitative-suite preparation started and `EXT-20` remains
  unchecked.
- Admission passed at clean local `main`, refreshed `origin/main`, raw-Git
  remote `main`, and public-REST `main` commit
  `4cdd716d3bdefd08066fd11e436d326deaf4242c`, tree
  `3c688dafc1cbf57b59a9bf1e3beb13d327b65396`. Workflow bytes at the commit
  and worktree were equal. Exact tools remained Go 1.22.12 SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The sole new output identity is
  `.revolvr/release-candidates/level1-v0.1.0-rc.17-4cdd716d3bde`.
  Construction and a separate read-only workflow verification passed. Exact
  candidate-authority SHA-256 is
  `4909b90eb351fb1eebff7249ae5138ff4af246d92fc100d1d1b8bf807b8f5700`,
  bundle-manifest SHA-256 is
  `5f3df6aae15ef7bf27afd66e93dfc7e62d53446f228854321174bafdf2201764`,
  and Linux candidate SHA-256 is
  `40d374ff2842e9a695e801d30686fbda4fff55cd9b2f4fc3f7d74765b951cb2a`.
- Linux, Darwin, and FreeBSD pass-one/pass-two SHA-256 values are respectively
  `40d374ff2842e9a695e801d30686fbda4fff55cd9b2f4fc3f7d74765b951cb2a`,
  `5acba83debc78c56ef128b9e452928844f442ea3103ca91dfa1436e84d8b76b5`,
  and `0b9cd87d2bd6aa530e3dac561b76ea3fbe0ff1226e4fab0f708f310dcb86ed9b`;
  every pair is byte-identical. The bundle has 48 regular single-link files,
  137,821,257 bytes, no symbolic links, and no residual `.work` root.
- Go 1.22/current ordinary tests, current race tests, module verification,
  vet, ordinary and verbose vulnerability scans, supported builds, embedded
  metadata checks, empty build IDs, and all reproducibility comparisons
  passed. Govulncheck retained the uncalled Windows-only module finding
  `GO-2026-5024`; it found no reachable or imported-package vulnerability.
  The dogfood suite's read-only static gate also passed without fixture
  preparation or a model call.
- RC.15 and RC.16 remain immutable. Their candidate authorities still hash to
  `07172fbe1f3cc2fd8930da84d71b6e66deadadab4d0cbfdd75cf3018ee7f87bd`
  and `a7ac0a73e27e72c77177ae4661ff8f6eee6f587e29f230704972475cccab5ccf`;
  their failed quantitative-operation manifests still hash to
  `97804009e36ae7010d3e913bc1b1c434e331196e232e12ce25b83c2e3b9e154c`
  and `e1f926a63a3ea07030281fe84ad0168f8c465072c28bf513161c25b1c681bb64`.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`, plus the ignored retained RC.17 candidate root.
  `.agent/DECISIONS.md` was unchanged because no implementation or
  architecture decision changed. No product source, dependency, historical
  evidence, model operation, suite, tag, release, or external-use decision
  changed.
- Verification commands: required durable-state reads; clean local/fetched/
  raw-Git/public-REST authority checks; exact workflow/tool/version/root/hash
  checks; the full reusable candidate `--build`; a separate workflow
  `--verify`; the dogfood suite `--static`; direct complete-manifest
  verification; status, reproducibility, file/byte/link/work-root, version,
  RC.15/RC.16 preservation, Git-cleanliness, and `git diff --check` checks.
- Follow-up operator review independently replayed workflow `--verify` and
  suite `--static`; checked the strict complete manifest, exact 48-file/
  137,821,257-byte topology, all three cross-platform build pairs, embedded
  source metadata, empty build IDs, retained test and vulnerability evidence,
  exact local/fetched/raw-Git/public source and workflow identity, and RC.15/
  RC.16 preservation; and passed `git diff --check`. No fixture, suite, or
  model operation occurred. The operator explicitly authorized raw-Git commit
  and push of the exact three-file candidate record.
- Result: **PASS for RC.17 construction and independent verification**. What
  remains is a separate fresh pass obtaining exact remote-CI evidence for
  RC.17, followed only later by a fresh collision-free quantitative Level-1
  real-Codex suite. There is no blocker for the next bounded gate.

## RC.16 Quantitative Gate Failed; Receipt-Root Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its remaining
  quantitative Level-1 real-Codex gate against exact RC.16 authority SHA-256
  `a7ac0a73e27e72c77177ae4661ff8f6eee6f587e29f230704972475cccab5ccf`.
  This was the sole task; `EXT-20` remains unchecked.
- Static admission passed at clean local and `origin/main` commit
  `950a588c3dfd7f313731e235e859d5ce180f9c07`. Preparation installed isolated
  exact Codex `0.144.4` and created two disposable repositories without a
  model call at `.revolvr/ext20-level1-rc16-quantitative-20260731`, suite ID
  `ext20-1212e8f1adba`. Prepared authority SHA-256 is
  `6c3619cf9ab1c532f9bf5c20227e2b3b1123aeafc8cf3b0fe94a2c538627114e`.
- Live operation `ext20-1212e8f1adba-01` crossed the planning-state boundary
  that failed RC.15, durably completed its planning attempt, and admitted an
  implementer. The implementer created the exact requested `results/a1.txt`
  and followed the supplied receipt instruction. The operation then stopped
  `unsafe_or_ambiguous` before verification or commit with `worker_source_after`:
  `policy-relevant ignored source or verification inputs are present:
  ".revolvr" (directory, classification=policy_relevant)`. No later suite
  operation started and no aggregate report exists.
- The root cause is exact control/execution-root ambiguity in the worker
  prompt. Worker evidence is stored under the control root, but the prompt
  rendered the repository-relative `.revolvr/receipts/<run>.md` path while
  Codex's working directory is the isolated task workspace. The real worker
  therefore correctly wrote that relative path inside the workspace. The
  source snapshot fail-closed rule correctly rejected the ignored runtime
  directory; the harness then synthesized the control-root receipt without
  observed changed files, so its independent changed-file validation also
  failed.
- The retained manifest SHA-256 is
  `e1f926a63a3ea07030281fe84ad0168f8c465072c28bf513161c25b1c681bb64`;
  terminal operation SHA-256 is
  `a4a805ad7c3a805fe5a5f8816c04e94fb034be60582fa12f2a9fe00f9fb9c680`.
  Direct manifest verification and exact outside-sentinel comparison passed.
  Control HEAD remained `00e9d7741dd7f366881c9ac4b1bbb6abf971c1ef`; candidate,
  Codex, approved configuration, and outside sentinel were unchanged. The
  task workspace retains the authorized uncommitted `results/a1.txt` plus the
  ignored worker-authored receipt as immutable failure evidence. RC.15's
  authority and failed manifest also remain exact.
- The one reasonable repair attempt changes the worker prompt to supply the
  exact absolute control-root receipt path while all durable artifact and
  ledger references remain repository-relative. The workspace regression now
  asserts that Codex still runs in the execution root but receives only the
  control-root receipt target. Focused autonomous-cycle and production tests,
  focused race coverage, and the full Go suite pass.
- Files changed: `internal/autonomouscycle/worker.go`,
  `internal/autonomouscycle/cycle_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained RC.16 suite root. No dependency, commit, push, tag, release,
  external-use decision, candidate, or historical evidence changed.
- Verification commands: required durable-state reads; clean Git and exact
  candidate-authority checks; Bash syntax and suite `--static`; suite
  `--prepare`; the explicitly confirmed suite `--live`; direct collector
  `--verify-manifest`; exact sentinel comparison; RC.15/RC.16 preservation
  hashes; `gofmt`; focused `go test -count=1 ./internal/autonomouscycle
  ./internal/app -run
  'Test(RunInWorkspaceUsesExecutionRootAndKeepsControlEvidenceAtControlRoot|ProductionAutonomousHappyPath|StrictFakeCodexContract)$'`;
  focused `go test -race`; `go test -count=1 ./...`; and `git diff --check`.
- Follow-up operator review independently replayed the exact retained manifest,
  terminal-operation, outside-sentinel, control/workspace HEAD, suite-
  preparation, candidate/Codex/config, and RC.15/RC.16 preservation checks;
  confirmed exactly one terminal operation and no aggregate report; reviewed
  the control/execution-root repair boundary; formatted both changed Go files;
  and reran the focused autonomous-cycle/production tests, focused race test,
  full Go suite, and `git diff --check`. All passed. The operator explicitly
  authorized raw-Git commit and push of the exact six-file repair record.
- Result: **RC.16 QUANTITATIVE GATE FAILED; SOURCE REPAIR VERIFICATION PASSED**.
  Independent review passed and publication is authorized. What remains after
  publication is construction/verification of a new source-bound candidate,
  exact remote CI evidence, and a new collision-free quantitative suite in
  separate fresh passes. RC.16 and its failed suite may not be retried,
  removed, modified, relabeled, or combined with later evidence. There is no
  implementation-test blocker; release progression is blocked on those new-
  candidate gates.

## RC.16 Exact-Source Remote CI Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to the separately
  authorized remote-CI evidence gate for exact RC.16 source commit
  `2be1c7831d5dd84d4871f8c9dca183ba2ec25dd9`. This was the sole task; no
  quantitative suite was prepared and no model call occurred. `EXT-20`
  remains unchecked.
- Clean local `main`, refreshed `origin/main`, and raw-Git remote `main` agreed
  at published candidate-record commit
  `568b14b89c0358f3f80437ae0f77a07c63535aa0`. The public GitHub commit API
  returned the exact RC.16 source SHA, its tree remained
  `a97c9d21d9a6fbb5eab5a5b0c0c2313944123ab4`, and that source is an ancestor
  of the published candidate record.
- Public GitHub Actions REST returned exactly one push-triggered `CI` run for
  the source SHA: run `30632362941`, number `138`, attempt `1`, branch `main`,
  status `completed`, conclusion `success`, created
  `2026-07-31T12:54:25Z`, and updated `2026-07-31T12:57:57Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30632362941`.
- The run contained exactly the ten mandatory unique jobs. Every job had the
  exact RC.16 source SHA, completed successfully, and had a successful
  `Report exact source commit` step:
  - `91161517081` — Go 1.22 source floor and tests
  - `91161516903` — Production autonomous strict-fake suite
  - `91161516993` — Race tests
  - `91161516902` — Vet and module verification
  - `91161516909` — Fake-Codex success smoke
  - `91161517006` — Fake-Codex verification-failure smoke
  - `91161517116` — Build linux/amd64
  - `91161517030` — Build darwin/amd64
  - `91161517084` — Build freebsd/amd64
  - `91161517027` — Build Windows diagnostic stub
- Post-CI verification replayed the reusable candidate workflow's read-only
  `--verify` mode and the dogfood suite's read-only `--static` mode against
  candidate-authority SHA-256
  `a7ac0a73e27e72c77177ae4661ff8f6eee6f587e29f230704972475cccab5ccf`.
  Direct complete-manifest verification passed. The bundle remained exactly 48
  regular single-link files and 137,821,064 bytes, with no symlink, hard link,
  or residual `.work` root. The Linux candidate remained
  `747d4580dbcdebf0c4bbed9e80734bc19b3df9b087b8bd751c47ee8eac8059bf`.
  RC.15's candidate authority and failed-operation manifest also remained
  exact and immutable.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`. `.agent/DECISIONS.md` was unchanged because no
  implementation or architecture decision changed. No product, dependency,
  candidate, historical evidence, remote ref, tag, workflow, suite root,
  release, or external-use decision changed.
- Verification commands: required durable-state reads; clean worktree and
  local/fetched/raw-Git/public commit and tree checks; exact GitHub Actions
  run-count/run-identity/job-set/job-source-step assertions through the public
  REST API; candidate workflow `--verify`; suite `--static`; direct
  `sha256sum -c`; candidate authority, manifest, artifact, workflow, file,
  byte, link, and work-root checks; RC.15 preservation checks; and
  `git diff --check`.
- Follow-up operator review independently repeated the public REST run and job
  assertions, including all ten successful exact-source reporting steps;
  replayed both read-only candidate gates, strict complete-manifest and bundle
  topology checks, exact source/workflow identity, and RC.15 preservation; and
  passed `git diff --check`. No model call or suite preparation occurred. The
  operator explicitly authorized raw-Git commit and push of the exact
  three-file durable record.
- Result: **PASS for the RC.16 exact-source remote-CI sub-gate**. What remains
  is a separate fresh pass preparing and executing a new collision-free
  quantitative Level-1 real-Codex suite against only this exact RC.16
  authority. There is no blocker for that next gate.

## RC.16 Candidate Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to construction
  and independent verification of exactly one new source-qualified candidate
  from the published planning-state repair. This was the sole task; no live
  Codex or quantitative-suite preparation started and `EXT-20` remains
  unchecked.
- Admission passed at clean local `main`, refreshed `origin/main`, raw-Git
  remote `main`, and public-REST `main` commit
  `2be1c7831d5dd84d4871f8c9dca183ba2ec25dd9`, tree
  `a97c9d21d9a6fbb5eab5a5b0c0c2313944123ab4`. Workflow bytes at the commit
  and worktree were equal. Exact tools remained Go 1.22.12 SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The sole new output identity is
  `.revolvr/release-candidates/level1-v0.1.0-rc.16-2be1c7831d5d`.
  Construction and a separate read-only workflow verification passed. Exact
  candidate-authority SHA-256 is
  `a7ac0a73e27e72c77177ae4661ff8f6eee6f587e29f230704972475cccab5ccf`,
  bundle-manifest SHA-256 is
  `1044231b5c2a096fa04f266f8d5403297d94ab4bd62c24f90c792fae18931af4`,
  and Linux candidate SHA-256 is
  `747d4580dbcdebf0c4bbed9e80734bc19b3df9b087b8bd751c47ee8eac8059bf`.
- Linux, Darwin, and FreeBSD pass-one/pass-two SHA-256 values are respectively
  `747d4580dbcdebf0c4bbed9e80734bc19b3df9b087b8bd751c47ee8eac8059bf`,
  `dc874a3aa04a2c9e2469398642e9c649f7f31fbe712be5f984805c7d4158c325`,
  and `b3b9ade1263d28f9a1f06d384ac29f5204d58836d5dea00b3b50d4c44dd90a8b`;
  every pair is byte-identical. The bundle has 48 regular single-link files,
  137,821,064 bytes, no symbolic links, and no residual `.work` root.
- Go 1.22/current ordinary tests, current race tests, module verification,
  vet, ordinary and verbose vulnerability scans, supported builds, embedded
  metadata checks, empty build IDs, and all reproducibility comparisons
  passed. Govulncheck retained the uncalled Windows-only module finding
  `GO-2026-5024`; it found no reachable or imported-package vulnerability.
  Direct `sha256sum -c SHA256SUMS` passed for all 48 manifest entries.
- RC.15 remains immutable: its candidate authority still hashes to
  `07172fbe1f3cc2fd8930da84d71b6e66deadadab4d0cbfdd75cf3018ee7f87bd`
  and its failed quantitative-operation manifest still hashes to
  `97804009e36ae7010d3e913bc1b1c434e331196e232e12ce25b83c2e3b9e154c`.
- Files changed: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/HANDOFF.md`, plus the ignored retained RC.16 candidate root. No
  product source, dependency, historical evidence, model operation, suite,
  tag, release, or external-use decision changed.
- Verification commands: required durable-state reads; clean local/fetched/
  raw-Git/public-REST authority checks; exact workflow/tool/version/root/hash
  checks; the full reusable candidate `--build`; a separate workflow
  `--verify`; direct complete-manifest verification; status, reproducibility,
  file/byte/link/work-root, RC.15 preservation, Git-cleanliness, and
  `git diff --check` checks.
- Follow-up operator review replayed the workflow `--verify` mode and suite
  `--static` gate; independently checked strict complete-manifest verification,
  inventory size, file/link/work-root bounds, cross-platform byte equality,
  embedded metadata, empty build IDs, retained test and vulnerability output,
  exact source/workflow identity, and RC.15 authority/failed-manifest
  preservation; and passed `git diff --check`. No fixture or model operation
  occurred. The operator explicitly authorized raw-Git commit and push of the
  exact three-file durable record.
- Result: **PASS for RC.16 construction and independent verification**. What
  remains is a separate fresh pass obtaining exact remote evidence for RC.16,
  followed only later by a fresh quantitative Level-1 real-Codex suite. There
  is no blocker for the next bounded gate.

## RC.15 Quantitative Gate Failed; Planning-State Repair Passes (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its remaining
  quantitative Level-1 real-Codex gate against exact RC.15 authority SHA-256
  `07172fbe1f3cc2fd8930da84d71b6e66deadadab4d0cbfdd75cf3018ee7f87bd`.
  This was the sole task; `EXT-20` remains unchecked.
- Static authority verification passed. Preparation installed isolated exact
  Codex `0.144.4` and created two disposable repositories without a model call
  at `.revolvr/ext20-level1-rc15-quantitative-20260731`. The live suite then
  started its first operation, `ext20-ad6fcc82272e-01`.
- The operation expected `completed` but terminally stopped
  `unsafe_or_ambiguous` before source mutation. Attempt admission/completion
  advanced canonical attempt accounting after the planning dossier was built;
  planning application then compared that pre-attempt dossier state with the
  later canonical state and rejected it with `dossier sources do not contain
  exact task/state identities (task=true state=false)`.
- The retained manifest SHA-256 is
  `97804009e36ae7010d3e913bc1b1c434e331196e232e12ce25b83c2e3b9e154c`;
  the captured terminal operation SHA-256 is
  `96ec31cdcf8a09e1cd13623cc1c161d68ac50410a5ff0eb64c99933705955e25`.
  Independent manifest verification and exact before/after sentinel comparison
  passed. Source HEAD remained `614df714536d13a6552859c6faad2ea65c64ab29`;
  candidate, Codex, approved configuration, and outside sentinel were
  unchanged; all receipt and ledger validations passed.
- The one reasonable repair attempt changed production planning application to
  carry the exact dossier state used before attempt accounting. It requires
  that state to be a valid canonical predecessor and permits only append-only
  attempt-accounting differences; unrelated state changes remain a typed
  dossier-identity refusal. A focused regression covers both acceptance and
  rejection, and the production runner now supplies the exact cycle state.
- Files changed: `internal/app/autonomous_run.go`,
  `internal/autonomousplanapply/apply.go`,
  `internal/autonomousplanapply/apply_test.go`, `.agent/TASKS.md`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, plus the
  ignored retained suite root. No dependency, commit, push, tag, release, or
  external-use decision changed.
- Verification commands: suite `--static`; suite `--prepare`; the confirmed
  suite `--live` invocation; direct collector `--verify-manifest`; exact
  sentinel `cmp`; focused `go test -count=1 ./internal/autonomousplanapply
  ./internal/app -run
  'TestApplyPlanningResultAcceptsDossierStateBeforeAttemptAccounting|TestProductionAutonomousHappyPath|TestStrictFakeCodexContract'`;
  `go test -count=1 ./...`; and `git diff --check`.
- Follow-up operator review independently reproduced the dossier-era compact
  state identity from terminal evidence by replacing only attempt accounting;
  replayed direct manifest and outside-sentinel verification; inspected the
  typed outcome, terminal operation, ledger export, and receipt validation;
  confirmed the implementation matches the established audit predecessor
  boundary; ran `gofmt` on all changed Go files; and passed the focused tests,
  `go test -count=1 ./...`, and `git diff --check`. No RC.15 operation or live
  model call was rerun. The operator explicitly authorized raw-Git commit and
  push of the exact seven-file repair record.
- Result: **RC.15 QUANTITATIVE GATE FAILED; SOURCE REPAIR VERIFICATION PASSED**.
  RC.15 and its failed suite are immutable evidence and cannot be retried or
  used for approval. What remains after publication is construction and remote
  verification of a new exact candidate, followed by a new quantitative gate
  from a fresh collision-free suite root. There is no implementation blocker;
  release progression is blocked on those required new-candidate gates.

## Reusable Candidate Constructed And Independently Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its current
  one-candidate construction and independent-verification stage. This was the
  sole task; no live Codex dogfood was started and `EXT-20` remains unchecked.
- Admission passed at clean local `main`, fetched `origin/main`, raw-Git public
  `main`, and public-REST `main` commit
  `5f340a8232a6d1bc9e8fff55fbe0f37ad0957085`, tree
  `d56ca3798c2fb2813c0a73193304bd88ba237b77`. Exact tools remained Go 1.22.12
  SHA-256 `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The first new output identity,
  `.revolvr/release-candidates/level1-v0.1.0-rc.14-5f340a8232a6`, stopped at
  its Go 1.22 floor test and retained exact typed failed evidence. Ambient
  `GOROOT=/usr/local/go` made the Go 1.22.12 driver select the Go 1.26.5 root
  and tools, producing deterministic tool-version mismatch diagnostics before
  any later check or artifact build. The failed root has no candidate
  authority and was not reused, modified, removed, completed, or relabeled.
- The single reasonable repair attempt changed no source or tool. A focused
  check proved that unsetting only ambient `GOROOT` makes each exact Go
  executable select its own root. A new output identity then constructed and
  independently verified
  `.revolvr/release-candidates/level1-v0.1.0-rc.15-5f340a8232a6`.
  Its authority SHA-256 is
  `07172fbe1f3cc2fd8930da84d71b6e66deadadab4d0cbfdd75cf3018ee7f87bd`,
  bundle-manifest SHA-256 is
  `8c24c9df94dd86dd1132079a1c8de76a0ff693fc4a307bc1061fc72bdf7a647e`,
  and exact Linux candidate SHA-256 is
  `c78ceffecb25361d2e3fa756b2955b2274426631ea8112562b59f83c0117f207`.
- The bundle contains 48 regular, non-symlink, single-link files totaling
  137,819,478 bytes and no residual `.work` root. Linux, Darwin, and FreeBSD
  pass-one/pass-two SHA-256 values are respectively
  `c78ceffecb25361d2e3fa756b2955b2274426631ea8112562b59f83c0117f207`,
  `2681ac92f5d09d8671e2597292a6e0a13bc81507cfb5e91098ccec6c8a3aa9ef`,
  and `b11a44c5bdb186f8aecb7b3947cac0a1b6ece96adbe7fa96bd321e3049c28a87`.
  Every pair is byte-identical, has an empty build ID, and records the exact
  clean source commit, target, architecture, and version.
- Go 1.22/current ordinary tests, current race tests, module verification,
  vet, ordinary and verbose vulnerability scans, supported builds, embedded
  metadata checks, and all three reproducibility comparisons passed.
  Govulncheck found no reachable or imported-package vulnerability and
  retained the uncalled Windows-only module finding `GO-2026-5024` separately.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, and
  `.agent/STATE.md`, plus the two ignored retained RC.14/RC.15 roots. No
  product source, dependency, historical evidence, model operation, tag,
  release, or external-use decision changed during the construction pass.
- Verification commands: required durable-state reads; clean Git and exact
  local/fetched/raw-Git/public-REST equality checks; exact tool version/root/
  SHA-256 checks; the retained RC.14 floor-failure inspection; the focused
  per-tool-root repair check; the full RC.15 candidate build; a separate
  workflow `--verify`; strict complete-manifest verification; authority,
  status, reproducibility, link, work-root, tool-root, vulnerability, file/
  byte-count, Git-cleanliness, and `git diff --check` reads.
- Follow-up operator review replayed the workflow `--verify` mode and suite
  `--static` gate; independently checked strict manifest verification,
  complete inventory, file/link/work-root bounds, RC.14 failure separation,
  RC.15 evidence classes, exact workflow identity, and the distinct intrinsic
  roots of both Go toolchains; and passed `git diff --check`. No fixture or
  model operation occurred. The operator explicitly authorized raw-Git commit
  and push of the exact three-file durable record.
- Result: **PASS for candidate construction and verification**. What remains
  is a later fresh pass running the quantitative Level-1 real-Codex dogfood
  gate against only the exact RC.15 authority above. `EXT-20` remains
  unchecked; there are no blockers for that next bounded stage.

## First Reusable Candidate Build Failed; Cleanup Repair Verified (2026-07-31)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its current
  one-candidate construction and independent-verification stage. This was the
  sole task; no live Codex dogfood was started and `EXT-20` remains unchecked.
- Clean source admission passed at local `main`, `origin/main`, and public
  `main` commit `463f13a7c54698493073f6a8feecdc76a55b2647`, tree
  `875567cbfa4aea225514994cd22a4903a7a33d44`. Exact tools were Go 1.22.12
  SHA-256 `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  Go 1.26.5 SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  and `govulncheck@v1.1.4` SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
- The first workflow use created retained ignored failed root
  `.revolvr/release-candidates/level1-v0.1.0-rc.13-463f13a7c546` for candidate
  ID `level1-v0.1.0-rc.13-463f13a7c546`. Go 1.22/current ordinary tests,
  current race tests, module verification, vet, ordinary and verbose
  vulnerability scans, supported builds, embedded metadata checks, and all
  three two-pass byte comparisons completed. Linux, Darwin, and FreeBSD hashes
  are respectively `b25ba2065a3b662054eff5679f387fd5ae146496512a60b80a1b3eab7ed6f2a2`,
  `06c5972c912d9ddf2293910191b93463fe951de99ccce16ddcef138d273cc47d`,
  and `6d5ab70c895bb2772fda4d6afb9680ffc1b4e34d92c926905c5c27b31a33021e`.
  Govulncheck found zero reachable or imported-package vulnerabilities and
  retained one uncalled Windows-only module finding, `GO-2026-5024`.
- Construction failed at final work-root removal because downloaded Go module
  cache directories are read-only and plain `find -delete` could not remove
  their entries. The partial cleanup removed the source clones and ordinary
  build caches before stopping on four module-cache trees. The EXIT handler
  then referenced function-local variables after scope unwind. Consequently
  the retained 1.5-GB failed root contains module-cache residue plus the
  required test, vulnerability, reproducibility, metadata, and artifact
  evidence, but no `status.tsv`, `SHA256SUMS`, or `candidate-authority.tsv`;
  it is not a candidate authority and must not be used for dogfood.
- The one reasonable repair attempt changed
  `scripts/build-level1-candidate.sh`: status writing now accepts explicit
  values captured safely into the EXIT trap, and final cleanup first grants
  owner write permission only within the workflow-owned work root. Focused
  EXIT-after-scope and read-only-cache cleanup probes passed, as did `bash -n`,
  `--help`, and `git diff --check`.
- Follow-up operator review independently reproduced both focused repair
  probes; checked the retained evidence, artifact hashes, byte equality, empty
  build IDs, source metadata, vulnerability result, and authority-file
  absences; ran Bash syntax across all 10 tracked shell scripts; and passed
  `go test -count=1 ./...` plus `git diff --check`. The operator explicitly
  authorized raw-Git commit and push of this exact four-file repair record.
- Files changed: `scripts/build-level1-candidate.sh`, `.agent/TASKS.md`,
  `.agent/HANDOFF.md`, and `.agent/STATE.md`, plus the retained ignored failed
  candidate root. `.agent/DECISIONS.md` was unchanged because no architecture
  or authority policy changed. During the construction pass, no dependency,
  commit, push, tag, release, model invocation, suite preparation, or dogfood
  operation occurred.
- Verification commands: clean/local/fetched/public Git identity checks; exact
  tool version, GOROOT, and SHA-256 checks; the full candidate workflow build;
  retained test/vulnerability/reproducibility evidence inspection; Bash syntax
  and help; focused scope-stable EXIT-status and read-only-cache cleanup probes;
  and `git diff --check`. Result: **candidate construction FAILED; repair
  verification PASSED**.
- What remains: a later fresh pass must prove the clean repair commit is
  identical at local, fetched, and public `main`, then use a new source-
  qualified output identity to construct and independently verify one
  candidate. It must not reuse or modify the failed root and must not start
  live dogfood.

## Reusable Level-1 Candidate Workflow (2026-07-30)

- Task selected: the first unchecked task, `EXT-20`, narrowed to its explicitly
  authorized reusable candidate build-and-verification workflow stage. This
  was the sole bounded task; `EXT-20` remains unchecked.
- Added executable `scripts/build-level1-candidate.sh`. Its build mode requires
  explicit candidate ID/version, a clean exact source commit and tree, a new
  output root, exact floor/current Go executables and versions, and an exact
  govulncheck executable. It verifies the running workflow against the source-
  commit copy before any output creation, uses two independent no-local clones
  and isolated caches, records Go 1.22/current/race/module/vet plus ordinary
  and verbose vulnerability evidence, builds Linux/Darwin/FreeBSD amd64 twice,
  verifies embedded metadata and empty build IDs, compares exact bytes, and
  emits a complete bundle manifest plus externally hash-bound
  `candidate-authority.tsv`. Failures after output admission retain a typed
  failed status and their work/evidence root.
- Updated `scripts/dogfood-external-level1-suite.sh` to remove its obsolete
  hard-coded RC.7 candidate. Every static, prepare, live, and verify-suite mode
  now requires an exact candidate-authority path and SHA-256, validates the
  bundle through the reusable workflow, derives the exact Linux binary/source/
  version identities from it, and persists the authority path/hash in prepared
  suite evidence.
- Files changed in this pass: `scripts/build-level1-candidate.sh`,
  `scripts/dogfood-external-level1-suite.sh`, `.agent/TASKS.md`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`. Pre-existing
  wrapper-cleanup changes were preserved and not reworked. Nothing under
  `.revolvr/` was modified.
- Verification commands: `bash -n` for every current `scripts/*.sh`; both new
  and updated `--help` paths; read-only missing-authority refusal for workflow
  verification; malformed candidate-ID build refusal with proof that no output
  root appeared; suite refusal without both authority arguments; search proving
  the obsolete RC.7 candidate identity/path is absent from the suite; and
  `git diff --check`.
- Verification result: **PASS**. No candidate was built, no repository fixture
  was prepared, no Codex/model process ran, and no commit, push, tag, artifact,
  release, or dogfood evidence was created. Go tests were not run because no Go
  source changed and this stage explicitly prohibited candidate construction;
  actual build/test/vulnerability execution belongs to the separate candidate-
  construction pass.
- What remains: `EXT-20` still requires one fresh candidate construction and
  independent verification, followed in a later pass by the quantitative real-
  Codex dogfood gate. The immediate candidate-build stage requires a clean
  exact commit containing this workflow and the current approved repository
  changes. The worktree is intentionally uncommitted under repository policy,
  so that stage must not start until the operator reviews the diff and gives
  explicit commit authorization. This is not a blocker to the completed
  implementation stage.

## Top-Level Agent Wrapper Policy And Cleanup (2026-07-30)

- Task selected: settle and implement the repository policy for accumulated
  tracked top-level `agent-*.sh` wrappers. This was the sole bounded task.
- Read-only inventory found 69 tracked top-level wrappers. Retained 2 as
  genuinely reusable fresh-pass infrastructure: `agent-one.sh` and
  `agent-loop.sh`. Required-current-authority count is 0, uncertain count is
  0, and 67 one-shot development-control wrappers were classified as retired
  historical artifacts and removed from the current tree. The exact per-file
  classification is recorded in `.agent/DECISIONS.md`.
- Exact-name and execution-pattern searches covered Go code, all tracked shell
  scripts, CI, documentation, durable state, and every other tracked file.
  References outside durable historical prose were confined to relationships
  among the retired wrappers and three uncalled RC-specific checker scripts.
  Removed those checkers with their launcher dependencies:
  `scripts/check-ext20-rc5-live-direct.sh`,
  `scripts/check-ext20-rc6-live-direct.sh`, and
  `scripts/check-ext20-rc7-live-direct.sh`. No executable or non-historical
  tracked reference to any of the 70 removed files remains.
- Durable policy now says reusable operational behavior belongs under
  `scripts/`; top-level entrypoints remain only when genuinely reusable or
  currently required; retired wrappers are preserved by Git history rather
  than `HEAD`; and future validation may not depend on retired wrappers being
  present or absent. This supersedes conflicting wrapper-retention directives
  while retaining their historical execution and evidence facts as prose.
- Reframed EXT-20 without another RC-numbered wrapper lineage. Its next pass
  implements one tracked, reusable, parameterized Level-1 candidate build-and-
  verification workflow under `scripts/` without building a candidate or
  calling live Codex. A later pass uses it once for one fresh candidate, after
  which the existing quantitative Level-1 dogfood gate runs against that exact
  candidate.
- Files changed: `.agent/DECISIONS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/TASKS.md`; 67 retired top-level wrappers deleted; and the three
  retired RC-specific checker scripts deleted. Nothing under `.revolvr/`, no
  Go source, no dependency, no product script, and no product behavior changed.
- Verification: the recorded retired inventory exactly matched all 67 deleted
  top-level wrappers; a per-removed-file search found no dangling executable
  or non-historical tracked reference across all 70 deletions; `bash -n`
  passed for all 9 remaining tracked shell scripts; and `git diff --check`
  passed. Go tests were intentionally not run because this cleanup did not
  affect executable product behavior.
- Result: **PASS**. No uncertainties or blockers remain. No candidate build,
  live Codex dogfood, commit, push, tag, or publication occurred. The exact
  next fresh-pass command is `./agent-one.sh`; its sole task is the reusable
  workflow implementation described above.

## Prospective RC.13 V7 Builder Design Rejected (2026-07-30)

- Task selected: the first unchecked task, `EXT-20`, narrowed to exactly one
  authorized prospective RC.13 v7 builder-design revalidation. `EXT-20`
  remains unchecked.
- Re-established clean local, fetched, and public `main` at
  `d9d32fd356ef1cafe47bd6635e107546d51b5ace`; pinned source commit/tree
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` /
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`; empty product-source diff;
  exact Go 1.22.12, Go 1.26.5, and govulncheck 1.1.4 identities; and absent
  local/remote RC.13 output, ref, tag, Actions-artifact, and release-asset
  collisions.
- Created exactly one persistent ignored root,
  `.revolvr/prospective-builder-validation-v7.hYcT7T`. The independently
  authored 29,474-byte, 626-line design SHA-256 is
  `42aecdbc849428c1ed0378b645705a14873426d64d11ed3bbc9d627aaf631635`.
  Its unexecuted full mode covers exact sealed-design/builder equality,
  launcher tracked-blob and dynamic-self authority, dynamic current published
  controller authority, permitted tracked history, actual collision classes,
  two shallow fetches, isolated dual-Go test/race/vet/module matrices,
  ordinary/verbose vulnerability evidence, reproducible supported builds,
  embedded metadata/empty build IDs, payload manifests, root-inclusive
  sealing/publication comparison, and terminal post-final retention. No full
  mode or product command ran.
- The initial sequence-one preservation bootstrap failed before a publication
  probe: `validate-sequence.sh` line 84 expanded local `phase` in the same
  declaration under `set -u`. Retained the exact bootstrap evidence, used the
  sole permitted repair to split the declaration, recorded
  `REPAIR-RECORD.tsv`, and hash-locked the repaired accepted inputs. No repair
  remains.
- Repaired sequence one passed five gates: accepted-byte preservation,
  v5/v6/RC.12 preservation, current controller/source/tool authority, Bash
  syntax, and exit/early-return/signal cleanup lifetime. Its success
  publication probe sealed both stage roots and descendants, created and
  copied the candidate final from the sealed stage, reapplied mode `0500`, and
  produced equal root-inclusive type/mode/link/size/hash inventories.
- After the neutral candidate final appeared, the probe stopped status `1` at
  design line 190 because `assert_distinct_file_inodes` expanded local `rel`
  in the same declaration under `set -u`. The terminal report correctly
  retained candidate `yes`, verification `no`, cleanup
  `forbidden-after-final-appearance`, and all build/stage/final/inventory/log
  evidence under non-RC.13 probe names. No cleanup was attempted.
- Because the repair allowance was consumed, no second repair or sequence two
  was attempted. The required two complete passing sequences do not exist.
  The root remains unsealed mode `0700`: 32 regular files, 65,387 bytes,
  content stream
  `73327470c97c4d9be73d7c2ca6fca6734b4d50ecc42ef55f651611d910194a91`,
  and root-inclusive inventory
  `5f8302a35b6d042212a904acccb9bd815b5e113596ac72d5a9b7d492b67b083b`.
  `FAILURE-SUMMARY.tsv` is the canonical failure record. No seal authority,
  manifest, or review launcher was created.
- Exact post-failure preservation passed: v5 snapshot
  `f01a28b55fb4f5a3a556af511ca610459c91bd9c0c864ec636a310adf2856de7`,
  v6 snapshot
  `2655f4122ce3a3a6e1a37470e657b678ca26b5129a183e8ec5bf49d7d0667b0f`,
  and RC.12 history snapshot
  `78bd6732dafe042647af2d93b70d7ea9e4892554302c4af45c2d4b75c96d2d5d`.
  Repaired accepted-input hashes and canonical one-final-LF history also
  remained exact.
- Files changed: the sole ignored rejected v7 root plus `.agent/TASKS.md`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`. No product
  source, dependency, historical evidence, builder, candidate, remote state,
  or review launcher changed.
- Verification commands: clean exact-controller/source/tool and collision
  reads; fresh v5/v6/RC.12 snapshots; `bash -n` for all prospective scripts;
  accepted-input `sha256sum -c`; the stopped bootstrap; the sole repair; the
  repaired sequence-one run through five passed gates and retained terminal
  publication failure; post-failure accepted/history preservation; local and
  remote forbidden-identity reads; and `git diff --check`. Product tests,
  builds, full mode, construction, and sequence two were forbidden or
  ineligible and did not run.
- Result: **FAIL / BLOCKED**. V7 is immutable unsealed rejected evidence.
  What remains is a new explicit operator decision; there is no authorized
  review, builder-publication, construction, or fresh-design gate.

## RC.13 V6 Review Rejected; Fresh V7 Revalidation Prepared (2026-07-30)

- Task selected: verify the completed independent read-only v6 review, decide
  whether the sealed design may proceed to builder publication, and prepare
  only the next safe gate.
- The operator completed the published review. Git and both v5/v6 roots stayed
  unchanged; the review preflight passed again; and no process, RC.13 builder,
  construction launcher, candidate, runtime/output path, ref, tag, workflow,
  artifact, release asset, suite, or external-use state appeared.
- Independent static inspection rejects v6 for publication. In exact design
  lines 209-217, full-role admission requires the already tracked v6 review
  launcher and future builder-publication launcher to be absent. Full mode
  would therefore fail deterministically before its first build root.
- Two additional authority defects reinforce rejection: full mode verifies the
  prospective builder only by path/mode/owner/link without comparing its bytes
  to the sealed design, and `seal_and_inventory` seals only descendants while
  stage/final root modes are omitted from inventory and comparison. Current
  controller identity after the validated commit and explicit post-final
  evidence retention are also not bound strongly enough for publication.
- V6 remains authentic sealed neutral evidence at 13 files, 45,820 bytes,
  manifest
  `ecdd6f9f5a589038754d1bdb8326d5e19a1ea660eb0bb53a17029fa2aa7734be`,
  stream `1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f`,
  and inventory
  `0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41`.
  Passing neutral sequences do not override the full-design review defects.
  V6 must not be repaired, rerun, copied, derived from, or used as builder
  input.
- Added inert `agent-ext20-rc13-builder-revalidation-v7.sh`, mode `0755`,
  10,953 bytes, 159 lines, SHA-256
  `d435626a07cadd9abbf77550b46315782cb5410d26d315d6547bcbde2b41e11b`.
  It preserves v5/v6, rejects all RC.13 collisions, and may start only one
  fresh independently authored persistent v7 neutral-validation pass.
- V7 must distinguish required builder/construction roles, permitted tracked
  validation/publication history, and forbidden outputs; bind exact builder
  bytes to sealed evidence and current published controller authority; include
  root directories in sealing/inventory/copy equality; and retain all evidence
  after any final path appears. Neutral full-role and post-first-final probes
  plus two complete sequences are required.
- Raw Git committed and pushed the five-file v6 rejection and v7 authorization
  as `4b90b5b511168034a890468a3336b71806c87300` (`Reject RC.13 v6 builder
  design`). Local, fetched, and public `main` matched exactly. The clean
  published v7 `--preflight-only` path exited `0` without creating a design,
  builder, or candidate.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and the new inert v7 launcher. No product source,
  dependency, ignored evidence, builder, candidate, or remote state changed.
- Verification: clean raw Git and exact local/fetched/public `main`; published
  v6 review preflight; v5/v6 manifest/stream/inventory preservation; source and
  remote collision reads; design and launcher `bash -n`; focused static
  full-role/self/seal/publication review; launcher control-flow inspection;
  and `git diff --check`. Product tests/builds and all design modes were not
  run.
- Result: **V6 REJECTED for builder publication; PASS for fresh v7 neutral
  revalidation authorization only**. Exact next command is
  `./agent-ext20-rc13-builder-revalidation-v7.sh`. RC.13 remains unconsumed and
  `EXT-20` remains unchecked.

## Prospective RC.13 V6 Builder Design Accepted For Read-Only Review (2026-07-30)

- Task selected: the first unchecked task, `EXT-20`, narrowed to exactly one
  explicitly authorized fresh prospective RC.13 v6 builder-design
  revalidation. `EXT-20` remains unchecked.
- Re-established exact source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, empty product-source diff,
  exact Go 1.22.12/1.26.5 and govulncheck 1.1.4 executable identities,
  surviving construction and controller history, all recorded terminal
  lost-root absences, RC.12 terminal builder/launcher/failure-review evidence,
  exact rejected-v5 aggregate identity, and all local/remote RC.13 collision
  absences.
- Created exactly one fresh ignored root,
  `.revolvr/prospective-builder-validation-v6.bHfL29`. The independently
  authored 22,459-byte, 541-line design SHA-256 is
  `e457a4f8566f24fe5cd824cc8dc186a96019470838b94b9794069037cd03b8ff`.
  Its unexecuted full design specifies two non-local shallow exact-source
  fetches; isolated exact Go environments, caches, and module caches; ordinary,
  race, vet, and module matrices under both Go versions; ordinary and verbose
  vulnerability evidence; two reproducible Linux/Darwin/FreeBSD amd64 build
  sets; embedded source/target/CGO metadata and empty build IDs; manifests;
  post-seal inventories; and terminal `mkdir` plus `cp -a source/.
  destination/.` publication with complete stage/final and distinct-inode
  comparisons.
- Before sequence one, static audit proved all design and validation traps use
  stable global identities. Successful publication, induced-failure status 73,
  status-propagation success, and early-return status 19 cleanup variants all
  passed with no residue. The canonical history baseline was written directly,
  independently remeasured, and proved exact at 1,662 bytes, 18 lines, SHA-256
  `09ee1691d91f1e1e63b83f63e0e3819c7db034c330253e193f9ec8e7797c1dd2`,
  final byte `0a`, penultimate byte `30`, with no added blank line.
- Both complete validation sequences reached and passed all 12 gates: syntax,
  all cleanup variants, full-context role/collision audit, focused static
  audit, exact status-64 self refusal, forbidden identity/residue audit,
  canonical history plus EOF preservation, no-collision status propagation,
  rejected-v5 preservation, and accepted-byte preservation. No repair was
  used. Sequence hashes are
  `d3543310e5308117bb18df9533ca9ac6071722326c02f00016fe085d6784ee37`
  and
  `d8d01c54f0b2f9c8e8b853254e98fdba723537d2145a1d999ccc0a2e182ec9bc`.
- V5 remained exact before and after each sequence at 11 files, 44,298 bytes,
  content stream
  `6931b60c434205e2ce3130c119aa82750c117f8c947dd7c39f62b5011ddcb7e0`,
  and inventory
  `3f7403726cf59e3d02533deeb0c0f975e773adc0423e1f9a470eb30e5cf88cb5`.
  No earlier builder, launcher, draft mode, script, or output was executed,
  edited, copied, transformed, derived from, removed, sealed, or reused.
- Sealed the sole v6 root mode `0500`: 13 mode-`0444`, single-link regular
  files, 45,820 bytes; 12-entry manifest SHA-256
  `ecdd6f9f5a589038754d1bdb8326d5e19a1ea660eb0bb53a17029fa2aa7734be`;
  root stream
  `1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f`;
  inventory
  `0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41`.
- Added inert `agent-ext20-rc13-builder-revalidation-v6-review.sh`, mode
  `0755`, 9,215 bytes, 146 lines, SHA-256
  `c0c187322fb4597b62666332ff8595296f81c59b8fb853a0ab24dc799a06f5e2`.
  It was syntax/static checked but not executed, staged, committed, pushed, or
  published during the validation pass. It cannot execute a retained design
  mode and may later launch only a read-only independent review.
- Independent controller inspection replayed all 12 manifest entries, both
  passing sequence tuples, canonical EOF identity, sealed root stream and
  inventory, rejected-v5 preservation, source/product and remote collision
  boundaries, and review-launcher control flow. Raw Git committed and pushed
  the five-file record as `bb68b8016646e571bff1711f66dd81ff5ede5e7d`
  (`Record accepted RC.13 v6 validation`). Local, fetched, and public `main`
  matched. Its clean `--preflight-only` path exited `0` without executing a
  design mode.
- Files changed: the sole ignored sealed v6 root, the inert review launcher,
  `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md`. No product source, dependency, builder, candidate,
  construction/output path, ref, tag, workflow, artifact, suite, dogfood,
  release, or external-use state changed.
- Verification commands: exact source/tool/history/v5/collision reads;
  `bash -n` on every prospective script; focused static audits; pre-sequence
  cleanup/status/context probes; direct and independent canonical-history
  measurements; both complete validation sequences; terminal accepted-byte,
  v5, forbidden-identity, and residue checks; manifest self-verification;
  sealed-root stream/inventory verification; review-launcher syntax/static
  checks; and `git diff --check`. Product tests/builds and full construction
  were expressly forbidden and did not run.
- Result: **PASS for later independent read-only review only**. Exact next
  command is `./agent-ext20-rc13-builder-revalidation-v6-review.sh`. No blocker
  exists within this task. No builder/construction/candidate authority exists.

## Prospective RC.13 Builder Validation Rejected (2026-07-30)

- Task selected: the first unchecked backlog item, `EXT-20`, narrowed to the
  explicitly authorized one-pass prospective RC.13 construction-design
  validation. `EXT-20` remains unchecked.
- Created exactly one ignored design/evidence root,
  `.revolvr/prospective-builder-validation-v5.tL50Wc`. The independently
  authored draft describes two shallow source fetches, isolated exact Go
  environments and both verification matrices, vet/module/vulnerability
  evidence, reproducible Linux/Darwin/FreeBSD amd64 builds, embedded metadata,
  empty build IDs, post-seal inventories, manifests, and terminal `mkdir` plus
  `cp -a source/. destination/.` publication with distinct-inode and complete
  comparisons. Full mode and all product tests/builds remained unexecuted.
- Re-established source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, empty product-source diff,
  exact Go 1.26.5/1.22.12 and govulncheck 1.1.4 identities, aggregate identities
  for every surviving release-candidate and tracked EXT-20 history entry,
  RC.12 builder/launcher/failure-review/manifest/stream identities, all named
  terminal lost-root absences, and complete local/remote RC.12/RC.13 output
  absences.
- Sequence one failed in semantic cleanup after syntax passed: an EXIT trap
  referenced function-local variables after return, leaving exact neutral root
  `/tmp/revolvr-rc13-neutral.GqSd8U`. The sole permitted repair verified and
  removed that owned non-symlink root, bound publication and status-regression
  cleanup to stable probe identities, and made no historical or construction
  change.
- The post-repair sequence passed Bash syntax, successful and induced-failure
  semantic publication/cleanup, full-context role/collision audit, focused
  static audit, expected status-64 exact-self refusal, and forbidden identity/
  residue audit. It failed available-history preservation because
  `available-history-baseline.tsv` has one extra final newline: fresh output
  ended at byte 863 line 10 while the baseline continued to byte 864 line 11.
  The exact stranded comparison file was safely removed. Per the one-repair
  rule, no further repair or sequence was attempted.
- Files changed: ignored root
  `.revolvr/prospective-builder-validation-v5.tL50Wc` plus tracked durable
  `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md`. No Go source, dependency, historical builder,
  candidate, workflow, ref, tag, suite, or release output changed. No review
  launcher was created.
- Verification commands: published
  `./agent-ext20-rc13-builder-validation.sh --preflight-only`; exact source,
  tool, history, RC.12 terminal, remote-collision, and absence reads;
  `bash -n` for the prospective scripts; focused static audit; validation
  sequence one; the sole neutral repair and cleanup audit; validation sequence
  two; final forbidden-path and `/tmp` residue audit; and `git diff --check`.
  Product tests/builds were expressly forbidden and were not run.
- Result: **FAIL / BLOCKED**. Both complete sequences did not pass, so the v5
  root is unsealed rejected evidence with no self-verifying final manifest.
  At the end of that v5 pass, no
  `agent-ext20-rc13-builder-validation-review.sh` or next gate existed.
- Independent controller verification reproduced both failures without running
  a draft mode: all scripts pass `bash -n`; the history baseline is 864 bytes,
  11 lines, and ends in two newline bytes; its first 10 lines exactly equal the
  canonical authority prefix; and no neutral/history/construction residue
  remains. Rejected root identity is mode `0700`, 11 mode-`0600` regular files,
  44,298 bytes, content stream
  `6931b60c434205e2ce3130c119aa82750c117f8c947dd7c39f62b5011ddcb7e0`,
  and complete inventory
  `3f7403726cf59e3d02533deeb0c0f975e773adc0423e1f9a470eb30e5cf88cb5`.
- Added inert `agent-ext20-rc13-builder-revalidation-v6.sh`, mode `0755`,
  11,954 bytes, 182 lines, SHA-256
  `9eec038f65be8c9f22d7000854f1eea654e3d56a0166f8f6a5b34d4b9efd429d`.
  It freezes v5, requires clean published controller/history and complete
  RC.13 collision absence, then may start only one independently authored v6
  neutral-validation pass. Its two sequences must completely reach explicit
  trap-lifetime, canonical-EOF, status-propagation, v5-preservation, and
  accepted-byte gates.
- Raw Git committed and pushed the five-file rejected-v5 record and v6
  authorization as `92eb3d85cad3e78f4e980da1031cca485c8ae8da` (`Record
  rejected RC.13 v5 validation`). Local, fetched, and public `main` matched.
  The clean published v6 `--preflight-only` path exited `0` without creating a
  design, builder, or candidate.
- Updated result: **v5 remains FAIL / BLOCKED; PASS for a separately authorized
  v6 neutral revalidation gate only**. Exact next command is
  `./agent-ext20-rc13-builder-revalidation-v6.sh`. Do not repair, rerun, seal,
  derive from, or reuse v5. RC.12 stays terminal and `EXT-20` stays incomplete.

## RC.12 Failure Review Accepted And RC.13 Prospective Gate Prepared (2026-07-30)

- Task selected: verify the completed read-only RC.12 construction-failure
  review, accept or reject its immutable conclusion, and prepare only the next
  non-construction gate.
- The operator completed the published no-argument failure-review pass. Git
  remained clean at exact local/fetched/public `main`; no process or RC.12
  preflight/build/stage/diagnostic/final/runtime output appeared. Independent
  controller replay of the published preflight passed without executing the
  builder or construction launcher.
- Exact RC.12 builder, sealed draft, construction launcher, evidence manifest,
  and content stream remain unchanged. Complete local/remote ref, tag,
  workflow, Actions-artifact, release-asset, and construction-path absences
  passed. The neutral Bash reproduction again proves that exact builder lines
  496-499 return status `1` when release assets are correctly absent, causing
  the terminal pre-root failure through `set -e`. The review is accepted.
- Added inert `agent-ext20-rc13-builder-validation.sh`, mode `0755`, 9,701
  bytes, 160 lines, SHA-256
  `052e97aa653f57bb380ccc130c1f1aa0181f8517f7e65332ab80727a6fcecb2c`.
  It requires clean published controller/self identity, replays the exact RC.12
  terminal preflight, and rejects every local/remote RC.13 collision before
  starting one fresh Codex design-validation pass.
- The prospective pass may author only a fresh design and sealed evidence in
  one unique persistent ignored `.revolvr/prospective-builder-validation-v5.*`
  root. It cannot reuse historical bytes, run product tests/builds or full
  construction, or create any RC.13 builder/candidate/remote/suite/release
  identity. Two complete neutral sequences and an explicit no-collision
  status-propagation regression are mandatory; at most one repair is permitted
  only between sequences. Success may add only one inert later review launcher
  and durable state.
- Raw Git committed and pushed the five-file authorization as
  `9e9e8740686efd17991da38b17fbda1d5eaaff0d` (`Authorize prospective RC.13
  builder validation`). Local, fetched, and public `main` matched exactly. The
  published launcher's `--preflight-only` path exited `0`, replayed the RC.12
  terminal preflight and all RC.13 collision guards, and created nothing.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and the new inert RC.13 validation launcher. No Go
  source, dependency, ignored runtime evidence, builder, or candidate changed.
- Verification: clean raw-Git local/fetched/public identity; published RC.12
  failure-review preflight; process and complete RC.12 residue inventory;
  launcher `bash -n`; exact mode/size/line/hash reads; focused control-flow
  review; and `git diff --check`. Product tests/builds were not run because no
  product code changed and this gate expressly forbids construction.
- Result: **PASS for prospective RC.13 builder-design authorization only**.
  Exact next command is
  `./agent-ext20-rc13-builder-validation.sh`. RC.12 remains terminal and
  `EXT-20` remains unchecked.

## RC.12 Terminal Pre-Root Construction Failure (2026-07-30)

- Task selected: verify the operator-reported one-shot RC.12 construction
  failure, determine its exact cause without rerunning the builder, preserve
  the terminal boundary, and prepare only a later read-only failure review.
- The operator executed published `./agent-ext20-rc12.sh` with no arguments.
  The launcher reached its exact builder, which printed
  `prospective construction failed before final path appearance`. This one
  execution consumed RC.12 under the published no-retry/no-repair rule.
- No RC.12 preflight, build, stage, diagnostic, candidate, verification,
  runtime, local/remote ref or tag, workflow, Actions artifact, or release
  asset exists. No RC.12 process remains. Exact builder, construction launcher,
  sealed draft, manifest, persistent evidence, historical source, and product
  boundary remain unchanged.
- Root cause is exact builder lines 496-499. With a release asset absent, the
  final `grep ... && fail` AND-list returns status `1`. The enclosing loop is
  the final command of `verify_remote_collisions`, so that function propagates
  status `1`; `set -e` reaches the generic terminal trap before the first
  construction `mktemp`. A neutral Bash reproduction returned status `1`, and
  the candidate and verification release assets were independently absent.
  Filesystem space, inodes, owner permissions, ACLs, and parent mode were
  healthy. This is a deterministic builder control-flow defect, not product,
  toolchain, test, build, storage, artifact, manifest, or publication evidence.
- Added inert `agent-ext20-rc12-construction-failure-review.sh`, mode `0755`,
  8,862 bytes, 182 lines, SHA-256
  `43cdfee5154ed70e689f4db7cc9df589f1b3bc6f56cd53a0ac6cd16c78148cd9`.
  Its preflight verifies immutable identities, complete output absence, remote
  collisions, and the status mechanism without executing the builder or
  construction launcher. Its no-argument path may start only one fresh
  read-only independent review and cannot create continuation.
- Raw Git committed and pushed the five-file terminal failure record as
  `cfee541546da35ea60eac102996691f144279e4f` (`Record terminal RC.12
  construction failure`). Local, fetched, and public `main` matched. The clean
  published review launcher's `--preflight-only` path exited `0` without
  executing the builder or construction launcher.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and the new inert review launcher. No Go source,
  dependency, ignored builder, sealed evidence, or runtime output changed.
- Verification: raw Git status/history/identity reads; complete RC.12 root,
  final-path, process, ref, tag, workflow, Actions-artifact, and release-asset
  absence reads; exact builder/draft/launcher hashes, modes, sizes, lines,
  syntax, and persistent evidence identities; filesystem capacity/inode/ACL
  inspection; neutral Bash status reproduction; launcher `bash -n`; and
  `git diff --check`. Product tests/builds were not run because no product code
  changed and RC.12 construction may not be retried.
- Result: **BLOCKED; RC.12 terminally failed before construction-root
  creation**. `EXT-20` remains unchecked. The exact next command is
  `./agent-ext20-rc12-construction-failure-review.sh`; it grants read-only
  review only.

## RC.12 Builder Publication Accepted And Construction Handoff (2026-07-30)

- Independent controller review accepted the exact ignored builder, hardened
  release parent, tracked construction launcher, and durable record. It
  replayed sealed evidence, byte/mode/hash/line/link/inode identities,
  temporary/final launcher equality, syntax and source control flow, authority
  exports, source/controller history, and complete local/remote refs, tags,
  artifacts, release assets, workflow, runtime, and final-output absences. No
  builder or construction path was executed.
- Raw Git committed and pushed the six-file tracked record as
  `b09a1c5d9973f39f2447711a58e03cacf8edf642` (`Publish RC.12 builder and
  construction launcher`). The ignored mode-`0555` builder and mode-`0755`
  parent remained unchanged. Local, fetched, and public `main` matched exactly.
- The clean published `./agent-ext20-rc12.sh --preflight-only` command exited
  `0` with launcher SHA-256
  `f2a5f95323cf95334aed2c79c08368d63d0a73646600155a0032e6027bec6572`.
  It did not execute the builder; candidate and verification final paths remain
  absent.
- Exact next command is `./agent-ext20-rc12.sh`. The operator's execution of
  that no-argument command grants one local construction attempt only. A
  failure is terminal and cannot be retried or repaired. No remote, suite,
  live-model, release, external-use, or `EXT-20` authority follows.

## RC.12 Builder And Construction Launcher Publication Prepared (2026-07-30)

- Task selected: exactly one bounded EXT-20 RC.12 builder-publication pass.
  The pass published only the reviewed builder bytes, one inert construction
  launcher at its tracked target, and one inert later publication-review
  launcher. It did not use `gh`, commit, push, stage, execute either published
  identity, run a draft mode, product test/build, full construction, candidate,
  remote workflow, suite, Revolvr/model operation, release, external-use
  action, or `EXT-20` completion. RC.12 remains unconsumed and `EXT-20`
  remains unchecked.
- Before either publication path appeared, construction-launcher bytes were
  authored only at unique anonymous temporary path
  `/tmp/revolvr-construction-launcher.zsWPHa/construction-launcher.sh`.
  `bash -n` and focused source review passed, then the 14,242-byte, 284-line
  source was sealed mode `0444` at SHA-256
  `f2a5f95323cf95334aed2c79c08368d63d0a73646600155a0032e6027bec6572`.
  Its explicit `--preflight-only` path returns before its sole builder
  execution site. It accepts no other arguments, dynamically hashes its final
  tracked bytes, exports all four required builder/launcher/manifest/stream
  identities, and then can exec only the exact builder with no argument.
- Protected parent
  `/home/gernsback/source/revolvr/.revolvr/release-candidates` retained its
  exact real path and UID `1000`; mode was tightened once from `0775` to
  `0755` before builder publication and remains `0755` without group/other
  write permission.
- The reviewed draft was copied once to sole ignored builder
  `/home/gernsback/source/revolvr/.revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh`.
  Published identity is mode `0555`, UID `1000`, link count 1, 38,528 bytes,
  756 lines, SHA-256
  `dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e`.
  It is byte-identical to, but a distinct inode from, the mode-`0444` sealed
  draft; syntax passed. It was not executed and is immutable after appearance.
- The already validated temporary bytes were copied once to inert tracked
  publication target `/home/gernsback/source/revolvr/agent-ext20-rc12.sh`.
  Published identity is mode `0755`, UID `1000`, link count 1, 14,242 bytes,
  284 lines, SHA-256
  `f2a5f95323cf95334aed2c79c08368d63d0a73646600155a0032e6027bec6572`.
  It is byte-identical to, but a distinct inode from, the sealed temporary
  source; syntax passed. It was not staged, committed, or executed.
- Sealed persistent root
  `/home/gernsback/source/revolvr/.revolvr/prospective-builder-revalidation-v4.5pWwTx`
  remained exact at mode `0500`, 10 mode-`0444` files, 53,626 bytes, manifest
  SHA-256 `f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2`,
  and content stream
  `22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8`.
  Exact source commit/tree and empty product-source diff passed. Local,
  fetched, and public `main` were exact at
  `ca702ef2931a006843c10b3b899db2b5ca0689dd`; all six persistent controller
  anchors, nine historical controller file hashes, and complete RC.12 local/
  remote output, ref, tag, workflow, runtime, Actions-artifact, and release-
  asset collision absences passed before publication.
- Only after both exact publications passed, added inert later review launcher
  `agent-ext20-rc12-builder-publication-review.sh`, mode `0755`, 8,431 bytes,
  148 lines, SHA-256
  `3360511b57aeee4258e2d5530f5a4067202e9160eb524b58b735a2d3a4a70966`.
  Syntax and static no-builder/no-construction-execution review passed. It has
  not been run and can return only a later independent read-only accept/reject
  report; it cannot create continuation authority.
- Files changed: ignored exact builder above; new inert launchers
  `agent-ext20-rc12.sh` and
  `agent-ext20-rc12-builder-publication-review.sh`; `.agent/TASKS.md`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`; plus the
  protected parent's safer ignored-directory mode. No Go source or dependency
  changed.
- Verification commands: raw `git fetch`, local/fetched/public-main and
  ancestry/tree comparisons, empty product-source `git diff`, sealed-root
  manifest/mode/count/size/hash/stream checks, draft and all launcher/builder
  `bash -n`, focused construction-launcher source review, raw-Git local/remote
  ref/tag checks, public Actions/release collision reads, one-shot copy byte/
  hash/mode/size/line/link/inode comparisons, review-launcher static review,
  and final `git diff --check`. Product tests/builds were expressly forbidden
  and were not run.
- Result: **PASS for builder-publication preparation only**. Blockers: none.
  What remains is one fresh independent read-only run of
  `agent-ext20-rc12-builder-publication-review.sh`, followed by separate
  controller publication if accepted. Builder execution and construction
  require new explicit authority.

## RC.12 Builder-Publication Launcher Published (2026-07-30)

- Raw Git committed and pushed the accepted fourth-design review record and
  inert builder-publication launcher as
  `2f21a4399a0a1bc00ceac345e0ebbeac9616d75a` (`Authorize RC.12 builder
  publication`). Local, fetched, and public `main` matched exactly.
- The clean published
  `./agent-ext20-rc12-builder-publication.sh --preflight-only` path exited `0`.
  It replayed all 12 sealed fourth-review diagnostics, source/public-main and
  collision guards, and reported that no identity was created. The exact
  builder, construction launcher, and publication-review launcher remain
  absent.
- Exact next command is `./agent-ext20-rc12-builder-publication.sh`. It may
  publish only the reviewed builder bytes and inert tracked construction
  launcher, then create one read-only publication-review launcher. It cannot
  execute construction. RC.12 remains unconsumed and `EXT-20` unchecked.

## Fourth Design Review And Builder-Publication Gate (2026-07-30)

- The operator completed the published fourth-design read-only review. The
  tracked repository and sealed persistent root remained unchanged. Independent
  controller verification replayed root/draft/manifest identities, both
  sequence records and the sole repair, surviving historical constants, lost
  roots, source/product/tool boundaries, cleanup, role/collision separation,
  the four corrected design concerns, and terminal post-seal publication; all
  passed without executing the draft.
- Review result: **ACCEPTED FOR BUILDER/CONSTRUCTION-LAUNCHER PUBLICATION
  ONLY**. No builder, construction launcher, candidate, product test/build,
  full mode, remote action, or continuation was created. RC.12 remains
  unconsumed and `EXT-20` unchecked.
- Added inert `agent-ext20-rc12-builder-publication.sh`, mode `0755`, 8,269
  bytes, 119 lines, SHA-256
  `180254489c4fe55b42681fe88726518b6b6acc6a83ae1d3593d8d462dccb16b7`.
  After clean publication it may start one fresh pass that prepares launcher
  bytes anonymously, publishes the sealed draft once as the exact mode-`0555`
  ignored builder, creates the mode-`0755` tracked construction launcher, and
  stops without executing either. It requires the release-candidate parent to
  become non-group/other-writable before builder publication.
- Files changed: this file, `.agent/HANDOFF.md`, `.agent/DECISIONS.md`,
  `.agent/TASKS.md`, and the new publication launcher. Verification includes
  shell syntax, launcher source review, `git diff --check`, raw local/remote
  identity and collision checks, sealed-root replay, and expected dirty-tree
  preflight refusal before publication. Exact next command after controller
  publication is `./agent-ext20-rc12-builder-publication.sh`.

## Fourth Persistent Validation Publication And Review Handoff (2026-07-30)

- Independent controller verification replayed the evidence manifest and
  content stream, draft identity/syntax, both sequence records, single repair,
  historical constants, terminal lost-root absences, source/product/tool
  boundary, role/collision corrections, and RC.12 local/remote absences. All
  passed. The only correction was backlog wording clarifying that the one
  repair occurred between sequence one and sequence two; evidence and design
  bytes were unchanged.
- Raw Git committed and pushed the exact five-file record as
  `bae8ff6b1e5d7e14a9002cd7fbba1ece101dc005` (`Record fourth prospective
  RC.12 neutral validation`). Local, fetched, and public `main` matched that
  commit exactly.
- On the clean published controller, all 12 named
  `agent-ext20-rc12-builder-revalidation-v4-review.sh --preflight-only`
  diagnostics passed. The draft was not executed and no product test/build,
  full mode, candidate identity, or continuation occurred.
- Exact next command is
  `./agent-ext20-rc12-builder-revalidation-v4-review.sh`. It authorizes only
  one fresh read-only accept/reject report and cannot create continuation or
  construction authority. RC.12 remains unconsumed and `EXT-20` unchecked.

## Fourth Persistent Prospective RC.12 Revalidation Passed (2026-07-30)

- Task selected: exactly one bounded EXT-20 pass to independently author and
  neutrally validate a fourth anonymous prospective builder design after the
  third design's volatile evidence disappeared. No `gh`, commit, push,
  builder, construction launcher, candidate, preflight/build/stage/diagnostic
  root, artifact, bundle, ref, workflow, tag, suite, launch record, product
  test/build, full mode, Revolvr/model operation, release, or external-use
  action occurred. RC.12 remains unconsumed and `EXT-20` remains unchecked.
- Created sole retained persistent ignored evidence root
  `/home/gernsback/source/revolvr/.revolvr/prospective-builder-revalidation-v4.5pWwTx`,
  sealed mode `0500`. It has exactly 10 mode-`0444` regular files totaling
  53,626 bytes, with no symlink or special entry. Draft
  `prospective-builder.sh` is 38,528 bytes, 756 lines, SHA-256
  `dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e`.
  Nine-entry, 902-byte `evidence-manifest.tsv` has SHA-256
  `f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2`;
  every entry's exact mode, size, and hash passed. Complete root content-stream
  SHA-256 is
  `22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8`.
- Before sequence one, read-only remeasurement matched all embedded surviving
  RC.6/RC.7 suite, launch, and terminal counts/streams; both terminal manifest
  pairs; RC.8/RC.9 builders and diagnostics; RC.9 preflight/stage/candidate/
  verification counts, streams, manifests, and artifact hashes; RC.10/RC.11
  builders; pinned tools; source commit/tree; empty product-source diff; and
  applicable local/raw-Git remote absence constants. The missing RC.8/RC.9
  build roots, RC.11 anonymous draft, and former validation roots
  `/tmp/revolvr-builder-validation.maYqgv`,
  `/tmp/revolvr-builder-revalidation.CSGs5E`, and
  `/tmp/revolvr-builder-revalidation-v3.PKfbRl` were explicitly recorded as
  terminal absences and were not recreated or reconstructed.
- Both complete validation sequences returned raw status tuple
  `0,0,0,0,64,0,0` for `bash -n`, `--neutral-admission`,
  `--neutral-full-context-audit`, focused static audit, expected no-argument
  exact-self refusal, forbidden-identity/residue audit, and available-history
  preservation. Sequence one passed; the one allowed neutral repair preserved
  executable build instructions/binaries, corrected tab-exact metadata
  matching, and added applicable RC.6/RC.7 absence and final copied-manifest
  checks. The accepted sequence two passed with repaired draft SHA-256
  unchanged throughout.
- Each semantic admission exercised successful `mkdir` plus `cp -a source/.
  destination/.` and induced status-`73` pre-copy failure. Both paths used two
  sealed nested levels containing mode-`0400` files. Cleanup accepted only its
  active exact `/tmp/revolvr-neutral-publication.XXXXXX` root, rejected unsafe
  entries, restored owner write permission to every directory depth-first,
  deleted only that tree depth-first, proved absence, and never hid cleanup
  status. Inventories, bytes, modes, single links, distinct inodes, no
  symlinks, and zero residue all passed.
- The unexecuted full-context design requires and hashes the exact read-only
  builder, separately tracked construction launcher, and this sealed root;
  permits tracked validation and recovery history; and rejects actual
  candidate/verification, post-candidate review, local/remote publication,
  construction, runtime, suite, and launch collisions. It captures inventories
  after final sealing, checks exact `.revolvr/ext20-rc12` plus dotted/hyphen
  descendants, preserves every applicable RC.6-RC.11 absence, and checks all
  three candidate/verification/attestation Actions artifact names.
- The unexecuted construction design retains two non-local shallow exact-
  source fetches at commit `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`
  and tree `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, excluding later controller
  objects/launchers; executable build instructions; source/controller/tool/
  environment identities; both Go ordinary/race/vet/module matrices; ordinary
  and verbose vulnerability results; reproducibility; exact target/CGO/VCS/
  build-ID metadata; and complete manifests/inventories/hashes. Final
  publication is terminal `mkdir`, appeared-state assignment only after
  success, `cp -a`, sealed-mode restoration, and complete post-seal stage/final
  equality, link, distinct-inode, and extra-entry checks, with no final-path
  cleanup or rename/link/symlink fallback after appearance.
- Added inert
  `agent-ext20-rc12-builder-revalidation-v4-review.sh`, mode `0755`, 8,372
  bytes, 167 lines, SHA-256
  `b98e2b84c93d65beb805b96cb2b6b1bc28e69de145b1d7b51fe8fb072ae33a33`.
  Its explicit named `--preflight-only` diagnostics passed without executing
  the draft or creating authority. It permits only a later read-only neutral
  report after controller publication and cannot create continuation.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, new tracked review launcher
  `agent-ext20-rc12-builder-revalidation-v4-review.sh`, and ignored sealed
  evidence root above. Verification commands: presequence syntax/static audit;
  complete read-only historical remeasurement and manifest checks; two full
  neutral sequences; final root manifest/mode/count/size/hash/stream checks;
  review-launcher `bash -n` and `--preflight-only`; and `git diff --check`.
  Product `go test ./...` was not run because the task expressly forbids
  product tests/builds and changes no Go source. Result: **PASS for independent
  review only**. No blocker exists in this gate. What remains is separately
  authorized controller publication and then the inert read-only review.

## RC.12 Persistent Recovery Review And Publication (2026-07-30)

- The operator completed the fresh recovery-review launcher. It left the exact
  intended six-file preparation unchanged. Independent controller verification
  repeated the tracked/untracked scope, shell syntax, executable modes and
  hashes, missing-root boundary, source/product boundary, local/fetched/public
  main identity, RC.12 runtime/ref/tag/workflow absences, and
  `git diff --check`; all passed.
- Raw Git committed and pushed the reviewed scope as
  `70aefe61ccdf6c6c6359558c483f6f1d9efac393` (`Prepare persistent RC.12
  validation recovery`). Local, fetched, and public `main` matched that exact
  commit after publication.
- The clean published command
  `./agent-ext20-rc12-builder-revalidation-v4.sh --preflight-only` exited `0`
  with `Fourth persistent neutral builder-revalidation preflight passed`. It
  started no Codex pass and created no evidence or RC.12 identity.
- Exact next command is
  `./agent-ext20-rc12-builder-revalidation-v4.sh`. It authorizes only one fresh
  fourth anonymous neutral-revalidation pass with retained evidence beneath
  persistent ignored `.revolvr/` state. Construction and `EXT-20` completion
  remain unauthorized.

## RC.12 Volatile Validation-Root Recovery Preparation (2026-07-30)

- Task selected: diagnose and prepare one fail-closed recovery for the missing
  third anonymous prospective-design evidence. The published review launcher
  fetched `main` and silently exited status `1` at line 28 because
  `/tmp/revolvr-builder-revalidation-v3.PKfbRl` is absent. Traced
  `--preflight-only` reproduced the exact stop before `codex exec`; no review,
  draft execution, product test/build, full mode, candidate identity, or
  continuation occurred.
- The third draft and evidence existed only in volatile `/tmp` and cannot be
  independently reviewed from durable summaries or hashes. Recovery must not
  recreate its path, derive its bytes from old Codex transcripts, or treat its
  summary as evidence. RC.12 remains unconsumed and `EXT-20` remains
  unchecked.
- Added `agent-ext20-rc12-builder-revalidation-v4.sh` to authorize, only after
  clean publication, one fresh independently authored fourth anonymous design.
  Its retained sealed evidence must live under a unique persistent ignored
  `.revolvr/prospective-builder-revalidation-v4.XXXXXX` root. The prompt
  preserves the two-sequence neutral gate, forbids product/full construction,
  records lost prior roots as absences, and requires corrections for all four
  controller concerns that the lost review was meant to decide.
- Added inert
  `agent-ext20-rc12-volatile-root-recovery-review.sh` for independent review of
  exactly this recovery preparation. Files changed are the two new launchers
  plus `.agent/HANDOFF.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/TASKS.md`. No Go source or dependency changed; no commit or push was
  authorized. The v4 launcher is mode `0755`, 10,543 bytes, 143 lines, SHA-256
  `5d9c82acbe9527e93421355a06843d60a2dd55c877dc2fb856c367fd02bc647c`;
  the recovery-review launcher is mode `0755`, 2,599 bytes, 26 lines, SHA-256
  `8def05def6c116b2b4645090a0661bd70146d52076710214b8be1084c3f771ea`.
- Verification for this preparation: launcher syntax, executable modes,
  tracked/untracked scope, `git diff --check`, missing-root reproduction,
  absence of RC.12 runtime/ref/tag/workflow identities, and expected dirty-tree
  refusal from the v4 `--preflight-only` mode. Exact next command is
  `./agent-ext20-rc12-volatile-root-recovery-review.sh`. That review cannot run
  the fourth revalidation or create construction authority.

## Controller Verification Before Daily Handoff (2026-07-26)

- Independently verified the third sealed root's mode, nine-file/53,181-byte
  inventory, all eight manifest entries, draft/manifest hashes, complete root
  stream, draft syntax, recorded two-sequence statuses, and zero probe/RC.12
  runtime residue. Historical streams/manifests and the source/product
  boundary remain unchanged. No draft mode, product test/build, or full mode
  was run.
- Source inspection confirms the corrected exact-root, all-directory,
  depth-first neutral cleanup. The neutral pass is authentic review evidence,
  not construction authority.
- Preliminary full-mode inspection found unresolved issues for independent
  review: a pre-seal verification inventory is compared with post-seal modes;
  the exact RC.12 runtime root is not covered by the dotted glob; retired
  runtime/ref/tag/glob absence coverage is incomplete; and remote artifact
  checks appear narrower than the durable contract. No builder or candidate
  continuation is authorized.
- Strengthened inert
  `agent-ext20-rc12-builder-revalidation-v3-review.sh` with an explicit
  `--preflight-only` mode and exact instructions to confirm or refute those
  concerns. Final identity: mode `0755`, 6,879 bytes, 79 lines, SHA-256
  `3bafb2b55cde7a872e5b159f3fc9e721d39942b208f83718875721d45dca888d`.
- Tomorrow's only command is
  `./agent-ext20-rc12-builder-revalidation-v3-review.sh`. It performs one
  read-only review and creates no continuation launcher. `EXT-20` remains
  unchecked and RC.12 remains unconsumed.
- Raw Git published the neutral record and strengthened review launcher as
  `aa47925f78f4fcaf44be9b9fa84403d57bed9fca`. Exact local/fetched/public
  `main` matched, and
  `./agent-ext20-rc12-builder-revalidation-v3-review.sh --preflight-only`
  passed without starting Codex. The session then stopped; the no-argument
  review command is reserved for the next fresh session.

## Third Prospective RC.12 Neutral Revalidation Passed (2026-07-26)

- Task selected: exactly one bounded pass to independently author and validate
  a third anonymous prospective RC.12 construction design. No `gh`, commit,
  push, RC.12 builder/candidate/construction launcher, product test/build,
  full mode, preflight/build/stage/diagnostic root, artifact, bundle, ref,
  workflow, tag, suite, launch record, Revolvr/model operation, release, or
  external-use action occurred. `EXT-20` remains unchecked.
- Before validation sequence one, every embedded historical count/hash
  constant was remeasured read-only and compared with durable state. Exact
  RC.6/RC.7 suite, launch, and terminal tuples matched `461/4/130` and their
  six recorded stream hashes. RC.8/RC.9 retained trees, RC.9 staged manifests,
  RC.10/RC.11 builders, both terminal checksum pairs, both prior sealed roots,
  pinned source commit/tree, and exact Go/govulncheck identities also passed.
  Authoring-time corrections to exact tool metadata, probe variable scope,
  explicit pre-cleanup status propagation, and cross-target version inspection
  were completed before sequence one and did not consume the neutral repair.
- Created unique sealed root
  `/tmp/revolvr-builder-revalidation-v3.PKfbRl`, mode `0500`, containing nine
  mode-`0444` regular files totaling 53,181 bytes. Draft
  `prospective-builder.sh` is 42,446 bytes, 789 lines, SHA-256
  `302738277108837a7282d9abe5650d34bee5896ec58ef13fefa85b8edf9345fc`.
  Eight-entry, 754-byte `evidence-manifest.tsv` has SHA-256
  `5018b79e47ea393eed9a3f0a3646f5d9ae20a65426993ed0bbe5c08a25b7e746`;
  every entry's mode, size, and hash passed. Complete root content-stream
  SHA-256 is
  `bf01b03207cbd2d3f056c31ee6c001f3efcff02a5fbace30cdeace228473bb77`.
- Both complete validation sequences independently returned statuses
  `0,0,0,0,64,0,0` for `bash -n`, `--neutral-admission`,
  `--neutral-full-context-audit`, focused static audit, expected no-argument
  exact-self refusal, forbidden-identity/residue scans, and history-
  preservation checks. Sequence one passed, so no repair was made before the
  mandatory passing sequence two.
- In each sequence the semantic probe wrote two files only beneath writable
  two-level parents, sealed files mode `0400` and all source/nested directories
  mode `0500`, separately created the destination, copied with `cp -a
  source/. destination/.`, and verified bytes, complete tree/file inventories,
  all modes, link count one, distinct inodes, and no symlinks. The single
  guarded cleanup accepted only its active exact unique root, proved no
  symlink, restored owner write permission to every directory depth-first,
  deleted that exact tree depth-first, and proved absence. An induced status-
  `73` pre-copy path with two sealed levels and two mode-`0400` files proved
  cleanup success without hiding cleanup status. No probe residue remains.
- The unexecuted full design separates required exact builder, tracked
  construction launcher, and sealed third validation root from permitted
  tracked validation-history launchers and forbidden output/collision
  namespaces. It snapshots all exact RC.6-RC.11 and prior validation history,
  verifies terminal/staged manifests and recorded absences, and requires the
  third root only as a full-mode immutable input after publication.
- The unexecuted construction path uses two non-local `git init` plus shallow
  exact-commit fetches, detached clean commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` and tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, exclusion of later controller
  objects/launchers, executable build instructions, exact source/controller/
  tool/environment evidence, both Go test/race/vet/module matrices, ordinary
  and verbose vulnerability evidence, reproducibility and exact target/CGO/
  VCS/build-ID metadata, and complete sorted manifests/inventories/hashes.
  Final publication uses `mkdir`, sets appeared state only after success,
  uses `cp -a`, restores sealed stage/root modes, and verifies stage/final
  manifests, inventories, counts, hashes, modes, links, distinct inodes, and
  absence of extra entries. It has no rename/link/symlink fallback and retains
  appeared paths terminally on failure.
- Files changed: `.agent/TASKS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and new inert review launcher
  `agent-ext20-rc12-builder-revalidation-v3-review.sh`; untracked sealed
  evidence root `/tmp/revolvr-builder-revalidation-v3.PKfbRl`. Product source,
  dependencies, all historical evidence, and every RC.12 output identity are
  unchanged/absent.
- Verification commands run: mandatory read-only constant remeasurement;
  terminal/staged manifest checks; two complete neutral sequences; final
  draft/root manifest, mode, size, count, line, stream, syntax, symlink, and
  residue checks; `bash -n` on the review launcher; and `git diff --check`.
  `go test ./...` was not run because this pass expressly forbids product
  tests/builds and changed no Go code. Result: **PASS for later independent
  review only**. There are no blockers in this bounded gate.
- Added inert review launcher
  `agent-ext20-rc12-builder-revalidation-v3-review.sh`, SHA-256
  `3bafb2b55cde7a872e5b159f3fc9e721d39942b208f83718875721d45dca888d`.
  Controller review strengthened it to mode `0755`, 6,879 bytes, 79 lines,
  then syntax-checked it without executing the review. Exact suggested next
  command after controller publication is
  `./agent-ext20-rc12-builder-revalidation-v3-review.sh`. Independent review
  cannot create construction authority; a separately published exact builder,
  construction launcher, and new explicit operator authorization remain
  required.

## Second Revalidation Review and Third Neutral Gate (2026-07-26)

- Independent controller review reverified sealed root
  `/tmp/revolvr-builder-revalidation.CSGs5E`: mode `0500`, 12 mode-`0444`
  files totaling 50,756 bytes, exact 42,999-byte/895-line draft SHA-256
  `ae560b6eebc0c2d77721df426477727c473774cc1661db9dc9ef2194fd120768`,
  exact 1,037-byte/11-line manifest SHA-256
  `0b962f2cfc2095c530d4414676e17c53a06d8ef65223baf7d9f26e6e958453ee`,
  and every manifest entry's mode, size, and content hash.
- Read-only source inspection confirmed `bash -n` and the terminal defect:
  cleanup changes only the probe, source, and destination root modes, while
  source/destination `nested` remain mode `0500`; unlinking their mode-`0400`
  files fails. The exact failed probe is absent. The full-context corrections
  are present but remain unexecuted and provide no construction authority.
- Independently reverified all RC.6/RC.7 suite, launch, and terminal-evidence
  counts/content-stream hashes; RC.8/RC.9 build/preflight/stage stream hashes;
  all six terminal/staged payload manifests; both terminal evidence bundles;
  RC.8-RC.11 and prior neutral-root file hashes; pinned Go/govulncheck hashes;
  source commit/tree; and the clean product diff. No product test/build or
  failed draft mode was run.
- Raw Git committed and pushed the terminal failed record as
  `3523fec538fade321b1d549adcc35e016889a166` (`Record failed prospective
  RC.12 revalidation`). Public `main` matched after push.
- Added mode-`0755` executable
  `agent-ext20-rc12-builder-revalidation-v3.sh`, 12,687 bytes, 178 lines,
  SHA-256
  `591a79098ced60f9aa0abbd11f66cf722b2adba22b1fd277237e0890aa536ee5`.
  It pins both sealed records, checks the clean published controller boundary,
  refuses every RC.12 output identity, and authorizes only one independently
  authored third anonymous draft with corrected all-directory depth-first
  cleanup and two complete neutral validation sequences.
- Exact next command: `./agent-ext20-rc12-builder-revalidation-v3.sh`.
  `EXT-20` remains unchecked and RC.12 remains unconsumed.

## Prospective RC.12 Neutral Builder Revalidation Failed (2026-07-26)

- Task selected: exactly one bounded independent authoring and validation pass
  for a corrected anonymous prospective RC.12 construction builder. No `gh`,
  commit, push, RC.12 builder/candidate/construction launcher, product test or
  build, full mode, preflight/build/stage/diagnostic root, artifact, bundle,
  ref, workflow, tag, suite, launch record, Revolvr/model operation, release,
  or external-use action occurred. `EXT-20` remains unchecked.
- Created unique root `/tmp/revolvr-builder-revalidation.CSGs5E`, now sealed
  mode `0500`. It contains 12 mode-`0444` regular files totaling 50,756 bytes.
  Draft `prospective-construction.sh` is 42,999 bytes, 895 lines, SHA-256
  `ae560b6eebc0c2d77721df426477727c473774cc1661db9dc9ef2194fd120768`.
  Self-verifying `evidence-manifest.tsv` is 1,037 bytes, 11 lines, SHA-256
  `0b962f2cfc2095c530d4414676e17c53a06d8ef65223baf7d9f26e6e958453ee`;
  every listed mode, size, and content hash passed before sealing.
- Independent full-context remediation is present in the unexecuted draft.
  Exact builder and separately published construction launcher are required
  full-mode inputs and never absence-list members. Both tracked validation-
  history launchers are allowed and verified. Exact final candidate and
  verification paths, post-candidate local-review launcher, local/remote refs
  and tags, workflow, release/asset/Actions-artifact namespaces, construction
  conflicts, and preflight/build/stage/diagnostic/suite/launch roots are the
  forbidden output set. `--neutral-full-context-audit` statically enforces
  these distinctions before self-identity enforcement.
- The unexecuted full design independently records before/after metadata and
  content identities for complete RC.6/RC.7 candidate, verification, suite,
  launch-record, and terminal-evidence history; RC.6/RC.7 terminal manifests;
  RC.8 builder/diagnostic/build/verification trees; RC.9 builder/diagnostic/
  preflight/build/stage trees and staged manifests; RC.10/RC.11 builders and
  the RC.11 neutral draft; both sealed neutral-validation roots; and every
  recorded RC.8-RC.11 absence. It compares the complete snapshots after work.
- Each prospective source clone uses `git init`, a non-local origin, and a
  `--depth=1` exact-commit fetch; requires detached source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, clean status, and exclusion of
  later controller commits/launchers from checkout and object database. The
  candidate/verification design includes executable instructions, exact
  source/tree/controller/product-diff and clean tool/environment identity,
  both Go versions' ordinary/race/vet/module results, ordinary and verbose
  vulnerability results, reproducibility hashes, exact GOOS/GOARCH/CGO/VCS/
  build-ID metadata, sorted complete inventories and hashes, and history
  preservation results.
- First validation sequence: `bash -n` status `0`; neutral admission status
  `1` on incorrectly entered RC.6/RC.7 stream file counts; full-context audit
  status `0`; focused static inspection completed; no-argument invocation
  status `64` at the expected self-identity refusal. The sole allowed neutral
  repair corrected counts to 461/4/130 for both histories and incorporated
  explicit glob checks, exact remote release/artifact collisions, stable
  directory comparisons, correct payload/manifest sealing order, and accurate
  terminal final-path state.
- Required second sequence: `bash -n` status `0`; neutral admission status `1`
  during publication-probe cleanup; full-context audit status `0`; focused
  static audit status `0`; no-argument invocation status `64` at the expected
  self-identity refusal. The semantic probe correctly wrote under writable
  parents, sealed file/nested/source, used `mkdir` and `cp -a`, and verified
  bytes, all modes, link counts, separate inodes, and no symlinks. Cleanup
  restored write permission only on the source/destination roots, not their
  sealed nested directories, so deletion of both mode-`0400` files was denied.
- Exact residue `/tmp/revolvr-neutral-publication.Jcoaht` was inspected and
  contained only the expected two roots, two nested directories, and two
  value files. Owner write permission was restored on exactly those parent
  directories; depth-first deletion removed only that probe and its absence
  passed. No other historical or candidate path was removed or changed.
- Files changed: `.agent/HANDOFF.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md`; untracked temporary sealed evidence root
  `/tmp/revolvr-builder-revalidation.CSGs5E`. `.agent/TASKS.md`, product
  source, dependencies, and all historical evidence remain unchanged. No
  revalidation-review launcher was created because the second verification
  failed after the one repair.
- Verification commands run: `bash -n` twice; both neutral modes twice; two
  focused static audits; expected no-argument identity refusal twice; exact
  probe-residue inspection/removal/absence; evidence-manifest mode/size/hash
  verification; final root/draft/manifest identity checks; and repository
  diff checks. `go test ./...` was not run because the pass expressly forbids
  product tests/builds and changed no Go code.
- Verification result: **FAILED after the one permitted repair**. Blocker is
  the prospective draft's incomplete nested-parent cleanup. The sealed draft
  cannot be edited, copied, executed, derived from, published, or used for
  construction. What remains is a separately authorized independent review of
  this exact failed record and a decision whether to permit a third anonymous
  neutral design. There is no active next command; RC.12 is unconsumed.

## Prospective RC.12 Neutral Builder Independent Review (2026-07-26)

- Controller review independently verified sealed root
  `/tmp/revolvr-builder-validation.maYqgv`: mode `0500`, 40 regular files,
  mode-`0444` 24,352-byte/575-line draft SHA-256
  `71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d`,
  and mode-`0444` 3,695-byte/38-line evidence-manifest SHA-256
  `2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85`.
  Every manifest entry's name/mode/size/hash passed; evidence totals
  37,123,439 bytes.
- Recorded syntax/admission statuses are all `0`, full-mode refusal is exact
  status `64`, both metadata and content before/after inventories are byte-
  identical, semantic probe output proves correct copy/mode/link/inode/cleanup
  behavior, and no prospective runtime identity exists. The review launcher
  passed syntax and pre-prompt sealed-evidence checks but was not executed.
- Raw Git committed and pushed the limited neutral-validation record plus
  inert review launcher as
  `7585c40f1a0048d3d7d29267403e0810ca6e352f` (`Record prospective RC.12
  builder validation`); public main readback matched.
- Independent code review rejects the draft for construction. Lines 564-568
  require the exact builder to exist before `run_full_mode`, while lines
  165-179 require the same `expected_builder` absent. That list also wrongly
  requires the necessary construction launcher and existing validation-review
  launcher absent, so full mode must fail before preflight.
- Additional blocking gaps: full mode preserves only RC.10/RC.11 rather than
  complete RC.6-RC.11 before/after history and terminal manifests; uses full
  clones containing later controller objects; and omits bundle build
  instructions, complete tool/environment identity, verbose unreachable-
  vulnerability evidence, remote artifact/release collision reads, and exact
  GOOS/GOARCH/CGO target metadata assertions. The first sealed draft cannot be
  modified, copied, derived from, or used for construction.
- Added executable `agent-ext20-rc12-builder-revalidation.sh`, SHA-256
  `9218d873e24214aa7fff9574e1e35ede1e967629947e101e961ad87eb285b4d7`.
  It authorizes only a second independent anonymous draft/revalidation root,
  with explicit neutral full-context audits for self/launcher admission and
  every settled candidate requirement above.
- Exact next command: `./agent-ext20-rc12-builder-revalidation.sh`. RC.12 is
  still unconsumed. No construction, remote, suite, live-model, release,
  external-use, recovery, queue, or `EXT-20` completion authority exists.

## EXT-20 Prospective RC.12 Neutral Builder Validation (2026-07-26)

- Task selected: exactly one bounded independent authoring and validation pass
  for an anonymous prospective Level-1 candidate-construction builder. No
  `gh`, commit, push, fetch, nested Codex/model operation, Revolvr operation,
  product test/build, candidate construction, candidate identity, preflight/
  build/stage root, artifact, bundle, ref, workflow, tag, suite, launch record,
  release, or external-use action was used. `EXT-20` remains unchecked.
- Created unique neutral root `/tmp/revolvr-builder-validation.maYqgv`, now
  sealed mode `0500`. Draft `candidate-construction-draft.sh` is mode `0444`,
  24,352 bytes, 575 lines, SHA-256
  `71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d`.
  The root path, draft filename, and draft bytes contain none of `rc12`,
  `rc.12`, or `level1-v0.1.0-rc.12`; prospective digits are joined only at
  runtime from separate values.
- The draft dispatches `--neutral-admission` after all function definitions
  and before self-identity enforcement. That mode ran only read-only source,
  history, tool, and collision admission plus fresh neutral publication
  probes. Exact published source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, public controller HEAD
  `ab525e26a8bdc837d6182a85ffcfda1bc7c5017f`, Go 1.22.12/1.26.5 executable
  identities and roots, and `govulncheck@v1.1.4` identity passed.
- The semantic probe correctly created source/nested writable, wrote the
  value, changed the file to mode `0400`, then sealed nested and source mode
  `0500`. It created the destination with `mkdir`, copied only with `cp -a
  source/. destination/.`, made the destination root `0500`, and proved byte
  equality; exact source/destination root `0500`, nested `0500`, and file
  `0400` modes; single link counts; distinct inodes; and no symlinks. It made
  destination/source parents writable before removing only the validated
  unique neutral probe and proved no residue.
- Initial commands `bash -n
  /tmp/revolvr-builder-validation.maYqgv/candidate-construction-draft.sh` and
  `bash /tmp/revolvr-builder-validation.maYqgv/candidate-construction-draft.sh
  --neutral-admission` both exited `0`; stdout, empty stderr, statuses, and
  commands are sealed. Focused review of all 57 command substitutions,
  manifest exclusions, the sole cleanup command/target, stage/final identity
  checks, failure traps, and three archival-copy lines found one repairable
  neutral design issue. One repair bound every Go metadata invocation to the
  clean environment, corrected the binary version probe to `--version`, made
  the second build cache explicit, and completed ERR/signal propagation.
- The full syntax/admission sequence was rerun after that sole repair and both
  commands again exited `0`. The focused audit passed with exactly one guarded
  `rm -rf` cleanup target, exactly three `cp -a` lines (one neutral probe and
  two full publications), no directory rename/hard-link/symlink command, no
  forbidden draft literal, and no probe residue. Final `bash -n` exited `0`.
- The draft was invoked exactly once without `--neutral-admission`. It wrote
  no stdout, exited required status `64`, and reported its neutral-path
  refusal against expected exact builder identity before the failure trap or
  full-mode function could run. All prospective runtime paths were absent
  afterward except the already existing validation-pass launcher.
- The unexecuted full-mode design pins the exact published source commit/tree;
  requires clean Go 1.22.12 and 1.26.5 invocation with independent source
  clones and caches; runs ordinary and race full test matrices under both Go
  versions plus vet, module, and vulnerability verification; builds twice and
  byte-compares Linux/Darwin/FreeBSD amd64 artifacts; verifies embedded source
  and empty build IDs; stages checksum manifests; seals trees; and publishes
  only with `mkdir` plus `cp -a`, followed by complete stage/final inventory,
  manifest, content, mode, and root-mode identity checks. Full mode was not
  executed.
- Historical before/after inventories are exact. The 57,135-entry metadata
  files are byte-identical at SHA-256
  `b4ea4ce169e5ec575c52d579aff6d868dbe8069245c556adb6a93ab301537360`;
  the 52,870-entry content files are byte-identical at SHA-256
  `15f89741ef58e4e6b7f1e590a6692bf60669faa91088aca1965228b04a8d6c59`.
  Coverage includes all `.revolvr` history, recorded RC.8/RC.9 temporary
  roots, the RC.11 neutral draft, all pre-existing EXT-20 launchers, and all
  Level-1 workflows. Neutral admission also reverified exact RC.10/RC.11
  builder/draft hashes, sizes, lines, modes, and the recorded absent paths.
  No historical source, dependency, builder, artifact, bundle, tree, suite,
  operation, record, diagnostic, evidence, identity, or absence changed.
- The sealed root has exactly 40 regular files: one draft and 39 evidence
  files. All are mode `0444`; evidence totals 37,123,439 bytes and the entire
  root totals 37,147,791 bytes. Mode-`0444` `evidence-manifest.tsv` is 3,695
  bytes, 38 lines, SHA-256
  `2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85`;
  its 38 entries reverify every other evidence file's exact mode, size, and
  SHA-256. Its own exact identity and the draft identity are recorded here.
- Files changed: new executable
  `agent-ext20-rc12-builder-validation-review.sh` plus `.agent/HANDOFF.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md`; no product source, dependency,
  or task-backlog byte changed. The review launcher is mode `0755`, 4,583
  bytes, 104 lines, SHA-256
  `0cdbde37d6c33404c988b68c2da28fd325c50c277333ba35983bad419a235fbb`.
  It passed `bash -n` and was not executed. Final prospective-name scanning
  finds exactly the pre-existing validation launcher and this authorized
  review launcher; the construction launcher and every candidate runtime
  identity remain absent.
- Verification result: **PASSED neutral validation**. `go test ./...` was not
  run because no Go source changed and this pass expressly prohibited product
  tests/builds. Final `bash -n` checks for the draft and review launcher,
  complete sealed-root/manifest identity verification, empty product-source
  and task-backlog diffs, and `git diff --check` all passed. What remains is
  one independent review of the exact sealed neutral draft/evidence. That
  review grants no construction authority; a later candidate attempt still
  requires separate explicit operator direction. There are no current
  blockers.

## EXT-20 RC.11 Review And Prospective RC.12 Builder Validation (2026-07-26)

- Independent controller review reproduced the mode-`0444` neutral draft and
  mode-`0555` exact RC.11 builder as byte-identical 35,599-byte, 634-line
  files at SHA-256
  `c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355`.
  Both still pass `bash -n`, and the neutral draft contains none of the
  forbidden literal RC.11 identity strings.
- Inspection and an independent neutral probe reproduced the semantic defect:
  creating `source/nested` mode `0500` succeeds, but the immediately following
  file creation fails with exit `1` because its parent is not writable. The
  original probe left no residue. Correct ordering is write while the nested
  directory is writable, seal file/nested/source, then copy and verify.
- The exact builder remains the only RC.11 runtime path. Both final paths and
  every preflight/build/stage/diagnostic/ref/workflow/suite/review path remain
  absent. Exact source/tree and empty product-source diff checks passed.
- RC.6/RC.7 terminal manifest pairs and six streams passed again. RC.8 files
  and trees, RC.9 files/trees/staged manifests, and RC.10's sole builder all
  matched. No source, dependency, or historical evidence changed.
- Raw Git committed the accepted RC.11 failure record as
  `0bb21ad157d79a2583f58ce385746cfc24713e12` (`Record failed RC.11 local
  construction`) and pushed it by fast-forward to `origin/main`; public
  readback matched.
- Added executable `agent-ext20-rc12-builder-validation.sh`, SHA-256
  `9aa31f45fc925e214c180fad8abac262d812f93b795f3a22105ac4d3853820e3`.
  This is a neutral preconstruction gate, not RC.12 construction authority.
- The next pass may author only an anonymous prospective builder under a
  unique `/tmp/revolvr-builder-validation.*` root. Before any RC.12-named
  runtime object, it must pass syntax, execute a complete non-mutating
  `--neutral-admission` mode with the corrected copy probe, pass a focused
  static audit, and safely refuse full mode from its neutral identity.
- Exact next command: `./agent-ext20-rc12-builder-validation.sh`. It grants no
  candidate construction, remote, suite, live-model, release, external-use,
  recovery, queue, or `EXT-20` completion authority.

## EXT-20 RC.11 Local Construction Failed At Copy Probe (2026-07-26)

- Task selected: exactly one bounded local construction and verification
  attempt for fresh candidate `level1-v0.1.0-rc.11`, under still-unchecked
  `EXT-20`. No `gh`, commit, push, nested Codex/model invocation, Revolvr live
  operation, suite, confirmation token, ref, workflow, tag, release,
  attestation, external-use decision, recovery, or queue action was used.
- Read-only admission passed before any RC.11 runtime identity. Local, fetched,
  and public `main` were exact at controller commit
  `846e76199a9ad6c7c286fd3371d670881e5ab2d8`; published source
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` was reachable from
  `origin/main` with tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`; and the product-source diff
  for `.agent/profiles`, `cmd`, `internal`, `go.mod`, and `go.sum` was empty.
  Local/remote candidate and attestation refs, tags, workflow, public release
  assets, public Actions artifacts, final bundles, runtime/build/stage/
  preflight/diagnostic roots, suite/launch roots, and local-review launcher
  were collision-free.
- The proposed builder was first authored only as neutral draft
  `/tmp/revolvr-builder-draft.cZLxf2/builder.sh`; neither its path nor content
  contains `rc11`, `rc.11`, or the candidate name. Its first `bash -n` passed.
  Quoting/inventory review found one `find -name` inventory-exclusion defect,
  which was corrected only while the file remained neutral; no syntax repair
  was needed. The final sealed draft is 35,599 bytes, 634 lines, mode `0444`,
  SHA-256
  `c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355`,
  and passed `bash -n` and the complete quoting/inventory review.
- Exact ignored builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.11.sh` was copied from
  the sealed draft, made mode `0555`, and required byte identity. It is also
  35,599 bytes and 634 lines with SHA-256
  `c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355`;
  `cmp` and its independent `bash -n` passed. It was then executed exactly
  once and was never edited.
- The sole execution failed closed at line 295 before the `cp -a` operation:
  `source/nested` had been created at mode `0500`, so the following
  `printf` could not create `source/nested/value.txt` and returned
  `Permission denied`. The function made the neutral probe parents writable,
  removed only that probe, confirmed the probe absent, and reported
  `candidate construction failed: copy-publication probe failed`. Because the
  exact builder already existed, RC.11 is exhausted and no repair or rerun is
  authorized.
- Failure preceded RC.11 preflight, selected-toolchain record, build root,
  clones, caches, stage, diagnostic, artifact, candidate bundle, and
  verification bundle. Intended final paths
  `.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3` and
  `.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3-verification`
  remain absent. No tests, vet, module verification, vulnerability scan,
  release builds, reproducibility comparison, embedded-metadata check, or
  bundle-manifest verification ran inside the builder. No local-review
  launcher was created because no candidate exists.
- Before and after, RC.6 suite / launch record / terminal evidence remained
  exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
  `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`,
  and RC.7 remained exact at
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`.
  Both terminal `files.sha256` / `bundle.sha256` manifest pairs passed before
  and after.
- RC.8's exact mode-`0555` builder, mode-`0664` diagnostic, 15,882-file
  partial root, 20-file verification subtree, four recorded hashes, and both
  final-path absences passed before and after. RC.9's exact builder,
  diagnostic, 6-file preflight root, 34,435-file build root, 63-file stage
  root, 13-file candidate stage, 50-file verification stage, inventory files,
  manifest checks, and final-path absences also passed. RC.10 remained exactly
  one 474-line mode-`0664` builder with SHA-256
  `229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be`;
  every other RC.10 path stayed absent.
- Verification commands run included forbidden-identity scans and `bash -n`
  on the neutral draft; quoting/inventory line review; `cmp`, SHA-256, mode,
  size, line-count, and `bash -n` checks on the exact builder; raw `git fetch`,
  `rev-parse`, `merge-base`, `diff`, `show-ref`, `ls-remote`, and public GitHub
  release/artifact/tree collision reads; the exact builder's sole execution;
  read-only `sha256sum -c` checks for both RC.6/RC.7 terminal manifest pairs
  and both RC.9 staged manifest pairs; complete recorded file/tree
  hash/count/mode checks for RC.6 through RC.10; and `git diff --check`.
  Admission, syntax, byte-identity, collision, and historical-preservation
  checks passed. Construction failed at the copy probe as described above;
  the explicit post-identity no-repair rule prohibited a repair attempt.
- Tracked durable-state changes are `.agent/TASKS.md`, `.agent/HANDOFF.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md`; no product source or dependency
  changed. Ignored output is only the exact RC.11 builder, while the neutral
  draft remains outside the repository. Result: **BLOCKED; RC.11 exhausted
  before construction**. `EXT-20` remains unchecked. What remains is an
  independent controller review of this exact failure record. Any future
  candidate requires a new collision-free identity and explicit operator
  authority.

## EXT-20 RC.10 Review And RC.11 Local Continuation (2026-07-26)

- Independent controller review reproduced RC.10's sole ignored builder at
  474 lines, mode `0664`, and SHA-256
  `229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be`.
  `bash -n` again exited `2` at line 443 with an unexpected `}`.
- Byte-level inspection found the precise cause at line 441: the final
  candidate-inventory `file_hash` substitution has no inner closing quote
  before `)`. The subsequent verification-inventory line has the correct
  quote/substitution order. The RC.10 builder remains unchanged and was not
  executed.
- The builder is the only RC.10 runtime path. Both final bundle paths, every
  preflight/build/stage/diagnostic path, refs, tags, workflow, suite, launch
  record, and review launcher remain absent. Exact source/tree admission and
  the empty product-source diff passed.
- RC.6 and RC.7 terminal manifest pairs passed again and all six protected
  streams matched. RC.8 files/trees and RC.9 files/trees/staged manifests and
  all historical final-path absences matched. No source, dependency, or
  historical artifact changed.
- Raw Git committed the accepted RC.10 failure record as
  `eda445f11eb52fcab9414311fb33f931921cd881` (`Record failed RC.10 local
  construction`) and pushed it by fast-forward to `origin/main`; public
  readback matched.
- Added executable `agent-ext20-rc11.sh`, SHA-256
  `f143179a6baa9b8851c5472aac9b8702b5f2800f76a2da4ce33c05446203b447`.
  It pins the same source, preserves RC.10's sole malformed-builder identity,
  rejects RC.11 collisions, and retains the clean exact-toolchain and proven
  copy-publication boundaries.
- RC.11 builder text must first pass `bash -n` in a neutral anonymous draft
  path containing no RC.11 identity. At most one syntax repair is allowed
  there. Only syntax-passing exact bytes may become the read-only RC.11
  builder, which must remain byte-identical to the draft and pass `bash -n`
  again before one execution. A later failure exhausts RC.11.
- Exact next command: `./agent-ext20-rc11.sh`. This grants no remote, suite,
  live-model, release, external-use, recovery, queue, or `EXT-20` completion
  authority.

## EXT-20 RC.10 Local Construction Failed Before Execution (2026-07-26)

- Task selected: exactly one bounded construction and local-verification pass
  for fresh candidate identity `level1-v0.1.0-rc.10-a24804bcf2a3`, under
  still-unchecked `EXT-20`. No nested Codex run was started; the tracked
  `agent-ext20-rc10.sh` wrapper was inspected but not executed because it
  delegates to `codex exec`, which this pass expressly prohibited.
- Pre-construction read-only admission passed against exact local, fetched,
  and public `main` commit
  `19f73c426558c5b9d2567c76b3a7fc57206e3e8a`. Published source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` was reachable from
  `origin/main`, resolved tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, and the product-source diff
  for `.agent/profiles`, `cmd`, `internal`, `go.mod`, and `go.sum` was empty,
  SHA-256
  `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
  Local/remote RC.10 refs and tags, workflow, exact public candidate and
  verification artifacts, release assets, runtime/build/stage/preflight/
  diagnostic roots, suite/launch roots, final bundles, and review launcher
  were collision-free.
- Fresh independently authored ignored builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.10.sh` is 474 lines,
  mode `0664`, and SHA-256
  `229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be`.
  Its first verification command, `bash -n
  .revolvr/release-candidates/build-level1-v0.1.0-rc.10.sh`, failed with exit
  status `2` at line 443: `syntax error near unexpected token '}'`. Because
  an RC.10 identity path already existed, the fail-closed no-repair rule
  exhausted RC.10. The builder was retained unchanged and was never executed.
- Failure occurred before selected-Go admission, the required neutral
  `mkdir`/`cp -a` publication probe, or creation of an RC.10 preflight root,
  build root, clone, cache, stage, diagnostic, artifact, candidate bundle, or
  verification bundle. Exact intended final paths
  `.revolvr/release-candidates/level1-v0.1.0-rc.10-a24804bcf2a3` and
  `.revolvr/release-candidates/level1-v0.1.0-rc.10-a24804bcf2a3-verification`
  remain absent. No local-review launcher was created because there is no
  candidate to review.
- Before builder creation and again after the syntax failure, RC.8's builder,
  diagnostic, 15,882-file partial root, 20-file verification subtree, and
  both final-path absences matched their recorded identities. RC.9's builder,
  diagnostic, 6-file preflight root, 34,435-file build root, 63-file stage
  root, 13-file staged candidate, 50-file staged verification subtree, both
  inventories, and both final-path absences also matched. Both RC.9 staged
  manifest pairs passed read-only.
- Before and after, RC.6 suite / launch record / terminal evidence remained
  exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
  `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`,
  and RC.7 remained exact at
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`.
  Both terminal `files.sha256` / `bundle.sha256` manifest pairs passed.
- Files changed in tracked durable state are `.agent/TASKS.md`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`; no
  product source or dependency changed. Result: **BLOCKED; RC.10 exhausted
  before construction**. It grants no remote-CI, attestation, dogfood,
  live-model, suite, tag, release, external-use, recovery, queue, or
  `EXT-20` completion authority. The next gate is independent controller
  review of this immutable failure. Any later construction requires a new
  collision-free identity and explicit operator authority; no continuation
  launcher is authorized here.

## EXT-20 RC.9 Review And RC.10 Local Continuation (2026-07-26)

- Independent controller review reproduced the RC.9 builder/diagnostic hashes
  and modes; all preflight/build/stage/subtree counts and content-stream
  hashes; both staged inventory and inventory-file hashes; all three artifact
  hashes; exact source/tree/tool metadata; and both absent final paths. Both
  staged manifests pass read-only.
- The retained Go 1.22.12 and Go 1.26.5 full-suite/module results, Go 1.26.5
  vet result, reproducibility table, embedded metadata, and govulncheck output
  are consistent. Govulncheck reports no reachable or imported-package
  vulnerability and only unreachable Windows-specific `GO-2026-5024` in a
  required module. No source or dependency changed.
- The exact RC.9 directory-move failure was reproduced with a fresh neutral
  probe under `.revolvr/release-candidates`: creating and sealing a directory
  succeeded, while moving it from its staging parent into a sibling path was
  denied. Ownership, modes, ACLs, ext4 mount options, and free space were
  compatible. A separate neutral probe proved `mkdir` plus `cp -a
  source/. destination/` succeeds and preserves read-only directory modes.
  All controller probe paths were removed.
- RC.6 and RC.7 terminal checksum manifest pairs passed again; all six
  protected content-stream hashes matched. RC.8 retained files/trees and
  absent paths also matched. Product source remained identical to pinned
  source commit `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`.
- Raw Git committed the accepted RC.9 failure record as
  `f81b4a0144164aac71f8d943ca2ee6f4ca801648` (`Record failed RC.9 local
  construction`) and pushed it by fast-forward to `origin/main`; public
  readback matched.
- Added executable `agent-ext20-rc10.sh`, SHA-256
  `14d99ada8f2adb78a2e80c18a19df27428a3848f66a0627bbe7cad6ff73fdde6`.
  It pins the same published source, preserves all failed RC.9 identities,
  rejects RC.10 collisions, retains the clean exact-Go environment, and
  requires a fresh build with a preflighted non-rename publication method.
- RC.10 may publish sealed staged content locally only by creating absent
  final directories, copying with `cp -a`, then verifying complete manifests,
  content hashes, modes, file counts, and stage/final equality. A failure after
  either final path appears exhausts RC.10 and cannot be repaired.
- Exact next command: `./agent-ext20-rc10.sh`. This grants no remote, suite,
  live-model, release, external-use, recovery, queue, or `EXT-20` completion
  authority.

## EXT-20 RC.9 Local Candidate Construction Failed Closed (2026-07-26)

- Task selected: exactly one bounded construction and local-verification
  attempt for fresh candidate identity `level1-v0.1.0-rc.9`, under still-
  unchecked `EXT-20`. The pass did not start Revolvr, Codex, a model, a
  dogfood operation, or a suite and did not create or publish a ref, workflow,
  attestation, tag, release, or external-use/recovery/queue authority.
- Pre-construction authority passed. Local, fetched, and public `main` were
  exact at controller commit
  `9393c58c36a0a9d55397cbf1f2db1490d8650f50`; pinned published source
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` was reachable in its ancestry
  with exact tree `2c8ee9f6b4283410547a9f99972e25eac06c9e33`. The diff through
  controller HEAD for `.agent/profiles`, `cmd`, `internal`, `go.mod`, and
  `go.sum` was empty, SHA-256
  `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.
  Local/remote RC.9 refs and tags, workflow, public candidate/verification
  artifacts, final bundles, temporary roots, suite/launch roots, diagnostics,
  and review launcher were collision-free.
- Before creating the build root, exact clean tool admission recorded source-
  floor Go
  `/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64/bin/go`,
  SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`,
  `go1.22.12`, GOROOT
  `/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64`,
  and matching `pkg/tool/linux_amd64`; release Go `/usr/local/go/bin/go`,
  SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  `go1.26.5`, GOROOT `/usr/local/go`, and matching
  `/usr/local/go/pkg/tool/linux_amd64`; and `govulncheck@v1.1.4` at
  `/home/gernsback/go/bin/govulncheck`, SHA-256
  `f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f`.
  Every selected Go invocation explicitly removed `GOROOT`, `GOTOOLDIR`, and
  `GOFLAGS`, set `GOENV=off` and `GOTOOLCHAIN=local`, and used distinct
  task/clone/toolchain `GOCACHE` and `GOMODCACHE` roots.
- Fresh independently authored ignored builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.9.sh` is mode `0555`,
  452 lines, SHA-256
  `0b4dcba0a68aa9d801d657a085fa1c8b7a81fd503bea773b0670ec394f456ab4`.
  `bash -n` passed before its sole execution. It independently fetched two
  shallow exact-source checkouts from `git@github.com:ponchione/revolvr.git`;
  both were clean at the pinned commit/tree and neither object database
  contained later controller commit
  `9393c58c36a0a9d55397cbf1f2db1490d8650f50`.
- All requested source and product verification completed successfully before
  sealing: with both Go 1.22.12 and Go 1.26.5, full ordinary package tests for
  `internal/autonomousplanning`, `internal/prompt`, and
  `internal/autonomouscycle`; the same packages with `-race`; focused
  `TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset`;
  ordinary and race `TestProductionAutonomousHappyPath` plus
  `TestStrictFakeCodexContract`; and `go test -mod=readonly -count=1 ./...`.
  Go 1.26.5 `go vet -mod=readonly ./...` passed, and `go mod verify` passed
  under both toolchains. All detailed command/output and effective-environment
  records are retained in the staged verification tree below.
- `govulncheck ./...` and `govulncheck -show verbose ./...` passed. The result
  reports zero reachable and zero imported-package vulnerabilities. It records
  unreachable Windows-only required-module finding `GO-2026-5024` in
  `golang.org/x/sys@v0.30.0`, fixed in `v0.44.0`; no dependency was changed.
- Exact Go 1.26.5, version `0.1.0`, module-readonly, CGO-disabled, amd64,
  trimpath, clean-VCS, empty-build-ID builds completed twice in independent
  clones for all three supported targets. Clone-one/clone-two artifacts were
  byte-identical: Linux SHA-256
  `c164c7da5a9926b5893c2a4679bf7b31b6423a74f7e86504cedab4d12fe68456`,
  Darwin `a9a7a56883ef5026ec87b4beaca277dfe1c54083565b17d3aa7fd0ce8c0bc88d`,
  and FreeBSD
  `7d2bd704259dad5ef060670ea33c0d5bbde1463e42c5700cac00110c40d1db94`.
  `revolvr 0.1.0`, embedded Go/target/source revision, clean VCS state, disabled
  CGO, and empty Go build IDs all passed.
- Both staged inventories verified before the terminal failure and verify
  read-only afterward. Staged candidate
  `.revolvr/release-candidates/.level1-v0.1.0-rc.9-stage.Znq9cp/candidate`
  has 13 regular files, content-stream SHA-256
  `435bf3a09174857c8c855538a3d29ab87d528db167d997e34a8db6737c889e54`,
  `files.sha256` SHA-256
  `d08e74abedbe089196ab4bc1e6f05384b370fa114bc715eab3196c708d60c69d`,
  and `files.sha256.sha256` SHA-256
  `afd7d45ca3e1e91972bcff57c3cb9435f21a7383a50b5e00822ca7dedf3332aa`.
  Staged verification
  `.revolvr/release-candidates/.level1-v0.1.0-rc.9-stage.Znq9cp/verification`
  has 50 regular files, content-stream SHA-256
  `9c47547b117436453f4610021bf677d245adf918da05d5f642361cde684342dd`,
  `files.sha256` SHA-256
  `52d9ad1cd586929a97c20c069f7ce086d4e7df464aeb679819f273d58773eb35`,
  and `files.sha256.sha256` SHA-256
  `62156c79a484c8337955963292919ea910b20fdfc194e472c5d0a6a6a6b8afc8`.
  They are terminal partial construction output, not candidate bundles, and
  cannot be moved, relabeled, completed, repaired, or reused.
- Construction failed closed at exact command
  `mv "$STAGE_CANDIDATE" "$FINAL_CANDIDATE"` with `Permission denied`, after
  staging/sealing but before either authorized final path appeared. Retained
  mode-`0400` diagnostic
  `.revolvr/release-candidates/diagnostics/level1-v0.1.0-rc.9-20260726T130454Z-370868.qiRJxA.txt`
  has SHA-256
  `b61fc1cf82d7777b58337766c0fe941b901101b6e14fd1b9e1cd9fb7a1774160`.
  Retained preflight root
  `.revolvr/release-candidates/.level1-v0.1.0-rc.9-preflight.XQ9PIf` has 6
  regular files and content-stream SHA-256
  `a52b2f624e03276d2079a45c73e3c172a5387608c341f7376bd7f6cb54959547`;
  complete stage root
  `.revolvr/release-candidates/.level1-v0.1.0-rc.9-stage.Znq9cp` has 63 files
  and SHA-256
  `382e16afb4efbbf25330572ab5ee186001a06bc21112cd18b41414e790519a46`;
  retained build root `/tmp/revolvr-ext20-rc9-build.CRYAYI` has 34,435 files
  and SHA-256
  `2f2d3392a20afffc6b676cd72b4b71e1f8283f2567dac623b506880c273237e6`.
- Intended final candidate
  `.revolvr/release-candidates/level1-v0.1.0-rc.9-a24804bcf2a3` and final
  verification path with suffix `-verification` remain absent. No candidate
  or verification bundle is admitted, and no local-review launcher was
  created. No ref, tag, workflow, suite, launch record, Revolvr/Codex/model
  operation, or external authority exists for RC.9.
- RC.6 and RC.7 terminal `files.sha256` and `bundle.sha256` manifests passed
  before and after. Their suite / launch-record / terminal-evidence stream
  SHA-256 values remained respectively RC.6
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
  `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`
  and RC.7
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`.
  RC.8 builder/diagnostic and 15,882-file/20-file retained roots remained at
  all four admitted hashes, and both intended RC.8 final paths stayed absent.
- Files changed in tracked state are only `.agent/HANDOFF.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md`; `.agent/TASKS.md` remains
  SHA-256 `e3fd07a399498aaebf4191938813081fc86b841f8baf1b050a169ceb57ba10b1`
  with `EXT-20` unchecked. Result: **BLOCKED; RC.9 terminally failed local
  construction**. The next gate is independent controller review of this
  exact failure record. Any later candidate requires a fresh collision-free
  identity and explicit operator authority; no command or review launcher for
  such work is authorized by this pass.

## EXT-20 RC.8 Review And RC.9 Local Continuation (2026-07-26)

- Independent controller review reproduced the failed RC.8 builder and
  diagnostic SHA-256 values, the 15,882-file partial-root and 20-file partial-
  verification content-stream hashes, both absent final paths, and the lack of
  RC.8 refs, tags, workflow, suite, launch record, or review launcher. The
  product-source diff from exact source commit
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` through controller HEAD was
  empty.
- With `GOROOT=/usr/local/go` inherited, the exact Go 1.22.12 executable is
  misbound. With `GOROOT` and `GOTOOLDIR` removed, `GOENV=off`, and
  `GOTOOLCHAIN=local`, it resolves its own module-toolchain root and matching
  tool directory. The exact Go 1.26.5 executable similarly resolves
  `/usr/local/go`. This confirms an environment defect rather than a product
  test failure.
- Both RC.6 and RC.7 terminal checksum manifests passed again. All six
  protected suite / launch-record / terminal-evidence content-stream hashes
  matched. No product or historical evidence file was changed.
- Raw Git committed the accepted RC.8 failure record as
  `247b4f6065049a77fa6f07504410de7549e10ac0` (`Record failed RC.8 local
  construction`) and pushed it by fast-forward to `origin/main`; public
  readback matched.
- Added executable `agent-ext20-rc9.sh`, SHA-256
  `4fe92d41af3d6d0144168d7b0867f9ea1bace7f4648b395485af7501cfdc05e3`.
  Its admission preserves the exact failed RC.8 files/trees and absent final
  paths, rejects every RC.9 collision, pins the approved source commit/tree,
  and requires exact clean bindings for the Go 1.22.12 source floor and Go
  1.26.5 release toolchain before a fresh construction pass.
- The child pass receives no inherited `GOROOT`, `GOTOOLDIR`, or `GOFLAGS`,
  uses `GOENV=off` and `GOTOOLCHAIN=local`, and must repeat those controls for
  every Go invocation with independent task-specific caches. It may only
  construct and locally verify `level1-v0.1.0-rc.9`; it cannot publish, create
  a suite, run live Revolvr/model work, tag, release, approve external use,
  grant recovery/queue authority, or complete `EXT-20`.
- Exact next command: `./agent-ext20-rc9.sh`.

## EXT-20 RC.8 Local Candidate Construction Failed Closed (2026-07-26)

- Task selected: exactly one bounded local construction and verification
  attempt for collision-free candidate `level1-v0.1.0-rc.8`, under the still-
  unchecked `EXT-20`. No live Revolvr/model operation, suite, remote ref,
  workflow, tag, release, external-use decision, recovery, or queue authority
  was admitted.
- Source/collision preflight passed before construction. Local, fetched, and
  public `main` agreed at controller commit
  `221d8becdc0aee9aa00b4879f11bec28f97c242f`; exact published source
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba` is reachable from it and has
  tree `2c8ee9f6b4283410547a9f99972e25eac06c9e33`. The product-source diff
  through controller HEAD is empty for `.agent/profiles`, `cmd`, `internal`,
  `go.mod`, and `go.sum`. Local/remote RC.8 refs and tags, workflow, public
  artifact name, final bundles, build root, suite/launch/runtime roots,
  diagnostic namespace, and local-review launcher were all absent.
- Exact tools admitted were release Go `/usr/local/go/bin/go`, `go1.26.5`,
  SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`;
  source-floor Go
  `/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64/bin/go`,
  `go1.22.12`, SHA-256
  `929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec`;
  and `govulncheck@v1.1.4`.
- Added ignored fail-closed builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.8.sh`, SHA-256
  `b4b23c2ede3502f666253e8b34e6962df58bbf848fe108c51172b95619d45b6e`.
  Its one construction attempt retained diagnostic
  `.revolvr/release-candidates/diagnostics/level1-v0.1.0-rc.8-20260726T121225Z-227936.txt`,
  SHA-256
  `5cc3e477270fd051d967bf63de50a4c2ad007ed8b5217a429e8a8525ebcc89c5`.
- Retained partial build root `/tmp/revolvr-ext20-rc8-build.wnKv7Q` contains
  15,882 regular files and has path-bearing content-stream SHA-256
  `576db5a2a76ef52013b54c1cb29d5623908e57b1f34a772d7f56e5635cf952e2`.
  Its unsealed 20-file verification subtree has content-stream SHA-256
  `ea9fdc5cc97e475d13109d8a3b1d7eafb25e4b3cd87e4085c5bef44aa3e2841a`.
  It includes two independently fetched, exact detached clean source clones;
  neither contains the later RC.8 controller launcher. This partial root is
  immutable diagnostic history, not candidate evidence, and must not be
  retried, repaired, completed, relabeled, or reused.
- Verification passed before the stop: complete ordinary and race tests for
  `internal/autonomousplanning`, `internal/prompt`, and
  `internal/autonomouscycle`; focused Structured Outputs compatibility guard
  `TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset`; and
  ordinary/race production tests `TestProductionAutonomousHappyPath` and
  `TestStrictFakeCodexContract`.
- Verification then failed during `go1.22.12 test -count=1 ./...`. The host
  exports `GOROOT=/usr/local/go`, so direct invocation of the exact Go 1.22.12
  binary selected the Go 1.26.5 standard library/tool directory and emitted
  repeated `compile: version "go1.26.5" does not match go tool version
  "go1.22.12"`. With `GOROOT` unset, the executable resolves its own exact
  module-toolchain root. Per the no-reuse/fail-closed direction, no repair or
  second attempt was made. This is a construction-environment blocker, not a
  failing product regression.
- Go 1.26.5 full tests, vet, module verification, vulnerability scans,
  release builds, reproducibility checks, artifact metadata checks, and bundle
  sealing did not run. Final candidate path
  `.revolvr/release-candidates/level1-v0.1.0-rc.8-a24804bcf2a3` and final
  verification path with suffix `-verification` remain absent. No inert
  local-review launcher was created because there is no candidate to review.
- Both RC.7 and RC.6 terminal `files.sha256` and `bundle.sha256` manifests
  passed before and after. Their suite / launch-record / terminal-evidence
  content-stream SHA-256 values remained respectively RC.7
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`
  and RC.6
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
  `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- `.agent/TASKS.md` remains unchanged at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. Tracked files changed by this pass are only
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`; ignored
  changes are the builder and retained diagnostic/work root above.
- Result: BLOCKED, no candidate constructed. No separately authorized local
  review gate exists. What remains is a fresh controller review of this
  failure and explicit operator direction for a new collision-free candidate
  identity and construction environment; the failed RC.8 namespace cannot be
  overwritten or reused.

## EXT-20 Planner Remediation Publication And RC.8 Continuation (2026-07-26)

- Controller review independently reverified exact local/fetched/public
  `main` at parent `a9b4f50465466fee61e13f131ae6679e3d8b4729`, shell syntax,
  zero formatting delta for all changed Go files, focused ordinary and race
  packages, production happy-path and strict-fake tests, `go test -count=1
  ./...`, `git diff --check`, and the RC.7 terminal checksum manifests. No
  blocker or source correction was found.
- Raw Git committed the accepted 12-file remediation/review scope as
  `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
  `2c8ee9f6b4283410547a9f99972e25eac06c9e33`, parent
  `a9b4f50465466fee61e13f131ae6679e3d8b4729`, with subject `Align planner
  lifecycle output contract`, and pushed it by fast-forward to
  `refs/heads/main`. Local, fetched, and public raw-Git readback all matched.
- The separate controller continuation adds executable
  `agent-ext20-rc8.sh`, SHA-256
  `d27842590accf418e20f9ddd5ca048e6b299776ad7d8b17f02fc933c51003284`.
  Syntax passed. Pre-construction checks found no local/remote RC.8 candidate
  or attestation ref, local/remote RC.8 tag, workflow, review launcher, runtime
  root, or bundle collision.
- The launcher pins candidate source to the published remediation commit/tree,
  requires clean exact local/fetched/public `main`, and refuses any later
  `.agent/profiles`, `cmd`, `internal`, `go.mod`, or `go.sum` delta. The
  controller-only launcher/state commit therefore cannot silently become
  candidate source.
- The next pass is local RC.8 candidate construction only. It preserves RC.6
  and RC.7 at their recorded immutable hashes, runs no live suite or product
  model operation, publishes nothing, and grants no remote-CI, attestation,
  dogfood, tag, release, external-use, recovery, queue, or `EXT-20` completion
  authority. Exact next command is `./agent-ext20-rc8.sh`.

## EXT-20 Planner Lifecycle-Contract Independent Review (2026-07-26)

- Task selected: one bounded independent review of the current uncommitted
  planner-contract remediation and its focused tests. No source or test fix
  was made because inspection and verification found no concrete correctness
  defect.
- The retained RC.7 planner prompt and raw output reproduced the contract
  failure: pending steps used grounding references as terminal evidence and
  carried skip-style rationale, while pending acceptance criteria carried
  disposition rationale. The remediation consistently directs grounding to
  plan provenance or top-level inputs and requires the empty/null
  representation for nonterminal work.
- The complete prompt, schema, planning-application, and autonomous-state path
  was inspected. `PlanStep.Validate` and `AcceptanceCriterion.Validate` remain
  unchanged and fail closed. Distinct nested `anyOf` branches cover every step
  and acceptance lifecycle status; every branch object is closed and requires
  every property, and the repository's Structured Outputs subset validator
  accepts the resulting production schema.
- Planning revision equality now treats only nil and empty evidence slices as
  the same empty value after canonical JSON state round-trips. Status,
  description, rationale, source, identity, ordering, and nonempty terminal
  evidence remain exact; existing mutation regressions passed.
- Verification passed: `bash -n agent-ext20-planner-contract-review.sh`;
  zero-diff `gofmt -d` inspection of all seven changed Go files;
  `go test -count=1 ./internal/autonomousplanning ./internal/prompt
  ./internal/autonomouscycle`; the same packages with `-race`;
  `go test -count=1 ./internal/app -run
  '^(TestProductionAutonomousHappyPath|TestStrictFakeCodexContract)$'`;
  `go test -count=1 ./...`; and `git diff --check`.
- The RC.7 terminal bundle passed the required read-only `files.sha256` and
  `bundle.sha256` verification before review and again after all review and
  durable-state commands. RC.6 and RC.7 were not otherwise accessed or
  changed.
- Files changed by this independent review are only `.agent/HANDOFF.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md`. The reviewed source/test delta
  and `agent-ext20-planner-contract-review.sh` remain otherwise unchanged and
  uncommitted.
- Result: PASS for a separate controller commit/publication decision. There
  are no review blockers. `EXT-20` remains unchecked, and no commit, push,
  candidate, suite, Revolvr/model operation, live path, tag, release,
  external-use decision, or queue authority was created or exercised.

## EXT-20 Planner Lifecycle-Contract Remediation (2026-07-26)

- Task selected: one bounded root-cause remediation for the RC.7 planner
  prompt/schema mismatch. The operator-supplied terminal-bundle checksum
  result was independently reproduced read-only; every `files.sha256` entry
  and `bundle.sha256` passed.
- The retained RC.7 raw output proved two instances of one lifecycle contract
  mismatch. All three `pending` plan steps carried evidence and rationale, and
  all three `pending` acceptance criteria carried rationale. Existing Go
  validators correctly rejected those nonterminal disposition fields before
  persistence or source mutation.
- The checked-in planner profile and its default template now state the exact
  nonterminal empty/null representation and direct grounding facts to plan
  provenance or top-level inputs. The generated plan-worker output section
  repeats the contract at the point where `PlanningOutput` is requested.
- `PlanningOutputSchema` now uses supported nested status-specific `anyOf`
  object branches. Pending/in-progress steps and pending criteria structurally
  require empty evidence and null rationale; completed/satisfied dispositions
  require evidence; skipped/waived/not-applicable dispositions retain their
  required rationale semantics. Every object remains closed with every
  property required, and the post-model Go validators remain unchanged.
- Planning revision comparison now uses semantic slice equality, so the
  schema-required JSON `[]` remains equal to an omitted empty slice after
  canonical state serialization. Exact nonempty evidence and all other stable
  step/criterion fields remain protected.
- Files changed for the remediation: `.agent/profiles/planner.md`,
  `internal/prompt/profile.go`, `internal/prompt/profile_test.go`,
  `internal/autonomouscycle/worker_prompt.go`,
  `internal/autonomouscycle/cycle_test.go`,
  `internal/autonomousplanning/schema.go`,
  `internal/autonomousplanning/contracts.go`, and
  `internal/autonomousplanning/contracts_test.go`. No dependency was added.
- Focused tests passed with
  `go test -count=1 ./internal/autonomousplanning ./internal/prompt
  ./internal/autonomouscycle` and the same focused packages with `-race`.
  `go test -count=1 ./...` and `git diff --check` also passed. Changed Go
  files were formatted with `gofmt`.
- `.agent/HANDOFF.md` was pruned from historical accumulation to a concise
  active resume pointer; full history remains here, in `.agent/DECISIONS.md`,
  Git, and immutable runtime evidence. New executable continuation launcher
  `agent-ext20-planner-contract-review.sh` starts exactly one fresh independent
  review and grants no publication or live authority.
- `EXT-20` remains unchecked. RC.6 and RC.7 remain immutable and retired. No
  candidate, suite, live operation, tag, release, external-use decision,
  queue authority, commit, push, or product/nested model call was created or
  executed during the remediation. Next work is the independent review
  command recorded in `.agent/HANDOFF.md`.

## EXT-20 RC.7 Terminal Live Failure (2026-07-25)

- The operator executed the exact one-time authorized wrapper. Controller
  local/fetched/public `main` was
  `a9b4f50465466fee61e13f131ae6679e3d8b4729`; the complete direct-launcher
  preflight passed and launch record
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7-launch-records/ext20-14b2bf40212b-20260725T183415Z-1784416`
  was created. It records terminal suite exit status `1` at
  `2026-07-25T18:35:48Z` and content-stream SHA-256
  `deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c`.
- Only `ext20-14b2bf40212b-01` ran. Its sealed terminal bundle is
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite/evidence/repo-a/01-successful-source-change-1`,
  content-stream SHA-256
  `6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`;
  both retained manifests verify. Operations 02 through 11 never started.
- Expected outcome was `completed`; observed outcome was
  `unsafe_or_ambiguous`. The plan application rejected planner step 0 because
  status `pending` included terminal evidence. No implementation,
  verification, audit, correction, commit, or checkpoint ran. Repository and
  workspace remained clean at
  `22bc5fd5ea1469fb76afef6425964f0b0c7f70bb`; the outside sentinel remained
  unchanged.
- Both real model calls completed normally with Codex exit code `0` and
  `turn.completed`: supervisor usage 19,118 input / 809 output tokens; planner
  usage 20,464 input / 3,542 output tokens. This was not an API failure.
- The planner output placed provenance-like references and explanatory
  rationale on every new `pending` step. `PlanStep.Validate` intentionally
  forbids both terminal evidence and skip rationale on `pending` or
  `in_progress` steps. The planner JSON schema requires the two fields and
  structurally permits those values, while the planner profile/prompt asks for
  evidence citations without stating the required `evidence: []` and
  `rationale: null` nonterminal representation. Safe post-model validation
  caught the contract mismatch.
- Post-failure suite content-stream SHA-256 is
  `ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a`.
  It contains exactly one terminal manifest and an empty aggregate. RC.7 is
  retired and must not be rerun, repaired, deleted, mutated, derived from, or
  reused. Any later candidate must be fresh RC.8 with new suite and authority.
- RC.6 remains preserved at its three recorded hashes. `.agent/TASKS.md`
  remains unchanged with `EXT-20` unchecked. No tag, release, external-use,
  queue, recovery, or completion authority exists.

## EXT-20 RC.7 Authorized One-Shot Live Wrapper Construction (2026-07-25)

- Raw Git published the human's exact one-time RC.7 live authorization as
  commit `92d9c38ec9e4bbc01b0b5cf2c75ff3819f36658d`, tree
  `24e43c5eaf002c272aa27bca578345415427bc7e`, parent
  `6a786e3ce6fa436ff66aa75fd663e20fafd425a1`; local, fetched, and public
  `main` readback matched before construction.
- Added executable `agent-ext20-rc7-live-once.sh`, SHA-256
  `240974ac774139e34a8f2f717d2c20e2eb01521d4d98c7bb25a43681d6888066`.
  It accepts only exact operator flag
  `--execute-authorized-rc7-live-suite-once`, refuses all other arguments
  before authority lookup, requires clean exact local/fetched/public main,
  verifies the published authorization commit, exact direct launcher and task
  backlog, and an absent RC.7 launch-record root, then sources definitions and
  `exec`s the direct launcher with its retained internal confirmation.
- Construction verification exercises only syntax, missing/wrong/multiple
  argument refusal, and the admitted flag's dirty-controller refusal. The live
  branch, direct confirmation, launch-record reservation, suite process,
  Revolvr, and model path were not executed during construction.
- The exact operator command is
  `./agent-ext20-rc7-live-once.sh
  --execute-authorized-rc7-live-suite-once`. It is one-shot: any launch-record
  root or operation evidence ends authority, including after terminal failure
  or interruption. No retry or recovery is authorized.
- RC.7 remains unstarted at content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`
  with ten pending zero-attempt tasks and no launch record. RC.6 remains exact
  and immutable. `.agent/TASKS.md` remains exact with `EXT-20` unchecked.

## EXT-20 RC.7 Explicit One-Time Human Live Authorization (2026-07-25)

- Immediately after the inert authorization-status script reported technical
  readiness, absent consent, and the exact material effects, the human
  operator responded `Authorized`. In that direct context, this is explicit
  consent for exactly one RC.7 live suite launch.
- The authorized scope is eleven real-Codex operations over ten tasks in two
  disposable repositories: five ordinary successful changes, correction/
  final-verification/re-audit, needs input, intentional verification failure,
  cancellation/new-operation restart, and safety refusal. Consent includes
  approval/sandbox bypass, inherited host environment and network access,
  real model cost/time, deliberate cancellation, workspace/source/control-
  metadata commits, and immutable launch/runtime/model/ledger/receipt/partial-
  failure evidence retention.
- Authority is one-shot and fail-closed. A launch record or operation evidence
  prevents automatic retry. The authorization grants no recovery, merge,
  push, tag, release, external-use, queue, or `EXT-20` completion authority.
  It does not authorize RC.6 execution or mutation.
- Before recording consent, clean local, fetched, and public `main` agreed at
  `6a786e3ce6fa436ff66aa75fd663e20fafd425a1`; the direct launcher remained
  SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`,
  and its complete `--check` preflight passed without a model call or launch
  record. RC.7 remains unstarted and `.agent/TASKS.md` remains exact with
  `EXT-20` unchecked.
- The next controller task may construct, check without entering its admitted
  branch, and publish one confirmation-gated wrapper. The controller may not
  execute it. Operator execution must remain a separate exact command after
  clean/public readback.

## EXT-20 RC.7 Authorization Boundary Publication And Status Handoff (2026-07-25)

- Independent controller replay accepted the human-authorization checkpoint's
  exact three durable-state files and its result: technical admission PASS,
  explicit human live authorization ABSENT. Raw Git published the record as
  commit `3c621ddfb8d11905150498c9cd3cc173323ac816`, tree
  `d5a313ed6a86d73bedb9ad6b1997f2e8e32e5b2e`, parent
  `133fb6ddb2f94873d572dd72b9c91ef337cb87b3`; local, fetched, and public
  `main` readback matched.
- Replay passed syntax, focused refusal/collision checks, complete sourced
  remote/bundle/suite/repository/task authority, and diff hygiene. No Go,
  source, or test file changed, so the exact already-published full Go test
  result remains applicable. The clean published direct-launcher `--check`
  path also passed without a model call, launch record, or runtime mutation.
- RC.7 remains exact at content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`
  with ten pending zero-attempt tasks, zero operation/collector/model/receipt
  evidence, empty aggregate, and no launch-record root. Protected RC.6 suite,
  launch record, and terminal evidence remain exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Added executable inert status continuation
  `agent-ext20-rc7-await-live-authorization.sh`, SHA-256
  `4a90526c5e5fc26c57470ac6cae197976aca449d7394899c4ca37b9efc23b332`.
  It binds clean local/fetched/public main, exact published checkpoint,
  launcher/backlog identities, and absent launch records; runs only the
  launcher's `--check`; and prints the effects requiring explicit human
  consent. It contains no Codex invocation, live confirmation, `--live`, or
  state mutation path.
- The current operator direction still forbids real model calls and does not
  authorize the eleven-operation live suite. `.agent/TASKS.md` remains exact
  at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No live command is active; after the inert status
  readback, further progress requires new explicit human authorization.

## EXT-20 RC.7 Human-Authorization Checkpoint (2026-07-25)

- Task selected: exactly one no-model human-authorization checkpoint under
  still-unchecked `EXT-20`. The prepared checkpoint wrapper was inspected but
  not executed because it starts a nested Codex session; this already-active
  fresh pass performed its bounded checks directly. No live authorization
  value was supplied, reproduced, derived, or embedded. No live path, launch
  record, Revolvr task operation, model call, executable change, tag, release,
  external-use approval, queue authority, or `EXT-20` completion was attempted.
- A fresh no-tags raw-Git fetch left the controller clean. Local `main`, fetched
  `origin/main`, raw-Git public `main`, and public-REST `main` agreed at
  `133fb6ddb2f94873d572dd72b9c91ef337cb87b3`. Published admission-
  recommendation commit `1a07077f24526b9202da55c2981911b7e0770a67`,
  tree `1cb2ed843d1d5cf294487e8e404a030bb9f9838c`, parent
  `b6351d108fc971dcfff5367267fb7eb1a3b00273` was exact and present in both
  local and fetched-main ancestry. Its exact inspected delta changed only
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`.
- The proposed one-shot live scope is exactly eleven planned operations over
  ten unique tasks in two disposable repositories. It includes five ordinary
  successful source changes, a correction/final-verification/re-audit path, a
  typed needs-input stop, an intentional verification-failure stop, an
  attended cancellation followed by a new-operation restart of the same task,
  and a supervisor safety refusal. Each operation may make real ephemeral
  `gpt-5.6-sol` calls at `xhigh`, with a 30-minute timeout per Codex invocation
  and at most 50 operation cycles; source-writer authority requires 32 minutes.
- Material authority requiring knowing human consent includes
  `dangerously_bypass_approvals_and_sandbox: true`, inherited host environment,
  no external-isolation enforcement, and unknown network access with no
  network enforcement. The run may create task workspaces and source commits,
  advance disposable control-repository task-metadata commits, write runtime
  task/operation/ledger/receipt/history state, and retain model output. The
  cancellation case deliberately sends an interrupt after durable admission.
- Retained evidence includes a unique launch record with pre-start authority,
  status, stdout, and stderr; per-operation before/after snapshots, logs,
  manifests, task-operation history, receipts, validated ledger exports, and
  captured runtime evidence; and deterministic aggregate operation/report
  hashes. A terminal failure or interruption deliberately leaves partial
  diagnostics and prior completed bundles in place. The direct gate refuses a
  pre-existing launch-record root or operation evidence, so this is one-shot
  authority without an automatic retry or recovery path. It does not merge or
  push task work or authorize use of retained RC.6 evidence.
- Direct launcher SHA-256 remained
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`.
  Inspection confirmed check-only exits after the complete preflight and
  before launch-record reservation, pre-start/status writes, interruption
  traps, suite execution, or model activity. `bash -n` passed for the
  checkpoint wrapper, direct launcher, and focused checker. Independent
  missing, wrong, and multiple-argument refusals each returned `64`, emitted
  zero stdout bytes, retained usage on stderr, and created no launch record.
  The only complete launcher path executed was its `--check` path; it returned
  `0` after the full published preflight and reported no model call or launch
  record.
- Prepared suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite` remained ID
  `ext20-14b2bf40212b` at authority/plan/content-stream SHA-256
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e` /
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1` /
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  All ten tasks remain pending and zero-attempt, with zero operation,
  collector, model-run, receipt, or aggregate evidence and no RC.7 launch-
  record root.
- Read-only RC.6 suite, launch record, and terminal evidence remained exact
  before and after at content-stream SHA-256
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed, repaired, deleted, derived from, reused, or mutated.
- Verification commands run: fresh raw-Git/public-REST authority readback,
  exact publication/delta/ancestry inspection, syntax checks, three safe
  argument refusals, the complete published direct-launcher `--check`, before/
  after suite and preservation hashes/counts, and `git diff --check`. Full Go
  tests were not rerun because inspection found no source or test delta and
  their exact published result is already admission evidence. Files changed:
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md` only.
- Result: **technical admission PASS; live authorization ABSENT**. The blocker
  is explicit human consent to the bounded effects above. What remains is
  independent controller review/publication of this exact three-file record
  and then a distinct human decision. `.agent/TASKS.md` remains exact at
  SHA-256 `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No live command is active.

## EXT-20 RC.7 Admission Publication And Human-Authorization Handoff (2026-07-25)

- Independent controller replay accepted exactly the final no-model admission
  recommendation's three durable-state files. Raw Git published them as
  commit `1a07077f24526b9202da55c2981911b7e0770a67`, tree
  `1cb2ed843d1d5cf294487e8e404a030bb9f9838c`, parent
  `b6351d108fc971dcfff5367267fb7eb1a3b00273`; local, fetched, and public
  `main` readback matched.
- Replay passed syntax, focused refusal/collision checks, complete sourced
  remote/bundle/suite/repository/task authority, `go test -count=1 ./...`, and
  diff hygiene. The clean published direct-launcher `--check` path also passed
  without a model call, launch record, or runtime mutation.
- RC.7 remains exact at content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`
  with ten pending zero-attempt tasks, zero operation/collector/model/receipt
  evidence, empty aggregate, and no launch-record root. Protected RC.6 suite,
  launch record, and terminal evidence remain exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Prepared executable continuation
  `agent-ext20-rc7-live-authorization-checkpoint.sh`, SHA-256
  `ca537cd5ef45afb23c31636c794653425e324ccfff2550532fbb0b7b83faa410`.
  It performs one attended no-model consent-boundary pass, records the eleven-
  operation/two-repository real-model scope requiring explicit human consent,
  and may update only the three durable-state files. It cannot supply or expose
  the live confirmation, invoke `--live`, create or edit executables, create a
  launch record, start Revolvr or a nested model, or grant any live/release/
  external-use/queue authority.
- The current operator direction authorizes verification, publication, and
  preparation of this safe continuation only; it does not authorize real model
  calls. `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No live command is active.

## EXT-20 RC.7 Final No-Model Live-Authority Admission Review (2026-07-25)

- Task selected: exactly one final no-model admission review under still-
  unchecked `EXT-20`. The prepared review wrapper was inspected but not
  executed because its continuation starts a nested Codex session; this pass
  performed the review directly under the supplied operator direction. No
  live confirmation was supplied, reproduced, or embedded, and no live path,
  launch record, Revolvr task operation, nested model, executable construction,
  tag, release, external-use approval, queue authority, or `EXT-20` completion
  was attempted.
- A fresh no-tags raw-Git fetch left an empty index and worktree. Local `main`,
  fetched `origin/main`, raw-Git public `main`, and public-REST `main` agreed at
  `b6351d108fc971dcfff5367267fb7eb1a3b00273`. Published pre-live review commit
  `f03258496621bbf8fd440a5c7293430a6ce44a22`, tree
  `39ac014ebdcd56482b931fd79073429e51419df3`, parent
  `460c2fa31155dca28a4f9ce861c03fbad8949acc` was exact and present in local
  and fetched-main ancestry. Its independently inspected delta changed only
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`, with clean
  diff hygiene and no admission discrepancy.
- Complete line-by-line inspection accepted the exact direct launcher and
  focused checker at SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`
  and `c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414`.
  Check-only exits immediately after the complete preflight and before launch-
  record reservation, pre-start/status writes, interruption traps, or the
  one-shot suite process. The focused checker's construction-only dirty-
  controller assertion was inspected rather than run against clean published
  main.
- `bash -n agent-ext20-rc7-live-direct.sh
  scripts/check-ext20-rc7-live-direct.sh` passed. Independent missing, wrong,
  and multiple-argument refusals from a temporary non-repository each returned
  status `64`, emitted zero stdout bytes, reported usage, and left no launch
  record. The only complete launcher path executed was
  `./agent-ext20-rc7-live-direct.sh --check`; it returned status `0` after the
  full published authority preflight and reported that no model call or launch
  record occurred.
- Before and after, raw Git retained the exact fourteen RC.1-through-RC.7 refs
  and no RC tags. Public REST retained exact source CI `30160277511` with ten
  successful jobs, attestation run/job `30163857880` / `89693466274`, sole
  unexpired artifact `8621008768`, and companion CI `30163853353` with ten
  successful jobs. Both sealed RC.7 candidate bundles, candidate source and
  binary identity, isolated Codex identity, guarded suite/collector scripts,
  repositories, configuration, doctor/source-writer results, and sentinels
  passed their exact authority checks.
- Prepared suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite` remained ID
  `ext20-14b2bf40212b` at authority/plan/content-stream SHA-256
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e` /
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1` /
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  It retained all ten tasks pending and zero-attempt, zero operation,
  collector, model-run, and receipt evidence, an empty aggregate, and no RC.7
  launch-record root.
- Read-only RC.6 suite, launch record, and terminal evidence remained exact
  before and after at content-stream SHA-256
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  No RC.1-through-RC.6 launcher or suite path was executed, repaired, deleted,
  derived from, reused, or mutated.
- Verification passed `go test -count=1 ./...` and `git diff --check`.
  `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. Files changed are `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md` only; blockers: none.
- Result and recommendation: **PASS for this final no-model admission review**.
  A later human/controller may separately decide whether to authorize a live
  invocation only after independently reviewing and publishing this exact
  three-file record. This review grants no live authority and leaves no live
  command active.

## EXT-20 RC.7 Pre-Live Publication And Final No-Model Review Handoff (2026-07-25)

- Independent controller replay accepted exactly the three durable-state files
  produced by the pre-live review. Raw Git published them as commit
  `f03258496621bbf8fd440a5c7293430a6ce44a22`, tree
  `39ac014ebdcd56482b931fd79073429e51419df3`, parent
  `460c2fa31155dca28a4f9ce861c03fbad8949acc`; local, fetched, and public
  `main` readback matched.
- Replay passed syntax, the focused refusal/collision checker, the complete
  sourced remote/bundle/suite/repository/task authority path,
  `go test -count=1 ./...`, and diff hygiene. On clean published `main`, the
  complete direct-launcher `--check` preflight also returned success without a
  model call, launch record, or runtime mutation.
- RC.7 remains exact at content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`
  with zero operation/collector/model/receipt evidence, empty aggregate, ten
  pending zero-attempt tasks, and no launch-record root. Protected RC.6 suite,
  launch record, and terminal evidence remain exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Prepared executable continuation
  `agent-ext20-rc7-live-authority-review.sh`, SHA-256
  `79d8aea3ec9f46d4ccee8bdcdd33aec09502e4d0e7ed65acef2b54dbd93e563f`.
  It admits one final attended no-model review and may update only
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md` with a
  recommendation. It forbids the live token, `--live`, launch-record creation,
  Revolvr/nested model execution, executable construction, tag/release/
  external-use/queue authority, and `EXT-20` completion.
- `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No live command is active.

## EXT-20 RC.7 Independent Pre-Live No-Model Review (2026-07-25)

- Task selected: exactly one bounded independent pre-live no-model review of
  the published RC.7 direct launcher under still-unchecked `EXT-20`. No live
  path, Revolvr process, nested Codex/model operation, launch record, tag,
  release, external-use approval, queue authority, or `EXT-20` completion was
  attempted or authorized.
- A fresh no-tags raw-Git fetch left clean local `main`, fetched
  `origin/main`, and public-REST `main` exact at
  `460c2fa31155dca28a4f9ce861c03fbad8949acc`, with an empty index and
  worktree. Published direct-launch record
  `d2e7d8ddda66db7685b021dcc33b94766ec00204`, tree
  `701639e624fc86a0a18c77cf19dc064f8e7bd511`, parent
  `6931b3ef790a6f0375944043596cb591cf589f2a`, is exact and present in both
  local and fetched-main ancestry.
- Complete line-by-line inspection accepted executable direct launcher
  `agent-ext20-rc7-live-direct.sh`, SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`,
  and executable focused checker `scripts/check-ext20-rc7-live-direct.sh`,
  SHA-256
  `c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414`.
  The check-only branch exits after the complete preflight and before every
  launch-record or suite-process action. The focused checker's final current-
  controller assertion is intentionally construction-time coverage requiring
  a dirty unpublished delta; it was inspected but not misclassified as a
  clean published-gate defect.
- `bash -n agent-ext20-rc7-live-direct.sh
  scripts/check-ext20-rc7-live-direct.sh` passed. Independent missing, wrong,
  and multiple-argument invocations from a temporary non-repository each
  returned status `64`, wrote zero stdout bytes, reported usage, supplied no
  live authority, and created no launch record.
- The only launcher path executed was
  `./agent-ext20-rc7-live-direct.sh --check`. It returned status `0` with
  `RC.7 live gate: complete preflight passed; no model call or launch record
  occurred`. That complete clean published-main preflight independently
  passed exact historical/current remote refs and absent RC tags; source CI,
  attestation run/job/artifact, and companion CI public REST evidence; both
  sealed bundles and guarded scripts; prepared-root authority; candidate and
  Codex identities; both repositories, all tasks and doctors, source-writer
  bounds, and sentinels; zero evidence and empty aggregate; absent launch-
  record root; unchanged backlog; and RC.6 preservation.
- Before and after all checks, prepared suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite` remained
  suite ID `ext20-14b2bf40212b`, authority/plan/content-stream SHA-256
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e` /
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1` /
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  It retained zero collector files, zero operation records, zero model-run
  entries, zero receipts, an empty aggregate, exactly ten pending zero-attempt
  doctor-ready tasks, and no RC.7 launch-record root.
- Read-only RC.6 suite
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite`, launch
  record
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365`,
  and terminal evidence
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1`
  remained exact before and after at content-stream SHA-256
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  No RC.1-through-RC.6 launcher or suite path was executed or mutated.
- Verification passed `go test -count=1 ./...` and `git diff --check`.
  `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- Files changed: `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md` only. Result: pass for this independent no-model review;
  blockers: none. What remains is a separate controller review and publication
  decision for this exact three-file record. No live command is active, and
  any later live-authority decision remains a distinct gate after publication.

## EXT-20 RC.7 Direct-Launcher Publication And Check-Only Readback (2026-07-25)

- Independent controller review accepted exactly the constructed launcher,
  focused checker, and three durable-state files. Raw Git published them as
  commit `d2e7d8ddda66db7685b021dcc33b94766ec00204`, tree
  `701639e624fc86a0a18c77cf19dc064f8e7bd511`, parent
  `6931b3ef790a6f0375944043596cb591cf589f2a`; local, fetched, and public
  `main` readback matched.
- Independent checks passed launcher/checker syntax, all focused refusal and
  isolated-collision coverage, direct sourced downstream authority checks,
  `go test -count=1 ./...`, and diff hygiene. Remote RC refs and absent RC
  tags, exact CI/attestation jobs and artifact, both sealed bundles, prepared
  suite/repository/task/doctor/source-writer/sentinel authority, and the
  unchanged task backlog all passed.
- On clean published `main`, `./agent-ext20-rc7-live-direct.sh --check`
  completed the entire fail-closed preflight and reported that no model call or
  launch record occurred. The RC.7 suite remains exact at content-stream
  SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`
  with zero operation/collector/model evidence, empty aggregate, ten pending
  zero-attempt tasks, and no launch-record root.
- Protected RC.6 suite, launch record, and terminal evidence remained exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed, repaired, deleted, derived from, or mutated.
- Prepared executable continuation `agent-ext20-rc7-prelive-review.sh`,
  SHA-256
  `f013427c57678950a17fc71a71e23a89e44fac30668d12d47971d6475c551b12`.
  It performs one fresh attended no-model pre-live review, requires exact
  clean published authority before starting, and forbids the live token,
  `--live`, launch-record creation, Revolvr/nested model calls, launcher or
  checker edits, tag/release/external-use/queue authority, and `EXT-20`
  completion. Its only admitted output is a durable-state-only review record
  for a later controller decision.
- `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No live command is active.

## EXT-20 RC.7 Direct-Live-Gate Construction (2026-07-25)

- Task selected: only the bounded construction and no-model checking sub-gate
  of still-unchecked `EXT-20`. No live confirmation was supplied and no live,
  Revolvr, nested Codex, or other model operation was started.
- Construction began only after a no-tags fetch left clean local `main`,
  fetched `origin/main`, and public remote `main` exact at
  `6931b3ef790a6f0375944043596cb591cf589f2a`. Published prepared-suite commit
  `83078e8467f00439956252955d5c130d51f34214`, tree
  `31e680996695e1bc71d38e1216a471250009fb0d`, and parent
  `29ca11e24f2cc8832615fe5274d79c151d1eb5c0` were exact and in both local and
  fetched-main ancestries.
- Added executable `agent-ext20-rc7-live-direct.sh`, SHA-256
  `9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45`.
  It accepts only `--check` or the exact retained live confirmation, rejects
  missing/wrong arguments before authority lookup, and binds the exact RC.7
  suite, candidate, Codex, bundle, script, remote ref, CI, attestation,
  artifact, repository, task, doctor, source-writer, sentinel, and RC.6
  preservation authorities. Check mode completes the preflight without a
  launch record or model call. A separately authorized future live invocation
  can reserve one new ignored launch-record root only after preflight, records
  pre-start authority plus terminal/interruption status, and has no retry or
  reuse path.
- Added executable `scripts/check-ext20-rc7-live-direct.sh`, SHA-256
  `c434380918c8ec726504126ac42b3a1338fa34741329ac60b28d44c03fc4f414`.
  It passed syntax, missing/wrong/multiple-argument refusal from a non-Git
  directory, definition loading without `main`, isolated existing-record
  collision refusal, dirty and clean-unpublished check-only refusal, and the
  construction repository's unpublished check-only refusal. It supplied no
  live token and created no RC.7 launch record.
- Before and after construction, both complete sealed RC.7 bundles passed
  their exact inventory/seal and candidate self-verification checks. Raw Git
  retained the exact fourteen RC.1-through-RC.7 refs and absent RC tags.
  Public REST retained exact source CI `30160277511`, its ten jobs, attestation
  run/job `30163857880` / `89693466274`, sole artifact `8621008768` named
  `level1-v0.1.0-rc.7-attestation` at digest
  `sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356`,
  and companion CI `30163853353` with its exact ten jobs.
- Direct read-only loading of the new launcher definitions exercised the full
  downstream preflight without calling `main`: both sealed bundles, exact
  remote evidence, suite/collector static checks, prepared authority and
  11-row plan, candidate/Codex identities, both clean repositories, ten unique
  pending zero-attempt tasks, every task's doctor-ready result, exact
  `timeout=32m0s heartbeat_interval=10m40s required=32m0s` source-writer
  authority, sentinels, zero operation/collector/model evidence, empty
  aggregate, and absent launch-record root all passed.
- Verification passed `bash -n agent-ext20-rc7-live-direct.sh
  scripts/check-ext20-rc7-live-direct.sh`,
  `bash scripts/check-ext20-rc7-live-direct.sh`, the sourced downstream
  read-only preflight, `go test -count=1 ./...`, and `git diff --check`.
- The prepared RC.7 suite remained byte-exact at content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  It retains no operation or collector manifest, no model run or receipt, an
  empty aggregate, only its two sealed npm-preparation logs, and no RC.7
  launch-record root. The protected RC.6 suite, launch record, and terminal
  evidence remained exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed, repaired, deleted, derived from, or mutated.
- Files changed: `agent-ext20-rc7-live-direct.sh`,
  `scripts/check-ext20-rc7-live-direct.sh`, this state file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. `.agent/TASKS.md` remains
  byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- Result: pass for construction and no-model checks; blockers: none. What
  remains is a fresh independent no-model review of the exact five-file delta,
  repeat verification of every authority and preservation claim, and
  controller publication only if that review passes. This pass grants no live
  command, launch record, model operation, tag, release, external-use
  approval, queue authority, or `EXT-20` completion.

## EXT-20 RC.7 Prepared-Suite Review And Live-Gate Construction Handoff (2026-07-25)

- Independent review accepted the exact four-file suite-preparation scope.
  The suite driver diff contains only RC.7 candidate source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, Linux SHA-256
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  and bundle-path substitutions; every other suite byte is unchanged.
- Prepared suite `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite`
  independently reverified at suite ID `ext20-14b2bf40212b`, authority/plan/
  content-stream SHA-256
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e` /
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1` /
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  It is the only persistent RC.7 parent. Exact candidate/Codex identities,
  clean repository heads, configs, sentinels, 11-row plan, ten unique pending
  zero-attempt tasks, and all ten doctor-ready results passed.
- Independent no-model checks passed both repository `config check` commands,
  exact `timeout=32m0s heartbeat_interval=10m40s required=32m0s` doctor
  authority, suite/collector syntax, suite `--static`, both retained fixture
  manifest validations and hashes, and expected `--verify-suite` refusal at
  missing first manifest. RC.7 suite content was byte-identical before and
  after; it retains zero evidence/manifests, an empty aggregate, and no launch
  record. `go test -count=1 ./...` and `git diff --check` passed.
- Both sealed RC.7 bundles and candidate self-verification passed. Raw Git and
  public REST kept candidate/attestation refs, exact runs/jobs/artifact, all
  historical refs, and absent RC tags exact. `.agent/TASKS.md` remains SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- The RC.6 terminal manifest passed read-only verification and its protected
  suite/launch-record/terminal-evidence streams remained
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed or mutated.
- Raw Git published the accepted prepared-suite record as commit
  `83078e8467f00439956252955d5c130d51f34214`, tree
  `31e680996695e1bc71d38e1216a471250009fb0d`, parent
  `29ca11e24f2cc8832615fe5274d79c151d1eb5c0`; exact remote `main` readback
  passed.
- New attended launcher `agent-ext20-rc7-live-gate.sh`, SHA-256
  `655e636894b22569feb934c53011582095d498fd0f80af410e2c7e83a7266f12`,
  is construction-only. It may create a fail-closed direct launcher and
  focused no-model checker, but cannot invoke `--live`, use confirmation,
  create launch evidence, start a Revolvr/nested model operation, or make a
  live command active. Its output requires fresh independent review.

## EXT-20 RC.7 Guarded No-Model Suite Preparation (2026-07-25)

- Task selected: only the bounded RC.7 no-model suite-preparation sub-gate of
  still-unchecked `EXT-20`. The guarded Level-1 suite now binds exact candidate
  source `f63cbe3989cb281652cf4eec3f92614fec98294d`, Linux artifact SHA-256
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb`. Those are the
  only three suite constants changed. Release output remains `revolvr 0.1.0`;
  exact Codex package/output/SHA-256 remains `0.144.4` / `codex-cli 0.144.4` /
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  Model `gpt-5.6-sol`, reasoning effort `xhigh`, plan, schemas, scenarios,
  thresholds, configuration, confirmation guard, and collector behavior are
  byte-unchanged.
- A fresh no-tags fetch left clean local `main`, `origin/main`, and remote
  `main` exact at `29ca11e24f2cc8832615fe5274d79c151d1eb5c0` with published
  remote-attestation review record `c17a8f08efe1fedb1edcdc5f98b6d03ebc0e5a3c`
  in both local and fetched-main ancestry. Before and after preparation, raw
  Git and public REST reverified exact candidate/attestation refs, source CI
  run `30160277511`, dedicated run/job `30163857880` / `89693466274`, sole
  unexpired artifact `8621008768` with its exact name and digest, and companion
  ten-job CI run `30163853353`. All twelve historical RC.1-through-RC.6 refs
  remained exact and all RC tags remained absent.
- Both complete sealed RC.7 bundles passed before and after preparation,
  including candidate inventory/seal
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`,
  verification inventory/seal
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`,
  candidate self-verification, exact source/tree, build instructions, and all
  three platform artifact hashes.
- Verification passed `bash -n` for suite and collector, exact three-constant
  transformed-file comparison, suite `--static`, two collector
  `--fixture-only` runs below the single RC.7 parent, both
  `--verify-manifest` checks, byte-identical raw manifests, and canonical
  comparison excluding only `collected_at_utc`. Raw fixture manifests are
  both SHA-256
  `2459a5c16bea4c1d9c66564fc68ca7e4c373226a4571f8fa7285e72850ab41cb`;
  canonical manifests are both
  `c907b7ef62d47b8a4080c437caa0271c73fcdfb7e77c6013de1eb15ebb08c5dc`.
  `go test -count=1 ./...` and `git diff --check` passed.
- Exactly one collision-free persistent parent was created with the required
  ignored-state template. The prepared suite is
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite`, suite ID
  `ext20-14b2bf40212b`, authority SHA-256
  `c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e`,
  unchanged plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and relative-path-bearing null-sorted regular-file content-stream SHA-256
  `2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943`.
  The suite has 268 regular files. Repo-a is clean on `main` at
  `22bc5fd5ea1469fb76afef6425964f0b0c7f70bb`; repo-b is clean on `main` at
  `f92954597d8bd35372ee181c959be9a5fc637429`.
- Independent inspection passed the prepared authority checksum; exact
  candidate/Codex paths, hashes, versions, and candidate clean-VCS source;
  unchanged model/reasoning/bounds/configuration; effective source-writer
  authority `timeout=32m0s heartbeat_interval=10m40s required=32m0s`; exact
  11-row plan; ten unique pending doctor-ready tasks; both tracked disposable
  markers; both clean repositories; and intact sentinel bytes, modes, hard
  links, and relative symlinks. The suite retains zero operation manifests,
  zero collector manifests, empty run/receipt and aggregate directories, and
  no launch record.
- The required no-live invocation
  `scripts/dogfood-external-level1-suite.sh --verify-suite --run-root <suite>`
  correctly failed closed at absent first-operation manifest
  `ext20-14b2bf40212b-01`. A successful post-live suite report is impossible
  for an intentionally unstarted suite with ten pending tasks; no evidence was
  manufactured and no unrelated verifier behavior was changed. The prepared
  content and empty aggregate remained exact after this expected refusal.
- The retained RC.6 terminal manifest passed before and after work. RC.6 suite,
  launch-record, and terminal-evidence streams remain byte-exact at
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed, repaired, reconciled, relabeled, or mutated.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this state file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No Go source, dependency,
  workflow, bundle, historical evidence, or live launcher changed.
  `.agent/TASKS.md` remains byte-exact at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- Result: pass for the bounded no-model preparation gate; blockers: none. No
  live/Revolvr/nested-model operation, launch record, live launcher,
  confirmation use, commit, push, tag, release, external-use approval, queue
  authority, or `EXT-20` completion occurred. What remains is a fresh
  independent no-model review of the exact four-file tracked delta and the
  retained prepared root, followed by controller publication only if every
  authority and preservation check passes. Any live-gate decision remains a
  later separately authorized pass.

## EXT-20 RC.7 Remote-Attestation Review And Suite Handoff (2026-07-25)

- Independent raw-Git readback accepted exact candidate ref
  `f63cbe3989cb281652cf4eec3f92614fec98294d` and exact attestation ref
  `3cc6d527f889c7b933828fbd832d07b5291aee79`. They remain the only two RC.7
  refs; all RC.7 tags remain absent. Reviewed workflow commit/tree/parent and
  workflow SHA-256 remain exact and published ancestors of `main`.
- Public REST independently verified dedicated attestation run
  `30163857880`, sole job `89693466274`, and sole unexpired artifact
  `8621008768` as exact completed/successful authority at the attestation head.
  The artifact remains `70275600` bytes with digest
  `sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356`.
  Exactly one matching dedicated workflow run exists. Unauthenticated archive
  access still returns HTTP 401, so no remote archive-byte claim is added.
- Public REST independently verified companion CI run `30163853353` as exact
  completed/successful push authority at the attestation head, with exactly
  the ten required unique successful jobs and IDs already retained below.
  The attestation branch has exactly these dedicated and companion push runs
  at its exact head.
- Both complete RC.7 sealed bundles and candidate self-verification passed.
  The workflow structure, exact retained embedded shell, retained 29-file
  checksum set, three byte-identical target pairs, six empty build IDs, and
  path-bearing stream SHA-256
  `5c56018487239868a8a5af83b14b37179b81e307d09956650085042be8ad31e0`
  reverified. All twelve historical RC refs remained exact and RC tags absent.
- The RC.6 terminal manifest passed read-only verification. Protected suite,
  launch-record, and terminal-evidence content-stream SHA-256 values remained
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  RC.6 was not executed, repaired, deleted, or mutated.
- Raw Git published only the accepted three-file remote-attestation record as
  commit `c17a8f08efe1fedb1edcdc5f98b6d03ebc0e5a3c`, tree
  `5d8580dc2b7a847c8d4ec83791781d3568b23296`, parent
  `c412c4fca47b655fc0e3aa48bafafdaa8c464d20`; exact remote `main` readback
  passed. `.agent/TASKS.md` remains unchanged at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- New attended launcher `agent-ext20-rc7-suite.sh`, SHA-256
  `2ca635a3015f2bd99f7c36bb2ac336e58b3ef5872d3b29525229ea58e625bcf2`,
  is the next bounded no-model gate. It may change only the guarded suite's
  exact candidate source/hash/bundle constants and create exactly one fresh
  persistent RC.7 suite root. It forbids `--live`, every model operation,
  launch-record/live-launcher creation, RC.6 reuse or mutation, tag, release,
  external-use approval, queue authority, and `EXT-20` completion.

## EXT-20 RC.7 Remote Artifact Attestation (2026-07-25)

- Task selected: the single bounded RC.7 remote artifact-attestation sub-gate
  of still-unchecked `EXT-20`. A fresh no-tags fetch left clean local `main`,
  `origin/main`, and remote `main` exact at
  `c412c4fca47b655fc0e3aa48bafafdaa8c464d20`. Reviewed workflow commit
  `3cc6d527f889c7b933828fbd832d07b5291aee79`, tree
  `1a35d15e75ce372d02f9499bd9634ca0f808f68d`, parent
  `b4c63dde894fec5805cd200164761c8f6e05b449`, and exact candidate source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, are published ancestors.
- Complete prepublication verification passed both RC.7 sealed bundles,
  candidate self-verification, exact build instructions and Linux/Darwin/
  FreeBSD artifact hashes, exact candidate ref and successful ten-job source
  CI run `30160277511`, PyYAML workflow structure, extracted embedded-shell
  syntax and hash, and the retained 29-file local result. Candidate inventory/
  seal remained
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`;
  verification inventory/seal remained
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
- Immediately before publication, the attestation ref was absent locally and
  remotely, every `*rc.7*` tag was absent, exact candidate ref
  `refs/heads/level1-v0.1.0-rc.7` was the sole RC.7 ref, and public REST
  returned zero artifacts and zero prior push runs in the attestation
  namespace. At `2026-07-25T15:33:24Z`, raw Git used the required empty-
  expected lease to create only
  `refs/heads/level1-v0.1.0-rc.7-attestation` at
  `3cc6d527f889c7b933828fbd832d07b5291aee79`. Exact remote readback passed at
  `2026-07-25T15:33:25Z`; no local branch was created and no existing ref was
  moved or deleted.
- Dedicated run `30163857880`, workflow ID `320320555`, run number/attempt
  `1`/`1`, name `Level 1 RC.7 candidate attestation`, path
  `.github/workflows/level1-rc7-candidate-attestation.yml`, event `push`,
  branch `level1-v0.1.0-rc.7-attestation`, and exact workflow head completed
  successfully. It was created/started `2026-07-25T15:33:34Z` and updated
  `2026-07-25T15:36:20Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30163857880`.
- Its sole job `89693466274`, `Rebuild and attest Level 1 RC.7 candidate`,
  reported the exact head and completed successfully from
  `2026-07-25T15:33:36Z` through `2026-07-25T15:36:19Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30163857880/job/89693466274`.
  Its sole run artifact is unexpired ID `8621008768`, name
  `level1-v0.1.0-rc.7-attestation`, size `70275600`, digest
  `sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356`,
  created/updated `2026-07-25T15:36:17Z`, and expiring
  `2026-10-23T15:33:35Z`. Archive endpoint:
  `https://api.github.com/repos/ponchione/revolvr/actions/artifacts/8621008768/zip`.
  No read-only token was configured; the unauthenticated endpoint returned
  HTTP 401, so this pass makes no controller-side archive-byte claim.
- Companion CI run `30163853353`, workflow ID `313588206`, run number/attempt
  `85`/`1`, exact `.github/workflows/ci.yml` push branch/head, completed
  successfully. It was created/started `2026-07-25T15:33:26Z` and updated
  `2026-07-25T15:36:50Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30163853353`. Its exact
  unique successful jobs, all at workflow head
  `3cc6d527f889c7b933828fbd832d07b5291aee79`, are:
  - `89693455164` — Go 1.22 source floor and tests —
    `2026-07-25T15:33:28Z` to `2026-07-25T15:35:14Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455164`
  - `89693455158` — Production autonomous strict-fake suite —
    `2026-07-25T15:33:28Z` to `2026-07-25T15:34:21Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455158`
  - `89693455167` — Race tests — `2026-07-25T15:33:28Z` to
    `2026-07-25T15:36:49Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455167`
  - `89693455176` — Vet and module verification —
    `2026-07-25T15:33:29Z` to `2026-07-25T15:34:15Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455176`
  - `89693455194` — Fake-Codex success smoke —
    `2026-07-25T15:33:28Z` to `2026-07-25T15:34:06Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455194`
  - `89693455193` — Fake-Codex verification-failure smoke —
    `2026-07-25T15:33:29Z` to `2026-07-25T15:34:11Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455193`
  - `89693455190` — Build linux/amd64 — `2026-07-25T15:33:29Z` to
    `2026-07-25T15:34:09Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455190`
  - `89693455186` — Build darwin/amd64 — `2026-07-25T15:33:29Z` to
    `2026-07-25T15:34:10Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455186`
  - `89693455183` — Build freebsd/amd64 — `2026-07-25T15:33:28Z` to
    `2026-07-25T15:34:02Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455183`
  - `89693455189` — Build Windows diagnostic stub —
    `2026-07-25T15:33:29Z` to `2026-07-25T15:33:47Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30163853353/job/89693455189`
- Post-publication preservation reverified both complete RC.7 bundles and
  candidate self-verification, workflow SHA-256
  `f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13`,
  the retained local 29-file stream
  `5c56018487239868a8a5af83b14b37179b81e307d09956650085042be8ad31e0`,
  exact current and all RC.1-through-RC.6 historical refs, and absent RC tags.
  The RC.6 terminal manifest passed before and after; protected suite, launch-
  record, and terminal-evidence streams remained
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Files changed: `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md`. No Go source or dependency changed. Verification used
  raw-Git no-tags fetch, exact ancestry/ref/collision/readback checks, complete
  bundle manifests and self-verification, PyYAML and `bash -n`, retained local
  artifact authority, public REST run/job/artifact queries and finite polling,
  RC.6 manifest/content streams, historical refs/tags, and `git diff --check`.
  Result: pass, remote-attestation sub-gate only. Blockers: none.
  `.agent/TASKS.md` remains byte-for-byte unchanged at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`,
  with `EXT-20` unchecked.
- What remains: a fresh independent no-model controller pass must reverify the
  exact remote ref/run/job/artifact and companion-CI evidence plus this exact
  three-file durable-state delta. Only after controller publication may a
  separate collision-free RC.7 suite-preparation gate be authored and
  reviewed. No suite, Revolvr/model operation, tag, release, external-use
  approval, queue authority, or `EXT-20` completion is authorized here.

## EXT-20 RC.7 Attestation-Workflow Review And Publication (2026-07-25)

- Independent review accepted exactly the new RC.7 attestation workflow and
  its three durable-state records. PyYAML structure and exact trigger/action/
  checkout/upload constants passed; the parsed embedded shell was byte-exact
  with retained `embedded-shell.sh`, passed `bash -n`, and remained SHA-256
  `3882c50d361ff95f0b49bc85c5ce44aceafd4ca5d632390dd4adfd686abaf646`.
  The complete workflow remains SHA-256
  `f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13`.
- Retained output
  `/tmp/revolvr-ext20-rc7-attestation.Uq3syS/runner-temp/level1-v0.1.0-rc.7-attestation`
  independently reverified as exactly 29 regular files with path-bearing
  content-stream SHA-256
  `5c56018487239868a8a5af83b14b37179b81e307d09956650085042be8ad31e0`.
  All six artifact checksum rows passed, all three target pairs remained byte-
  identical, and all six build-ID files remained empty.
- Both complete RC.7 bundle inventories and seals passed, as did candidate
  self-verification. Candidate/verification inventory/seal hashes remained
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`
  and
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
  Exact Linux/Darwin/FreeBSD hashes and build-instructions authority remained
  unchanged.
- Raw Git reverified exact candidate ref
  `f63cbe3989cb281652cf4eec3f92614fec98294d` and all historical RC refs.
  Public REST reverified candidate CI run `30160277511` as exact completed/
  successful push run with its ten exact unique completed/successful jobs.
  RC.7 attestation ref/tags/workflow-on-remote/artifact-name collisions were
  absent before publication. `.agent/TASKS.md` remained unchanged at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked.
- The retained RC.6 terminal manifest passed read-only verification. Protected
  suite, launch-record, and terminal-evidence content-stream hashes remained
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
  Only operation `ext20-7b4a5932090f-01` remains retained; RC.6 was not
  executed or mutated.
- Raw Git published only the accepted four-file workflow record as commit
  `3cc6d527f889c7b933828fbd832d07b5291aee79`, tree
  `1a35d15e75ce372d02f9499bd9634ca0f808f68d`, parent
  `b4c63dde894fec5805cd200164761c8f6e05b449`. Exact remote `main` readback
  passed. No attestation ref/run/artifact, suite, model operation, tag,
  release, external-use approval, queue authority, or completion occurred.
- New attended launcher `agent-ext20-rc7-attestation-remote.sh` is the next
  bounded gate. It may create only the collision-free attestation ref at exact
  workflow commit `3cc6d527f889c7b933828fbd832d07b5291aee79`, then collect its dedicated
  run/artifact and companion-CI evidence. It cannot prepare a suite or start a
  live or nested model operation.

## EXT-20 RC.7 Local Artifact-Attestation Workflow (2026-07-25)

- Task selected: the bounded local RC.7 artifact-attestation workflow sub-gate
  of still-unchecked `EXT-20`. Added only the separate workflow
  `.github/workflows/level1-rc7-candidate-attestation.yml`, triggered solely by
  a push to `level1-v0.1.0-rc.7-attestation`. Its SHA-256 is
  `f3e06992e72029d80162c9b5901c398dbfb3c79cfeae43c0e72ddd28cff4ee13`.
  Trigger HEAD is workflow authority only; checkout remains exact candidate
  source `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`.
- The workflow installs exact Go 1.26.5 with action caching disabled. Two
  independent clean `--no-local` clones use distinct build and module caches
  and build Linux, Darwin, and FreeBSD amd64 with module-readonly/local-
  toolchain mode, disabled CGO, trimpath, explicit clean VCS metadata, empty
  build IDs, and exact `main.version=0.1.0`. Both build sets are retained in
  one future artifact named `level1-v0.1.0-rc.7-attestation` with exact hashes,
  build metadata, version assertions, reproducibility evidence, and authority
  manifest.
- Local YAML verification passed PyYAML structure plus exact trigger, action,
  environment, checkout, and upload assertions. Exact-constant and historical-
  leak scans, extracted-shell `bash -n`, and `git diff --check` passed. The
  exact parsed embedded shell has SHA-256
  `3882c50d361ff95f0b49bc85c5ce44aceafd4ca5d632390dd4adfd686abaf646`.
- The complete unmodified embedded shell executed successfully from fresh
  detached exact-source clone
  `/tmp/revolvr-ext20-rc7-attestation.Uq3syS/source`. Its two inner clean
  clones remained detached at the exact candidate source/tree and used four
  distinct `GOCACHE`/`GOMODCACHE` paths. All three build pairs were byte-
  identical and matched exact Linux/Darwin/FreeBSD SHA-256 values
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  `fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086`,
  and `13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe`.
  Every binary passed exact Go/tool/path/compiler/trimpath/target/CGO/source/
  clean-VCS metadata, empty-build-ID, and `main.version` authority checks.
- Retained output beneath
  `/tmp/revolvr-ext20-rc7-attestation.Uq3syS/runner-temp/level1-v0.1.0-rc.7-attestation`
  contains exactly 29 regular single-link files. Its path-bearing content-
  stream SHA-256 is
  `5c56018487239868a8a5af83b14b37179b81e307d09956650085042be8ad31e0`.
  It has six exact artifact hash rows, three identical reproducibility rows,
  six build-metadata files, six empty build-ID files, six version-authority
  files, two exact Linux version outputs, and one exact authority manifest.
- Pre/post raw-Git and public-REST checks kept local/remote `main` exact at
  `b4c63dde894fec5805cd200164761c8f6e05b449` with published review record
  `a0f5b069ea5189b11f1d4de71c93e825814adee5` in ancestry. Candidate ref,
  source/tree, successful run `30160277511`, and its ten exact successful jobs
  remain exact. The candidate is still the only RC.7 ref; the remote workflow
  path, attestation ref, RC.7 tag namespace, and candidate/attestation artifact
  names remain collision-free.
- Both complete RC.7 bundles reverified at candidate inventory/seal
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`
  and verification inventory/seal
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
  Build-instructions SHA-256 remained
  `ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd`.
  Fourteen historical sealed bundles, all 16 RC.1-through-RC.6 candidate roots,
  five prior workflows, 13 remote historical/candidate refs, and absent RC
  tags were unchanged before and after execution.
- The RC.6 terminal manifest reverified without executing any retired path.
  Protected suite, launch-record, and terminal-evidence content-stream hashes
  remain respectively
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Files changed: `.github/workflows/level1-rc7-candidate-attestation.yml`,
  `.agent/STATE.md`, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No Go
  source or dependency changed, so no Go test was required. Verification used
  PyYAML structural assertions, exact constant/action scans, `bash -n`, full
  local execution of the exact embedded shell, artifact/hash/metadata/build-ID/
  version/reproducibility assertions, raw Git and public GitHub REST, complete
  sealed-bundle inventories, historical content comparisons, RC.6 manifest and
  content-stream checks, and `git diff --check`.
- Result: pass, local-only attestation-workflow gate. Blockers: none.
  `.agent/TASKS.md` remains byte-for-byte unchanged at SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. No commit, push, attestation ref, remote workflow
  run/artifact, suite, live/nested model operation, tag, release, external-use
  approval, queue authority, or `EXT-20` completion occurred.
- What remains: a fresh independent pass must review the exact four-file
  worktree scope, workflow structure/constants/shell, retained local result,
  sealed bundles, remote candidate/CI authority, collision state, and all
  preservation evidence. Only after acceptance may a separately authorized
  controller publish the reviewed workflow/state scope; attestation-ref and
  remote-run authority remain later separate gates.

## EXT-20 RC.7 Remote-CI Review And Attestation Handoff (2026-07-25)

- Independent raw-Git readback accepted only exact candidate ref
  `refs/heads/level1-v0.1.0-rc.7` at
  `f63cbe3989cb281652cf4eec3f92614fec98294d`; no local candidate branch,
  attestation ref, or RC.7 tag exists.
- Public GitHub REST independently verified run `30160277511` as run number
  `80`, attempt `1`, push-triggered from the exact RC.7 branch/head, completed
  successfully. Its jobs endpoint returned exactly the ten required unique
  jobs; every job has the exact head and completed successfully. Exact
  candidate and attestation artifact-name queries both returned zero.
- Both complete sealed RC.7 bundles and candidate self-verification passed.
  Historical refs remain exact, RC tags and an RC.7 workflow remain absent,
  and `.agent/TASKS.md` remains unchanged with EXT-20 unchecked. The RC.6
  terminal manifest and protected suite/launch-record/terminal-evidence hashes
  reverified exactly.
- Raw Git published only the accepted three durable-state files as commit
  `a0f5b069ea5189b11f1d4de71c93e825814adee5`, tree
  `46bbdfa195d6f2f45dbb0955328bca7d89c0baf0`. No attestation workflow/ref,
  remote artifact, suite, live Revolvr operation, tag, release, external-use
  approval, queue authority, or EXT-20 completion occurred during review.
- New launcher `agent-ext20-rc7-attestation.sh` is the next local-only gate. It
  may add and fully exercise only the collision-free exact-source Go 1.26.5
  attestation workflow. It grants no commit, push, remote run/artifact, suite,
  live-model, tag, release, external-use, queue, or completion authority.

## EXT-20 RC.7 Exact-Candidate Remote CI (2026-07-25)

- Task selected: the single bounded candidate-ref and remote-CI sub-gate of
  still-unchecked `EXT-20`. No-tags fetch placed exact local `main`,
  `origin/main`, and remote `main` at
  `4bd712502a0a5fd27b80587b1ae38c916d92d390`; published local-candidate
  record `1055d7fc8ccdb844b5fe1405674133a244a1be64` is an ancestor, and exact
  source `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, is published and reachable.
- Independent prepublication verification passed both complete sealed RC.7
  bundles, exact candidate/release/source/tree/build authority, all three
  artifact hashes, clean bundle topology, and candidate self-verification.
  Candidate inventory/seal remained
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`;
  verification inventory/seal remained
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
  Build-instructions SHA-256 remained
  `ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd`;
  Linux/Darwin/FreeBSD artifact SHA-256 values remained
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  `fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086`,
  and `13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe`.
- Immediately before publication, the candidate and attestation refs were
  absent locally and remotely; all RC.7 tags were absent; no RC.7 workflow
  existed; and public GitHub REST returned zero exact candidate or attestation
  artifact-name collisions. Raw Git used the required empty-expected lease to
  create only `refs/heads/level1-v0.1.0-rc.7`. Exact remote readback is
  `f63cbe3989cb281652cf4eec3f92614fec98294d`; no local branch was created.
- Public GitHub REST identified exactly one matching push-triggered CI run:
  run `30160277511`, run number `80`, attempt `1`, event `push`, branch
  `level1-v0.1.0-rc.7`, head SHA
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, status `completed`, conclusion
  `success`, created `2026-07-25T13:43:27Z`, updated
  `2026-07-25T13:46:44Z`:
  `https://github.com/ponchione/revolvr/actions/runs/30160277511`.
- The run has exactly the ten mandatory unique jobs. Every job reports exact
  head `f63cbe3989cb281652cf4eec3f92614fec98294d`, status `completed`, and
  conclusion `success`:
  - `89684369461` — Go 1.22 source floor and tests —
    `2026-07-25T13:43:29Z` to `2026-07-25T13:45:07Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369461`
  - `89684369485` — Production autonomous strict-fake suite —
    `2026-07-25T13:43:29Z` to `2026-07-25T13:44:23Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369485`
  - `89684369426` — Race tests — `2026-07-25T13:43:30Z` to
    `2026-07-25T13:46:43Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369426`
  - `89684369423` — Vet and module verification —
    `2026-07-25T13:43:30Z` to `2026-07-25T13:44:07Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369423`
  - `89684369490` — Fake-Codex success smoke —
    `2026-07-25T13:43:29Z` to `2026-07-25T13:44:10Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369490`
  - `89684369458` — Fake-Codex verification-failure smoke —
    `2026-07-25T13:43:29Z` to `2026-07-25T13:44:10Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369458`
  - `89684369418` — Build linux/amd64 — `2026-07-25T13:43:29Z` to
    `2026-07-25T13:44:04Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369418`
  - `89684369432` — Build darwin/amd64 — `2026-07-25T13:43:30Z` to
    `2026-07-25T13:44:08Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369432`
  - `89684369443` — Build freebsd/amd64 — `2026-07-25T13:43:30Z` to
    `2026-07-25T13:44:10Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369443`
  - `89684369447` — Build Windows diagnostic stub —
    `2026-07-25T13:43:29Z` to `2026-07-25T13:43:44Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30160277511/job/89684369447`
- Post-CI preservation reverified both complete RC.7 bundles, candidate
  self-verification, the RC.6 terminal manifest, every RC.1-through-RC.6
  remote ref, absent historical/RC.7 tags, and absent RC.7 attestation ref,
  workflow, and artifact names. RC.6 suite, launch-record, and terminal-
  evidence content-stream SHA-256 values remained respectively
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Files changed: `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md`. Verification used raw-Git no-tags fetch, ancestry and
  exact-ref checks, complete bundle manifests and candidate self-verification,
  RC.6 retained-manifest/content-stream checks, public REST collision/run/job
  queries, finite run polling, and `git diff --check`. Result: pass, remote-CI
  sub-gate only. Blockers: none. `.agent/TASKS.md` remains unchanged at
  SHA-256 `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`
  with `EXT-20` unchecked. What remains is a separately bounded local-only
  exact-checkout Go 1.26.5 artifact-attestation workflow construction and
  verification gate; no attestation ref or remote artifact is yet authorized.

## EXT-20 RC.7 Independent Local Review And Publication (2026-07-25)

- Independent review accepted exact source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, and the complete sealed RC.7
  candidate and verification bundles. Both inventories, complete file sets,
  build metadata, artifact hashes, reproducibility rows, empty build IDs,
  embedded version, and clean detached source authority passed.
- The reviewer reran the adjacent planner-provenance packages and both full Go
  suites independently with Go 1.26.5 and the Go 1.22.12 source floor. Vet and
  module verification passed. The sealed vulnerability evidence reports zero
  reachable/imported-package vulnerabilities and retains only documented
  Windows-only module finding `GO-2026-5024` as not called.
- The RC.6 terminal manifest reverified and its suite, launch record, and
  terminal evidence remained exact at content-stream SHA-256 values
  `d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`,
  `2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`,
  and `e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`.
- Raw Git published only the accepted three durable-state files as commit
  `1055d7fc8ccdb844b5fe1405674133a244a1be64`, tree
  `96c7d1df60409cc7e6b9be24f03f4c17c83ecbe7`. Ignored candidate evidence
  remains local. No candidate ref, remote CI, attestation, suite, live Revolvr
  operation, tag, release, external-use approval, queue authority, or EXT-20
  completion occurred during review.
- New launcher `agent-ext20-rc7-remote.sh` is the next bounded gate. It may
  create only absent `refs/heads/level1-v0.1.0-rc.7` at the exact candidate
  source with an empty-expected raw-Git lease, then require the exact
  push-triggered ten-job CI matrix. Blockers: none.

## EXT-20 RC.7 Local Candidate Construction (2026-07-25)

- Task selected: the single bounded local-candidate sub-gate of still-unchecked
  `EXT-20`. Fresh collision-free candidate `level1-v0.1.0-rc.7` was built only
  from exact published remediation source
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`. Raw Git readback placed
  source and controller HEAD `6c932653f9ea79892569c71aa4e05e69947eaaa5`
  on `origin/main`; the source is reachable, and the `cmd`, `internal`,
  `go.mod`, and `go.sum` diff from source through controller HEAD is empty.
  The controller launcher is absent from the detached candidate source.
- Pre-construction checks found no local or remote RC.7 candidate/attestation
  ref, tag, workflow, artifact, candidate bundle, verification bundle, build
  root, suite root, launch record, or diagnostic. The public artifact queries
  returned zero exact-name collisions. No existing output was overwritten or
  reused.
- Candidate bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb`. Its 15
  regular files include 13 inventoried payload files. Inventory/seal SHA-256
  values are
  `7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba` /
  `2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0`.
  Build-instructions SHA-256 is
  `ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd`.
- Linux, Darwin, and FreeBSD amd64 artifact SHA-256 values are respectively
  `1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a`,
  `fb3ecdc5a6c9199b4c4f28e9b5d3babeaa54d645551b88e09b7dcf1969b6a086`,
  and `13859c85ebf7d08aca5139625298bf90e2b4a4770976edaa710864cc077729fe`.
  Exact Go 1.26.5 built every target twice from independent fresh non-local
  clean clones with version `0.1.0`, module-readonly/local-toolchain mode,
  disabled CGO, amd64, trimpath, explicit clean VCS metadata, and empty Go
  build IDs. Every pair is byte-identical; embedded metadata and output
  `revolvr 0.1.0` pass.
- Separate verification bundle:
  `.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb-verification`.
  Its 54 regular files include 52 inventoried payload files. Inventory/seal
  SHA-256 values are
  `ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733` /
  `6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b`.
  Both bundles passed their complete sorted regular-file manifests after final
  placement; neither contains a symlink or multiply linked regular file.
- Verification passed on the detached exact source: full adjacent
  `internal/prompt`, `internal/autonomouscycle`,
  `internal/autonomousauditapply`, and `internal/autonomousplanapply` tests in
  ordinary and race modes, including the normalized planner-profile final-
  newline acceptance and meaningful-tamper rejection; the recursive Structured
  Outputs compatibility guard; focused lifecycle-routing authority tests in
  ordinary and race modes; production autonomous happy path and strict-fake
  Codex contract in ordinary and race modes; Go 1.22.12 source-floor full
  suite; Go 1.26.5 full suite; `go vet ./...`; and `go mod verify`.
- `govulncheck@v1.1.4` against the 2026-07-24 database reports zero reachable
  and zero imported-package vulnerabilities. The sole module-only finding is
  unchanged `GO-2026-5024` in `golang.org/x/sys@v0.30.0`, Windows-only and not
  called, fixed in v0.44.0. No dependency was changed.
- The RC.6 terminal manifest verified before and after work. Pre/post
  relative-path-bearing regular-file content-stream SHA-256 values match for
  the protected suite
  (`d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b`),
  launch record
  (`2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce`),
  and terminal evidence
  (`e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`).
  All sealed RC.1-through-RC.6 bundles reverified, historical remote refs
  remained exact, and historical tags remained absent.
- Files changed: ignored builder
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.7.sh`, the two ignored
  immutable RC.7 bundle trees above, `.agent/STATE.md`,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. Retained build root is
  `/tmp/revolvr-ext20-rc7-build.N1aDxk`. `.agent/TASKS.md` is unchanged at
  SHA-256
  `33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448`,
  with `EXT-20` unchecked.
- Result: pass, local-candidate-only. No ref, remote CI, attestation workflow,
  suite, live/model operation, confirmation token, tag, release, external-use
  approval, queue authority, commit, push, or `EXT-20` completion occurred.
  Blockers: none. What remains is a fresh independent read-only review of the
  exact bundles, tracked three-file state scope, source authority, verification
  evidence, and RC.6 preservation before any later controller publication.

## EXT-20 RC.7 Local-Candidate Continuation Prepared (2026-07-25)

- Planner-profile remediation commit
  `f63cbe3989cb281652cf4eec3f92614fec98294d`, tree
  `43fc099d966cd6c06a74f00272c945fe3ca0a0f9`, is published on `origin/main`.
- New attended-shell launcher `agent-ext20-rc7.sh` prepares one fresh session
  whose only task is collision-free local construction and verification of
  `level1-v0.1.0-rc.7` from that exact source. It forbids live Revolvr/model
  operations, suite preparation or launch, remote refs, workflows, tags,
  releases, external-use approval, queue authority, and EXT-20 completion.
- The launcher requires clean local/remote-main agreement and binds RC.1
  through RC.6 as immutable history. It explicitly protects the RC.6 suite,
  launch record, and terminal evidence with manifest and pre/post content-hash
  checks and forbids any reuse. Its next command is `./agent-ext20-rc7.sh`.

## EXT-20 RC.6 Failure Remediation (2026-07-25)

- Read-only controller authority reverified clean local `main`, local
  `origin/main`, and remote `main` at
  `cf7be211cdda6d5d7441c58a2ccd758022d20809`. The retained RC.6 terminal
  manifest verifies without mutation. Exactly operation
  `ext20-7b4a5932090f-01` ran and stopped `unsafe_or_ambiguous` before
  implementation, verification, or commits after both model calls completed.
- RC.6 is a preserved failed attempt. Its suite
  `.revolvr/ext20-rc6.LOLauh/suite`, launch record
  `.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365`,
  and terminal evidence are immutable and were not rerun, repaired, deleted,
  or mutated. `EXT-20` remains unchecked.
- Root cause: `prompt.LoadRunProfile` supplies normalized profile content, and
  worker evidence correctly records that normalized identity, but planner
  application formerly used its 1,458-byte size as the raw file read limit.
  The normal 1,459-byte newline-terminated planner file therefore failed before
  identity comparison. Planner application now follows auditor application's
  1 MiB bounded exact-or-`TrimSpace` validation, preserving protected reads and
  rejection of meaningful content changes.
- Added focused regression coverage accepting the normal final newline under
  its normalized identity and rejecting non-whitespace tampering. Focused
  prompt/cycle/audit/planner tests, `go test ./...`, `git diff --check`,
  `go run ./cmd/revolvr --help`, and `go run ./cmd/revolvr config check` pass.
  No live suite, real product Codex call, tag, release, external-use approval,
  queue authority, or EXT-20 completion occurred.

## EXT-20 RC.6 Prepared-Suite Controller Review And Live Gate (2026-07-25)

- Independent controller review accepted exactly the four-file suite-
  preparation scope. The only implementation change is the three candidate
  constants in `scripts/dogfood-external-level1-suite.sh`; its SHA-256 is
  `d16caafba9fb5fb8db83188d87006e57c8b77d88159d28502bb736e93672a3d7`.
  `bash -n`, suite `--static`, both retained collector manifests, candidate and
  Codex identity, both complete RC.6 sealed bundles, raw-Git refs, dedicated
  run/job/artifact, companion ten-job CI, and unchanged task ledger passed.
- Persistent suite `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite`
  remained exact at content-stream SHA-256
  `cb75bb94dca396d14d856e001fa1ed3a7d8d6ac46cf8c5d60eed2ca902f033c0`
  before and after review. Authority, plan, repository heads, exact candidate/
  Codex identities, ten pending doctor-ready tasks, 32-minute source-writer
  authority, sentinels, zero operation/collector manifests, empty aggregate,
  and absent RC.6 launch-record root all passed. `go test -count=1 ./...`
  passed. The three immutable RC.5 content streams and retirement checker also
  passed without mutation.
- Raw Git committed and pushed the accepted scope as
  `87be94f8fc4f04cb25c40598cc1f44cfe3b57efe` (`Prepare persistent RC.6
  dogfood suite`), tree `0e9009b53749d9f43706f57617bea04f8f84128e`.
- Added confirmation-gated direct foreground launcher
  `agent-ext20-rc6-live-direct.sh` and no-model helper
  `scripts/check-ext20-rc6-live-direct.sh`. The launcher binds the exact
  published suite, candidate/attestation/artifact, bundle, script, Codex,
  repository, task, source-lock, sentinel, and zero-evidence authorities. It
  creates one collision-free ignored external launch record containing exact
  pre-start authority, suite stdout/stderr, and durable exit or interruption
  status before starting the guarded suite once.
- The helper passed shell syntax, missing/wrong-token refusal, expected dirty-
  tree check-only refusal, isolated collision refusal, and suite/diagnostic
  preservation. No token-bearing path ran; no model call, live operation,
  launch record, aggregate, tag, release, external-use approval, queue
  authority, or `EXT-20` completion occurred. After controller publication,
  one final clean published `--check` must pass before handing off the exact
  live command. Blockers: none.

## EXT-20 RC.6 Guarded No-Model Suite Preparation (2026-07-25)

- Task selected: only the bounded RC.6 no-model suite-preparation sub-gate of
  still-unchecked `EXT-20`. The guarded Level-1 suite now binds exact candidate
  source `73f1f81f1c51d927114f19818a18161d0fcb8541`, Linux artifact SHA-256
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51`.
  Those are the only three suite constants changed. Release output remains
  `revolvr 0.1.0`; exact Codex package/output/SHA-256 remains `0.144.4` /
  `codex-cli 0.144.4` /
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  Model `gpt-5.6-sol`, reasoning effort `xhigh`, plan, schemas, scenarios,
  thresholds, configuration, live confirmation, and collector behavior are
  unchanged.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this state file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No Go source, dependency,
  workflow, candidate, bundle, ref, historical evidence, or live launcher
  changed. `.agent/TASKS.md` remains byte-exact at SHA-256
  `77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917`
  with `EXT-20` unchecked.
- Before editing, clean local and remote `main` agreed at
  `d4c570296c9dfd100cb10ef25b89c87ce492f396`, with published controller record
  `c2c8c41e51bb4ebb6c65c93778b88ebad4a4eaa7` an ancestor of both. Raw Git
  readback and public REST reconfirmed candidate ref/source
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, attestation ref/workflow commit
  `226276f151ae389d06c0118a931596712fbc7cc1`, workflow SHA-256
  `708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf`,
  successful candidate CI run `30153462797`, successful attestation run/job
  `30155142491` / `89671812731`, exact unexpired artifact `8618790256` named
  `level1-v0.1.0-rc.6-attestation` with digest
  `sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa`,
  and successful companion ten-job CI run `30155142490`.
- Complete candidate and verification bundles passed exact file-set,
  single-link regular-file, manifest, seal, and candidate self-verification
  checks before editing and passed their sealed manifests again after
  preparation. Their inventory/seal values remain
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  and `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
- Verification passed: `bash -n` for the suite and collector; suite
  `--static`; two collector `--fixture-only` runs; both `--verify-manifest`
  checks; canonical manifest comparison after excluding only
  `collected_at_utc`; `go test -count=1 ./...`; and `git diff --check`.
  Retained fixture root is `/tmp/revolvr-ext20-rc6-collector.kkvqrq`; both raw
  manifests have SHA-256
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`,
  and both timestamp-excluded canonical manifests have SHA-256
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- Exactly one persistent parent was created beneath ignored repository runtime
  state with the required template. The prepared no-model suite is
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite`; preparation
  used only `--prepare --run-root <that-path> --install-codex`. Suite ID is
  `ext20-7b4a5932090f`; `authority.tsv` SHA-256 is
  `5d90db1fca978451f0fa9b0950bc71dd9b334a00e0647d7e16158698a2358e40`;
  unchanged `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`;
  and its relative-path-bearing, null-sorted regular-file content-stream
  SHA-256 is
  `cb75bb94dca396d14d856e001fa1ed3a7d8d6ac46cf8c5d60eed2ca902f033c0`.
- Independent read-only inspection verified the prepared checksum; exact
  candidate path/hash/version/source/clean-VCS identity; exact Codex package,
  path/hash/version; unchanged model and reasoning effort; and effective
  source-writer authority
  `timeout=32m0s heartbeat_interval=10m40s required=32m0s`. The exact 11-row
  plan has ten unique pending doctor-ready tasks. Repo-a is clean on `main` at
  `fa611a1e2c72f5469095c3460dc3268cd21765c8`; repo-b is clean on `main` at
  `a73c742f2a0e5934d6da12069599dfde2a17b00d`. Both disposable markers and all
  sentinel bytes, modes, hard links, and symlinks are intact. There are zero
  operation manifests, zero collector manifests, an empty aggregate, and no
  RC.6 launch-record directory. An isolated empty-confirmation evaluation of
  the suite's exact first live guard refused before any prepared-root read or
  collector call; the complete suite content/layout fingerprint remained
  unchanged.
- Post-preparation raw-Git/public-REST readback reconfirmed every RC.6 remote
  authority above. RC.5 suite, first-operation evidence, and launch-record
  streams remain byte-exact at
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`;
  the retired launcher checker passed before and after without mutation.
- Result: no model call, live operation, launch record, tag, release,
  external-use approval, queue authority, or `EXT-20` completion occurred.
  Blockers: none. What remains is fresh independent review of the exact
  four-file tracked scope and prepared authority, controller publication, and
  only then a separately confirmed live pass using the exact command retained
  in `.agent/HANDOFF.md`.

## Current Focus

The audit backlog remains closed. A new working release gate now lives at
`.agent/AUTONOMOUS_EXTERNAL_READINESS.md` for autonomous use in external
projects. Its previously open policy questions are settled in that document
and `.agent/DECISIONS.md`: staged readiness levels, quarantine rather than
heuristic in-flight resume, rootless Linux OCI isolation for unattended modes,
mode-aware preflight, exact Codex executable/version authority, finite
unattended budgets, sequential first queue approval, immutable tagged release
authority, and quantitative dogfood/soak thresholds.

The settled gates are now decomposed into 41 ordered, independently verifiable
.agent/TASKS.md items. The sequence approves attended single-task operation
first, then a Linux-only sequential bounded queue, then the Linux-only
foreground daemon. Every item has explicit acceptance and verification
evidence, and release commit/push/tag actions retain the repository rule that
they require direct operator authorization.

EXT-01 through EXT-19 are complete. EXT-14's previously implemented production
interruption matrix has now passed its separate fresh verification pass.
Doctor, status, canonical task loading, configuration reads, and exact-task/
queue admission use one descriptor-backed, read-only repository-path
inspection. Doctor now normalizes bare invocation to
attended-task, supports the three settled modes, validates the strict canonical
autonomous graph and protected task/state/archive authority, and optionally
requires one exact attended task to be ready. Preflight and execution now share
the initial Git repository, submodule, cleanliness, platform, verification, and
finite attended-bound admission. External admission now binds the exact
release-authored Codex version and resolved executable digest plus the resolved
Git executable identity, rechecks them before execution, and records them in
effective configuration and invocation provenance. A standalone strict fake
Codex executable now validates the complete invocation contract and produces
deterministic supervisor/worker evidence through the ordinary runner. The real
production task runner now has exact strict-fake composition proofs spanning
the direct happy path, a blocking finding through one cited correction and
clean re-audit, and the complete attended terminal matrix: needs input,
authorized block, verification failure, no progress, trusted safety refusal,
caller cancellation, exact durable replay, and maximum cycle. Exact
task-workspace proof now binds the deterministic task branch,
baseline, control root, Git common directory, linked-worktree registration,
ownership marker, current HEAD, and checkpoint evidence while refusing
foreign or drifted authority before source mutation.
The shared commit boundary now uses the same exact literal admitted path set
for staging and `git commit --only`, so late unrelated index/worktree bytes
cannot enter a generated commit. The unused destructive workspace restore
surface is removed; production composition is command-spy proven free of
push, merge, rebase, reset, clean, and stash while an unrelated linked
worktree remains unchanged. The real-Git containment matrix now proves dirty
and staged admission refusal, ignored-source refusal before workspace
publication, active-submodule refusal, exact SHA-1 and SHA-256 task commits, a
linked control worktree, and an operator commit injected during task
publication. Exact
control/task/unrelated branch, index, worktree, commit, and sentinel authority
remains separated. The external recovery contract now enumerates all 30
before/during/after transition seams for supervisor, worker, verification,
commit, checkpoint, audit, finalization, queue reconciliation, notification,
and archive publication. Every row binds exact durable replay, quarantine,
readiness-level continuation, prohibited inference, and operator action.
The next fresh task remains EXT-20. RC.1 and its remote evidence are immutable
rejected history after the omitted-work-directory defect. RC.2 and its fresh
exact-commit CI/artifact attestation are also immutable rejected history after
its first live operation exposed inconsistent source-lock default authority.
RC.3 source `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, candidate evidence,
remote CI, artifact attestation, failed suite, and retained live evidence are
now immutable rejected history: operation `ext20-802d9db69596-01` failed
before inference because the supervisor Structured Outputs schema emitted
unsupported `uniqueItems`. The suite is permanently retired. The first local
schema repair passed tests but failed independent audit because its objects
were not all strict-compatible and its regression guard was only a denylist.
The follow-up closes all four production schemas and adds a recursive
supported-subset guard. Its complete dirty-tree scope passed a separate
read-only review and was committed and pushed with explicit operator authority
as exact source `2546913e38ec273f64417dece2f91df78fd42fc2`, tree
`8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. RC.4's candidate, exact ref,
ten-job CI run, artifact-attestation workflow/run/artifact, guarded suite, and
terminal failure are now immutable rejected history. Its first supervisor
selected `implement` from `pending` because the prompt omitted exact lifecycle
routing authority; runtime enforcement correctly rejected it before any
worker attempt. The bounded no-model source remediation passed independent
review and was published as exact source
`19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
`2fb39c93694e72d986e7a8a849a542fc1bf1728d`. Collision-free RC.5 is now
constructed and locally verified from only that source. Its exact candidate
ref, ten-job source CI, artifact-attestation ref/run/job/artifact, and
companion ten-job CI all pass. The original prepared RC.5 suite was lost to
external `/tmp` cleanup before any model call. A collision-free replacement is
now prepared without model calls under ignored repository runtime state at
`/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`.
Its reviewed launcher and durable authority updates are published as exact
recovery commit `d0bde8dffd8e233c04e593519546b7634d836304`; exact remote
readback and its clean no-model direct preflight pass. The later reported live
command retained no evidence of a model call or API result and is classified
as an unproven no-start, and that suite is now immutable retired evidence. One
new no-model replacement is prepared at
`/home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite`.
Its direct gate retains launch authority, output, and terminal or interruption
status outside the suite before starting a guarded live path. Independent
review passed, and exact confirmation `EXT20_PUBLISH_RC5_NO_START_REPLACEMENT`
published the reviewed replacement as commit
`d4e00d99252888528db29785eefedbf9f087859a`. Exact local/remote readback and its
clean no-model/no-launch-record preflight pass. No tag, release, external-use
approval, or `EXT-20` completion has occurred. Local tests do not establish
live API acceptance.
Recovery inspection uses a distinct
read-only workspace/Git inspection path that takes no mutation lease and
publishes no retained ambiguity ref when live HEAD has drifted. EXT-14 now has
independent focused, race, and full-suite verification evidence.
Current external-project decision remains not approved; the readiness
document's remaining blockers stay open until their ordered tasks pass.

## EXT-20 RC.6 Remote-Attestation Controller Review (2026-07-25)

- Independent controller verification accepted exactly the three durable-state
  files from the remote attestation pass. Raw Git readback confirmed candidate
  ref `73f1f81f1c51d927114f19818a18161d0fcb8541` and attestation ref
  `226276f151ae389d06c0118a931596712fbc7cc1`; all RC.6 tags remain absent.
- Public REST independently confirmed dedicated run `30155142491` and sole job
  `89671812731` as exact-workflow-head completed/success. Its sole unexpired
  artifact is exact ID `8618790256`, name
  `level1-v0.1.0-rc.6-attestation`, size `70273424`, and digest
  `sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa`.
  Companion CI run `30155142490` is exact-head completed/success with exactly
  the ten mandatory uniquely named successful jobs and recorded IDs. Candidate
  CI run `30153462797` remains exact-source completed/success.
- No `GITHUB_TOKEN` or `GH_TOKEN` was configured. Independent archive-endpoint
  inspection returned HTTP 401, agreeing with the pass and adding no archive-
  byte claim. Both complete RC.6 candidate/verification bundles passed their
  manifests and exact inventory/seal hashes. The original 29-file attestation
  output matched its documented relative-path hash stream, and the independent
  29-file replay matched its documented absolute-path hash stream; every
  `SHA256SUMS` row passed.
- RC.5 suite, first-operation evidence, and launch record independently remain
  byte-exact at content-stream SHA-256 values
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  The permanent-retirement checker passed. `.agent/TASKS.md` remains exact at
  SHA-256 `77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917`
  with `EXT-20` unchecked.
- Raw Git committed and pushed the accepted record as
  `c2c8c41e51bb4ebb6c65c93778b88ebad4a4eaa7` (`Record RC.6 remote artifact
  attestation`), tree `c81f4496984b02cec6fc41f7bea58c51f1b947f5`.
  No Go source or dependency changed, so no Go test was required.
- Added `agent-ext20-rc6-suite.sh` for the next fresh bounded pass. It may
  update only the guarded suite's exact candidate source/hash/bundle constants,
  run all no-model static and Go checks, and prepare exactly one persistent
  collision-free suite below ignored `.revolvr/` runtime state. It must retain
  and report authority without starting a model or adding a live launcher.
  It grants no tag, release, external-use approval, queue authority, or
  `EXT-20` completion. Blockers: none.

## EXT-20 RC.6 Remote Artifact Attestation (2026-07-25)

- Task selected: the bounded RC.6 remote artifact-attestation prerequisite of
  still-unchecked `EXT-20`. The pass started clean on `main`; a fresh no-tags
  fetch left local `main`, `origin/main`, and `HEAD` exact at
  `fed0a56c1511ad03702f330f239f223e58cf85f4`. Reviewed workflow commit
  `226276f151ae389d06c0118a931596712fbc7cc1`, tree
  `eac5c8d4696cec6f5383d9fd19f3d482045028c8`, parent
  `f282f263d817ff4ab32e04fe86e3c42612d18ca9`, and workflow SHA-256
  `708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf`
  were exact published ancestors of `origin/main`.
- Before publication, the complete 15-file RC.6 candidate bundle and 50-file
  verification bundle passed their sealed manifest, exact file-set, regular
  single-link file, candidate self-verification, source/tree, build-instruction,
  and artifact checks. Candidate inventory/seal remained
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`;
  verification inventory/seal remained
  `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  Source commit/tree remained `73f1f81f1c51d927114f19818a18161d0fcb8541` /
  `7c9753461a08b25915f4f53533d91e57d40a20ca`, build-instructions SHA-256
  remained `94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906`,
  and Linux/Darwin/FreeBSD artifacts remained
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
- Candidate ref `refs/heads/level1-v0.1.0-rc.6` remained exact at the candidate
  SHA. Public REST readback reconfirmed candidate CI run `30153462797` as
  push-triggered, attempt 1, exact branch/head, completed/success, with exactly
  its ten recorded unique successful jobs. PyYAML workflow structure, sole
  trigger/job, action versions, exact checkout/upload authority, and extracted
  embedded-shell syntax passed. Both retained 29-file results passed all six
  checksum rows, three byte-identical build pairs, metadata, empty-build-ID,
  version, and manifest checks. Local result
  `/tmp/revolvr-ext20-rc6-attestation.9dtfzU` retained normalized path-bearing
  stream SHA-256
  `fde643d9b1a262c087a661b1ef617d143414ce149e44a967221af7e21d93d7c1`;
  controller replay `/tmp/revolvr-ext20-rc6-attestation-review.OkDN5p`
  retained path-bearing stream SHA-256
  `6d6ae497c29da7a9722ccefa643f4240e7be7c8b6c9bcd6f8270da35c9e6f050`.
- Immediately before publication, the attestation ref was absent locally and
  remotely, all `*rc.6*` tags were absent, the exact candidate ref was the only
  RC.6 ref, and the public artifact query returned zero artifacts named
  `level1-v0.1.0-rc.6-attestation`. The sole external mutation used raw Git
  with an empty-expected lease at `2026-07-25T10:49:06Z`:
  `git push --force-with-lease=refs/heads/level1-v0.1.0-rc.6-attestation:
  origin 226276f151ae389d06c0118a931596712fbc7cc1:refs/heads/level1-v0.1.0-rc.6-attestation`.
  Exact remote readback is
  `226276f151ae389d06c0118a931596712fbc7cc1` at
  `refs/heads/level1-v0.1.0-rc.6-attestation`; no local branch was created and
  no existing ref was moved or deleted.
- Public REST identified exactly one dedicated run: `30155142491`, run number
  `1`, attempt `1`, name `Level 1 RC.6 candidate attestation`, workflow
  `.github/workflows/level1-rc6-candidate-attestation.yml`, event `push`, branch
  `level1-v0.1.0-rc.6-attestation`, exact workflow head, completed/success.
  It was created and started at `2026-07-25T10:49:09Z`, updated at
  `2026-07-25T10:51:53Z`, and is
  `https://github.com/ponchione/revolvr/actions/runs/30155142491`. Its sole job
  `89671812731`, `Rebuild and attest Level 1 RC.6 candidate`, ran from
  `2026-07-25T10:49:11Z` through `2026-07-25T10:51:52Z` and completed/success
  at the exact head:
  `https://github.com/ponchione/revolvr/actions/runs/30155142491/job/89671812731`.
- That run has exactly one unexpired artifact: ID `8618790256`, name
  `level1-v0.1.0-rc.6-attestation`, size `70273424` bytes, digest
  `sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa`,
  created/updated `2026-07-25T10:51:50Z`, expiring
  `2026-10-23T10:49:09Z`, archive endpoint
  `https://api.github.com/repos/ponchione/revolvr/actions/artifacts/8618790256/zip`.
  No read-only token was configured; the unauthenticated archive endpoint
  returned HTTP `401` with no redirect. Therefore this pass makes no
  controller-side archive-byte or 29-file archive-content claim; remote job
  success and the public artifact metadata/digest are the available authority.
- Companion CI is exact run `30155142490`, run number `71`, attempt `1`,
  workflow `.github/workflows/ci.yml`, event `push`, exact attestation
  branch/head, completed/success. It was created and started at
  `2026-07-25T10:49:09Z`, updated at `2026-07-25T10:52:23Z`, and is
  `https://github.com/ponchione/revolvr/actions/runs/30155142490`. Its exact
  head successful jobs were:
  - `89671812720` — Go 1.22 source floor and tests —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:50:58Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812720`
  - `89671812726` — Production autonomous strict-fake suite —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:50:05Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812726`
  - `89671812717` — Race tests — `2026-07-25T10:49:11Z` to
    `2026-07-25T10:52:22Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812717`
  - `89671812747` — Vet and module verification —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:49:56Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812747`
  - `89671812733` — Fake-Codex success smoke —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:49:47Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812733`
  - `89671812719` — Fake-Codex verification-failure smoke —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:49:48Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812719`
  - `89671812737` — Build linux/amd64 — `2026-07-25T10:49:11Z` to
    `2026-07-25T10:49:49Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812737`
  - `89671812730` — Build darwin/amd64 — `2026-07-25T10:49:11Z` to
    `2026-07-25T10:49:50Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812730`
  - `89671812748` — Build freebsd/amd64 — `2026-07-25T10:49:11Z` to
    `2026-07-25T10:49:47Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812748`
  - `89671812744` — Build Windows diagnostic stub —
    `2026-07-25T10:49:11Z` to `2026-07-25T10:49:26Z` —
    `https://github.com/ponchione/revolvr/actions/runs/30155142490/job/89671812744`
- Post-run preservation reverified all ten RC.1-through-RC.5 sealed bundles and
  all ten historical refs, with historical and RC.6 tags absent. RC.5 suite,
  first-operation evidence, and launch-record streams remained
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`;
  the permanent-retirement checker passed without evidence mutation. Both
  RC.6 sealed bundles, the candidate ref, and the new attestation ref remained
  exact. `.agent/TASKS.md` remains unchanged at SHA-256
  `77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917`
  with `EXT-20` unchecked.
- Files changed: this file, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md` only.
  No Go source or dependency changed, so no Go test was required. Verification
  comprised fresh Git/main/ancestry/collision/readback checks, complete current
  and historical sealed-bundle checks, candidate and workflow checks, retained
  output and RC.5 preservation checks, exact public-REST run/job/artifact
  polling and identity checks, archive-endpoint inspection, and final
  `git diff --check`. Result: passed. Blockers: none.
- What remains: after independent controller review and publication of these
  durable-state changes, one separately authorized no-model pass may prepare
  one new collision-free RC.6 Level-1 dogfood suite from the exact attested
  Linux artifact and current immutable authority. It must stop before any
  model call or live suite launch and grants no tag, release, external-use,
  queue, or `EXT-20` completion authority.

## EXT-20 RC.6 Attestation Workflow Controller Review (2026-07-25)

- Independent controller review accepted exactly the new RC.6 attestation
  workflow and the three associated durable-state files. Workflow SHA-256 is
  unchanged at
  `708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf`;
  PyYAML structure, sole trigger/job, read-only permissions, exact action pins,
  constants, extracted-shell syntax, and `git diff --check` all passed.
- The full unmodified embedded shell passed again from a fresh detached clone
  of exact candidate source `73f1f81f1c51d927114f19818a18161d0fcb8541`
  with remote `origin`, isolated runner root, two clean non-local source clones,
  and separate per-pass build/module caches. Independent root
  `/tmp/revolvr-ext20-rc6-attestation-review.OkDN5p` retained exactly 29
  regular single-link output files with path-bearing content-stream SHA-256
  `6d6ae497c29da7a9722ccefa643f4240e7be7c8b6c9bcd6f8270da35c9e6f050`.
  All six `SHA256SUMS` rows, three reproducibility rows, exact manifests,
  metadata/version authority, empty build IDs, and clean source clones passed.
- Both complete ignored RC.6 bundles reverified from their manifests. Exact
  inventory/seal values remain
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  and `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  Raw-Git candidate readback remained exact, candidate CI run `30153462797`
  remained completed/success at the exact source, the artifact query returned
  zero, and attestation ref/tag authorities remained absent.
- Raw Git committed the accepted four-file scope as
  `226276f151ae389d06c0118a931596712fbc7cc1` (`Add RC.6 artifact attestation
  workflow`), tree `eac5c8d4696cec6f5383d9fd19f3d482045028c8`, exact parent
  `f282f263d817ff4ab32e04fe86e3c42612d18ca9`, and pushed it to
  `origin/main`. No Go source or dependency changed, so no Go test was needed.
- Added `agent-ext20-rc6-attestation-remote.sh` for the next fresh bounded
  pass. Its execution authorizes only empty-expected raw-Git publication of
  the still-absent attestation ref at exact workflow commit
  `226276f151ae389d06c0118a931596712fbc7cc1` and
  collection of dedicated run/job/artifact and companion ten-job CI evidence.
  It forbids committing/pushing `main`, suite preparation, model work, tag,
  release, external-use approval, queue authority, and `EXT-20` completion.
  `.agent/TASKS.md` remains unchanged and `EXT-20` remains unchecked.

## EXT-20 RC.6 Local Artifact-Attestation Workflow (2026-07-25)

- Task selected: the bounded local RC.6 release-artifact attestation-workflow
  prerequisite of still-unchecked `EXT-20` only. Added separate workflow
  `.github/workflows/level1-rc6-candidate-attestation.yml`, triggered only by
  a push to `level1-v0.1.0-rc.6-attestation`. Its SHA-256 is
  `708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf`.
  Trigger HEAD is workflow authority only: checkout remains exact candidate
  source `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`.
- The workflow installs exact Go 1.26.5 with action caching disabled. Two
  independent clean `--no-local` clones use separate `GOCACHE` and
  `GOMODCACHE` paths and build Linux, Darwin, and FreeBSD amd64 with
  module-readonly/local-toolchain mode, `CGO_ENABLED=0`, trimpath, explicit
  clean VCS metadata, an empty build ID, and exact `main.version=0.1.0`.
  Corresponding pairs must be byte-identical and match exact SHA-256 values
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
- Every retained binary is checked for exact Go/tool/path/compiler/trimpath/
  target/CGO/source/`vcs.modified=false` metadata, empty build ID, one exact
  16-byte `main.version` symbol, and exactly one release-version string. Both
  Linux builds must print exactly `revolvr 0.1.0`. One future upload named
  `level1-v0.1.0-rc.6-attestation` retains both binary sets, hashes, build
  metadata, build-ID and version assertions, Linux version output,
  reproducibility evidence, and an exact manifest binding candidate CI run
  `30153462797` plus every workflow, ref, source, tool, target, flag, and hash
  authority.
- The embedded shell fails closed unless the candidate ref remains exact and
  the RC.6 tag, attestation-ref, attestation-namespace, and artifact-name
  authorities have no foreign collision. Its explicit local-validation mode
  admits exactly the sole existing candidate ref before publication; ordinary
  workflow execution instead requires exactly the candidate ref and its own
  exact trigger ref before artifact upload.
- Pre-edit checks passed with local and remote `main` exact at
  `f282f263d817ff4ab32e04fe86e3c42612d18ca9`, published remote-CI record
  `716c9a2293555f9abee80d384a673be426fe2ce0` in ancestry, candidate ref exact,
  RC.6 attestation ref/tags/workflow/artifact name absent, and CI run
  `30153462797` plus all ten recorded jobs exact and successful. The complete
  candidate and verification bundles reverified at inventory/seal values
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  and `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  Build-instructions and all three candidate artifact hashes also matched.
- Local verification passed: PyYAML structure and exact-action assertions;
  exact constant and historical-leak scans; extracted embedded-shell `bash
  -n`; `git diff --check`; and complete execution of the unmodified embedded
  shell from a fresh detached exact-source clone against the still-absent
  remote attestation namespace. The retained root is
  `/tmp/revolvr-ext20-rc6-attestation.9dtfzU`; its attestation output contains
  exactly 29 regular single-link files and has path-bearing content-stream
  SHA-256
  `fde643d9b1a262c087a661b1ef617d143414ce149e44a967221af7e21d93d7c1`.
  It contains six exact hash rows, three identical build pairs, six exact
  metadata/version authorities, six empty build-ID records, two exact Linux
  version outputs, distinct per-pass build/module caches, and exact authority
  and reproducibility manifests.
- Post-execution preservation passed. Both RC.6 complete sealed bundles, the
  exact candidate ref/tree and ten-job CI run, all ten RC.1-through-RC.5
  remote refs, all historical RC tag absences, and the five earlier workflow
  hashes reverified. The RC.5 evidence manifest passed independently; suite,
  evidence, and launch-record content-stream hashes remain exactly
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  `agent-ext20-rc5-live-direct.sh` remains permanently failed closed. Final
  remote readback still finds only the exact RC.6 candidate ref, no RC.6 tag
  or attestation ref, and no artifact-name collision.
- Files changed: the new RC.6 workflow, this file, `.agent/HANDOFF.md`, and
  `.agent/DECISIONS.md`. `.agent/TASKS.md` remains byte-for-byte unchanged at
  SHA-256 `77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917`
  with `EXT-20` unchecked. No Go source or dependency changed, so no Go test
  was required. No `gh`, commit, push, attestation ref, remote workflow run or
  artifact, suite, live/nested Codex or model operation, tag, release,
  external-use approval, queue authority, or `EXT-20` completion occurred.
  Verification result: passed. Blockers: none.
- What remains: a fresh independent review must repeat the exact workflow,
  sealed-bundle, local-output, collision, remote-CI, historical, and retained-
  evidence checks. Only after acceptance and separate publication authority
  may the controller commit the reviewed four-file scope, publish the still-
  absent attestation ref with an empty-expected raw-Git lease, and collect the
  dedicated run/job/artifact readback. Suite preparation and quantitative
  dogfood remain later separate gates.

## EXT-20 RC.6 Remote-CI Review And Attestation Handoff (2026-07-25)

- Independent review used raw Git and the public GitHub REST API to confirm
  remote candidate ref `refs/heads/level1-v0.1.0-rc.6` resolves only to exact
  source `73f1f81f1c51d927114f19818a18161d0fcb8541`, with no local branch or
  RC.6 tag. CI run `30153462797` independently read back as run number `66`,
  attempt `1`, event `push`, exact branch/head, `completed`, and `success`.
  Its jobs endpoint returned exactly the ten recorded mandatory unique jobs;
  every job had the exact candidate head, `completed`, and `success`.
- Review also repeated both RC.6 complete bundle verifiers and exact
  inventory/seal hashes, raw historical ref/tag comparison, all three RC.5
  retained content-stream hashes, the permanent RC.5 retirement check,
  `.agent/TASKS.md` hash/unchecked status, exact three-file scope, and `git
  diff --check`. All passed without a real model call or retained-evidence
  mutation.
- Raw Git published the accepted three-file remote-CI record as exact commit
  `716c9a2293555f9abee80d384a673be426fe2ce0`, tree
  `26c3039581cf12278ae5d58b58c0bd72fcd3e592`; exact local and remote `main`
  readback matched after push.
- New launcher `agent-ext20-rc6-attestation.sh` prepares one local-only pass to
  add and fully exercise the collision-free exact-candidate Go 1.26.5
  artifact-attestation workflow. That pass makes no commit, push, attestation
  ref, remote run/artifact, suite, model-call, tag, release, external-use,
  queue, or `EXT-20` completion claim. Blockers: none.

## EXT-20 RC.6 Exact-Candidate Remote CI (2026-07-25)

- Task selected: the bounded RC.6 candidate-ref and push-triggered remote-CI
  sub-gate of still-unchecked `EXT-20`. The pass stopped after remote CI
  evidence and made exactly one external mutation.
- Before publication, the complete 15-file candidate bundle and 50-file
  verification bundle passed their sealed inventory and exact file-set
  checks. Candidate ID `level1-v0.1.0-rc.6`, release `0.1.0`, source commit
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, source tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`, build-instructions SHA-256
  `94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906`,
  candidate inventory/seal
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`,
  and verification inventory/seal
  `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`
  matched exactly. Linux/Darwin/FreeBSD artifact SHA-256 values remained
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
- Immediately before publication, raw Git fetched `origin/main` without tags
  and proved exact local/remote `main`
  `4aa16b63f05da16d2d1620be83aec18a28e6d0cb`, published local-candidate
  record `60a19d4e38ddd5ea76490221973601e0efce2625` as its ancestor, and the exact
  candidate source as reachable from `origin/main`. Local and remote RC.6
  candidate and attestation refs, local and remote `*rc.6*` tags, the RC.6
  attestation workflow, and remote artifact name
  `level1-v0.1.0-rc.6-attestation` were all absent.
- The sole external mutation used raw Git with the required empty-expected
  lease:
  `git push --force-with-lease=refs/heads/level1-v0.1.0-rc.6: origin
  73f1f81f1c51d927114f19818a18161d0fcb8541:refs/heads/level1-v0.1.0-rc.6`.
  Exact remote readback is
  `73f1f81f1c51d927114f19818a18161d0fcb8541` at
  `refs/heads/level1-v0.1.0-rc.6`; no local branch was created and no existing
  ref was updated or deleted.
- The public GitHub Actions REST API identified exactly one matching CI run:
  run `30153462797`, run number `66`, attempt `1`, workflow `CI` at
  `.github/workflows/ci.yml`, event `push`, branch
  `level1-v0.1.0-rc.6`, head SHA
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, status `completed`, conclusion
  `success`, URL
  `https://github.com/ponchione/revolvr/actions/runs/30153462797`. It was
  created at `2026-07-25T09:51:09Z` and completed by
  `2026-07-25T09:54:24Z` within the finite polling bound.
- The jobs endpoint returned exactly the ten mandatory unique jobs. Each had
  head SHA `73f1f81f1c51d927114f19818a18161d0fcb8541`, status `completed`, and
  conclusion `success`:
  - `89667615436` — Go 1.22 source floor and tests —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615436`
  - `89667615420` — Production autonomous strict-fake suite —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615420`
  - `89667615421` — Race tests —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615421`
  - `89667615427` — Vet and module verification —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615427`
  - `89667615433` — Fake-Codex success smoke —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615433`
  - `89667615415` — Fake-Codex verification-failure smoke —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615415`
  - `89667615483` — Build linux/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615483`
  - `89667615546` — Build darwin/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615546`
  - `89667615476` — Build freebsd/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615476`
  - `89667615399` — Build Windows diagnostic stub —
    `https://github.com/ponchione/revolvr/actions/runs/30153462797/job/89667615399`
- Post-CI preservation reverified both RC.6 complete sealed bundles, every
  historical RC.1-through-RC.5 bundle inventory/seal, and all ten historical
  remote candidate/attestation refs. Historical RC tags remain absent. The
  retained RC.5 suite, first-operation evidence, and launch-record
  path-bearing content-stream SHA-256 values remain exactly
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`;
  the RC.5 live launcher remains permanently failed closed.
- Files changed: `.agent/HANDOFF.md`, this file, and
  `.agent/DECISIONS.md` only. `.agent/TASKS.md` is unchanged at SHA-256
  `77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917`
  with `EXT-20` unchecked. No product, dependency, bundle, workflow, tag,
  attestation ref, suite, operation, model call, `main` commit/push, release,
  external-use approval, queue authority, or `EXT-20` completion occurred.
- Verification commands run: complete RC.6 sealed-inventory/file-set and
  candidate self-verification; exact manifest/source/tree/artifact checks;
  RC.5 content-stream and permanent-retirement checks before and after work;
  no-tags raw-Git fetch, ancestry and collision checks, empty-expected push and
  exact readback; finite public-REST run polling; exact ten-job set comparison;
  all historical inventory/seal and remote-ref preservation checks; and final
  worktree/diff checks. Result: passed. Blockers: none.
- What remains: one separately bounded pass may construct and locally verify
  only a collision-free exact-checkout Go 1.26.5 RC.6 artifact-attestation
  workflow with two clean release builds, byte equality, sealed artifact hash,
  embedded metadata, version, and empty-build-ID checks. It must stop before
  commit, push, attestation ref, remote run/artifact, suite preparation, model
  work, tag, release, external-use approval, queue authority, or `EXT-20`
  completion.

## EXT-20 RC.6 Independent Local Review And Publication (2026-07-25)

- Independent review accepted the exact three-file durable-state diff and both
  ignored immutable RC.6 bundles. Candidate and verification inventory/seal
  SHA-256 values independently matched
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  and `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`.
  Both directories contain only complete listed regular single-link files.
- Review repeated the candidate self-verifier; verification-bundle hashes;
  helper/candidate byte equality and shell syntax; artifact hashes, embedded
  metadata, version output, and empty build IDs; exact source commit/tree,
  remediation ancestry, protected product-source diff, origin reachability,
  absent RC.6 refs/tags, and retained clean source authority; vulnerability
  result inspection; a fresh `GOTOOLCHAIN=local /usr/local/go/bin/go test
  -count=1 ./...` on exact candidate source; the permanent RC.5 retirement
  check; all three retained RC.5 content-stream hashes; and `git diff --check`.
  All passed without a real model call or retained-evidence change.
- Raw Git published the reviewed three-file construction record as exact
  commit `60a19d4e38ddd5ea76490221973601e0efce2625`, tree
  `d4d8bedccb5309f108f8d10a18c42c8fa9a750e6`; exact local and remote `main`
  readback matched after push.
- New launcher `agent-ext20-rc6-remote.sh` prepares the next fresh pass. Its
  execution authorizes only absent RC.6 candidate-ref creation at exact source
  `73f1f81f1c51d927114f19818a18161d0fcb8541` and collection of the exact
  push-triggered ten-job CI result. It makes no attestation, suite, model-call,
  tag, release, external-use, queue, or `EXT-20` completion claim. Blockers:
  none.

## EXT-20 RC.6 Local Candidate Construction (2026-07-25)

- Task selected: construct and locally verify only fresh collision-free Level-1
  candidate `level1-v0.1.0-rc.6` from exact published source commit
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`. Published `origin/main` at
  verification was `7598be79d8cfcbaa7ead3e184c7cd1c71be29244`; the source is
  reachable from it, remediation commit
  `010a8939ef6ad889a34590d05ce0326b6df57571` is an ancestor, and the
  intervening diff for `cmd`, `internal`, `go.mod`, and `go.sum` is empty.
  Controller launcher `agent-ext20-rc6.sh` is absent from the source commit and
  every candidate clone and artifact.
- Files changed: added ignored build helper
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.6.sh`, immutable
  candidate bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51`, and separate
  immutable verification bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51-verification`;
  updated this file, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md` only.
  `.agent/TASKS.md` is unchanged with `EXT-20` unchecked. There is no product,
  dependency, workflow, ref, tag, suite, operation, or historical-evidence
  change. Retained clean source/build root is
  `/tmp/revolvr-ext20-rc6-build.RPbxfY`.
- Collision and authority verification passed before construction: local and
  remote RC.6 candidate/attestation refs, local and remote `*rc.6*` tags,
  workflow, public artifact name, helper, both bundle paths, build root, suite
  root, and diagnostic namespace were absent. Exact release Go is
  `/usr/local/go/bin/go`, SHA-256
  `8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9`,
  at `go1.26.5`; source-floor verification used `go1.22.12`.
- Verification commands run: focused planner-role acceptance plus wrong-role/
  malformed-projection refusal and fallback-receipt tests in ordinary/race
  modes; Structured Outputs compatibility; lifecycle routing authority in
  ordinary/race modes; production autonomous happy path and strict-fake Codex
  contract in ordinary/race modes; complete Go 1.22.12 and Go 1.26.5 suites;
  `go vet ./...`; `go mod verify`; `govulncheck ./...` and
  `govulncheck -show verbose ./...`; the settled build helper and candidate
  self-verifier; independent `go version -m`, build-ID, symbol-size, exact
  version-string, and version-output checks; all ten RC.1-through-RC.5 sealed
  bundle inventories before/after; the retained RC.5 dogfood manifest before/
  after; the permanently retired launcher check; raw-Git ref/tag comparison;
  and both RC.6 complete manifest/file-set verifiers.
  Final durable-state checks also require `.agent/TASKS.md` to remain identical
  to `HEAD`, `EXT-20` to remain unchecked, and `git diff --check` to pass.
- Verification result: passed. Two independent fresh non-local clean clones
  produced byte-identical Go 1.26.5, version `0.1.0`, module-readonly,
  `CGO_ENABLED=0`, amd64, trimpath, explicit clean-VCS, empty-build-ID artifacts
  for Linux, Darwin, and FreeBSD. SHA-256 values are Linux
  `f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d`,
  Darwin
  `596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7`,
  and FreeBSD
  `60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344`.
  Build-instructions SHA-256 is
  `94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906`.
  Govulncheck found zero reachable and zero imported-package vulnerabilities;
  the retained module-only result is Windows-only, uncalled `GO-2026-5024` in
  `golang.org/x/sys@v0.30.0`, fixed in `v0.44.0`.
- The candidate inventory/seal SHA-256 values are
  `30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1` /
  `d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88`
  for 15 total/13 payload files. The verification inventory/seal values are
  `9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf` /
  `f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f`
  for 50 total/48 payload files. Both complete file sets verify from their
  manifests.
- Before and after construction, retained RC.5 suite, first-operation evidence,
  and launch-record path-bearing content-stream SHA-256 values remained exactly
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  `agent-ext20-rc5-live-direct.sh` remains permanently failed closed. All ten
  historical bundle seals and all existing remote candidate/attestation refs
  and tags remained exact.
- What remains: one fresh independent read-only review of the helper, both
  sealed bundles, source/tool/artifact authority, tests, vulnerability result,
  and preservation evidence. Candidate-ref publication and remote CI require a
  later separately authorized gate. RC.6 local construction makes no real API,
  remote-CI, attestation, dogfood, live-model, tag, release, external-use,
  queue, or `EXT-20` completion claim. Blockers: none.

## Developer-Alpha Publication And RC.6 Handoff (2026-07-25)

- Independent review accepted exactly `.agent/DECISIONS.md`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, `README.md`,
  `docs/attended-developer-alpha.md`, and
  `docs/external-project-runbook.md`. It found no Go, dependency, helper,
  configuration, release, or RC-evidence change and no unsupported approval
  claim.
- Review repeated `bash -n` for the attended smoke, retired RC.5 check and
  launcher, and alpha-readiness launcher; `bash
  scripts/smoke-external-attended.sh`; `go test -count=1 -v ./internal/app
  -run '^TestProductionAutonomousHappyPath$'`; `go test -count=1 ./...`;
  `scripts/check-ext20-rc5-live-direct.sh`; exact source-scope and local/remote
  Git reads; and `git diff --check`. All passed without a real model call.
- The retained RC.5 suite, first-operation evidence, and launch-record
  content-stream SHA-256 values independently remained
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
- Explicit operator authority published the reviewed documentation/state
  scope with raw Git as exact commit
  `73f1f81f1c51d927114f19818a18161d0fcb8541`, tree
  `7c9753461a08b25915f4f53533d91e57d40a20ca`. Exact local and remote `main`
  readback matched after push.
- The operator directed work to continue. New launcher `agent-ext20-rc6.sh`
  prepares one fresh pass for collision-free local RC.6 construction from the
  exact published commit above. That pass makes no live model call, commit,
  push, ref, workflow, tag, release, external-use, or `EXT-20` completion
  claim. RC.1 through RC.5 remain immutable rejected history. Blockers: none.

## Attended Developer-Alpha Path (2026-07-25)

- Task selected: the operator-directed bounded developer-alpha subtask within
  still-unchecked `EXT-20`. The pass documented the simplest attended path from
  the fixed current `main`; it did not continue real-Codex qualification or
  construct another candidate.
- Files changed: `docs/attended-developer-alpha.md`, `README.md`,
  `docs/external-project-runbook.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  and `.agent/DECISIONS.md`. No Go source, dependency, configuration, build
  helper, release artifact, RC evidence, or retired launcher changed.
- Exact developer-alpha build entry is
  `go build -trimpath -o ./bin/revolvr ./cmd/revolvr`; the binary is
  `<revolvr-source-root>/bin/revolvr`. The guide then covers `init`, attended
  configuration and finite-bound inspection, general and exact-task doctor,
  one foreground `run --until-terminal` operation capped at three cycles,
  status/task/run/receipt evidence inspection, typed stops/recovery, and review
  of the generated task-workspace commit before separate integration.
- Verification commands run: `bash -n` for
  `scripts/smoke-external-attended.sh`,
  `scripts/check-ext20-rc5-live-direct.sh`,
  `agent-ext20-rc5-live-direct.sh`, and
  `agent-attended-alpha-readiness.sh`; `bash
  scripts/smoke-external-attended.sh`; `go test -count=1 -v ./internal/app
  -run '^TestProductionAutonomousHappyPath$'`; `go test -count=1 ./...`;
  `scripts/check-ext20-rc5-live-direct.sh`; and `git diff --check`.
  The external smoke exercised root/subcommand help, init, config check,
  status, task/evidence surfaces, and expected attended doctor refusal for its
  unlisted fake without executing Codex. The focused test completed the real
  production composition with strict fake Codex in a separate collision-free
  temporary Git repository. All passed.
- Before and after work, suite, first-operation evidence, and launch-record
  path-bearing content-stream SHA-256 values remained exactly
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  The permanent-retirement check also passed without changing the suite or
  launch record.
- Result: the documented local developer path is ready for independent review.
  These checks prove only deterministic local fake behavior; no real model/API
  call, commit, push, candidate, tag, release, or external-use approval
  occurred. `EXT-20` remains incomplete and unchecked. The next gate is a
  fresh read-only review of the exact documentation/state diff before any
  separately authorized publication. Blockers: none.

## Published RC.5 Source Remediation And Developer-Alpha Handoff (2026-07-24)

- Fresh independent review accepted the terminal-failure remediation after
  moving the human no-verification placeholder out of list/claim syntax. The
  complete focused, race, production, full-Go, shell, manifest, diff, and
  immutable-evidence checks passed again.
- Raw Git published the exact ten-file remediation as source commit
  `010a8939ef6ad889a34590d05ce0326b6df57571`, a direct child of
  `42fb9c42b67a14149779cacf800f5771a030a023`. No candidate ref, tag, release,
  live operation, external-use approval, or `EXT-20` completion occurred.
- The next prepared task intentionally does not construct RC.6. New launcher
  `agent-attended-alpha-readiness.sh` may establish and strict-fake verify a
  minimal attended developer-alpha path from fixed `main`, with explicit
  disposable/recoverable-repository and incomplete-EXT-20 limitations. It may
  not make a real model call, commit, push, or change product behavior.
- `.agent/HANDOFF.md` is the exact next-session resume document. `EXT-20`
  remains unchecked; full release qualification can be resumed later only by
  an explicit operator decision.

## EXT-20 RC.5 Terminal Live-Failure Remediation (2026-07-24)

- Task selected: the single bounded source remediation within still-unchecked
  `EXT-20`. The pass fixed only the planning-dossier application defect and
  two fallback-receipt consistency defects proven by the first RC.5 live
  operation, permanently retired the RC.5 direct launcher, and added focused
  regressions. It did not retry, resume, reconcile, relabel, or mutate RC.5;
  start a live or nested model operation; prepare a later candidate; or commit,
  push, tag, release, approve external use, or complete `EXT-20`.
- Retained failure authority remains exact and immutable. Scenario
  `successful-source-change-1` expected `completed`, observed
  `unsafe_or_ambiguous`, exited 1, made one `plan` attempt, and recorded zero
  source changes, verification runs, audits, corrections, commits, or
  checkpoints. Its exact stop detail remains `cycle dossier identity is
  missing or malformed`. Launch record status 130 is the distinct controller-
  interruption fact; it is not the descendant operation's terminal product
  outcome.
- Planning application now admits either the exact legacy task-dossier
  authority or an exact `autonomous-role-dossier-manifest-v1` projection for
  the planner role. Task, hash, size, source, profile, decision, worker,
  artifact, and planning-output provenance checks remain in force; missing,
  malformed, wrong-role, wrong-schema, or mismatched authorities still fail
  closed. The positive role regression uses retained evidence shape
  `51c79f1d2cbe5ba7fe18f77b4ea9154479eed3996fd163e52611bbd646a948e0`
  and 6225 bytes.
- Fallback receipts now preserve every byte of a nonblank task body, including
  its final newline, while blank fallback and identifier/status normalization
  remain unchanged. The fixed human no-verification placeholder parses as no
  claim, structured entries remain exact, and an ordinary full receipt
  validation agrees with the ledger for a newline-terminated autonomous task
  and zero verification claims.
- `agent-ext20-rc5-live-direct.sh` is permanently failed closed with status 1
  for all arguments and contains no retained suite path, confirmation token,
  launch-record writer, child-process boundary, or live execution primitive.
  Its focused shell check dynamically proves missing/check/arbitrary/multiple
  argument refusal and exact retained suite/launch-record preservation without
  passing the historical live token.
- Files changed: `agent-ext20-rc5-live-direct.sh`,
  `scripts/check-ext20-rc5-live-direct.sh`,
  `internal/autonomousplanapply/apply.go`,
  `internal/autonomousplanapply/apply_test.go`,
  `internal/receipt/fallback.go`, `internal/receipt/parser.go`,
  `internal/receipt/receipt_test.go`,
  `.agent/HANDOFF.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`. No
  dependency, candidate, workflow, suite, task, release, or configuration file
  changed.
- Verification commands run: `gofmt -w` on all five changed Go files; focused
  ordinary and race tests in `./internal/autonomousplanapply` and
  `./internal/receipt` for accepted legacy/planner dossiers, wrong-role and
  malformed refusal, fallback exact task bytes, zero claims, and structured
  entries; complete ordinary and race tests for both changed packages;
  ordinary and race
  `TestProductionAutonomousHappyPath|TestStrictFakeCodexContract` in
  `./internal/app`; `scripts/check-ext20-rc5-live-direct.sh`;
  `go test -count=1 ./...`; `git diff --check`; and pre/post
  `scripts/dogfood-external-level1.sh --verify-manifest` plus exact retained
  content-stream checks.
- Verification result: passed. Before and after work, suite, evidence, and
  launch-record path-bearing content-stream SHA-256 values remained exactly
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
- Fresh independent review passed after one scope-local correction: the fixed
  human no-verification placeholder is plain section text rather than a list
  item, so the generic verification parser needs no literal denylist. The
  focused and complete verification matrix passed again. The operator
  authorized raw-Git publication in this session. A future candidate and any
  foreground/visible child-process live boundary require separate design and
  independent review. Tag, release, external-use approval, later-candidate
  construction, and `EXT-20` completion remain separate. Blockers: none.

## EXT-20 RC.5 Terminal Live Failure (2026-07-24)

- The retained diagnostic record proves that the replacement suite did start
  exactly once from clean published controller commit
  `82a966adde168f7845c5cc22bfff19a0324b3d1f`. Launch record
  `.revolvr/ext20-rc5-launch-records/ext20-9b7cc650ea0c-20260724T230449Z-468858`
  binds the exact candidate, Codex, suite, plan, repository heads, and
  pre-start content authority. Its status records launcher interruption 130;
  descendant evidence subsequently reached a terminal product result, so the
  launcher status is not used as the suite outcome.
- Operation `ext20-9b7cc650ea0c-01` reached the planner for scenario
  `successful-source-change-1`, consumed one supervisor and one worker attempt,
  then terminated `unsafe_or_ambiguous` with run exit 1 before verification,
  audit, correction, source mutation, or commit. The exact stop detail is
  `cycle dossier identity is missing or malformed`. The operation's planning
  output and provenance carry a valid planner role-projected dossier using
  `autonomous-role-dossier-manifest-v1`; the planning application gate still
  requires the legacy `autonomous-task-dossier-manifest-v1` schema.
- The fallback receipt also exposed two independent consistency defects:
  fallback normalization removed the canonical task body's final newline, and
  its bullet-form no-verification placeholder parsed as a verification claim.
  Receipt validation therefore failed exact task identity and verification
  result agreement. The retained Level-1 manifest itself verifies, source and
  containment authority stayed unchanged, and no commit was created.
- The suite, evidence, and launch-record path-bearing content-stream SHA-256
  values are respectively
  `875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c`,
  `9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76`,
  and `f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8`.
  They are immutable rejected evidence. The later `prepared file content
  changed` preflight is the expected refusal to reuse this now-nonpristine
  suite; it is not a no-start and must not be retried.
- RC.5 is rejected and `EXT-20` remains unchecked. New launcher
  `agent-ext20-rc5-live-failure-remediation.sh` is the sole next pass. It may
  repair only these evidence-backed source/receipt defects, permanently
  fail-close the retired live launcher, add regressions, and update durable
  state. It grants no commit, push, live operation, candidate construction,
  tag, release, external-use approval, or `EXT-20` completion authority.

## EXT-20 RC.5 Replacement Publication Controller Verification (2026-07-24)

- Independent verification accepted replacement commit
  `d4e00d99252888528db29785eefedbf9f087859a` with exact parent
  `239008cfbe819e65c1b2f886a9ba81323e5643e9` and only the reviewed seven-file
  scope. Controller-record commit
  `8b87d0b87ad4cdf031b1b128871ad9d76bd4364e` is its direct child and changes
  only `.agent/DECISIONS.md`, `.agent/HANDOFF.md`, and `.agent/STATE.md`.
- Local and remote `main` matched the controller record, the repository was
  clean, candidate/attestation refs remained exact, both suite roots retained
  their exact hashes, and no launch-record directory existed.
  `./agent-ext20-rc5-live-direct.sh --check` and
  `scripts/check-ext20-rc5-live-direct.sh` passed without a model call or
  launch record.
- The already published diagnostic-retaining direct launcher is the exact
  next gate. Its token-bearing path remains separately live-confirmation
  gated; this verification grants no live call, tag, release, external-use
  approval, or `EXT-20` completion.

## EXT-20 RC.5 No-Start Replacement Publication (2026-07-24)

- Task selected: reverify, publish, and record only the independently reviewed
  diagnostic-retaining RC.5 no-start replacement, then run its check-only
  preflights from clean published `main`. `EXT-20` remains unchecked.
- Files changed: replacement commit
  `d4e00d99252888528db29785eefedbf9f087859a` contains exactly
  `.agent/DECISIONS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `agent-ext20-rc5-live-direct.sh`,
  `agent-ext20-rc5-no-start-publish.sh`,
  `agent-ext20-rc5-no-start-review.sh`, and
  `scripts/check-ext20-rc5-live-direct.sh`; this controller record changes
  only the first three files. No Go source, dependency, suite authority,
  candidate/attestation ref, tag, release, or runtime operation changed.
- Verification commands run: exact raw-Git local/remote refs and leases;
  read-only public REST for runs `29697069305`, `29698647782`, and
  `29698647807`, job `88223716039`, and artifact `8445792045`; exact workflow,
  retired/replacement suite, candidate, Codex, repository, task, sentinel,
  authority, empty-evidence, and sealed-bundle checks; shell syntax; suite
  `--static`; `scripts/check-ext20-rc5-live-direct.sh`;
  `go test -count=1 ./...`; and `git diff --check`.
- Verification result: passed. The replacement commit is the exact child of
  `239008cfbe819e65c1b2f886a9ba81323e5643e9`, raw-Git remote readback matched,
  and clean published `./agent-ext20-rc5-live-direct.sh --check` reported
  `RC.5 live gate: preflight passed; no model call or launch record occurred`.
  Both suite hashes remained exact and no launch-record directory was created.
- What remains: only a separately confirmation-gated live pass may proceed;
  tagging, release, external-use approval, and `EXT-20` completion remain later
  independent gates. Blockers for this bounded publication task: none.

## EXT-20 RC.5 No-Start Replacement Independent Review (2026-07-24)

- The fresh independent review completed and left the accepted six-file scope
  unchanged with an empty index. Local and remote `main` remained exact at
  `239008cfbe819e65c1b2f886a9ba81323e5643e9`.
- Controller verification repeated exact diff scope, `git diff --check`, shell
  syntax, focused missing/wrong/check/collision behavior, retired-root hash,
  replacement prepared/hash/clean/empty authority, absence of launch records,
  and `go test ./...`; all passed without a live path or model call.
- `agent-ext20-rc5-no-start-publish.sh` is now the sole next launcher. Exact
  token `EXT20_PUBLISH_RC5_NO_START_REPLACEMENT` grants only the reviewed
  replacement/controller commits, exact-leased raw-Git `main` pushes, and
  check-only preflight. It grants no live, tag, release, external-use, or
  `EXT-20` completion authority.
- The operator supplied that exact publication confirmation for this fresh
  bounded pass. The reviewed scope and all required pre-publication checks
  passed again without a live path, model call, or launch record.

## EXT-20 RC.5 No-Start Replacement Preparation (2026-07-24)

- Task selected: preserve and retire the pristine RC.5 suite whose reported
  invocation retained no outcome, prepare exactly one collision-free no-model
  replacement, and make the future direct live boundary retain diagnostics.
  This is one bounded subtask within still-unchecked `EXT-20`.
- The reported old invocation remains classified only as an unproven no-start.
  There is no retained exit status and no evidence of model/API acceptance or
  rejection. Retired root
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`
  remained byte-for-byte exact before and after this work: 268 files, suite ID
  `ext20-c871c96647e9`, authority SHA-256
  `c4c6cd842aca0861db9c26bc269a6e5d38300d44f37cc44c78aea583564acc7f`,
  plan SHA-256
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and content-stream SHA-256
  `06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783`.
  It has no operation/collector manifest or aggregate entry and must never be
  executed again.
- Exactly one replacement-specific parent was created beneath ignored runtime
  state:
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20`.
  Only its `suite` child was prepared. The new suite ID is
  `ext20-9b7cc650ea0c`; authority, plan, and path-bearing content-stream
  SHA-256 values are respectively
  `3af0ccb8a7dd7fa5c2205882ce63c03dcb59935cb297fd4804a6a544a4827289`,
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and `44a0bacaf8c9ba0c52e3724553652dedda677a4118947e34b44fefa38671cee7`.
  Preparation installed exact Codex `0.144.4` and made no model call.
- Candidate output/hash/source and Codex output/hash are exact. Repo-a is clean
  on `main` at `41b6aa970b209aaad4291d35b04554464c4ba93c`; repo-b is
  clean on `main` at `6ae0cd3ff68995b93b9c22162ca395ed03fca157`.
  The exact plan has 11 rows and ten unique pending doctor-ready tasks;
  sentinels and links are exact; both repositories report source-writer
  authority `timeout=32m0s heartbeat_interval=10m40s required=32m0s`; and the
  replacement has zero operation manifests, zero collector manifests, and an
  empty aggregate.
- `agent-ext20-rc5-live-direct.sh` is rebound only to the replacement's exact
  root, suite/hash, and repository authority. `--check` remains read-only and
  creates no launch record. Only a later exact token-bearing path can reserve
  one collision-free directory beneath ignored
  `.revolvr/ext20-rc5-launch-records`, retain pre-start authority plus suite
  stdout/stderr and durable status, and invoke the guarded suite once. Signal,
  launcher, collision, and suite failures fail closed while preserving the
  available record.
- Files changed: `agent-ext20-rc5-live-direct.sh`, new
  `scripts/check-ext20-rc5-live-direct.sh`, new
  `agent-ext20-rc5-no-start-review.sh`, new
  `agent-ext20-rc5-no-start-publish.sh`, `.agent/HANDOFF.md`, this file, and
  `.agent/DECISIONS.md`. The review passed; the publication launcher is the
  exact next fresh pass and grants no live authority. No Go source,
  dependency, candidate, workflow, sealed bundle, suite/collector
  implementation, remote ref, task status, tag, or release authority changed.
- Verification result: passed. Commands covered raw-Git candidate/attestation
  readback; read-only public REST for runs `29697069305`, `29698647782`, and
  `29698647807`, job `88223716039`, and artifact `8445792045`; exact workflow
  hash; both complete sealed bundles; shell syntax; suite `--static`; focused
  direct-gate refusal/check/collision checks without the live token; exact old
  and new suite inspection; `go test -count=1 ./...`; and
  `git diff --check`.
- What remains: separately authorized publication, clean published `--check`,
  and only then a separately confirmed live pass. Tagging, release,
  external-use approval, and `EXT-20` completion remain later independent
  gates. Blockers for this bounded preparation task: none.

## EXT-20 RC.5 Unproven Live No-Start (2026-07-24)

- The operator reported completing the published token-bearing direct live
  command. Independent post-command inspection found no matching running
  process and no retained first-operation stdout/stderr, operation manifest,
  collector manifest, aggregate, task transition, source repository change,
  or controller change. No exit status was retained.
- Prepared root
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`
  remains exact at content-stream SHA-256
  `06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783`;
  repo-a and repo-b remain clean at their prepared heads, all 268 files remain,
  and the safe `--check` still passes. This establishes a pristine no-start,
  not a successful or failed model/API result.
- The explicit live attempt makes the suite terminal for reuse under the
  fail-closed gate. `agent-ext20-rc5-live-direct.sh` now refuses both paths
  until a new collision-free suite is prepared. `EXT-20` remains unchecked;
  no tag, release, or external-use approval exists.
- Files changed in this controller pass: `agent-ext20-rc5-live-direct.sh`,
  new `agent-ext20-rc5-no-start-remediation.sh`, this file,
  `.agent/HANDOFF.md`, and `.agent/DECISIONS.md`. The new launcher is the exact
  next fresh no-model task and grants no commit, push, or live authority.

## EXT-20 RC.5 Recovery Publication Controller Verification (2026-07-24)

- Independent verification accepted recovery commit
  `d0bde8dffd8e233c04e593519546b7634d836304` with exact parent
  `49dafe186f4c081c49483c46c6487914b1fd9c00` and only the reviewed six-file
  scope. It accepted controller-record commit
  `c896ebb81cf6168c21358b03fa6731ba43029663` as its direct child with changes
  limited to `.agent/DECISIONS.md`, `.agent/HANDOFF.md`, and
  `.agent/STATE.md`.
- Local and remote `main` matched the controller record, the repository was
  clean, candidate and attestation refs remained exact, and
  `./agent-ext20-rc5-live-direct.sh --check` passed from the final published
  tree with `RC.5 live gate: preflight passed; no model call occurred`.
- The exact next launcher is the already published
  `agent-ext20-rc5-live-direct.sh`. Its token-bearing command remains a
  separate explicit live-model gate; this verification made no suite/model
  call and grants no tag, release, external-use, or `EXT-20` completion
  authority.

## EXT-20 RC.5 Recovery Publication (2026-07-24)

- Task selected: independently reverify, publish, and record only the reviewed
  persistent RC.5 prepared-suite recovery under exact operator authority
  `EXT20_PUBLISH_RC5_RECOVERY`, then run its no-model direct preflight.
  `EXT-20` remains unchecked.
- Files changed: recovery commit
  `d0bde8dffd8e233c04e593519546b7634d836304` contains exactly
  `.agent/DECISIONS.md`, `.agent/HANDOFF.md`, `.agent/STATE.md`,
  `agent-ext20-rc5-live-direct.sh`,
  `agent-ext20-rc5-recovery-publish.sh`, and
  `agent-ext20-rc5-recovery-review.sh`; this controller record changes only
  the first three files. No candidate source, workflow, bundle, suite script,
  collector, Go source, dependency, task status, tag, candidate/attestation
  ref, or runtime operation changed.
- Verification result: passed. Raw-ref and public REST evidence, both sealed
  bundles, shell syntax/static mode, complete prepared-root identity and empty
  evidence, all ten doctor-ready pending tasks, `go test -count=1 ./...`,
  `git diff --check`, exact staged scope/diff, exact-leased raw-Git push, and
  local/remote readback all passed. From clean published recovery commit
  `d0bde8dffd8e233c04e593519546b7634d836304`,
  `./agent-ext20-rc5-live-direct.sh --check` reported
  `RC.5 live gate: preflight passed; no model call occurred`.
- Blockers for this bounded publication task: none. What remains is the
  separately confirmation-gated live command recorded in `.agent/HANDOFF.md`;
  this pass grants no authority to execute it, tag, release, approve external
  use, or complete `EXT-20`.

## EXT-20 RC.5 Recovery Independent Review (2026-07-24)

- The exact fresh review launcher completed and left the reviewed recovery
  tree unchanged: four tracked recovery files remained modified, only the
  review launcher remained untracked, the index stayed empty, and local and
  remote `main` stayed at
  `49dafe186f4c081c49483c46c6487914b1fd9c00`.
- Independent controller verification after the review repeated the exact
  diff scope, `git diff --check`, shell syntax, suite `--static`, prepared
  checksum/authority/plan/content hashes, exact clean repository heads, empty
  operation/collector/aggregate state, candidate and attestation raw-ref
  readback, and `go test ./...`; all passed. No suite/model operation, commit,
  push, tag, release, external-use approval, or `EXT-20` completion occurred.
- The operator supplied exact `EXT20_PUBLISH_RC5_RECOVERY` authority to this
  fresh pass. Publishing the reviewed recovery and running its no-model direct
  preflight is the authorized bounded task; live-suite authority remains
  excluded and `EXT-20` remains unchecked.

## EXT-20 RC.5 Volatile-Root Recovery (2026-07-24)

- Task selected: recover only the lost no-model RC.5 prepared-suite authority;
  `EXT-20` remains unchecked. The recorded direct `--check` failed closed with
  `prepared suite is unavailable` before any model call because
  `/tmp/revolvr-ext20-rc5.weLZtI/suite` no longer existed. Read-only inspection
  found the older RC.1-through-RC.4 `/tmp` roots absent as well. Their sealed
  Git/bundle/remote authority and durable summaries remain, but their former
  filesystem copies are not claimed as retained evidence.
- Exact immutable authority was revalidated without `gh`: candidate and
  attestation refs remain
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` and
  `109b38cdb309b50c38ab2ef0df33998e92dfd5e6`; workflow SHA-256 remains
  `9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852`.
  Public REST readback reconfirmed successful ten-job candidate run
  `29697069305`, successful attestation run/job `29698647782` / `88223716039`,
  exact unexpired artifact `8445792045` with digest
  `sha256:ab0febbc035f634d39babb897edd0c94bfaf1805ebc212e767a551fb1758b0e2`,
  and successful companion run `29698647807`. Both complete RC.5 sealed bundle
  manifests pass.
- One collision-free parent was created under ignored runtime state rather
  than volatile `/tmp`. Exact prepared root is
  `/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite`;
  suite ID is `ext20-c871c96647e9`, authority SHA-256 is
  `c4c6cd842aca0861db9c26bc269a6e5d38300d44f37cc44c78aea583564acc7f`,
  operation-plan SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`,
  and the direct path-bearing content-stream SHA-256 is
  `06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783`.
  Preparation installed exact Codex `0.144.4` but started no model.
- Candidate and Codex hashes/version outputs are exact. Repo-a is clean on
  `main` at `7ff28f8e19c4d3b57ea0e565b764db35a5207599`; repo-b is clean on
  `main` at `697def8f11122af055c69726277e88dd86e63a6c`. All ten unique tasks are
  pending and doctor-ready across the exact 11-row plan; sentinels and hard/
  symbolic links are exact. Effective source-writer authority remains
  `timeout=32m0s heartbeat_interval=10m40s required=32m0s`. There are zero
  operation manifests, zero collector manifests, and zero aggregate entries.
- Files changed: `agent-ext20-rc5-live-direct.sh`,
  `agent-ext20-rc5-recovery-review.sh`,
  `agent-ext20-rc5-recovery-publish.sh`, this file, `.agent/HANDOFF.md`, and
  `.agent/DECISIONS.md`. At that recovery stage, the review and publication
  were still separate gates; their completed result is recorded above. No Go
  source, dependency, candidate, workflow, suite plan/scenario/threshold,
  remote ref, or runtime operation changed.
- Verification result: the no-model preparation and independent inspection
  pass. Commands included shell syntax, suite `--static`, raw remote-ref and
  public REST readback, both sealed-bundle checksum manifests, exact
  `--prepare --install-codex`, prepared checksum/hash/identity/layout checks,
  ten focused doctor readiness checks, `go test ./...`, and
  `git diff --check`. At that recovery stage, the rebound direct launcher's
  `--check` was also proved to stop exactly at `controller repository is not
  clean`, as required before publication. The separately authorized review,
  publication, and complete preflight are recorded above. No live/nested model
  call, tag, release, external-use approval, or `EXT-20` completion occurred.

## EXT-20 RC.5 Prepared-Suite Controller Review And Day-End Handoff (2026-07-19)

- Independent controller verification passed the suite/collector shell
  syntax, both sealed RC.5 bundles, suite static mode, exact three-line tracked
  delta, full `go test -count=1 ./...`, and `git diff --check`. Read-only
  prepared-root inspection passed `prepared.sha256`, authority/plan hashes,
  exact candidate and Codex identities, exact clean repository heads, ten
  pristine pending task states, zero operation/collector evidence, and empty
  aggregate authority.
- The suite preparation result was committed and raw-Git pushed to
  `origin/main` as `f5ba71d3be8c13b201c7609101a5269b6f463af5` (`Prepare RC.5 live
  dogfood suite`). Candidate and attestation refs remained exact. No model
  call occurred.
- `agent-ext20-rc5-live-direct.sh` is prepared for the next fresh session. It
  binds the exact retained root, refs, candidate/Codex hashes, authority/plan,
  repository heads, script hashes, sentinels, task counts/lifecycles, and empty
  evidence. Its path-bearing null-sorted `sha256sum` stream is independently
  bound at
  `c945bb72f24c226215565ac868bb2c255d0a9f75be31819a1dafc030cb032009`;
  this is a separate representation from the preparation pass's retained
  content fingerprint. `--check` performs preflight only. The exact live token
  is still required before the launcher can execute the guarded suite.
- The direct launcher and initial day-end resume state were committed and
  raw-Git pushed as `d3872c00c30e15cc92dfbae8b890602c05b5fe8a` (`Prepare RC.5 direct
  live gate`). Its complete `--check` mode then passed against clean matching
  local/remote `main`; no model call occurred. A fresh session must rerun that
  safe preflight because retained-root drift remains fail-closed.
- Day-end state: stop here. No live/nested model call, tag, release,
  external-use approval, or `EXT-20` completion occurred. The next session
  must first run the no-model `--check` preflight, then obtain fresh explicit
  live confirmation before the one admitted execution.

## EXT-20 RC.5 Guarded No-Model Suite Preparation (2026-07-19)

- Task selected: only the bounded RC.5 no-model suite-preparation sub-gate of
  `EXT-20`. `EXT-20` remains unchecked.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this state file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. The suite script changed
  only its three candidate-authority constants: source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, Linux SHA-256
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  and bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61`.
  Release output remains `revolvr 0.1.0`; exact Codex package/output/SHA-256
  remains `0.144.4` / `codex-cli 0.144.4` /
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, confirmation,
  collector, Go source, or dependency changed.
- Pre-edit authority passed with raw Git and public REST only. Candidate and
  attestation refs read back at exact SHAs
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` and
  `109b38cdb309b50c38ab2ef0df33998e92dfd5e6`; the workflow remained exact at
  SHA-256
  `9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852`.
  Candidate run `29697069305` had ten successful jobs; attestation run
  `29698647782` and sole job `88223716039` succeeded; artifact `8445792045`
  retained exact name `level1-v0.1.0-rc.5-attestation` and digest
  `sha256:ab0febbc035f634d39babb897edd0c94bfaf1805ebc212e767a551fb1758b0e2`;
  companion run `29698647807` had ten successful jobs. Both complete RC.5
  bundles passed their 15-file/44-file inventories and inventory seals.
- Verification commands passed: `bash -n` for the suite and collector;
  `sha256sum -c files.sha256` and `sha256sum -c files.sha256.sha256` for both
  complete RC.5 bundles; suite `--static`; two collector `--fixture-only`
  runs; both `--verify-manifest` checks; canonical manifest comparison after
  excluding only `collected_at_utc`; `go test -count=1 ./...`; and
  `git diff --check`. Retained fixture root is
  `/tmp/revolvr-ext20-rc5-collector.VVOYxA`; both raw manifest hashes are
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`
  and both canonical hashes are
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- One new parent was created with exact template
  `/tmp/revolvr-ext20-rc5.XXXXXX`. The retained prepared suite is
  `/tmp/revolvr-ext20-rc5.weLZtI/suite`; preparation used only
  `--prepare --run-root /tmp/revolvr-ext20-rc5.weLZtI/suite
  --install-codex` and started no model. Suite ID is
  `ext20-f87a569b5efa`; `authority.tsv` SHA-256 is
  `6577bd6c433db64178f5406b62c554370b64f030523c28c0486c6f35fc779b7e`;
  unchanged `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Independent read-only inspection used `GIT_OPTIONAL_LOCKS=0` and verified
  the prepared checksum, exact candidate path/hash/version/source/clean VCS
  metadata, exact Codex package/path/hash/version, and effective source-writer
  authority `timeout=32m0s heartbeat_interval=10m40s required=32m0s`. The
  exact 11-row plan exposes ten unique ready tasks. Both disposable
  repositories are clean on `main`: repo-a at
  `7f1a2135c8dc403a612913195068d2ba1db21690` and repo-b at
  `7d8510cd82281776bb6ebe2436db56da84e7802c`. There are zero runtime operation
  manifests, zero collector manifests, and zero aggregate entries. Whole-root
  content and metadata/layout fingerprints stayed unchanged at
  `004104a72f5feb21392b71d48a69c6720a91df9899d0ba5c8b8ec69e6b144812`
  and
  `aa87d2e2acf07ed242ae63010ea6701fab3559bd0477bd52011404d1c49e6524`.
  The empty-confirmation predicate was exercised in isolation and source
  order proves refusal before a prepared-root read; neither `--live` nor the
  confirmation value was passed to the suite.
- Preservation passed for all ten RC.1-through-RC.4 sealed inventories, all
  four historical workflow hashes, all eight historical candidate/attestation
  refs, and terminal RC.4 suite
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite`. Its operation remains exactly
  `ext20-2bd21aea4f72-01`; fresh whole-root content and metadata/layout
  fingerprints remained
  `46f0e63c7d4cf02a445ef1391870b65010547a8b8a0cbdce6cd21ca64fbae03a`
  and
  `78fc7d6ad2bdecf031253a5b997124fc1b09dadad4d09612a087ac11df1f5b68`.
- Verification result: passed. Blockers for this bounded preparation task:
  none. What remains is a separately authorized, confirmation-gated live
  execution using exactly:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc5.weLZtI/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. This command was not executed. No `gh`,
  commit, push, live/nested model operation, tag, release, external-use
  approval, or `EXT-20` completion occurred.

## EXT-20 RC.5 Attestation Controller Review And Suite Handoff (2026-07-19)

- Independent controller review reverified the workflow's exact minimal
  specialization, YAML/embedded-shell structure, 29-file retained local
  artifact evidence, exact raw-Git main/candidate/attestation refs, successful
  dedicated run/job/artifact, and successful companion ten-job CI run.
- The remote attestation result was committed and raw-Git pushed to
  `origin/main` as `6d32de0c9c932e202ea37ecb9d435fd70ad013ad` (`Record RC.5 remote
  artifact attestation`). Candidate source and workflow authority remain
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` and
  `109b38cdb309b50c38ab2ef0df33998e92dfd5e6`, respectively.
- `agent-ext20-rc5-suite.sh` is prepared for the next fresh bounded pass. It
  may update only the guarded suite's three candidate-authority constants,
  verify without model calls, and prepare one collision-free RC.5 suite root.
  It must stop before the confirmation-gated live pass, tag, release,
  external-use approval, or `EXT-20` completion.

## EXT-20 RC.5 Remote Artifact Attestation Result (2026-07-19)

- Independent controller review passed workflow SHA-256
  `9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852`,
  exact RC.4-to-RC.5 minimal specialization, PyYAML structure and constants,
  extracted embedded-shell `bash -n`, the retained 29-file output shape and
  hash stream, all six `SHA256SUMS` rows, metadata/version authorities, empty
  build IDs, Linux version outputs, and three byte-identical pairs. Candidate
  ref readback remained exact and attestation ref/tag/artifact namespaces were
  empty immediately before publication.
- Raw Git committed the reviewed workflow and local state on `main` as
  `109b38cdb309b50c38ab2ef0df33998e92dfd5e6` (`Add RC.5 artifact
  attestation workflow`) with exact parent
  `76cef08c9b3739f9745663e9c5ecf15a2d02131b`, then pushed it after a fresh
  parent check. An empty-expected force-with-lease created only
  `refs/heads/level1-v0.1.0-rc.5-attestation` at that workflow commit. Remote
  `main` and attestation-ref readbacks matched the workflow commit while the
  candidate ref remained exact source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`.
- Dedicated run `29698647782`, run number `1`, attempt `1`, workflow `Level 1
  RC.5 candidate attestation`, event `push`, branch
  `level1-v0.1.0-rc.5-attestation`, and exact workflow head SHA completed with
  `success`: `https://github.com/ponchione/revolvr/actions/runs/29698647782`.
  It was created at `2026-07-19T18:24:07Z` and updated at
  `2026-07-19T18:26:34Z`. Its sole job `88223716039`, `Rebuild and attest
  Level 1 RC.5 candidate`, ran from `2026-07-19T18:24:10Z` through
  `2026-07-19T18:26:33Z` and completed with `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29698647782/job/88223716039`.
- The run retained exactly one unexpired artifact: ID `8445792045`, name
  `level1-v0.1.0-rc.5-attestation`, size 70,270,595 bytes, digest
  `sha256:ab0febbc035f634d39babb897edd0c94bfaf1805ebc212e767a551fb1758b0e2`,
  created/updated `2026-07-19T18:26:30Z`, expiring
  `2026-10-17T18:24:08Z`. Its archive endpoint is
  `https://api.github.com/repos/ponchione/revolvr/actions/artifacts/8445792045/zip`.
  No `GITHUB_TOKEN` or `GH_TOKEN` was available and unauthenticated download
  returned HTTP 401, so controller-side archive comparison was not possible.
  The successful remote job performed both clean rebuilds, sealed hash and
  identity assertions, pair comparisons, checksum verification, and remote
  authority readback before upload.
- Companion CI run `29698647807`, run number `46`, attempt `1`, completed with
  `success` at the exact workflow commit:
  `https://github.com/ponchione/revolvr/actions/runs/29698647807`. Its exactly
  ten successful job IDs are `88223716087`, `88223716115`, `88223716091`,
  `88223716099`, `88223716095`, `88223716116`, `88223716106`, `88223716142`,
  `88223716136`, and `88223716097`, corresponding to the mandatory EXT-15 job
  names retained for the candidate CI gate.
- No candidate source/ref, bundle, historical workflow/ref/evidence, tag,
  release, retired suite, or external-use decision changed. No `gh` or
  live/nested model operation ran. `EXT-20` remains unchecked. RC.5's
  candidate-ref, remote-CI, and artifact-attestation prerequisites are now
  complete; the next bounded pass is fresh collision-free no-model suite
  preparation.

## EXT-20 RC.5 Local Artifact-Attestation Workflow (2026-07-19)

- Task selected: the bounded local RC.5 release-artifact attestation-workflow
  prerequisite of `EXT-20` only. `EXT-20` remains unchecked.
- Added the separate
  `.github/workflows/level1-rc5-candidate-attestation.yml`, triggered only by
  a push to `level1-v0.1.0-rc.5-attestation`. Trigger HEAD is workflow
  authority only: the workflow checks out exact candidate source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` and requires tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`. The locally reviewed workflow
  SHA-256 is
  `9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852`.
- Exact Go 1.26.5 is installed with action caching disabled. Two independent
  clean `--no-local` clones use distinct `GOCACHE` and `GOMODCACHE` paths and
  build Linux, Darwin, and FreeBSD amd64 with `CGO_ENABLED=0`, local
  toolchain selection, module-readonly mode, `-trimpath`, explicit clean VCS
  metadata, an empty build ID, and exact `main.version=0.1.0`.
- Every pass artifact must carry exact Go/tool/path/compiler/trimpath/target/
  CGO/source/`vcs.modified=false` metadata, one exact 16-byte `main.version`
  symbol, exactly one `0.1.0` string, and an empty retained build-ID result.
  Both Linux binaries must print exactly `revolvr 0.1.0`. Corresponding pass
  artifacts must be byte-identical and reproduce exact SHA-256 values Linux
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  Darwin
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and FreeBSD
  `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`.
- One upload named `level1-v0.1.0-rc.5-attestation` retains both binary sets,
  all hashes, build metadata, empty build IDs, exact per-binary version
  authority, Linux version outputs, pairwise reproducibility evidence, and an
  exact authority manifest binding the workflow/ref/artifact namespace,
  candidate source/tree/ref/ID, toolchain, build environment and flags,
  source passes, targets, and hashes.
- Pre-edit authority passed. The 15-file candidate and 44-file verification
  bundles reverified against their complete sealed inventories and retained
  inventory/seal SHA-256 values
  `ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88` /
  `8bf947efd3d7f6467d500f88278913c0bcf5dd922331e558d483176a777584ab`
  and
  `e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366` /
  `2cded484b787daa903ebf457f3f96bb9520af122bd48114300d78e543f39ccb8`.
  Raw Git fetched `origin/main` at
  `76cef08c9b3739f9745663e9c5ecf15a2d02131b`, re-read the exact candidate
  ref, and found the proposed attestation ref, every `*rc.5*` tag, and the
  workflow path absent. Public REST found the artifact name absent and
  independently revalidated run `29697069305` plus its exact ten recorded
  jobs at the candidate SHA as `completed` / `success`.
- Local verification passed: PyYAML `BaseLoader` structure and exact-constant
  assertions; extracted embedded-shell `bash -n`; an exact minimal
  RC.4-to-RC.5 specialization comparison; workflow diff hygiene; and complete
  execution of the unmodified embedded shell from a detached exact-source
  clone against a collision-safe local raw-Git ref fixture. The retained root
  is `/tmp/revolvr-ext20-rc5-attestation.qYVLU1`; its output has exactly 29
  regular single-link files, six exact hash rows, three byte-identical pairs,
  six exact metadata/version authorities, six empty build-ID records, two
  exact Linux version outputs, and complete authority/reproducibility
  manifests. Its relative-file/hash-stream SHA-256 is
  `7323a9872b7a9fb8ac4a9b3cf8826bbc0455d929b05b27d7c22c4c568c3daf89`.
  The first minimal-delta command used an incomplete read-only `rc.4`
  replacement; correcting only that comparison transform produced an exact
  match. Final closeout-harness probes also first assumed a colon rather than
  the task line's em dash and treated a seal file's own digest as its embedded
  inventory digest. Direct inspection corrected those read-only harness
  assumptions; the copied candidate seal intentionally retains basename
  `files.sha256` while its exact copy is named `candidate-files.sha256`.
  Digest and byte-copy comparisons then passed. No workflow, candidate, or
  evidence repair was required.
- Preservation passed. All available historical sealed inventories and the
  RC.4 terminal prepared/collector inventories reverified. RC.1 through RC.4
  workflow SHA-256 values remain
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`,
  `4c96ec62a3757878926b62aee65ce8ba3ec6ac2148ac251622ab294109312c6d`,
  `76b0b9c3e683a4df5fa4103588ea668459b00efa18eee4930baa81c440df4da6`,
  and `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`.
  The fresh 40-target historical content/layout fingerprints were identical
  before and after at
  `e4a0fbfa7527e683d5bc3e81ee038bf9b50e0e175ef98d08f130554891c13c04`
  and `8cde67fa6c5fdd1a575d8818eb7faa0cd62f805d5383e5e6f6db39e599057bce`.
  RC.4 suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite` and operation
  `ext20-2bd21aea4f72-01` remain unchanged terminal rejected history. The
  exact RC.5 candidate ref remains unchanged; the attestation ref and tag
  namespace remain absent. Final public-REST readback again found run
  `29697069305` and all ten exact jobs `completed` / `success`, with the
  proposed attestation artifact name still absent.
- Files changed: the new RC.5 workflow, this file, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md`. `.agent/TASKS.md` is unchanged. No Go source or
  dependency changed, so no Go test was required. No `gh`, commit, push,
  attestation ref, remote run/artifact, suite, live/nested Codex or model
  operation, candidate source/ref mutation, tag, release, external-use
  approval, or `EXT-20` completion occurred. Blockers for this bounded local
  task: none.
- Collision-free raw-Git ref reserved for later controller publication is
  `refs/heads/level1-v0.1.0-rc.5-attestation`. After independent review and a
  workflow commit, the controller must publish only that still-absent ref with
  an empty-expected lease while preserving the exact candidate ref, require
  the dedicated remote job to pass, and record the workflow commit/ref, run
  and job IDs/URLs/status/conclusions, artifact ID/name/size/digest/creation/
  expiry, and retained-archive comparison. Suite preparation and live
  quantitative dogfood remain later separate gates.

## EXT-20 RC.5 Remote-CI Controller Review And Handoff (2026-07-19)

- Independent raw-Git readback reconfirmed
  `refs/heads/level1-v0.1.0-rc.5` at exact source
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`; the RC.5 attestation ref and
  `*rc.5*` tag namespace remain absent. Both candidate and verification bundle
  inventories and their seals passed again.
- Independent public REST readback reconfirmed `ci.yml` run `29697069305` as
  run number `42`, attempt `1`, `push`, branch `level1-v0.1.0-rc.5`, exact
  candidate head SHA, and `completed` / `success`. It returned exactly the ten
  job names and IDs recorded below; every job had the exact candidate SHA and
  `completed` / `success`.
- The durable result was committed and raw-Git pushed to `origin/main` as
  `1cd46ad7d0c240da378522c9540b421f39376f58` (`Record RC.5 remote candidate
  CI`). `agent-ext20-rc5-attestation.sh` is prepared for the next fresh pass:
  construct and locally verify only the collision-free exact-checkout Go
  1.26.5 RC.5 artifact-attestation workflow, then stop before commit,
  publication, remote execution, suite preparation, or model work.

## EXT-20 RC.5 Exact-Candidate Remote CI (2026-07-19)

- Task selected: the bounded RC.5 candidate-ref and remote-CI subtask of
  `EXT-20` only. `EXT-20` remains unchecked.
- Before publication, the 15-file candidate and 44-file verification bundles
  reverified against their complete sealed inventories. Candidate ID
  `level1-v0.1.0-rc.5`, release `0.1.0`, source commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, source tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`, build-instructions SHA-256
  `69e0e533258b88b810db465935e66c49fd4e294fb745fc13998115dc8951dcb8`,
  and Linux/Darwin/FreeBSD SHA-256 values
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`
  matched exactly. Candidate inventory/seal remained
  `ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88` /
  `8bf947efd3d7f6467d500f88278913c0bcf5dd922331e558d483176a777584ab`;
  verification inventory/seal remained
  `e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366` /
  `2cded484b787daa903ebf457f3f96bb9520af122bd48114300d78e543f39ccb8`.
  The candidate self-verifier passed.
- The immediate publication gate fetched `origin/main` without tags and
  required exact launcher/controller HEAD and remote main
  `4bcd73d5ae35517cd5fd09ed0a6dca8d1af43f2b`, whose exact parent is the
  independently reviewed local-candidate record
  `13973d8952d5de3ad20c5e13a7e6a419c8d8b9e2`. It proved the exact source
  object/tree and ancestry from `origin/main`; local and remote candidate and
  attestation refs absent; every local and remote `*rc.5*` tag absent; the RC.5
  workflow absent locally and on `origin/main`; and all eight RC.1 through
  RC.4 candidate/attestation refs at their sealed identities.
- Two read-only fail-closed harness checks stopped before publication. The
  first incorrectly treated the local-candidate record as current launcher
  HEAD; inspection proved the expected directly-descendant launcher commit.
  The second sorted remote-ref rows by SHA rather than the already documented
  ref-name normalization; the diff showed the same eight identities. The
  corrected checks were rerun from the beginning, and the candidate ref was
  still absent before the sole push.
- The only external mutation used raw Git with the required empty-expected
  lease at `2026-07-19T17:33:42Z`:
  `git push --force-with-lease=refs/heads/level1-v0.1.0-rc.5: origin
  19c1ef4b6a610016487880aa8ad69ec0204bd4f7:refs/heads/level1-v0.1.0-rc.5`.
  Exact remote readback is
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` at
  `refs/heads/level1-v0.1.0-rc.5`. No existing ref was moved or deleted.
- The public GitHub Actions REST API identified exactly one matching `ci.yml`
  run: run `29697069305`, run number `42`, attempt `1`, event `push`, branch
  `level1-v0.1.0-rc.5`, exact head SHA
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, status `completed`, conclusion
  `success`, URL
  `https://github.com/ponchione/revolvr/actions/runs/29697069305`. It was
  created and started at `2026-07-19T17:33:44Z` and completed at
  `2026-07-19T17:36:59Z` within the finite polling bound.
- The jobs endpoint returned exactly the ten mandatory unique jobs. Each had
  head SHA `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, status `completed`, and
  conclusion `success`:
  - `88219542577` — Go 1.22 source floor and tests —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542577`
  - `88219542546` — Production autonomous strict-fake suite —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542546`
  - `88219542556` — Race tests —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542556`
  - `88219542557` — Vet and module verification —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542557`
  - `88219542574` — Fake-Codex success smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542574`
  - `88219542580` — Fake-Codex verification-failure smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542580`
  - `88219542581` — Build linux/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542581`
  - `88219542568` — Build darwin/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542568`
  - `88219542566` — Build freebsd/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542566`
  - `88219542586` — Build Windows diagnostic stub —
    `https://github.com/ponchione/revolvr/actions/runs/29697069305/job/88219542586`
- Post-CI preservation reverified both RC.5 seals, all ten available sealed
  historical candidate/verification inventories, all four historical
  attestation workflow hashes, all eight historical remote refs, and the
  terminal RC.4 collector bundle for operation `ext20-2bd21aea4f72-01`.
  The full 40-target historical content/metadata fingerprint was identical
  before and after at
  `56b3fd0c61c2dd7842597ceb0fa46e66216c61317b5477cd3e326fa671416ef3`.
  The RC.5 attestation ref, tag namespace, and workflow remain absent.
- Files changed: this file, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`
  only. `.agent/TASKS.md` is unchanged. No source, dependency, workflow,
  candidate bundle, verification bundle, historical evidence, or local tag
  changed; no `main` commit or push occurred.
- Verification commands run: complete sealed-inventory/file-set checks and
  candidate self-verification; historical inventory, workflow, terminal-suite,
  operation, and recursive preservation checks; raw-Git no-tags fetch,
  ancestry/collision/ref-preservation checks, empty-expected publication and
  exact readback; finite public-REST run polling; exact ten-job REST
  comparison; and final diff/worktree checks. Result: passed with no blocker.
  No `gh`, attestation workflow/ref, suite, live/nested model operation, tag,
  release, external-use approval, or `EXT-20` completion occurred.
- What remains: in a separate fresh bounded pass, construct and locally verify
  a collision-free RC.5 attestation workflow that checks out exact candidate
  source `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, uses exact Go `1.26.5`,
  performs two clean release builds, checks the three sealed artifact hashes
  and embedded identities, and retains one exact remote artifact only after a
  later separately reviewed publication step.

## EXT-20 RC.5 Independent Local Review And Handoff (2026-07-19)

- Independent controller review reverified the 15-file candidate and 44-file
  verification bundle, their inventory seals, exact source commit/tree,
  artifact and build-instructions hashes, binary build metadata using the
  sealed absolute paths, empty Go build IDs, and exact `main.version` symbols.
  The full `go test ./...` suite and focused `go test -race ./internal/app`
  both passed. `git diff --check` passed.
- The durable local-candidate result was committed and raw-Git pushed to
  `origin/main` as `13973d8952d5de3ad20c5e13a7e6a419c8d8b9e2` (`Record local RC.5
  candidate`). Exact candidate source remains
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, not the controller commit.
- Local and remote `refs/heads/level1-v0.1.0-rc.5` remained absent after that
  publication. `agent-ext20-rc5-remote.sh` is prepared for the next bounded
  pass: reverify authority, create only that collision-free candidate ref with
  an empty-expected raw-Git lease, and require its exact push-triggered ten-job
  CI run. It must stop before attestation, suite preparation, any model call,
  tag, release, external-use approval, or `EXT-20` completion.

## EXT-20 RC.5 Replacement Candidate — Local Verification (2026-07-19)

- Task selected: construct and locally verify collision-free candidate
  `level1-v0.1.0-rc.5` from exact published source commit
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7`, tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`. The source commit was clean,
  published, and reachable from `origin/main`; controller-only helper
  `agent-ext20-rc5.sh` was absent from the source and was not run or copied.
- Files changed: added the ignored settled build helper
  `.revolvr/release-candidates/build-level1-v0.1.0-rc.5.sh`, immutable candidate
  bundle `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61`, and
  separate immutable verification bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61-verification`;
  updated this state file, `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`.
  `.agent/TASKS.md` is unchanged and `EXT-20` remains unchecked. No product
  source, dependency, workflow, ref, tag, suite, or historical evidence changed.
- Collision and source authority passed before construction: local and remote
  RC.5 candidate/attestation refs, RC.5 tags, bundle paths, workflow, remote
  artifact name, run root, and diagnostic were absent. Exact Go authority was
  `/usr/local/go/bin/go` at `go1.26.5`; the source-floor toolchain resolved as
  `go1.22.12`.
- Verification passed from retained clean clone root
  `/tmp/revolvr-ext20-rc5-build.6Ci9vy/source-authority`: focused lifecycle
  routing, supervisor prompt/provenance, cycle fail-closed, Structured Outputs,
  production happy-path, and strict-fake regressions in ordinary and race
  modes; verbose focused proof; Go 1.22.12 and Go 1.26.5 full suites; `go vet
  ./...`; `go mod verify`; and reachable/imported vulnerability scans. There
  are zero reachable or imported-package vulnerabilities; the sole module-only
  result remains Windows-only, uncalled `GO-2026-5024` in
  `golang.org/x/sys@v0.30.0`, fixed in v0.44.0.
- The settled EXT-18 build procedure produced Linux, Darwin, and FreeBSD amd64
  binaries with Go 1.26.5, release version `0.1.0`, `CGO_ENABLED=0`, trimpath,
  VCS metadata, and empty build IDs. Two independent `git clone --no-local`
  builds were byte-identical. Independent inspection confirmed exact
  `main.version` symbol size 16, exactly one `0.1.0` string, version output
  `revolvr 0.1.0`, source revision, `vcs.modified=false`, tool, command path,
  target, and CGO authority.
- Artifact SHA-256 values are Linux
  `1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043`,
  Darwin
  `a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d`,
  and FreeBSD
  `f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6`.
  Build-instructions SHA-256 is
  `69e0e533258b88b810db465935e66c49fd4e294fb745fc13998115dc8951dcb8`.
- The candidate has 15 files, 13 inventoried payload entries, inventory
  SHA-256
  `ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88`,
  and inventory-seal SHA-256
  `8bf947efd3d7f6467d500f88278913c0bcf5dd922331e558d483176a777584ab`.
  The verification bundle has 44 files, 42 inventoried evidence entries,
  inventory SHA-256
  `e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366`,
  and inventory-seal SHA-256
  `2cded484b787daa903ebf457f3f96bb9520af122bd48114300d78e543f39ccb8`.
- RC.1 through RC.4 candidate/evidence inventories reverified before and after
  construction. RC.4 suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite`, operation
  `ext20-2bd21aea4f72-01`, both terminal collector bundles, all historical
  remote refs, and all sentinels remained unchanged. The 40-target historical
  content/layout baseline SHA-256 is
  `b0adbd4c9082ca10a9c344bc0f1cdc24458a23da77db274b98bd27e5af6c38b2`;
  post-construction content/ctime and sealed inventories passed. The one
  evidence-harness stop was repaired by normalizing remote-ref order before
  comparison; ref identities did not change and no candidate authority changed.
- Result: local RC.5 candidate construction passed with no blockers. What
  remains is an independent read-only review, then separately authorized raw-
  Git publication of the still-absent `refs/heads/level1-v0.1.0-rc.5` at the
  exact source commit and remote CI on that exact ref. No remote or live-model
  acceptance is claimed.

## EXT-20 Lifecycle-Authority Controller Review And Publication (2026-07-19)

- Independent controller review found the repair bounded to the lifecycle
  authority projection, supervisor prompt/provenance, cycle handoff, and focused
  tests. Runtime lifecycle enforcement remains fail-closed; pending still
  admits only `plan`, `block`, and `needs_input`, and the model receives that
  exact ordered authority before deciding. No Structured Outputs, downstream
  action/profile, attempts, verification, audit, source-lock, or commit gate was
  weakened.
- Controller verification passed formatting; focused and race tests for
  `internal/autonomouspolicy`, `internal/supervisor`, and
  `internal/autonomouscycle`; production `TestProductionAutonomousHappyPath`
  and `TestStrictFakeCodexContract` in ordinary and race modes; `go test
  -count=1 ./...`; and `git diff --check`.
- RC.4 terminal preservation passed again: collector manifest, all 112
  inventoried files, evidence and whole-suite fingerprints, both candidate
  inventories, exact remote candidate/attestation refs, clean repository
  heads, and sentinels remained unchanged.
- Raw Git committed the reviewed repair and local state as
  `19c1ef4b6a610016487880aa8ad69ec0204bd4f7` (`Expose lifecycle routing
  authority to supervisor`), tree
  `2fb39c93694e72d986e7a8a849a542fc1bf1728d`, after a fresh `origin/main`
  parent check, then pushed it to `main`. This exact repair commit is the only
  allowed RC.5 source; the later `agent-ext20-rc5.sh` launcher/state commit is
  controller authority only.
- No candidate, ref, workflow, remote CI, attestation, suite, live/nested model
  operation, tag, release, external-use approval, or `EXT-20` completion
  occurred. The next bounded pass is collision-free local RC.5 construction.

## EXT-20 RC.4 Lifecycle-Authority Remediation (2026-07-19)

- Task selected: the bounded no-model lifecycle-authority remediation within
  unchecked `EXT-20`. RC.4 operation `ext20-2bd21aea4f72-01` remains
  `unsafe_or_ambiguous`: one supervisor decision selected `implement` from a
  `pending` lifecycle with no plan, while zero worker attempts, verification,
  audits, corrections, commits, or source changes occurred. The runtime gate
  remains unchanged in authority and still rejects `implement` from pending.
- Implementation: `internal/autonomouspolicy` now owns
  `autonomous-lifecycle-routing-authority-v1`, the single deterministic
  lifecycle-to-actions projection used by enforcement, prompt construction,
  and provenance. Pending exposes only `plan`, `block`, and `needs_input`;
  ready exposes the complete settled action vocabulary; every in-flight,
  suspended, finalizing, terminal, or unknown lifecycle fails closed before a
  supervisor call. The supervisor prompt renders the exact authority after the
  global profile, supervisor provenance advances to
  `revolvr-supervisor-provenance-v2`, and the cycle checks exact retained
  authority before accepting the supervisor result. Structured Outputs,
  action/profile validation, lifecycle enforcement, and all downstream gates
  remain intact.
- Files changed: `internal/autonomouspolicy/policy.go`,
  `internal/autonomouspolicy/policy_test.go`, `internal/supervisor/prompt.go`,
  `internal/supervisor/prompt_schema_test.go`,
  `internal/supervisor/execution.go`,
  `internal/supervisor/execution_test.go`,
  `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/cycle_test.go`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No suite, candidate,
  workflow, dependency, configuration, release, or task-backlog content
  changed; `EXT-20` remains unchecked.
- Verification passed:
  - `gofmt -w` on all eight changed Go files;
  - focused lifecycle/prompt/provenance/cycle regressions across
    `./internal/autonomouspolicy`, `./internal/supervisor`, and
    `./internal/autonomouscycle`;
  - `go test -count=1 ./internal/autonomouspolicy ./internal/supervisor
    ./internal/autonomouscycle`;
  - `go test -race -count=1 ./internal/autonomouspolicy
    ./internal/supervisor ./internal/autonomouscycle`;
  - `go test -count=1 ./internal/app -run
    'Test(ProductionAutonomousHappyPath|StrictFakeCodexContract)$'` and the
    same command with `-race`;
  - `go test -count=1 ./...`;
  - `git diff --check`.
- RC.4 preservation passed before editing and after all code checks. The
  collector manifest and all 112 bundle files verify; manifest SHA-256 remains
  `33a6e800fdd32b0e5873f3c59b2d90d4d47d73ae93f6700acf572e88bbd85a23`,
  inventory SHA-256 remains
  `81028ea618dee019fb37b95e91ac0863d105b31426893b10e633798ecca5d43b`,
  evidence content remains
  `b253ebb96f8c6e7989db20fa820aa14fd12f323b910a73aaae039d4fa2fbdc9a`,
  whole-suite content remains
  `a44d88d7419db1d6b325daaf792dd775fe46523d63e42f9503289e0059b7c2e2`,
  and the fresh pre/post metadata/layout fingerprint remains
  `7e1d80bb37022da5874c01db09cf07cefb6e39b86941117f75efc8dc69d0b722`.
  Candidate and verification inventory SHA-256 values remain
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`
  and
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`;
  RC.4 refs and workflow hash remain exact. Both disposable repositories stay
  clean at their recorded heads and the sentinel comparison remains exact.
- Result: the bounded repair and its independent publication gate are complete
  with no blocker. Exact published commit/tree authority is recorded above.
  A fresh collision-free local RC.5 candidate may now be constructed, but it
  must not reuse RC.4 suite, operation, evidence, refs, workflows, artifacts,
  identities, or failed authority.

## EXT-20 RC.4 Terminal Live Failure And Root Cause (2026-07-19)

- Exact suite `/tmp/revolvr-ext20-rc4.DGg1pW/suite` entered its first guarded
  live operation and stopped immediately. Operation
  `ext20-2bd21aea4f72-01`, task `ext20-success-a1`, scenario
  `successful-source-change-1`, expected `completed`, observed
  `unsafe_or_ambiguous`, exit status 1. Terminal bundle:
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite/evidence/repo-a/01-successful-source-change-1`.
- Supervisor run `019f7afa-562a-732e-948a-920096198000` and Codex thread
  `019f7afa-5a6e-7c11-8f05-4e1ec8541a3e` produced a structurally valid
  `implement` / `implementer` decision with a bounded one-file strategy. The
  durable task lifecycle was `pending` with no plan. Runtime policy correctly
  failed closed: `pending lifecycle admits only "plan", "block", or
  "needs_input", not "implement"`.
- Root cause: the supervisor profile lists every global action and the dossier
  shows `Lifecycle: pending`, but the model-facing prompt does not state exact
  lifecycle-admitted actions. The model therefore lacked authority necessary
  to distinguish a reasonable implementation recommendation from an illegal
  route. No model coercion, retry, policy weakening, or dogfood exception is
  authorized; lifecycle authority must be communicated from a deterministic
  source shared with enforcement.
- Independent verification passed the collector's `--verify-manifest`, the
  bundle checksum and all 112 inventoried files, ledger export/replay evidence,
  exact candidate/Codex/config identities, and terminal operation/state JSON.
  Manifest SHA-256 is
  `33a6e800fdd32b0e5873f3c59b2d90d4d47d73ae93f6700acf572e88bbd85a23`;
  `files.sha256` and recorded bundle SHA-256 are
  `81028ea618dee019fb37b95e91ac0863d105b31426893b10e633798ecca5d43b`;
  evidence content fingerprint is
  `b253ebb96f8c6e7989db20fa820aa14fd12f323b910a73aaae039d4fa2fbdc9a`;
  whole-suite content fingerprint is
  `a44d88d7419db1d6b325daaf792dd775fe46523d63e42f9503289e0059b7c2e2`.
- Exactly one collector manifest and no aggregate exist. The supervisor
  completed once; worker attempts, verification, audits, corrections, commits,
  and source changes are all zero. Control repo-a and its retained workspace
  remain clean at exact head
  `a75d4f059721ec7c9320650bd49d6d4cef9526cf`; repo-b remains clean and
  untouched at `11eb46ae242cf2a3cb5ce32cf94e0df3aab2ab0b`; before/after outside
  sentinel evidence is identical.
- RC.4 and this suite are immutable rejected history and must never be retried,
  reconciled, relabeled, mutated, or reused. `EXT-20` remains unchecked. The
  bounded no-model source repair is recorded above and its launcher must not
  be rerun. Independent review and separately authorized publication are next;
  RC.5 construction remains a later collision-free gate.

## EXT-20 RC.4 First Confirmed Live No-Start Diagnostic (2026-07-19)

- After the operator returned from the first confirmation-gated launcher, the
  controller found no repository diff and no live-suite activity. Exact root
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite` still contains zero operation
  manifests, zero collector manifests, an empty aggregate, and only its two
  unchanged preparation logs. Both disposable repositories remain clean on
  `main` at exact prepared heads; no evidence, operation, Codex run, source
  change, or terminal suite result exists.
- Read-only diagnosis reconfirmed the exact prepared regular-file content
  fingerprint
  `5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305`,
  authority and plan, candidate/Codex identities, repository heads, sentinels,
  and zero-runtime state. No root mutation or model retry occurred.
- Because the orchestration wrapper retained no failure diagnostic, it is
  retired. The replacement `agent-ext20-rc4-live-direct.sh` requires a new
  exact confirmation argument, performs deterministic raw-Git, bundle,
  content, identity, repository, sentinel, and zero-runtime checks, and then
  directly executes the guarded suite once. Any preflight drift stops before
  live work. Any later suite failure or interruption remains terminal.
- `EXT-20` remains unchecked. No tag, release, or external-use approval exists.

## EXT-20 RC.4 Prepared-Suite Controller Verification (2026-07-19)

- Independent controller inspection confirmed the implementation diff changes
  exactly the RC.3-to-RC.4 source commit, Linux hash, and bundle constants in
  `scripts/dogfood-external-level1-suite.sh`; the release, Codex, plan,
  scenarios, thresholds, configuration, and confirmation gate did not change.
- Verification reran `bash -n` for the suite and collector, the complete RC.4
  bundle verifier, suite `--static`, `go test -count=1 ./...`, and
  `git diff --check`; all passed without a model call. Read-only retained-root
  checks reconfirmed `prepared.sha256`, authority/plan hashes, candidate hash
  and version, Codex hash and version, exact clean repo-a/repo-b heads, zero
  operation and collector manifests, and an empty aggregate.
- Raw Git fetched and reverified `origin/main`, exact RC.4 candidate ref
  `2546913e38ec273f64417dece2f91df78fd42fc2`, and exact attestation ref
  `52c2db07a86677e67921bcbfbcbdf26397b47615`. The suite-authority and durable
  changes were committed as
  `3284971acfc542fa64d600f7c40a58891b16cb7c` (`Bind Level 1 suite to RC.4
  candidate`) and pushed to `main` after a fresh remote-parent check.
- No model call, live operation, evidence mutation, retry, tag, release, or
  external-use approval occurred. The next separately confirmed pass must use
  `agent-ext20-rc4-live.sh` with its exact confirmation argument. `EXT-20`
  remains unchecked until live and retained-evidence verification pass.

## EXT-20 RC.4 Guarded No-Model Suite Preparation (2026-07-19)

- Task selected: the bounded RC.4 no-model suite-preparation subtask of
  `EXT-20`. The guarded suite now binds exact source
  `2546913e38ec273f64417dece2f91df78fd42fc2`, Linux SHA-256
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec`. Release
  output remains `revolvr 0.1.0`; Codex authority remains exact package
  `@openai/codex@0.144.4`, output `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, confirmation value, Go
  source, or dependency changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. The implementation change is
  exactly three shell constants. `.agent/TASKS.md` remains unchanged with
  `EXT-20` unchecked.
- Verification passed: `bash -n` for the suite and collector; the complete
  RC.4 candidate-bundle verifier; suite `--static`; two collector
  `--fixture-only` collections; both `--verify-manifest` checks; canonical
  manifest comparison after excluding only `collected_at_utc`; `go test
  -count=1 ./...`; and `git diff --check`. The retained collector fixture root
  is `/tmp/revolvr-ext20-rc4-collector.5SFvwa`; both raw manifest SHA-256
  values are
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`
  and the timestamp-excluded canonical SHA-256 is
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- A new parent was created with exact template
  `/tmp/revolvr-ext20-rc4.XXXXXX`. The retained prepared suite is
  `/tmp/revolvr-ext20-rc4.DGg1pW/suite`; preparation used only `--prepare
  --run-root /tmp/revolvr-ext20-rc4.DGg1pW/suite --install-codex`, installed
  the pinned package, and started no model. Suite ID is
  `ext20-2bd21aea4f72`; `authority.tsv` SHA-256 is
  `4f9b653c9e62e5fc5932b219952bbe61fccd79d331ac2bd7fcf2c570035eacb7`
  and `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Independent read-only inspection verified the prepared-authority checksum;
  exact candidate path/hash/version/source and clean VCS metadata; exact
  Codex package/path/hash/version; effective schema v8; and source-writer
  authority `timeout=32m0s heartbeat_interval=10m40s required=32m0s`. The
  unchanged 11-row plan has ten unique ready tasks. Both tracked disposable
  repositories are clean on `main`: repo-a has six ready tasks at
  `a75d4f059721ec7c9320650bd49d6d4cef9526cf`; repo-b has four ready tasks at
  `11eb46ae242cf2a3cb5ce32cf94e0df3aab2ab0b`.
- There are zero runtime operation manifests, zero collector manifests, and
  zero aggregate entries. Inspection used `GIT_OPTIONAL_LOCKS=0`; complete
  regular-file content fingerprint
  `5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305`
  and layout/type/mode/owner/size/link/inode/mtime/ctime fingerprint
  `5e52e1be955403644fd33ee2b95c832896994305f95806ebac533ec93525244f`
  matched before and after. The empty-confirmation guard was exercised in
  isolation and its source order proves refusal before prepared-root reads;
  neither `--live` nor the confirmation value was passed to the suite.
- The first inspection harness stopped on an overly specific config-rendering
  assertion that expected the doctor-only `required` field on the config-check
  line. The one read-only repair asserted config check's exact timeout and
  heartbeat plus doctor's exact required window, then reran the complete
  fingerprinted inspection successfully; the prepared suite never changed.
- Preservation passed. All ten available RC.1/RC.2/RC.3/RC.4 sealed
  inventories, all four attestation-workflow hashes, and all eight raw remote
  candidate/attestation refs reverified exactly. No retired suite was present,
  recreated, reused, reconciled, or relabeled, and no historical ref, bundle,
  workflow, hash, artifact record, diagnostic, or available root changed.
- Verification result: passed. No `gh`, commit, push, tag, live or nested
  Codex/model operation, release, or external-use approval occurred. There are
  no blockers for this bounded preparation task. What remains is a separately
  confirmed live pass using exactly:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc4.DGg1pW/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. That command was recorded but not run;
  `EXT-20` remains unchecked.

## EXT-20 RC.4 Remote Artifact Attestation Result (2026-07-19)

- Independent controller review passed the workflow's PyYAML structure and
  exact trigger/job constants, extracted embedded-shell `bash -n`, workflow
  SHA-256
  `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`,
  retained 29-file output shape, and all six sealed `SHA256SUMS` rows. The
  candidate ref remained exact and the attestation ref/tag namespace was
  empty immediately before publication.
- Raw Git committed the reviewed workflow and local state on `main` as
  `52c2db07a86677e67921bcbfbcbdf26397b47615` (`Add RC.4 artifact attestation
  workflow`) and pushed it after a fresh parent check. An empty-expected
  force-with-lease then created only
  `refs/heads/level1-v0.1.0-rc.4-attestation` at that workflow commit. Remote
  readback matched `main` and the new attestation ref at the workflow commit,
  while `refs/heads/level1-v0.1.0-rc.4` remained exact candidate source
  `2546913e38ec273f64417dece2f91df78fd42fc2`.
- Dedicated run `29690065853`, `Level 1 RC.4 candidate attestation`, event
  `push`, branch `level1-v0.1.0-rc.4-attestation`, head SHA
  `52c2db07a86677e67921bcbfbcbdf26397b47615`, completed with `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853`. Its sole job
  `88201098277`, `Rebuild and attest Level 1 RC.4 candidate`, completed with
  `success`:
  `https://github.com/ponchione/revolvr/actions/runs/29690065853/job/88201098277`.
- The run retained exactly one unexpired artifact: ID `8443312175`, name
  `level1-v0.1.0-rc.4-attestation`, size 70,214,949 bytes, digest
  `sha256:0a3567ec0fbc31aff65424790402f81a20df3f22c49659854993dcbeb1eb8fbc`,
  created/updated `2026-07-19T14:05:56Z`, expiring
  `2026-10-17T14:03:12Z`. Its archive endpoint is
  `https://api.github.com/repos/ponchione/revolvr/actions/artifacts/8443312175/zip`.
  Public unauthenticated download returned HTTP 401 and neither `GH_TOKEN` nor
  `GITHUB_TOKEN` was available, so controller-side archive comparison was not
  possible. The successful remote job itself performed the two clean rebuilds,
  exact sealed hash/identity assertions, pair comparisons, final checksum
  check, and authority readback before upload.
- The same push's ordinary CI run `29690065840` completed with `success` at the
  exact workflow commit:
  `https://github.com/ponchione/revolvr/actions/runs/29690065840`. Exactly ten
  jobs were returned and every job completed successfully: IDs `88201098140`,
  `88201098145`, `88201098159`, `88201098170`, `88201098176`, `88201098180`,
  `88201098187`, `88201098200`, `88201098201`, and `88201098223`.
- No candidate ref, source, bundle, historical workflow/ref/evidence, tag,
  release, retired suite, or external-use decision changed. No `gh` or
  live/nested model operation ran. `EXT-20` remains unchecked. RC.4's
  candidate-ref, remote-CI, and artifact-attestation prerequisites are now
  complete; the next bounded pass is fresh collision-free no-model suite
  preparation through `agent-ext20-rc4-suite.sh`.

## EXT-20 RC.4 Remote Artifact Attestation Workflow (2026-07-19)

- Task selected: the bounded RC.4 release-artifact attestation-workflow
  prerequisite of `EXT-20` only. `EXT-20` remains unchecked.
- New separate workflow
  `.github/workflows/level1-rc4-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.4-attestation`. It checks out exact candidate
  source `2546913e38ec273f64417dece2f91df78fd42fc2` and tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, not trigger HEAD, and requires
  the published candidate ref to remain at that source while the triggering
  attestation ref and complete RC.4 ref/tag namespace match exact authority.
  The locally reviewed workflow SHA-256 is
  `340b82093d469e86e2e27e4729a51caa1da88f814017d6f6ab1bcabd89a56101`.
- The workflow installs exact Go 1.26.5 with setup cache disabled, makes two
  independent clean `--no-local` clones, and gives each pass separate
  `GOCACHE` and `GOMODCACHE` paths. Both passes build Linux, Darwin, and
  FreeBSD amd64 with disabled CGO, local toolchain selection, module-readonly
  mode, `-trimpath`, explicit VCS metadata, an empty build ID, and exact
  `main.version=0.1.0`.
- Every artifact must expose exact Go version, command path, `gc` compiler,
  trimpath, target, CGO, Git source revision, and `vcs.modified=false`
  metadata; an empty retained build-ID result; exactly one 16-byte
  `main.version` data symbol; and exactly one `0.1.0` string. Both Linux
  copies must print exactly `revolvr 0.1.0`.
- Each pass must reproduce sealed SHA-256 values Linux
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
  Corresponding pass artifacts must also be byte-identical.
- One upload named `level1-v0.1.0-rc.4-attestation` retains both binary sets,
  all six build metadata files, empty build-ID records, per-binary version
  authority, both Linux version outputs, the six-row hash manifest, the
  three-pair reproducibility table, and an exact authority manifest naming the
  workflow path, ref/namespace, artifact, candidate ref/ID/source/tree,
  toolchain, environment, flags, passes, targets, and hashes.
- Collision checks passed before construction. Raw Git read `origin/main` and
  local `HEAD` at launcher commit
  `e6cd453931b5fa1c0261e984749425e4409b0bb0`; remote candidate ref
  `refs/heads/level1-v0.1.0-rc.4` remained exact at the candidate SHA; proposed
  ref `refs/heads/level1-v0.1.0-rc.4-attestation`, every RC.4 tag, and the new
  workflow path on both remote main and candidate source were absent. Public
  REST listed only the three historical RC.1/RC.2/RC.3 artifacts, so the
  exact RC.4 artifact name and namespace were absent.
- Verification passed: PyYAML `BaseLoader` structure and exact-constant
  assertions; extracted embedded-shell `bash -n`; workflow-only whitespace
  checks; and execution of the complete unmodified embedded shell under host
  Go 1.26.5 from a detached exact-source clone with a collision-safe local
  raw-Git ref fixture. The shell created two further clean non-local clones
  and independently downloaded/built through separate caches. Retained-output
  inspection found exactly 29 regular single-link files, six exact hash rows,
  three identical pairs, six empty build-ID records, six exact version
  authorities, two exact Linux outputs, and complete authority and
  reproducibility manifests. The retained verification root is
  `/tmp/revolvr-ext20-rc4-attestation.Y4TLEM`; the relative-file/hash stream
  digest is
  `1c08a35517d12e1993184143c43a97c753644d1f0dae68de1cbd2a59ee07e4b9`.
- The first complete-shell harness invocation correctly failed before any
  build because the harness started Bash in controller `main` rather than the
  detached candidate workspace. The single harness repair changed only the
  invocation working directory and reran the same extracted shell unchanged;
  the successful retained root above contains the complete result. No
  workflow repair or candidate byte change was required.
- Preservation passed. Historical workflow SHA-256 values remain RC.1
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`,
  RC.2
  `4c96ec62a3757878926b62aee65ce8ba3ec6ac2148ac251622ab294109312c6d`,
  and RC.3
  `76b0b9c3e683a4df5fa4103588ea668459b00efa18eee4930baa81c440df4da6`.
  All ten available RC.1/RC.2/RC.3/RC.4 inventory manifests and contents
  reverified. All six historical remote refs remained exact, the RC.4
  candidate ref remained exact, and the four retired suite roots remained
  absent. No historical workflow, ref, bundle, artifact, hash, suite,
  operation, run, or diagnostic was reused or changed.
- Files changed: the new RC.4 workflow, this file, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. No Go source or dependency changed, so no Go test was required.
  No `gh`, commit, push, remote CI, suite preparation, live/nested Codex or
  model operation, candidate source/ref mutation, tag, release, or
  external-use approval occurred. Blockers for this bounded local task: none.
- Collision-free raw-Git ref reserved for the controller's later reviewed
  publication is `refs/heads/level1-v0.1.0-rc.4-attestation`. The controller
  must independently verify and commit this workflow, publish only that absent
  ref with an empty-expected lease while preserving the candidate ref, require
  the exact remote job to pass, and record workflow commit/ref, run/job
  IDs/URLs/status/conclusions plus artifact ID/name/size/digest/creation/expiry
  and retained-archive comparison. No-model suite preparation and live
  quantitative dogfood remain later gates.

## EXT-20 RC.4 Remote-CI Verification And Attestation Handoff (2026-07-19)

- Independent controller raw-Git and public-REST readback reconfirmed exact
  candidate ref `level1-v0.1.0-rc.4` at
  `2546913e38ec273f64417dece2f91df78fd42fc2`, run `29688941202` as
  `completed` / `success` for that push branch/SHA, and exactly ten distinct
  mandatory jobs as `completed` / `success` on the same SHA.
- Both RC.4 inventories, the candidate self-verifier, and all eight historical
  inventories passed again. The remote-CI state was committed as
  `8c0379aa3fb6824fb56d4f3c1180f4cc411ada2a` (`Record RC.4 remote candidate
  CI`) and pushed to raw-Git `origin/main`.
- `agent-ext20-rc4-attestation.sh` is the next controller-only launcher. It is
  limited to local construction and complete local verification of the
  collision-free exact-checkout Go 1.26.5 RC.4 attestation workflow. It grants
  no commit, push, remote workflow/ref, suite, live model, tag, release, or
  external-use authority.
- What remains: run `./agent-ext20-rc4-attestation.sh`, then independently
  verify and publish its reviewed workflow/ref in a later controller step.
  `EXT-20` remains unchecked.

## EXT-20 RC.4 Exact-Candidate Remote CI (2026-07-19)

- Task selected: the bounded RC.4 candidate-ref and remote-CI subtask of
  `EXT-20` only. `EXT-20` remains unchecked.
- Before publication, both sealed RC.4 inventories, the candidate
  self-verifier, candidate ID `level1-v0.1.0-rc.4`, release `0.1.0`, source
  commit `2546913e38ec273f64417dece2f91df78fd42fc2`, source tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, and all three artifact hashes
  reverified exactly. Candidate inventory SHA-256 remained
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`;
  verification inventory SHA-256 remained
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`.
- The immediate pre-publication raw-Git gate fetched `origin` without tags,
  proved the exact source object/tree and ancestry from fetched
  `origin/main` at `af123c7ce38e41982a2302d76cb7e2fa6bdf5608`, proved remote
  candidate and attestation refs absent, proved every remote tag matching
  `*rc.4*` absent, and reread all six historical RC.1/RC.2/RC.3 refs at their
  sealed identities.
- The sole external mutation used the required empty-expected lease:
  `git push --force-with-lease=refs/heads/level1-v0.1.0-rc.4: origin
  2546913e38ec273f64417dece2f91df78fd42fc2:refs/heads/level1-v0.1.0-rc.4`.
  Remote readback returned exactly
  `2546913e38ec273f64417dece2f91df78fd42fc2` at
  `refs/heads/level1-v0.1.0-rc.4`. No existing ref was updated or deleted.
- The public GitHub Actions REST API identified exactly one matching workflow
  run: run `29688941202`, event `push`, branch
  `level1-v0.1.0-rc.4`, head SHA
  `2546913e38ec273f64417dece2f91df78fd42fc2`, status `completed`, conclusion
  `success`, URL
  `https://github.com/ponchione/revolvr/actions/runs/29688941202`. It started
  at `2026-07-19T13:28:34Z` and completed at `2026-07-19T13:31:54Z`.
- The jobs endpoint returned exactly the ten mandatory distinct jobs. Every
  job reported the exact candidate head SHA and `completed` / `success`:
  - `88198118677` — Go 1.22 source floor and tests —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118677`
  - `88198118646` — Production autonomous strict-fake suite —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118646`
  - `88198118664` — Race tests —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118664`
  - `88198118665` — Vet and module verification —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118665`
  - `88198118653` — Fake-Codex success smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118653`
  - `88198118661` — Fake-Codex verification-failure smoke —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118661`
  - `88198118641` — Build linux/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118641`
  - `88198118668` — Build darwin/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118668`
  - `88198118681` — Build freebsd/amd64 —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118681`
  - `88198118662` — Build Windows diagnostic stub —
    `https://github.com/ponchione/revolvr/actions/runs/29688941202/job/88198118662`
- Post-CI preservation reverified both RC.4 inventories, all eight sealed
  historical inventories, the exact contents of all three historical
  attestation workflows, all six historical remote refs, exact RC.4 candidate
  readback, RC.4 attestation-ref/tag absence, and all four recorded retired-
  suite absences. RC.1, RC.2, and RC.3 evidence remained immutable.
- Files changed: `.agent/STATE.md`, `.agent/DECISIONS.md`, and
  `.agent/HANDOFF.md` only. No source, dependency, workflow, candidate bundle,
  verification bundle, task entry, or historical evidence changed. No commit
  or push of `main` occurred.
- Verification commands run: candidate `build-instructions.sh --verify`;
  strict SHA-256 checks for both RC.4 inventories and all eight historical
  inventories; exact artifact/source/tree/manifest assertions; raw-Git fetch,
  ancestry, collision, force-with-lease publication, ref readback, and
  preservation checks; finite public REST run polling; exact ten-job REST
  comparison; `git diff --check`; and final worktree inspection.
- Verification result: passed. No `gh`, attestation workflow/ref, suite,
  live/nested Codex or model operation, tag, release, external-use approval,
  or `EXT-20` completion occurred. Blockers for this bounded task: none.
- What remains: in a separate fresh, explicitly authorized pass, add and
  publish a collision-free RC.4 attestation workflow/ref that checks out this
  exact candidate SHA, uses exact Go 1.26.5, reproduces two clean build passes,
  verifies the three sealed artifact hashes and embedded identities, and
  retains the remote artifact/run evidence. No-model suite preparation and
  live quantitative dogfood remain later gates.

## EXT-20 RC.4 Independent Verification And Remote Handoff (2026-07-19)

- Independent controller verification passed both sealed RC.4 inventories,
  exact inventory digests, the candidate self-verifier, all three artifact
  hashes and Go/VCS metadata, source tree and published ancestry, candidate
  ref/attestation-ref/tag collision absence, retained vulnerability and test
  results, eight historical inventories, six historical remote refs, the
  focused Structured Outputs and production happy-path tests, and a fresh
  `go test -count=1 ./...`.
- The local candidate state was committed as
  `1917df5c374f8337a7bebb429478e7e16ea8420d` (`Record reproducible RC.4
  candidate`) and pushed to raw-Git `origin/main`; no candidate ref, workflow,
  CI request, suite, live model operation, tag, release, or external-use
  approval was created by that publication.
- `agent-ext20-rc4-remote.sh` is the next controller-only launcher. Executing
  it supplies explicit authority only to create absent remote ref
  `refs/heads/level1-v0.1.0-rc.4` at exact source
  `2546913e38ec273f64417dece2f91df78fd42fc2` with an empty-expected raw-Git
  force-with-lease and to collect the complete ten-job EXT-15 CI result on that
  SHA. It forbids main publication, attestation, suite preparation, live model
  work, tags, release, and external-use approval.
- What remains: run `./agent-ext20-rc4-remote.sh`, then independently verify
  its ref/CI evidence before committing state. `EXT-20` remains unchecked.

## EXT-20 RC.4 Replacement Candidate — Local Verification (2026-07-19)

- Task selected: the bounded RC.4 local replacement-candidate subtask of
  `EXT-20` only. Candidate `level1-v0.1.0-rc.4` binds release version `0.1.0`
  to exact source commit `2546913e38ec273f64417dece2f91df78fd42fc2`
  and tree `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`. A raw-Git fetch,
  object/tree readback, remote-main readback, and ancestry check proved the
  source is published and reachable from `origin/main` at
  `45a7f2384aaf21e36174660618c5f00a91edb1ab`. The controller helper is absent
  from the source commit and clean source clone.
- Collision checks passed before construction. Local and remote candidate and
  attestation refs, RC.4 tags, candidate/verification bundles, build script,
  local and `origin/main` workflow, public Actions artifact name
  `level1-v0.1.0-rc.4-attestation`, `/tmp` run root, and RC.4 diagnostic names
  were all absent. The retained construction root is
  `/tmp/revolvr-ext20-rc4-build.okV2nU`; it is diagnostic workspace, not
  candidate authority.
- The required first local regressions passed on the exact clean source:
  `TestProductionModelOutputSchemasUseSupportedStructuredOutputsSubset` and
  `TestProductionAutonomousHappyPath`. These results are retained in the
  verification bundle and establish local compatibility/regression proof only;
  they do not claim live API acceptance.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/`. Its pinned
  build-instruction SHA-256 is
  `5d87ff8eb5e89865729237dda500c8387ef5880b3c10ea0bd77f896938d606e9`,
  and its 13-entry complete evidence inventory SHA-256 is
  `3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d`.
  Two independent `--no-local` clean clones produced byte-identical artifacts:
  Linux amd64
  `98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe`,
  Darwin amd64
  `042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab`,
  and FreeBSD amd64
  `128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6`.
- Every artifact independently records Go 1.26.5, command path
  `revolvr/cmd/revolvr`, compiler `gc`, trimpath, exact `GOOS/amd64`, disabled
  CGO, Git source revision, and `vcs.modified=false`. Each has an empty Go
  build ID, one exact 16-byte `main.version` symbol, and exactly one `0.1.0`
  release string; the Linux binary prints exactly `revolvr 0.1.0`.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec-verification/`.
  Its 31-entry complete evidence inventory SHA-256 is
  `75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c`.
  It retains exact source/tree/tool identities, commands and raw logs, the
  Structured Outputs guard, production happy path, Go-floor/full tests, vet,
  module and vulnerability results, candidate construction and self-check,
  independent build metadata/version assertions, collision checks, artifact
  authority, and before/after historical preservation evidence.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  Linux/Darwin/FreeBSD amd64 release builds; two-pass byte comparison; embedded
  metadata/version/build-ID assertions; candidate self-check; verification
  inventory; exact clean-clone authority; and helper exclusion. Govulncheck
  found zero reachable and zero imported-package vulnerabilities. The retained
  module-only finding remains `GO-2026-5024` in the Windows-only
  `golang.org/x/sys/windows` surface at `v0.30.0`, which Revolvr does not call;
  the report names `v0.44.0` as fixed.
- The first independent metadata command stopped on a read-only harness error:
  it indexed `go tool nm -size` symbol type/name as fields 4/5 instead of 3/4.
  The single repair corrected only that assertion and reran the complete check;
  no candidate byte changed. The exact diagnostic is retained as
  `metadata-check-repair.txt` in the verification bundle.
- Preservation passed. Eight available RC.1/RC.2/RC.3 inventories reverified;
  20 historical candidate, verification, failed-diagnostic, workflow, and
  retired-suite targets retained identical content and layout fingerprints;
  all six RC.1/RC.2/RC.3 remote refs retained exact identities. Retired RC.3
  suite `/tmp/revolvr-ext20-rc3.Qghf19/suite` was absent before and after and
  was not recreated or used. No historical suite, operation, run, ref,
  workflow, artifact, hash, bundle, or diagnostic became RC.4 authority.
- Files changed: this state file, `.agent/DECISIONS.md`, `.agent/HANDOFF.md`,
  the new ignored RC.4 pinned build script, candidate bundle, and verification
  bundle. `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked. No Go
  source or dependency changed.
- Verification result: local RC.4 construction passed after the one read-only
  harness repair. No `gh`, commit, push, candidate ref, workflow, remote CI
  request, attestation, suite, live/nested Codex or model operation, tag,
  release, or external-use approval occurred.
- What remains: after independent controller verification and explicit raw-Git
  publication authority, create only collision-free ref
  `refs/heads/level1-v0.1.0-rc.4` at the exact source commit and require every
  EXT-15 CI job to pass on that SHA. Remote artifact attestation and any new
  no-model/live suite remain later separate passes. Blockers for this bounded
  local task: none.

## EXT-20 Structured Outputs Repair Publication And RC.4 Handoff (2026-07-19)

- The operator explicitly authorized committing and pushing the already
  reviewed Structured Outputs follow-up with raw Git. The pre-publication base,
  fetched `origin/main`, and `FETCH_HEAD` all matched
  `45e92302843ad1cafe7a4a6bc58a319d606fb497`; no remote divergence existed.
- Verification passed immediately before publication: formatting of every
  changed Go file; the six-package focused suite; the same focused suite with
  `-race`; the production autonomous happy path; the recursive four-builder
  Structured Outputs guard; `go test -count=1 ./...`; staged and unstaged
  `git diff --check`; and exact staged-scope inspection.
- Raw Git created commit `2546913e38ec273f64417dece2f91df78fd42fc2`
  (`Make model output schemas strict-compatible`) with tree
  `8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5`, then pushed `main` from
  `45e9230` to `2546913`. A subsequent fetch confirmed local `HEAD` and
  `origin/main` matched exactly.
- The controller-only `agent-ext20-rc4.sh` binds local RC.4 construction to
  that exact source and tree. It forbids `gh`, commit, push, ref publication,
  remote attestation, suite preparation, live/nested model calls, tags,
  release, or external-use approval. It also forbids reuse or mutation of all
  RC.1, RC.2, and RC.3 authority and evidence.
- What remains: run one fresh bounded pass with `./agent-ext20-rc4.sh` to
  construct and locally verify collision-free RC.4. `EXT-20` remains unchecked;
  no API-acceptance or external-use claim is authorized.

## EXT-20 RC.3 Rejection, Failed First Repair Audit, And Follow-up (2026-07-18)

- Task selected: finish the Structured Outputs compatibility repair exposed by
  the first RC.3 live operation and the failed independent audit of the first
  repair. Work remains based on exact `main` commit
  `45e92302843ad1cafe7a4a6bc58a319d606fb497`; rejected candidate source is
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`.
- Failure authority: suite `/tmp/revolvr-ext20-rc3.Qghf19/suite`, operation
  `ext20-802d9db69596-01`, evidence bundle
  `/tmp/revolvr-ext20-rc3.Qghf19/suite/evidence/repo-a/01-successful-source-change-1`,
  and Codex run `019f761f-078d-7b81-932b-278339f2a000`. The API returned HTTP
  400 `invalid_json_schema`: `In context=('properties', 'conflicts'),
  'uniqueItems' is not permitted.` The terminal outcome is
  `unsafe_or_ambiguous`; statistics and captured state show zero worker
  attempts, zero verification runs, zero audits, zero commits, and no source
  mutation.
- The first repair removed emitted `uniqueItems`, `allOf`, `not`, and `oneOf`,
  moved set uniqueness to semantic Go validation, and passed focused, race,
  strict-fake production, and full-suite tests. Independent audit nevertheless
  rejected it. Authoritative findings were: every object must have
  `additionalProperties: false`; every property must appear exactly once in
  `required`; semantic optionals must be required-nullable or required-empty;
  unconstrained objects are prohibited; and the finite keyword denylist was
  not a supported-subset compatibility guard.
- The follow-up audits and repairs exactly the four ordinary production
  builders: supervisor `DecisionOutputSchema`, planner `PlanningOutputSchema`,
  auditor `AuditOutputSchema`, and corrector `CorrectionOutputSchema`. Every
  object and definition is concrete and closed, every declared property is
  required exactly once, every array has concrete `items`, and supported null
  or empty-array wire values preserve existing Go zero/nil meaning.
- Supervisor root optionals, `strategy`, `needs_input_question`, and
  `child_task` optional collections now use required null/empty forms. A
  nullable worker-profile enum includes JSON null. Required-null
  `content_sha256` still decodes to the Go zero value and `ParseDecision`
  computes and validates its deterministic content identity.
- Planning `plan_step.evidence`, `plan_step.rationale`,
  `task_plan.supersedes_plan_id`, `acceptance_criterion.evidence`, and
  `acceptance_criterion.rationale` preserve absence as empty slices, empty Go
  strings decoded from null, or the existing zero predecessor identity. Go
  status/revision validation still rejects fabricated rationale or authority.
- Audit `report.findings`, `verification_summary.command`, both tiered fields,
  `mutation.decision_id`, and `provenance.latest_source_mutation` now have exact
  required empty/nullable representations. The former unconstrained
  `verification_summary.tiered` is a closed model projection fixed to null;
  the host compares that projection and deterministically reattaches the exact
  trusted full tiered result before canonicalization and persistence. The
  optional final-gate projection has exact closed plan/tier definitions.
- Correction now requires every root property, models
  `VerificationFailureTarget` exactly, models every `EvidenceReference` item as
  a concrete closed object, and uses null versus empty `finding_ids` for the
  exclusive authority partition. Strict decoding and Go partition, outcome,
  evidence, duplicate, and authority validation remain unchanged.
- Semantic validation remains fail-closed without deduplication or reordering:
  supervisor and correction finding IDs, resolved/remaining correction IDs,
  child `depends_on`/`tags`/`conflicts`, needs-input option IDs and meanings,
  independent-work IDs and exact option identities, audit finding IDs, and
  audit verification-tier identity arrays all reject duplicates. Focused
  decoded-response and validator tests prove null/empty decoding and continued
  conditional validation.
- The stdlib-only recursive regression validator enumerates all four
  production builders and uses an explicit current-documentation allowlist. It
  distinguishes schema keywords from property and `$defs` names, checks every
  object including nullable variants, requires array `items`, resolves local
  `$ref` targets, recursively validates `$defs` and `anyOf`, and reports exact
  JSON paths. Negative fixtures cover `contains`, every named unsupported
  composition/uniqueness keyword, missing/true `additionalProperties`, missing,
  duplicate, and unknown `required` entries, unconstrained objects, missing
  array `items`, nullable enums without null, bad definitions, and unresolved
  refs.
- Before editing, both RC.3 evidence manifests passed with `sha256sum -c
  --strict`. Read-only regular-file content/layout fingerprints captured with
  `GIT_OPTIONAL_LOCKS=0` were evidence bundle
  `e47642eb4e8ade29ff213a3012891dc11a4bf800b654f80cb8c08a527564c689` /
  `bded4ce56ff6b2d8407978a40a3945b06eb1c0e982ec942e3671b14258b1b335`
  and whole suite
  `e070947f3a6cc3d0f598a3a78948757d7c1c0837c8baad70028cffb54b5734be` /
  `84e24f06525d81af2ff84061488d19170cc9791ca3139632892e3c5bf0431d58`.
  After all code checks, both manifests passed again and all four fingerprints
  matched these pre-edit values exactly.
- Files changed: `internal/supervisor/schema.go`,
  `internal/supervisor/prompt_schema_test.go`,
  `internal/autonomousplanning/schema.go`,
  `internal/autonomousplanning/contracts_test.go`,
  `internal/autonomous/correction.go`,
  `internal/autonomous/correction_test.go`,
  `internal/autonomous/contracts_test.go`,
  `internal/autonomous/needs_input_test.go`,
  `internal/autonomousaudit/schema.go`,
  `internal/autonomousaudit/contracts.go`,
  `internal/autonomousaudit/contracts_test.go`,
  `internal/autonomousauditapply/apply.go`,
  `internal/autonomousauditapply/apply_test.go`,
  `internal/autonomouscycle/worker_prompt.go`,
  `internal/autonomouscycle/schema_compatibility_test.go`, this file,
  `.agent/DECISIONS.md`, and `.agent/HANDOFF.md`. No dependency was added.
- Verification passed: `gofmt -w` on every changed Go file; focused ordinary
  and `-race -count=1` tests for `internal/autonomous`, `internal/supervisor`,
  `internal/autonomousplanning`, `internal/autonomousaudit`,
  `internal/autonomouscycle`, and changed `internal/autonomousauditapply`;
  `go test -count=1 ./internal/app -run
  '^TestProductionAutonomousHappyPath$'`; final `go test -count=1 ./...`; final
  recursive four-builder compatibility test; and `git diff --check`. Direct
  generated-schema inspection found zero structural violations and only
  allowlisted keywords; generated SHA-256 values were supervisor
  `5ef89243892e156bfdf098c132ea42ddc0a0ff74bd12af5276a493ac16be6c76`,
  planning `b4d088d7833a604ae4e91999dbc52a49b925d75ecfb3953dc56bdcceca2a1e09`,
  audit `899b551851549974b07c5352ba675e11f598763433c31c458a4c21e1e37e5eb3`,
  and correction
  `0badb90760b7ca013c2cc7cb0ebf4c54f404a9d749dd9f5d29e4051e5812f022`.
- RC.3 is permanently retired and must never be retried, reconciled,
  relabeled, or reused. RC.1 and RC.2 remain equally immutable. No `gh`,
  commit, push, tag, branch/ref publication, release, live/nested model
  operation, or external-use approval occurred. No API-acceptance claim is
  made. `EXT-20` remains unchecked. RC.4 remains blocked until this follow-up
  passes a new independent audit and is separately committed with explicit
  authority; this pass did not construct it.

## EXT-20 RC.3 Guarded No-Model Suite Preparation (2026-07-18)

- Task selected: the bounded RC.3 no-model suite-preparation subtask of
  `EXT-20`. The guarded suite now binds exact source
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, Linux SHA-256
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4`. Release
  output remains `revolvr 0.1.0`; Codex authority remains exact package
  `@openai/codex@0.144.4`, output `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, confirmation value,
  Go source, or dependency changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, this state file,
  and `.agent/HANDOFF.md`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. `.agent/DECISIONS.md` remains unchanged because this pass applies
  the already settled immutable RC.3 authority.
- No-model verification passed: `bash -n` for suite and collector; the complete
  RC.3 candidate bundle verifier; suite `--static`; two collector
  `--fixture-only` collections; both `--verify-manifest` checks; canonical
  manifest comparison after excluding only `collected_at_utc`; `go test
  -count=1 ./...`; and `git diff --check`. The retained fixture root is
  `/tmp/revolvr-ext20-rc3-collector.Uad2DH`; both raw manifest hashes are
  `5169090fe855302da1fe70ca98535d7b66e4769c1af81ddec27affd9e9fc64e9`
  and the timestamp-excluded canonical manifest hash is
  `01a47fd5ab7f7eb8e7def144ce7cbef17d5b306366cb2967896fb7b874e8452c`.
- The retained prepared suite authority is
  `/tmp/revolvr-ext20-rc3.Qghf19/suite`. Preparation used a new parent from
  exact template `/tmp/revolvr-ext20-rc3.XXXXXX` and only `--prepare
  --run-root <parent>/suite --install-codex`; it installed the pinned package
  and started no model. `authority.tsv` SHA-256 is
  `adc8095701e1fa6fdcd6180df93ea88ccac6f1d4d21cba1a751476a7a1ef3fb4`;
  `operation-plan.tsv` SHA-256 is
  `5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1`.
- Independent read-only inspection verified the authority hash; exact
  candidate hash/version/source metadata and `vcs.modified=false`; exact
  Codex package/version/hash; effective configuration schema v8; source-writer
  lock `timeout=32m0s heartbeat_interval=10m40s required=32m0s`; the exact
  11-row plan and ten unique ready tasks; tracked disposable markers; and both
  clean `main` repositories. `repo-a` is at
  `07239693a95a1c73b61de77b74ade0a234e84075`; `repo-b` is at
  `2de82370e6c8bd3775e25da783fc7d6dcf0e0b5c`. There are zero runtime
  operation manifests, zero collector manifests, and zero aggregate entries.
- Inspection set `GIT_OPTIONAL_LOCKS=0` so read-only Git commands could not
  persist optional index refreshes. Complete regular-file content fingerprint
  `90424fc5544fd3be146eef965a95b5300a910036e0a19703672fc95f1fb756ae`
  and layout/type/mode/owner/size/link/inode/mtime/ctime fingerprint
  `8fdd4272d36eefe1f9c99c4793bc47d51db5751ed12450b986755a875ae467b7`
  were identical before and after every authority, config, doctor, Git,
  manifest, and aggregate check. The exact empty-confirmation guard was tested
  in isolation and refused before any prepared-root read or collector call;
  neither `--live` nor the confirmation value was passed to the suite.
- A first preparation at `/tmp/revolvr-ext20-rc3.5TQPha/suite` is retained
  only as a failed inspection diagnostic. An independent doctor/status sweep
  was run without disabling Git optional locks, and the whole-root comparison
  caught an optional stat-cache rewrite of `repo-b/.git/index`. Its source
  worktree stayed clean, both HEADs stayed fixed, and it has zero operations,
  collector manifests, and aggregate entries, but it is not live authority and
  must not be used. The single repair attempt produced the verified root above.
- Verification result: passed for the tracked three-constant RC.3 update and
  the repaired no-model prepared suite. RC.1 and RC.2 remain rejected history;
  no historical ref, bundle, workflow, artifact record, hash, diagnostic, or
  available root was reused or changed. No `gh`, commit, push, tag, live or
  nested Codex/model operation, or external-use approval occurred.
- Independent controller verification repeated shell syntax, the complete
  candidate-bundle verifier, suite `--static`, exact candidate and Codex
  identities, prepared-authority verification, the 11-row/ten-task plan, both
  fixed clean repository heads, effective schema v8, and all ten task-specific
  doctor checks with the exact 32-minute lock authority. It confirmed zero
  runtime operations, collector manifests, and aggregate files. Controller
  content fingerprint
  `ba6436c19168f9af8b615f16b655b6f12e8fd86c6829bafbab45105230af6e9c`
  and layout fingerprint
  `154879f66815174b9bb4f5e3ae30380327b92a6da7d811b5eca75780d191f403`
  were unchanged before/after inspection with `GIT_OPTIONAL_LOCKS=0`.
  `go test -count=1 ./...` and final diff hygiene passed. No live argument,
  confirmation value, model operation, or prepared-root mutation occurred.
- Remaining separately confirmed live command, recorded but not executed:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc3.Qghf19/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. After that pass, every EXT-17 manifest
  and the EXT-20 aggregate still require independent validation. `EXT-20`
  remains unchecked until all quantitative and zero-tolerance thresholds pass.
  The intentional blocker is the separate live confirmation, which was not
  supplied in this no-model pass.

## EXT-20 RC.3 Remote Artifact Attestation Result (2026-07-18)

- The controller independently parsed the new workflow, asserted its exact
  trigger, permissions, candidate checkout, toolchain, cache policy, constants,
  upload contract, and shell syntax, then executed the complete unmodified
  embedded shell under host Go 1.26.5 from a fresh detached candidate clone.
  Both independent no-local build passes reproduced all three exact artifact
  hashes and passed byte, metadata, version, authority-manifest, and retained-
  output checks. The controller root is
  `/tmp/revolvr-controller-rc3-attestation.wzxrrfaj`; it contains 21 retained
  regular single-link files. Historical RC.1/RC.2 workflow hashes, the five
  pre-existing remote release refs, exact RC.3 source commit/tree, and diff
  hygiene also passed independent readback before commit.
- Raw Git committed the reviewed workflow and state as
  `80441464d55af466bbea15f20448099e2a163684` (`Add RC.3 artifact attestation
  workflow`), pushed `main`, and published the previously absent ref
  `level1-v0.1.0-rc.3-attestation` at that exact workflow commit. Remote
  readback preserved candidate ref `level1-v0.1.0-rc.3` at exact source
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`. No historical ref moved.
- Dedicated GitHub Actions run `29651665454` completed `success` at the exact
  workflow commit. Job `88098916882`, `Rebuild and attest Level 1 RC.3
  candidate`, passed checkout, pinned Go setup, both clean-clone rebuilds and
  attestations, retained-authority upload, and all post steps. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29651665454`.
- The retained unexpired artifact is
  `level1-v0.1.0-rc.3-attestation`, artifact ID `8431664217`, size 70,202,355
  bytes, GitHub artifact digest
  `sha256:8ac9f82795233b4808fc8c2fc895a11d1fb622e30272f73f90b1f68218c99cd1`,
  creation `2026-07-18T16:20:16Z`, and expiry
  `2026-10-16T16:17:29Z`. Public REST metadata was inspected; the archive
  endpoint requires authentication and returned HTTP 401, so no independent
  claim of downloading the remote ZIP is made.
- General push-triggered CI run `29651665483` also completed `success` at the
  exact workflow commit. All ten required jobs passed: Go 1.22 source floor
  and tests; Darwin, Linux, and FreeBSD amd64 builds; Windows diagnostic stub;
  vet and module verification; both fake-Codex smokes; race tests; and the
  production autonomous strict-fake suite.
- RC.3 remote candidate and artifact prerequisites are complete. `EXT-20`
  remains unchecked and external use remains unapproved. The next bounded pass
  is to update the guarded Level-1 suite from rejected RC.2 to immutable RC.3,
  prepare a fresh collision-free no-model root, and stop before any separately
  confirmed live real-Codex invocation. No `gh`, tag, live/nested Codex, or
  model operation was used.

## EXT-20 RC.3 Remote Artifact Attestation Workflow (2026-07-18)

- Task selected: the bounded RC.3 remote-artifact prerequisite of `EXT-20`
  only. New separate workflow
  `.github/workflows/level1-rc3-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.3-attestation` and checks out exact candidate
  source `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, not the trigger HEAD.
- The workflow installs exact Go 1.26.5 with cache restoration disabled and
  makes two independent clean `--no-local` clones. Each pass has its own Go
  build and module caches and builds Linux, Darwin, and FreeBSD amd64 with
  disabled CGO, local toolchain selection, module-readonly mode, trimpath,
  explicit clean VCS metadata, an empty build ID, and
  `main.version=0.1.0`.
- Every pass artifact must expose exact Go version, command path, compiler,
  trimpath, target, CGO, Git source revision, and `vcs.modified=false`
  metadata plus one exact 16-byte `main.version` symbol and one exact `0.1.0`
  string. Both Linux copies must report `revolvr 0.1.0`. The workflow requires
  byte-identical pass pairs and exact SHA-256 values Linux
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  Darwin
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`,
  and FreeBSD
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
  One uploaded artifact retains both binary sets, all six build-metadata and
  version assertions, all six hashes, the three-pair reproducibility table,
  and the exact tabular authority manifest.
- Files changed: the new RC.3 workflow and this state file only.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because the workflow implements the
  already settled RC.3 authority. No Go source or dependency changed.
- Verification commands run: all required durable-state and newest RC.3
  local/remote authority reads; raw-Git exact candidate object/tree and remote
  ref/collision checks; PyYAML BaseLoader parsing with exact trigger, checkout,
  toolchain, cache, clone, target, build, metadata, version, hash, and upload
  assertions; `bash -n` on the extracted embedded shell; and execution of the
  complete unmodified embedded shell under host Go 1.26.5 from a fresh
  detached exact-source clone. The shell created two further independent clean
  clones and passed all six builds and hashes. An independent retained-output
  inspection verified 21 regular single-link files, six hash-manifest rows,
  three byte-identical pairs, three exact authority rows, two clean source
  passes with separate caches, and the complete authority/reproducibility
  manifests. The retained local verification root is
  `/tmp/revolvr-ext20-rc3-attestation.desFNJ`; its relative-file/hash stream
  digest is
  `3303495dac203c5206c36d477f173c11c0ccc57e850ac92ab11e46aa6cd3daaa`.
  Workflow-only whitespace checks and final `git diff --check` passed. No Go
  test was required because no Go file changed.
- Preservation evidence: the RC.1 and RC.2 workflows remain exact SHA-256
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`
  and
  `4c96ec62a3757878926b62aee65ce8ba3ec6ac2148ac251622ab294109312c6d`.
  Every available RC.1/RC.2/RC.3 sealed candidate and verification inventory,
  plus both inventoried RC.1/RC.2 failed candidate diagnostics, reverified.
  The two RC.3 fail-closed verification diagnostics were not used or written.
  Raw Git still resolves RC.1, RC.1 attestation, RC.2, RC.2 attestation, and
  RC.3 candidate refs to `ed65049fba6bf82852fd406ebc17afa90a953e3f`,
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8`,
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`,
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`, and
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, respectively. No remote
  artifact or ref was mutated. The three historical temporary suite roots
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite`,
  `/tmp/revolvr-ext20-live2.3ZVQcm/suite`, and
  `/tmp/revolvr-ext20-rc2.96ibla/suite` were already absent during the initial
  pre-change inspection and remained absent; this pass did not remove,
  recreate, or represent them as locally available. Their prior recorded
  fingerprints remain unchanged in the sealed RC.3 verification bundle.
- Verification result: passed for the workflow structure, exact constants,
  embedded shell syntax, complete two-pass execution, hashes, byte equality,
  metadata, version authority, retained output, available sealed inventories,
  raw refs, and diff hygiene. No remote workflow, live/nested Codex, model,
  commit, push, tag, candidate-source/ref mutation, or external-use approval
  was started.
- Collision-free raw-Git attestation ref to publish after controller review
  and commit: `refs/heads/level1-v0.1.0-rc.3-attestation`. Raw Git found it
  absent at `origin`; publication must use the reviewed workflow commit as its
  tip and must not move exact candidate ref `level1-v0.1.0-rc.3`.
- What remains: the controller must independently verify and commit this local
  change, publish only the collision-free attestation ref, require the remote
  job to succeed, and record its exact workflow commit, run/job URL and
  conclusions, uploaded artifact ID/name/size/digest/expiry, and comparison
  with the local RC.3 authority. Only after that remote evidence exists may a
  fresh collision-free no-model RC.3 dogfood suite be prepared. `EXT-20`
  remains unchecked; external use remains unapproved.

## EXT-20 RC.3 Exact-Candidate Remote CI (2026-07-18)

- Independent controller verification passed both sealed RC.3 bundles, exact
  inventory digests, source/tree authority, a third non-local clean-clone
  rebuild, byte equality for all three supported artifacts, and a fresh
  no-model RC.3 binary config/doctor smoke.
- The local evidence/helper update was committed as `41d5319`. Raw Git pushed
  `main` and published collision-free candidate branch
  `level1-v0.1.0-rc.3` at exact source commit
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`; remote readback resolves that
  ref to the same SHA. RC.1 and RC.2 refs were not moved.
- Push-triggered GitHub Actions CI run `29642126354` completed `success` on
  that exact candidate SHA. All ten required jobs passed: Go 1.22 source floor
  and tests; Darwin, Linux, and FreeBSD amd64 builds; Windows diagnostic stub;
  vet and module verification; fake-Codex success and verification-failure
  smokes; race tests; and the production autonomous strict-fake suite. Run
  evidence: `https://github.com/ponchione/revolvr/actions/runs/29642126354`.
- No `gh`, tag, attestation workflow, live/nested Codex, model operation, or
  external-use approval was used or created. EXT-20 remains unchecked.
- The next bounded pass is a separate collision-free RC.3 exact-checkout
  Go 1.26.5 artifact-attestation workflow. It must reproduce both clean build
  passes, all three recorded RC.3 hashes and embedded identities, retain the
  remote authority artifact, and preserve RC.1/RC.2 workflows and evidence.

## EXT-20 RC.3 Replacement Candidate — Local Verification (2026-07-18)

- Task selected: the bounded RC.3 local replacement-candidate subtask of
  `EXT-20` only. Candidate `level1-v0.1.0-rc.3` binds release version `0.1.0`
  to exact clean source commit
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c` and tree
  `23c0d27fc62be5f41feb45192e74f1df8ecff3fa`. Local and remote candidate/
  attestation refs, RC.3 bundle paths, and `/tmp` RC.3 construction roots were
  absent before construction. The controller-created untracked
  `agent-ext20-rc3.sh` was absent from the source commit and every clean clone.
- The focused source-lock proof ran before candidate construction against the
  exact clean source. Ordinary and race tests across `internal/lock`,
  `internal/runonce`, `internal/app`, and `internal/cli` passed the required
  window calculation, safe defaulting, invalid/overflow refusal, effective
  fingerprint, config rendering, ready doctor, and zero-model preflight
  regressions. Exact retained outputs are `source-lock-test.txt` and
  `source-lock-race-test.txt` in the verification bundle.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4/`. Its pinned
  build-instruction SHA-256 is
  `2deaa06d380dfd7d86277e2090229ba1d212e1bad85c6b3db6a83a31036c1405`;
  its 13-entry complete evidence inventory SHA-256 is
  `766856c2783073c8ffa10cbce0e3c0a9f8ebee4db1785c309f6c5680f5e5ddae`.
  Two independent `--no-local` clean clones produced byte-identical artifacts:
  Linux amd64
  `9e9c13f43977edf49e7e6385c595aa20a01c16308ddfea1c30455ea88252ae9b`,
  Darwin amd64
  `ee6db29cfcbcbd2e645184927fc5d7348ed924d6036a46d7b23eae55b5b43fd4`,
  and FreeBSD amd64
  `6a42dc423ab1975e8ea4296f56be6c9a3773d9ccfda1c57244a50261e64f368a`.
- Every artifact independently records Go 1.26.5, compiler `gc`, trimpath,
  its exact `GOOS/amd64`, `CGO_ENABLED=0`, Git VCS authority, source revision
  `a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c`, and
  `vcs.modified=false`. Each contains the exact 16-byte `main.version` symbol
  and exactly one `0.1.0` release string; the Linux artifact prints exactly
  `revolvr 0.1.0`.
- The separate immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.3-a16ea1bdc1a4-verification/`.
  Its 29-entry complete evidence inventory SHA-256 is
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff`.
  It retains exact source/tree/tool identities, commands and raw logs, focused
  source-lock proof, candidate manifest and inventory, two-build
  reproducibility, independent build metadata/version assertions, collision
  checks, all four preserved remote refs, and before/after preservation
  inventories.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  Linux/Darwin/FreeBSD amd64 release builds; exact two-pass artifact comparison;
  embedded version/source/tool/target/CGO/VCS metadata; candidate self-check;
  verification-bundle inventory check; clean-clone authority; and exclusion of
  the controller helper. Govulncheck found zero reachable and zero imported-
  package vulnerabilities. The retained module-only finding remains
  `GO-2026-5024` in the Windows-only `golang.org/x/sys/windows` surface at
  `v0.30.0`, which Revolvr does not call; the report names `v0.44.0` as fixed.
- Two fail-closed verification-assembly diagnostics are retained unchanged at
  suffixes `.failed-readonly-commands-append` and
  `.failed-sealed-before-inventory`. The first refused an append after an
  early read-only install; the second refused inventory creation after an
  early directory seal. Their relative content/layout fingerprints are,
  respectively,
  `5b0a359ff8432730dad07e25b290bc6b3c52b7811d9b9e2c8f2324e4804bd8e7` /
  `6264d0f32ac7f08d413d245cf85a122b226f9fa33811348efe9d14f7605aee65`
  and
  `006cb0b7f2215878e757ae8ee104bb1b88d5ae31b8661168bcf5c705d08353ff` /
  `69d555e7193ea069b7a2be176d1f3455434bd4b3a748a9580fee08d333c8fba9`.
  The authoritative finalization created and verified both inventories before
  sealing the completed bundle read-only.
- Preservation passed. RC.1 and RC.2 candidate/verification/failed bundles,
  both existing attestation workflows, both earlier retired roots, and failed
  RC.2 suite `/tmp/revolvr-ext20-rc2.96ibla/suite` retained identical content
  and layout/metadata fingerprints. Raw Git readback preserved candidate refs
  RC.1 `ed65049fba6bf82852fd406ebc17afa90a953e3f`, RC.2
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`, and attestation refs RC.1
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8` and RC.2
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`. No old artifact, hash,
  workflow, ref, run root, or evidence file changed.
- Files changed: this state file, `.agent/DECISIONS.md`, the new ignored RC.3
  pinned build script, candidate bundle, verification bundle, two retained
  assembly diagnostics, and controller-created fresh-session helper
  `agent-ext20-rc3.sh`. `.agent/TASKS.md` remains unchanged with `EXT-20`
  unchecked. No Go source, dependency, candidate ref, workflow, dogfood suite,
  tag, live/nested Codex, model operation, remote CI request, or external-use
  approval was created during local candidate construction.
- Independent controller verification passed both sealed-bundle self-checks,
  exact inventory digests, candidate source/tree authority, retained Go-floor/
  release/focused-race/vulnerability evidence, and collision-free RC.3 refs.
  A third independent non-local clean clone rebuilt both passes and reproduced
  all three artifact bytes exactly. The rebuilt Linux binary then passed a
  fresh disposable-repository no-model smoke: config check reported effective
  schema v8 and `32m0s`/`10m40s` lock authority, doctor reported the same
  required window and `Ready: true`, Git stayed clean, and no task operation
  was created.
- Exact candidate-ref publication and mandatory CI completion are recorded in
  `EXT-20 RC.3 Exact-Candidate Remote CI` above. The next separate pass must
  add and run a collision-free exact-checkout Go 1.26.5 RC.3 artifact
  attestation before preparing a new no-model dogfood suite. Blockers for that
  bounded workflow task: none.

## EXT-20 RC.2 Source-Lock Configuration Defect (2026-07-18)

- Task selected: the bounded `EXT-20` configuration-normalization/preflight
  repair only. The retained terminal evidence for operation
  `ext20-3601e63c616b-01` was read before source changes. Its `config check`
  recorded Codex timeout `30m0s`, Git timeout `30s`, and effective-config
  schema v7; doctor reported `Ready: true`; execution then terminalized
  `unsafe_or_ambiguous` because the effective source-writer timeout was `5m0s`
  while autonomous execution required `32m0s`.
- The retained operation proves zero model attempts: task-operation statistics
  contain one started/completed cycle but zero attempts, verification runs,
  audits, corrections, commits, and checkpoints. No Codex `exec` evidence,
  receipt, or model run exists. The failure occurred at autonomous-cycle
  configuration validation before supervisor/model invocation.
- Root cause: `runonce` finalized Codex and Git timeouts but independently used
  a fixed five-minute source-writer default. Supervisor and autonomous-cycle
  validation separately required `CodexTimeout + 2*GitTimeout + 1m`, allowing
  config check/doctor to admit authority that execution rejected.
- Fix: `internal/lock` now owns one overflow-safe required-window calculation
  shared by effective normalization, supervisor, and autonomous cycle.
  Unspecified effective lock authority is derived only after final Codex/Git
  timeouts; valid explicit longer authority is preserved; negative, too-short,
  and overflowing authority fails closed; heartbeat authority is derived only
  after the final lock timeout. Config check and doctor now render the exact
  effective timeout, heartbeat, and required window. The changed effective
  contract is fingerprinted as `revolvr-effective-run-config-v8`.
- Files changed: `internal/lock/source_writer_window.go` and its test;
  `internal/runonce/runonce.go`, `effectiveconfig.go`, and focused tests;
  `internal/supervisor/execution.go`; `internal/autonomouscycle/cycle.go`;
  `internal/app/external_admission.go`, `external_preflight_test.go`,
  `config_numeric_test.go`, and `app_test.go`; `internal/cli/config.go`,
  `root_test.go`, and `doctor_test.go`; the EXT-17 fixture's effective-schema
  projection in `scripts/dogfood-external-level1.sh`; the controller-created
  fresh-session helper `agent-ext20-source-lock.sh`; this state file; and
  `.agent/DECISIONS.md`.
- Verification commands run: `gofmt` on every changed Go file; focused
  ordinary tests across `internal/lock`, `internal/runonce`, `internal/app`,
  and `internal/cli`; the same source-window/config/preflight/CLI regressions
  with `-race`; ordinary supervisor/autonomous-cycle and package tests;
  `bash -n scripts/dogfood-external-level1.sh`; `go test -count=1 ./...` after
  the final repair; CLI `config check` and `doctor --for attended-task`
  no-model probes; tracked/untracked Go formatting inspection; source-schema
  scans; raw-Git candidate/attestation ref inspection; protected evidence and
  bundle content/layout fingerprints; and `git diff --check`.
- Verification result: passed. The focused 30m Codex/30s Git regression
  derives timeout `32m0s` and heartbeat `10m40s`; a ready attended doctor
  exposes the same required authority with zero model invocations. Custom
  45s/12s authority derives `2m9s`, explicit valid 5m/30s authority is
  retained, and short, negative, or overflowing authority is refused. The
  standalone CLI probe printed schema v8 and the same 32m/10m40s authority;
  its doctor source-lock check was OK (the disposable probe as a whole was not
  ready because initialization refused its umask-created `.git` mode 0775).
- Independent controller verification reviewed every production/test diff,
  repeated changed-Go formatting and shell syntax checks, ran the focused
  source-window/config/preflight tests ordinarily and with the race detector,
  ran supervisor and autonomous-cycle package tests, collected and verified a
  fresh EXT-17 fixture manifest, ran focused CLI race tests, and reran
  `go test -count=1 ./...` plus `git diff --check`; all passed.
- Preservation: RC.1, RC.2, all candidate/attestation refs and bundles, both
  earlier retired roots, and failed RC.2 suite root
  `/tmp/revolvr-ext20-rc2.96ibla/suite` were not edited. Repeated content and
  layout/metadata fingerprints agreed at close. No live/nested Codex or model
  operation, external-repository edit, commit, push, tag, ref update, candidate
  build, or publication was performed.
- Release consequence: RC.2 remains immutable rejected historical evidence for
  the quantitative gate; this material configuration defect invalidates its
  affected live operation and prevents RC.2 from satisfying `EXT-20`. No RC.3
  was built or published in this pass. `EXT-20` remains unchecked.
- What remains: after independent controller verification/commit, build a new
  collision-free replacement candidate (RC.3), reproduce and attest its exact
  supported-platform artifacts and remote CI, prepare a fresh collision-free
  no-model suite, then separately confirm and run all quantitative real-Codex
  scenarios and independently verify every manifest/aggregate. Blockers for
  this bounded code repair: none.

## EXT-20 RC.2 Guarded No-Model Suite Preparation (2026-07-18)

- Task selected: the bounded RC.2 no-model preparation subtask of `EXT-20`.
  The guarded external Level-1 suite now binds exact candidate source
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`, exact Linux SHA-256
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  and immutable bundle
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd`. Release output
  remains exactly `revolvr 0.1.0`; Codex authority remains exactly
  `@openai/codex@0.144.4`, `codex-cli 0.144.4`, and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  No plan, schema, scenario, threshold, configuration, or confirmation value
  changed.
- Files changed: `scripts/dogfood-external-level1-suite.sh`, the controller-
  created fresh-session helper `agent-ext20-rc2-suite.sh`, and this state file.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this applies already settled
  RC.2 authority.
- Verification commands run: all required durable-state and newest RC.2
  local/remote authority reads; raw-Git readback of all four RC.1/RC.2 refs;
  before/after content and layout fingerprints for both attestation workflows,
  the RC.1/RC.2 candidate and verification bundles, the retained RC.2 failed
  diagnostic, both retired RC.1 live roots, and the pre-existing helper;
  `bash -n` for the suite and collector; the complete RC.2 candidate bundle
  `--verify`; suite `--static`; two collector `--fixture-only` runs, both
  `--verify-manifest` checks, and their canonical-manifest comparison after
  removing only `collected_at_utc`; `git diff --check`; and
  `go test -count=1 ./...`. All passed.
- A new parent was created with exact template
  `/tmp/revolvr-ext20-rc2.XXXXXX`, and no-model preparation with
  `--prepare --run-root <parent>/suite --install-codex` passed. The retained
  prepared suite is `/tmp/revolvr-ext20-rc2.96ibla/suite`. Its authority hash
  verifies; candidate version/hash/source and isolated Codex package/version/
  hash agree exactly; the 11-row plan names exactly 10 ready tasks; repository
  `repo-a` is clean at `ea72ad021d711b21522e4cd586400ae625ce308f`
  and `repo-b` is clean at `99e41baade47d24abc17e6ff0d6b18acd9765bbb`;
  both retain the exact tracked disposable marker. There are zero collector
  manifests, zero runtime operation manifests, and zero aggregate entries.
- Independent inspection left the prepared suite byte-for-byte and
  metadata-for-metadata unchanged, with regular-file fingerprint
  `37546e2d33e0239e63a029e8ca303a52a3c22d4c10abd939d50c6f72551f32b0`
  and layout/metadata fingerprint
  `ee0babb21f61731de593870b20e7c5693215608ffc6007faa86245c1cf56cb5d`.
  The exact empty-confirmation guard was executed in isolation and refused
  before the first prepared-root read or collector call. In accordance with
  the operator direction, no `--live` argument or confirmation value was
  passed to the suite, and no live, nested Codex, or model operation started.
- Verification result: the tracked shell-only RC.2 update and the new no-model
  suite preparation passed. RC.1, both retired live roots, every RC.1/RC.2
  ref, bundle, workflow, artifact, hash, and retained diagnostic remained
  unchanged. No commit, push, tag, or external-use approval was created.
- Independent controller verification repeated helper/suite/collector syntax,
  suite static verification, RC.2 bundle verification, exact prepared
  authority/candidate/Codex checks, all ten task-ready doctor checks, both Git
  cleanliness checks, and the zero-manifest/zero-aggregate assertions. A full
  prepared-file content inventory was identical before and after inspection.
- What remains: a separately confirmed live pass must execute and then
  independently verify all retained manifests and aggregate thresholds with
  the exact command below. It was recorded but not executed:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-rc2.96ibla/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. `EXT-20` remains unchecked until that
  live suite and `--verify-suite` complete successfully.
- Blocker for the remaining EXT-20 work: the separate live confirmation was
  intentionally not supplied in this no-model pass.

## EXT-20 RC.2 Remote Artifact Attestation Result (2026-07-17)

- Raw Git published collision-free attestation ref
  `level1-v0.1.0-rc.2-attestation` at exact workflow commit
  `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`; remote readback preserved the
  candidate ref at exact source commit
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`.
- Dedicated GitHub Actions run `29621464972` completed `success` at the exact
  workflow commit. Job `88017202674`, `Rebuild and attest Level 1 RC.2
  candidate`, passed its exact-candidate checkout, pinned Go setup, two-clean-
  clone rebuild and attestation, and retained-authority upload steps. The
  workflow therefore enforced all six recorded hashes, all three byte-for-byte
  build-pair comparisons, embedded build/version metadata, and the authority
  manifest before upload. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29621464972`.
- The retained unexpired artifact is
  `level1-v0.1.0-rc.2-attestation`, artifact ID `8422371223`, size
  70,182,577 bytes, with GitHub artifact digest
  `sha256:f68f6b5a02ce5f18c31ba50b11dc4a0e653145161b0636dce265401216902018`
  and expiry `2026-10-15T23:44:45Z`. The public REST metadata was inspected;
  its archive endpoint requires authentication and returned HTTP 401, so no
  independent claim of downloading the remote ZIP is made.
- General push-triggered CI run `29621464929` also completed `success` at exact
  workflow commit `7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2`.
- The RC.2 remote prerequisites are complete. EXT-20 remains unchecked. The
  next bounded pass is to update and verify the external Level-1 suite against
  immutable RC.2, prepare a new collision-free no-model run root, and stop
  before the separately confirmed live real-Codex invocation. No tag or
  external-use approval was created.

## EXT-20 RC.2 Remote Artifact Attestation Workflow (2026-07-17)

- Task selected: the bounded RC.2 remote-artifact prerequisite of `EXT-20`
  only. A new separate
  `.github/workflows/level1-rc2-candidate-attestation.yml` triggers only on a
  push to `level1-v0.1.0-rc.2-attestation` and checks out exact candidate
  source commit `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` rather than the
  trigger HEAD.
- The workflow installs exact Go 1.26.5 with cache restoration disabled, makes
  two independent clean `--no-local` source clones with separate Go build and
  module caches, and builds Linux, Darwin, and FreeBSD amd64 artifacts with
  the settled EXT-18 environment, build flags, empty build ID, and
  `main.version=0.1.0`. It compares each pass pair byte-for-byte and requires
  the exact RC.2 SHA-256 values
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  `05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1`,
  and `5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29`.
- Every artifact must expose the exact Go toolchain, command path, compiler,
  trimpath, target, disabled CGO, Git source revision, and
  `vcs.modified=false` build metadata. Each artifact must also carry exactly
  one `main.version` symbol and exact `0.1.0` string; both Linux copies must
  execute with exact output `revolvr 0.1.0`. One upload retains both binary
  sets, all six build-metadata/version assertions, `SHA256SUMS`, pairwise
  reproducibility hashes, and the exact tabular authority manifest.
- Files changed: the new RC.2 workflow and this state file only.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this workflow implements the
  already settled RC.2 authority. The RC.1 workflow remains byte-for-byte
  unchanged at SHA-256
  `d1314182a0cffd78927e6a5cc688e370c42f3d17a4e4ffe426f647a384c40a41`;
  no RC.1 ref, hash, bundle, or failed live evidence changed.
- Verification commands run: all required durable-state reads; raw Git status,
  history, exact candidate object/tree, and local/remote ref-collision checks;
  PyYAML BaseLoader parsing with exact trigger/checkout/toolchain/build/hash/
  metadata/upload assertions; `bash -n` on the embedded workflow shell; and
  two executions of the actual embedded shell against fresh detached clones
  under host Go 1.26.5. The first execution completed all builds and hash
  checks but its outer local harness miscounted the retained files; the single
  repair corrected that harness assertion from 18 to 21. The complete rerun
  passed all six required hashes, all three byte-for-byte pair comparisons,
  every retained-authority assertion, and `git diff --check`.
- Verification result: local workflow syntax, exact constants, clean two-pass
  execution, metadata, version, hashes, reproducibility, and retained evidence
  passed. Independent controller verification parsed the workflow authority,
  syntax-checked and executed its exact embedded shell from a detached clean
  checkout, then verified all 21 retained files, six SHA-256 entries, three
  byte-identical artifact pairs, and the complete authority manifest. No
  remote workflow, live/nested Codex, model, commit, push, or tag operation was
  started during either local verification pass.
- Collision-free raw-Git attestation ref to publish after controller review
  and commit: `refs/heads/level1-v0.1.0-rc.2-attestation`. Raw Git found that
  ref absent both locally and at `origin`; publication should use the reviewed
  workflow commit as its tip and must not move the exact candidate ref.
- Completion is recorded in `EXT-20 RC.2 Remote Artifact Attestation Result`
  above. A new collision-free RC.2 EXT-20 real-Codex suite may now be prepared,
  but live model execution still requires its separate explicit confirmation.
  External use remains unapproved; blocker for the next local subtask: none.

## EXT-20 RC.2 Exact-Candidate Remote CI (2026-07-17)

- Independent controller verification passed for the local RC.2 bundle and a
  third clean-clone rebuild. Raw Git published collision-free branch
  `level1-v0.1.0-rc.2` at exact source commit
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`; remote readback resolves that
  ref to the same SHA.
- Push-triggered GitHub Actions CI run `29620366441` completed `success` on
  that exact SHA. All ten jobs passed: Go 1.22 source floor and tests; Darwin,
  Linux, and FreeBSD amd64 builds; Windows diagnostic stub; vet and module
  verification; fake-Codex success and verification-failure smokes; race
  tests; and the production autonomous strict-fake suite. Run evidence:
  `https://github.com/ponchione/revolvr/actions/runs/29620366441`.
- The local evidence update was committed as `916c856`; raw Git pushed `main`
  and the exact candidate ref. No `gh`, tag, attestation workflow, live Codex,
  or external-use approval was used or created.
- EXT-20 remains unchecked. The next bounded pass is the separate RC.2 remote
  artifact attestation workflow only. It must check out exact `eeaaf50`, pin Go
  1.26.5, reproduce both clean build passes and all three recorded hashes,
  verify embedded metadata, retain the attested artifact bundle, and preserve
  every RC.1 workflow/evidence unchanged.

## EXT-20 RC.2 Replacement Candidate — Local Verification (2026-07-17)

- Task selected: the bounded replacement-candidate subtask of `EXT-20` only.
  Candidate `level1-v0.1.0-rc.2` binds release version `0.1.0` to exact clean
  source commit `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec` and tree
  `f1eef2999103ffdc9b76fceda34af824191dd4b7`. Local and remote ref checks plus
  local bundle/root checks found no prior RC.2 collision. RC.1, both retired
  EXT-20 live roots, their remote refs, and their evidence were not changed.
- The production omitted-work-directory fix was inspected first. Its focused
  regression passed on Go 1.26.5, under the race detector, and with the full
  `internal/cli` package before candidate construction; the focused ordinary
  and race results are also retained with the release verification evidence.
- The immutable candidate bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd/`.
  Its build-instruction SHA-256 is
  `c9e38a38684c5022445fbc59725f4ecd4f7d143540ad2bb99a3aa8c208f2bdcf`
  and its complete inventory SHA-256 is
  `d398e5ae9a2a74965ad76134b48311e593d299a0c1003e7e2f19b72a74f1c0e7`.
  Two independent non-local clean clones produced byte-identical artifacts:
  Linux amd64
  `06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010`,
  Darwin amd64
  `05a15786dd1617d77ec671f420075922f6f9a78bf03de1245f03008f0960dee1`,
  and FreeBSD amd64
  `5891c88e1e13f5a0a0e3452c15221981a187652c2e563a7b8b218b63c07d2a29`.
- Every artifact independently records Go 1.26.5, its exact target,
  `CGO_ENABLED=0`, source revision `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`,
  and `vcs.modified=false`; the Linux binary reports `revolvr 0.1.0`. The
  sibling immutable verification bundle is
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd-verification/`.
  It retains exact source/Go/Git/govulncheck identities, commands, logs,
  independent build metadata, both build-attempt logs, and a complete
  inventory with SHA-256
  `45a6d14e90aaf95de7f1a67e0179edf50142e226ecdbc52259e8c60c77531c9c`.
- Required verification passed: Go 1.22.12 source-floor tests; Go 1.26.5 full
  tests; `go vet ./...`; `go mod verify`; `govulncheck ./...` and verbose scan;
  supported Linux/Darwin/FreeBSD amd64 builds; exact duplicate-build hash
  comparison; embedded version/source/toolchain checks; candidate self-check;
  verification-bundle inventory check; and source cleanliness. Govulncheck
  found zero reachable and zero imported-package vulnerabilities. Its one
  retained module-only finding is `GO-2026-5024` in the Windows-only
  `golang.org/x/sys/windows` surface at `v0.30.0`; Revolvr does not call it and
  the report names `v0.44.0` as fixed.
- Independent controller verification reran both bundle inventories, rejected
  links/aliases, checked every artifact hash and embedded build field, verified
  RC.1 remained intact, and ran the RC.2 Linux binary from a retired external
  repository. RC.2 passed repository/external admission and refused only an
  intentionally absent task at scheduler selection, with no operation or Git
  mutation. A third build from another clean non-local clone reproduced all
  three artifact hashes exactly. Its complete bundle inventory differs because
  the first line of each retained `go version -m` text records that rebuild's
  absolute artifact pathname; this documented path-only text variance is not
  artifact or embedded-metadata authority. The candidate binaries and all
  embedded fields remain byte-identical to the authoritative bundle.
- The first construction used a relative output path. Artifacts reproduced,
  but fail-closed verification rejected the path-bearing build metadata. That
  immutable diagnostic is retained at
  `.revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd.failed-relative-metadata-path/`.
  The single repair used the settled absolute output-path invocation and
  passed every check; no failed bundle was overwritten.
- Files changed: this state file, `.agent/DECISIONS.md`, the new ignored RC.2
  pinned build script, candidate bundle, verification bundle, and failed
  diagnostic. No Go source, dependency, RC.1 evidence, failed EXT-20 root,
  commit, push, tag, remote ref, workflow run, or live/model operation changed.
  `.agent/TASKS.md` remains unchanged with `EXT-20` unchecked.
- Exact remaining remote-attestation step: after independent controller
  verification, use raw Git to publish a collision-free
  `level1-v0.1.0-rc.2` ref at exact source
  `eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec`; require every EXT-15 CI job on
  that SHA to pass. Then publish a separate RC.2 attestation-workflow commit
  that checks out that immutable source, pins Go 1.26.5 and the three hashes
  above, rebuilds both clean passes, verifies embedded metadata, and retains a
  remote artifact digest. Compare its outputs byte-for-byte with this local
  bundle before preparing a new collision-free EXT-20 live suite. No `gh`,
  push, remote CI request, or attestation was performed in this pass.
- Result: local replacement-candidate construction passed. What remains is
  fresh remote exact-commit CI/artifact attestation followed by the complete
  quantitative real-Codex gate. Blockers for this bounded local task: none.

## EXT-20 Candidate Rejected: CLI Work Directory (2026-07-17)

- The second live attempt reached the exact RC.1 binary but stopped before
  supervisor/model admission. `revolvr run --until-terminal` returned
  `inspect repository paths: harness runtime path: repository root is required`
  for operation `ext20-579a7316af31-01`; the collector consequently retained
  an incomplete, non-manifest diagnostic tree at
  `/tmp/revolvr-ext20-live2.3ZVQcm/suite/evidence/repo-a/01-successful-source-change-1`.
  No task-operation directory, run, receipt, source change, commit, or model
  invocation exists. The suite root is retired because incomplete evidence is
  collision authority and must not be overwritten.
- Root cause: the production entry point constructs `cli.Options` without
  `WorkDir`. Read-oriented commands normalize an empty work directory through
  `resolveStatePaths`, but autonomous `run` passes it directly to external
  admission. The immutable RC.1 candidate therefore cannot start any real
  attended task, even when its process current directory is the repository.
- Fix: `cli.NewRootCommand` now resolves an omitted work directory from
  `os.Getwd` once before constructing subcommands. A CLI regression test calls
  autonomous run with production-style empty options and proves the exact
  current directory reaches `app.Config`.
- Verification passed: `gofmt`; focused CLI, app, repository-path, and runtime-
  path tests; `go test -count=1 ./...`; `git diff --check`; and a newly built
  production CLI invoked from the pristine second external repository. The
  smoke command passed repository/external admission and refused only the
  intentionally absent task at scheduler selection, created no operation, and
  left Git clean.
- Release consequence: candidate RC.1 at source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` is rejected for Level-1 external
  use. Its EXT-18/EXT-19 evidence remains immutable historical evidence but
  cannot satisfy EXT-20. A replacement candidate must be built reproducibly
  from the work-directory fix, receive fresh local/remote attestation, and use
  a new collision-free EXT-20 suite. The old candidate binary, bundles, remote
  refs, artifacts, and failed roots must not be edited or relabeled.
- EXT-20 remains unchecked. The next fresh bounded pass is replacement local
  candidate construction and verification only; it must not start live Codex,
  push, tag, or mark external use approved.

## EXT-20 Live Preflight Bound-Rendering Fix (2026-07-17)

- The first confirmation-gated live command stopped during EXT-17 collector
  admission for operation `ext20-e92c1bdec435-01` with status 1 and no terminal
  manifest. The retained diagnostic is
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite/logs/01-successful-source-change-1.stderr`.
  No Codex/model process, task operation, runtime evidence, source change, or
  aggregate file was started; both disposable repositories remained clean.
- Root cause: `scripts/dogfood-external-level1.sh` still asserted the obsolete
  strict-fixture rendering `audit:4`, `elapsed_nanoseconds`, and
  `process_nanoseconds`. The exact EXT-18 candidate renders its settled public
  config contract as `action_attempts=[audit=4,...]`, `elapsed=4h0m0s`, and
  `process_duration=30m0s`, so the collector rejected the valid release before
  model admission.
- Fix: the collector fixture now reproduces the exact release rendering and
  real-operation preflight compares the complete canonical attended-bounds
  line byte-for-byte. This removes the fixture/production drift while retaining
  fail-closed validation of every Level-1 bound.
- Verification passed: shell syntax; two fresh fixture-only collections and
  independent manifest verification; wrong-Codex preflight rejection; suite
  static verification; and a real-candidate/no-model preflight probe that
  passed candidate, Codex, config, and exact bounds authority before refusing
  only the intentionally absent task at doctor. The probe created no evidence
  and left Git clean.
- The failed preparation root is retired to preserve its diagnostic. A fresh
  no-model suite is prepared at `/tmp/revolvr-ext20-live2.3ZVQcm/suite` with
  zero manifests, an empty aggregate directory, two clean repositories, and
  exact isolated `codex-cli 0.144.4`. Live confirmation refusal and pre-live
  aggregate refusal both passed without mutation.
- EXT-20 remains unchecked. Resume with:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-live2.3ZVQcm/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`.

## EXT-20 Guarded Level-1 Suite Driver (2026-07-17)

- Task selected: `EXT-20`, implement the guarded driver for the complete
  quantitative Level-1 real-Codex dogfood suite without starting live model
  work in this pass.
- `scripts/dogfood-external-level1-suite.sh` now has four explicit modes:
  no-write/no-model `--static`, no-model `--prepare`, confirmation-gated
  `--live`, and independent `--verify-suite`. Live execution is impossible
  without the exact value
  `EXT20_LIVE_REAL_CODEX_MODEL_CALLS` supplied to
  `--confirm-live-real-codex`.
- Preparation either installs isolated `@openai/codex@0.144.4` or accepts an
  explicit isolated npm prefix, then requires exact output
  `codex-cli 0.144.4` and SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  It separately verifies the complete immutable EXT-18 bundle and Linux
  candidate SHA-256
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`
  at source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`.
- The prepared plan contains 11 unique operations across two new disposable
  repositories: five named successful source changes, a completed correction
  with final verification and re-audit, a retained production verification
  failure, needs input, graceful cancellation followed by a new-operation
  restart, and a hostile-instruction supervisor safety refusal. Every
  operation invokes `scripts/dogfood-external-level1.sh` once. Existing
  terminal bundles are verified and reused; incomplete path collisions fail
  without overwrite.
- Aggregate verification independently verifies every EXT-17 manifest,
  outside-sentinel equality, control-HEAD containment, candidate/Codex/config
  identity, terminal operation/history, every retained ledger/receipt
  validation result, attempt charge equality, exact task-branch commit counts,
  unique commit heads, all quantitative thresholds, and every zero-tolerance
  counter. Its sorted operation table and report are deterministic and
  hash-listed; conflicting retained rows, report, or checksum authority are
  never overwritten. Failed verification removes its unpublished temporary
  aggregate files. The driver never edits task-run, state, history, receipt,
  recovery, or other live runtime evidence.
- Controller review found and repaired two evidence-quality defects before any
  live call: failed pre-live verification retained an unpublished aggregate
  temporary file, and aggregate verification did not independently inspect
  every hashed collector ledger/receipt result. It also narrowed permitted
  control-root metadata changes to the selected task, checkpoints them after
  every terminal outcome, makes retained-bundle replay recheck exact task and
  outcome authority, and counts only the five explicit successful-source
  scenarios toward that threshold.
- Files changed in this pass:
  `scripts/dogfood-external-level1-suite.sh`, `agent-ext20.sh`, and this state
  file. The helper preserves the fresh-session implementation command used to
  create the driver; the live model suite remains a separate confirmation-
  gated command.
  `.agent/TASKS.md` remains unchanged with EXT-20 unchecked, and
  `.agent/DECISIONS.md` remains unchanged because this applies the settled
  EXT-17 through EXT-20 evidence authority without changing architecture.
- Verification commands run: all required durable-state reads; shell syntax
  checks for the new driver and EXT-17 collector; the driver's `--static`
  mode; the pinned candidate bundle `--verify` path; a fresh standalone
  no-model `--prepare --install-codex` run; a second no-model preparation using
  the accepted-prefix path; exact installed package/version/executable hash
  inspection; clean Git/status and exact-task doctor checks for both prepared
  repositories; refusal of live mode without the confirmation value; refusal
  of a colliding preparation root; refusal of pre-live aggregate validation
  with no manifests and no aggregate publication; independent controller
  syntax/static review; confirmation-refusal testing; source scans for nested
  Codex, `gh`, push, and source-runtime edits; `go test ./...`; focused CLI and
  task-run persistence/cancellation tests; and `git diff --check`. One
  initial preparation found a same-line Bash `local` expansion under
  `set -u`; the single repair split the derived assignment, after which both
  preparation forms and the complete static verification passed. A later
  attempt to remove two temporary preparation-check roots was rejected before
  execution by the command guard and changed no repository or suite evidence.
- Verification result: implementation and no-model preparation passed. The old
  preparation root was retired after it retained a temporary file produced by
  the pre-repair verifier. A fresh standalone prepared suite with zero
  operation manifests and an empty aggregate directory is retained at
  `/tmp/revolvr-ext20-fresh.OmCBwv/suite`; both external repositories are
  clean, every task doctor reported ready during preparation, and its isolated
  Codex path reports the exact required version and digest.
- What remains: run the real suite with the unmistakable explicit
  confirmation, retain all 11 terminal bundles, and independently validate
  the aggregate:
  `scripts/dogfood-external-level1-suite.sh --live --run-root
  /tmp/revolvr-ext20-fresh.OmCBwv/suite --confirm-live-real-codex
  EXT20_LIVE_REAL_CODEX_MODEL_CALLS`. EXT-20 must remain unchecked until this
  command finishes and `--verify-suite` passes every retained manifest and
  threshold.
- Blockers: this implementation pass explicitly forbids live or nested Codex
  operations, so it cannot produce qualifying manifests. No live model call,
  current-repository commit, push, tag, or remote mutation was started.

## EXT-20 Real-Codex Dogfood Gate Blocked (2026-07-17)

- Task selected: `EXT-20`, execute and independently validate the quantitative
  Level-1 real-Codex dogfood gate for the exact release candidate.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-20 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: all required durable-state reads; `git status
  --branch --porcelain=v2`; exact HEAD and remote candidate-ref inspection;
  candidate-bundle inventory, version, SHA-256, and Go build-metadata checks;
  the pinned candidate bundle `--verify` command; installed Codex path,
  SHA-256, and version inspection; the embedded release-manifest inspection;
  searches for dogfood manifests under the repository, `/tmp`, and
  `/home/gernsback`; inspection of every discovered intact manifest; current
  collector `--verify-manifest` for all four intact bundles; and `git diff
  --check`.
- Verification result: blocked. The Linux candidate remains intact at exact
  SHA-256
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  reports `revolvr 0.1.0`, and records source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`. The only discovered Level-1
  manifests are four valid EXT-17 fixture bundles whose manifests explicitly
  record `fixture_only\ttrue`, the synthetic candidate, and synthetic Codex
  identity. They prove the collector mechanism but contribute zero real-Codex
  operations, repositories, successful source changes, or required production
  scenarios to EXT-20.
- What remains: collect at least ten qualifying manifests from real-Codex task
  operations across at least two disposable external repositories, including
  five successful source changes plus verification failure/correction, needs
  input, cancellation/restart, and safety refusal. Then verify every manifest,
  total the independent thresholds, compare Git and outside-sentinel
  authority, and validate every ledger and receipt before completing EXT-20.
- Blockers: this pass explicitly forbids starting a nested Codex run, so it
  cannot generate the missing real-operation evidence. In addition, the only
  installed Codex reports `codex-cli 0.144.5`; the candidate's embedded
  release manifest admits exactly `codex-cli 0.144.4` with the observed
  executable SHA-256, so current admission would reject it before model work.
  No model, repository fixture, operation, commit, push, tag, or remote
  mutation was started.

## EXT-19 Exact Candidate Remote CI And Artifact Attestation (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI plus tested-binary evidence.
- Under the operator's explicit raw-Git publication authorization, remote
  branch `level1-v0.1.0-rc.1` names exact candidate source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`. Push-triggered CI run
  `29612464054` completed successfully on that SHA; all ten mandatory EXT-15
  jobs succeeded.
- Supplemental workflow commit
  `a1afdd73a7bfb03e9e5ef361616604115f9db5b8` is published at remote branch
  `level1-v0.1.0-rc.1-attestation`. It checks out the immutable candidate SHA,
  uses Go 1.26.5 and the exact EXT-18 release commands, verifies embedded
  version/source metadata, and compares the Linux, Darwin, and FreeBSD amd64
  binaries with the recorded EXT-18 SHA-256 values.
- Attestation run `29615752091` completed successfully and retained artifact
  `level1-v0.1.0-rc.1-attestation`, size 35,090,832 bytes, with GitHub artifact
  digest
  `sha256:def158256b667447248a0370ee6e2dbe724b2dc1971216300e21751d706ff94f`.
  The artifact is not expired. Its run URL is
  `https://github.com/ponchione/revolvr/actions/runs/29615752091`; the candidate
  CI URL is `https://github.com/ponchione/revolvr/actions/runs/29612464054`.
- Controller verification used raw `git` for local and remote refs and the
  official read-only GitHub Actions REST projections for run, job, and artifact
  conclusions. No `gh`, tag, merge, rebase, or Revolvr publication operation
  was used.
- Verification result: the remote candidate identity, complete required CI
  matrix, exact release-binary hashes, and retained artifact evidence all
  passed. EXT-19 is complete.
- What remains: `EXT-20`, the quantitative Level-1 real-Codex dogfood gate.
  External-project use remains unapproved pending EXT-20 and EXT-21.
- Blockers for EXT-19: none.

## EXT-19 Supplemental Candidate Attestation — Fresh Local Verification (2026-07-17)

- Task selected: `EXT-19`, add the smallest supplemental remote attestation
  workflow for exact Level-1 candidate commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` while leaving completion gated on
  its remote result.
- The already-present
  `.github/workflows/level1-candidate-attestation.yml` requires no repair. It
  triggers only on pushes to `level1-v0.1.0-rc.1-attestation`, checks out the
  exact candidate commit, installs Go 1.26.5, reproduces the EXT-18 Linux,
  Darwin, and FreeBSD amd64 command/environment/version metadata, compares the
  three recorded SHA-256 values, and uploads the binaries, metadata, and
  `SHA256SUMS` as one artifact.
- Files changed in this pass: this state file only. The workflow remains
  unchanged, `.agent/TASKS.md` remains unchanged with EXT-19 unchecked, and
  `.agent/DECISIONS.md` remains unchanged because no implementation or
  architecture authority changed. Candidate source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f` remains the exact immutable build
  source.
- Verification commands run: all required durable-state reads; raw `git`
  worktree, history, candidate-object, ancestry, and workflow inspection;
  PyYAML BaseLoader parsing plus exact trigger/checkout/toolchain/build/hash/
  artifact assertions; host `go1.26.5` verification; execution of the actual
  embedded workflow shell block in a fresh detached clone of the candidate;
  hash-manifest verification; the pinned EXT-18 bundle `--verify` command; and
  `git diff --check`. An initial command wrapper was rejected before execution
  because of its temporary-cleanup spelling; the same check was rerun with an
  exact temporary path and completed successfully without repository changes.
- Verification result: local workflow structure and execution passed. The
  rebuilt Linux, Darwin, and FreeBSD amd64 artifacts matched the recorded
  EXT-18 SHA-256 values
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  The uploaded-file projection contained the three binaries, their build
  metadata, Go/source/version metadata, and `SHA256SUMS`; the complete EXT-18
  bundle also reverified.
- What remains: the controller must verify the workflow commit/ref, confirm a
  successful remote attestation job, and record its run URL, conclusion, and
  retained artifact evidence. Only then may EXT-19 be marked complete.
- Blockers: this local pass does not establish the remote workflow conclusion
  or artifact retention. No commit, push, tag, `gh`, or nested Codex operation
  was used.

## EXT-19 Supplemental Candidate Attestation Workflow (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence. This pass implements only the operator-directed supplemental
  remote binary attestation workflow and does not publish it.
- `.github/workflows/level1-candidate-attestation.yml` triggers only on a push
  to `level1-v0.1.0-rc.1-attestation`, explicitly checks out candidate source
  commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`, installs Go 1.26.5,
  rebuilds Linux/Darwin/FreeBSD amd64 with the exact EXT-18 environment and Go
  flags, validates and retains `go version -m` evidence, compares all three
  SHA-256 values with the recorded EXT-18 values, and uploads the binaries,
  metadata, and `SHA256SUMS` as one workflow artifact.
- Files changed in this pass:
  `.github/workflows/level1-candidate-attestation.yml` and this state file.
  `.agent/TASKS.md` remains unchanged and EXT-19 remains unchecked.
  `.agent/DECISIONS.md` remains unchanged because the workflow directly
  implements already-settled release authority rather than changing it. The
  pre-existing untracked `agent-ext19.sh` was not modified.
- Verification commands run: the required durable-state reads; worktree and
  exact candidate inspection using raw `git`; PyYAML BaseLoader parsing plus
  assertions for the exact trigger, checkout, toolchain, build flags, metadata
  checks, recorded hashes, and artifact upload; execution of the workflow's
  extracted build-and-attest shell block in a fresh detached clone of the
  candidate commit under host Go 1.26.5; the pinned EXT-18 bundle `--verify`
  command; untracked-workflow whitespace validation; and `git diff --check`.
- Verification result: local workflow structure and execution passed. The
  fresh candidate rebuild produced exact EXT-18 SHA-256 values for Linux
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  Darwin `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and FreeBSD
  `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  The complete immutable EXT-18 bundle also reverified.
- What remains: the controller must verify, commit, and push the supplemental
  workflow to the exact trigger branch, wait for its remote job to succeed,
  and record the run URL, conclusion, and retained artifact evidence. EXT-19
  must remain unchecked until that remote workflow passes.
- Blockers: the required remote workflow has not run because this pass is
  expressly prohibited from committing or pushing. No commit, push, tag,
  `gh`, or nested Codex operation was used.

## EXT-19 Remote CI Observed — Completion Still Blocked (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-19 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: the required durable-state reads; `git status
  --branch --porcelain=v2`; `git rev-parse --verify HEAD`; `git branch
  --show-current`; `git remote -v`; `git branch -vv`; recent `git log`; exact
  candidate/evidence commit inspection; `command -v gh`; `git ls-remote
  --heads origin`; read-only GitHub commit, workflow, and status connector
  queries; the public GitHub Actions run, jobs, and artifacts API projections;
  `.github/workflows/ci.yml` inspection; local candidate manifest and artifact
  hashing; the candidate bundle's pinned `--verify` command; and `git diff
  --check`.
- Verification result: remote branch `level1-v0.1.0-rc.1` points to exact
  candidate source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f` while
  remote `main` remains `e76280cc93404aab403f8fe34036e6971e58bb78`.
  Push-triggered CI run `29612464054` on the candidate commit completed on its
  first attempt with conclusion `success`; all ten required jobs and their
  exact-source assertions succeeded. The run URL is
  `https://github.com/ponchione/revolvr/actions/runs/29612464054`.
- The immutable EXT-18 bundle still verifies. Its Linux, Darwin, and FreeBSD
  amd64 SHA-256 values remain
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
- What remains: obtain direct operator authorization or an explicit operator
  confirmation naming the already-published exact commit and target ref. Also
  obtain remote evidence that hashes the binaries actually tested by CI and
  compares them with EXT-18. This run retained zero workflow artifacts, and
  the candidate workflow builds only into runner-temporary paths without
  hashing or uploading those outputs, so that required comparison cannot be
  reconstructed from run `29612464054`.
- Blockers: this pass contains no explicit commit/push authorization, and
  EXT-19 expressly forbids completion without it; remote state alone is not
  authorization. The required tested-binary hash evidence is also absent. The
  `gh` executable is unavailable, though the official read-only Actions API
  exposed the run and job conclusions. No push, commit, tag, workflow rerun,
  or other remote mutation was attempted.

## EXT-19 Remote CI Gate Blocked — Missing Push Authorization (2026-07-17)

- Task selected: `EXT-19`, push the exact Level-1 candidate and obtain remote
  CI evidence.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-19 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --branch --porcelain=v2`; `git
  rev-parse --verify HEAD`; `git branch --show-current`; `git remote -v`;
  `git branch -vv`; recent `git log`; `git show --stat --summary` for candidate
  source commit `ed65049fba6bf82852fd406ebc17afa90a953e3f` and evidence commit
  `413c3f11053f8d04e2aca10c5d5d33d38078ae29`; `git diff --check`; untracked
  file inspection; `git ls-remote --heads origin`; an attempted `gh auth
  status`/workflow query; and read-only GitHub connector checks for repository,
  branch, candidate-commit, and candidate-workflow authority.
- Verification result: blocked before publication. The local tree is clean at
  `413c3f11053f8d04e2aca10c5d5d33d38078ae29` on `main`, two commits ahead of
  `origin/main`. The ignored release artifacts are bound to exact source commit
  `ed65049fba6bf82852fd406ebc17afa90a953e3f`; the later local commit records
  EXT-18 evidence only. GitHub has only `main` at
  `e76280cc93404aab403f8fe34036e6971e58bb78`, reports the candidate source
  commit absent, and has no workflow run for it. The required `gh` executable
  is also unavailable in this environment.
- What remains: obtain direct operator authorization naming the exact push and
  target ref, publish the candidate source commit without using Revolvr, wait
  for the complete EXT-15 workflow on that exact commit, inspect every required
  job, compare tested artifact hashes with EXT-18, and record the CI URL and
  conclusions in release-decision evidence.
- Blocker: this pass contains no explicit commit/push authorization, and EXT-19
  expressly forbids completion without it. Pushing `main` as-is would test the
  later evidence commit rather than make the EXT-18 source commit the pushed
  ref tip, so the target ref must be explicit. No push, commit, tag, workflow
  rerun, or remote mutation was attempted.

## EXT-18 Reproducible Level-1 Release Candidate (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Candidate `level1-v0.1.0-rc.1` uses release version `0.1.0`, exact source
  commit `ed65049fba6bf82852fd406ebc17afa90a953e3f`, and the current official
  stable patched toolchain `go1.26.5`. The exact future artifact is versioned
  now so Level-1 dogfood tests the bytes eligible for the later `v0.1.0`
  decision rather than a differently versioned development build.
- The ignored local bundle at
  `.revolvr/release-candidates/level1-v0.1.0-rc.1-ed65049fba6b/` contains the
  pinned build instructions, Linux/Darwin/FreeBSD amd64 artifacts, `go version
  -m` evidence, byte-for-byte duplicate-build comparison, canonical candidate
  manifest, complete SHA-256 inventory, and inventory digest. Its inventory
  SHA-256 is
  `7a87c571f59a758fcf979acd9980e9799fda0a0c06bc9be4dce8ca44f37b1dde`;
  build-instruction SHA-256 is
  `6e1782dedfd56b6e0ac4350d35a8379650ac2aa7af34e1f1b41057272cae9b84`.
- Artifact SHA-256 values are Linux amd64
  `6239ec551a01b96b95dbaa2aac50ff3036f8f1ccccfff785f1136cd82323591a`,
  Darwin amd64
  `1c28e844196e88dd03daffde2a24a417d88571ab31bba2b022438b9453aa9fdb`,
  and FreeBSD amd64
  `8b7860b801e30f7d36258cde1da4a8af5e9cb312177bd46fc0003a439fca0e17`.
  Both independent clean-clone builds produced those exact hashes.
- The sibling `-verification` bundle retains source-floor, host, vet, module,
  vulnerability, and candidate-verification logs under complete read-only
  SHA-256 inventory. Its inventory SHA-256 is
  `0dcccc3ae6051791fd10effeac754ad2d5c6dcdc8b61fd0900e3c862aefe68f2`.
  `govulncheck` found zero reachable and zero imported-package
  vulnerabilities. Its one separately retained module-only finding is
  `GO-2026-5024` in `golang.org/x/sys@v0.30.0`, a Windows-only
  `NewNTUnicodeString` symbol Revolvr does not call; the report names v0.44.0
  as fixed.
- Files changed in this pass: `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md`; ignored local candidate instructions, artifacts, and
  verification evidence were added under `.revolvr/release-candidates/`. No
  Go source, dependency, commit, push, tag, or real Codex operation changed.
  The first generated bundle failed its own verification because its recorded
  `go version -m` filename named a temporary artifact. The one permitted
  repair records metadata from the final installed artifact; the failed
  diagnostic bundle is retained with suffix `.failed-metadata-path`.
- Verification commands: clean-source Git admission; official Go release
  inspection; `bash -n` on the pinned instructions; two independent clean
  builds of every supported target; bundle inventory/metadata/version
  verification; `go version`; `GOTOOLCHAIN=go1.22.12 go version`;
  `GOTOOLCHAIN=go1.22.12 go test -count=1 ./...`; `go test -count=1 ./...`;
  `go vet ./...`; `go mod verify`; `govulncheck ./...`; `govulncheck -show
  verbose ./...`; and complete verification-evidence inventory checks.
- Verification result: all required source-floor, host, static, module,
  vulnerability, supported-build, reproducibility, metadata, and inventory
  checks passed after the single metadata-path repair.
- What remains: `EXT-19`, push this exact candidate commit and obtain remote
  CI evidence. That task must not push without direct operator authorization.
  External-project use remains unapproved pending EXT-19 through EXT-21.
- Blockers for EXT-18: none.

## EXT-18 Release Candidate Blocked — Fresh Recheck (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-18 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --branch --porcelain=v2`; `git
  rev-parse --verify HEAD`; `git describe --tags --always --dirty`; `git diff
  --stat`; and `git ls-files --others --exclude-standard`.
- Verification result: blocked before release construction. `HEAD` remains
  exact commit `e76280cc93404aab403f8fe34036e6971e58bb78`, but the candidate
  source remains outside that commit as 45 tracked-file modifications plus
  untracked EXT-01 through EXT-17 release files. `git describe` reports
  `e76280c-dirty`, so duplicate builds, candidate hashes, vulnerability
  conclusions, supported-platform artifacts, and embedded source metadata
  would not be authoritative release evidence.
- What remains: obtain direct operator authorization to review and commit the
  complete intended source tree as one exact candidate source commit, then run
  EXT-18 in a fresh pass to finalize immutable build instructions, build twice
  in fresh directories, compare hashes, run the full test/vet/module/
  vulnerability/platform matrix, and verify embedded version/source metadata.
- Blocker: this pass explicitly forbids commits. Making the tree clean would
  require committing the intended release source or destructively discarding
  or hiding repository changes; neither action is authorized. No repair was
  attempted because every available repair would violate the pass rules or
  destroy user-owned work.

## EXT-18 Release Candidate Blocked (2026-07-17)

- Task selected: `EXT-18`, produce a reproducible, versioned Level-1 release
  candidate from one clean exact source commit.
- Files changed in this pass: this state file only. `.agent/TASKS.md` remains
  unchanged and EXT-18 remains unchecked. `.agent/DECISIONS.md` remains
  unchanged because no durable implementation or architecture decision was
  made.
- Verification commands run: `git status --short`; `git status --branch
  --porcelain=v2`; `git rev-parse HEAD`; `git describe --tags --always
  --dirty`; `git tag --list --sort=-version:refname`; and a focused source scan
  for existing version, build-metadata, and reproducibility surfaces.
- Verification result: blocked before candidate construction. The checkout is
  based on exact commit `e76280cc93404aab403f8fe34036e6971e58bb78` but has
  dozens of tracked modifications and untracked files containing the completed
  EXT-01 through EXT-17 work. It is therefore not the clean exact source
  commit required by EXT-18, and no candidate hash, duplicate build,
  vulnerability conclusion, supported-platform artifact, or embedded metadata
  claim can be authoritative for this tree.
- What remains: with direct operator commit authorization, commit the complete
  intended source tree as one reviewed candidate source commit, then run EXT-18
  in a fresh pass to add or finalize immutable build instructions, build twice
  in fresh directories, compare hashes, run the full test/vet/module/
  vulnerability/platform matrix, and verify embedded version/source metadata.
- Blocker: this pass forbids commits. No repair was attempted because making
  the tree clean would require either committing the intended release source or
  destructively discarding/hiding repository changes; neither action is
  authorized.

## EXT-17 Wrong-Codex Refusal Oracle Repair (2026-07-17)

- Task selected: `EXT-17`, repair and freshly verify the opt-in Level-1
  dogfood evidence collector after the wrong-Codex no-mutation oracle failed
  nondeterministically.
- The failure was reproduced three times in 30 runs. `stat_fields` emitted
  literal `\\t` text, so the Git index projection's `cut -f1-3` retained its
  volatile mtime. A read-only status refresh crossing a one-second boundary
  then appeared to change Git authority even though the index bytes, mode,
  size, and link count were unchanged. Stat fields now contain real tab
  delimiters, preserving the intended metadata while excluding only index
  mtime from semantic Git authority. Any future Git projection mismatch prints
  the exact recursive diff before the temporary diagnostics are removed.
- Files changed in this fresh pass:
  `scripts/dogfood-external-level1.sh`, `.agent/TASKS.md`, and this state file.
  No dependency, production Go code, durable architecture decision, or commit
  was added.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; 50
  consecutive wrong-Codex fault repetitions; two independent `--fixture-only`
  collections; canonical manifest comparison after removing only
  `collected_at_utc`; `--verify-manifest` for both bundles; all four dirty,
  non-disposable, wrong-binary, and wrong-Codex refusal fixtures; explicit
  missing, changed, extra-regular-file, symlink, and hard-link tampering; `git
  diff --check`; and a no-index whitespace check for the untracked collector.
- Verification result: every syntax, repetition, deterministic collection,
  manifest, refusal, no-mutation, tamper, and diff-hygiene check passed. All 50
  wrong-Codex repetitions returned the intended status 64 with unchanged
  authority evidence.
- What remains: `EXT-18`, the reproducible versioned Level-1 release
  candidate. Blockers for EXT-17: none. Real dogfood remains intentionally
  uncollected until exact candidate and remote-CI authority exist;
  external-project use remains unapproved.

## Controller Rejection — EXT-17 Flaky Wrong-Codex Refusal Oracle (2026-07-16)

- EXT-17 is not complete. In the controller's independent complete matrix, the
  wrong-Codex fixture exited `1` with `rejected wrong-codex input changed Git
  authority` instead of proving the required pre-mutation refusal.
- Five immediate isolated repetitions of the same wrong-Codex case exited the
  intended `64` with unchanged-authority evidence. That inconsistency makes
  the refusal/no-mutation oracle nondeterministic; a passing retry does not
  erase the failed required verification occurrence.
- The hard-link repair itself passed: both original bundles verified, missing,
  changed, extra-file, symlink, and hard-link tampering were rejected, and the
  reproduced aliased files had link count two. Syntax, canonical-manifest
  comparison, the other refusal cases, and diff hygiene also passed.
- Repair requires reproducing and removing the wrong-Codex Git-authority
  nondeterminism, retaining enough diagnostic evidence to identify any future
  mismatch, and rerunning the complete EXT-17 matrix reliably.
- EXT-17 is restored to unchecked. EXT-18 has not been assessed or authorized.
  Blocker: none; run one fresh pass on EXT-17 only.

## EXT-17 Hard-Link Alias Repair And Fresh Verification (2026-07-16)

- Task selected: `EXT-17`, repair and freshly verify the opt-in Level-1
  dogfood evidence collector after the reproduced hard-link substitution.
- Inventory creation and manifest verification now require every regular
  evidence file to have exactly one link before and after hashing. The
  manifest, file inventory, and bundle digest receive the same single-link
  check, so byte-identical paths cannot share inode authority.
- Fixture-only collection permanently copies its completed bundle, replaces
  `identity/doctor.err` with a hard link to the byte-identical
  `identity/config-check.err`, and requires verification of that copied bundle
  to fail without changing the retained fixture evidence.
- Files changed in this fresh pass:
  `scripts/dogfood-external-level1.sh`, `.agent/TASKS.md`, and this state file.
  No dependency, production Go code, durable architecture decision, or commit
  was added.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; two
  independent `--fixture-only` collections; canonical manifest comparison
  after removing only `collected_at_utc`; `--verify-manifest` for both original
  bundles; all four dirty/non-disposable/wrong-binary/wrong-Codex refusal
  fixtures; explicit missing, changed, extra-regular-file, symlink, and
  reproduced hard-link tampering; `git diff --check`; and a no-index whitespace
  check for the untracked collector.
- Verification result: every required collection, determinism, validation,
  refusal, tamper, and diff-hygiene check passed. The hard-linked
  `config-check.err`/`doctor.err` pair had link count two and was rejected.
- What remains: EXT-18, the reproducible versioned Level-1 release candidate.
  Blockers for EXT-17: none. Real dogfood remains intentionally uncollected
  until exact candidate and remote-CI authority exist; external-project use
  remains unapproved.

## Controller Rejection — EXT-17 Manifest Alias Verification (2026-07-16)

- EXT-17 is not complete. Its durable decision says bundle verification
  rejects aliased evidence, but independent verification replaced
  `identity/doctor.err` with a hard link to the byte-identical
  `identity/config-check.err`; both paths then had link count two and
  `--verify-manifest` still exited zero.
- The required syntax check, two independent fixture-only collections,
  canonical-manifest comparison, verification of both original bundles, all
  four pre-admission refusal fixtures, `git diff --check`, and no-index script
  whitespace check passed. Missing, changed, extra-regular-file, and symlink
  tampering were also correctly rejected.
- Repair requires inventory creation and verification to reject aliased
  regular evidence, with a permanent regression for the reproduced hard-link
  substitution, followed by the complete EXT-17 verification matrix.
- EXT-17 is restored to unchecked. EXT-18 has not been assessed or authorized.
  Blocker: none; run one fresh pass on EXT-17 only.

## EXT-17 Level-1 Dogfood Evidence Collector (2026-07-16)

- Task selected: `EXT-17`, add the opt-in Level-1 external-project dogfood
  evidence collector.
- `scripts/dogfood-external-level1.sh` now refuses a real operation unless the
  external repository is clean, non-bare, explicitly and doubly identified as
  disposable, and bound to the exact approved config, candidate binary hash,
  clean Go VCS source revision, Revolvr version output, listed Codex version/
  digest/path, task, operation, finite cycle bound, expected typed outcome,
  outside sentinel, declared UTC evidence time, and new external evidence
  directory. Candidate config check and exact-task attended doctor must pass
  before the evidence directory or operation is started.
- Each operation bundle records before/after source HEAD, branch, status,
  index, refs, diffs, worktrees, canonical task/runtime trees, and complete
  outside-sentinel metadata/content; effective config and resource bounds;
  task/state/operation history, runs, receipts, completion, workspace, ledger,
  export/replay validation, resource/disk/output use, and the typed outcome.
  The manifest and every regular evidence file are covered by a canonical
  SHA-256 inventory plus an inventory digest, and `--verify-manifest` rejects
  missing, extra, symlinked, or changed evidence.
- `--fixture-only` builds a deterministic VCS-stamped candidate and disposable
  external fixture without invoking a model. Two independent bundles produced
  identical manifests after removing only the declared `collected_at_utc` row
  and both verified. Fixture faults for dirty, non-disposable, wrong-candidate,
  and wrong-Codex input each exited nonzero after proving the complete source
  tree, semantic Git/index authority, and outside sentinel unchanged and that
  no evidence directory was created.
- Files changed for EXT-17: `scripts/dogfood-external-level1.sh`,
  `.agent/TASKS.md`, `.agent/DECISIONS.md`, and this state file. No dependency
  or commit was added, and no real Codex process was started.
- Verification commands: `bash -n scripts/dogfood-external-level1.sh`; two
  executions of `scripts/dogfood-external-level1.sh --fixture-only`; canonical
  manifest comparison after removing only the declared collection-time row;
  `--verify-manifest` for both bundles; all four refusal fixtures with absence
  of evidence assertions; `git diff --check`; and a no-index whitespace check
  for the new untracked script.
- Verification result: the complete required matrix passed after one repair.
  The first full refusal matrix exposed that a read-only Git status can refresh
  volatile `.git` timestamps; the repaired no-mutation oracle excludes those
  timestamps while separately comparing exact source evidence, refs, status,
  diffs, worktree registrations, and raw index bytes/mode/size/link count.
- What remains: EXT-18, the reproducible versioned Level-1 release candidate.
  Blockers for EXT-17: none. Real dogfood evidence remains intentionally
  uncollected until the exact EXT-18/EXT-19 candidate authority exists, and
  external-project use remains unapproved.

## EXT-16 Attended External-Project Runbook (2026-07-16)

- Task selected: `EXT-16`, write and smoke-test the attended external-project
  operator runbook.
- `docs/external-project-runbook.md` covers immutable release/hash pinning,
  initialization and protected path modes, attended configuration and safety
  responsibilities, every finite Level-1 default and its preflight/run
  evidence, task authoring/import/migration/scheduling/checkpoint/input,
  foreground start/monitor/cancel/restart, evidence inspection, every typed
  Level-1 stop and recovery boundary, exact confirmed reconciliation,
  workspace review and operator-only integration/removal, archive, export,
  retention, upgrade, and guarded runtime-state retirement. Queue and daemon
  remain explicitly unapproved.
- `scripts/smoke-external-attended.sh` builds a disposable versioned binary
  and external Git repository, exercises every documented non-destructive
  command or safe refusal plus all referenced help surfaces, verifies ledger
  export/replay and retention planning, proves no Codex execution, and safely
  retires only the disposable runtime tree.
- Notification list/show now resolve an omitted work directory through the
  shared app state-path boundary. Focused CLI coverage proves current-directory
  resolution and that missing notification reads create no runtime state.
- Files changed for EXT-16: `docs/external-project-runbook.md`,
  `scripts/smoke-external-attended.sh`, `internal/app/notification_inspect.go`,
  `internal/cli/autonomous_run_test.go`, `.agent/TASKS.md`, and this state
  file. No dependency or commit was added; no durable architecture decision
  changed.
- Verification commands: `gofmt -w internal/app/notification_inspect.go
  internal/cli/autonomous_run_test.go`; `go test -count=1 ./internal/cli -run
  '^TestNotificationWarningRenderingAndReadOnlyInspection$'`; `bash -n
  scripts/smoke-external-attended.sh`; `bash
  scripts/smoke-external-attended.sh`; `go test -count=1 ./...`; `go run
  ./cmd/revolvr --help` and 34 referenced subcommand help invocations; and
  `git diff --check`.
- Verification result: the focused test, shell syntax check, complete
  disposable smoke, full Go suite, help inventory, formatting, and whitespace
  checks passed. The first complete smoke after the notification repair found
  fixture cleanup paths anchored to the source checkout; the one permitted
  repair computed the guarded retirement paths inside the disposable fixture,
  and the rerun passed.
- What remains: EXT-17, the opt-in Level-1 dogfood evidence collector. There
  are no blockers for EXT-16; external-project use remains unapproved until
  the remaining ordered release gates pass.
- Blockers: none.

## EXT-16 Attended Runbook Blocked (2026-07-16)

- Task selected: `EXT-16`, write and smoke-test the attended external-project
  operator runbook.
- `docs/external-project-runbook.md` now covers release/hash pinning,
  initialization and protected path modes, attended configuration and safety
  responsibilities, every finite Level-1 default and its preflight/run
  evidence, task authoring/import/migration/scheduling/checkpoint/input,
  foreground start/monitor/cancel/restart, evidence inspection, typed stops,
  every Level-1 recovery boundary, explicit task recovery, workspace review
  and operator-only integration/removal, archive, export/retention, upgrade,
  and guarded runtime-state retirement. It explicitly marks queue and daemon
  unapproved.
- `scripts/smoke-external-attended.sh` builds a disposable versioned binary,
  creates and initializes an external Git fixture, uses a no-model unlisted
  fake Codex, exercises the documented non-destructive/read-only commands and
  safe refusals, checks referenced command help, and guards runtime retirement.
- Files changed in this pass: `docs/external-project-runbook.md`,
  `scripts/smoke-external-attended.sh`, and this state file. No dependency or
  commit was added. `.agent/TASKS.md` remains unchanged and EXT-16 remains
  unchecked.
- Verification commands run: `bash -n
  scripts/smoke-external-attended.sh`; `git diff --check --
  docs/external-project-runbook.md scripts/smoke-external-attended.sh`; and
  `bash scripts/smoke-external-attended.sh` twice. The first smoke run exposed
  an invalid empty-body Markdown import fixture; the one permitted repair added
  explicit task-body text. Shell syntax and the initial diff check passed.
- Verification result: blocked after the repaired full smoke run reached the
  production command `revolvr notification list`. In an ordinary initialized
  current-directory fixture it returns `harness runtime path: repository root
  is required`. `internal/cli` passes an empty default `Options.WorkDir` to
  `app.ListNotifications`, which forwards it directly to
  `autonomousnotification.List` instead of resolving the current canonical
  repository root as other app read projections do.
- What remains: in a fresh EXT-16 pass, make notification list/show resolve
  the omitted work directory through the ordinary app state-path boundary,
  add focused CLI coverage, then rerun the complete required smoke/help/diff
  verification. Re-review the generated runbook/script diff before marking
  EXT-16 complete.
- Blocker: the production notification inspection root-resolution defect
  prevents the required runbook smoke from completing. No second repair was
  attempted in this pass, as required by the fresh-loop verification rule.

## EXT-15 Exact-Candidate Release CI Matrix (2026-07-16)

- Task selected: `EXT-15`, make the complete release CI matrix mandatory for
  the exact candidate commit.
- `.github/workflows/ci.yml` now triggers on every pushed branch and tag plus
  every pull request, with no path filters, job conditions, or dependency
  chains. Independent required jobs cover the Go 1.22 source floor and full
  suite, the six-test production autonomous strict-fake suite, the full race
  suite, `go vet`, module verification, each fake-Codex smoke path, supported
  Linux/macOS/FreeBSD amd64 builds, and the separate Windows diagnostic stub.
  Every job compares checked-out `HEAD` with `GITHUB_SHA` and publishes that
  exact source commit in the job summary.
- The two mixed-pass smoke fakes now report the structurally exact version
  `codex-cli 1.2.3`. That unlisted identity remains diagnostic rather than
  release authority, while the smoke fixtures can exercise the current config
  and run-once paths instead of failing on the obsolete `fake-codex` version
  grammar.
- Files changed for EXT-15: `.github/workflows/ci.yml`,
  `scripts/smoke-run-once-fake-codex.sh`,
  `scripts/smoke-run-once-fake-codex-verification-failure.sh`,
  `.agent/TASKS.md`, `.agent/DECISIONS.md`, and this state file. No dependency
  or commit was added.
- Verification commands: a PyYAML parse plus explicit trigger/job/command/SHA
  structural assertions; `GOTOOLCHAIN=go1.22.12 go version`; Go 1.22.12
  `go test ./...`; the explicit six-test production strict-fake command; full
  host and Go 1.22.12 `go test -race -count=1 ./...`; Go 1.22.12 `go mod
  verify` and `go vet ./...`; Bash syntax and execution of both fake-Codex
  smokes; Go 1.22.12 Linux, Darwin, FreeBSD, and Windows amd64 builds; the
  Windows unsupported-platform string assertion; and `git diff --check`.
- Verification result: workflow syntax/structure and every locally
  reproducible job passed. The first smoke execution exposed both stale fake
  version strings; the single bounded repair changed the reported and expected
  versions to the current exact grammar, after which both host and Go 1.22.12
  smoke executions passed. Remote execution proof remains intentionally owned
  by EXT-19.
- What remains: EXT-16 and the later ordered external-readiness tasks.
  Blockers for EXT-15: none. Autonomous external-project use remains
  unapproved.

## EXT-14 Fresh Verification Pass (2026-07-16)

- Task selected: `EXT-14`, prove Level-1 task and explicit-administration
  interruption recovery at every production durable transition seam.
- The existing production matrix and nil-by-default failure hooks were
  inspected for all 18 before/after task, notification, and archive seams.
  Task restart preserves the stable operation and stops in-flight work as
  `unsafe_or_ambiguous`; notification replay retains its delivery ID; archive
  restart rolls the admitted journal forward without duplicating publication.
- Files changed in this fresh pass:
  `internal/app/production_interruption_recovery_test.go`, `.agent/TASKS.md`,
  and this state file. Receipt evidence now compares the exact artifact path,
  mode, and content-hash tree across restart rather than only its count. No
  dependency or commit was added.
  `.agent/DECISIONS.md` was not changed because the durable interruption
  ownership decision was already recorded.
- Verification commands: `gofmt -w` on the seven EXT-14 Go files; `go test
  -count=1 ./internal/app -run
  '^TestProductionTaskInterruptionRecoveryMatrix$'`; the same focused command
  with `-race`; `go test -count=1 ./...`; a final `gofmt -l` check on the seven
  EXT-14 Go files; and `git diff --check`.
- Verification result: the ordinary 18-seam matrix, race-enabled matrix, and
  complete repository suite passed. Stable task operation/delivery/archive
  identities replayed without duplicate commits, attempt charges,
  notification success claims, task completion, terminal ledger evidence,
  receipts, completion artifacts, or archives. The first focused rerun exposed
  that a numeric at-most-one receipt assertion rejected two legitimate worker
  receipts at later seams; the single repair replaced it with exact receipt-
  tree equality, which directly proves restart adds or rewrites no receipt.
  Formatting and diff-hygiene checks also passed.
- What remains: EXT-15 and the later ordered external-readiness tasks.
  Blockers for EXT-14: none. Autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-13 Read-only Recovery Repair (2026-07-16)

- Task selected: `EXT-13`, repair the Level-1 autonomous recovery command so
  default inspection remains strictly read-only when the live workspace HEAD
  has drifted from durable authority.
- `autonomousworkspace.Inspect` now verifies the same marker, deterministic
  workspace identity, Git common directory, worktree registration, linked
  `.git` file, branch, HEAD, tree, source revision, and cleanliness as reopen,
  but acquires no Git-administration mutation lease and never publishes a
  retained ambiguity ref. The mutating reopen path retains its established
  retained-ref recovery behavior.
- App recovery uses that read-only projection and reports expected and
  observed Git HEAD/tree/source identities. The focused regression advances
  the real task-workspace HEAD, proves the Git authority check fails, and
  compares every Git ref plus the complete repository contents and metadata
  before and after inspection.
- Files changed for this pass: `internal/autonomousworkspace/manager.go`,
  `internal/app/autonomous_recovery.go`,
  `internal/app/autonomous_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on the three changed Go files; `go test
  -count=1 ./internal/app ./internal/cli -run
  'Test(RecoverAutonomousTaskRequiresExactReconciliation|TaskRecoveryCommand)$'`;
  the same focused command with `-race`; `go run ./cmd/revolvr task --help`;
  and `go test -count=1 ./...`.
- Verification result: the focused ordinary/race tests, CLI help, and complete
  repository suite passed. The advanced-HEAD case retained byte-for-byte and
  metadata-for-metadata repository evidence and the exact complete ref set.
- What remains: EXT-14 and the later ordered external-readiness tasks.
  Blockers for EXT-13: none. External use remains unapproved.

## Controller Rejection — EXT-13 Read-only Recovery (2026-07-16)

- EXT-13 is not complete. `inspectAutonomousRecovery` calls
  `autonomousworkspace.Reopen`; that path acquires the Git administration lock
  and calls `git update-ref` to publish a retained ambiguity ref when the live
  task-workspace HEAD differs from durable authority. Therefore the default
  `revolvr task recover` inspection is not read-only for the required drift
  case.
- The existing read-only test snapshots only an agreeing workspace and does
  not exercise advanced-HEAD/ref drift, so its passing result does not satisfy
  the exact acceptance criterion.
- Repair requires a genuinely non-mutating workspace/Git inspection path and
  a regression proving complete refs and runtime/source evidence remain
  unchanged when live workspace authority has drifted.
- EXT-14 was completed in the same prior pass and is restored to unchecked so
  it receives its own fresh invocation after EXT-13 passes.
- Blocker: none; run one fresh pass on EXT-13 only.

## EXT-14 Production Interruption Recovery Matrix (2026-07-16)

- Task selected: `EXT-14`, prove Level-1 task and explicit-administration
  interruption recovery at every production durable transition seam.
- Nil-by-default failure injection now brackets supervisor, worker,
  verification, commit, checkpoint, audit, finalization, notification
  delivery, and archive manifest publication. The production matrix exercises
  all 18 before/after points with stable operation or delivery identities.
- Interrupted task operations retain their durable in-flight authority and
  restart as `unsafe_or_ambiguous` without rerunning Codex or changing the
  already-published domain effects. Notification restart reuses its stable
  delivery ID, and archive restart rolls the same admitted journal forward.
- Archive recovery now reconstructs and verifies the exact journal-bound
  manifest when interruption occurred before immutable manifest publication;
  a manifest already published before journal advancement remains the restart
  authority.
- The matrix proves no duplicate source commit, attempt admission/completion,
  notification success, completed task, terminal ledger event, receipt,
  completion artifact, archive entry, or administrative archive commit.
- Files changed for EXT-14: `internal/autonomouscycle/types.go`,
  `internal/autonomouscycle/cycle.go`, `internal/autonomouscycle/worker.go`,
  `internal/app/autonomous_run.go`, `internal/app/notification.go`,
  `internal/autonomousarchive/coordinator.go`,
  `internal/app/production_interruption_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on all changed Go files;
  `go test -count=1 ./internal/app -run
  '^TestProductionTaskInterruptionRecoveryMatrix$'`; the same focused command
  with `-race`; `go test -count=1 ./...`; `git diff --check`; and `gofmt -l`
  on the changed Go files.
- Verification result: the focused 18-seam matrix, focused race run, and full
  repository suite passed. Formatting and whitespace checks passed.
- What remains: EXT-15 and the later ordered external-readiness tasks. There
  are no blockers for EXT-14; autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-13 Explicit Operator Recovery (2026-07-16)

- Task selected: `EXT-13`, add the Level-1 read-only autonomous task recovery
  inspection and exact confirmed reconciliation command.
- `revolvr task recover <task-id> --operation-id <id>` now reports ordered
  task, state, workspace, Git, ledger, receipt, and artifact authority without
  starting a model or mutating recovery state. Missing artifact authorities are
  reported explicitly when no completed run exists.
- Reconciliation requires both `--reconcile` and
  `--confirm-operation <operation-id>`, applies only to terminal
  `unsafe_or_ambiguous` operations, repeats all authority checks under the
  execution lease, refuses drift, and publishes a deterministic new admitted
  operation linked to the immutable old operation and authority digest.
- Existing retry and unblock commands remain independent and cannot invoke or
  clear autonomous recovery. Focused application and CLI tests prove the
  read-only tree, exact confirmation, immutable old operation, deterministic
  replay, and refusal after task drift.
- One verification repair moved the fully-unattended daemon mode prerequisite
  ahead of ambient Codex identity admission. Invalid operator-attended daemon
  requests now fail for their requested mode, while valid unattended requests
  still perform the complete external identity admission.
- Files changed for EXT-13: `internal/app/autonomous_recovery.go`,
  `internal/app/autonomous_recovery_test.go`, `internal/app/autonomous_run.go`,
  `internal/autonomoustaskrun/recovery.go`,
  `internal/autonomoustaskrun/ledger.go`, `internal/cli/root.go`,
  `internal/cli/task_recovery_test.go`, `.agent/TASKS.md`,
  `.agent/DECISIONS.md`, and this state file.
- Verification commands: `gofmt -w` on all changed Go files;
  `go test -count=1 ./internal/app ./internal/cli -run
  'Test(RecoverAutonomousTaskRequiresExactReconciliation|TaskRecoveryCommand)$'`;
  the same focused command with `-race`;
  `go test -count=1 ./internal/autonomoustaskrun`;
  `go run ./cmd/revolvr task --help`; `go test -count=1 ./...`;
  `git diff --check`; and `gofmt -l` on the changed Go files.
- Verification result: all required focused, race, CLI, package, full-suite,
  formatting, and whitespace checks passed after the single repair attempt.
- What remains: EXT-14 and the later ordered external-readiness tasks. There
  are no blockers for EXT-13; autonomous external-project use remains
  unapproved until the remaining release gates pass.

## EXT-12 External Interruption And Recovery Contract (2026-07-16)

- Task selected: `EXT-12`, publish the settled interruption and recovery
  contract as a complete transition-seam matrix.
- `docs/external-recovery.md` defines the shared exact-replay boundary, fresh-
  ephemeral-process rule, immutable old-operation authority, task-scoped
  `unsafe_or_ambiguous` quarantine, Level-1 stop behavior, Level-2/3 unrelated-
  work continuation only after durable exclusion, and the prohibition on
  generic retry or manual runtime-state edits clearing quarantine.
- Ten three-row tables cover before, during, and after supervisor, worker,
  verification, commit, checkpoint, audit, finalization, queue reconciliation,
  notification, and archive publication. Every row explicitly names durable
  restart authority, exact replay, ambiguity handling, permitted L1/L2/L3
  continuation, prohibited inference, and the exact operator inspection or
  reconciliation action.
- The contract distinguishes at-least-once notification recovery from task
  quarantine, preserves receiver-side stable-key deduplication, keeps archive
  administration explicit, and requires the future `task recover` command to
  preserve the old operation while creating a new identity only after exact
  reconciliation.
- Files changed for EXT-12: `docs/external-recovery.md`, `.agent/TASKS.md`, and
  this state file. `.agent/DECISIONS.md` was not changed because the document
  applies the already-settled readiness and owner contracts without adding an
  implementation or architecture decision.
- Verification commands: the required `rg -n` term scan from EXT-12;
  heading/row-count scans proving all ten seams and exactly 30 timing rows;
  `git diff --check`; a no-index `git diff --check` of the new untracked
  document (no whitespace diagnostics; the expected content-difference status
  was 1); and a manual row-by-row cross-check against
  `.agent/AUTONOMOUS_EXTERNAL_READINESS.md` and `.agent/DECISIONS.md` covering
  in-flight quarantine, immutable history/checkpoint precedence, process
  settlement, attempt/verification/commit uniqueness, finalization roll-
  forward, queue exclusion/order, notification idempotency, and archive
  reconciliation.
- Verification result: all required terms, ten seam headings, and 30 matrix
  rows are present; diff hygiene passed; every row agrees with the settled
  readiness and durable owner decisions. No dependency, production code, or
  commit was added.
- What remains: EXT-13 and the later ordered external-readiness gates. Blockers
  for EXT-12: none. External use remains unapproved.

## EXT-11 External Git Containment Edge Matrix (2026-07-16)

- Task selected: `EXT-11`, the real-Git dirty, staged, ignored,
  linked-worktree, SHA-1, SHA-256, concurrent external-commit, and active-
  submodule containment matrix.
- `TestExternalGitContainmentMatrix` enters the public attended admission path
  for dirty, staged, and active-submodule refusals and proves no task step or
  ref/workspace publication begins. The ignored case passes clean Git
  admission, then proves the app workspace preparation boundary rejects
  policy-relevant ignored source before creating the task ref or linked
  workspace.
- Positive real-Git cases create exact task workspaces and path-scoped commits
  from SHA-1 and SHA-256 repositories, including a control root that is itself
  a linked worktree. They prove exact object-ID length, task branch/ref,
  baseline parent, one-path commit delta and bytes, clean matching index/tree,
  workspace reconciliation, and unchanged operator/unrelated authority.
- The concurrent case injects a real operator commit on the control branch
  after the task commit's pre-HEAD lookup and before task staging. The operator
  and task commits remain on their exact independent branches and neither tree
  absorbs the other's path.
- Outside and unrelated-worktree sentinels contain regular, executable,
  symlink, and hard-linked entries. Complete snapshots prove exact entries,
  bytes, modes, targets, timestamps, and link counts plus unrelated branch,
  HEAD, status, and index authority. Index authority itself compares exact
  bytes, size, type, mode, and link count while excluding the non-semantic file
  timestamp that ordinary read-only Git status refreshes.
- Files changed for EXT-11:
  `internal/app/external_git_containment_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w
  internal/app/external_git_containment_test.go`; `go test -count=1
  ./internal/app -run '^TestExternalGitContainmentMatrix$'`; `go test -race
  -count=1 ./internal/app -run '^TestExternalGitContainmentMatrix$'`; `go test
  -count=1 ./...`; a final `gofmt -l` check; and `git diff --check`.
- Verification result: the required focused ordinary/race runs and the full
  repository suite passed. The first focused run refined the fixture oracle to
  distinguish exact index content authority from Git's timestamp-only status
  refresh and normalized linked-checkout `.agent` permissions under the host
  umask; the complete final matrix then passed. No production code, dependency,
  or commit was added.
- What remains: EXT-12 and the later ordered external-readiness gates. Blockers
  for EXT-11: none. External use remains unapproved.

## EXT-10 Run-Owned Commit And Git-Operation Containment (2026-07-16)

- Task selected: `EXT-10`, exact run-owned commit containment plus the
  prohibited production Git-operation and unrelated-worktree boundary.
- The shared commit gate now applies `--literal-pathspecs` and `--only` to the
  exact same sorted paths admitted for staging. A real-Git regression captures
  one source change plus required task metadata, injects unrelated staged,
  tracked-worktree, and untracked bytes afterward, and proves the exact commit
  delta/tree while every late byte and the unrelated staged index entry remain
  unchanged.
- The production happy-path fixture now has a command-spy form that executes
  the ordinary runner, fails immediately on push, merge, rebase, reset, clean,
  or stash, observes real workspace and commit operations, and proves a second
  linked worktree retains its branch, HEAD, status, tracked bytes, untracked
  sentinel bytes, and sentinel mode. The unused destructive workspace restore
  function and its reset/clean-only support code were removed; no production
  caller existed.
- Files changed for EXT-10: `internal/commit/commit.go`,
  `internal/commit/commit_test.go`,
  `internal/app/production_autonomous_happy_path_test.go`,
  `internal/autonomousworkspace/manager.go`,
  `internal/autonomousworkspace/manager_test.go`,
  `internal/runonce/runonce_test.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; the required
  focused ordinary and race commands for
  `TestExternalCommitContainsOnlyRunOwnedDelta|TestProductionAutonomyForbidsRepositoryIntegrationOps`;
  complete commit, autonomous-workspace, and app package tests; `go test
  -count=1 ./...`; a production prohibited-Git-verb source scan; `gofmt -l` on
  every changed Go file; and `git diff --check`.
- Verification result: the required focused ordinary/race runs, all directly
  affected package tests, and the final full repository suite passed. The
  first full-suite run exposed one legacy runonce assertion for the former
  unscoped commit argv; the single expectation repair records the new exact
  literal path-scoped command, and the final full suite passed. No dependency
  or commit was added.
- What remains: EXT-11 and the later ordered external-readiness gates.
  Blockers for EXT-10: none. External use remains unapproved.

## EXT-09 Exact Task-Workspace Authority (2026-07-16)

- Task selected: `EXT-09`, exact task-scoped branch, linked-workspace,
  baseline, control-root, Git-common-directory, registration, marker, and
  current-HEAD authority for external autonomous work.
- `TestExternalTaskWorkspaceAuthority` proves a task workspace is created from
  the requested exact baseline on its deterministic `refs/heads/revolvr/tasks/`
  branch while the ambient operator branch and its newer bytes remain
  untouched. The complete control/execution roots, Git common directory,
  branch, baseline, HEAD, checkpoint, and ownership-marker identities survive
  deterministic durable JSON projection.
- Reopen and commit reconciliation now compare every deterministic workspace
  identity, including the ownership-marker path, before acquiring a Git-admin
  lock. Reusing the task execution worktree as a changed control root therefore
  fails before it can create `.revolvr` state inside task source.
- Ownership markers use the descriptor-rooted runtime-path boundary for
  creation, synchronization, and reads. Linked-worktree `.git` files use a
  bounded no-follow open with regular-file/single-link and pre/post identity
  checks. Marker and `.git` symlinks, foreign workspace paths, common
  directories, refs, baselines, and registrations all fail while exact source
  HEAD, branch, status, and tracked bytes remain unchanged.
- Files changed for EXT-09: `internal/autonomousworkspace/manager.go`,
  `internal/autonomousworkspace/manager_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on both changed Go files; `go test
  -count=1 ./internal/autonomousworkspace -run
  '^TestExternalTaskWorkspaceAuthority$'`; the same focused command with
  `-race`; `go test -count=1 ./internal/autonomousworkspace`; `go test
  -count=1 ./...`; Darwin and FreeBSD amd64 package test cross-compiles;
  `gofmt -l` on both changed Go files; and `git diff --check`.
- Verification result: the required focused ordinary/race runs, complete owner
  package, full repository suite, both supported-platform cross-compiles,
  formatting, and diff checks passed. The first focused run exposed that Git
  may create linked-worktree parent directories with the invoking umask; the
  repair kept marker storage on the strict runtime boundary while validating
  the Git-created `.git` file directly with no-follow identity checks. No
  dependency or commit was added.
- What remains: EXT-10 and the later ordered external-readiness gates. Blockers
  for EXT-09: none. External use remains unapproved.

## EXT-08 Production Attended Terminal Matrix (2026-07-16)

- Task selected: `EXT-08`, the production attended-task terminal-outcome
  matrix through `app.RunTaskUntilTerminal` with no injected task runner.
- One table-driven strict-fake production test now proves separate
  `needs_input`, authorized `blocked`, verification-failure
  `unsafe_or_ambiguous`, identical-strategy `no_progress`, trusted
  `safety_stop`, caller `operation_cancelled`, exact terminal authority replay,
  and `max_cycles` outcomes. Every case enters the public app boundary and the
  ordinary production workspace, supervisor, worker, attempt, verification,
  commit, state, task-run, and ledger composition.
- The matrix asserts exact stop detail and cycle/replay facts; strict model
  invocation/version counts; canonical task bytes/status; control and
  workspace HEAD/status/commit authority; workspace checkpoint identity;
  receipt and verification presence/absence; exact allowed state-history
  categories; task-run ledger shape; model/verification/commit event counts;
  and the absence of completion artifacts, finalization state/runs/events,
  unrelated state history, or other unauthorized effects.
- Trusted supervisor read-only mutation is now preserved as `safety_stop`
  using the cycle's typed `SourceDifference.Changed` evidence. Other
  supervisor failures and verification failures remain
  `unsafe_or_ambiguous`; error prose does not grant trusted safety authority.
- Files changed for EXT-08:
  `internal/app/production_autonomous_terminal_test.go`,
  `internal/app/autonomous_run.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w internal/app/autonomous_run.go
  internal/app/production_autonomous_terminal_test.go`; `go test -count=1
  ./internal/app -run '^TestProductionAutonomousTerminalMatrix$'`; `go test
  -race -count=1 ./internal/app -run
  '^TestProductionAutonomousTerminalMatrix$'`; `go test -count=1 ./...`;
  `git diff --check`; and a final `gofmt -l` check on both changed Go files.
- Verification result: every required focused, race, and full repository test
  passed; formatting and diff checks passed. The first focused run corrected
  exact porcelain-status expectations, nil/empty receipt comparison, the
  two-cycle no-progress fixture, and cancellation contract setup. The tightened
  final matrix then passed ordinary and race execution. No dependency or commit
  was added.
- What remains: EXT-09 and the later ordered external-readiness gates.
  Blockers for EXT-08: none. External use remains unapproved.

## EXT-07 Production Correction And Re-audit (2026-07-16)

- Task selected: `EXT-07`, production correction, distinct final verification,
  exact finding resolution, and clean independent re-audit through
  `app.RunTaskUntilTerminal` without an injected task runner.
- The strict-fake operation records one blocking `incorrect-result` finding,
  admits exactly one correction attempt, changes and commits only
  `docs/result.md`, runs a distinct final verification occurrence, resolves
  the exact finding, runs a distinct clean auditor process, advances one
  checkpoint, and completes. The test asserts the exact attempt pairs, single
  source commit and diff, two verification occurrences, two audit runs, three
  worker receipts, audit-history ordering, frozen evidence, ledger runs, and
  separate control/workspace source authority.
- Top-level audit application now validates its dossier against the exact
  pre-admission execution state while accepting the current state only when it
  is a legal successor differing solely by append-only attempt accounting.
  Verification provenance is compared through its canonical JSON projection,
  matching the worker-visible schema while still requiring exact persisted
  values.
- Correction re-audit receives an ephemeral workspace/state projection at the
  exact corrected commit and source revision; durable checkpoint authority is
  still advanced only after correction, final verification, resolution, and
  clean audit all succeed. Reopening stored audit evidence accepts the same
  exact-or-trimmed profile identity contract used at initial audit admission.
- Files changed for EXT-07:
  `internal/app/production_autonomous_correction_test.go`,
  `internal/app/autonomous_run.go`,
  `internal/autonomousauditapply/apply.go`,
  `internal/autonomouscorrection/coordinator.go`,
  `internal/autonomousstate/audit_store.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; `go test -count=1
  ./internal/app -run '^TestProductionAutonomousCorrectionAndReaudit$'`;
  `go test -count=1 -race ./internal/app -run
  '^TestProductionAutonomousCorrectionAndReaudit$'`; `go test -count=1
  ./internal/autonomousauditapply ./internal/autonomousstate
  ./internal/autonomouscorrection`; `go test -count=1 ./...`; and `git diff
  --check`.
- Verification result: every focused, race, directly affected package, and
  full repository test passed; formatting and diff checks passed. No
  dependency or commit was added.
- What remains: EXT-08 and the later ordered external-readiness gates.
  Blockers for EXT-07: none. External use remains unapproved.

## EXT-06 Production-Composition Happy Path (2026-07-16)

- Task selected: `EXT-06`, the complete attended autonomous happy path through
  `app.RunTaskUntilTerminal` with no injected `TaskRunInput.Runner`.
- One strict-fake operation now reaches the real `productionStepRunner` and
  proves exact workspace creation, supervisor document decision, documentor
  action, attempt admission/completion, final tier verification, run-owned
  commit, checkpoint advancement, fresh independent audit, complete decision,
  frozen evidence, canonical task/state terminalization, and finalization/task
  ledger completion. It asserts exact source placement, Git diff and heads,
  receipt bytes, state/task bytes, workspace marker material identity,
  completion evidence/manifest identities, run/event sets, and task-run
  operation bytes.
- The production path now propagates exact admitted Codex/Git identities and a
  release manifest to every supervisor/worker invocation, injects deterministic
  IDs only through unexported test seams, rehydrates current mutation,
  verification, and audit authority from durable audit history between cycles,
  and carries the worker's role-projected dossier as worker evidence.
- Optional-role composition gives its mandatory nested audit the exact
  post-commit ephemeral workspace authority before the durable checkpoint is
  advanced. Audit application admits the exact auditor role dossier while
  independently retaining supervisor-dossier provenance, and accepts either
  exact profile-file identity or the prompt loader's whitespace-normalized
  profile identity. Checkpoint advancement treats the just-created verified
  run-owned commit as trusted input, while terminal revalidation recognizes
  only its exact matching in-progress finalization envelope.
- Files changed for EXT-06: `internal/app/production_autonomous_happy_path_test.go`,
  `internal/app/autonomous_run.go`, `internal/app/external_admission.go`,
  `internal/app/strict_fake_codex_test.go`,
  `internal/app/testdata/strictfakecodex/main.go`,
  `internal/codexexec/codexexec.go`, `internal/supervisor/execution.go`,
  `internal/autonomouscycle/types.go`, `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/worker.go`,
  `internal/autonomousoptional/coordinator.go`,
  `internal/autonomousauditapply/apply.go`, `.agent/TASKS.md`, this state file,
  and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; `go test -count=1
  ./internal/app -run TestProductionAutonomousHappyPath`; `go test -race
  -count=1 ./internal/app -run TestProductionAutonomousHappyPath`; `go test
  -count=1 ./internal/autonomousauditapply`; `go test -count=1 ./...`; and
  `git diff --check`.
- Verification result: every required focused, race, compatibility-package,
  and full repository test passed. The first full-suite run exposed legacy
  audit fixtures that retained exact profile-file bytes rather than the prompt
  loader's normalized bytes; the single compatibility repair accepts either
  exact representation while preserving the expected digest and size, and the
  final full run passed. No dependency or commit was added.
- What remains: EXT-07 and the later ordered external-readiness gates.
  Blockers for EXT-06: none. External use remains unapproved.

## EXT-05 Strict Reusable Fake-Codex Contract (2026-07-16)

- Task selected: `EXT-05`, one strict reusable fake-Codex contract fixture for
  the production autonomous app test path.
- The fixture is a separately built Go executable under app testdata. Its
  strict sibling contract names the exact version-call count, exec invocation
  order, argv, working directory, prompt, schema bytes, environment-name set
  and exact environment SHA-256, last-message bytes, JSONL event sequence, and
  optional receipt bytes. An atomically replaced sibling state records completed
  version calls, exec calls, and emitted event types.
- The positive contract performs one version probe followed by distinct
  supervisor and worker processes through `codexexec.Run` with its default
  `runner.Run`. Both calls require one fresh `exec --json --ephemeral`
  invocation, forbid `resume`, produce exact last-message and JSONL artifacts,
  and make the worker publish a parseable deterministic receipt. No model,
  network, injected `StepRunner`, command-runner replacement, or in-process
  Codex implementation is used.
- The permanent refusal matrix proves that unexpected argv, working directory,
  schema bytes, environment, invocation count, output-event sequence, and a
  missing ephemeral flag all exit with the fixture's refusal status. An extra
  call after the complete supervisor/worker sequence is also refused. Ambient
  environment values are never persisted in the contract; only sorted names
  and the exact sorted-environment SHA-256 are retained.
- Files changed for EXT-05: `internal/app/strict_fake_codex_test.go`,
  `internal/app/testdata/strictfakecodex/main.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on both new Go files; `go test -count=1
  ./internal/app -run '^TestStrictFakeCodexContract$'`; `go test -race
  -count=1 ./internal/app -run '^TestStrictFakeCodexContract$'`; `go test
  -count=1 ./internal/codexexec ./internal/runner`; `go test -count=1 ./...`;
  and `git diff --check`.
- Verification result: every required focused, race, owner-package, and full
  repository test passed. The focused test was iterated while implementing the
  fixture to correct schema-path and inherited `PWD` expectations; the final
  required matrix is green. No dependency, production behavior, or commit was
  added.
- What remains: EXT-06 and the later ordered external-readiness gates.
  Blockers for EXT-05: none. External use remains unapproved.

## EXT-04 Release-Authored Executable Identity Authority (2026-07-15)

- Task selected: `EXT-04`, exact release-authored Codex executable/version
  admission plus resolved Git executable identity projection and recording.
- An embedded, strict release manifest admits exactly `codex-cli 0.144.4` with
  SHA-256
  `134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477`.
  Executable inspection binds the configured spelling to its canonical
  symlink-resolved regular file and hashes the opened bytes while checking the
  named/opened identity remains stable. Exact version-and-digest equality is
  the only Codex authority; semantic ranges are rejected.
- Shared preflight records and renders the admitted Codex and Git identities.
  Autonomous execution rechecks both identities before fingerprinted task
  effects, and the Codex runner independently rechecks release authorization
  before creating artifacts or invoking the admitted resolved path. The
  identities flow through effective-config schema v7, supervisor/worker
  invocation provenance, and durable run evidence. Config check renders and
  fingerprints the observed exact identities while reporting release refusal
  separately, so it remains diagnostic for an installed but unlisted build.
- Files changed for EXT-04: `internal/codexexec/release_manifest.json`,
  `internal/codexexec/identity.go`, `internal/codexexec/invocation.go`,
  `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`,
  `internal/runonce/runonce.go`, `internal/runonce/effectiveconfig.go`,
  `internal/app/external_admission.go`, `internal/app/autonomous_run.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/external_preflight_test.go`, `internal/app/app_test.go`,
  `internal/autonomouscycle/types.go`, `internal/autonomouscycle/cycle.go`,
  `internal/autonomouscycle/worker.go`, `internal/supervisor/execution.go`,
  `internal/cli/config.go`, `internal/cli/doctor.go`, `internal/cli/root.go`,
  `internal/cli/doctor_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`,
  this state file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; the required focused
  ordinary and race runs for
  `TestExternalExecutableIdentityAdmission|TestReleaseCodexAllowlist`; focused
  CLI regression tests; `go test -count=1 ./...`; `go run ./cmd/revolvr
  --help`; `go run ./cmd/revolvr doctor --help`; a built CLI `config check` in
  a clean temporary repository; and `git diff --check`.
- Verification result: all required focused, race, CLI, and full repository
  checks passed. The initial full run exposed a stale ready-doctor fixture with
  a noncanonical fake version; the single repair made it derive the exact
  release version, and the final full run passed. The first manual config-check
  fixture inherited unsafe Git directory permissions; repeating it with a
  restrictive fixture umask passed and showed matching Codex/Git identities
  and effective schema v7. No dependency or commit was added.
- What remains: EXT-05 and the later ordered external-readiness gates.
  Blockers for EXT-04: none. External use remains unapproved.

## EXT-03 Initial External Scope and Attended Bounds (2026-07-15)

- Task selected: `EXT-03`, initial repository, platform, verification, and
  attended operational-bound enforcement at shared preflight/no-model
  admission.
- The shared external-scope check resolves the configured Git executable,
  requires the requested root to be an operator-controlled non-bare Git
  worktree, rejects active submodules, requires a clean worktree and at least
  one verification command, and admits attended-task only on Linux, macOS, or
  FreeBSD while queue/daemon remain Linux-only. Public attended, queue, and
  daemon entry points run it before the execution lock, workspace, ledger,
  task, model, or verification effects.
- Level-1 effective configuration now carries finite task/action, elapsed,
  token, cycle, process, output, retained-disk, and notification-attempt
  bounds. The documented defaults are fingerprinted, rendered by config check
  and doctor, applied to existing attempt/cycle and subprocess authorities,
  and copied into durable task-operation and ledger evidence. Caller cycle
  overrides are recorded exactly and unlimited attended cycles are refused;
  retained-disk evidence derives from the effective retention operation cap.
- Files changed for EXT-03: `.agent/AUTONOMOUS_EXTERNAL_READINESS.md`,
  `internal/app/external_admission.go`, `internal/app/autonomous_run.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/config_test.go`, `internal/app/app_test.go`,
  `internal/app/autonomous_scheduler_test.go`,
  `internal/app/external_preflight_test.go`,
  `internal/autonomoustaskrun/contracts.go`,
  `internal/autonomoustaskrun/ledger.go`,
  `internal/autonomoustaskrun/run.go`, `internal/runonce/effectiveconfig.go`,
  `internal/runonce/runonce.go`, `internal/cli/config.go`,
  `internal/cli/doctor_test.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`,
  this state file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on changed Go files; the required focused
  ordinary and race runs for
  `TestExternalRepositoryShapeAndPlatformMatrix|TestAttendedEffectiveBoundsVisibleAndRecorded`;
  focused affected-package tests; `GOOS=darwin go test -c ./internal/app`;
  `GOOS=freebsd go test -c ./internal/app`; `go test -count=1 ./...`; a built
  CLI `config check` in a clean temporary repository; `go run ./cmd/revolvr
  doctor --help`; and `git diff --check`.
- Verification result: all required focused, race, cross-compile, CLI, and full
  repository checks passed. Regressions prove the platform matrix, safe
  non-bare admission, bare/active-submodule/missing-verification/dirty/unresolved-
  Git refusal without model calls or runtime/task mutation, visible finite
  defaults, fingerprint inclusion, and exact durable operation evidence. A
  manual check also found and repaired omitted-working-directory resolution in
  config check. No dependency or commit was added.
- What remains: EXT-04 and the later ordered external-readiness gates.
  Blockers for EXT-03: none. External use remains unapproved.

## EXT-02 Mode-Aware Read-Only Doctor (2026-07-15)

- Task selected: `EXT-02`, the settled mode-aware, read-only doctor command
  surface.
- Bare `doctor` and `doctor --for attended-task` normalize to identical
  preflight input and output. `--for` accepts only `attended-task`, `queue`,
  and `daemon`; `--task <id>` is an exact selector admitted only for attended
  mode. Invalid modes, empty explicit flags, malformed selectors, and
  mode/selector conflicts return before repository inspection, external
  commands, or writes.
- Preflight now reports its normalized mode/task authority and runs the same
  strict autonomous graph loader used by direct task and queue execution. The
  loader validates every canonical task, required autonomous state, child
  publication lineage, archive authority, and graph diagnostic. An exact
  attended selector must identify an autonomous task currently classified
  `ready`. Existing execution paths reload this authority; preflight is not a
  lease.
- Files changed: `internal/app/preflight.go`,
  `internal/app/autonomous_run.go`, `internal/app/app_test.go`,
  `internal/app/external_preflight_test.go`, `internal/cli/doctor.go`,
  `internal/cli/doctor_test.go`, `.agent/TASKS.md`, this state file, and
  `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; focused ordinary
  and race commands for
  `TestModeAwarePreflight|TestDoctorForModesAndTaskSelector`; complete app and
  CLI package tests; all existing preflight/doctor tests; `go run
  ./cmd/revolvr doctor --help`; `go test -count=1 ./...`; and `git diff
  --check`.
- Verification result: focused ordinary/race tests, affected package tests,
  CLI help, and the complete repository suite passed. Regressions prove all
  modes, bare/explicit byte equivalence, exact-task readiness, pre-command
  invalid-request refusal, unsafe protected-state and invalid-graph refusal,
  repository immutability, and execution recheck after authority drift. No
  dependency or commit was added.
- What remains: EXT-03 and the later ordered external-readiness gates.
  Blockers for EXT-02: none. External use remains unapproved.

## EXT-01 Shared Repository-Path Admission (2026-07-15)

- Task selected: `EXT-01`, shared repository-path admission agreement across
  doctor, status, canonical task loading, and no-model autonomous admission.
- `internal/repositorypath` now binds one canonical repository-root identity,
  validates present `.agent`, `.agent/tasks`, canonical Markdown task files,
  `.revolvr`, optional config, and ledger paths without creating anything, and
  retains protected descriptor reads/enumeration for consumers. Missing paths
  remain nonmutating presence facts; status initialization keeps its existing
  runtime-directory-plus-ledger meaning.
- Doctor and status run that inspection first. Unsafe doctor input yields one
  failed state check and no command; status refuses the same authority.
  Configuration and task bytes are read through the inspected root identity.
  Exact-task and queue entry points inspect before acquiring the global
  autonomous-execution lock, so refused no-model probes create no locks or
  runtime evidence.
- Files changed: `internal/repositorypath/repositorypath.go`,
  `internal/taskfile/taskfile.go`, `internal/app/app.go`,
  `internal/app/preflight.go`, `internal/app/config.go`,
  `internal/app/autonomous_run.go`,
  `internal/app/external_preflight_test.go`,
  `internal/cli/doctor_test.go`,
  `internal/autonomousmigration/plan_test.go`, `.agent/TASKS.md`, this state
  file, and `.agent/DECISIONS.md`.
- Verification commands: `gofmt -w` on every changed Go file; focused ordinary
  and race commands for
  `TestExternalPreflightSharedPathMatrix|TestDoctorStatusAdmissionAgreeOnUnsafeAgent`;
  affected repositorypath/taskfile/app/CLI package tests; autonomous-migration
  package tests; `go test -count=1 ./...`; and `git diff --check`.
- Verification result: the focused ordinary and race regressions passed, as
  did the complete suite after one compatibility repair updated the migration
  symlink fixture to expect the new earlier shared refusal. The matrix proves
  safe/missing/wrong-type/final-symlink/ancestor-symlink/hard-link/group-write/
  identity-substitution behavior for both roots and exact repository/outside
  snapshot preservation on every refusal. No dependency or commit was added.
- What remains: EXT-02 and the later ordered external-readiness gates.
  Blockers for EXT-01: none. External use remains unapproved.

## External Readiness Backlog Decomposition (2026-07-15)

- EXT-01 through EXT-21 cover common and attended Level-1 authority:
  shared/mode-aware preflight, repository and executable scope, strict
  production fake-Codex composition, workspace/Git containment, explicit
  recovery, interruption proof, mandatory CI, candidate/runbook/evidence
  preparation, quantitative live dogfood, and an immutable release decision.
- EXT-22 through EXT-36 cover Level 2 only after Level-1 approval:
  explicit finite bounds and enforcement, deterministic stop policy, durable
  quarantine and unrelated-work continuation, production sequential queue
  composition, administrative recovery, exact unattended acknowledgement,
  rootless OCI isolation, retention/notification operations, the Level-2
  runbook and soak, and a separate tagged decision.
- EXT-37 through EXT-41 cover Level 3 only after Level-2 approval:
  self-wake exclusion, daemon interruption/restart recovery, the daemon
  runbook, the 72-hour quantitative soak, and a separate tagged decision.
- Each task names exact pass/fail behavior and either focused ordinary/race/full
  Go commands, concrete smoke/adversarial scripts, or immutable CI/build/
  dogfood/tag evidence. Quantitative thresholds and zero-tolerance soak
  failures are copied from the settled readiness policy rather than reopened.
- No production code, dependency, readiness policy, or decision was changed in
  this decomposition pass. Files changed by the pass are .agent/TASKS.md and
  this state summary. Backlog-decomposition blockers: none; external-use
  approval blockers remain those in
  .agent/AUTONOMOUS_EXTERNAL_READINESS.md.

## Final R4 Audit Closure (2026-07-14)

- AP-01: `runtimepath.Boundary` provides descriptor-rooted no-follow
  traversal plus identity-checked create, open, enumerate, link, replace,
  unlink, read, and sync operations. Autonomous state, notification,
  exact-task run, archive, and finalization owners retain stable parents and
  opened files across publication, cleanup, lease checks, and readback. The
  six named evidence readers and the additionally discovered migration reader
  use the same protected descriptor reads; the audited bespoke walkers and
  check-then-reopen reads are absent.
- Fresh AP-01 execution covered the original autonomous-state outside-mutation
  reproducer ten times, the shared boundary ten times, every other durable
  owner five times, and all seven evidence readers ten times. Final/ancestor
  symlinks, hard links, unsafe modes, renamed ancestors, metadata boundaries,
  cleanup, enumeration, held-lease replacement, and complete outside-tree
  contents, modes, targets, and link counts remain permanent assertions.
- AP-02: post-`SIGKILL` settlement has its own deadline, preserves signal and
  inspection errors, returns `ErrProcessTreeUnsettled` when absence cannot be
  proven, and checks leader identity reuse before every post-reap signal or
  poll. Ten focused runs included the real held-zombie descendant boundary.
- AP-03: active `q` and `ctrl+c` request cancellation but only the matching
  run token's fully applied terminal message emits `tea.Quit`. Twenty focused
  runs covered run-once, loop, exact-task, and queue cleanup and refresh.
- AP-04: explicit usage schemas have declared parent/key precedence; legacy
  recursive usage is accepted only when unique and otherwise returns typed,
  sorted ambiguity evidence without changing receipt bytes. Twenty focused
  runs covered every supported shape, precedence boundary, and 1,000 parses
  of the multi-candidate event per invocation.
- AP-05: regular and symlink source evidence is read through opened identities
  with immediate and post-read descriptor/path checks. Twenty focused runs
  covered final-symlink, inode, mutation, regular ABA, symlink ABA, and
  post-read substitution cases with replacement metadata equalized.
- AP-06: prior action budgets are sorted by action and archive byte checks use
  sorted expected paths. Twenty focused runs asserted exact multi-invalid
  first errors and archive object-read order across 1,000 calls per test.
- Final verification passed: complete ordinary, shuffled, and race suites;
  `go vet ./...`; `go mod verify`; `govulncheck ./...` with zero reachable
  vulnerabilities; tracked-Go formatting; `git diff --check`; Bash syntax for
  all tracked shell scripts; and CLI help. A fresh `umask 0022` Git fixture
  passed `init`, `config check`, and `status`; the existing working copy's
  group-writable `.agent` directory was correctly refused and left unchanged.
- Cross-builds passed for Linux 386/amd64/arm64, Darwin amd64/arm64, and
  FreeBSD amd64/arm64. Darwin and FreeBSD `gitstate` test binaries compile,
  and the Windows diagnostic stub contains the required unsupported-platform
  message. No dependency was added, the audit document is deleted, the
  backlog is empty, and blockers are none.

## Deterministic Remaining First-Error Diagnostics (2026-07-14)

- Attempt-transition validation copies and sorts prior action budgets by
  action before checking authority, consumption, and disappearance. This
  matches the action-budget canonicalization used by the attempt controller
  and removes map iteration from diagnostic selection.
- Archive commit verification already sorts the exact expected path set for
  commit comparison. Expected file bytes are now checked in that same order,
  while paths without an expected byte payload remain path-only evidence.
- Multi-invalid regressions supply action budgets in reverse order and assert
  the exact first error for two missing and two changed budgets. Archive
  regressions supply reverse paths with two missing or two byte-mismatched
  files and assert both the exact error and the first Git object read. Every
  case executes 1,000 times per test invocation.
- Verification passed: twenty focused repetitions, ten complete repetitions
  of both owner packages, owner race tests, complete ordinary/shuffled/race
  suites, `go vet ./...`, `go mod verify`, CLI help, formatting and diff
  checks, Linux/Darwin/FreeBSD amd64 builds, and the unsupported-Windows
  diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-CLOSE-01`; blockers: none.

## Descriptor-Bound Source Snapshot Entries (2026-07-14)

- Regular entries are opened with no final-component symlink following and
  nonblocking semantics. Immediate descriptor metadata must match the initial
  pathname identity, type, mode, size, and modification time before hashing.
- After hashing, both the opened descriptor and the pathname are checked
  against the opened identity before the digest is published. This prevents a
  pathname from supplying metadata for bytes read from a substituted inode.
- Symlinks are opened as link-inode descriptors and their targets are read
  through those descriptors: `O_PATH` plus descriptor-relative `readlinkat`
  on Linux and FreeBSD, and `O_SYMLINK` plus `freadlink` on macOS. Descriptor
  and pathname identity checks bracket the target read.
- Deterministic capture hooks reproduce final-symlink replacement, regular
  inode replacement, regular and symlink A-to-B-to-A substitutions, a
  regular-file mutation after hashing, and symlink replacement around and
  after the descriptor target read. Replacement metadata is deliberately
  matched so identity checks, not incidental size or timestamp differences,
  cause rejection.
- Verification passed: twenty focused adversarial repetitions, complete
  ordinary/shuffled/race suites, `go vet ./...`, `go mod verify`, CLI help,
  formatting and diff checks, Linux/Darwin/FreeBSD amd64 builds, Darwin and
  FreeBSD `gitstate` test-package builds, and the unsupported-Windows
  diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-11`; blockers: none.

## Codex Usage Schema Precedence (2026-07-14)

- Direct `usage`, `total_usage`, `token_usage`, and `total_token_usage`
  objects have highest authority in that order. The same key order under
  `response`, `result`, `message`, and `event` envelopes follows in that
  parent order, then bare event metric fields.
- The legacy arbitrary-depth shape is compatibility-only. The parser
  enumerates every metric-bearing named usage object, sorts RFC 6901-style
  candidate paths for diagnostics, and accepts it only when exactly one
  candidate remains. Traversal order never chooses among candidates.
- Competing legacy candidates return `CodexUsageAmbiguityError` with the
  record number and stable candidate paths and unwrap to
  `ErrCodexUsageAmbiguity`. File parsing classifies it as a parse diagnostic,
  not a source failure; Codex results publish no usage and receipt rewriting
  returns the original bytes unchanged.
- Regressions cover all 20 named usage-object paths, the bare-metrics and
  unique-legacy shapes, every schema-precedence boundary, receipt
  preservation, file classification, and 1,000 fresh parses of the reproduced
  multi-candidate event per test invocation.
- Verification passed: twenty focused repetitions, ten complete receipt-
  package repetitions, the receipt race suite, complete ordinary/shuffled/
  race suites, `go vet ./...`, `go mod verify`, CLI help, tracked-Go formatting
  and diff checks, Linux/Darwin/FreeBSD amd64 builds, and the unsupported-
  Windows diagnostic-stub build. No dependency was added. The next task is
  `AUDIT-R4-10`; blockers: none.

## Protected Missing-Receipt Assertion Repair (2026-07-14)

- `runtimepath.Boundary.ReadFileLimit` deliberately reports absent required
  files with `os.ErrNotExist`. The taskschedule regression now asserts that
  contract through `os.ErrNotExist.Error()` rather than the obsolete
  `no such file` literal produced by the former pathname read.
- Fifty focused repetitions, ten complete taskschedule-package repetitions,
  and `go test -count=1 ./...` passed. No production behavior or dependency
  changed. The next task is `AUDIT-R4-09`; blockers: none.

## TUI Quit-After-Settlement (2026-07-14)

- Active `q`/`ctrl+c` sets `QuitAfterSettlement` on the current token-bound run
  state, requests cancellation once, and returns no `tea.Quit`. The existing
  `c` key remains cancel-without-quit.
- Progress and terminal commands continue draining the operation's buffered
  message stream while quit is pending. Cancellation stops producers from
  blocking on progress sends, while the terminal send remains consumable by
  the outstanding start/wait command.
- Only a terminal message with the current operation token can release the
  delayed quit. The model first applies the mode-specific terminal result,
  refreshed status, run detail, logs, and inactive state, then clears the quit
  marker and emits `tea.Quit`. Stale terminal messages do neither.
- Autonomous task and queue completion normally reload workflow selectors;
  delayed quit intentionally takes precedence after terminal application,
  because the completion goroutine has already performed status refresh and
  durable domain cleanup.
- One table-driven regression covers run-once, bounded loop, exact task run,
  and autonomous queue under both `q` and `ctrl+c`. Each action blocks after
  observing cancellation, proves no quit or terminal before cleanup release,
  rejects a stale terminal, then proves cleanup and refresh precede the exact
  terminal and `tea.Quit`.
- Verification passed: twenty focused repetitions, ten complete TUI-package
  repetitions, focused and package race tests, complete ordinary/shuffled/race
  suites, `go vet ./...`, `go mod verify`, CLI help, formatting and diff checks,
  Linux/Darwin/FreeBSD amd64 builds, and the unsupported-Windows diagnostic
  stub. No dependency was added. The next task is `AUDIT-R4-09`; blockers:
  none.

## Bounded Post-Kill Process-Tree Settlement (2026-07-14)

- `runner.Command` has a distinct `KillSettlementPeriod`, defaulting to five
  seconds independently of the graceful TERM period. After the grace timer
  sends `SIGKILL`, the runner continues bounded liveness polling and cannot
  return until the original process group is proven gone.
- Failure to prove post-kill settlement returns `ErrProcessTreeUnsettled`.
  Graceful-signal, force-signal, and inspection errors remain joined with the
  cancellation/deadline cause instead of being discarded or replaced.
- One guarded lifecycle owns both signals and polls. Before every operation
  after `cmd.Wait` has reaped the leader, it checks whether that PID/process-
  group identity was reused. Natural-exit settlement and cancellation races
  share the same guard and fail closed without touching the replacement.
- A deterministic lower-level regression holds the post-kill liveness result
  until explicitly released. A Linux end-to-end regression adds a TERM-
  ignoring child to the command's process group, holds its killed zombie
  unreaped, proves `Run` has not returned, then reaps it and proves every tree
  member is non-executable and no sentinel exists beginning at return.
- Regressions also prove the separate kill deadline, typed unsettled failure,
  preservation of every error class, and refusal of natural-exit and
  cancellation identity-reuse races before platform signal/poll calls.
- Verification passed: ten complete runner-package repetitions, focused
  adversarial tests, runner race tests, complete ordinary/shuffled/race suites,
  `go vet ./...`, `go mod verify`, formatting and diff checks, Linux/Darwin/
  FreeBSD amd64 builds, and the unsupported-Windows diagnostic-stub build. No
  dependency was added. The next task is `AUDIT-R4-08`; blockers: none.

## Stable Remaining Evidence Readers (2026-07-14)

- `runtimepath` now provides capped reads through opened file descriptors.
  The limit is checked before allocation, enforced while reading, and followed
  by the existing named/opened inode recheck. A typed limit error lets owners
  preserve their established diagnostics.
- Audit apply, plan apply, operator checkpoint receipts, ledger export
  verification/replay, Codex last-message handling, and autonomous task views
  retain one repository or parent boundary across their authoritative reads.
  Exact hash/size validation and existing read caps remain in force.
- The inventory also found autonomous migration's exact orphan-state reader.
  Namespace enumeration and state comparison now share one opened directory
  and reject substituted ancestors, unsafe modes, aliases, and wrong types.
- Codex last-message handling retains its opened parent through raw read,
  redacted temporary write/sync, descriptor-relative replacement, raw cleanup,
  canonical mode readback, and directory sync. Cleanup can safely remove an
  owned unaliased inode after an external writer changes only its mode, while
  hard-linked or substituted files remain fail-closed.
- Permanent owner regressions bind first, replace an evidence ancestor with an
  outside symlink, and prove outside bytes and entries remain unchanged. The
  Codex regression exercises the complete `Run` path and failure cleanup.
- Verification passed: ten focused adversarial repetitions; all affected
  package tests and race tests; complete ordinary, shuffled, and race suites;
  `go vet ./...`; `go mod verify`; formatting and diff checks; Linux, Darwin,
  and FreeBSD amd64 builds; and the unsupported-Windows diagnostic-stub build.
  No dependency was added. The next task is `AUDIT-R4-07`; blockers: none.

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
- `.agent/HANDOFF.md`: active pause/resume point, exact next command, and
  compact release authority needed to continue safely.
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

- Read `AGENTS.md` and `.agent/HANDOFF.md` first, then `.agent/TASKS.md`,
  `.agent/STATE.md`, and `.agent/DECISIONS.md` before acting.
- Resume with the exact command recorded in `.agent/HANDOFF.md`; at this
  boundary it is `./agent-ext20-rc3-attestation.sh`.
- Do one bounded task, verify it, update durable state, and stop.
- Do not use `codex resume` or depend on an old session transcript.
