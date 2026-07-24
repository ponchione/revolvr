#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly SCRIPT_NAME="check-ext20-rc5-live-direct"
SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
readonly SOURCE_ROOT
readonly LAUNCHER="$SOURCE_ROOT/agent-ext20-rc5-live-direct.sh"
readonly CHECK_RUN_ROOT="$SOURCE_ROOT/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite"
readonly CHECK_LAUNCH_RECORD_ROOT="$SOURCE_ROOT/.revolvr/ext20-rc5-launch-records"
TEST_ROOT=""

fail_check() {
	printf '%s: %s\n' "$SCRIPT_NAME" "$*" >&2
	exit 1
}

cleanup() {
	if [[ -n "$TEST_ROOT" && "$TEST_ROOT" == /tmp/tmp.* ]]; then
		rm -rf -- "$TEST_ROOT"
	fi
}

snapshot_diagnostics() {
	if [[ -d "$CHECK_LAUNCH_RECORD_ROOT" ]]; then
		find "$CHECK_LAUNCH_RECORD_ROOT" -mindepth 1 -printf '%P\t%y\t%m\t%s\t%l\n' | LC_ALL=C sort
		find "$CHECK_LAUNCH_RECORD_ROOT" -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	fi
}

run_refusal() {
	local expected_status="$1" stdout="$2" stderr="$3"
	shift 3
	local status=0
	set +e
	"$LAUNCHER" "$@" >"$stdout" 2>"$stderr"
	status=$?
	set -e
	[[ "$status" -eq "$expected_status" ]] || fail_check "expected status $expected_status, got $status for launcher arguments"
}

main() {
	local before_diagnostics after_diagnostics before_content after_content
	TEST_ROOT="$(mktemp -d)"
	trap cleanup EXIT
	before_diagnostics="$(snapshot_diagnostics)"
	before_content="$(cd "$CHECK_RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')"

	run_refusal 64 "$TEST_ROOT/missing.stdout" "$TEST_ROOT/missing.stderr"
	run_refusal 64 "$TEST_ROOT/wrong.stdout" "$TEST_ROOT/wrong.stderr" WRONG_CONFIRMATION
	grep -Fq 'usage:' "$TEST_ROOT/missing.stderr" || fail_check "missing confirmation did not report usage"
	grep -Fq 'usage:' "$TEST_ROOT/wrong.stderr" || fail_check "wrong confirmation did not report usage"

	set +e
	"$LAUNCHER" --check >"$TEST_ROOT/check.stdout" 2>"$TEST_ROOT/check.stderr"
	local check_status=$?
	set -e
	if [[ "$check_status" -ne 0 ]]; then
		grep -Fq 'controller repository is not clean' "$TEST_ROOT/check.stderr" || fail_check "check-only failed outside the expected unpublished-tree guard"
	fi

	# Loading the function definitions cannot invoke main. Exercise only the
	# collision boundary against an isolated pre-existing record.
	# shellcheck source=../agent-ext20-rc5-live-direct.sh
	source "$LAUNCHER"
	mkdir -p "$TEST_ROOT/records/existing"
	set +e
	(reserve_launch_record "$TEST_ROOT/records" existing) >"$TEST_ROOT/collision.stdout" 2>"$TEST_ROOT/collision.stderr"
	local collision_status=$?
	set -e
	[[ "$collision_status" -eq 1 ]] || fail_check "existing launch-record collision was admitted"
	grep -Fq 'launch-record collision' "$TEST_ROOT/collision.stderr" || fail_check "collision refusal was not diagnostic"

	after_diagnostics="$(snapshot_diagnostics)"
	[[ "$after_diagnostics" == "$before_diagnostics" ]] || fail_check "a refusal or check-only path changed launch diagnostics"
	after_content="$(cd "$CHECK_RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')"
	[[ "$after_content" == "$before_content" ]] || fail_check "focused checks changed the prepared suite"
	[[ "$(find "$CHECK_RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail_check "focused checks exercised an operation"
	[[ "$(find "$CHECK_RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail_check "focused checks exercised the collector"
	[[ "$(find "$CHECK_RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail_check "focused checks changed aggregate state"
	printf 'RC.5 direct live gate checks passed; no live path or launch record was exercised.\n'
}

main "$@"
