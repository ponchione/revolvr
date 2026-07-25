#!/usr/bin/env bash
set -euo pipefail

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"
readonly RUN_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite"
readonly LAUNCH_RECORD_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records"
readonly SUITE_ID="ext20-7b4a5932090f"
readonly SUITE_AUTHORITY_COMMIT="87be94f8fc4f04cb25c40598cc1f44cfe3b57efe"
readonly CANDIDATE_SOURCE="73f1f81f1c51d927114f19818a18161d0fcb8541"
readonly CANDIDATE_SHA256="f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d"
readonly ATTESTATION_COMMIT="226276f151ae389d06c0118a931596712fbc7cc1"
readonly ATTESTATION_ARTIFACT_ID="8618790256"
readonly ATTESTATION_ARTIFACT_DIGEST="sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa"
readonly AUTHORITY_SHA256="5d90db1fca978451f0fa9b0950bc71dd9b334a00e0647d7e16158698a2358e40"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="cb75bb94dca396d14d856e001fa1ed3a7d8d6ac46cf8c5d60eed2ca902f033c0"
readonly REPO_A_HEAD="fa611a1e2c72f5469095c3460dc3268cd21765c8"
readonly REPO_B_HEAD="a73c742f2a0e5934d6da12069599dfde2a17b00d"
readonly SUITE_SCRIPT_SHA256="d16caafba9fb5fb8db83188d87006e57c8b77d88159d28502bb736e93672a3d7"
readonly COLLECTOR_SHA256="2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd"
readonly CODEX_SHA256="134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477"

ACTIVE_LAUNCH_RECORD=""
ACTIVE_CHILD_PID=""
ACTIVE_STATUS_FINAL=false

fail() {
	printf 'RC.6 live gate: %s\n' "$*" >&2
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
		printf 'suite_authority_commit\t%s\n' "$SUITE_AUTHORITY_COMMIT"
		printf 'suite_root\t%s\n' "$RUN_ROOT"
		printf 'suite_id\t%s\n' "$SUITE_ID"
		printf 'candidate_source\t%s\n' "$CANDIDATE_SOURCE"
		printf 'candidate_sha256\t%s\n' "$CANDIDATE_SHA256"
		printf 'attestation_commit\t%s\n' "$ATTESTATION_COMMIT"
		printf 'attestation_artifact_id\t%s\n' "$ATTESTATION_ARTIFACT_ID"
		printf 'attestation_artifact_digest\t%s\n' "$ATTESTATION_ARTIFACT_DIGEST"
		printf 'authority_sha256\t%s\n' "$AUTHORITY_SHA256"
		printf 'plan_sha256\t%s\n' "$PLAN_SHA256"
		printf 'content_sha256\t%s\n' "$CONTENT_SHA256"
		printf 'codex_sha256\t%s\n' "$CODEX_SHA256"
		printf 'repo_a_head\t%s\n' "$REPO_A_HEAD"
		printf 'repo_b_head\t%s\n' "$REPO_B_HEAD"
	} >"$record/pre-start-authority.tsv"
}

verify_remote_artifact() {
	local response
	response="$(curl --fail --silent --show-error --location \
		--header 'Accept: application/vnd.github+json' \
		--header 'X-GitHub-Api-Version: 2022-11-28' \
		"https://api.github.com/repos/ponchione/revolvr/actions/artifacts/$ATTESTATION_ARTIFACT_ID")" || fail "attestation artifact readback failed"
	[[ "$(jq -r '.id|tostring' <<<"$response")" == "$ATTESTATION_ARTIFACT_ID" ]] || fail "attestation artifact identity changed"
	[[ "$(jq -r '.name' <<<"$response")" == "level1-v0.1.0-rc.6-attestation" ]] || fail "attestation artifact name changed"
	[[ "$(jq -r '.digest' <<<"$response")" == "$ATTESTATION_ARTIFACT_DIGEST" ]] || fail "attestation artifact digest changed"
	[[ "$(jq -r '.expired' <<<"$response")" == false ]] || fail "attestation artifact expired"
}

main() {
	local check_only=false root candidate codex repo repository sentinel expected_head task doctor_output
	local controller_head origin_head launcher_sha256 launch_id status

	if [[ "$#" -eq 1 && "$1" == "--check" ]]; then
		check_only=true
	elif [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
		printf 'usage: %s [--check | %s]\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
		exit 64
	fi

	root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
	cd "$root"

	[[ "$LAUNCH_RECORD_ROOT" == "$root/.revolvr/ext20-rc6-launch-records" ]] || fail "launch-record root is outside controller runtime state"
	[[ -d "$root/.revolvr" && ! -L "$root/.revolvr" ]] || fail "controller runtime root is unavailable"
	[[ "$(cd "$root/.revolvr" && pwd -P)" == "$root/.revolvr" ]] || fail "controller runtime root changed identity"
	[[ -x scripts/dogfood-external-level1-suite.sh ]] || fail "guarded suite is unavailable"
	[[ -x scripts/dogfood-external-level1.sh ]] || fail "collector is unavailable"
	[[ -d "$RUN_ROOT" && ! -L "$RUN_ROOT" ]] || fail "prepared suite is unavailable"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git status --porcelain=v1 --untracked-files=all)" ]] || fail "controller repository is not clean"
	controller_head="$(git rev-parse HEAD)"
	origin_head="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
	[[ "$origin_head" == "$controller_head" ]] || fail "origin/main does not match local HEAD"
	git merge-base --is-ancestor "$SUITE_AUTHORITY_COMMIT" "$controller_head" || fail "suite authority commit is not published ancestry"
	[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.6 | awk '{print $1}')" == "$CANDIDATE_SOURCE" ]] || fail "candidate ref changed"
	[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.6-attestation | awk '{print $1}')" == "$ATTESTATION_COMMIT" ]] || fail "attestation ref changed"
	git check-ignore -q -- .revolvr/ext20-rc6-launch-records/probe || fail "launch-record path is not ignored runtime state"
	command -v curl >/dev/null 2>&1 || fail "curl is required for artifact readback"
	command -v jq >/dev/null 2>&1 || fail "jq is required for artifact readback"
	command -v setsid >/dev/null 2>&1 || fail "setsid is required for retained live launch status"
	verify_remote_artifact

	bash -n scripts/dogfood-external-level1-suite.sh scripts/dogfood-external-level1.sh
	[[ "$(sha256sum scripts/dogfood-external-level1-suite.sh | awk '{print $1}')" == "$SUITE_SCRIPT_SHA256" ]] || fail "guarded suite script changed"
	[[ "$(sha256sum scripts/dogfood-external-level1.sh | awk '{print $1}')" == "$COLLECTOR_SHA256" ]] || fail "collector changed"
	.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51/build-instructions.sh \
		--verify .revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51 >/dev/null
	(
		cd .revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51-verification
		sha256sum --check files.sha256 >/dev/null
		sha256sum --check files.sha256.sha256 >/dev/null
	) || fail "candidate verification bundle changed"
	(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared checksum changed"
	[[ "$(awk -F '\t' '$1 == "suite_id" {print $2}' "$RUN_ROOT/authority.tsv")" == "$SUITE_ID" ]] || fail "prepared suite identity changed"
	[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
	[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
	[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "prepared file content changed"

	candidate="$root/.revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51/artifacts/revolvr-v0.1.0-linux-amd64"
	codex="$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js"
	[[ "$(sha256sum "$candidate" | awk '{print $1}')" == "$CANDIDATE_SHA256" ]] || fail "candidate binary changed"
	[[ "$("$candidate" --version)" == "revolvr 0.1.0" ]] || fail "candidate version changed"
	go version -m "$candidate" | grep -Fq "vcs.revision=$CANDIDATE_SOURCE" || fail "candidate source metadata changed"
	go version -m "$candidate" | grep -Fq 'vcs.modified=false' || fail "candidate records modified source"
	[[ "$(sha256sum "$codex" | awk '{print $1}')" == "$CODEX_SHA256" ]] || fail "Codex binary changed"
	[[ "$("$codex" --version)" == "codex-cli 0.144.4" ]] || fail "Codex version changed"

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
		for task in "$repository"/.agent/tasks/*.md; do
			doctor_output="$(cd "$repository" && "$candidate" doctor --for attended-task --task "$(basename "$task" .md)")" || fail "$repo task doctor failed"
			grep -Fxq 'OK source-writer lock: timeout=32m0s heartbeat_interval=10m40s required=32m0s' <<<"$doctor_output" || fail "$repo source-writer authority changed"
			grep -Fxq 'Ready: true' <<<"$doctor_output" || fail "$repo task is not ready"
		done
	done

	[[ "$(find "$RUN_ROOT/repositories" -path '*/.agent/tasks/*.md' -type f | wc -l)" -eq 10 ]] || fail "prepared task count changed"
	[[ "$(find "$RUN_ROOT/repositories" -path '*/.revolvr/autonomous/tasks/*/state.json' -type f | wc -l)" -eq 10 ]] || fail "prepared task-state count changed"
	[[ "$(grep -l '"lifecycle": "pending"' "$RUN_ROOT"/repositories/*/.revolvr/autonomous/tasks/*/state.json | wc -l)" -eq 10 ]] || fail "prepared task lifecycle changed"
	[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
	[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
	[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"
	[[ ! -e "$LAUNCH_RECORD_ROOT" && ! -L "$LAUNCH_RECORD_ROOT" ]] || fail "RC.6 launch record already exists"

	if [[ "$check_only" == true ]]; then
		printf 'RC.6 live gate: preflight passed; no model call or launch record occurred\n'
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
	printf 'RC.6 live gate: guarded suite completed; diagnostics retained at %s\n' "$ACTIVE_LAUNCH_RECORD"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	main "$@"
fi
