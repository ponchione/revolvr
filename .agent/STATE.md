# Agent State

## Current Focus

None.

## Last Run

Task completed on 2026-07-07:

- Selected task: resolve the dogfood run diagnostics found after the README dogfooding pass: stale receipt body facts, false `.agent/STATE.md` changed-file mismatch warning, zero Codex usage metrics, and missing live `run` progress output.
- Files changed: `internal/receipt/claims.go`, `internal/receipt/metrics.go`, `internal/receipt/update.go`, `internal/receipt/receipt_test.go`, `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: final receipt rewrites now refresh harness-owned `Changed Files` and `Verification` body sections to match finalized frontmatter; dotfile path claims keep their leading `.`; Codex usage parsing continues past malformed JSONL fragments when a later valid usage event exists; `revolvr run --once` and `run --max-passes` stream summarized Codex progress to stdout before the final summary.
- Verification run: `gofmt -w internal/receipt/claims.go internal/receipt/metrics.go internal/receipt/update.go internal/receipt/receipt_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/cli/root.go internal/cli/root_test.go`; `go test ./internal/receipt`; `go test ./internal/codexexec`; `go test ./internal/cli`; `go test ./internal/runonce`; `bash -n scripts/smoke-local.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: commit the completed repair slice.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a README Dogfooding section with the commands for `doctor`, `task add`, `run --once`, `status`, and `show`.
- Files changed: `README.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: grouped the dogfooding flow into one README section with commands for preflight, queueing a task, running one pass, checking status, and showing a recorded run.
- Verification run: not run; documentation-only change and the Revolvr harness owns pass verification.
- Verification result: not run.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a `revolvr doctor` dogfood preflight for Codex, Git identity, clean worktree, runtime ignore state, and verification readiness.
- Files changed: `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr doctor` now reports initialized state, effective config loading, configured Codex executable availability, configured Git executable availability, Git identity, clean worktree state, `.revolvr/` Git ignore readiness, and effective verification command readiness. It exits nonzero with `doctor: preflight failed` when required checks are not ready.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/doctor.go internal/cli/root_test.go internal/cli/doctor_test.go`; `go test ./internal/cli`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; expected-failure `go run ./cmd/revolvr doctor` on the dirty implementation checkout, asserting `Dogfood preflight:`, `Ready: false`, and `doctor: preflight failed`; final `go test ./internal/cli` after cleanup.
- Verification result: all commands passed, with the doctor command failing only in the expected pre-commit dirty-worktree check.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: make `revolvr init` locally ignore `.revolvr/` in Git worktrees so fresh dogfood repos do not start dirty.
- Files changed: `internal/cli/state.go`, `internal/cli/root_test.go`, `scripts/smoke-run-once-fake-codex.sh`, `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: `revolvr init` now idempotently adds `/.revolvr/` to `.git/info/exclude` when initialized from a Git worktree, leaving non-Git directories alone. The fake-Codex smoke tests no longer create a tracked `.gitignore` and assert that post-init Git status stays clean.
- Verification run: `gofmt -w internal/cli/state.go internal/cli/root_test.go`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` verification failure path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex-verification-failure.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex-verification-failure.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, has fake Codex create `generated.txt` and a matching failure receipt, intentionally fails verification with `test -f required.txt`, checks the run failure summary, confirms the task is blocked, confirms no commit is created, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex-verification-failure.sh`; `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex-verification-failure.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a no-real-Codex integration smoke test for `revolvr run --once` success path using a strict fake Codex executable.
- Files changed: `scripts/smoke-run-once-fake-codex.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-run-once-fake-codex.sh` builds a temporary `revolvr` binary, creates a temporary Git repo, configures local Git identity, initializes Revolvr state, writes `.revolvr/config.yaml` with `codex.executable` pointing at a strict fake Codex script, verifies `generated.txt`, runs `revolvr run --once`, checks completed task/run status, confirms a commit, checks run/receipt artifacts, and runs `revolvr show <run-id>`.
- Verification run: `bash -n scripts/smoke-run-once-fake-codex.sh`; `./scripts/smoke-run-once-fake-codex.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Files changed: `scripts/smoke-local.sh`, `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; development smoke-test script and documentation only.
- Smoke test added: `scripts/smoke-local.sh` builds a temporary `revolvr` binary, runs `init`, `task add`, `task list`, `config check`, and `status` in a temporary workspace, and asserts expected outputs without invoking Codex.
- Verification run: `bash -n scripts/smoke-local.sh`; `./scripts/smoke-local.sh`; `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: no unchecked backlog items remain.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Files changed: `README.md`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: none; documentation-only change.
- Documentation added: root README covering setup/build, `init`, task queue commands, optional `.revolvr/config.yaml`, `config check`, `run --once`, `run --max-passes`, `status`, `show`, and development checks.
- Verification run: `go test ./...`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a targeted smoke-test note or script for exercising `init`, `task add`, `task list`, `config check`, and `status` without invoking Codex.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: expand `revolvr config check` output to show effective verification command details, not only the command count.
- Files changed: `internal/cli/config.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: `config check` now prints one deterministic detail row per effective verification command after `Verification command count`, including command index, name, args, and optional dir/timeout fields. No detail rows are printed when the effective command list is empty.
- Verification run: `gofmt -w internal/cli/config.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next unchecked backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI; the targeted smoke-test note/script task also remains.
- Blockers: none.

Task completed on 2026-07-07:

- Selected task: add Codex yolo/dangerous bypass support for autonomous harness runs, fix fresh-session wrapper flags, and update focused tests.
- Files changed: `internal/codexexec/codexexec.go`, `internal/codexexec/codexexec_test.go`, `internal/runonce/runonce.go`, `internal/runonce/runonce_test.go`, `internal/cli/config.go`, `internal/cli/root.go`, `internal/cli/root_test.go`, `agent-one.sh`, `.agent/TASKS.md`, `.agent/STATE.md`, `.agent/DECISIONS.md`.
- Behavior changed: CLI-initiated harness runs now default to Codex dangerous bypass/yolo mode and pass `--dangerously-bypass-approvals-and-sandbox` instead of separate sandbox/approval flags when enabled. Config supports `codex.dangerously_bypass_approvals_and_sandbox` and `codex.yolo` aliases, including explicit `false` to disable the default. `agent-one.sh` now uses the dangerous bypass flag with valid `codex exec` ordering.
- Verification run: refreshed Codex manual with `node /home/gernsback/.codex/skills/.system/openai-docs/scripts/fetch-codex-manual.mjs`; `gofmt -w internal/cli/root.go internal/cli/config.go internal/cli/root_test.go internal/codexexec/codexexec.go internal/codexexec/codexexec_test.go internal/runonce/runonce.go internal/runonce/runonce_test.go`; `go test ./...`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config`; `bash -n agent-one.sh agent-loop.sh`; `codex exec --dangerously-bypass-approvals-and-sandbox --help`; `codex exec --yolo --help`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous task completed on 2026-07-07:

- Selected task: replace bare parent command placeholder output for `revolvr task` and `revolvr config` with normal help output, and update focused CLI tests.
- Files changed: `internal/cli/root.go`, `internal/cli/root_test.go`, `.agent/TASKS.md`, `.agent/STATE.md`.
- Behavior changed: bare `revolvr task` and `revolvr config` now show Cobra help instead of placeholder "not implemented yet" output.
- Verification run: `gofmt -w internal/cli/root.go internal/cli/root_test.go`; `go test ./...`; `go run ./cmd/revolvr task`; `go run ./cmd/revolvr config`; `go run ./cmd/revolvr --help`; `go run ./cmd/revolvr config check`; `go run ./cmd/revolvr status`.
- Verification result: all commands passed.
- What remains: next backlog item is to add a concise README with setup, task queue, config, run, status, and show examples for the current CLI.
- Blockers: none.

Previous setup performed on 2026-06-29:

- Initialized local Revolvr runtime state with `go run ./cmd/revolvr init`.
- Created fresh-session agent loop setup files.
- Did not run `agent-one.sh`; that would invoke a nested Codex session.

## Current Repository Understanding

- Stack: Go 1.22 CLI application using Cobra, YAML config, and SQLite.
- Build command: `go build ./cmd/revolvr`.
- Test command: `go test ./...`.
- Lint/typecheck command: none configured; use `gofmt -w <changed go files>` and `go test ./...`.
- Important directories: `cmd/revolvr`, `internal/cli`, `internal/runonce`, `internal/ledger`, `internal/taskqueue`, `internal/codexexec`, `internal/receipt`, `internal/verification`, `internal/commit`, `internal/gitstate`, `internal/runner`.
- Runtime state: `.revolvr/`, created by `revolvr init` and ignored by Git.

## Verification Gaps

- No separate lint command is configured.

## Notes For Next Fresh Session

- Read `AGENTS.md` first.
- Read `.agent/TASKS.md`, `.agent/STATE.md`, and `.agent/DECISIONS.md` before making changes.
- Do one task, verify, update state, and stop.
- Do not run nested Codex from inside another active Codex session.
