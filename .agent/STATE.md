# Agent State

## Current Focus

Resolve `AUDIT-FIX-03`, the first unchecked task in `.agent/TASKS.md`. Mixed
passes now require a clean worktree at every staging boundary; the next pass
must harden the source-writer and retention lock-file identities.

## Latest Completed Audit Fix (2026-07-14)

- Task selected: `AUDIT-FIX-02`.
- Mixed-pass run configuration rejects the legacy
  `commit.allow_pre_existing_dirty: true` setting before source locking, runner
  invocation, verification, staging, or commit work. Explicit `false` remains
  accepted for configuration compatibility.
- The commit package no longer exposes a dirty-worktree bypass. Mixed and
  autonomous source-changing paths unconditionally refuse a nonempty pre-run
  capture, so filename subtraction cannot misattribute operator bytes to a run.
- Real-Git regressions cover unrelated and overlapping operator/run edits. Both
  cases fail closed with no commit or staged paths and retain the exact operator,
  run, and task-metadata worktree bytes.
- Files changed: `README.md`, `internal/app/app_test.go`,
  `internal/app/autonomous_run.go`, `internal/app/config.go`,
  `internal/autonomouscorrection/coordinator.go`,
  `internal/autonomouscycle/types.go`, `internal/autonomouscycle/worker.go`,
  `internal/cli/root_test.go`, `internal/commit/commit.go`,
  `internal/commit/commit_test.go`, `internal/runonce/effectiveconfig_test.go`,
  `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`,
  `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md`.
- Verification passed:
  `go test -count=10 -run 'TestRunRefusesRealPreExistingDirtyWorktreeWithoutStaging' ./internal/commit`,
  focused run-once/app/CLI rejection tests, `go test -count=1 ./...`, and
  `go run ./cmd/revolvr config check`.
- Remaining audit work: `AUDIT-FIX-03` through `AUDIT-FIX-07`. No blocker is
  recorded.

## Wide-Sweep Audit (2026-07-14)

- Six evidence-backed problems were found: a cancellation/source-lease race,
  unsafe staging when pre-existing dirty work is allowed, unprotected
  coordinator lock-file identities, writable ledger opens in read projections,
  an unstated/inconsistent non-Unix build contract, and a stale local smoke-test
  assertion.
- No speculative performance or line-count cleanup was recorded.
- The ordinary full suite exposed the ownership race once. The race-enabled and
  shuffled suites passed, which is consistent with an intermittent terminal
  ordering defect rather than a broad data race.
- The local smoke failed only at its stale header assertion. Both fake-Codex
  run-once smokes passed.
- Linux, Darwin, and FreeBSD builds passed; the Windows build failed on untagged
  Unix syscall usage.

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
- `go test -count=1 ./...` passes after the clean mixed-pass commit contract.
- `go test -race -count=1 ./...` and
  `go test -shuffle=on -count=1 ./...` pass.
- `go vet ./...`, `go mod verify`, and `govulncheck ./...` pass; no reachable
  vulnerabilities were reported.
- `git diff --check` passes.
- Root help, config check, and both fake-Codex run-once smokes pass.
- `scripts/smoke-local.sh` fails at an outdated task-list header assertion.
- No live Codex execution was started through Revolvr during the final
  scheduling/checkpoint/migration campaign, readiness assessment, or audit.

## Durable Documentation

- `README.md`: operator setup, commands, workflows, and safety guidance.
- `AGENTS.md`: repository working and verification rules.
- `.agent/TASKS.md`: current backlog and compact completed-program index.
- `.agent/DECISIONS.md`: durable architecture and implementation decisions.
- `.agent/LOOP_PROMPT.md`: reusable fresh-session pass instructions.
- `.agent/CYBER_ARPG_READINESS_ASSESSMENT.md`: bounded external readiness
  evidence.
- `AUDIT_PROBLEMS.md`: current wide-sweep audit findings and remediation
  guidance.

Obsolete setup handoffs, design-target drafts, and completed-program kickoff
notes were removed after their durable content was incorporated into the files
above. Detailed historical prose remains available through Git history.

## Current Repository Understanding

- Stack: Go 1.22 CLI application.
- Entry point: `cmd/revolvr/main.go`.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test -count=1 ./...`.
- Formatting: `gofmt` on changed Go files.
- Runtime state: `.revolvr/`, local and ignored by Git.

## Verification Gaps

See `AUDIT_PROBLEMS.md` and the remaining `AUDIT-FIX-*` backlog. The documented
local smoke is red, Windows is not a buildable target under the currently
unstated platform contract, and coordinator locks and read-only ledger
projections still require their bounded fixes.

## Notes For Next Fresh Session

- Read `AGENTS.md`, `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md` before acting.
- Do one bounded task, verify it, update durable state, and stop.
- Do not use `codex resume` or depend on an old session transcript.
