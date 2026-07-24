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

snapshot_tree() {
	local root="$1"
	[[ -d "$root" && ! -L "$root" ]] || fail_check "retained authority is unavailable: $root"
	(
		cd "$root"
		find . -printf '%P\t%y\t%m\t%s\t%l\n' | LC_ALL=C sort
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum
	)
}

run_retired_refusal() {
	local name="$1"
	shift
	local status=0 stdout="$TEST_ROOT/$name.stdout" stderr="$TEST_ROOT/$name.stderr"
	set +e
	"$LAUNCHER" "$@" >"$stdout" 2>"$stderr"
	status=$?
	set -e
	[[ "$status" -eq 1 ]] || fail_check "$name returned $status instead of retired status 1"
	[[ ! -s "$stdout" ]] || fail_check "$name wrote unexpected stdout"
	grep -Fxq 'RC.5 live gate: retired terminal authority; RC.5 has no check or live execution path' "$stderr" || fail_check "$name did not report exact retirement"
}

main() {
	local before_suite after_suite before_records after_records
	TEST_ROOT="$(mktemp -d)"
	trap cleanup EXIT

	bash -n "$LAUNCHER"
	if grep -Eq 'dogfood-external-level1-suite|setsid|--live|reserve_launch_record|write_status' "$LAUNCHER"; then
		fail_check "retired launcher still contains a live or diagnostic mutation primitive"
	fi

	before_suite="$(snapshot_tree "$CHECK_RUN_ROOT")"
	before_records="$(snapshot_tree "$CHECK_LAUNCH_RECORD_ROOT")"

	run_retired_refusal missing
	run_retired_refusal former-check --check
	run_retired_refusal arbitrary-argument WRONG_CONFIRMATION
	run_retired_refusal multiple-arguments first second

	after_suite="$(snapshot_tree "$CHECK_RUN_ROOT")"
	after_records="$(snapshot_tree "$CHECK_LAUNCH_RECORD_ROOT")"
	[[ "$after_suite" == "$before_suite" ]] || fail_check "retired launcher checks changed the retained suite"
	[[ "$after_records" == "$before_records" ]] || fail_check "retired launcher checks changed the retained launch record"
	printf 'RC.5 direct live gate is permanently retired; all checked arguments refused without retained-evidence mutation.\n'
}

main "$@"
