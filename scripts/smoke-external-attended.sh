#!/usr/bin/env bash
set -euo pipefail
umask 0022

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/revolvr-external-attended.XXXXXX")"
cleanup() {
  rm -rf -- "$TMP_ROOT"
}
trap cleanup EXIT

BIN_DIR="$TMP_ROOT/bin"
REPO="$TMP_ROOT/external-project"
OUTPUT="$TMP_ROOT/output"
REVOLVR_BIN="$BIN_DIR/revolvr"
CODEX_BIN="$BIN_DIR/codex"
CODEX_CALL_LOG="$TMP_ROOT/codex-exec-called"
mkdir -p "$BIN_DIR" "$REPO" "$OUTPUT"

export PATH="$BIN_DIR:$PATH"

fail() {
  echo "external attended smoke: $*" >&2
  exit 1
}

assert_contains() {
  local path="$1"
  local expected="$2"
  grep -Fq -- "$expected" "$path" || {
    sed 's/^/  /' "$path" >&2 || true
    fail "expected $path to contain: $expected"
  }
}

run_capture() {
  local name="$1"
  shift
  echo "Running: $*"
  (cd "$REPO" && "$@") >"$OUTPUT/$name.out" 2>"$OUTPUT/$name.err" || {
    sed 's/^/  /' "$OUTPUT/$name.out" >&2 || true
    sed 's/^/  /' "$OUTPUT/$name.err" >&2 || true
    fail "command failed: $*"
  }
}

expect_failure() {
  local name="$1"
  shift
  echo "Running expected refusal: $*"
  if (cd "$REPO" && "$@") >"$OUTPUT/$name.out" 2>"$OUTPUT/$name.err"; then
    fail "command unexpectedly succeeded: $*"
  fi
}

echo "Building the disposable candidate binary..."
go build -ldflags '-X main.version=ext16-smoke' -o "$REVOLVR_BIN" ./cmd/revolvr

cat >"$CODEX_BIN" <<'FAKE_CODEX'
#!/usr/bin/env bash
set -euo pipefail
if [[ "$#" -eq 1 && "$1" == "--version" ]]; then
  printf 'codex-cli 1.2.3\n'
  exit 0
fi
printf 'unexpected Codex execution\n' >>"${CODEX_CALL_LOG:?}"
exit 64
FAKE_CODEX
chmod 0755 "$CODEX_BIN"
export CODEX_CALL_LOG

git init -q "$REPO"
git -C "$REPO" config user.name "Revolvr Attended Smoke"
git -C "$REPO" config user.email "revolvr-attended-smoke@example.invalid"
printf '# Disposable external project\n' >"$REPO/README.md"
git -C "$REPO" add README.md
git -C "$REPO" commit -q -m "initial fixture"

# Pinned-install and repository admission commands from the runbook.
run_capture version revolvr --version
assert_contains "$OUTPUT/version.out" "revolvr ext16-smoke"
run_capture root-help revolvr --help
assert_contains "$OUTPUT/root-help.out" "Run bounded Codex harness passes"
run_capture revolvr-hash sha256sum "$(command -v revolvr)"
run_capture revolvr-build go version -m "$(command -v revolvr)"
run_capture codex-version codex --version
run_capture codex-hash sha256sum "$(command -v codex)"
run_capture git-root git rev-parse --show-toplevel
run_capture git-bare git rev-parse --is-bare-repository
assert_contains "$OUTPUT/git-bare.out" "false"
run_capture git-submodules git submodule status --recursive
run_capture git-status-before git status --short --branch

run_capture init revolvr init
assert_contains "$OUTPUT/init.out" "Initialized revolvr state:"
test -f "$REPO/.revolvr/ledger.sqlite" || fail "init did not create the ledger"

cat >"$REPO/.revolvr/config.yaml" <<YAML
codex:
  executable: $CODEX_BIN
  model: gpt-5.6-sol
  reasoning_effort: xhigh
  ephemeral: true
  dangerously_bypass_approvals_and_sandbox: true
  timeout_seconds: 1800
git:
  executable: git
  timeout_seconds: 30
verification:
  missing_policy: fail
  commands:
    - name: sh
      args: ["-c", "exit 0"]
      timeout_seconds: 30
commit:
  allow_pre_existing_dirty: false
  allow_missing_verification: false
  timeout_seconds: 30
autonomy:
  schema_version: revolvr-autonomous-safety-declaration-v1
  mode: operator_attended
  external_isolation:
    expectation: none
    enforcement: none
  network:
    access: unknown
    enforcement: none
  hooks:
    policy: operator_attended
  environment:
    inherit_host: true
  redaction:
    schema_version: revolvr-secret-redaction-policy-v1
    environment_variables: []
YAML
chmod 0644 "$REPO/.revolvr/config.yaml"

run_capture protected-modes find .agent .revolvr -xdev -perm /022 -print
test ! -s "$OUTPUT/protected-modes.out" || fail "protected fixture paths are group/world writable"
run_capture config-check revolvr config check
assert_contains "$OUTPUT/config-check.out" "Autonomy mode: operator_attended"
assert_contains "$OUTPUT/config-check.out" "task_attempts=16"
assert_contains "$OUTPUT/config-check.out" "model_tokens=1000000"
assert_contains "$OUTPUT/config-check.out" "retained_disk_bytes=1073741824"

run_capture task-add revolvr task add "Document the disposable fixture" --summary "Attended smoke task"
TASK_ID="$(awk '/^Added task / {value=$3; sub(/:$/, "", value); print value}' "$OUTPUT/task-add.out")"
[[ -n "$TASK_ID" ]] || fail "could not read the created task ID"
export TASK_ID

cat >"$REPO/import-tasks.md" <<'TASKS'
## Task: Imported smoke example

Preview one imported task without changing canonical task files.

### Summary

Demonstrate deterministic Markdown import.

### Acceptance

- The dry run reports one task.

### Verification

- Inspect the dry-run output.
TASKS
run_capture task-import-dry-run revolvr task import import-tasks.md --dry-run
assert_contains "$OUTPUT/task-import-dry-run.out" "Dry run: 1 task(s) would be imported."
rm -f -- "$REPO/import-tasks.md"

git -C "$REPO" add .agent
git -C "$REPO" commit -q -m "add attended smoke task"
run_capture task-list revolvr task list
assert_contains "$OUTPUT/task-list.out" "$TASK_ID"
run_capture migration-plan revolvr task migrate --to autonomous-v1 --dry-run "$TASK_ID"
assert_contains "$OUTPUT/migration-plan.out" "Autonomous migration dry-run: 1 task(s); no files written."
run_capture migration-apply revolvr task migrate --to autonomous-v1 "$TASK_ID"
assert_contains "$OUTPUT/migration-apply.out" "Autonomous migration applied: 1 task(s)."
git -C "$REPO" add .agent
git -C "$REPO" commit -q -m "migrate attended smoke task"

run_capture status revolvr status
run_capture task-show revolvr task show "$TASK_ID"
run_capture task-show-json revolvr task show "$TASK_ID" --json
run_capture task-why revolvr task why "$TASK_ID"
assert_contains "$OUTPUT/task-show.out" "Task ID: $TASK_ID"
assert_contains "$OUTPUT/task-show-json.out" '"source_kind": "active"'

# The fake has a valid version grammar but is deliberately absent from the
# release allowlist. Doctor must inspect it, refuse it, and start no model.
expect_failure doctor-bare revolvr doctor
expect_failure doctor-attended revolvr doctor --for attended-task
cmp "$OUTPUT/doctor-bare.out" "$OUTPUT/doctor-attended.out" || fail "bare and attended doctor output differ"
assert_contains "$OUTPUT/doctor-attended.out" "Ready: false"
expect_failure doctor-task revolvr doctor --for attended-task --task "$TASK_ID"
assert_contains "$OUTPUT/doctor-task.out" "Ready: false"

# Read-only evidence surfaces. Missing selectors exercise safe, non-mutating
# refusals; the export commands exercise positive immutable evidence reads.
run_capture archive-list revolvr archive list
run_capture notification-list revolvr notification list
run_capture metrics-live revolvr metrics show
run_capture metrics-json revolvr metrics show --json

run_capture ledger-export revolvr ledger export \
  --operation-id export-attended-smoke \
  --exported-at 2026-07-16T12:00:00Z
EXPORT_ID="$(awk '/^Ledger export: / {print $3}' "$OUTPUT/ledger-export.out")"
[[ -n "$EXPORT_ID" ]] || fail "could not read export ID"
export EXPORT_ID
run_capture ledger-verify revolvr ledger export verify "$EXPORT_ID"
run_capture ledger-replay revolvr ledger export replay-validate "$EXPORT_ID"
run_capture metrics-export revolvr metrics show --export "$EXPORT_ID"
assert_contains "$OUTPUT/ledger-verify.out" "Ledger export verification: true"
assert_contains "$OUTPUT/ledger-replay.out" "passed=true"

run_capture gc-plan revolvr artifact gc \
  --operation-id gc-attended-smoke \
  --planned-at 2026-07-16T12:05:00Z
assert_contains "$OUTPUT/gc-plan.out" "Artifact GC dry-run"

OPERATION_ID="missing-operation"
RUN_ID="missing-run"
DELIVERY_ID="missing-delivery"
ARCHIVE_SELECTOR="missing-archive"
MISSING_EXPORT_ID="missing-export"
export OPERATION_ID RUN_ID DELIVERY_ID ARCHIVE_SELECTOR MISSING_EXPORT_ID
expect_failure task-recover revolvr task recover "$TASK_ID" --operation-id "$OPERATION_ID"
expect_failure run-show revolvr show "$RUN_ID"
expect_failure receipt-validate revolvr receipt validate "$RUN_ID"
expect_failure notification-show revolvr notification show "$DELIVERY_ID"
expect_failure archive-show revolvr archive show "$ARCHIVE_SELECTOR"
expect_failure archive-verify revolvr archive verify "$ARCHIVE_SELECTOR"
expect_failure ledger-verify-missing revolvr ledger export verify "$MISSING_EXPORT_ID"
expect_failure ledger-replay-missing revolvr ledger export replay-validate "$MISSING_EXPORT_ID"
expect_failure gc-inspect revolvr artifact gc inspect "$OPERATION_ID"

# Read-only task-workspace review commands. The fixture root stands in for the
# execution_root emitted by `task show --json`; the command shapes are exact.
WORKSPACE="$REPO"
TASK_COMMIT="$(git -C "$WORKSPACE" rev-parse HEAD)"
export WORKSPACE TASK_COMMIT
run_capture workspace-status git -C "$WORKSPACE" status --short --branch
run_capture workspace-log git -C "$WORKSPACE" log --oneline --decorate -n 10
run_capture workspace-show git -C "$WORKSPACE" show --stat --oneline "$TASK_COMMIT"
run_capture worktree-list git worktree list --porcelain

# Verify help for every subcommand referenced by the attended runbook,
# including mutation commands that the smoke deliberately does not apply.
help_commands=(
  "init"
  "config check"
  "doctor"
  "status"
  "task"
  "task add"
  "task import"
  "task list"
  "task migrate"
  "task show"
  "task why"
  "task recover"
  "task retry"
  "task unblock"
  "checkpoint fulfill"
  "run"
  "show"
  "receipt validate"
  "ledger"
  "ledger export"
  "ledger export verify"
  "ledger export replay-validate"
  "artifact gc"
  "artifact gc inspect"
  "archive"
  "archive list"
  "archive show"
  "archive verify"
  "archive create"
  "archive reopen"
  "metrics show"
  "notification list"
  "notification show"
  "tui"
)
for command in "${help_commands[@]}"; do
  read -r -a args <<<"$command"
  run_capture "help-${command// /-}" revolvr "${args[@]}" --help
done

test ! -e "$CODEX_CALL_LOG" || fail "a smoke command attempted to execute Codex"

# Exercise the runbook's guarded same-filesystem retirement procedure only in
# this disposable fixture.
(cd "$REPO" && {
  RUNTIME_DIR="$(pwd -P)/.revolvr"
  RETIRED_DIR="$(pwd -P)/.revolvr.retired.$(date -u +%Y%m%dT%H%M%SZ)"
  [[ "$RUNTIME_DIR" == "$(pwd -P)/.revolvr" ]] || exit 64
  [[ -d "$RUNTIME_DIR" && ! -L "$RUNTIME_DIR" ]] || exit 64
  mv -- "$RUNTIME_DIR" "$RETIRED_DIR"
  [[ -d "$RETIRED_DIR" && ! -L "$RETIRED_DIR" ]] || exit 64
  [[ "$RETIRED_DIR" == "$(pwd -P)/.revolvr.retired."* ]] || exit 64
  rm -rf -- "$RETIRED_DIR"
})
test ! -e "$REPO/.revolvr" || fail "runtime state was not retired"

echo "External attended operator runbook smoke test passed."
