#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly SCRIPT_NAME="check-ext20-rc7-live-direct"
SOURCE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
readonly SOURCE_ROOT
readonly LAUNCHER="$SOURCE_ROOT/agent-ext20-rc7-live-direct.sh"
readonly CHECK_RUN_ROOT="$SOURCE_ROOT/.revolvr/ext20-rc7.rpIUM5/suite"
readonly CHECK_LAUNCH_RECORD_ROOT="$SOURCE_ROOT/.revolvr/ext20-rc7-launch-records"
readonly RC6_SUITE="$SOURCE_ROOT/.revolvr/ext20-rc6.LOLauh/suite"
readonly RC6_LAUNCH_RECORD="$SOURCE_ROOT/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365"
readonly RC6_TERMINAL_EVIDENCE="$RC6_SUITE/evidence/repo-a/01-successful-source-change-1"
readonly TASKS_SHA256="33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448"
readonly RC7_CONTENT_SHA256="2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943"
readonly RC6_SUITE_SHA256="d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b"
readonly RC6_LAUNCH_SHA256="2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce"
readonly RC6_TERMINAL_SHA256="e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259"
TEST_ROOT=""

fail_check() {
	printf '%s: %s\n' "$SCRIPT_NAME" "$*" >&2
	exit 1
}

cleanup() {
	if [[ -n "$TEST_ROOT" && "$TEST_ROOT" == /tmp/tmp.* && -d "$TEST_ROOT" && ! -L "$TEST_ROOT" ]]; then
		find "$TEST_ROOT" -depth -delete
	fi
}

content_stream_sha256_check() {
	local root="$1"
	[[ -d "$root" && ! -L "$root" ]] || fail_check "protected content root is unavailable: $root"
	(
		cd "$root"
		find . -type f -print0 | sort -z | xargs -0 -r sha256sum | sha256sum | awk '{print $1}'
	)
}

assert_zero_activity() {
	local repo state
	[[ -z "$(find "$CHECK_RUN_ROOT/evidence" -type f -print -quit)" ]] || fail_check "focused checks created collector evidence"
	[[ -z "$(find "$CHECK_RUN_ROOT" -type f -name operation.tsv -print -quit)" ]] || fail_check "focused checks created operation evidence"
	[[ -z "$(find "$CHECK_RUN_ROOT/aggregate" -mindepth 1 -print -quit)" ]] || fail_check "focused checks changed aggregate state"
	cmp -s \
		<(find "$CHECK_RUN_ROOT/logs" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | sort) \
		<(printf '%s\n' npm-install.err npm-install.out) || fail_check "focused checks created operation logs"
	[[ "$(sha256sum "$CHECK_RUN_ROOT/logs/npm-install.out" | awk '{print $1}')" == "65c731cd02e19c79f6f5a3e84a4dd64a49acd6c47d2ee551d7cc9da191e8c96c" ]] || fail_check "focused checks changed the npm preparation log"
	[[ "$(sha256sum "$CHECK_RUN_ROOT/logs/npm-install.err" | awk '{print $1}')" == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]] || fail_check "focused checks changed the npm preparation error log"
	[[ ! -e "$CHECK_LAUNCH_RECORD_ROOT" && ! -L "$CHECK_LAUNCH_RECORD_ROOT" ]] || fail_check "focused checks created an RC.7 launch record"
	for repo in repo-a repo-b; do
		[[ -z "$(find "$CHECK_RUN_ROOT/repositories/$repo/.revolvr/runs" -mindepth 1 -print -quit)" ]] || fail_check "focused checks created a $repo model run"
		[[ -z "$(find "$CHECK_RUN_ROOT/repositories/$repo/.revolvr/receipts" -mindepth 1 -print -quit)" ]] || fail_check "focused checks created a $repo receipt"
		while IFS= read -r state; do
			jq -e '.lifecycle == "pending" and .attempts.total_attempts == 0 and .attempts.consecutive_failures == 0' "$state" >/dev/null || fail_check "focused checks changed a $repo task state"
		done < <(find "$CHECK_RUN_ROOT/repositories/$repo/.revolvr/autonomous/tasks" -name state.json -type f | sort)
	done
}

run_refusal() {
	local name="$1" expected_status="$2" expected_message="$3" working_directory="$4"
	shift 4
	local status=0 stdout="$TEST_ROOT/$name.stdout" stderr="$TEST_ROOT/$name.stderr"
	set +e
	(cd "$working_directory" && "$LAUNCHER" "$@") >"$stdout" 2>"$stderr"
	status=$?
	set -e
	[[ "$status" -eq "$expected_status" ]] || fail_check "$name returned $status instead of $expected_status"
	[[ ! -s "$stdout" ]] || fail_check "$name wrote unexpected stdout"
	grep -Fq "$expected_message" "$stderr" || fail_check "$name did not report the expected refusal"
}

initialize_repository() {
	local repository="$1"
	git init -q -b main "$repository"
	git -C "$repository" config user.name "RC.7 focused checker"
	git -C "$repository" config user.email "rc7-checker@example.invalid"
	printf 'initial\n' >"$repository/tracked.txt"
	git -C "$repository" add -- tracked.txt
	git -C "$repository" commit -q -m initial
}

main() {
	local before_rc7 before_rc6_suite before_rc6_launch before_rc6_terminal before_tasks
	local after_rc7 after_rc6_suite after_rc6_launch after_rc6_terminal load_status collision_status current_check_status
	TEST_ROOT="$(mktemp -d)"
	trap cleanup EXIT

	before_rc7="$(content_stream_sha256_check "$CHECK_RUN_ROOT")"
	before_rc6_suite="$(content_stream_sha256_check "$RC6_SUITE")"
	before_rc6_launch="$(content_stream_sha256_check "$RC6_LAUNCH_RECORD")"
	before_rc6_terminal="$(content_stream_sha256_check "$RC6_TERMINAL_EVIDENCE")"
	before_tasks="$(sha256sum "$SOURCE_ROOT/.agent/TASKS.md" | awk '{print $1}')"
	[[ "$before_rc7" == "$RC7_CONTENT_SHA256" ]] || fail_check "prepared RC.7 suite does not match its authority"
	[[ "$before_rc6_suite" == "$RC6_SUITE_SHA256" ]] || fail_check "protected RC.6 suite does not match its authority"
	[[ "$before_rc6_launch" == "$RC6_LAUNCH_SHA256" ]] || fail_check "protected RC.6 launch record does not match its authority"
	[[ "$before_rc6_terminal" == "$RC6_TERMINAL_SHA256" ]] || fail_check "protected RC.6 terminal evidence does not match its authority"
	[[ "$before_tasks" == "$TASKS_SHA256" ]] || fail_check "task backlog changed before focused checks"
	assert_zero_activity

	bash -n "$LAUNCHER" "$SOURCE_ROOT/scripts/check-ext20-rc7-live-direct.sh"
	mkdir "$TEST_ROOT/no-repository"
	run_refusal missing 64 'usage:' "$TEST_ROOT/no-repository"
	run_refusal wrong 64 'usage:' "$TEST_ROOT/no-repository" WRONG_CONFIRMATION
	run_refusal multiple 64 'usage:' "$TEST_ROOT/no-repository" --check extra

	set +e
	(cd "$TEST_ROOT/no-repository" && bash -c 'source "$1"; declare -F main >/dev/null; declare -F reserve_launch_record >/dev/null' _ "$LAUNCHER") \
		>"$TEST_ROOT/load.stdout" 2>"$TEST_ROOT/load.stderr"
	load_status=$?
	set -e
	[[ "$load_status" -eq 0 ]] || fail_check "loading launcher definitions failed"
	[[ ! -s "$TEST_ROOT/load.stdout" && ! -s "$TEST_ROOT/load.stderr" ]] || fail_check "loading launcher definitions invoked main or produced output"

	mkdir "$TEST_ROOT/records"
	set +e
	bash -c 'source "$1"; reserve_launch_record "$2" isolated-collision' \
		_ "$LAUNCHER" "$TEST_ROOT/records" \
		>"$TEST_ROOT/collision.stdout" 2>"$TEST_ROOT/collision.stderr"
	collision_status=$?
	set -e
	[[ "$collision_status" -eq 1 ]] || fail_check "an existing isolated launch-record root was admitted"
	grep -Fq 'launch-record collision' "$TEST_ROOT/collision.stderr" || fail_check "isolated launch-record collision lacked a diagnostic"
	[[ -z "$(find "$TEST_ROOT/records" -mindepth 1 -print -quit)" ]] || fail_check "collision check changed the isolated launch-record root"

	initialize_repository "$TEST_ROOT/dirty-controller"
	printf 'dirty\n' >"$TEST_ROOT/dirty-controller/untracked.txt"
	run_refusal dirty-check 1 'controller repository is not clean' "$TEST_ROOT/dirty-controller" --check

	git init -q --bare "$TEST_ROOT/unpublished-origin.git"
	initialize_repository "$TEST_ROOT/unpublished-controller"
	git -C "$TEST_ROOT/unpublished-controller" remote add origin "$TEST_ROOT/unpublished-origin.git"
	git -C "$TEST_ROOT/unpublished-controller" push -q -u origin main
	printf 'unpublished\n' >>"$TEST_ROOT/unpublished-controller/tracked.txt"
	git -C "$TEST_ROOT/unpublished-controller" commit -q -am unpublished
	run_refusal unpublished-check 1 'local main does not match origin/main' "$TEST_ROOT/unpublished-controller" --check

	[[ -n "$(GIT_OPTIONAL_LOCKS=0 git -C "$SOURCE_ROOT" status --porcelain=v1 --untracked-files=all)" ]] || fail_check "construction check expected an unpublished controller delta"
	set +e
	(cd "$SOURCE_ROOT" && "$LAUNCHER" --check) >"$TEST_ROOT/current-check.stdout" 2>"$TEST_ROOT/current-check.stderr"
	current_check_status=$?
	set -e
	[[ "$current_check_status" -eq 1 ]] || fail_check "construction check-only path returned $current_check_status instead of refusing unpublished work"
	grep -Fq 'controller repository is not clean' "$TEST_ROOT/current-check.stderr" || fail_check "construction check-only refusal was not at the clean-controller gate"

	assert_zero_activity
	after_rc7="$(content_stream_sha256_check "$CHECK_RUN_ROOT")"
	after_rc6_suite="$(content_stream_sha256_check "$RC6_SUITE")"
	after_rc6_launch="$(content_stream_sha256_check "$RC6_LAUNCH_RECORD")"
	after_rc6_terminal="$(content_stream_sha256_check "$RC6_TERMINAL_EVIDENCE")"
	[[ "$after_rc7" == "$before_rc7" ]] || fail_check "focused checks changed the prepared RC.7 suite"
	[[ "$after_rc6_suite" == "$before_rc6_suite" ]] || fail_check "focused checks changed the protected RC.6 suite"
	[[ "$after_rc6_launch" == "$before_rc6_launch" ]] || fail_check "focused checks changed the protected RC.6 launch record"
	[[ "$after_rc6_terminal" == "$before_rc6_terminal" ]] || fail_check "focused checks changed the protected RC.6 terminal evidence"
	[[ "$(sha256sum "$SOURCE_ROOT/.agent/TASKS.md" | awk '{print $1}')" == "$before_tasks" ]] || fail_check "focused checks changed the task backlog"
	printf 'RC.7 direct live gate focused checks passed; only refusal and isolated no-model paths were exercised.\n'
}

main "$@"
