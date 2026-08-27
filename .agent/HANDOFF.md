# Agent Handoff

Updated: 2026-08-27

## Where We Stopped

- The post-gate attended-runtime patch was reviewed and reconciled as one
  maintenance change. The embedded single-build Codex allowlist is gone;
  admission instead freezes configured/resolved paths, SHA-256, and exact safe
  version output, then rejects identity drift before later effects.
- Invalid model receipts are preserved byte-for-byte with exact evidence and
  replaced by a valid `completed_with_concerns` harness receipt. CLI run views,
  validation, prompts, dogfood/smoke scripts, operator docs, and active
  readiness policy now describe the same contract.
- Group/other write permission was removed from the protected `.agent` tree
  and `.revolvr/config.yaml`, and the two legacy `status: complete` task files
  were normalized to `completed`. `revolvr task list` now loads the canonical
  graph successfully.
- Verification passed: `gofmt` on changed Go files, `bash -n` on changed shell
  scripts, `go test ./...`, CLI help/config/status/task-list checks,
  `scripts/smoke-external-attended.sh`, and `git diff --check`. No dependency
  was added.
- This maintenance work does not unlock the active architecture sequence.
- One fresh Architecture 024 pass confirmed that
  `.agent/tasks/architecture-024-ui.md` was the sole dependency-satisfied
  pending task at entry, then stopped at its required Phase 9 gate without
  changing Go or frontend implementation files.
- The gate is not satisfied. Section 23.3 requires acceptable measured
  real-project thresholds before sequential queue autonomy is enabled, but
  `evals/golden/baseline.json` still records `threshold: null`, explicitly
  omits live dogfood, and says the quality threshold was not set before the
  deterministic baseline.
- The bounded queue is deterministically implemented and fail-closed, but is
  not fully operable from the ordinary CLI: `internal/queue` and migration
  00013 admit only `deterministic_evaluation_only`, and
  `internal/app.StartSequentialQueue` rejects a production call without an
  injected executor before database or worker effects. A focused real CLI
  invocation returned `sequential queue: Section 23.3 real-project quality
  gate has no approved measured threshold`.
- Deterministic fixture and queue evidence cannot be relabeled as the missing
  real-project evidence. Building a desktop queue/run surface while its
  CLI-first service is deliberately unavailable would violate ADR-020 and the
  Architecture 024 phase gate.
- Architecture 024 is now recorded as blocked, not complete. No `web/` tree,
  Wails/Vue dependency, lockfile, local API, SSE stream, view, mutation,
  security behavior, or accessibility behavior was added or claimed.
- Focused gate checks passed:
  `go test ./internal/app -run
  '^TestSequentialQueueRealProjectStartFailsClosedWithoutMeasuredGate$'
  -count=1` and `go test ./internal/evaluation -run
  '^TestGoldenBaseline$' -count=1`. Architecture 024 build verification was
  not run because implementation was forbidden by the failed phase gate.
- `git diff --check` passed. A read-only `revolvr task list` confirmation
  attempt failed closed because the existing `.agent` directory mode is 0775;
  permissions were not changed in this gate-only pass. Task-file frontmatter
  and the active-sequence record supplied the dependency check instead.
- No commit was created. Architecture 025, Graphiti, all deferred PTC work,
  and the legacy external-readiness backlog remain non-selectable.

## Exact Blocker And Resume Condition

There is no legal implementation task to select from the active architecture
sequence. Architecture 024 may return from `blocked` to `pending` only after a
separately authorized evidence pass:

1. records approved numeric Section 23.3 thresholds for false completion,
   unrecovered crashes, repeated-strategy loops, scope violations, host safety,
   and task completion from real-project baseline data;
2. records qualifying real-project bounded-queue results against those
   thresholds with exact source/model/prompt/sandbox/task/outcome identities;
3. adds an admitted canonical production executor so ordinary
   `revolvr queue start` is fully operable without test injection; and
4. revalidates the CLI-first core loop and queue as trustworthy without
   weakening their fail-closed authority.

The prior checkout-local path-permission blocker is resolved. No permission or
task-schema repair remains before repository-path-aware CLI commands can load
the canonical graph.

Do not start Architecture 024 merely because its file dependency is complete,
and do not start Architecture 025 while Architecture 024 is blocked. Do not
invent a threshold, treat deterministic fixtures as real-project runs, or add
the missing evidence/production authority under the UI task.
