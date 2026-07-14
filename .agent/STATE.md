# Agent State

## Current Focus

`AUDIT-R3-04` closes structural interpretation of headings inside fenced
Markdown examples. The first unchecked follow-up is `AUDIT-R3-05`, which makes
Git object-ID validation consistent for SHA-1 and SHA-256 repositories.

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

- Read `AGENTS.md`, `.agent/TASKS.md`, `.agent/STATE.md`, and
  `.agent/DECISIONS.md` before acting.
- Do one bounded task, verify it, update durable state, and stop.
- Do not use `codex resume` or depend on an old session transcript.
