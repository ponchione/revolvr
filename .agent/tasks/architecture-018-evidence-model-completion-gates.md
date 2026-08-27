---
id: architecture-018-evidence-model-completion-gates
status: completed
workflow: mixed-pass-v1
phase: implement
depends_on: architecture-017-verification-engine
---

# Implement the evidence model and completion rejection gates

## Sequence and status

- Sequence: `018` of `025`.
- Status: completed.
- Prerequisite: `architecture-017-verification-engine`.
- Phase gate: completion machinery begins only after source-bound final
  verification evidence exists; successful live completion still waits for
  task 019's independent clean audit.

## Primary outcome

Make claims/evidence and completion a host-owned, false-completion-resistant,
transactional boundary that produces immutable human- and machine-readable
capsules.

## Required reading

- ADR-016, ADR-019, ADR-023, and ADR-024.
- Specification Sections 3.5, 9.16-9.18, 12.8, 18,
  29 Phase 5, 37.11, 39, 44.5, 48, and 56.5.
- The superseded
  `REVOLVR_PROGRAMMATIC_WORKSPACE_AND_CONTINUAL_HARNESS_SPEC.md` Sections 2.2,
  10.3, and 25-26 only as subordinate provenance compatibility requirements.

## Existing foundations to inspect

- Artifact metadata/bytes, task/plan/criterion state, workspace/commit/diff,
  verification records, events, and scheduler lease/run state.
- Existing `internal/receipt`, `internal/ledger`,
  `internal/autonomousfinalization`, and `internal/autonomousarchive` for
  useful evidence/manifest/recovery behavior only; do not preserve SQLite or
  filesystem lifecycle authority.

## Starting assumptions

- A supervisor `complete` response is only a proposal.
- Audit records may be supplied as deterministic test fixtures now; task 019
  owns producing them through an independent role.
- Artifact bytes are immutable and content-addressed before terminal state.

## Implementation requirements

- Add the minimal claims, claim-evidence, completion, and artifact-provenance
  schema/queries with database constraints for exact ownership and hashes.
- Implement read-only completion preflight binding accepted task version,
  source/diff/commit/tree, terminal plan and criteria, fresh final verification,
  fresh clean audit, finding dispositions, budget, workspace, prompt/model,
  image/profile, artifacts, and operator inputs into one preflight hash.
- Reject every false-completion condition in Section 18.7 with a stable reason
  and no terminal mutation.
- Materialize deterministic completion evidence JSON, readable Markdown, and a
  manifest of exact hashes before the terminal transaction.
- Include a versioned trajectory-manifest provenance envelope/state in
  completion evidence and bind it into the preflight/capsule/manifest hashes.
  The current direct-tools core loop may record an explicit
  `inactive_not_applicable` trajectory-extension state; it must not fabricate a
  normalized post-core trajectory manifest.
- Include the exact harness-asset-set manifest and hash that influenced the
  run. Current `direct_tools_v1` uses a canonical exact empty/inactive asset
  set rather than an omitted, null, or inferred value.
- Fail closed when a run used any trajectory or harness input but its exact
  versions, hashes, covered sequence/range, referenced artifacts, or asset-set
  manifest are missing, unresolved, stale, divergent, or changed at preflight
  or terminal revalidation.
- In one transaction revalidate the preflight hash/aggregate versions, attach
  artifacts and claims, mark task/run completed, release the lease, and append
  terminal events.
- Recover a crash after artifact materialization by verifying and reusing exact
  bytes; reject divergent artifacts or canonical state and never emit duplicate
  completion events.

## Scope boundaries and non-goals

- Do not implement the auditor/corrector, archive/export, retention, task-state
  inference from prose, or automatic operator waivers.
- Do not implement the post-core trajectory normalization/query service,
  Python workspace, skills, refinement proposal/activation, or harness-asset
  evaluation. Full trajectory generation remains PTC-101.
- Do not mark criteria or findings terminal without their required authority
  and evidence.
- Do not store raw secrets in any capsule or manifest.

## Acceptance criteria

- Every completion capsule field from Sections 18.6 and 48 resolves to exact
  canonical evidence and verified artifact bytes.
- Direct-tools completion fixtures record the explicit inactive trajectory
  envelope and exact empty/inactive harness-asset-set manifest/hash. Fixtures
  that claim trajectory or harness influence fail when any exact provenance,
  coverage, artifact, version, or hash is absent, stale, or changed.
- Parameterized tests reject missing/nonterminal plan or criteria, stale/failed
  verification, missing/stale/non-clean audit, open findings, changed source,
  invalid budget/workspace/commit, incomplete artifacts, stale preflight, and
  unreconciled lease/workspace.
- A fully valid synthetic evidence fixture completes atomically once.
- Forced artifact/transaction failures and crash points resume idempotently or
  fail on divergence without partial terminal authority.
- Secret sentinel scanning, migration/sqlc, PostgreSQL integration, and full Go
  tests pass.

## Deterministic verification

```bash
test -z "$(gofmt -l cmd internal)"
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0 generate -f db/sqlc.yaml
REVOLVR_TEST_DATABASE_URL="$REVOLVR_DATABASE_URL" go test ./internal/evidence ./internal/completion
go test ./...
git diff --check
```

## Expected completion report

Report schema/artifact/package changes, preflight and capsule hashes, every
false-completion reason tested, successful atomic fixture, crash/rollback/retry
evidence, secret scan, and complete test results.
