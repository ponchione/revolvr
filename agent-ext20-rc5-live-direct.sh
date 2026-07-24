#!/usr/bin/env bash
set -euo pipefail

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"
readonly RUN_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite"
readonly LAUNCH_RECORD_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc5-launch-records"
readonly SUITE_ID="ext20-9b7cc650ea0c"
readonly CANDIDATE_SOURCE="19c1ef4b6a610016487880aa8ad69ec0204bd4f7"
readonly CANDIDATE_SHA256="1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043"
readonly ATTESTATION_COMMIT="109b38cdb309b50c38ab2ef0df33998e92dfd5e6"
readonly AUTHORITY_SHA256="3af0ccb8a7dd7fa5c2205882ce63c03dcb59935cb297fd4804a6a544a4827289"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="44a0bacaf8c9ba0c52e3724553652dedda677a4118947e34b44fefa38671cee7"
readonly REPO_A_HEAD="41b6aa970b209aaad4291d35b04554464c4ba93c"
readonly REPO_B_HEAD="6ae0cd3ff68995b93b9c22162ca395ed03fca157"
readonly SUITE_SCRIPT_SHA256="bd7fcfb15e91db5361b9c4c91471618ad8ac4fe45c98f028e1b439127b0e66f6"
readonly COLLECTOR_SHA256="2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd"
readonly CODEX_SHA256="134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477"

ACTIVE_LAUNCH_RECORD=""
ACTIVE_CHILD_PID=""
ACTIVE_STATUS_FINAL=false

fail() {
	printf 'RC.5 live gate: %s\n' "$*" >&2
	exit 1
}

reserve_launch_record() {
	local parent="$1" launch_id="$2" record
	[[ "$launch_id" =~ ^[A-Za-z0-9._-]+$ ]] || fail "unsafe launch-record identity"
	if [[ ! -e "$parent" && ! -L "$parent" ]]; then
		mkdir -- "$parent" || fail "cannot create launch-record parent"
	fi
	[[ -d "$parent" && ! -L "$parent" ]] || fail "launch-record parent is not a directory"
	[[ "$(cd "$parent" && pwd -P)" == "$parent" ]] || fail "launch-record parent changed identity"
	record="$parent/$launch_id"
	umask 0077
	mkdir -- "$record" || fail "launch-record collision at $record"
	printf '%s\n' "$record"
}

write_status() {
	local record="$1" state="$2" kind="$3" detail="$4" exit_status="$5" temporary
	temporary="$record/status.tsv.tmp.$$"
	{
		printf 'schema_version\trevolvr-ext20-live-launch-status-v1\n'
		printf 'state\t%s\n' "$state"
		printf 'kind\t%s\n' "$kind"
		printf 'detail\t%s\n' "$detail"
		printf 'exit_status\t%s\n' "$exit_status"
		printf 'recorded_at_utc\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	} >"$temporary"
	mv -- "$temporary" "$record/status.tsv"
}

retain_launcher_exit() {
	local status="$1"
	trap - EXIT
	if [[ -n "$ACTIVE_LAUNCH_RECORD" && "$ACTIVE_STATUS_FINAL" != true ]]; then
		write_status "$ACTIVE_LAUNCH_RECORD" failed launcher_exit before_terminal_status "$status" 2>/dev/null || true
	fi
	exit "$status"
}

retain_interruption() {
	local signal="$1" status
	case "$signal" in
	HUP) status=129 ;;
	INT) status=130 ;;
	QUIT) status=131 ;;
	TERM) status=143 ;;
	*) status=1 ;;
	esac
	trap - HUP INT QUIT TERM
	if [[ -n "$ACTIVE_CHILD_PID" ]]; then
		kill -s "$signal" -- "-$ACTIVE_CHILD_PID" 2>/dev/null || kill -s "$signal" "$ACTIVE_CHILD_PID" 2>/dev/null || true
		wait "$ACTIVE_CHILD_PID" 2>/dev/null || true
	fi
	write_status "$ACTIVE_LAUNCH_RECORD" interrupted signal "$signal" "$status" 2>/dev/null || true
	ACTIVE_STATUS_FINAL=true
	exit "$status"
}

write_pre_start_authority() {
	local record="$1" controller_root="$2" controller_head="$3" origin_head="$4" launcher_sha256="$5"
	{
		printf 'schema_version\trevolvr-ext20-live-launch-authority-v1\n'
		printf 'launch_record\t%s\n' "$record"
		printf 'created_at_utc\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		printf 'controller_root\t%s\n' "$controller_root"
		printf 'controller_head\t%s\n' "$controller_head"
		printf 'origin_main\t%s\n' "$origin_head"
		printf 'launcher_sha256\t%s\n' "$launcher_sha256"
		printf 'suite_root\t%s\n' "$RUN_ROOT"
		printf 'suite_id\t%s\n' "$SUITE_ID"
		printf 'candidate_source\t%s\n' "$CANDIDATE_SOURCE"
		printf 'candidate_sha256\t%s\n' "$CANDIDATE_SHA256"
		printf 'attestation_commit\t%s\n' "$ATTESTATION_COMMIT"
		printf 'authority_sha256\t%s\n' "$AUTHORITY_SHA256"
		printf 'plan_sha256\t%s\n' "$PLAN_SHA256"
		printf 'content_sha256\t%s\n' "$CONTENT_SHA256"
		printf 'codex_sha256\t%s\n' "$CODEX_SHA256"
		printf 'repo_a_head\t%s\n' "$REPO_A_HEAD"
		printf 'repo_b_head\t%s\n' "$REPO_B_HEAD"
	} >"$record/pre-start-authority.tsv"
}

main() {
	local check_only=false root candidate codex repo repository sentinel expected_head
	local controller_head origin_head launcher_sha256 launch_id status

	if [[ "$#" -eq 1 && "$1" == "--check" ]]; then
		check_only=true
	elif [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
		printf 'usage: %s [--check | %s]\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
		exit 64
	fi

	root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
	cd "$root"

	[[ "$LAUNCH_RECORD_ROOT" == "$root/.revolvr/ext20-rc5-launch-records" ]] || fail "launch-record root is outside controller runtime state"
	[[ -d "$root/.revolvr" && ! -L "$root/.revolvr" ]] || fail "controller runtime root is unavailable"
	[[ "$(cd "$root/.revolvr" && pwd -P)" == "$root/.revolvr" ]] || fail "controller runtime root changed identity"
	[[ -x scripts/dogfood-external-level1-suite.sh ]] || fail "guarded suite is unavailable"
	[[ -x scripts/dogfood-external-level1.sh ]] || fail "collector is unavailable"
	[[ -d "$RUN_ROOT" ]] || fail "prepared suite is unavailable"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git status --porcelain=v1 --untracked-files=all)" ]] || fail "controller repository is not clean"
	controller_head="$(git rev-parse HEAD)"
	origin_head="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
	[[ "$origin_head" == "$controller_head" ]] || fail "origin/main does not match local HEAD"
	[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.5 | awk '{print $1}')" == "$CANDIDATE_SOURCE" ]] || fail "candidate ref changed"
	[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.5-attestation | awk '{print $1}')" == "$ATTESTATION_COMMIT" ]] || fail "attestation ref changed"
	git check-ignore -q -- .revolvr/ext20-rc5-launch-records/probe || fail "launch-record path is not ignored runtime state"
	command -v setsid >/dev/null 2>&1 || fail "setsid is required for retained live launch status"

	bash -n scripts/dogfood-external-level1-suite.sh scripts/dogfood-external-level1.sh
	[[ "$(sha256sum scripts/dogfood-external-level1-suite.sh | awk '{print $1}')" == "$SUITE_SCRIPT_SHA256" ]] || fail "guarded suite script changed"
	[[ "$(sha256sum scripts/dogfood-external-level1.sh | awk '{print $1}')" == "$COLLECTOR_SHA256" ]] || fail "collector changed"
	.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61/build-instructions.sh \
		--verify .revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61 >/dev/null
	(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared checksum changed"
	[[ "$(awk -F '\t' '$1 == "suite_id" {print $2}' "$RUN_ROOT/authority.tsv")" == "$SUITE_ID" ]] || fail "prepared suite identity changed"
	[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
	[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
	[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "prepared file content changed"

	candidate="$root/.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61/artifacts/revolvr-v0.1.0-linux-amd64"
	codex="$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js"
	[[ "$(sha256sum "$candidate" | awk '{print $1}')" == "$CANDIDATE_SHA256" ]] || fail "candidate binary changed"
	[[ "$("$candidate" --version)" == "revolvr 0.1.0" ]] || fail "candidate version changed"
	[[ "$(sha256sum "$codex" | awk '{print $1}')" == "$CODEX_SHA256" ]] || fail "Codex binary changed"
	[[ "$(node "$codex" --version)" == "codex-cli 0.144.4" ]] || fail "Codex version changed"

	for repo in repo-a repo-b; do
		repository="$RUN_ROOT/repositories/$repo"
		sentinel="$RUN_ROOT/sentinels/$repo"
		expected_head="$REPO_A_HEAD"
		[[ "$repo" == repo-b ]] && expected_head="$REPO_B_HEAD"
		[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" branch --show-current)" == main ]] || fail "$repo branch changed"
		[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" rev-parse HEAD)" == "$expected_head" ]] || fail "$repo HEAD changed"
		[[ -z "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" status --porcelain=v1 --untracked-files=all)" ]] || fail "$repo is dirty"
		[[ -f "$sentinel/value.txt" && ! -L "$sentinel/value.txt" ]] || fail "$repo sentinel changed"
		[[ "$(stat -c '%d:%i:%h' "$sentinel/value.txt")" == "$(stat -c '%d:%i:%h' "$sentinel/value-hardlink.txt")" ]] || fail "$repo sentinel hard link changed"
		[[ "$(stat -c '%h' "$sentinel/value.txt")" == 2 ]] || fail "$repo sentinel link count changed"
		[[ -L "$sentinel/value-link.txt" && "$(readlink "$sentinel/value-link.txt")" == value.txt ]] || fail "$repo sentinel symbolic link changed"
	done

	[[ "$(find "$RUN_ROOT/repositories" -path '*/.agent/tasks/*.md' -type f | wc -l)" -eq 10 ]] || fail "prepared task count changed"
	[[ "$(find "$RUN_ROOT/repositories" -path '*/.revolvr/autonomous/tasks/*/state.json' -type f | wc -l)" -eq 10 ]] || fail "prepared task-state count changed"
	[[ "$(grep -l '"lifecycle": "pending"' "$RUN_ROOT"/repositories/*/.revolvr/autonomous/tasks/*/state.json | wc -l)" -eq 10 ]] || fail "prepared task lifecycle changed"
	[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
	[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
	[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"

	if [[ "$check_only" == true ]]; then
		printf 'RC.5 live gate: preflight passed; no model call or launch record occurred\n'
		exit 0
	fi

	launcher_sha256="$(sha256sum "$0" | awk '{print $1}')"
	launch_id="$SUITE_ID-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	ACTIVE_LAUNCH_RECORD="$(reserve_launch_record "$LAUNCH_RECORD_ROOT" "$launch_id")"
	umask 0077
	trap 'retain_launcher_exit "$?"' EXIT
	trap 'retain_interruption HUP' HUP
	trap 'retain_interruption INT' INT
	trap 'retain_interruption QUIT' QUIT
	trap 'retain_interruption TERM' TERM
	write_pre_start_authority "$ACTIVE_LAUNCH_RECORD" "$root" "$controller_head" "$origin_head" "$launcher_sha256"
	: >"$ACTIVE_LAUNCH_RECORD/suite.stdout"
	: >"$ACTIVE_LAUNCH_RECORD/suite.stderr"
	write_status "$ACTIVE_LAUNCH_RECORD" prepared pre_start none not_started
	sync -f "$ACTIVE_LAUNCH_RECORD" 2>/dev/null || true

	write_status "$ACTIVE_LAUNCH_RECORD" running suite_process admitted not_exited
	set +e
	setsid --wait scripts/dogfood-external-level1-suite.sh \
		--live \
		--run-root "$RUN_ROOT" \
		--confirm-live-real-codex "$LIVE_CONFIRMATION" \
		>"$ACTIVE_LAUNCH_RECORD/suite.stdout" \
		2>"$ACTIVE_LAUNCH_RECORD/suite.stderr" &
	ACTIVE_CHILD_PID=$!
	wait "$ACTIVE_CHILD_PID"
	status=$?
	ACTIVE_CHILD_PID=""
	set -e
	write_status "$ACTIVE_LAUNCH_RECORD" exited suite_exit none "$status"
	ACTIVE_STATUS_FINAL=true
	sync -f "$ACTIVE_LAUNCH_RECORD" 2>/dev/null || true
	trap - EXIT HUP INT QUIT TERM
	[[ "$status" -eq 0 ]] || fail "guarded suite failed with status $status; diagnostics retained at $ACTIVE_LAUNCH_RECORD"
	printf 'RC.5 live gate: guarded suite completed; diagnostics retained at %s\n' "$ACTIVE_LAUNCH_RECORD"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	main "$@"
fi
