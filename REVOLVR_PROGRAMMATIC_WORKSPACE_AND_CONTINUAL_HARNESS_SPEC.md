# Revolvr Programmatic Workspace and Safe Continual Harness Specification

**Status:** Proposed post-core architecture extension
**Date:** 2026-08-06
**Applies to:** `ponchione/revolvr` after completion of the canonical architecture sequence
**Primary inspiration:** Prime Agent's programmatic context management, persistent computational workspace, append-only trajectory, verification-gate deduplication, and continual-harness refinement model
**Authority:** This document is subordinate to the accepted Revolvr ADRs and `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`. Where this document conflicts with an accepted ADR, the ADR controls until deliberately amended.

---

## 1. Executive Decision

Revolvr shall adopt a **bounded subset** of the Prime Agent design rather than copying Prime Agent wholesale.

The adopted direction is:

1. Add a **task-scoped programmatic Python workspace** inside the existing disposable rootless sandbox.
2. Let implementer and corrector agents use that workspace to inspect, filter, transform, and retain large task-local context without repeatedly placing all raw material into the model context window.
3. Preserve the complete run trajectory as append-only, queryable evidence.
4. Add explicit task-local durable scratch entries rather than relying on opaque Python-process snapshots.
5. Add reusable, versioned Python skills behind capability manifests and tests.
6. Add a safe continual-harness workflow in which agents may **propose** prompt notes, skills, retrieval rules, or project memories, but may never activate those changes themselves.
7. Deduplicate unchanged verification gates using an exact execution fingerprint.
8. Evaluate the programmatic worker against the existing direct-tool worker before enabling it by default.

Revolvr shall **not** adopt:

- parallel sub-agent fan-out;
- background agent swarms;
- unrestricted agent-to-agent messaging;
- persistent cross-task model conversations;
- host-level Python execution;
- an agent-editable base system prompt;
- automatic activation of self-authored skills or memory;
- autonomous capability escalation;
- an always-running heartbeat loop that can mutate projects without a newly admitted operation;
- IPython as the only tool surface;
- Graphiti as part of this feature.

The Revolvr host remains authoritative for task state, policy, lifecycle, source identity, verification, evidence, and completion. The Python workspace is an untrusted computational tool inside the sandbox.

---

## 2. Integration Timing

### 2.1 Do not interrupt the current core rewrite

The current architecture sequence should continue in order. Architecture tasks 001-014 have established PostgreSQL, artifacts/events, project mirrors, task lifecycle, scheduler leases, sandbox validation/runtime, managed workspaces, the OpenAI structured-output boundary, and the decision-only supervisor. Task 015 begins planning; tasks 016-024 build the core execution loop, retrieval, evaluation, queue, and UI.

The full programmatic-workspace implementation shall begin **after** the core sequence and initial core-loop dogfooding.

### 2.2 Amend several pending task specifications now

Small contract-level amendments should be made before the affected current tasks execute. These amendments prevent avoidable redesign without adding the Python runtime yet.

#### Architecture 016 — Tool broker and implementer

Add the following requirements:

- Tool-execution records must support a generic `runtime_kind`, not assume every action is a direct host tool.
- Every tool result must be representable by exact inline bounded data or one or more content-addressed artifact references.
- Tool records must retain ordered trajectory sequence, request hash, result hash, workspace/source identity, timeout, resource policy, and cancellation outcome.
- The broker must expose a narrow internal extension interface capable of adding a future `python_exec` tool without changing canonical lifecycle or worker contracts.
- No Python runtime, REPL, persistent kernel, or self-modifying skill system is implemented by task 016.

#### Architecture 017 — Verification engine

Add exact gate deduplication:

- Compute an `execution_fingerprint` from the candidate source tree, verification-plan version/hash, ordered command argv, working directories, declared environment names and nonsecret values, image digest, sandbox profile hash, project environment contract, verifier implementation version, and verification-authority file hashes.
- When the exact fingerprint already has a terminal result, do not execute the gate again unless policy explicitly requests a fresh occurrence.
- Reusing a prior failure produces an explicit `unchanged_failure_reused` occurrence; it does not convert the failure into a pass or erase the original result.
- Any change to source, plan, command, environment, image, profile, verifier version, fixtures, goldens, scripts, or authority hashes invalidates reuse.
- Completion policy may require a fresh final occurrence even when an identical earlier result exists; reuse and freshness are distinct concepts.

#### Architecture 018 — Evidence and completion

Add:

- A completion capsule references the exact run trajectory manifest hash.
- A completion capsule records all active harness-asset versions that influenced supervisor, planner, implementer, verifier, auditor, or corrector behavior.
- Missing, changed, or unresolved trajectory/harness provenance blocks completion when those inputs were used by the run.

#### Architecture 021 — Context assembly

Add:

- Context-package items may be direct content or immutable references to queryable run artifacts and trajectory ranges.
- The context manifest records which large sources were intentionally left external and how a worker may retrieve them.
- Exact files, symbols, accepted task state, and canonical evidence continue to outrank model-generated scratch or refinement content.
- The context subsystem must expose a read-only internal query interface that a future sandboxed programmatic workspace can call through the host broker.

#### Architecture 022 — Deterministic evaluation

Add:

- Worker execution mode is an explicit evaluated dimension, initially `direct_tools_v1` and later `programmatic_workspace_v1`.
- Baseline fixture results record execution mode, context bytes, model input/output/reasoning tokens where available, tool count, repeated-read count, verification occurrences, correction cycles, wall time, and final outcome.
- The suite must be capable of running the same scenario against both worker modes without changing task or acceptance authority.

#### Architecture 025 — Graphiti gate

No change is required. Programmatic workspace and continual-harness evaluation are separate from the Graphiti decision gate. Neither is evidence that Graphiti is needed.

---

## 3. Goals

The feature shall improve long-horizon task performance by giving a worker a programmable way to manage large context while preserving Revolvr's existing safety and authority model.

Specific goals:

- Reduce repeated reading and repeated model ingestion of large files, logs, diffs, and histories.
- Allow deterministic local computation over task data before model reasoning.
- Preserve useful task-local derived state across implement/correct cycles.
- Make full trajectory history accessible without injecting it wholesale into prompts.
- Convert reusable lessons into explicit, reviewable proposals.
- Preserve exact provenance and rollback for every activated harness asset.
- Keep the feature removable and optional.
- Keep the system strictly sequential.

---

## 4. Non-Goals

This feature does not:

- train or fine-tune a model;
- create a multi-agent swarm;
- add parallel workers;
- grant workers PostgreSQL access;
- grant workers access to the host filesystem, Docker socket, local credentials, or original checkout;
- replace deterministic verification or independent audit;
- permit the model to mark goals or tasks complete;
- make Python state canonical;
- preserve arbitrary Python objects across project tasks;
- automatically install Python packages;
- replace PostgreSQL FTS, pgvector, structural indexing, direct reads, or ordinary tools;
- implement Graphiti, Neo4j, FalkorDB, or another graph service;
- make refinement proposals authoritative merely because they were generated from a successful run.

---

## 5. System Context

```text
Trusted host process

  PostgreSQL canonical state
  Task/lifecycle/policy engine
  Artifact and trajectory stores
  OpenAI model client
  Context/retrieval service
  Verification and completion engine
  Sandbox broker
           |
           | framed, bounded requests
           v
Disposable rootless worker container

  Implementer/corrector model loop
  Direct brokered tools
  Programmatic Python workspace
  Task-local scratch catalog client
  No canonical-state authority
  No host/database/runtime credentials
           |
           v
Managed Git workspace mounted at /workspace
```

The host owns all durable authority. The container owns only untrusted task execution. The programmatic workspace is a subordinate tool inside that container.

---

## 6. Core Concepts

### 6.1 Programmatic workspace

A long-lived Python process scoped to one admitted Revolvr run and managed workspace. It allows multiple `python_exec` calls to share transient Python variables during the active worker lifecycle.

### 6.2 Durable scratch

Explicit, named, content-addressed task-local entries created through a controlled API. Durable scratch survives worker/container restart and can be reloaded into a fresh Python process.

Durable scratch is not arbitrary process serialization and shall not use Python pickle as canonical state.

### 6.3 Trajectory

The complete ordered record of model requests/responses, context manifests, direct tool calls, Python executions, host decisions, denials, artifacts, verification, audit, and lifecycle events for one run.

### 6.4 Harness asset

A versioned prompt note, reusable Python skill, retrieval rule, project memory note, or verification hint that can influence future runs.

### 6.5 Refinement proposal

An evidence-backed request to create or change a harness asset. A proposal is advisory until validated, evaluated, and explicitly approved.

### 6.6 Execution fingerprint

A hash of every material input to one deterministic external gate. Equal fingerprints permit exact result reuse under policy; unequal fingerprints require execution.

---

## 7. Trust and Authority Model

### 7.1 Trusted host authority

Only trusted Go host code may:

- select a task;
- acquire or release scheduler leases;
- create managed workspaces;
- admit sandbox execution;
- construct canonical role dossiers;
- decide legal lifecycle transitions;
- persist canonical task/plan/criterion/finding state;
- accept verification or audit evidence;
- activate harness assets;
- mark a run or task complete;
- publish candidate commits;
- read or write PostgreSQL credentials;
- control Docker/rootless runtime commands.

### 7.2 Untrusted worker authority

A worker may:

- read admitted workspace files;
- edit admitted workspace files when its role permits;
- execute admitted direct-argv commands;
- invoke `python_exec` when its role permits;
- create task-local scratch entries;
- make structured claims;
- propose a refinement;
- request additional context;
- stop, fail, or request operator input.

A worker may not:

- access PostgreSQL directly;
- mutate canonical tasks, plans, criteria, findings, or lifecycle;
- activate skills or memories;
- broaden mounts, network, environment, or resource policy;
- install dependencies outside the task's admitted source change and sandbox policy;
- start another worker or model session;
- create background work that outlives its admitted worker operation;
- declare its own work verified, audited, or complete.

---

## 8. Programmatic Workspace Runtime

### 8.1 Runtime form

Implement a small Python service named conceptually `revolvr-repld` inside the pinned worker image.

The initial implementation should use a custom framed JSON protocol over container-local standard I/O or a private Unix socket. It should not introduce Jupyter Server, ZeroMQ networking, or a separately exposed service.

The runtime may use IPython's execution machinery internally, but Revolvr shall own the external protocol and lifecycle.

### 8.2 Lifecycle

```text
Host admits worker operation
  -> sandbox container created
  -> managed workspace mounted
  -> repld started with run/workspace identity
  -> implementer or corrector invokes python_exec zero or more times
  -> final worker output produced
  -> repld stopped
  -> container removed
```

A later implement/correct cycle may start a fresh container and fresh Python process while reusing explicit durable scratch entries. Transient globals are not assumed to survive.

### 8.3 Identity

Every programmatic workspace binds:

- project ID;
- task ID and task-version hash;
- run ID;
- worker occurrence ID;
- role;
- plan/version/step identity;
- managed workspace ID;
- source commit/tree;
- container image digest;
- sandbox profile hash;
- runtime implementation version;
- Python version;
- enabled skill-set manifest hash;
- context-package hash.

Any mismatch causes the runtime or broker to refuse execution.

### 8.4 `python_exec` tool contract

Suggested input:

```json
{
  "code": "...python source...",
  "expected_runtime_generation": 3,
  "timeout_ms": 30000,
  "result_mode": "summary",
  "max_inline_bytes": 16384
}
```

Suggested result:

```json
{
  "execution_id": "uuid",
  "runtime_generation_before": 3,
  "runtime_generation_after": 4,
  "code_sha256": "...",
  "status": "completed",
  "inline_result": "bounded text or null",
  "stdout_artifact_id": "uuid or null",
  "stderr_artifact_id": "uuid or null",
  "result_artifact_id": "uuid or null",
  "created_scratch_entry_ids": ["uuid"],
  "duration_ms": 412,
  "resource_evidence": {},
  "truncated": false
}
```

Terminal statuses include:

- `completed`;
- `python_error`;
- `timeout`;
- `cancelled`;
- `output_limit`;
- `runtime_lost`;
- `stale_identity`;
- `policy_denied`;
- `protocol_error`.

### 8.5 Output handling

- Inline output is small and bounded.
- Large stdout, stderr, values, tables, or generated files become immutable artifacts.
- Binary values are never inserted directly into model context.
- Model-visible summaries must state that full bytes are available by artifact reference.
- Every truncation is explicit.

### 8.6 Runtime state

The Python process may retain transient globals between calls within one active worker occurrence.

The following are not durable authority:

- Python globals;
- imported module instances;
- open file handles;
- interpreter history;
- in-memory indexes;
- subprocess handles.

Durable state requires an explicit scratch write.

### 8.7 No arbitrary process snapshotting

Do not persist a full Python process image or arbitrary pickle graph. Recovery uses:

1. a fresh pinned runtime;
2. the exact context-package reference;
3. explicit durable scratch entries;
4. trajectory queries;
5. versioned skills.

This avoids unsafe deserialization, hidden authority, environment coupling, and unreproducible process state.

---

## 9. Python-Side Revolvr API

The runtime preloads a small `revolvr` Python package as `rv`.

### 9.1 `rv.context`

Read-only access to host-brokered context and retrieval:

```python
rv.context.manifest()
rv.context.get_item(item_id)
rv.context.read_file(path, start_line=None, end_line=None)
rv.context.search_text(query, paths=None, limit=20)
rv.context.search_code(query, limit=20)
rv.context.symbol(name)
rv.context.related_symbols(symbol_id, depth=1)
rv.context.list_artifact_refs(kind=None)
```

All methods enforce the admitted project/workspace and host retrieval policy.

### 9.2 `rv.trajectory`

Read-only access to the current run trajectory:

```python
rv.trajectory.list(kinds=None, after_sequence=0, limit=100)
rv.trajectory.get(sequence)
rv.trajectory.search(query, kinds=None, limit=20)
rv.trajectory.tool_results(tool_name=None, limit=50)
rv.trajectory.failures(limit=20)
rv.trajectory.context_manifests(limit=10)
```

The API returns metadata and bounded previews. Full values are artifact references.

### 9.3 `rv.scratch`

Explicit durable task-local state:

```python
rv.scratch.put_text(name, text, metadata=None)
rv.scratch.put_json(name, value, metadata=None)
rv.scratch.put_bytes(name, value, media_type, metadata=None)
rv.scratch.put_table(name, rows, schema=None, metadata=None)
rv.scratch.get(name_or_id)
rv.scratch.list(prefix=None, kind=None)
rv.scratch.delete(name_or_id)
```

Delete creates a tombstone; it does not erase prior evidence.

### 9.4 `rv.artifacts`

```python
rv.artifacts.metadata(artifact_id)
rv.artifacts.read_text(artifact_id, offset=0, limit=65536)
rv.artifacts.read_bytes(artifact_id, offset=0, limit=65536)
rv.artifacts.create_text(kind, text, metadata=None)
rv.artifacts.create_bytes(kind, value, media_type, metadata=None)
```

Creation remains run-scoped and policy-bounded.

### 9.5 `rv.skills`

```python
rv.skills.list()
rv.skills.describe(name)
rv.skills.run(name, **kwargs)
```

Only host-activated, hash-pinned, role-permitted skills appear.

### 9.6 Prohibited APIs

The Python package shall expose no API to:

- mark criteria complete;
- transition task lifecycle;
- change plan authority;
- resolve findings;
- activate refinements;
- access secrets;
- request arbitrary mounts;
- change sandbox/network policy;
- call OpenAI directly;
- spawn another model or worker;
- access PostgreSQL;
- publish commits to the original project.

---

## 10. Trajectory Model

### 10.1 Append-only requirement

Every material action becomes an ordered append-only trajectory entry. Existing canonical event and artifact systems should be extended rather than replaced by a second event store.

Required entry kinds include:

- context package built;
- model request prepared;
- model response completed/refused/failed;
- direct tool requested/denied/completed;
- Python execution requested/denied/completed;
- scratch entry created/updated/tombstoned;
- worker summary received;
- host source capture completed;
- verification scheduled/executed/reused;
- audit/correction occurrence;
- refinement proposed/evaluated/approved/rejected/activated/rolled back;
- lifecycle and policy decisions.

### 10.2 Ordering

Each run has a host-assigned monotonically increasing sequence. Model or container clocks never define canonical ordering.

### 10.3 Trajectory manifest

At stable boundaries, generate a manifest containing:

- run and source identities;
- first and last sequence;
- entry count by kind;
- ordered entry IDs and hashes;
- referenced artifact IDs and hashes;
- context package hashes;
- model request/response identities;
- active harness-asset versions;
- manifest version and SHA-256.

Completion references the final manifest.

### 10.4 Query behavior

Trajectory querying is a read projection over canonical entries. Search indexes are rebuildable. Search results cannot override canonical entry bytes or ordering.

---

## 11. Durable Scratch Model

### 11.1 Scope

Scratch entries are scoped to one task and usually one run. Optional promotion can later turn a scratch lesson into a project harness asset through the refinement process.

### 11.2 Entry kinds

Initial kinds:

- `text`;
- `json`;
- `bytes`;
- `table`;
- `file_set`;
- `symbol_set`;
- `failure_analysis`;
- `strategy_note`;
- `context_index`.

### 11.3 Required metadata

Each entry records:

- ID and stable name;
- project/task/run/workspace identities;
- kind and media type;
- content artifact ID/hash/size;
- creator role and worker occurrence;
- source trajectory sequence;
- source context/artifact references;
- created/updated time from the host;
- version;
- supersedes/tombstone linkage;
- optional expiry policy;
- secret-scan result.

### 11.4 Authority

Scratch content is advisory. It never outranks:

- accepted task versions;
- accepted plans;
- direct project source;
- host-observed Git state;
- verification evidence;
- independent audit;
- accepted ADRs and operator decisions.

---

## 12. Versioned Skills

### 12.1 Skill definition

A skill is a versioned Python module plus a manifest describing:

- stable skill ID and semantic version;
- source proposal and evidence;
- package/module entry point;
- accepted inputs and outputs;
- permitted roles;
- required `rv.*` capabilities;
- whether it may execute subprocesses through approved APIs;
- maximum runtime and output;
- dependencies included in the pinned worker image;
- tests and expected fixtures;
- content and manifest hashes;
- activation scope: global, language, project, or task class.

### 12.2 Skill constraints

- Skills execute only in the sandbox.
- Skills cannot import host credentials or database clients.
- Skills cannot install packages at runtime.
- Skills cannot call arbitrary network endpoints.
- Skills cannot mutate canonical state.
- Skills may create scratch/artifacts only through `rv` APIs.
- A skill requiring new container capabilities is a separate operator-reviewed architecture change, not an ordinary refinement.

### 12.3 Activation

Only the host can activate a skill version. Every model request records the exact active skill-set manifest hash.

### 12.4 Rollback

Activation is append-only and reversible. Rolling back creates a new activation record pointing to the previous valid set; it does not edit history.

---

## 13. Safe Continual Harness

### 13.1 Allowed proposal kinds

An agent may propose exactly one bounded change of these kinds:

- `prompt_note` — supplemental role guidance, never the immutable base policy;
- `python_skill` — reusable sandboxed programmatic behavior;
- `retrieval_rule` — a bounded ranking/query heuristic;
- `project_memory` — a source-grounded project-specific lesson;
- `verification_hint` — advisory candidate for a project verification configuration change;
- `task_compiler_rule` — candidate lint/normalization rule for future task intake.

### 13.2 Forbidden proposal effects

A refinement may not:

- change the immutable base system prompt;
- weaken sandbox policy;
- add network access;
- add mounts or secrets;
- bypass verification/audit/completion;
- broaden worker roles;
- grant PostgreSQL or Docker control;
- activate itself;
- modify accepted task or architecture authority;
- hide, rewrite, or delete prior evidence;
- increase model spend or autonomous budgets without operator approval;
- add a Python dependency or container package silently.

### 13.3 Proposal triggers

A proposal may be created after:

- successful task completion with a clearly reusable tactic;
- repeated matching failure signatures;
- repeated unnecessary retrieval/tool patterns;
- explicit operator request;
- an evaluation finding.

No background timer automatically creates or applies proposals.

### 13.4 Proposal contents

Each proposal contains:

- proposal ID/version/kind/scope;
- concise problem statement;
- proposed asset content or patch;
- exact source trajectory entries, artifacts, tasks, failures, or successes;
- expected benefit;
- known risks;
- capability impact declaration;
- affected roles/projects/languages;
- deterministic validation plan;
- A/B evaluation plan where applicable;
- rollback plan;
- model/prompt/schema identities that created it.

### 13.5 Lifecycle

```text
proposed
  -> schema_validated
  -> policy_validated
  -> tests_passed
  -> evaluation_pending
  -> evaluated
  -> operator_approved | rejected
  -> active
  -> retired | rolled_back
```

Failures do not silently advance the lifecycle.

### 13.6 Operator approval

Initial versions require explicit local operator approval for every activation. A future ADR may allow automatic activation only for narrowly classified, deterministic, capability-neutral assets after sufficient evidence.

### 13.7 Evaluation

Refinement evaluation must compare exact baseline and candidate asset sets against fixed fixture identities. Results retain success, safety, quality, token, time, and retrieval evidence.

---

## 14. Verification Fingerprint and Gate Reuse

### 14.1 Fingerprint components

At minimum:

```text
hash(
  verifier_protocol_version,
  project_id,
  task_version_hash,
  verification_plan_hash,
  candidate_tree_hash,
  ordered_commands_and_cwds,
  declared_environment_hash,
  worker_image_digest,
  sandbox_profile_hash,
  project_environment_contract_hash,
  verification_authority_file_hashes,
  fixtures_and_goldens_hash,
  parser_versions,
  output_policy_hash
)
```

### 14.2 Reuse rules

- Equal fingerprint and terminal result allow reuse when policy permits.
- Reuse creates a new occurrence linked to the original execution.
- Reused pass remains a pass only for the exact fingerprint.
- Reused failure remains a failure.
- Cancelled, infrastructure-failed, incomplete, or ambiguous occurrences are not reusable unless the specific typed outcome is deliberately modeled as reusable.
- A final completion gate may require fresh execution regardless of reuse availability.

### 14.3 No false freshness

A reused result never receives a new execution timestamp. The new occurrence records reuse time and original execution time separately.

---

## 15. Context Management

### 15.1 Keep role-specific frozen dossiers

Prime-style programmatic context does not replace Revolvr dossiers. Supervisor, planner, implementer, auditor, and corrector continue to receive bounded frozen packages.

### 15.2 Externalize large material

Large logs, trajectories, code collections, and historical artifacts remain external. The dossier includes:

- bounded summaries;
- exact references;
- hashes;
- query instructions;
- explicit omissions.

### 15.3 Model-request continuation

Within a worker occurrence, later model calls receive:

- immutable task/plan/policy identity;
- current source identity;
- recent bounded model/tool turns;
- current active scratch catalog summary;
- previous context-package reference;
- exact trajectory range available externally.

The full old conversation is not required in every request.

### 15.4 Compaction

The host may compact recent conversational material when budget thresholds are reached. Compaction produces an artifact with:

- exact covered sequence range;
- source entry hashes;
- summarizer model/prompt/schema identities when model-assisted;
- omissions and unresolved references;
- summary hash.

The full trajectory remains available and authoritative.

### 15.5 Model-requested compaction

A worker may request compaction, but the host decides whether and how to perform it. The worker cannot delete history.

---

## 16. Role Permissions

### 16.1 Supervisor

- No Python workspace.
- No tools.
- One fresh structured decision request.

### 16.2 Planner

- No mutable Python workspace in the initial release.
- Uses host-built repository map, exact search results, and frozen dossier.
- A later read-only programmatic planner experiment requires a separate evaluation gate.

### 16.3 Implementer

- Direct brokered read/write/command tools.
- `python_exec` permitted.
- Durable scratch permitted.
- Active skills limited to implementer role.

### 16.4 Auditor

Initial release remains independent and receives frozen evidence/diff/verification. Read-only Python may be evaluated later, but the auditor must not share implementer transient state.

### 16.5 Corrector

- `python_exec` permitted.
- May read prior task scratch and trajectory.
- May create new scratch.
- Must bind every action to exact finding or verification-failure authority.

### 16.6 Verifier

No model-controlled Python workspace. Verification commands run through the host-owned verifier sandbox.

---

## 17. Security Requirements

The programmatic workspace runs only under the existing validated rootless sandbox contract.

Required controls:

- rootless container runtime evidence;
- no Docker/runtime socket;
- no original checkout mount;
- only the managed workspace mounted read-write;
- read-only container root filesystem;
- bounded tmpfs for `/tmp` and runtime state;
- no network by default;
- explicit PID, CPU, memory, file-size, process, and wall-clock limits;
- replacement environment, not ambient inheritance;
- no OpenAI, PostgreSQL, Git-hosting, SSH, cloud, or desktop credentials;
- no host home directory;
- pinned image digest;
- fixed Python and skill dependency versions;
- no runtime `pip install`, `uv add`, package-manager mutation, or remote code fetch;
- bounded protocol frames and output;
- cancellation kills descendant processes and removes the container;
- exact container/workspace labels and orphan reconciliation;
- secret scanning before artifacts become model-visible or durable;
- symlink/hard-link/path substitution checks at every host boundary.

Python's ability to execute arbitrary code is acceptable only because the surrounding container is disposable and untrusted. Do not attempt to treat Python-level import filtering as the primary security boundary.

---

## 18. PostgreSQL Data Model

Reuse current canonical tables where possible. Do not duplicate artifacts or general events.

### 18.1 `programmatic_workspaces`

Suggested fields:

```text
id uuid primary key
project_id uuid not null
task_id uuid not null
run_id uuid not null
worker_occurrence_id uuid not null
workspace_id uuid not null
role text not null
status text not null
runtime_version text not null
python_version text not null
image_digest text not null
sandbox_profile_hash text not null
context_package_hash text not null
skill_set_hash text not null
generation bigint not null
created_at timestamptz not null
last_used_at timestamptz not null
terminated_at timestamptz
terminal_reason text
unique(run_id, worker_occurrence_id)
```

### 18.2 `programmatic_executions`

```text
id uuid primary key
programmatic_workspace_id uuid not null
trajectory_sequence bigint not null
runtime_generation_before bigint not null
runtime_generation_after bigint
code_artifact_id uuid not null
code_sha256 text not null
status text not null
stdout_artifact_id uuid
stderr_artifact_id uuid
result_artifact_id uuid
request_hash text not null
result_hash text
duration_ms bigint
resource_json jsonb not null
created_at timestamptz not null
unique(programmatic_workspace_id, trajectory_sequence)
```

### 18.3 `scratch_entries`

```text
id uuid primary key
project_id uuid not null
task_id uuid not null
run_id uuid not null
workspace_id uuid not null
name text not null
kind text not null
version bigint not null
content_artifact_id uuid not null
content_hash text not null
metadata_json jsonb not null
created_by_role text not null
source_trajectory_sequence bigint not null
supersedes_id uuid
tombstoned boolean not null default false
created_at timestamptz not null
unique(run_id, name, version)
```

### 18.4 `harness_assets`

```text
id uuid primary key
stable_key text not null
kind text not null
scope_kind text not null
scope_id text
status text not null
created_at timestamptz not null
unique(kind, scope_kind, scope_id, stable_key)
```

### 18.5 `harness_asset_versions`

```text
id uuid primary key
asset_id uuid not null
version bigint not null
content_artifact_id uuid not null
content_hash text not null
manifest_json jsonb not null
manifest_hash text not null
source_proposal_id uuid
created_at timestamptz not null
unique(asset_id, version)
```

### 18.6 `harness_activations`

```text
id uuid primary key
asset_version_id uuid not null
scope_kind text not null
scope_id text
status text not null
approved_by text not null
approval_evidence_artifact_id uuid not null
activated_at timestamptz
retired_at timestamptz
supersedes_activation_id uuid
```

### 18.7 `refinement_proposals`

```text
id uuid primary key
project_id uuid
task_id uuid
run_id uuid not null
kind text not null
scope_kind text not null
scope_id text
status text not null
proposal_artifact_id uuid not null
proposal_hash text not null
source_evidence_json jsonb not null
capability_impact_json jsonb not null
created_by_model_policy_hash text not null
created_at timestamptz not null
updated_at timestamptz not null
```

### 18.8 `refinement_evaluations`

```text
id uuid primary key
proposal_id uuid not null
baseline_asset_set_hash text not null
candidate_asset_set_hash text not null
fixture_set_hash text not null
status text not null
result_artifact_id uuid not null
metrics_json jsonb not null
created_at timestamptz not null
unique(proposal_id, baseline_asset_set_hash, candidate_asset_set_hash, fixture_set_hash)
```

### 18.9 Existing verification tables

Add or preserve:

```text
execution_fingerprint text not null
reused_from_verification_run_id uuid
reuse_reason text
original_executed_at timestamptz
```

### 18.10 Constraints

- Every content hash uses one canonical lowercase SHA-256 representation.
- Status columns use closed database constraints or enum-like checks.
- Cross-project/run/workspace references must match through composite constraints or transaction validation.
- Canonical rows reference immutable artifact bytes.
- No model-supplied timestamp, sequence, hash, or identity is trusted without host recomputation.

---

## 19. Go Interfaces

Illustrative boundaries:

```go
type ProgrammaticWorkspaceManager interface {
    Start(ctx context.Context, req StartProgrammaticWorkspaceRequest) (ProgrammaticWorkspace, error)
    Execute(ctx context.Context, req ProgrammaticExecutionRequest) (ProgrammaticExecutionResult, error)
    Inspect(ctx context.Context, id uuid.UUID) (ProgrammaticWorkspace, error)
    Stop(ctx context.Context, req StopProgrammaticWorkspaceRequest) error
}

type TrajectoryReader interface {
    List(ctx context.Context, q TrajectoryQuery) ([]TrajectoryEntry, error)
    Get(ctx context.Context, runID uuid.UUID, sequence int64) (TrajectoryEntry, error)
    Search(ctx context.Context, q TrajectorySearchQuery) ([]TrajectorySearchHit, error)
    Manifest(ctx context.Context, runID uuid.UUID, throughSequence int64) (TrajectoryManifest, error)
}

type ScratchStore interface {
    Put(ctx context.Context, req PutScratchRequest) (ScratchEntry, error)
    Get(ctx context.Context, req GetScratchRequest) (ScratchEntry, error)
    List(ctx context.Context, q ScratchQuery) ([]ScratchEntry, error)
    Tombstone(ctx context.Context, req TombstoneScratchRequest) (ScratchEntry, error)
}

type SkillRegistry interface {
    ActiveManifest(ctx context.Context, scope HarnessScope, role Role) (SkillSetManifest, error)
    Resolve(ctx context.Context, stableKey string, version int64) (SkillVersion, error)
}

type RefinementService interface {
    Propose(ctx context.Context, req RefinementProposalRequest) (RefinementProposal, error)
    Validate(ctx context.Context, id uuid.UUID) (RefinementValidation, error)
    Evaluate(ctx context.Context, id uuid.UUID) (RefinementEvaluation, error)
    Approve(ctx context.Context, req ApproveRefinementRequest) (HarnessActivation, error)
    Reject(ctx context.Context, req RejectRefinementRequest) error
    Rollback(ctx context.Context, req RollbackHarnessActivationRequest) (HarnessActivation, error)
}

type VerificationResultCache interface {
    LookupTerminal(ctx context.Context, executionFingerprint string) (VerificationOccurrence, bool, error)
    RecordReuse(ctx context.Context, req VerificationReuseRequest) (VerificationOccurrence, error)
}
```

Implementations must keep model/runtime code outside storage packages and preserve sqlc-owned query boundaries.

---

## 20. Failure and Recovery Semantics

### 20.1 Python process crash

- Mark the active programmatic workspace occurrence `runtime_lost`.
- Preserve completed execution records and scratch entries.
- Do not claim transient globals survived.
- Policy may start a fresh programmatic workspace bound to the same exact source and reload explicit scratch.

### 20.2 Container crash

- Existing sandbox/workspace reconciliation applies.
- Host captures any trustworthy external evidence available before cleanup.
- Unfinished Python execution is terminally interrupted.
- No automatic retry repeats an external effect unless the broker can prove it did not occur or the operation is idempotent by exact identity.

### 20.3 Host crash

- PostgreSQL and immutable artifact operations remain authoritative.
- Recovery reconstructs the latest legal state from canonical rows/events.
- A missing live Python process is expected; restart uses scratch and trajectory.

### 20.4 Artifact failure

- A Python execution that cannot persist required output evidence is not accepted as successful.
- Scratch creation is atomic with artifact registration and trajectory event persistence.

### 20.5 Skill failure

- A skill exception is ordinary untrusted execution evidence.
- Repeated matching failures may produce a refinement or retirement proposal.
- The worker cannot silently change the active skill implementation.

---

## 21. Evaluation and Adoption Gate

### 21.1 Modes

- `direct_tools_v1` — current bounded worker using explicit brokered tools.
- `programmatic_workspace_v1` — same task/policy/model with `python_exec`, trajectory query, scratch, and active evaluated skills.

### 21.2 Required comparisons

Run both modes against:

- every relevant deterministic architecture evaluation scenario;
- large test-log diagnosis;
- multi-file code navigation;
- repeated correction after stable verification failure;
- long task history retrieval;
- context-budget pressure;
- runtime crash and recovery;
- stale scratch/identity refusal;
- malicious Python attempting host/database/runtime access;
- at least ten real sequential dogfood tasks after the core loop is stable.

### 21.3 Metrics

Record:

- final typed outcome;
- criteria satisfied with evidence;
- false-completion attempts;
- safety/policy denials;
- original-checkout changes;
- model input/output/reasoning/cached tokens where available;
- context-package bytes;
- direct tool calls;
- Python executions;
- repeated file/artifact reads;
- retrieval calls and hit rate;
- verification executions and reuses;
- correction cycles;
- wall-clock duration;
- sandbox CPU/memory/disk usage;
- operator interventions;
- runtime/protocol failures.

### 21.4 Initial enablement threshold

`programmatic_workspace_v1` may become the default implementer/corrector mode only when:

1. It passes every deterministic safety, completion, recovery, and original-checkout invariant passed by `direct_tools_v1`.
2. It introduces zero new host-boundary violations or unauthorized canonical mutations.
3. It does not reduce real-task completion success.
4. It materially improves at least one measured long-context outcome, such as task success, median input tokens, repeated reads, or correction cycles.
5. Any wall-time or operational-cost regression is recorded and judged acceptable by the operator.
6. The exact worker image, Python runtime, protocol, and active skill set are pinned and reproducible.

Until those conditions hold, the mode remains experimental and opt-in.

### 21.5 Skill/refinement activation threshold

No proposed asset activates without:

- schema and policy validation;
- deterministic tests;
- exact source provenance;
- no capability escalation;
- evaluation against a fixed baseline when behavior changes;
- explicit operator approval;
- rollback evidence.

---

## 22. Operator and UI Requirements

The UI/CLI should eventually expose:

- active programmatic workspace status;
- Python execution sequence and bounded previews;
- full stdout/stderr/result artifact links;
- durable scratch catalog;
- active skill-set versions and hashes;
- trajectory manifest and covered sequence;
- verification fingerprint and reuse source;
- pending refinement proposals;
- proposal evidence, evaluation results, capability impact, approve/reject controls;
- active harness assets and rollback controls;
- worker-mode comparison metrics.

No UI control may directly alter canonical source or activate a proposal without the same host policy and transaction path used by CLI operations.

---

## 23. Configuration

Suggested configuration shape:

```yaml
programmatic_workspace:
  enabled: false
  mode: experimental
  runtime_image: revolvr-worker-python@sha256:<digest>
  python_version: "3.13.x"
  protocol_version: revolvr-programmatic-workspace-v1
  roles: [implementer, corrector]
  max_executions_per_worker: 50
  max_execution_seconds: 60
  max_inline_bytes: 16384
  max_total_artifact_bytes: 268435456
  memory_limit_bytes: 8589934592
  cpu_limit: 4
  pids_limit: 128
  network: none
  allow_runtime_package_install: false

continual_harness:
  proposals_enabled: true
  automatic_activation: false
  require_operator_approval: true
  allowed_kinds:
    - prompt_note
    - python_skill
    - retrieval_rule
    - project_memory
    - verification_hint
    - task_compiler_rule

verification:
  reuse_exact_fingerprints: true
  require_fresh_final_gate: true
```

Configuration hashes become part of run/model/tool/evaluation provenance.

---

## 24. Implementation Task Sequence

The following sequence begins after the canonical architecture sequence and initial core-loop dogfooding. Task IDs are illustrative and may be adapted to the repository's canonical task compiler.

### PTC-001 — Amend pending core task specifications

**Timing:** immediately, before architecture tasks 016-018, 021-022 execute.
**Outcome:** incorporate the contract-level amendments in Section 2.2 without implementing the Python workspace.

Acceptance:

- Only pending task specifications and architecture authority documents change.
- No runtime dependency or product code is added.
- Current task ordering remains intact.

### PTC-101 — Trajectory manifest and query service

**Depends on:** completed core loop, artifacts/events, context assembly, evaluation suite.
**Outcome:** normalize complete ordered run trajectories and provide bounded read/query interfaces.

Acceptance:

- Every model/tool/context/evidence action appears in a deterministic manifest.
- Equal canonical inputs produce equal manifest bytes.
- Search indexes are rebuildable and never authority.
- Completion capsules can resolve trajectory manifests.

### PTC-102 — Programmatic workspace protocol and sandbox runtime

**Depends on:** PTC-101.
**Outcome:** start, execute, inspect, cancel, and stop a pinned Python workspace inside the worker container.

Acceptance:

- No host/database/runtime credential is reachable.
- Transient globals persist within one occurrence.
- Timeout/cancellation kills descendants and leaves evidence.
- Container/process loss is typed and recoverable through a fresh occurrence.
- No arbitrary process snapshot or pickle authority is introduced.

### PTC-103 — `python_exec` broker integration

**Depends on:** PTC-102.
**Outcome:** add `python_exec` to implementer/corrector through the existing role-scoped broker.

Acceptance:

- Tool schemas and role permissions are closed.
- Exact code/result/artifact/trajectory evidence is retained.
- Claimed work remains reconciled against host-observed Git state.
- The worker cannot call OpenAI, PostgreSQL, Docker, or canonical-state APIs.

### PTC-104 — Durable scratch catalog

**Depends on:** PTC-103.
**Outcome:** add explicit named text/JSON/bytes/table scratch entries and Python APIs.

Acceptance:

- Scratch survives container restart.
- Scratch identity/source/provenance is exact.
- Scratch never overrides direct source or canonical evidence.
- Tombstones preserve history.

### PTC-105 — Versioned Python skills

**Depends on:** PTC-104.
**Outcome:** load hash-pinned, tested, role-scoped skills into the programmatic workspace.

Acceptance:

- Skills have manifests, tests, capability declarations, and immutable versions.
- No runtime package installation.
- Activation and rollback are host-owned and fully recorded.

### PTC-106 — Refinement proposal workflow

**Depends on:** PTC-105.
**Outcome:** allow bounded evidence-backed proposals for prompt notes, skills, retrieval rules, project memory, verification hints, and task compiler rules.

Acceptance:

- No proposal self-activates.
- Capability escalation is rejected.
- Operator approve/reject/rollback works through canonical transactions.
- Every active asset version is included in future run provenance.

### PTC-107 — Programmatic context and continuation policy

**Depends on:** PTC-104, PTC-101.
**Outcome:** let workers query externalized context/trajectory and continue across bounded model turns without full-history reinjection.

Acceptance:

- Context packages remain frozen and authoritative.
- Full trajectory remains recoverable.
- Compaction summaries bind exact source ranges.
- Model-requested compaction cannot delete or rewrite history.

### PTC-108 — Paired evaluation and default-mode decision

**Depends on:** PTC-102 through PTC-107.
**Outcome:** compare `direct_tools_v1` and `programmatic_workspace_v1`, then record `defer`, `retain_opt_in`, or `make_default`.

Acceptance:

- Deterministic and real dogfood evidence is exact and repeatable where applicable.
- Safety/completion invariants do not regress.
- The decision links all metrics and failures.
- A default-mode change is configuration/decision only; direct-tool mode remains a rollback path.

---

## 25. Definition of Done

This extension is complete when:

- the core Revolvr sequential loop is already functioning and evaluated;
- the task-scoped Python workspace runs only inside the disposable sandbox;
- complete trajectories are append-only, queryable, and manifest-backed;
- workers can externalize and recover explicit scratch without process snapshots;
- `python_exec` is role-scoped, bounded, evidenced, cancellable, and reconciled;
- verification fingerprint reuse is exact and cannot create false passes;
- skills are versioned, tested, pinned, and host-activated;
- refinement proposals are evidence-backed, evaluated, operator-approved, and reversible;
- the programmatic worker passes all existing safety and false-completion tests;
- paired evaluation supports an evidence-backed default-mode decision;
- disabling the feature returns Revolvr to the direct-tool worker without data loss or migration of canonical authority.

---

## 26. Removal and Rollback

The feature must remain removable.

- Canonical tasks, plans, source, verification, audit, completion, and events remain valid without the Python runtime.
- Scratch and trajectory artifacts remain readable as ordinary artifacts.
- Harness assets can be deactivated without deleting their history.
- `direct_tools_v1` remains supported through the adoption decision.
- Programmatic tables may remain historical when the runtime is disabled.
- No completion depends solely on an opaque Python variable or unavailable process snapshot.

---

## 27. Source References

- Prime Agent article: `https://www.primeintellect.ai/blog/prime-agent`
- Prime Agent repository: `https://github.com/PrimeIntellect-ai/prime-agent`
- Revolvr canonical architecture: `REVOLVR_CANONICAL_ARCHITECTURE_SPEC.md`
- Current Revolvr architecture tasks: `.agent/tasks/architecture-015-planner.md` through `.agent/tasks/architecture-025-memory-graphiti-phase-gate.md`

---

## 28. Final Design Principle

Revolvr may give an agent a powerful programmable workspace, but it must never confuse **better cognition** with **greater authority**.

Python may help the worker understand and manipulate task data. PostgreSQL, host policy, independent verification, exact evidence, and deterministic completion continue to decide what is true and what is allowed.
