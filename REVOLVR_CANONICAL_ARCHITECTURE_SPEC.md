# Revolvr — Canonical Architecture and Product Specification

**Status:** Canonical architecture baseline\
**Version:** 0.1\
**Date:** 2026-08-03\
**Primary audience:** Project owner, implementation agents, task-generation agents, and future maintainers\
**Intended use:** Convert this specification into epics, implementation plans, bounded tasks, acceptance criteria, verification suites, and architectural decision records\
**Working product name:** **Revolvr**

---

## Document Purpose

This document defines the next architecture of Revolvr: a local-first, single-user, sequential autonomous software-engineering harness. It supersedes the earlier assumption that Sodoryard should remain backed by Shunter and replaces the prior SQLite/LanceDB-oriented storage design with PostgreSQL, `sqlc`, PostgreSQL full-text search, and `pgvector`.

The specification intentionally combines the strongest ideas from the existing Revolvr and Sodoryard projects while excluding subsystems that do not materially improve autonomous engineering correctness.

The resulting system is not an “AI swarm,” a hosted agent platform, or a generalized application runtime. It is a personal engineering operating system that:

1. Accepts a bounded list of software-engineering tasks.
2. Compiles vague requests into executable task contracts.
3. Selects and executes one task at a time.
4. Runs every untrusted coding action in a disposable sandbox.
5. Uses OpenAI models for high-value reasoning.
6. Uses local models for inexpensive retrieval utilities where they prove useful.
7. Requires deterministic verification and evidence before accepting claims.
8. Preserves durable knowledge about decisions, failures, attempts, and outcomes.
9. Stops safely when the correct next action is unclear.
10. Remains understandable, recoverable, and maintainable for years.

This document is deliberately detailed. It should serve as the root source for generating the project backlog.

---

# 1. Executive Summary

Revolvr is a local autonomous coding harness for one operator. It runs on a powerful personal workstation and is designed to work across real local software projects without granting model-controlled processes direct access to the host operating system.

Its core architecture is:

```text
Human intent
    |
    v
Task compiler
    |
    v
Canonical task contract
    |
    v
Sequential autonomy coordinator
    |
    +--> policy and lifecycle validation
    +--> role-specific context assembly
    +--> OpenAI model reasoning
    +--> disposable sandbox execution
    +--> deterministic verification
    +--> independent audit
    +--> correction loop
    |
    v
Evidence-bound completion
    |
    v
Durable project memory and history
```

The core technology direction is:

```text
Go
PostgreSQL
sqlc
pgx
SQL migrations
pgvector
PostgreSQL full-text search
Tree-sitter-based code intelligence
OpenAI Responses API
Local embedding service
Rootless OCI containers
Optional gVisor sandbox runtime
Vue 3 + TypeScript desktop/web UI
```

The system must remain sequential. It may manage a queue of tasks, but only one source-mutating task may execute at a time. Parallel workers, swarms, and concurrent mutation are explicit non-goals.

The model never owns truth. Models may propose:

- plans,
- next actions,
- source changes,
- audit findings,
- corrections,
- summaries,
- candidate memories.

Deterministic host code owns:

- lifecycle transitions,
- task readiness,
- source identity,
- workspace admission,
- tool permissions,
- network permissions,
- acceptance-criterion status,
- verification results,
- finding resolution,
- commits,
- completion,
- recovery.

---

# 2. Product Definition

## 2.1 Vision

Revolvr should become a durable personal engineering companion capable of accurately completing bounded lists of software tasks with minimal supervision while retaining enough evidence, observability, and human-readable state that the operator can understand and trust what happened.

The long-term success criterion is not “the agent produced code.” It is:

> Revolvr repeatedly transforms bounded task intent into verified, reviewable, recoverable repository changes without silently broadening scope, corrupting the host, fabricating completion, or forgetting prior failures.

## 2.2 Primary User

There is exactly one supported user: the local operator.

No account model is required. No organizations, teams, memberships, tenants, RBAC hierarchy, or remote identity provider should be built.

The security question is not:

> Is user X authorized to mutate project Y?

It is:

> Is this model-proposed action permitted by the current task contract, policy, lifecycle state, and sandbox boundary?

## 2.3 Target Environment

The first-class deployment target is a powerful local Linux workstation with:

- substantial local storage,
- substantial system memory,
- a modern NVIDIA GPU,
- a local OCI container runtime,
- local PostgreSQL running in a container,
- reliable access to OpenAI APIs,
- local Git repositories.

Windows support may later be provided through WSL2. Native Windows execution is not a v1 requirement.

## 2.4 Primary Operating Modes

Revolvr should eventually support four operator modes:

### Interactive inspection

The operator can inspect projects, tasks, plans, runs, diffs, evidence, retrieval context, model usage, findings, and history without starting autonomous work.

### Single-task execution

The operator explicitly selects one ready task and runs it until it:

- completes,
- blocks,
- needs input,
- exhausts its budget,
- encounters an unsafe condition,
- is cancelled.

### Sequential queue execution

The operator starts a bounded queue operation. Revolvr repeatedly selects the next ready task and runs tasks sequentially. It skips or yields blocked tasks so unrelated ready work may continue.

### Design and task compilation

The operator supplies prose, a specification, an issue, or a Markdown import. Revolvr compiles it into one or more bounded task contracts for review before execution.

## 2.5 Product Goals

1. **Correctness over throughput.**
2. **Sequential, bounded execution.**
3. **Evidence before completion.**
4. **Disposable execution environments.**
5. **Model-independent durable state.**
6. **Role-specific context instead of giant prompts.**
7. **Local-first operation and storage.**
8. **Strong crash recovery and idempotency.**
9. **Useful long-term memory without making memory authoritative.**
10. **Simple enough for one developer to maintain.**

## 2.6 Explicit Non-Goals

The following are out of scope unless this specification is intentionally revised:

- Multi-user authentication.
- Organizations, teams, or permissions administration.
- Multi-tenancy.
- Hosted SaaS operation.
- Distributed control planes.
- Kubernetes as a requirement.
- Parallel source-mutating workers.
- Agent swarms.
- Arbitrary agent-to-agent delegation.
- Infinite autonomous loops.
- A custom database runtime.
- Shunter integration.
- LanceDB integration.
- Neo4j or Graphiti as a launch dependency.
- Local coding models as the primary reasoning engine.
- Automatic production deployment.
- Automatic release publication.
- Self-modifying system prompts.
- Self-modifying policy.
- Model-controlled container specifications.
- Direct model access to the host Docker or Podman socket.
- Automatic merging into protected branches.
- Silent network access.
- Silent secret access.

---

# 3. Core Design Principles

## 3.1 Models Propose; the Host Decides

A model response is never a state transition merely because it is well-formed.

Every model output must pass:

1. Schema validation.
2. Task-identity validation.
3. Source-revision validation.
4. Lifecycle validation.
5. Policy validation.
6. Evidence validation.
7. Budget validation.
8. Replay/idempotency validation.

## 3.2 Canonical State Is Relational

PostgreSQL is the system of record for runtime state.

Search indexes, embeddings, code graphs, summaries, and future knowledge graphs are derived projections. They may be rebuilt or discarded without losing canonical task or run history.

## 3.3 One Coordinator Owns Writes

One trusted Revolvr coordinator owns canonical state transitions.

Worker sandboxes do not write directly to PostgreSQL. They return bounded structured results and artifacts to the coordinator.

This creates a clear authority boundary:

```text
Worker:
    may mutate its isolated workspace
    may emit candidate results

Coordinator:
    validates results
    persists state
    runs verification
    creates commits
    finalizes tasks
```

## 3.4 Sequential by Default and by Design

Only one source-mutating task may execute at a time.

The scheduler may analyze a queue, but it must not start concurrent workers. Sequential execution reduces:

- conflicting assumptions,
- merge complexity,
- runaway cost,
- hidden dependency races,
- difficult recovery,
- ambiguous evidence,
- host resource exhaustion.

## 3.5 Every Claim Requires Evidence

An agent may claim “the task is complete,” but completion is impossible unless the host can bind that claim to:

- a specific source revision,
- a specific task version,
- a terminal plan,
- terminal acceptance criteria,
- verification evidence,
- an independent clean audit,
- resolved blocking findings,
- a bounded final diff,
- a completion manifest.

## 3.6 Sandboxes Are Mandatory for Untrusted Work

Models must never execute arbitrary commands directly on the host.

All shell commands, builds, test runs, code generation, package-manager calls, and source mutations happen inside disposable worker or verifier sandboxes.

## 3.7 Human Input Is a Typed State

When a decision requires the operator, Revolvr should persist a structured question with:

- stable question identity,
- revision,
- reason,
- options,
- recommendation,
- evidence,
- exact answer,
- answer time,
- resume history.

The system must not auto-select its own recommendation.

## 3.8 Memory Supports Work; Memory Does Not Authorize Work

Project memory may help answer:

- why a decision was made,
- what failed previously,
- which conventions apply,
- what code is related,
- which strategies have already failed.

Memory cannot independently:

- satisfy a dependency,
- complete a criterion,
- authorize a mutation,
- resolve a finding,
- mark a task complete.

## 3.9 Everything Important Is Versioned

Version or hash:

- task contracts,
- prompts,
- JSON schemas,
- policy,
- tool definitions,
- container images,
- embedding models,
- source revisions,
- verification plans,
- context packages,
- artifacts,
- model settings.

---

# 4. Canonical Architecture Decisions

These decisions should be converted into individual ADR files during implementation.

## ADR-001 — Product Name

The working product name remains **Revolvr**.

The implementation may continue in the current Revolvr repository or begin from a clean branch/rewrite. The architecture does not depend on repository history.

## ADR-002 — Single-User Local-First Product

Revolvr is built exclusively for one local operator.

No user table, organization table, team table, membership table, or remote authentication system will be created.

A small local API secret may still be used to protect the loopback API from accidental local access; this is transport protection, not a user-account system.

## ADR-003 — Remove Shunter Entirely

Shunter is an independent product and must not remain a runtime dependency of Revolvr.

Do not port:

- Shunter modules,
- reducers,
- protocol clients,
- generated TypeScript bindings,
- Shunter RPC,
- Shunter snapshots,
- Shunter subscriptions.

Relevant data and behavior should be reimplemented using PostgreSQL transactions, SQL queries, application events, and a simple local streaming API.

## ADR-004 — PostgreSQL Is the Canonical Database

PostgreSQL is the sole canonical application database.

SQLite is not a supported production backend for the new architecture.

Tests may use ephemeral PostgreSQL containers. Do not create a second SQLite implementation merely for test convenience.

## ADR-005 — Go + pgx + sqlc

Application persistence uses:

- Go,
- `pgx`,
- `sqlc`,
- SQL-first migrations.

Queries should remain explicit and inspectable.

Dynamic or highly specialized search queries may use a narrowly contained handwritten query layer when `sqlc` is unsuitable, but this should be exceptional.

## ADR-006 — SQL Migrations

Use a SQL-first migration tool such as Goose.

Migrations must be:

- ordered,
- reversible when reasonably possible,
- tested against an empty database,
- tested against the previous released schema,
- included in backup/restore validation.

## ADR-007 — pgvector Replaces LanceDB

Embedding vectors live in PostgreSQL through `pgvector`.

PostgreSQL full-text search and pgvector form the baseline hybrid retrieval system.

LanceDB is not carried into the new architecture.

## ADR-008 — Local Embeddings, Remote Frontier Reasoning

Initial model responsibilities:

### OpenAI remote models

- supervisor decisions,
- planning,
- implementation reasoning,
- correction reasoning,
- independent auditing,
- difficult task compilation.

### Local models

- embeddings,
- optional reranking,
- optional low-risk classification,
- optional entity extraction after evaluation.

Local coding models are not a v1 dependency.

## ADR-009 — OpenAI Is the Only Remote Provider

The provider abstraction should be narrow enough to avoid hard coupling, but only OpenAI needs to be implemented initially.

Do not carry forward broad multi-provider complexity from Sodoryard.

## ADR-010 — Direct API Credentials Stay Outside Workers

The OpenAI API key remains in the trusted control plane.

Worker sandboxes do not receive long-lived OpenAI credentials.

The trusted agent runtime performs model calls and brokers validated tools into sandboxes.

## ADR-011 — Sequential Autonomous Execution

There is no parallel worker execution.

A queue may contain many tasks, but one source-mutating task is active at a time.

## ADR-012 — Managed Project Copies

The safest default project mode uses a Revolvr-managed Git mirror or clone and ephemeral worktrees.

The operator’s original checkout is not bind-mounted read-write into worker containers.

Successful work becomes:

- a commit in the managed project repository,
- an exportable patch,
- or a branch that can be explicitly pushed or applied.

## ADR-013 — Disposable Sandboxes

Every role that may execute commands operates against a disposable sandbox.

At minimum:

- implementer,
- corrector,
- verifier.

Auditors should normally inspect a read-only source snapshot and evidence without mutation.

## ADR-014 — Rootless OCI Runtime

Use a rootless container engine.

The initial implementation should abstract Docker and Podman behind a small `SandboxRuntime` interface. Implement one backend first.

## ADR-015 — Strong Sandbox Profile

Support sandbox profiles:

- `strict`: rootless runtime plus gVisor when compatible, no network by default.
- `compatible`: rootless standard OCI runtime with hardening.
- `diagnostic`: explicitly attended, less restrictive, never used silently.

The selected profile becomes part of run identity and evidence.

## ADR-016 — Local Filesystem Artifact Store

Large artifacts live in a content-addressed local filesystem store.

Do not add MinIO or S3 compatibility to v1.

PostgreSQL stores artifact metadata and hashes. A future object-store adapter may be added without changing artifact identities.

## ADR-017 — Relational Relationship Graph First

Typed project relationships initially live in PostgreSQL tables.

Graphiti, Neo4j, or FalkorDB are deferred dogfooding experiments.

## ADR-018 — Graphiti Is Optional and Derived

A future Graphiti integration may consume accepted documents, tasks, runs, findings, and decisions to produce a temporal knowledge projection.

It may improve retrieval but may never become canonical lifecycle authority.

## ADR-019 — No Full Event Sourcing

Revolvr uses ordinary current-state tables plus an append-only event/audit table.

Do not rebuild the entire system around event sourcing.

State transitions should atomically update current state and append the associated durable event.

## ADR-020 — CLI First, Desktop UI Second

The core must be fully operable from the CLI.

The desktop UI should use the same application service interfaces and must not contain unique business logic.

The preferred desktop stack is Wails + Vue 3 + TypeScript.

## ADR-021 — REST + Server-Sent Events

Use local REST APIs for commands and queries.

Use Server-Sent Events for progress/event streaming unless a concrete requirement proves WebSockets necessary.

## ADR-022 — Manual Queue Start Before Daemon Mode

The operator explicitly starts bounded queue execution.

A persistent unattended daemon is deferred until evaluation and recovery gates demonstrate reliability.

## ADR-023 — Prompt and Policy Immutability During a Run

A run pins the exact prompt version, tool schema version, policy version, and task version.

A worker may not alter these inputs during that run.

## ADR-024 — Verification Is Host-Owned

Verification commands and acceptance methods are admitted before execution.

The implementing agent does not gain authority by editing tests, scripts, or verification definitions.

Changes to verification authority must be detected and either rejected or escalated.

---

# 5. High-Level System Context

```text
+-------------------------------------------------------------------+
|                           Local Operator                           |
|                                                                   |
| CLI / Wails desktop UI / Markdown task imports                     |
+-------------------------------+-----------------------------------+
                                |
                                v
+-------------------------------------------------------------------+
|                    Trusted Revolvr Control Plane                   |
|                                                                   |
| API | Task compiler | Scheduler | Policy | Lifecycle | Agent loop |
| Context compiler | Evidence engine | Verification coordinator      |
+----------------------+--------------------------+-----------------+
                       |                          |
                       v                          v
             +------------------+       +---------------------------+
             |   PostgreSQL     |       | Content-addressed files   |
             | + pgvector       |       | logs, patches, reports    |
             +------------------+       +---------------------------+
                       |
                       v
             +------------------+
             | Local embedding  |
             | service on GPU   |
             +------------------+

                                |
                     typed sandbox request
                                |
                                v
+-------------------------------------------------------------------+
|                  Trusted Local Sandbox Manager                    |
|                                                                   |
| Validates image, mounts, network, resources, paths, and runtime    |
+-------------------------------+-----------------------------------+
                                |
                                v
+-------------------------------------------------------------------+
|                 Disposable Untrusted Worker Sandbox               |
|                                                                   |
| Managed worktree | project toolchain | shell | tests | build       |
| No host home | no runtime socket | no ambient secrets              |
+-------------------------------------------------------------------+
```

---

# 6. Trust Boundaries

## 6.1 Trusted Components

- Revolvr CLI and desktop launcher.
- Revolvr API/control plane.
- PostgreSQL.
- Sandbox manager.
- Policy engine.
- Verification coordinator.
- Artifact store.
- Local embedding service, for availability only.
- Operator-authored configuration.
- Accepted task contracts and policy.

## 6.2 Untrusted or Partially Trusted Components

- Model-generated text.
- Model-generated shell commands.
- Model-generated source changes.
- Dependency install scripts.
- Project build scripts.
- Repository hooks.
- Package-manager lifecycle scripts.
- Test binaries.
- Downloaded project dependencies.
- Imported task prose.
- Candidate extracted graph facts.

## 6.3 Critical Rule

No untrusted component receives:

- host root filesystem mounts,
- host home directory,
- container runtime socket,
- PostgreSQL administrative credentials,
- persistent OpenAI API credentials,
- SSH agent socket,
- cloud credentials,
- GitHub tokens,
- arbitrary host network access.

---

# 7. Deployment Topology

## 7.1 Recommended Local Layout

```text
Host
├── revolvr CLI / Wails launcher
├── revolvr-sandboxd
├── rootless container runtime
└── managed data root
    ├── repositories/
    ├── workspaces/
    ├── artifacts/
    ├── backups/
    ├── models/
    └── runtime/

Persistent control stack
├── revolvr-api
├── postgres + pgvector
└── embedding-service

Ephemeral
├── worker container
├── verifier container
└── optional audit-support container
```

## 7.2 Why a Small Host Sandbox Manager Exists

Granting a general web/API container access to a container runtime socket creates a powerful host-control path.

Instead, a minimal trusted `revolvr-sandboxd` process should run as the local user and expose a narrow Unix-socket API.

It accepts only a validated sandbox specification:

```text
approved image digest
managed workspace identifier
approved read-only mounts
approved read-write workspace mount
resource limits
network profile
environment allowlist
runtime profile
command
timeout
```

It must reject:

- arbitrary host paths,
- privileged mode,
- host PID namespace,
- host network namespace,
- added capabilities,
- device mounts not explicitly allowed,
- container runtime socket mounts,
- unsafe symlinks,
- path traversal,
- unknown images.

The model never calls `sandboxd` directly.

## 7.3 Control Stack Containerization

The persistent control stack should be started through Docker Compose or an equivalent local composition tool.

The stack should include:

### `revolvr-api`

- Go control-plane service.
- Loopback-only or Unix-socket exposure.
- OpenAI API access.
- PostgreSQL access.
- Artifact-store access.
- No direct arbitrary host filesystem access.

### `postgres`

- Persistent named volume.
- pgvector extension installed.
- Local-only network.
- No host-public port by default unless development tools require it.
- Optional loopback binding for DataGrip.

### `embedding-service`

- GPU access.
- Read-only model mount.
- No project source access.
- Internal network only.
- OpenAI-compatible embedding endpoint preferred.

## 7.4 Sandbox Runtime

The first-class runtime should be rootless.

A strict profile should use gVisor where project compatibility permits. Standard rootless OCI execution may be used as an explicit compatibility fallback.

Containers must run:

- as a non-root UID,
- with all Linux capabilities dropped,
- with `no-new-privileges`,
- with a read-only root filesystem where practical,
- with a writable workspace mount only,
- with bounded PIDs,
- with bounded CPU,
- with bounded memory,
- with bounded temporary storage,
- with a deadline,
- with network disabled unless admitted,
- without host devices,
- without host sockets.

## 7.5 Network Profiles

Define explicit profiles:

### `none`

No network namespace connectivity.

Use for:

- source inspection,
- most implementation work after dependencies are available,
- verification,
- audits.

### `dependencies`

Access only to a controlled dependency-fetch path or explicitly approved registries.

Use for:

- `go mod download`,
- npm/pnpm package installation,
- Python dependency resolution.

Prefer a local caching proxy later.

### `open`

Explicitly attended diagnostic mode.

Never selected automatically.

## 7.6 GPU Access

Worker sandboxes do not need GPU access in v1.

The GPU is reserved for the local embedding or reranking service.

This reduces the attack surface and avoids coupling project containers to NVIDIA runtime details.

## 7.7 Container Image Policy

Each project declares a project environment contract.

A run pins:

- image name,
- image digest,
- Dockerfile hash,
- build-argument hash,
- toolchain versions,
- sandbox profile.

Mutable tags alone are not sufficient evidence.

---

# 8. Suggested Repository Structure

```text
revolvr/
├── cmd/
│   ├── revolvr/
│   ├── revolvr-api/
│   └── revolvr-sandboxd/
├── internal/
│   ├── api/
│   ├── app/
│   ├── artifact/
│   ├── audit/
│   ├── config/
│   ├── context/
│   ├── evidence/
│   ├── event/
│   ├── evaluation/
│   ├── git/
│   ├── graph/
│   ├── index/
│   ├── lifecycle/
│   ├── model/
│   ├── observability/
│   ├── policy/
│   ├── project/
│   ├── retrieval/
│   ├── sandbox/
│   ├── scheduler/
│   ├── storage/
│   ├── task/
│   ├── tool/
│   ├── verification/
│   └── workspace/
├── db/
│   ├── migrations/
│   ├── queries/
│   └── sqlc.yaml
├── prompts/
│   ├── supervisor/
│   ├── planner/
│   ├── implementer/
│   ├── auditor/
│   ├── corrector/
│   └── compiler/
├── schemas/
│   ├── task/
│   ├── decision/
│   ├── receipt/
│   ├── audit/
│   └── evidence/
├── sandbox/
│   ├── images/
│   ├── profiles/
│   └── seccomp/
├── web/
│   ├── src/
│   └── package.json
├── compose/
│   ├── compose.yaml
│   └── compose.dev.yaml
├── evals/
│   ├── fixtures/
│   ├── scenarios/
│   └── goldens/
├── docs/
│   ├── adr/
│   ├── architecture/
│   ├── runbooks/
│   └── specs/
└── Makefile
```

---

# 9. Core Subsystems

# 9.1 Project Registry

The project registry records repositories Revolvr may operate on.

Responsibilities:

- Register a local source repository.
- Resolve and validate its Git identity.
- Create a managed mirror or clone.
- Track default branch and remote configuration.
- Store project environment configuration.
- Store project guidance locations.
- Track indexing state.
- Track baseline verification configuration.
- Prevent path aliasing and duplicate registration.

A project record should include:

```text
id
name
source_path
managed_repo_path
default_branch
remote_url
status
environment_manifest_path
guidance_paths
created_at
updated_at
```

## 9.2 Managed Git Repository Service

The repository service should:

- Maintain a controlled mirror or clone.
- Fetch explicitly.
- Create task-specific branches.
- Create ephemeral worktrees.
- Bind every run to a source commit and tree.
- Capture dirty-state evidence.
- Create commits only after verification policy allows it.
- Export patches.
- Push branches only with explicit operator authority.
- Never force-push automatically.
- Never mutate the operator’s original checkout files.

## 9.3 Task Intake

Task sources may include:

- UI prose.
- CLI prose.
- Markdown import.
- A larger specification.
- A GitHub issue exported by the operator.
- A previously generated task bundle.

Task intake stores the original bytes as an immutable artifact.

## 9.4 Task Compiler

The task compiler transforms human intent into one or more executable task contracts.

It must identify:

- goal,
- scope,
- exclusions,
- dependencies,
- conflicts,
- acceptance criteria,
- verification methods,
- expected source area,
- mutation class,
- risk class,
- required human checkpoints,
- unresolved questions.

The compiler may use an OpenAI model, but its result is a proposal.

The host validates:

- unique IDs,
- bounded text,
- valid dependency graph,
- no cycles,
- supported verification methods,
- legal mutation class,
- non-empty acceptance criteria,
- no duplicate criteria,
- no unresolved placeholders,
- no unsupported external requirements.

Tasks that cannot be made executable must enter `needs_clarification`, not `ready`.

## 9.5 Task Review Gate

Before an imported or compiled task becomes ready, the operator should be able to review:

- task summary,
- scope,
- excluded scope,
- criteria,
- verification,
- risk,
- dependencies,
- expected files/modules,
- network requirement,
- secret requirement,
- budget.

The operator may approve one task or an entire compiled batch.

## 9.6 Scheduler

The scheduler selects one task at a time.

Readiness requires:

- status is pending,
- task version is accepted,
- all required dependencies are completed,
- no dependency is terminal-unsatisfied,
- no operator checkpoint is awaiting,
- project is healthy,
- no active mutating task exists,
- task graph is valid,
- required container image exists,
- required baseline checks are available.

Selection order:

1. Explicit priority ascending.
2. Task creation order or canonical path.
3. Stable task ID.

The scheduler does not let the model choose a different task after admission.

## 9.7 Lifecycle Engine

The lifecycle engine owns legal transitions.

Suggested task states:

```text
draft
compiled
awaiting_approval
pending
admitted
planning
ready
working
verifying
auditing
correcting
documenting
simplifying
needs_input
blocked
finalizing
completed
cancelled
budget_exhausted
unsafe
superseded
abandoned
```

Not every task must visit every state.

## 9.8 Policy Engine

The policy engine receives:

- current task state,
- exact task version,
- proposed model action,
- source revision,
- plan state,
- criterion state,
- finding state,
- verification state,
- audit state,
- budget state,
- sandbox profile,
- risk class.

It returns:

```text
allowed
denied
needs_input
unsafe
```

A denial includes a deterministic reason code.

## 9.9 Supervisor

The supervisor is a fresh, decision-only OpenAI invocation.

It receives a role-specific dossier and returns exactly one structured decision.

Potential actions:

```text
plan
implement
correct
document
simplify
complete
block
needs_input
```

Verification and finalization are host operations, not worker actions.

The supervisor cannot:

- run tools,
- edit source,
- mutate state,
- create a commit,
- answer its own operator question,
- broaden the task.

## 9.10 Planner

The planner produces or revises:

- plan identity,
- ordered plan steps,
- acceptance mapping,
- expected files/components,
- test strategy,
- risks,
- assumptions,
- evidence references.

Plans are versioned.

Existing completed steps cannot silently revert to pending.

Plan revisions must explain material changes.

## 9.11 Implementer

The implementer receives:

- accepted task contract,
- current plan,
- one active plan step or bounded batch,
- relevant code,
- conventions,
- prior failure memory,
- tool policy,
- sandbox limits.

It may mutate only its isolated worktree.

It returns:

- structured summary,
- changed-file claim,
- tests it ran voluntarily,
- concerns,
- candidate plan progress,
- candidate follow-up work.

Its claims do not automatically update canonical plan or criteria.

## 9.12 Verification Engine

Verification is deterministic and host-owned.

It should support tiers:

### Tier 0 — Admission baseline

Run before mutation when configured.

Purpose:

- establish pre-existing failures,
- prove the environment works,
- bind baseline evidence.

### Tier 1 — Focused checks

Task-specific tests, formatters, linters, generated-code checks.

### Tier 2 — Project checks

Project build and ordinary test suite.

### Tier 3 — Risk checks

Security, race, integration, migration, or performance checks.

### Tier 4 — Final clean verification

Fresh verifier sandbox against the exact candidate source revision.

Verification records:

- command,
- arguments,
- environment names,
- working directory,
- image digest,
- source revision,
- start/end time,
- exit code,
- stdout artifact,
- stderr artifact,
- structured test results,
- timeout,
- resource outcome.

## 9.13 Auditor

The auditor is independent from the implementer.

It receives:

- task contract,
- plan,
- acceptance matrix,
- candidate diff,
- changed-file manifest,
- verification evidence,
- code graph blast radius,
- relevant source,
- previous findings.

It returns:

```text
clean
changes_required
blocked
```

Findings must be structured:

```text
id
significance
summary
required correction
source evidence
affected files/symbols
criterion impact
```

## 9.14 Conditional Specialist Audits

Do not run every specialist on every task.

A deterministic impact router may request additional audit perspectives:

- security,
- performance,
- integration,
- database migration,
- documentation,
- API compatibility.

The router uses changed paths, symbols, task risk, and configuration.

## 9.15 Corrector

The corrector receives only:

- exact failed verification evidence, or
- exact open audit findings,
- source revision,
- previous strategy history,
- relevant code.

It may not redesign unrelated parts of the system.

After correction, the host runs verification again.

## 9.16 Completion Finalizer

Completion is a host transaction.

It verifies:

- task identity,
- accepted task version,
- source revision,
- plan terminality,
- criterion terminality,
- verification freshness,
- clean audit freshness,
- finding dispositions,
- budget legality,
- workspace identity,
- diff identity,
- commit identity,
- artifact hashes.

It then writes:

- completion evidence JSON,
- human-readable completion Markdown,
- completion manifest,
- terminal database state,
- completion event.

## 9.17 Artifact Store

Use a content-addressed filesystem:

```text
data/artifacts/sha256/ab/cd/<full-hash>
```

Artifact metadata includes:

```text
id
sha256
size
media_type
compression
logical_kind
source_run_id
source_task_id
created_at
retention_class
```

Artifact kinds include:

- prompt,
- dossier,
- model output,
- tool input,
- tool output,
- stdout,
- stderr,
- patch,
- diff,
- verification report,
- audit report,
- receipt,
- completion capsule,
- screenshot,
- task source,
- plan source,
- context report.

## 9.18 Evidence Engine

The evidence engine connects claims to artifacts and canonical records.

Example:

```text
Claim:
    "Expired tokens are rejected."

Evidence:
    verification check V-12
    test case TestExpiredToken
    source revision abc123
    container image digest sha256:...
```

A claim without admissible evidence remains unproven.

## 9.19 Context Compiler

The context compiler builds a frozen, role-specific dossier.

It must:

- receive an exact task/run scope,
- retrieve candidates,
- score and deduplicate,
- enforce a token budget,
- preserve provenance,
- serialize a stable format,
- store a context manifest,
- bind the dossier hash to the model invocation.

## 9.20 Retrieval Engine

Retrieval combines:

1. Canonical state.
2. Explicit files.
3. Exact symbols.
4. Text search.
5. Structural code graph.
6. PostgreSQL full-text search.
7. pgvector semantic search.
8. Typed project relationships.
9. Optional reranking.

## 9.21 Code Indexer

The code indexer should reuse good Sodoryard ideas while removing LanceDB.

Responsibilities:

- walk admitted project files,
- parse supported languages,
- chunk by semantic units,
- extract symbols,
- extract imports/calls/references,
- create code descriptions,
- produce embeddings,
- store hashes and index state,
- update incrementally after commits.

Initial language priorities:

- Go,
- TypeScript/JavaScript,
- Python,
- Markdown,
- SQL.

## 9.22 Project Memory

Project memory consists of human-readable durable documents:

- architecture decisions,
- conventions,
- debugging notes,
- project guidance,
- task completions,
- known issues,
- lessons.

Documents may be imported from the repository or created by Revolvr.

Canonical document bytes should be stored as artifacts or controlled files with database metadata.

## 9.23 Relationship Graph

Initial relationship types may include:

```text
DEPENDS_ON
CONFLICTS_WITH
IMPLEMENTS
AFFECTS
USES
SUPERSEDES
DOCUMENTED_IN
PRODUCED_BY
FOUND_IN
RESOLVED_BY
VALIDATED_BY
FAILED_WITH
RETRIED_WITH
APPLIES_TO
CHANGED
CONTAINS
CALLS
REFERENCES
```

Every semantic relationship stores provenance and authority.

## 9.24 Observability

Revolvr should record:

- structured logs,
- state transition events,
- model calls,
- token usage,
- latency,
- tool executions,
- sandbox lifecycle,
- verification results,
- audit results,
- context retrieval,
- failures,
- costs.

OpenTelemetry support is desirable, but a separate Prometheus/Grafana stack is not a v1 requirement.

## 9.25 Evaluation Harness

The evaluation harness runs deterministic scenarios against fixture repositories and optionally live OpenAI dogfood tasks.

It is a first-class subsystem, not an afterthought.

---

# 10. PostgreSQL Data Architecture

## 10.1 Database Organization

Use one local PostgreSQL database.

Suggested schemas:

```text
core
retrieval
telemetry
```

This is optional organizational structure, not separate databases.

## 10.2 ID Strategy

Use UUIDv7 generated in Go for major entities.

External human-readable task IDs may coexist with internal UUIDs.

## 10.3 Time

Store UTC timestamps using `timestamptz`.

Never use local time as canonical state.

## 10.4 JSONB Policy

Use normalized relational columns for:

- identity,
- lifecycle,
- relationships,
- query-critical status,
- time,
- authority.

Use JSONB for:

- model-specific settings,
- bounded structured payloads,
- diagnostic metadata,
- versioned envelopes.

Do not hide the entire domain model in JSONB.

## 10.5 Current State Plus Events

For every meaningful transition, one transaction should:

1. Validate expected current version.
2. Update current-state rows.
3. Insert an append-only event.
4. Insert references to produced artifacts.
5. Commit.

Optimistic concurrency uses version columns or exact expected hashes.

## 10.6 Suggested Core Tables

### `projects`

Registered projects.

### `project_sources`

Source repository paths, managed mirror identity, remotes, branches.

### `project_snapshots`

Source commit, tree, environment, dependency, policy, and image identities.

### `tasks`

Current task status and accepted version.

### `task_versions`

Immutable compiled task contracts.

### `task_dependencies`

Typed dependency edges.

### `task_conflicts`

Explicit task conflict edges.

### `task_acceptance_criteria`

Current criterion status.

### `task_acceptance_versions`

Immutable criterion definitions.

### `plans`

Current accepted plan.

### `plan_versions`

Immutable plan revisions.

### `plan_steps`

Current step state.

### `runs`

One bounded task operation.

### `run_cycles`

One supervisor/worker cycle within a run.

### `supervisor_decisions`

Structured accepted or rejected decisions.

### `agent_invocations`

Model, role, prompt, schema, usage, timing, status.

### `tool_executions`

Validated tool calls and results.

### `workspaces`

Managed worktree and sandbox lifecycle.

### `sandbox_executions`

Container image, runtime, profile, resources, exit status.

### `verification_runs`

Verification occurrence envelope.

### `verification_checks`

Individual deterministic checks.

### `audit_runs`

Audit occurrence and disposition.

### `audit_findings`

Canonical findings.

### `finding_dispositions`

Resolution, waiver, supersession, or rejection evidence.

### `operator_questions`

Versioned typed questions.

### `operator_answers`

Exact operator answers.

### `commits`

Commit and tree identities created or observed by Revolvr.

### `artifacts`

Content-addressed artifact metadata.

### `claims`

Claims requiring evidence.

### `claim_evidence`

Links claims to evidence sources.

### `events`

Append-only operational and lifecycle history.

### `budgets`

Task/run/cycle resource limits and consumption.

### `strategies`

Normalized strategy descriptions and fingerprints.

### `failure_signatures`

Normalized failure identities.

### `strategy_outcomes`

Strategy/failure/source outcomes.

### `locks`

Coordinator and project execution leases.

## 10.7 Suggested Retrieval Tables

### `documents`

Current document metadata.

### `document_versions`

Immutable document bytes/hash references.

### `chunks`

Searchable text or code chunks.

### `chunk_embeddings`

pgvector embeddings.

### `embedding_spaces`

Model, revision, dimensions, normalization, quantization, active status.

### `symbols`

Code symbols.

### `symbol_edges`

Calls, references, imports, implementations.

### `entities`

Knowledge entities.

### `entity_aliases`

Resolved aliases.

### `relations`

Typed relationships.

### `relation_sources`

Provenance and authority.

### `index_states`

Per-project index freshness.

## 10.8 Suggested Telemetry Tables

### `context_packages`

Dossier identity, role, hash, token estimate.

### `context_items`

Included/excluded retrieval items and scores.

### `model_usage`

Input, output, cached, reasoning, cost metadata.

### `system_metrics`

Versioned aggregate snapshots.

### `diagnostics`

Bounded structured diagnostics.

---

# 11. Task Contract

## 11.1 Task Contract Requirements

Every executable task must include:

```text
identity
title
goal
scope
excluded scope
dependencies
conflicts
acceptance criteria
verification plan
risk class
mutation class
network profile
secret requirements
expected source areas
budget
operator checkpoints
```

## 11.2 Mutation Classes

Suggested classes:

```text
read_only
documentation
test_only
bounded_source
database_migration
dependency_change
architecture_change
security_sensitive
release_or_deployment
```

Higher-risk classes require stronger review or checkpoints.

## 11.3 Risk Classes

```text
low
medium
high
critical
```

Risk affects:

- sandbox profile,
- verification tiers,
- audit types,
- allowed automatic completion,
- operator checkpoints.

## 11.4 Acceptance Criterion Model

Each criterion includes:

```text
id
requirement
verification_method
verification_reference
status
evidence
rationale
```

Statuses:

```text
pending
passed
failed
waived
not_applicable
blocked
```

Only terminal criteria permit completion.

## 11.5 Example Task Document

````markdown
---
schema: revolvr-task-v1
id: auth-expired-token
priority: 100
mutation_class: bounded_source
risk: medium
network: none
depends_on: []
conflicts: []
expected_paths:
  - internal/auth/**
budget:
  max_cycles: 8
  max_model_tokens: 500000
  max_wall_time: 2h
---

# Reject expired access tokens

## Goal

Ensure expired JWT access tokens are rejected by the authentication middleware.

## Scope

- Authentication middleware.
- Focused unit tests.
- Error response behavior.

## Excluded Scope

- Refresh-token rotation.
- User-session storage.
- Login UI.

## Acceptance

### AC-1

Expired access tokens are rejected.

Verification:

```text
go test ./internal/auth -run TestExpiredToken
```

### AC-2

Existing valid-token behavior remains passing.

Verification:

```text
go test ./internal/auth
```
````

The task compiler converts this into canonical database records.

---

# 12. Autonomous Workflow

## 12.1 Admission

Before a run:

1. Load accepted task version.
2. Validate project health.
3. Validate task graph.
4. Verify no active mutating run.
5. Fetch managed repository if requested.
6. Pin source commit and tree.
7. Resolve container image digest.
8. Pin policy and prompt versions.
9. Run baseline verification when configured.
10. Create run and budget records.
11. Acquire project execution lease.

## 12.2 Planning

If the task lacks an accepted plan or requires plan reconciliation:

1. Build planner dossier.
2. Invoke planner.
3. Validate structured plan.
4. Compare against task scope.
5. Validate criterion mapping.
6. Store candidate plan.
7. Accept automatically only under configured policy; otherwise expose review.
8. Transition to ready.

## 12.3 Implementation

1. Create managed worktree.
2. Start hardened sandbox.
3. Build implementer dossier.
4. Run fresh model invocation with brokered tools.
5. Capture all commands and outputs.
6. Stop at bounded limits.
7. Capture workspace diff.
8. Validate changed paths.
9. Reject protected-path changes.
10. Create candidate commit or immutable source snapshot.
11. Persist run result.

## 12.4 Verification

1. Destroy or freeze implementer sandbox.
2. Start fresh verifier sandbox.
3. Mount exact candidate source.
4. Run admitted verification plan.
5. Store every check and artifact.
6. Compare against baseline.
7. Detect verification-authority changes.
8. Transition based on deterministic result.

## 12.5 Audit

1. Build audit dossier.
2. Include exact diff and verification evidence.
3. Invoke independent auditor.
4. Validate finding schema.
5. Persist findings.
6. Route:
   - clean -> completion/documentation decision,
   - findings -> correction,
   - blocked -> blocked or needs input.

## 12.6 Correction

1. Build corrector dossier from exact findings or failure.
2. Include prior failed strategy history.
3. Require materially distinct strategy when retrying.
4. Execute in a fresh sandbox or clean continuation worktree.
5. Re-run verification.
6. Re-audit.

## 12.7 Documentation and Simplification

These are conditional, not mandatory phases.

Run documentation when:

- public behavior changed,
- operator workflow changed,
- architecture changed,
- task contract requires it.

Run simplification only when:

- the audit identifies unnecessary complexity, or
- the task explicitly includes simplification.

## 12.8 Finalization

1. Run final clean verification.
2. Confirm latest audit is clean.
3. Reconcile plan steps.
4. Reconcile acceptance criteria.
5. Confirm no findings are open.
6. Confirm source and diff identity.
7. Create final commit.
8. Materialize completion artifacts.
9. Mark task completed.
10. Release workspace and lease.
11. Update indexes and memory.

## 12.9 Needs Input

When blocked on operator judgment:

1. Persist exact question.
2. Stop source mutation.
3. Preserve workspace safely.
4. Release model budget.
5. Surface options and evidence.
6. Await explicit answer.
7. Revalidate source/task identity on resume.
8. Continue only if the answer is still applicable.

## 12.10 Blocked

A blocked task records:

- blocking reason,
- evidence,
- whether child tasks are suggested,
- whether other queue tasks may continue.

## 12.11 Cancellation

Cancellation must:

- signal active model/tool work,
- stop the sandbox,
- capture partial artifacts,
- reconcile workspace,
- preserve canonical evidence,
- release leases,
- leave a typed terminal-for-now outcome.

---

# 13. Role-Specific Context

## 13.1 Supervisor Context

Include:

- task contract summary,
- current lifecycle,
- plan state,
- criterion state,
- latest verification,
- latest audit,
- open findings,
- attempt history,
- strategy history,
- budget,
- high-authority project decisions.

Exclude:

- broad raw source dumps,
- unrelated code,
- entire conversation history.

## 13.2 Planner Context

Include:

- full task contract,
- architecture constraints,
- relevant project map,
- code/module relationships,
- conventions,
- prior related decisions,
- acceptance requirements,
- baseline verification.

## 13.3 Implementer Context

Include:

- exact active plan step,
- relevant files,
- relevant symbols,
- local conventions,
- focused tests,
- applicable architecture notes,
- prior failure memory,
- protected paths,
- tool/sandbox limits.

## 13.4 Auditor Context

Include:

- task contract,
- acceptance matrix,
- plan,
- exact diff,
- changed files/symbols,
- verification evidence,
- blast radius,
- previous findings.

## 13.5 Corrector Context

Include:

- exact failure or findings,
- current source,
- prior attempted strategies,
- relevant tests,
- bounded code context.

## 13.6 Context Package Manifest

Every dossier records:

```text
schema version
role
task ID
run ID
source revision
included sources
excluded sources
source hashes
scores
token estimates
final byte size
dossier SHA-256
retrieval configuration
embedding space
```

---

# 14. Retrieval and Indexing

## 14.1 Retrieval Order

The context compiler should prefer deterministic sources in this order:

1. Canonical task/run state.
2. Explicitly referenced files.
3. Exact symbol lookup.
4. Exact text/regex search.
5. Structural code relationships.
6. PostgreSQL full-text search.
7. pgvector semantic search.
8. Relationship graph expansion.
9. Optional reranking.

Vector similarity must not override a direct path or exact symbol reference.

## 14.2 Code Chunking

Prefer syntax-aware chunks:

- function,
- method,
- type,
- class,
- interface,
- module,
- SQL statement,
- Markdown section.

Fallback chunking should be deterministic and bounded.

## 14.3 Chunk Content

Each indexed chunk should retain:

```text
project
source revision
file path
language
symbol identity
chunk kind
start/end lines
content hash
raw body
semantic description
embedding
```

## 14.4 Embedding Service Interface

Define an internal interface:

```go
type Embedder interface {
    EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, text string) ([]float32, error)
    Metadata(ctx context.Context) (EmbeddingModelMetadata, error)
}
```

The first implementation should call a local containerized endpoint.

The service should expose model metadata:

- model name,
- revision,
- dimensions,
- pooling,
- normalization,
- quantization,
- artifact hash.

## 14.5 Embedding Model Selection

Do not permanently reuse the old Sodoryard model merely because it exists.

Build an embedding evaluation set from real projects:

- natural-language-to-code retrieval,
- symbol paraphrases,
- architecture-note retrieval,
- known bug retrieval,
- convention retrieval.

Choose the initial model by measured recall and latency on the local 4090.

## 14.6 Embedding Space Changes

A project uses one active embedding space at a time in v1.

Changing dimensions or model revision requires a controlled full reindex.

Store the previous embedding space until the new index passes validation, then switch atomically.

## 14.7 Hybrid Search

Combine:

- PostgreSQL FTS rank,
- vector similarity,
- exact-path boost,
- exact-symbol boost,
- graph-neighbor boost,
- recency/authority boost.

Use Reciprocal Rank Fusion or another transparent rank-combination method before introducing a learned ranker.

## 14.8 Optional Local Reranker

A local cross-encoder reranker may be added if evaluation proves it improves context hit rate.

It is not required for v1.

## 14.9 Freshness

Index state should be:

```text
never_indexed
clean
dirty
building
failed
```

A source commit records which index revision was used.

Stale indexes should degrade gracefully:

- direct reads and text search remain available,
- vector/graph results are labeled stale,
- high-risk autonomous runs may require a clean index.

---

# 15. Model Runtime

## 15.1 OpenAI API Use

Use the OpenAI Responses API or its current supported successor.

Requirements:

- streaming,
- tool calling,
- structured JSON schema outputs,
- usage reporting,
- reasoning-effort configuration,
- prompt/version metadata,
- retryable error classification.

## 15.2 Model Registry

Model selection must be configuration-driven by role:

```text
supervisor
planner
implementer
auditor
corrector
task_compiler
summarizer
```

Each role policy includes:

```text
model
reasoning effort
max output tokens
timeout
tool availability
structured schema
retry policy
cost ceiling
```

## 15.3 Fresh Sessions

Each role invocation is fresh and ephemeral.

Durable context comes from Revolvr state, not resumed model sessions.

## 15.4 Tool Calling

The model sees a limited tool schema.

The tool broker validates every call before executing it in the sandbox.

The model never supplies raw container flags or host mounts.

## 15.5 Local Model Policy

### Required at v1

- embeddings.

### Optional after evaluation

- reranking,
- simple classification,
- extraction,
- summarization.

### Deferred

- primary planning,
- implementation,
- auditing,
- correction.

## 15.6 Model Change Control

Before changing a role’s default model:

1. Run deterministic fixture evals.
2. Run representative live tasks.
3. Compare completion, false-completion, intervention, cost, and token metrics.
4. Record the model-policy decision.
5. Pin the new role policy version.

---

# 16. Tool System

## 16.1 Tool Categories

### Read-only source tools

- file read,
- directory list,
- text search,
- symbol search,
- Git diff/status/log,
- dependency inspection.

### Source mutation tools

- apply patch,
- file write,
- file edit,
- rename,
- delete within allowed scope.

### Command tool

- run a command in the sandbox.

### Memory tools

- search accepted project knowledge,
- read source documents.

### No direct canonical write tools

Workers do not receive tools to update:

- task status,
- criteria,
- plans,
- findings,
- database state,
- completion state.

## 16.2 Protected Paths

By default, workers may not modify:

```text
Revolvr task authority
Revolvr policy
Revolvr prompts
Revolvr runtime state
sandbox profiles
container runtime configuration
operator secrets
managed artifact store
```

Project-specific protected paths may include:

```text
CI security policy
release keys
production deployment manifests
database migration history
generated contracts
```

Explicit task authority may allow some protected changes with a higher risk class.

## 16.3 Command Policy

Commands are parsed and evaluated before execution.

Policy should inspect:

- executable,
- arguments,
- working directory,
- environment names,
- network profile,
- timeout,
- resource limits.

Shell strings should be avoided where direct argv execution is possible.

## 16.4 Output Limits

Every tool execution has:

- stdout limit,
- stderr limit,
- wall timeout,
- cancellation,
- artifact spillover.

Large output should be stored as an artifact and summarized in the model context.

---

# 17. Sandbox Security Specification

## 17.1 Threat Model

The sandbox must assume that model-generated commands or project scripts may attempt to:

- read host files,
- read credentials,
- alter unrelated repositories,
- access the container runtime,
- consume all memory,
- fork indefinitely,
- fill disks,
- access the network,
- persist after cancellation,
- exploit unsafe mounts.

## 17.2 Required Controls

- Rootless runtime.
- User namespace.
- Non-root container user.
- Drop all capabilities.
- `no-new-privileges`.
- Read-only root filesystem.
- No host network.
- No host PID/IPC namespace.
- No privileged mode.
- No runtime socket.
- No home-directory mount.
- No SSH agent.
- No Docker configuration mount.
- Bounded cgroups.
- PIDs limit.
- Tmpfs limits.
- Explicit workspace mount.
- Symlink-safe path validation.
- Container deadline.
- Forced cleanup.
- Image digest pinning.

## 17.3 Strict Mode

Strict mode should prefer gVisor.

A project that cannot run under strict mode may opt into compatible mode after an explicit diagnostic.

The run record must state which isolation profile was used.

## 17.4 Workspace Mounts

Suggested container mounts:

```text
/workspace          read-write task worktree
/context            read-only frozen dossier
/artifacts          write-only or scoped output staging
/cache/go           optional controlled cache
/cache/npm          optional controlled cache
/tmp                bounded tmpfs
```

Never mount the original project checkout.

## 17.5 Secrets

The default worker environment has no secrets.

When a task requires a credential:

1. Define the secret by name in policy.
2. Confirm the task is authorized.
3. Inject a short-lived scoped value.
4. Redact it from logs and artifacts.
5. Never persist the raw value.
6. Destroy the sandbox after use.

## 17.6 Repository Hooks

Git hooks and package-manager lifecycle hooks are untrusted.

The sandbox should:

- disable Git hooks by default,
- record when hooks are explicitly enabled,
- avoid inheriting user-level Git configuration,
- use controlled package-manager settings.

## 17.7 Verification Isolation

Final verification runs in a fresh sandbox, not in the implementer process.

This prevents accidental dependence on implementer-local state.

---

# 18. Evidence and Correctness

## 18.1 Evidence Hierarchy

Suggested authority levels:

1. Operator-approved task/specification.
2. Deterministic host state.
3. Verification results.
4. Source commit/tree.
5. Independent audit.
6. Accepted architecture/convention documents.
7. Worker receipt.
8. Model summary.
9. Candidate extracted memory.

Lower levels cannot override higher levels.

## 18.2 Baseline Verification

When practical, run verification before mutation.

This distinguishes:

- pre-existing failures,
- task-introduced failures,
- environment failures.

## 18.3 Differential Verification

Compare baseline and candidate:

```text
new failures
resolved failures
unchanged failures
flaky outcomes
```

Do not classify a task as failed merely because a pre-existing unrelated check remains failing unless policy requires a globally clean baseline.

## 18.4 Verification Authority Tampering

If the worker modifies:

- test commands,
- test scripts,
- CI configuration,
- acceptance fixtures,
- golden files,
- generated validators,

the verification engine must detect the change.

Depending on task authority:

- reject,
- run pre-change and post-change checks,
- require specialist audit,
- require operator approval.

## 18.5 Changed-File Manifest

The host captures actual changed paths from Git.

The worker’s changed-file claim is compared to the host manifest.

Mismatches become audit signals.

## 18.6 Completion Capsule

A completion capsule includes:

```text
task identity and version
plan identity
criterion outcomes
source before/after
commit/tree
diff hash
verification summary
audit summary
finding dispositions
container image/runtime profile
prompt/model versions
artifact manifest
cost/token summary
operator inputs
```

## 18.7 False Completion Prevention

A model-produced `complete` decision must be rejected if any of the following hold:

- plan missing,
- plan step nonterminal,
- criterion nonterminal,
- verification stale,
- verification failed,
- audit missing,
- audit stale,
- audit changes required,
- blocking finding open,
- source revision changed,
- budget state invalid,
- workspace unreconciled,
- artifact manifest incomplete.

---

# 19. Strategy and Failure Memory

## 19.1 Failure Signature

Normalize failures using:

- failed command,
- exit code,
- stable error excerpts,
- failed test IDs,
- affected component,
- source revision,
- finding IDs.

## 19.2 Strategy Fingerprint

A retry strategy includes:

```text
approach
techniques
target files/symbols
expected evidence
```

Normalize and hash its semantic structure.

## 19.3 Repeat Prevention

If the same strategy is proposed against the same failure with no material evidence change, policy should deny the retry or require operator input.

## 19.4 No-Progress Detection

Signals:

- repeated identical diff,
- repeated failure signature,
- no changed files,
- unchanged plan,
- no new evidence,
- circular tool calls,
- repeated reads without action,
- budget consumption without state progress.

A no-progress outcome stops the cycle safely.

## 19.5 Cross-Task Learning

Store relationships such as:

```text
strategy FAILED_WITH failure
commit RESOLVED finding
decision SUPERSEDES decision
test VALIDATED criterion
```

Retrieval can surface this history on future related tasks.

---

# 20. Human Checkpoints

## 20.1 Checkpoint Examples

Require explicit input for:

- architecture changes,
- database migration policy changes,
- security model changes,
- destructive deletion,
- dependency replacement with broad impact,
- network/secret escalation,
- release/deployment,
- ambiguous product behavior,
- large scope expansion.

## 20.2 Checkpoint Contract

A checkpoint records:

```text
question
options
recommendation
blocking reason
evidence
operator answer
operator identity label
accepted at
content hash
```

## 20.3 No Implicit Approval

Silence is never approval.

A recommendation is never automatically selected.

---

# 21. Sequential Queue

## 21.1 Queue Rules

- One active task.
- Stable operation ID.
- Bounded maximum tasks.
- Bounded cycles per task.
- Bounded total tokens/cost/time.
- Re-evaluate dependencies after each task.
- Yield blocked tasks.
- Stop on unsafe ambiguity.
- Preserve deterministic selection order.

## 21.2 Queue Stop Reasons

```text
drained
waiting_on_dependencies
waiting_on_input
all_remaining_blocked
budget_exhausted
cancelled
unsafe
system_failure
```

## 21.3 No Daemon in v1

The queue is started explicitly.

A daemon may be considered only after:

- recovery tests pass,
- false-completion metrics are acceptable,
- sandbox escape risk is reviewed,
- queue dogfood is stable,
- operator notifications exist.

---

# 22. Observability and Metrics

## 22.1 Per-Run Timeline

Display:

- admission,
- context assembly,
- supervisor decision,
- worker start/end,
- tool calls,
- source mutations,
- verification,
- audit,
- correction,
- completion.

## 22.2 Core Metrics

```text
tasks attempted
tasks completed
tasks blocked
tasks needing input
false completion attempts
verification failure rate
audit finding rate
average correction cycles
repeated strategy denials
human intervention rate
tokens per completed task
tokens per completed criterion
wall time per task
context hit rate
reactive search rate
unnecessary file read rate
sandbox failures
recovery events
```

## 22.3 Context Quality

After a role invocation, record whether it had to retrieve files or facts absent from the proactive dossier.

This measures retrieval effectiveness.

## 22.4 Cost Ledger

Track:

- model input/output/cached tokens,
- estimated cost,
- local GPU time,
- sandbox CPU/memory time,
- artifact growth.

---

# 23. Evaluation System

## 23.1 Deterministic Fixture Scenarios

Create fixture repositories for:

1. Straight success.
2. Compile failure and correction.
3. Test failure and correction.
4. Audit finding and correction.
5. Ambiguous product requirement.
6. Missing dependency.
7. Dependency cycle.
8. Scope violation.
9. Protected-path change.
10. Repeated failed strategy.
11. No source changes.
12. Test tampering.
13. Source revision changes mid-run.
14. Cancellation.
15. Crash during state transition.
16. Crash after external effect.
17. Stale retrieval index.
18. Missing embedding service.
19. Sandbox timeout.
20. Network-denied dependency install.

## 23.2 Live Dogfood

Live model dogfood remains opt-in and records exact:

- source commit,
- model policy,
- prompt versions,
- sandbox profile,
- task contract,
- outcome.

## 23.3 Quality Gates

Before enabling sequential queue autonomy on real projects, require acceptable thresholds for:

- false completion,
- unrecovered crashes,
- repeated strategy loops,
- scope violations,
- host safety,
- task completion.

Exact thresholds should be set after baseline data exists.

---

# 24. UI and CLI

## 24.1 CLI Commands

Suggested shape:

```text
revolvr init
revolvr up
revolvr down
revolvr doctor

revolvr project add
revolvr project list
revolvr project inspect
revolvr project index
revolvr project verify-baseline

revolvr task compile
revolvr task import
revolvr task approve
revolvr task list
revolvr task show
revolvr task why
revolvr task run
revolvr task cancel
revolvr task answer

revolvr queue start
revolvr queue status
revolvr queue cancel

revolvr run show
revolvr evidence show
revolvr artifact show
revolvr audit show
revolvr context show

revolvr backup create
revolvr backup verify
revolvr restore
revolvr eval run
```

## 24.2 Desktop UI

Primary views:

- Dashboard.
- Projects.
- Task backlog.
- Task compiler/review.
- Active run.
- Plan and criteria.
- Diff.
- Verification.
- Audit findings.
- Context inspector.
- Evidence browser.
- Model usage.
- System health.
- Settings.

## 24.3 No Hidden UI State

All task/run state displayed by the UI comes from canonical APIs.

The UI must not infer lifecycle truth from local component state.

---

# 25. Local API Security

Even without user accounts:

- bind to loopback or Unix socket,
- generate a local installation secret,
- require it for mutating API calls,
- protect against browser-origin request abuse,
- reject external binding by default,
- do not expose PostgreSQL publicly,
- do not expose the container runtime API over TCP.

---

# 26. Backup, Retention, and Recovery

## 26.1 Backups

A backup includes:

- PostgreSQL dump,
- artifact manifest,
- artifact files,
- project configuration,
- prompt/schema versions,
- optional managed Git repositories.

## 26.2 Backup Verification

A backup is not complete until:

- database dump restores into a temporary database,
- artifact hashes verify,
- required commits exist,
- completion capsules resolve.

## 26.3 Retention

Retention classes:

```text
permanent
task_lifetime
recent
diagnostic
cache
```

Never prune:

- accepted task sources,
- final verification,
- final audit,
- completion capsules,
- operator answers,
- final diffs,
- source/commit identities.

## 26.4 Crash Recovery

Every multi-stage operation should have:

- stable operation ID,
- expected prior state,
- immutable transition records,
- idempotent resume,
- explicit terminal marker.

Recovery must reconcile actual external effects rather than trusting a mutable checkpoint alone.

---

# 27. Graph Memory and Graphiti Roadmap

## 27.1 What Exists First

PostgreSQL stores deterministic relationships from ordinary operation.

This already supports many useful questions:

- Which commit resolved this finding?
- Which test validates this criterion?
- Which tasks depend on this task?
- Which files changed during this strategy?
- Which decision superseded another decision?

## 27.2 When Graphiti Becomes Worthwhile

Consider Graphiti after Revolvr has accumulated enough history that relational queries and hybrid retrieval struggle with:

- entity aliases,
- temporal fact supersession,
- cross-document multi-hop reasoning,
- repeated architectural evolution,
- large volumes of decisions and failure history.

## 27.3 Dogfood Integration Shape

```text
Canonical PostgreSQL + artifacts
            |
            v
Graph projection worker
            |
            v
Graphiti / graph database
            |
            v
Optional retrieval lane
```

## 27.4 Graphiti Authority

Graphiti output is candidate context.

Every returned fact must include source provenance.

Graphiti cannot:

- mutate task state,
- satisfy criteria,
- resolve findings,
- authorize actions,
- complete tasks.

## 27.5 Python Policy

Python services are allowed behind versioned interfaces when they add measurable value.

Potential Python services:

- Graphiti projection,
- reranking,
- embedding evaluation,
- specialized parsing,
- offline analytics.

They do not own canonical state.

---

# 28. Reuse Map from Existing Projects

## 28.1 Revolvr Concepts to Preserve

Preserve or adapt:

- durable task identity,
- task dependency validation,
- sequential selection,
- fresh sessions,
- supervisor decision boundary,
- deterministic lifecycle routing,
- exact evidence references,
- attempts and budgets,
- needs-input questions,
- operator checkpoints,
- workspace isolation,
- safety preflight,
- verification,
- audit/correction,
- completion capsules,
- append-only ledgers/events,
- restartable operations,
- deterministic metrics,
- archive concepts,
- strategy retry discipline.

## 28.2 Sodoryard Concepts to Preserve

Preserve or adapt:

- provider abstraction, narrowed to OpenAI,
- model/tool iteration loop,
- role prompts,
- code indexing,
- Tree-sitter parsing,
- structural code graph,
- semantic chunking,
- context assembly,
- explicit-file priority,
- context token budgeting,
- context reports,
- brain/project knowledge concepts,
- tool registry design,
- changed-file guardrails,
- desktop operator experience.

## 28.3 Revolvr Code Not to Preserve Blindly

Do not automatically port:

- release-candidate machinery unrelated to the new core,
- broad historical compatibility layers,
- parallel queue workers,
- daemon mode,
- legacy task formats that conflict with the new contract,
- duplicated runtime journals where PostgreSQL transactions suffice.

## 28.4 Sodoryard Code Not to Preserve

Do not port:

- Shunter integration,
- Shunter RPC,
- Shunter generated clients,
- LanceDB,
- multi-provider configuration,
- arbitrary chain orchestration as the autonomous core,
- thirteen mandatory roles,
- parallel agent spawning,
- SQLite canonical storage,
- duplicate REST/subscription pathways.

---

# 29. Implementation Phases

Each phase should be decomposed into epics and bounded tasks.

## Phase 0 — Architecture and Repository Baseline

Deliverables:

- ADR set.
- Repository structure.
- Go module.
- build/test commands.
- coding conventions.
- task-generation conventions.
- threat model.
- configuration schema.
- CI baseline.
- decision on in-place Revolvr evolution versus clean branch.

Definition of done:

- Architecture decisions are tracked.
- No Shunter dependency.
- No LanceDB dependency.
- PostgreSQL development environment starts.
- Full test command exists.

## Phase 1 — PostgreSQL Foundation

Deliverables:

- Compose PostgreSQL + pgvector.
- `pgx` connection layer.
- `sqlc`.
- migration framework.
- core schemas.
- transaction helper.
- event table.
- artifact metadata table.
- integration-test harness using ephemeral PostgreSQL.

Definition of done:

- Empty database migrates.
- Previous migration upgrades.
- sqlc generation is repeatable.
- transaction/event write is tested.
- backup/restore smoke passes.

## Phase 2 — Project and Task Core

Deliverables:

- project registry,
- managed clone/mirror,
- task source artifacts,
- task compiler schema,
- task validation,
- task approval,
- dependencies/conflicts,
- sequential scheduler,
- lifecycle engine,
- CLI surfaces.

Definition of done:

- A Markdown task imports.
- A compiled task is reviewed and approved.
- Invalid graphs fail closed.
- One ready task is selected deterministically.

## Phase 3 — Sandbox and Workspace

Deliverables:

- `revolvr-sandboxd`,
- rootless runtime adapter,
- sandbox spec,
- path validator,
- managed worktrees,
- resource limits,
- network profiles,
- artifact collection,
- cancellation,
- cleanup,
- strict/compatible profiles.

Definition of done:

- Worker cannot access host home.
- Worker cannot access runtime socket.
- Worker cannot mount arbitrary paths.
- timeout cleanup works.
- source diff is captured.
- original checkout is untouched.

## Phase 4 — OpenAI Agent Runtime

Deliverables:

- OpenAI client,
- Responses API streaming,
- structured outputs,
- model registry,
- prompt versioning,
- tool broker,
- fresh sessions,
- supervisor,
- planner,
- implementer.

Definition of done:

- A supervisor produces one validated action.
- A planner produces a validated plan.
- An implementer uses brokered tools in a sandbox.
- API secrets never enter worker environment.
- model usage is recorded.

## Phase 5 — Verification and Evidence

Deliverables:

- baseline verification,
- tiered verification,
- fresh verifier sandbox,
- artifact capture,
- claims/evidence,
- changed-file reconciliation,
- verification tamper detection,
- completion preconditions.

Definition of done:

- A worker claim cannot complete a task.
- Failed verification routes to correction state.
- final verification binds exact source/image.
- artifacts hash and verify.

## Phase 6 — Audit and Correction Loop

Deliverables:

- auditor,
- findings,
- finding lifecycle,
- corrector,
- strategy fingerprints,
- failure signatures,
- no-progress detector,
- re-verification,
- re-audit.

Definition of done:

- Audit findings are structured.
- Correction is bound to exact findings.
- repeated identical strategies are rejected.
- task completes only after clean audit.

## Phase 7 — Retrieval and pgvector

Deliverables:

- code parser/chunker,
- PostgreSQL FTS,
- pgvector extension/query layer,
- local embedding service,
- embedding evaluation,
- exact symbol lookup,
- structural graph,
- hybrid ranking,
- context compiler,
- role dossiers,
- context reports.

Definition of done:

- Retrieval evaluation baseline exists.
- Relevant code is retrieved for fixture queries.
- index freshness is tracked.
- stale embedding service degrades safely.
- LanceDB is absent.

## Phase 8 — Project Memory and Relationships

Deliverables:

- documents,
- document versions,
- conventions,
- decisions,
- entities,
- relations,
- provenance,
- completion history documents,
- memory search.

Definition of done:

- Accepted decisions are retrievable.
- supersession is represented.
- evidence sources resolve.
- memory cannot mutate lifecycle authority.

## Phase 9 — Desktop UI and Observability

Deliverables:

- Wails/Vue application,
- dashboard,
- task review,
- active run,
- diff,
- verification,
- audit,
- context inspector,
- evidence browser,
- SSE progress,
- health diagnostics.

Definition of done:

- All core CLI state is visible.
- UI contains no unique lifecycle logic.
- active runs stream progress.
- operator can answer needs-input questions.

## Phase 10 — Sequential Queue and Recovery Hardening

Deliverables:

- bounded queue,
- operation IDs,
- queue resume,
- yielded-task handling,
- crash injection tests,
- cancellation,
- backup/restore,
- retention.

Definition of done:

- Multiple tasks run sequentially.
- blocked tasks do not starve unrelated work.
- crash recovery is deterministic.
- no parallel workers exist.

## Phase 11 — Graphiti Dogfood Experiment

Entry criteria:

- substantial real run history,
- baseline retrieval metrics,
- clear multi-hop retrieval failures.

Deliverables:

- optional Python projection service,
- Graphiti adapter,
- source-grounded graph retrieval,
- A/B evaluation,
- removal path.

Definition of done:

- graph retrieval measurably improves selected evals,
- every fact carries provenance,
- system works without Graphiti,
- no canonical authority moved into graph storage.

## Phase 12 — Unattended Daemon Consideration

This is not automatically approved.

Entry criteria:

- queue dogfood stability,
- acceptable false-completion rate,
- strong sandbox confidence,
- reliable notifications,
- proven recovery.

---

# 30. v1 Definition of Done

Revolvr v1 is complete when it can:

1. Register a local Git project.
2. Maintain a managed project copy.
3. Import or compile a bounded task.
4. Validate and approve the task.
5. Select one ready task.
6. Pin source, task, prompt, policy, model, and image identities.
7. Run a planner when needed.
8. Run an implementer through brokered tools in a disposable sandbox.
9. Capture the actual diff.
10. Run deterministic verification in a fresh sandbox.
11. Run an independent audit.
12. Run bounded correction cycles.
13. Stop on ambiguity, budget, cancellation, or unsafe state.
14. Produce a completion capsule only when all gates pass.
15. Persist complete evidence in PostgreSQL and the artifact store.
16. Retrieve relevant code and project knowledge through FTS and pgvector.
17. Operate entirely sequentially.
18. Recover safely from a crash during a run.
19. Leave the operator’s original checkout untouched unless explicit export is requested.
20. Be fully operable through the CLI.

---

# 31. Key Risks and Mitigations

## Risk: Overengineering before dogfood

Mitigation:

- phase gates,
- CLI first,
- no Graphiti initially,
- no daemon initially,
- no parallelism,
- no multi-provider layer.

## Risk: PostgreSQL operational overhead

Mitigation:

- Compose-managed local instance,
- one database,
- tested backup/restore,
- simple migrations,
- DataGrip-friendly loopback option.

## Risk: Container escape or host corruption

Mitigation:

- rootless runtime,
- strict gVisor profile,
- no original checkout mount,
- no runtime socket in workers,
- narrow sandbox manager,
- no ambient secrets,
- path validation,
- resource limits.

## Risk: Model falsely claims success

Mitigation:

- host-owned verification,
- independent audit,
- evidence-bound criteria,
- completion finalizer,
- source revision binding.

## Risk: Agent changes tests to make work pass

Mitigation:

- baseline verification,
- verification-authority hashing,
- changed-file risk routing,
- independent audit,
- protected-path policy.

## Risk: Retrieval injects stale or irrelevant information

Mitigation:

- provenance,
- index freshness,
- deterministic source priority,
- context reports,
- retrieval evals,
- bounded graph expansion.

## Risk: Endless correction loops

Mitigation:

- cycle budgets,
- strategy fingerprints,
- failure signatures,
- no-progress detector,
- operator escalation.

## Risk: Model/API changes over time

Mitigation:

- role-based model registry,
- prompt/schema versioning,
- evaluation before model changes,
- model-independent state.

## Risk: Local embedding model becomes outdated

Mitigation:

- embedding-space metadata,
- evaluation suite,
- controlled full reindex,
- adapter-based service.

## Risk: Existing project code is too environment-specific

Mitigation:

- project environment contract,
- diagnostic sandbox mode,
- explicit compatibility profile,
- baseline verification.

---

# 32. Open Questions with Default Decisions

## Q1. Continue the current Revolvr repository or start clean?

**Default:** Continue the Revolvr name and repository, but treat the new architecture as a versioned rewrite. Port code selectively; do not preserve compatibility merely for compatibility’s sake.

## Q2. Docker or Podman?

**Default:** Define an interface and implement whichever rootless runtime is most reliable on the workstation first. Do not implement both initially.

## Q3. Is gVisor mandatory?

**Default:** Mandatory for strict mode, optional compatibility fallback for projects that cannot run under it.

## Q4. Which local embedding model?

**Default:** Select through a project-specific retrieval evaluation rather than hard-coding the old Sodoryard model.

## Q5. Which PostgreSQL major version?

**Default:** Pin a currently supported version when Phase 1 begins. Do not encode a moving “latest” version in this architecture specification.

## Q6. Which OpenAI model per role?

**Default:** Configuration and evaluation determine this. Do not make model names part of the domain model.

## Q7. Is the desktop UI required for v1?

**Default:** No. CLI completion is the v1 gate. Desktop UI follows after the core loop is trustworthy.

## Q8. Does Revolvr automatically export successful commits?

**Default:** Create the commit in the managed repository. Export/push into the operator’s repository is explicit.

## Q9. Should task files be tracked in target repositories?

**Default:** Support portable Markdown export/import, but keep canonical execution state in PostgreSQL.

## Q10. Should Graphiti be included from day one?

**Default:** No. Preserve a clean projection interface and dogfood it later.

---

# 33. Task Generation Rules

When generating implementation tasks from this specification:

1. One task must have one primary outcome.
2. Every task must identify the relevant specification section.
3. Every task must include acceptance criteria.
4. Every task must include deterministic verification.
5. Do not combine architecture, implementation, UI, and migration in one task.
6. Create interfaces before multiple implementations.
7. Create fixture-based tests alongside domain state machines.
8. Prefer vertical slices that can be dogfooded.
9. Do not generate Graphiti, daemon, or parallel-worker tasks before their phase gates.
10. Do not introduce Shunter, LanceDB, SQLite, or multi-user concerns.
11. Every security-sensitive task must include abuse-case tests.
12. Every lifecycle task must include illegal-transition tests.
13. Every external-effect task must include idempotency/recovery tests.
14. Every model-output task must include malformed/refusal/schema tests.
15. Every sandbox task must include path traversal and unsafe-mount tests.
16. Every database task must include migration and transaction tests.
17. Every retrieval task must include quality fixtures, not only unit tests.
18. Every completion task must include false-completion rejection tests.

---

# 34. Suggested Initial Epic Breakdown

## Epic A — Canonical Decisions and Clean Build

- Add ADRs.
- Remove Shunter dependencies.
- Remove LanceDB dependencies.
- Establish Go module and CI.
- Establish configuration and logging.
- Establish coding and testing conventions.

## Epic B — PostgreSQL and sqlc

- Compose PostgreSQL/pgvector.
- Connection and health.
- Migrations.
- sqlc generation.
- transaction/event primitive.
- integration tests.
- backup/restore smoke.

## Epic C — Projects and Managed Git

- project registration,
- repository validation,
- managed clone,
- snapshot identity,
- worktree lifecycle,
- patch export.

## Epic D — Tasks and Lifecycle

- task schema,
- import,
- compilation,
- approval,
- dependencies,
- scheduler,
- lifecycle,
- policy.

## Epic E — Sandbox

- sandboxd,
- runtime adapter,
- strict spec,
- rootless execution,
- mount validation,
- network policy,
- cancellation,
- cleanup.

## Epic F — Agent Runtime

- OpenAI client,
- streaming,
- structured output,
- prompt registry,
- model registry,
- tool broker,
- supervisor/planner/implementer.

## Epic G — Evidence and Verification

- artifacts,
- claims,
- baseline checks,
- verifier sandbox,
- changed-file manifest,
- tamper detection,
- completion gate.

## Epic H — Audit and Correction

- audit schema,
- findings,
- corrector,
- strategy/failure memory,
- no-progress detection,
- re-audit.

## Epic I — Retrieval

- code parsing,
- FTS,
- pgvector,
- local embeddings,
- hybrid search,
- context packages,
- evals.

## Epic J — Operator Experience

- CLI,
- SSE,
- Wails/Vue UI,
- context/evidence inspector,
- system diagnostics.

## Epic K — Sequential Queue and Recovery

- queue,
- operation identity,
- resume,
- crash injection,
- retention,
- backups.

---

# 35. Glossary

**Acceptance criterion** — A specific requirement that must receive a terminal evidence-backed disposition.

**Artifact** — Immutable content-addressed bytes produced or consumed by a run.

**Audit** — Independent model review performed after deterministic verification.

**Canonical state** — Authoritative PostgreSQL state used for lifecycle and completion decisions.

**Completion capsule** — Final immutable evidence bundle proving why a task was accepted as complete.

**Context package / dossier** — Frozen role-specific context supplied to one model invocation.

**Control plane** — Trusted Go services that own policy, state, model calls, and orchestration.

**Derived projection** — Rebuildable search or graph data that does not own lifecycle truth.

**Embedding space** — A specific embedding model/revision/dimension configuration.

**Evidence** — A source-revision-bound fact or artifact used to prove a claim.

**Failure signature** — Stable normalized representation of a verification or execution failure.

**Finding** — Structured audit concern requiring disposition.

**Managed repository** — Revolvr-controlled clone or mirror used instead of mutating the operator’s original checkout.

**No-progress** — Bounded outcome where additional cycles are not producing materially new evidence.

**Policy engine** — Deterministic authorization layer for proposed actions.

**Sandbox** — Disposable rootless container environment used for untrusted execution.

**Strategy fingerprint** — Normalized identity of a correction or implementation approach.

**Task compiler** — System that converts prose into validated executable task contracts.

**Worker** — Role invocation that may inspect or mutate only its admitted isolated workspace.

---

# 36. Final Architectural Statement

Revolvr is a local, single-user, sequential autonomous engineering harness.

Its durable core is:

```text
PostgreSQL
Go
sqlc
explicit state machines
evidence
sandboxed execution
deterministic verification
independent audit
```

Its intelligence layer is:

```text
OpenAI reasoning
local embeddings
hybrid retrieval
project memory
typed relationships
```

Its trust model is:

```text
Models propose.
Policies constrain.
Sandboxes contain.
Verification proves.
Audits challenge.
Evidence persists.
The host decides.
```

That principle should remain the deciding criterion whenever a future subsystem, framework, model, database, or agent pattern is considered.


---

# 37. Functional Requirements

The following requirements should be assigned stable IDs when converted into implementation tracking. They are grouped by subsystem and may be used directly as epic acceptance criteria.

## 37.1 Project Management Requirements

### FR-PROJ-001 — Register a project

The operator can register a local Git repository by path.

The system must:

- resolve the canonical path,
- verify the path is a Git repository,
- identify the current commit and branch,
- identify configured remotes,
- reject duplicate aliases of the same repository,
- create a Revolvr-managed repository identity.

### FR-PROJ-002 — Create a managed repository

Revolvr creates and owns a mirror or clone under its managed data root.

The managed repository must be isolated from the operator’s normal working checkout.

### FR-PROJ-003 — Refresh project source

The operator can explicitly fetch or refresh the managed repository.

Automatic fetch may be configurable, but no hidden network fetch should occur during a network-disabled task.

### FR-PROJ-004 — Pin source identity

Every run stores:

- source commit,
- source tree,
- branch/ref source,
- managed repository identity,
- dirty-state status,
- submodule state where applicable.

### FR-PROJ-005 — Project environment contract

Each project may declare:

- worker image,
- verifier image,
- toolchain versions,
- verification commands,
- network requirements,
- cache mounts,
- guidance paths,
- protected paths,
- indexing include/exclude patterns.

### FR-PROJ-006 — Baseline health

The operator can run a project baseline verification without starting a task.

### FR-PROJ-007 — Project removal

Removing a project registration must not silently delete:

- task history,
- artifacts,
- completion evidence,
- managed commits.

Destructive cleanup must be a separate explicit operation.

## 37.2 Task Intake and Compilation Requirements

### FR-TASK-001 — Import task source

The operator can import:

- prose,
- Markdown,
- a specification file,
- a task bundle.

The source bytes are stored immutably.

### FR-TASK-002 — Compile task contract

The task compiler produces a versioned structured task contract.

### FR-TASK-003 — Validate scope

The compiler must distinguish:

- in-scope behavior,
- excluded behavior,
- assumptions,
- unresolved questions.

### FR-TASK-004 — Validate acceptance criteria

A task cannot be approved without at least one acceptance criterion unless it is explicitly classified as a read-only investigation.

### FR-TASK-005 — Map verification

Each acceptance criterion should have a supported verification method or an explicit operator checkpoint.

### FR-TASK-006 — Decompose large requests

A compiler may propose multiple tasks when the requested change cannot be safely bounded as one unit.

The decomposition must preserve:

- parent source identity,
- dependency relationships,
- non-overlapping scope where possible,
- original acceptance intent.

### FR-TASK-007 — Operator approval

Compiled tasks do not enter the runnable queue until approved.

### FR-TASK-008 — Immutable task versions

Editing an approved task creates a new task version.

An active run remains pinned to the version it admitted.

### FR-TASK-009 — Dependency validation

Missing references, duplicate edges, self-dependencies, cycles, and ambiguous active/archive identities fail closed.

### FR-TASK-010 — Conflict validation

Explicit task conflicts are checked before admission even though execution is sequential, because conflicts may indicate invalid queue ordering or stale assumptions.

## 37.3 Scheduling Requirements

### FR-SCHED-001 — Single active mutation

At most one source-mutating run may be active globally in v1.

A future revision may scope this per project, but no parallel execution is allowed under this specification.

### FR-SCHED-002 — Deterministic selection

Given identical canonical state, task selection returns the same task.

### FR-SCHED-003 — Pin selected task

Once admitted, a queue operation cannot replace the selected task with another task mid-run.

### FR-SCHED-004 — Yield blocked work

A blocked or needs-input task may be yielded so an unrelated ready task can run later in the same bounded queue.

### FR-SCHED-005 — Stable queue operation

A queue operation has a stable operation ID and pinned limits.

### FR-SCHED-006 — Bounded execution

The queue enforces:

- maximum tasks,
- maximum cycles,
- maximum wall time,
- maximum remote model tokens,
- maximum estimated API cost.

### FR-SCHED-007 — Safe stop

Unsafe or ambiguous canonical state stops the entire queue.

## 37.4 Lifecycle and Policy Requirements

### FR-LIFE-001 — Legal transitions only

Every transition is validated against an explicit state machine.

### FR-LIFE-002 — Optimistic concurrency

A transition must include the expected current version or hash.

### FR-LIFE-003 — Atomic state and event

Current state and append-only event are committed atomically.

### FR-LIFE-004 — Model output is advisory

No model output directly updates current state.

### FR-LIFE-005 — Decision provenance

Accepted supervisor decisions record:

- prompt identity,
- dossier identity,
- model identity,
- schema identity,
- source revision,
- decision hash.

### FR-LIFE-006 — Illegal decision handling

An illegal but validly formatted model decision is persisted as rejected evidence and does not advance state.

### FR-LIFE-007 — Budget-aware policy

Policy denies new model or worker work after relevant budget exhaustion.

### FR-LIFE-008 — Risk-aware policy

High-risk mutation classes require stronger verification and audit policies.

## 37.5 Workspace and Sandbox Requirements

### FR-SBX-001 — Managed worktree

Every source-mutating run receives a dedicated worktree created by trusted host code.

### FR-SBX-002 — No original checkout mutation

The worker cannot write to the operator’s original checkout.

### FR-SBX-003 — Typed sandbox request

The control plane sends `sandboxd` a versioned typed request.

### FR-SBX-004 — Path allowlist

Every mount source must be under an approved managed root.

### FR-SBX-005 — Image allowlist

Only configured image digests may run.

### FR-SBX-006 — Runtime profile evidence

The selected runtime and sandbox profile are recorded.

### FR-SBX-007 — No runtime socket

Worker containers never receive the Docker, Podman, containerd, or CRI socket.

### FR-SBX-008 — Bounded resources

Every sandbox has explicit CPU, memory, PIDs, disk, and time constraints.

### FR-SBX-009 — Network default deny

Network access defaults to none.

### FR-SBX-010 — Cleanup

Terminal sandbox states trigger container and temporary-mount cleanup.

### FR-SBX-011 — Orphan reconciliation

Startup diagnostics detect and reconcile orphaned Revolvr containers and workspaces.

### FR-SBX-012 — Symlink defense

Managed-path validation rejects symlink substitution and path traversal.

### FR-SBX-013 — Fresh verifier environment

Final verification runs in a separate fresh container.

## 37.6 Agent Runtime Requirements

### FR-MODEL-001 — Fresh invocation

Every supervisor, planner, implementer, auditor, and corrector call is a fresh model invocation.

### FR-MODEL-002 — Structured output

Decision-oriented roles return JSON conforming to versioned JSON schemas.

### FR-MODEL-003 — Refusal handling

Model refusals are detected and persisted as typed outcomes.

### FR-MODEL-004 — Streaming

Long-running calls stream progress without making partial text canonical.

### FR-MODEL-005 — Tool validation

Every tool call is validated before dispatch.

### FR-MODEL-006 — No credential exposure

Remote API credentials remain in trusted control-plane memory.

### FR-MODEL-007 — Usage recording

Token, latency, caching, and cost metadata are recorded.

### FR-MODEL-008 — Role-specific limits

Each role has separate output, reasoning, timeout, and tool limits.

### FR-MODEL-009 — Prompt immutability

Prompt versions are immutable once referenced by a run.

### FR-MODEL-010 — Retry classification

Transport or service retries are distinguished from semantic/correctness retries.

## 37.7 Tool Requirements

### FR-TOOL-001 — Read tools

The agent can inspect permitted source and repository state.

### FR-TOOL-002 — Mutation tools

Only mutation-capable roles receive write tools.

### FR-TOOL-003 — Exact working directory

Every command executes in a validated container path.

### FR-TOOL-004 — Direct argv preference

Commands should use executable and argument arrays instead of an implicit shell where practical.

### FR-TOOL-005 — Output artifact spill

Truncated output is retained as a complete artifact when policy permits.

### FR-TOOL-006 — Command evidence

The exact command specification and result are recorded.

### FR-TOOL-007 — Protected path enforcement

Protected path changes fail before final acceptance.

### FR-TOOL-008 — Duplicate read elision

Repeated identical file reads may be summarized or elided from model history while remaining in telemetry.

## 37.8 Verification Requirements

### FR-VER-001 — Versioned verification plan

Verification configuration is versioned and pinned to the run.

### FR-VER-002 — Baseline support

The system supports pre-change baseline verification.

### FR-VER-003 — Focused and full checks

Verification plans may contain ordered tiers.

### FR-VER-004 — Structured test parsing

When available, JUnit, JSON, Go test JSON, or equivalent structured output is parsed.

### FR-VER-005 — Raw output retention

Raw stdout/stderr remains available as artifacts.

### FR-VER-006 — Source binding

Verification evidence names the exact commit/tree tested.

### FR-VER-007 — Environment binding

Verification evidence names the exact image and sandbox profile.

### FR-VER-008 — Tamper detection

Changes to admitted verification inputs are detected.

### FR-VER-009 — Timeout classification

Timeout is not treated as a passing result.

### FR-VER-010 — Flake classification

A rerun policy may classify flaky checks, but a later pass cannot erase evidence of a prior failure.

## 37.9 Audit Requirements

### FR-AUD-001 — Independent role

The implementer cannot be accepted as the only auditor of its own work.

### FR-AUD-002 — Evidence-backed findings

Every finding cites exact source or verification evidence.

### FR-AUD-003 — Finding significance

Findings are blocking or non-blocking.

### FR-AUD-004 — Finding lifecycle

Findings remain open until resolved, waived, rejected, superseded, or proven stale.

### FR-AUD-005 — Freshness

An audit is stale when the source revision changes.

### FR-AUD-006 — Conditional specialist routing

Risk-specific audits are selected deterministically.

## 37.10 Correction Requirements

### FR-CORR-001 — Exact correction authority

A corrector receives exact findings or one exact verification failure.

### FR-CORR-002 — Strategy declaration

The corrector declares a structured strategy.

### FR-CORR-003 — Repeat detection

Materially identical failed strategies are denied.

### FR-CORR-004 — Bounded retries

Correction cycles are bounded.

### FR-CORR-005 — Full re-verification

A corrected source revision receives fresh verification.

## 37.11 Evidence and Completion Requirements

### FR-EVID-001 — Content-addressed artifacts

Artifact bytes are named and verified by SHA-256.

### FR-EVID-002 — Provenance links

Artifacts link to the run, role, task, source revision, and producing operation.

### FR-EVID-003 — Claim model

Acceptance claims can be linked to one or more evidence sources.

### FR-EVID-004 — Completion preflight

The finalizer performs a read-only preflight before mutation.

### FR-EVID-005 — Transactional completion

Completion state and final evidence references are committed atomically.

### FR-EVID-006 — Human-readable capsule

Every completed task has a readable Markdown summary.

### FR-EVID-007 — Machine-readable capsule

Every completed task has a versioned JSON evidence record.

### FR-EVID-008 — Manifest

The final capsule has a manifest of exact file hashes.

## 37.12 Retrieval Requirements

### FR-RET-001 — Exact sources first

Explicit file and symbol references bypass fuzzy retrieval.

### FR-RET-002 — Incremental indexing

Only changed files are reprocessed after ordinary commits.

### FR-RET-003 — Full rebuild

A complete reproducible index rebuild is available.

### FR-RET-004 — Hybrid retrieval

Keyword and semantic results are combined transparently.

### FR-RET-005 — Provenance

Every context item identifies its source.

### FR-RET-006 — Role budgets

Each role has a separate context budget.

### FR-RET-007 — Context manifest

Included and excluded retrieval items are stored.

### FR-RET-008 — Degraded mode

Missing embeddings do not disable direct file and lexical search.

### FR-RET-009 — Evaluation

Retrieval changes must be measured against fixture queries.

## 37.13 Local Model Requirements

### FR-LOCAL-001 — Containerized service

The local embedding model runs in a dedicated container.

### FR-LOCAL-002 — GPU isolation

The embedding service has no repository mount.

### FR-LOCAL-003 — Model metadata

The system records exact model and revision metadata.

### FR-LOCAL-004 — Health and fallback

Unavailable local embeddings produce a clear degraded state.

### FR-LOCAL-005 — Reindex on model change

Changing the active embedding space requires controlled reindex.

## 37.14 Operator Requirements

### FR-OP-001 — Explain current state

`task why` explains why a task is or is not runnable and what decision is required next.

### FR-OP-002 — Inspect evidence

The operator can inspect all completion evidence.

### FR-OP-003 — Answer questions

The operator can answer a current needs-input question.

### FR-OP-004 — Cancel work

The operator can cancel a task or queue.

### FR-OP-005 — No hidden mutation

Every source or task-state mutation is visible in the event timeline.

---

# 38. Non-Functional Requirements

## NFR-001 — Maintainability

A single experienced Go developer should be able to understand and maintain the core system without learning a custom database/runtime product.

## NFR-002 — Recoverability

A process crash must not leave canonical state silently ahead of or behind external effects.

## NFR-003 — Portability

Core state can be backed up and restored using standard PostgreSQL and filesystem tools.

## NFR-004 — Local privacy

Project source and embeddings remain local except for context intentionally sent to OpenAI.

## NFR-005 — Bounded resource use

Every model call, sandbox, queue, and artifact operation has configured bounds.

## NFR-006 — Determinism

Given identical canonical state and no model call, scheduling, validation, state projection, and verification planning are deterministic.

## NFR-007 — Auditability

Every accepted state transition can be traced to input state, decision, evidence, and policy version.

## NFR-008 — Graceful degradation

Optional retrieval and graph services may fail without corrupting canonical state.

## NFR-009 — No silent fallback

Security, model, sandbox, or verification downgrades require explicit configuration or operator acknowledgement.

## NFR-010 — Testability

External systems are behind narrow interfaces, with deterministic fakes for state-machine testing.

## NFR-011 — Data integrity

Database constraints should enforce domain invariants where practical.

## NFR-012 — Performance

The system is optimized for correctness on one workstation, not high request throughput.

## NFR-013 — Storage efficiency

Large repeated text and binary artifacts are deduplicated by content hash.

## NFR-014 — Long-term model independence

Historical tasks remain understandable if the original model is no longer available.

## NFR-015 — Backward migration discipline

Schema and artifact-format changes include migration or explicit compatibility policy.

---

# 39. Detailed State Transitions

## 39.1 Task Transition Matrix

| Current State | Proposed Next State | Authority | Key Preconditions |
|---|---|---|---|
| `draft` | `compiled` | task compiler + host validator | valid compiled contract |
| `compiled` | `awaiting_approval` | host | compilation persisted |
| `awaiting_approval` | `pending` | operator | exact task version approved |
| `pending` | `admitted` | scheduler | dependencies satisfied; lease available |
| `admitted` | `planning` | policy | plan required or revision required |
| `admitted` | `ready` | policy | valid accepted plan already exists |
| `planning` | `ready` | planner result + host | plan validated and accepted |
| `ready` | `working` | supervisor + policy | implement action admitted |
| `working` | `verifying` | host | candidate source captured |
| `verifying` | `auditing` | host | verification passed |
| `verifying` | `correcting` | host/policy | verification failed and retry allowed |
| `auditing` | `correcting` | host/policy | blocking findings exist |
| `auditing` | `documenting` | policy | clean audit; docs required |
| `auditing` | `simplifying` | policy | clean audit; simplification admitted |
| `auditing` | `finalizing` | policy | completion prerequisites appear satisfied |
| `correcting` | `verifying` | host | corrected candidate captured |
| `documenting` | `verifying` | host | documentation mutation captured |
| `simplifying` | `verifying` | host | simplification mutation captured |
| any active state | `needs_input` | supervisor + policy | typed question required |
| any active state | `blocked` | supervisor + policy | durable blocker |
| any active state | `cancelled` | operator/host | cancellation reconciled |
| any active state | `budget_exhausted` | host | relevant budget reached |
| any active state | `unsafe` | host/policy | unsafe or ambiguous condition |
| `finalizing` | `completed` | completion finalizer | all evidence gates pass |
| terminal state | `pending` | explicit reopen operation | new task version/lineage; never in-place reversal |

## 39.2 Criterion Transitions

```text
pending -> passed
pending -> failed
pending -> waived
pending -> not_applicable
pending -> blocked

failed -> passed          only with fresh evidence
blocked -> passed         only when blocker resolved
blocked -> waived         only with operator authority
```

A terminal criterion cannot be silently rewritten. A task-version revision may introduce a replacement criterion with explicit lineage.

## 39.3 Finding Transitions

```text
open -> resolved
open -> waived
open -> rejected
open -> superseded
open -> stale
```

`resolved` requires evidence against a source revision and a fresh audit or deterministic proof.

## 39.4 Workspace Transitions

```text
planned
creating
ready
active
frozen
reconciling
completed
cancelled
failed
cleaned
```

A cleaned workspace retains metadata and artifact references even though its files are gone.

## 39.5 Sandbox Transitions

```text
requested
validated
creating
running
stopping
exited
timed_out
cancelled
failed
removed
```

---

# 40. Service and Interface Boundaries

The following Go interfaces are illustrative. Names may change, but responsibilities should remain narrow.

## 40.1 Project Store

```go
type ProjectStore interface {
    CreateProject(ctx context.Context, project Project) error
    GetProject(ctx context.Context, id uuid.UUID) (Project, error)
    ListProjects(ctx context.Context) ([]Project, error)
    RecordSnapshot(ctx context.Context, snapshot ProjectSnapshot) error
}
```

## 40.2 Task Store

```go
type TaskStore interface {
    CreateDraft(ctx context.Context, draft TaskDraft) error
    AddVersion(ctx context.Context, version TaskVersion) error
    ApproveVersion(ctx context.Context, taskID, versionID uuid.UUID) error
    GetCurrent(ctx context.Context, taskID uuid.UUID) (TaskAggregate, error)
    ListRunnableCandidates(ctx context.Context, projectID uuid.UUID) ([]TaskCandidate, error)
}
```

## 40.3 Transition Service

```go
type TransitionService interface {
    Apply(ctx context.Context, cmd TransitionCommand) (TransitionResult, error)
}
```

`TransitionCommand` includes expected aggregate version.

## 40.4 Model Client

```go
type ModelClient interface {
    Invoke(ctx context.Context, req ModelRequest) (ModelResponse, error)
    Stream(ctx context.Context, req ModelRequest) (<-chan ModelEvent, error)
}
```

## 40.5 Tool Broker

```go
type ToolBroker interface {
    Execute(ctx context.Context, scope ToolScope, call ToolCall) (ToolResult, error)
}
```

## 40.6 Sandbox Runtime

```go
type SandboxRuntime interface {
    Create(ctx context.Context, spec SandboxSpec) (SandboxHandle, error)
    Exec(ctx context.Context, handle SandboxHandle, cmd CommandSpec) (CommandResult, error)
    Stop(ctx context.Context, handle SandboxHandle) error
    Inspect(ctx context.Context, handle SandboxHandle) (SandboxStatus, error)
    Remove(ctx context.Context, handle SandboxHandle) error
}
```

## 40.7 Workspace Manager

```go
type WorkspaceManager interface {
    Create(ctx context.Context, req WorkspaceRequest) (Workspace, error)
    Snapshot(ctx context.Context, id uuid.UUID) (WorkspaceSnapshot, error)
    Diff(ctx context.Context, id uuid.UUID) (DiffArtifact, error)
    Commit(ctx context.Context, id uuid.UUID, req CommitRequest) (CommitEvidence, error)
    Cleanup(ctx context.Context, id uuid.UUID) error
}
```

## 40.8 Verifier

```go
type Verifier interface {
    Run(ctx context.Context, req VerificationRequest) (VerificationResult, error)
}
```

## 40.9 Context Compiler

```go
type ContextCompiler interface {
    Compile(ctx context.Context, req ContextRequest) (ContextPackage, error)
}
```

## 40.10 Retriever

```go
type Retriever interface {
    Retrieve(ctx context.Context, req RetrievalRequest) ([]RetrievalCandidate, error)
}
```

## 40.11 Artifact Store

```go
type ArtifactStore interface {
    Put(ctx context.Context, r io.Reader, meta ArtifactMetadata) (Artifact, error)
    Open(ctx context.Context, id uuid.UUID) (io.ReadCloser, Artifact, error)
    Verify(ctx context.Context, id uuid.UUID) error
}
```

## 40.12 Embedder

```go
type Embedder interface {
    EmbedDocuments(ctx context.Context, input []string) (EmbeddingBatch, error)
    EmbedQuery(ctx context.Context, input string) (Embedding, error)
    ModelInfo(ctx context.Context) (EmbeddingModelInfo, error)
}
```

---

# 41. Local API Surface

The exact URL structure may evolve. The important point is separation between command and query operations.

## 41.1 Project Endpoints

```text
POST   /api/projects
GET    /api/projects
GET    /api/projects/{id}
POST   /api/projects/{id}/refresh
POST   /api/projects/{id}/index
POST   /api/projects/{id}/baseline
```

## 41.2 Task Endpoints

```text
POST   /api/tasks/import
POST   /api/tasks/compile
POST   /api/tasks/{id}/approve
GET    /api/tasks
GET    /api/tasks/{id}
GET    /api/tasks/{id}/why
POST   /api/tasks/{id}/run
POST   /api/tasks/{id}/cancel
POST   /api/tasks/{id}/answer
```

## 41.3 Run Endpoints

```text
GET    /api/runs
GET    /api/runs/{id}
GET    /api/runs/{id}/events
GET    /api/runs/{id}/context
GET    /api/runs/{id}/diff
GET    /api/runs/{id}/verification
GET    /api/runs/{id}/audit
```

## 41.4 Queue Endpoints

```text
POST   /api/queues
GET    /api/queues/{id}
POST   /api/queues/{id}/cancel
```

## 41.5 Artifact and Evidence Endpoints

```text
GET    /api/artifacts/{id}
GET    /api/evidence/{id}
GET    /api/completions/{task-id}
```

## 41.6 Event Stream

```text
GET /api/events/stream
```

SSE events contain stable event IDs so clients may resume.

---

# 42. Example Configuration

```yaml
schema_version: revolvr-config-v1

data_root: /home/mitchell/.local/share/revolvr

api:
  listen: 127.0.0.1:7437
  local_secret_file: runtime/api-token

database:
  url_env: REVOLVR_DATABASE_URL
  max_connections: 20

openai:
  api_key_env: OPENAI_API_KEY
  roles:
    supervisor:
      model: configured-at-install
      reasoning_effort: high
      max_output_tokens: 12000
      timeout: 10m
    planner:
      model: configured-at-install
      reasoning_effort: high
      max_output_tokens: 30000
      timeout: 20m
    implementer:
      model: configured-at-install
      reasoning_effort: high
      max_output_tokens: 30000
      timeout: 30m
    auditor:
      model: configured-at-install
      reasoning_effort: high
      max_output_tokens: 20000
      timeout: 20m
    corrector:
      model: configured-at-install
      reasoning_effort: high
      max_output_tokens: 30000
      timeout: 30m

embeddings:
  endpoint: http://embedding-service:8080/v1
  model: selected-by-evaluation
  dimension: configured-at-index-init
  timeout: 60s
  batch_size: 64

sandbox:
  runtime: docker-rootless
  socket: /run/user/1000/docker.sock
  default_profile: strict
  strict_runtime: runsc
  max_memory: 24GiB
  max_cpus: 8
  pids_limit: 1024
  default_timeout: 45m
  network_default: none

artifacts:
  root: artifacts
  compression: zstd
  compress_after: 7d

scheduler:
  parallel_workers: 1
  default_max_tasks: 10
  default_max_cycles_per_task: 12
  default_max_wall_time: 8h

retrieval:
  max_context_tokens:
    supervisor: 12000
    planner: 30000
    implementer: 40000
    auditor: 35000
    corrector: 25000
  fts_limit: 50
  vector_limit: 50
  final_limit: 25
  structural_hops: 1

telemetry:
  structured_logs: true
  retain_context_reports: true
  retain_model_outputs: true
```

The config loader must reject `parallel_workers` values other than `1` under this architecture version.

---

# 43. Illustrative SQL Definitions

These are conceptual, not final migrations.

## 43.1 Tasks

```sql
CREATE TABLE core.tasks (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects(id),
    external_id text NOT NULL,
    status text NOT NULL,
    current_version_id uuid,
    aggregate_version bigint NOT NULL DEFAULT 0,
    priority integer NOT NULL DEFAULT 100,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    UNIQUE (project_id, external_id)
);
```

## 43.2 Task Versions

```sql
CREATE TABLE core.task_versions (
    id uuid PRIMARY KEY,
    task_id uuid NOT NULL REFERENCES core.tasks(id),
    version integer NOT NULL,
    schema_version text NOT NULL,
    source_artifact_id uuid NOT NULL REFERENCES core.artifacts(id),
    contract_json jsonb NOT NULL,
    contract_sha256 text NOT NULL,
    approved_at timestamptz,
    created_at timestamptz NOT NULL,
    UNIQUE (task_id, version),
    UNIQUE (task_id, contract_sha256)
);
```

## 43.3 Events

```sql
CREATE TABLE core.events (
    id uuid PRIMARY KEY,
    project_id uuid,
    task_id uuid,
    run_id uuid,
    event_type text NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL,
    UNIQUE (aggregate_type, aggregate_id, aggregate_version)
);
```

## 43.4 Artifacts

```sql
CREATE TABLE core.artifacts (
    id uuid PRIMARY KEY,
    sha256 text NOT NULL UNIQUE,
    size_bytes bigint NOT NULL CHECK (size_bytes >= 0),
    media_type text NOT NULL,
    logical_kind text NOT NULL,
    storage_path text NOT NULL,
    compression text,
    created_at timestamptz NOT NULL
);
```

## 43.5 Chunks and Embeddings

```sql
CREATE TABLE retrieval.chunks (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects(id),
    source_revision text NOT NULL,
    file_path text NOT NULL,
    symbol_id uuid,
    chunk_kind text NOT NULL,
    language text NOT NULL,
    body text NOT NULL,
    description text,
    body_sha256 text NOT NULL,
    search_vector tsvector,
    created_at timestamptz NOT NULL
);

CREATE TABLE retrieval.chunk_embeddings (
    chunk_id uuid PRIMARY KEY REFERENCES retrieval.chunks(id) ON DELETE CASCADE,
    embedding_space_id uuid NOT NULL REFERENCES retrieval.embedding_spaces(id),
    embedding vector,
    created_at timestamptz NOT NULL
);
```

The final indexed embedding column may use a dimension-specific cast/index generated for the active embedding space.

## 43.6 Relations

```sql
CREATE TABLE retrieval.relations (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES core.projects(id),
    subject_entity_id uuid NOT NULL REFERENCES retrieval.entities(id),
    predicate text NOT NULL,
    object_entity_id uuid NOT NULL REFERENCES retrieval.entities(id),
    valid_from timestamptz,
    valid_until timestamptz,
    authority text NOT NULL,
    confidence double precision,
    created_at timestamptz NOT NULL
);

CREATE TABLE retrieval.relation_sources (
    relation_id uuid NOT NULL REFERENCES retrieval.relations(id) ON DELETE CASCADE,
    source_type text NOT NULL,
    source_id uuid NOT NULL,
    source_artifact_id uuid REFERENCES core.artifacts(id),
    source_sha256 text NOT NULL,
    PRIMARY KEY (relation_id, source_type, source_id)
);
```

---

# 44. Transaction Boundaries

## 44.1 Task Approval Transaction

Atomically:

1. Verify task is awaiting approval.
2. Verify expected task aggregate version.
3. Mark task version approved.
4. Set current task version.
5. Set status pending.
6. Append approval event.

## 44.2 Run Admission Transaction

Atomically:

1. Verify task ready.
2. Verify no active global mutation lease.
3. Create run.
4. Create budget.
5. Acquire lease.
6. Transition task to admitted.
7. Append events.

Workspace creation happens after commit and is reconciled through the run operation journal.

## 44.3 Worker Result Transaction

After external workspace capture:

1. Verify run/cycle expected state.
2. Record sandbox result.
3. Record model invocation.
4. Record tool-execution references.
5. Record diff artifact.
6. Record candidate source identity.
7. Transition to verifying.
8. Append event.

## 44.4 Verification Result Transaction

1. Verify candidate source identity.
2. Store verification run/checks.
3. Update criterion evidence candidates.
4. Transition to auditing or correcting.
5. Append event.

## 44.5 Completion Transaction

1. Verify completion preflight hash.
2. Store completion records.
3. Update criteria and plan terminal state if exact accepted reconciliation is included.
4. Mark task completed.
5. Mark run completed.
6. Release lease.
7. Append terminal events.

Completion artifact materialization should occur before the terminal transaction and be content-addressed, so retry can reuse it safely.

---

# 45. Agent Loop Pseudocode

```text
function run_role(role, dossier, limits):
    pin model policy
    pin prompt version
    pin tool schema
    persist invocation intent

    while iteration < limits.max_iterations:
        response = openai.responses.create(
            model = role.model,
            input = conversation,
            tools = role.tools,
            structured_output = role.schema,
            reasoning = role.reasoning_effort
        )

        persist streamed diagnostic events

        if response is refusal:
            persist typed refusal
            return refused

        if response contains tool calls:
            for tool_call in response.tool_calls:
                validated = policy.validate_tool_call(tool_call, role, task, sandbox)
                if not validated:
                    tool_result = denied_result
                else:
                    tool_result = tool_broker.execute(validated)
                persist tool execution
                append tool result to conversation
            continue

        validate final structured output
        persist immutable final output
        return output

    return budget_exhausted
```

The model conversation is invocation-local. Durable state is not represented by carrying forward hidden conversation history.

---

# 46. Example Supervisor Decision

```json
{
  "schema_version": "revolvr-supervisor-decision-v1",
  "task_id": "auth-expired-token",
  "task_version": 3,
  "source_revision": "git-tree-or-policy-hash",
  "action": "implement",
  "rationale": "The accepted plan has one pending implementation step and no current verification or audit blocker.",
  "worker_role": "implementer",
  "strategy": {
    "approach": "Update middleware expiry validation and add focused regression tests.",
    "techniques": [
      "Reuse the existing token validation path",
      "Avoid changing refresh-token behavior"
    ],
    "targets": [
      {
        "kind": "file",
        "reference": "internal/auth/middleware.go"
      }
    ]
  },
  "inputs": [
    {
      "kind": "task",
      "reference": "task-version:3",
      "sha256": "..."
    },
    {
      "kind": "plan",
      "reference": "plan-version:2",
      "sha256": "..."
    }
  ]
}
```

The host rejects this if any identity is stale or the action is not admitted.

---

# 47. Example Sandbox Request

```json
{
  "schema_version": "revolvr-sandbox-request-v1",
  "sandbox_id": "0198...",
  "project_id": "0198...",
  "run_id": "0198...",
  "role": "implementer",
  "image": {
    "reference": "revolvr/go-worker",
    "digest": "sha256:..."
  },
  "runtime_profile": "strict",
  "command": ["/usr/local/bin/revolvr-worker"],
  "mounts": [
    {
      "source_id": "workspace:0198...",
      "target": "/workspace",
      "mode": "rw"
    },
    {
      "source_id": "context:0198...",
      "target": "/context",
      "mode": "ro"
    }
  ],
  "network": "none",
  "resources": {
    "cpus": 8,
    "memory_bytes": 25769803776,
    "pids": 1024,
    "timeout_seconds": 2700,
    "tmpfs_bytes": 4294967296
  },
  "environment": {
    "TASK_ID": "auth-expired-token",
    "RUN_ID": "0198...",
    "ROLE": "implementer"
  }
}
```

`sandboxd` resolves symbolic source IDs to managed paths. It does not accept arbitrary source paths from the API caller.

---

# 48. Example Evidence Manifest

```json
{
  "schema_version": "revolvr-completion-evidence-v1",
  "task": {
    "id": "auth-expired-token",
    "version": 3,
    "contract_sha256": "..."
  },
  "source": {
    "before_commit": "...",
    "before_tree": "...",
    "after_commit": "...",
    "after_tree": "...",
    "diff_sha256": "..."
  },
  "environment": {
    "worker_image_digest": "sha256:...",
    "verifier_image_digest": "sha256:...",
    "sandbox_profile": "strict"
  },
  "plan": {
    "id": "0198...",
    "version": 2,
    "sha256": "...",
    "terminal": true
  },
  "acceptance": [
    {
      "criterion_id": "AC-1",
      "status": "passed",
      "verification_check_id": "0198..."
    }
  ],
  "verification": {
    "run_id": "0198...",
    "source_revision": "...",
    "status": "passed"
  },
  "audit": {
    "run_id": "0198...",
    "source_revision": "...",
    "disposition": "clean"
  },
  "findings": [],
  "artifacts": [
    {
      "kind": "diff",
      "artifact_id": "0198...",
      "sha256": "..."
    }
  ],
  "models": [
    {
      "role": "implementer",
      "model": "configured-model",
      "prompt_version": "implementer-v1",
      "dossier_sha256": "..."
    }
  ]
}
```

---

# 49. Task Compiler Algorithm

```text
input:
    original task prose or specification

stage 1: normalize
    preserve original bytes
    extract title and obvious constraints
    identify referenced files/issues/specs

stage 2: investigate
    read relevant project guidance
    inspect repository map
    identify existing task overlap
    identify available verification

stage 3: propose
    create bounded task(s)
    define scope/exclusions
    define acceptance
    define risk/mutation/network
    define dependencies/conflicts

stage 4: validate
    schema
    IDs
    graph
    bounds
    verification methods
    unsupported placeholders
    policy

stage 5: present
    show operator summary and unresolved questions

stage 6: approve
    store immutable approved version
```

Compiler output must explicitly mark uncertain assumptions. It must not hide them in narrative prose.

---

# 50. Local Embedding Evaluation Protocol

The local model should be chosen empirically.

## 50.1 Dataset Construction

Build queries from real source:

- “where is provider routing configured?”
- “what code persists tool executions?”
- “why was this storage backend removed?”
- “find the test for expired tokens.”
- “which component owns workspace cleanup?”

Each query has judged relevant chunks.

## 50.2 Metrics

- Recall@5.
- Recall@10.
- Mean reciprocal rank.
- Exact-symbol preservation.
- Latency.
- VRAM usage.
- Index size.
- Query throughput.
- Code-language breakdown.

## 50.3 Comparison

Compare:

- old Sodoryard embedding model,
- at least one current code-oriented embedding model,
- at least one general multilingual/text model if documents matter,
- optional OpenAI embedding baseline.

## 50.4 Selection Rule

The chosen model must meet a minimum retrieval-quality threshold and acceptable local latency.

Model size alone is not a selection criterion.

## 50.5 Re-Evaluation

Re-run the suite before changing:

- model,
- quantization,
- pooling,
- chunk-description strategy,
- vector normalization.

---

# 51. Hybrid Retrieval Scoring Baseline

A transparent first implementation may use:

```text
exact path match              +100
exact symbol match             +80
direct task reference          +70
lexical FTS normalized score   0..30
vector normalized score        0..30
one-hop structural relation    +15
accepted architecture source   +10
recent successful prior use     +5
stale index                    -20
low-authority candidate fact   -15
```

The actual formula should be configured and evaluated, not treated as permanent doctrine.

Deduplicate by source chunk identity before final packing.

---

# 52. Project Environment Contract

A project may include a file such as:

```yaml
schema_version: revolvr-project-environment-v1

worker:
  dockerfile: .revolvr/Dockerfile
  context: .
  target: worker

verifier:
  dockerfile: .revolvr/Dockerfile
  context: .
  target: verifier

toolchains:
  go: "1.26"
  node: "24"

verification:
  baseline:
    - id: go-test
      command: ["go", "test", "./..."]
      timeout: 15m
  focused:
    - id: gofmt
      command: ["sh", "-lc", "test -z \"$(gofmt -l .)\""]
      timeout: 5m
  final:
    - id: go-test-final
      command: ["go", "test", "-count=1", "./..."]
      timeout: 20m

network:
  default: none
  dependency_commands:
    - ["go", "mod", "download"]

protected_paths:
  - .github/workflows/security.yml
  - .revolvr/**
  - deploy/production/**

index:
  include:
    - "**/*.go"
    - "**/*.md"
    - "**/*.sql"
  exclude:
    - "**/vendor/**"
    - "**/node_modules/**"
```

The environment contract itself is part of run identity.

---

# 53. Source Scope Prediction and Enforcement

Before implementation, the plan may predict source areas.

After implementation, the host classifies actual changes:

```text
expected
adjacent
unexpected
protected
```

Policy:

- expected changes proceed,
- adjacent changes create an audit signal,
- unexpected changes require justification or plan revision,
- protected changes fail or escalate.

Scope prediction must not be so strict that legitimate necessary changes become impossible. It is a guardrail and evidence signal, not a naive filename prison.

---

# 54. Dependency and Supply-Chain Features

These features materially improve autonomous correctness and should be included in later verification phases.

## 54.1 Dependency-change detection

Detect changes to:

- `go.mod`,
- `go.sum`,
- package manifests,
- lockfiles,
- container base images.

## 54.2 Dependency rationale

A new dependency requires:

- purpose,
- alternatives considered,
- license metadata when available,
- security scan,
- impact on image/build.

## 54.3 Language-native scanning

Examples:

- `govulncheck` for Go projects,
- package-manager audit where appropriate,
- secret scanning,
- container image scanning.

These are configurable project verification checks, not hard-coded global commands.

## 54.4 Reproducible dependency cache

Caches are mounted separately from source and identified in evidence.

A poisoned cache must be removable without losing canonical state.

---

# 55. Additional Correctness Features

## 55.1 Metamorphic verification

For suitable tasks, verify behavior under transformed inputs rather than only one expected fixture.

## 55.2 Property-based tests

The planner or auditor may recommend property-based testing for parsers, state machines, and serialization boundaries.

## 55.3 Mutation testing

Optional high-risk verification can use mutation testing to determine whether tests actually detect behavioral changes.

## 55.4 Differential tests

Compare old and new implementations for behavior intended to remain stable.

## 55.5 Contract tests

Use explicit contracts for APIs, database schemas, generated clients, and cross-component boundaries.

## 55.6 Static architecture checks

Enforce package-layer or dependency rules deterministically where a project defines them.

## 55.7 Test-selection explanation

When a focused test set is used instead of the full suite, record why it is sufficient and which final tier will cover the remainder.

---

# 56. Recovery Scenarios

## 56.1 Crash before sandbox creation

Resume from admitted run and create the pinned sandbox.

## 56.2 Crash after sandbox creation before state update

Discover sandbox by stable ID, inspect it, and reconcile.

## 56.3 Crash after worker exits before artifact ingestion

Recover output staging by stable sandbox/run identity.

## 56.4 Crash after candidate commit before database update

Detect commit in managed repository and verify it matches the operation journal before adopting it.

## 56.5 Crash after completion artifacts before terminal state

Verify immutable artifact hashes and retry the terminal transaction.

## 56.6 Crash after external branch export

External export must have an idempotency marker and exact target ref. Recovery may confirm the ref, but must not blindly repeat a potentially divergent push.

---

# 57. Migration from Existing Revolvr and Sodoryard

## 57.1 Recommended Strategy

Use the current Revolvr repository as the product lineage, but create an explicit architecture rewrite boundary.

Possible approach:

```text
main
  existing stable Revolvr

branch:
  architecture/postgres-v2
```

Alternatively, create a new top-level `v2` development line and later replace main.

## 57.2 Port by Capability, Not Directory

For each old subsystem:

1. Identify behavior worth keeping.
2. Write a new interface/contract.
3. Add tests describing preserved behavior.
4. Port or rewrite the minimum code.
5. Remove legacy coupling.

## 57.3 Suggested Revolvr Port Order

1. Task models and task validation.
2. Lifecycle routing concepts.
3. Supervisor decision schemas.
4. Evidence references.
5. Budget concepts.
6. Needs-input model.
7. Verification.
8. Workspace safety.
9. Completion evidence.
10. Recovery and metrics.

## 57.4 Suggested Sodoryard Port Order

1. OpenAI/provider request abstractions.
2. Stream accumulation.
3. Tool registry and tool execution concepts.
4. Tree-sitter parsers.
5. Code graph.
6. Context analyzer and budgeter.
7. Context reports.
8. Relevant UI components.

## 57.5 Explicit Legacy Deletion Targets

- Shunter module and adapters.
- Shunter-specific project-memory code.
- generated Shunter bindings.
- vendored Shunter client.
- LanceDB and CGO packaging.
- SQLite storage backend.
- multi-provider defaults.
- parallel chain spawn logic.
- obsolete migration utilities.
- duplicate old role prompts.

## 57.6 Data Migration

Historical data migration is optional.

Prioritize importing:

- completed task summaries,
- durable decisions,
- known failures,
- useful receipts.

Do not spend disproportionate effort migrating low-value telemetry from experimental runs.

---

# 58. Development and Quality Standards

## 58.1 Go

- `gofmt`.
- `go test`.
- `go vet`.
- race tests for coordination code.
- static analysis.
- explicit context propagation.
- bounded goroutines.
- no hidden global mutable state.

## 58.2 SQL

- SQL-first migrations.
- named sqlc queries.
- constraints for invariants.
- `EXPLAIN` for important retrieval/scheduler queries.
- no unbounded table scans in hot UI paths.
- transaction isolation explicitly chosen.

## 58.3 Frontend

- TypeScript strict mode.
- generated API types where practical.
- no lifecycle inference.
- accessible keyboard navigation.
- state sourced from API.

## 58.4 Security

- threat-model tests.
- path traversal tests.
- symlink race tests.
- unsafe container-spec tests.
- secret-redaction tests.
- local API request-origin tests.

## 58.5 Model Integration

- JSON-schema validation.
- refusal tests.
- malformed output tests.
- stale identity tests.
- illegal action tests.
- tool-call denial tests.
- deterministic fake model.

---

# 59. Operational Runbooks Required Before v1

1. Install and configure rootless container runtime.
2. Install strict sandbox runtime.
3. Start/stop control stack.
4. Register a project.
5. Build a project worker image.
6. Diagnose baseline verification.
7. Recover an interrupted run.
8. Back up and restore Revolvr.
9. Rotate OpenAI credentials.
10. Replace/reindex local embedding model.
11. Inspect artifact storage growth.
12. Clean orphaned sandboxes safely.
13. Export a successful branch or patch.
14. Diagnose a needs-input task.
15. Verify a completion capsule.

---

# 60. Security Acceptance Tests

Before real-project autonomous use, prove:

- worker cannot read a test secret in host home,
- worker cannot write outside managed workspace,
- worker cannot access container runtime socket,
- worker cannot access PostgreSQL credentials,
- worker cannot access OpenAI credentials,
- worker cannot reach the network under `none`,
- worker is killed at timeout,
- fork bomb is stopped by PID/resource limits,
- disk growth is bounded,
- symlink mount escape is rejected,
- original checkout remains unchanged,
- cancelled container cannot persist,
- artifact paths reject traversal,
- protected paths are enforced.
