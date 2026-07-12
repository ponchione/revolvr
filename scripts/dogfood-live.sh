#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/revolvr-live-dogfood.XXXXXX")"
cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

BIN="$TMP_ROOT/revolvr"
DOGFOOD_FILE="${REVOLVR_DOGFOOD_FILE:-docs/dogfood-live.txt}"
RUN_TAG="${REVOLVR_DOGFOOD_TAG:-$(date -u +%Y%m%dT%H%M%SZ)}"
EXPECTED_LINE="live dogfood run $RUN_TAG"
TASK_SUMMARY="Live dogfood $RUN_TAG"
TASK_TEXT="Live dogfood task: create or update $DOGFOOD_FILE with exactly one line: $EXPECTED_LINE. Keep the change limited to $DOGFOOD_FILE, update the configured receipt, run go test ./..., and stop."
TASK_ID="live-dogfood-$RUN_TAG"
TASK_REL=".agent/tasks/$TASK_ID.md"
TASK_FILE="$ROOT/$TASK_REL"
CODEX_TIMEOUT_SECONDS="${REVOLVR_DOGFOOD_CODEX_TIMEOUT_SECONDS:-1800}"
VERIFICATION_TIMEOUT_SECONDS="${REVOLVR_DOGFOOD_VERIFICATION_TIMEOUT_SECONDS:-300}"

fail() {
  echo "$*" >&2
  exit 1
}

assert_numeric() {
  local name="$1"
  local value="$2"
  if [[ -z "$value" || "$value" == *[!0-9]* ]]; then
    fail "$name must be a positive integer, got: $value"
  fi
  if [[ "$value" == "0" ]]; then
    fail "$name must be greater than zero"
  fi
}

assert_contains() {
  local file="$1"
  local expected="$2"
  if ! grep -Fq "$expected" "$file"; then
    echo "Expected $file to contain:" >&2
    echo "  $expected" >&2
    echo "Actual output:" >&2
    sed 's/^/  /' "$file" >&2
    exit 1
  fi
}

assert_file_exists() {
  local path="$1"
  if [[ ! -f "$path" ]]; then
    fail "Expected file to exist: $path"
  fi
}

assert_nonempty() {
  local name="$1"
  local value="$2"
  if [[ -z "$value" ]]; then
    fail "Expected non-empty $name"
  fi
}

assert_clean_worktree() {
	local label="$1"
	local output="$TMP_ROOT/git-status-$label.out"
	git status --short --untracked-files=all >"$output"
	if [[ -s "$output" ]]; then
		echo "Expected clean Git worktree during $label." >&2
		echo "This live dogfood script resets .revolvr/ and creates task baseline plus run commits, so start from a clean tree." >&2
		sed 's/^/  /' "$output" >&2
		exit 1
	fi
}

run_revolvr() {
  local output_name="$1"
  shift
  echo "Running: revolvr $*"
  if ! "$BIN" "$@" >"$TMP_ROOT/$output_name.out" 2>"$TMP_ROOT/$output_name.err"; then
    echo "revolvr $* failed" >&2
    echo "stdout:" >&2
    sed 's/^/  /' "$TMP_ROOT/$output_name.out" >&2
    echo "stderr:" >&2
    sed 's/^/  /' "$TMP_ROOT/$output_name.err" >&2
    exit 1
  fi
}

assert_numeric "REVOLVR_DOGFOOD_CODEX_TIMEOUT_SECONDS" "$CODEX_TIMEOUT_SECONDS"
assert_numeric "REVOLVR_DOGFOOD_VERIFICATION_TIMEOUT_SECONDS" "$VERIFICATION_TIMEOUT_SECONDS"

command -v codex >/dev/null || fail "codex executable not found on PATH"
git config --get user.name >/dev/null || fail "Git user.name is not configured"
git config --get user.email >/dev/null || fail "Git user.email is not configured"

assert_clean_worktree "start"

echo "Building revolvr live-dogfood binary..."
go build -o "$BIN" ./cmd/revolvr

echo "Resetting .revolvr runtime state..."
rm -rf .revolvr

run_revolvr init init
assert_contains "$TMP_ROOT/init.out" "Initialized revolvr state:"
assert_file_exists ".revolvr/ledger.sqlite"
assert_contains ".git/info/exclude" "/.revolvr/"
git check-ignore --quiet .revolvr/ || fail "Expected .revolvr/ to be ignored by Git"
assert_file_exists ".agent/profiles/implementer.md"
assert_file_exists ".agent/profiles/auditor.md"
assert_file_exists ".agent/profiles/documentor.md"

mkdir -p "$(dirname "$TASK_FILE")"
cat >"$TASK_FILE" <<TASK_MD
---
id: $TASK_ID
status: pending
priority: 10
---
# $TASK_SUMMARY

$TASK_TEXT
TASK_MD
TASK_SHA="$(sha256sum "$TASK_FILE" | awk '{print $1}')"
TASK_BYTES="$(wc -c <"$TASK_FILE" | tr -d ' ')"
git add .agent/profiles "$TASK_REL"
git commit -q -m "Add live dogfood task $RUN_TAG"
assert_clean_worktree "task-baseline"

cat >".revolvr/config.yaml" <<CONFIG_YAML
codex:
  timeout_seconds: $CODEX_TIMEOUT_SECONDS
autonomy:
  mode: operator_attended
verification:
  missing_policy: fail
  commands:
    - name: go
      args:
        - test
        - ./...
      timeout_seconds: $VERIFICATION_TIMEOUT_SECONDS
CONFIG_YAML

run_revolvr config-check config check
assert_contains "$TMP_ROOT/config-check.out" "Config found: true"
assert_contains "$TMP_ROOT/config-check.out" "Codex executable: codex"
assert_contains "$TMP_ROOT/config-check.out" "Verification command count: 1"
assert_contains "$TMP_ROOT/config-check.out" 'Verification command 0: name=go args=["test", "./..."]'

run_revolvr doctor doctor
assert_contains "$TMP_ROOT/doctor.out" "Dogfood preflight:"
assert_contains "$TMP_ROOT/doctor.out" "OK worktree clean: no changes"
assert_contains "$TMP_ROOT/doctor.out" "OK runtime state ignored: .revolvr/ ignored by Git"
assert_contains "$TMP_ROOT/doctor.out" "OK verification commands: 1 command configured"
assert_contains "$TMP_ROOT/doctor.out" "Ready: true"

run_revolvr run-once run --once
assert_contains "$TMP_ROOT/run-once.out" "Run "
assert_contains "$TMP_ROOT/run-once.out" " completed task "
assert_contains "$TMP_ROOT/run-once.out" "; commit "

RUN_ID="$(awk '/^Run / && / completed task / {print $2; exit}' "$TMP_ROOT/run-once.out")"
RUN_COMMIT="$(sed -n 's/^Run .*; commit \([0-9a-f][0-9a-f]*\)\.$/\1/p' "$TMP_ROOT/run-once.out" | head -n 1)"
assert_nonempty "run id" "$RUN_ID"
assert_nonempty "run commit" "$RUN_COMMIT"

HEAD_SHA="$(git rev-parse --verify HEAD)"
if [[ "$HEAD_SHA" != "$RUN_COMMIT" ]]; then
  fail "Run summary commit $RUN_COMMIT did not match HEAD $HEAD_SHA"
fi

assert_file_exists "$DOGFOOD_FILE"
if [[ "$(tr -d '\r' <"$DOGFOOD_FILE")" != "$EXPECTED_LINE" ]]; then
  echo "Expected $DOGFOOD_FILE to contain exactly:" >&2
  echo "  $EXPECTED_LINE" >&2
  echo "Actual content:" >&2
  sed 's/^/  /' "$DOGFOOD_FILE" >&2
	exit 1
fi
assert_contains "$TASK_FILE" "status: completed"

mapfile -t COMMITTED_FILES < <(git show --name-only --format= HEAD | sed '/^$/d')
printf '%s\n' "${COMMITTED_FILES[@]}" >"$TMP_ROOT/committed-files.out"
if [[ "${#COMMITTED_FILES[@]}" -ne 2 ]] || ! grep -Fxq "$DOGFOOD_FILE" "$TMP_ROOT/committed-files.out" || ! grep -Fxq "$TASK_REL" "$TMP_ROOT/committed-files.out"; then
  echo "Expected live dogfood commit to contain $DOGFOOD_FILE and $TASK_REL." >&2
  printf '  %s\n' "${COMMITTED_FILES[@]}" >&2
  exit 1
fi
git show --format=%s --no-patch HEAD >"$TMP_ROOT/git-show-subject.out"
assert_contains "$TMP_ROOT/git-show-subject.out" "$TASK_SUMMARY"

assert_file_exists ".revolvr/runs/$RUN_ID/context.md"
assert_file_exists ".revolvr/runs/$RUN_ID/context.json"
assert_file_exists ".revolvr/runs/$RUN_ID/codex.jsonl"
assert_file_exists ".revolvr/runs/$RUN_ID/codex.stderr"
assert_file_exists ".revolvr/runs/$RUN_ID/last-message.txt"
assert_file_exists ".revolvr/receipts/$RUN_ID.md"
assert_contains ".revolvr/runs/$RUN_ID/context.md" "## Run Profile"
assert_contains ".revolvr/runs/$RUN_ID/context.md" "## Selected Task"
assert_contains ".revolvr/runs/$RUN_ID/context.md" "## Required Receipt Schema"
assert_contains ".revolvr/runs/$RUN_ID/context.json" '"context_payload_path"'
assert_contains ".revolvr/runs/$RUN_ID/context.json" '"context_payload_sha256"'
assert_contains ".revolvr/runs/$RUN_ID/context.json" '"label": "selected_task"'
assert_contains ".revolvr/runs/$RUN_ID/context.json" "\"path\": \"$TASK_REL\""
assert_contains ".revolvr/runs/$RUN_ID/context.json" "\"sha256\": \"$TASK_SHA\""
assert_contains ".revolvr/runs/$RUN_ID/context.json" "\"byte_size\": $TASK_BYTES"
assert_contains ".revolvr/receipts/$RUN_ID.md" "schema_version: revolvr.receipt.v1"
assert_contains ".revolvr/receipts/$RUN_ID.md" "run_id: $RUN_ID"
assert_contains ".revolvr/receipts/$RUN_ID.md" "verdict: completed"
assert_contains ".revolvr/receipts/$RUN_ID.md" "verification_status: passed"
assert_contains ".revolvr/receipts/$RUN_ID.md" "commit_sha: $HEAD_SHA"
assert_contains ".revolvr/receipts/$RUN_ID.md" "$DOGFOOD_FILE"
assert_contains ".revolvr/receipts/$RUN_ID.md" "$TASK_REL"

run_revolvr status status
assert_contains "$TMP_ROOT/status.out" "Recent runs: 1"
assert_contains "$TMP_ROOT/status.out" "Latest run: $RUN_ID (completed)"
assert_contains "$TMP_ROOT/status.out" "Latest verification: passed"
assert_contains "$TMP_ROOT/status.out" "Latest commit: $HEAD_SHA"

run_revolvr show-run show "$RUN_ID"
assert_contains "$TMP_ROOT/show-run.out" "Run ID: $RUN_ID"
assert_contains "$TMP_ROOT/show-run.out" "Status: completed"
assert_contains "$TMP_ROOT/show-run.out" "Verification status: passed"
assert_contains "$TMP_ROOT/show-run.out" "Commit SHA: $HEAD_SHA"
assert_contains "$TMP_ROOT/show-run.out" "Artifacts:"
assert_contains "$TMP_ROOT/show-run.out" "context payload: .revolvr/runs/$RUN_ID/context.md"
assert_contains "$TMP_ROOT/show-run.out" "context manifest: .revolvr/runs/$RUN_ID/context.json"
assert_contains "$TMP_ROOT/show-run.out" "codex stdout jsonl: .revolvr/runs/$RUN_ID/codex.jsonl"
assert_contains "$TMP_ROOT/show-run.out" "receipt: .revolvr/receipts/$RUN_ID.md"
assert_contains "$TMP_ROOT/show-run.out" "Diagnostics:"
assert_contains "$TMP_ROOT/show-run.out" "outcome: committed"
assert_contains "$TMP_ROOT/show-run.out" "verification: passed"
assert_contains "$TMP_ROOT/show-run.out" "commit: $HEAD_SHA"
assert_contains "$TMP_ROOT/show-run.out" "receipt: completed (.revolvr/receipts/$RUN_ID.md)"
assert_contains "$TMP_ROOT/show-run.out" "changed files:"
assert_contains "$TMP_ROOT/show-run.out" "$DOGFOOD_FILE"
assert_contains "$TMP_ROOT/show-run.out" "$TASK_REL"

run_revolvr receipt-validate receipt validate "$RUN_ID"
assert_contains "$TMP_ROOT/receipt-validate.out" "Receipt validation: passed"
assert_contains "$TMP_ROOT/receipt-validate.out" "identity: ok"
assert_contains "$TMP_ROOT/receipt-validate.out" "completion_time: ok"
assert_contains "$TMP_ROOT/receipt-validate.out" "commit_sha: ok"
assert_contains "$TMP_ROOT/receipt-validate.out" "changed_files: ok"
assert_contains "$TMP_ROOT/receipt-validate.out" "verification_results: ok"
assert_contains "$TMP_ROOT/receipt-validate.out" "artifacts: ok"

assert_clean_worktree "finish"

echo "Live dogfood run passed: run=$RUN_ID commit=$HEAD_SHA"
