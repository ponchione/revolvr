#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

TMP_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/revolvr-smoke.XXXXXX")"
cleanup() {
  rm -rf "$TMP_ROOT"
}
trap cleanup EXIT

BIN="$TMP_ROOT/revolvr"
WORK_DIR="$TMP_ROOT/work"
mkdir -p "$WORK_DIR"

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

run_revolvr() {
  local output_name="$1"
  shift
  echo "Running: revolvr $*"
  (cd "$WORK_DIR" && "$BIN" "$@") >"$TMP_ROOT/$output_name.out"
}

echo "Building revolvr smoke-test binary..."
go build -o "$BIN" ./cmd/revolvr

run_revolvr init init
assert_contains "$TMP_ROOT/init.out" "Initialized revolvr state:"
test -f "$WORK_DIR/.revolvr/ledger.sqlite"

run_revolvr task-add task add --summary "Smoke task" "Exercise local CLI smoke test"
assert_contains "$TMP_ROOT/task-add.out" "Added task "
assert_contains "$TMP_ROOT/task-add.out" "Exercise local CLI smoke test"
assert_contains "$TMP_ROOT/task-add.out" "summary: Smoke task"

run_revolvr task-list task list
assert_contains "$TMP_ROOT/task-list.out" $'ID\tSTATUS\tWORKFLOW\tPHASE'
assert_contains "$TMP_ROOT/task-list.out" "pending"
assert_contains "$TMP_ROOT/task-list.out" "Exercise local CLI smoke test"
assert_contains "$TMP_ROOT/task-list.out" "Smoke task"

run_revolvr config-check config check
assert_contains "$TMP_ROOT/config-check.out" "Config found: false"
assert_contains "$TMP_ROOT/config-check.out" "Codex dangerously bypass approvals and sandbox: true"
assert_contains "$TMP_ROOT/config-check.out" "Verification command count: 0"

run_revolvr status status
assert_contains "$TMP_ROOT/status.out" "Total tasks: 1"
assert_contains "$TMP_ROOT/status.out" "Pending tasks: 1"
assert_contains "$TMP_ROOT/status.out" "Blocked tasks: 0"
assert_contains "$TMP_ROOT/status.out" "Completed tasks: 0"
assert_contains "$TMP_ROOT/status.out" "Recent runs: 0"
assert_contains "$TMP_ROOT/status.out" "Latest run: none"

echo "Local CLI smoke test passed."
