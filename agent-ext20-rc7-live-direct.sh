#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"
readonly CONTROLLER_ROOT="/home/gernsback/source/revolvr"
readonly RUN_ROOT="$CONTROLLER_ROOT/.revolvr/ext20-rc7.rpIUM5/suite"
readonly LAUNCH_RECORD_ROOT="$CONTROLLER_ROOT/.revolvr/ext20-rc7-launch-records"
readonly SUITE_ID="ext20-14b2bf40212b"
readonly PREPARED_COMMIT="83078e8467f00439956252955d5c130d51f34214"
readonly PREPARED_TREE="31e680996695e1bc71d38e1216a471250009fb0d"
readonly PREPARED_PARENT="29ca11e24f2cc8832615fe5274d79c151d1eb5c0"
readonly CANDIDATE_REF="refs/heads/level1-v0.1.0-rc.7"
readonly CANDIDATE_SOURCE="f63cbe3989cb281652cf4eec3f92614fec98294d"
readonly CANDIDATE_TREE="43fc099d966cd6c06a74f00272c945fe3ca0a0f9"
readonly ATTESTATION_REF="refs/heads/level1-v0.1.0-rc.7-attestation"
readonly ATTESTATION_COMMIT="3cc6d527f889c7b933828fbd832d07b5291aee79"
readonly CANDIDATE_BUNDLE="$CONTROLLER_ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb"
readonly VERIFICATION_BUNDLE="$CONTROLLER_ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.7-f63cbe3989cb-verification"
readonly CANDIDATE_BINARY="$CANDIDATE_BUNDLE/artifacts/revolvr-v0.1.0-linux-amd64"
readonly CANDIDATE_SHA256="1ebbedc87b9a91d2e097df405a2ca23d68d67e79a861166aac2ed697e5866c8a"
readonly CANDIDATE_INVENTORY_SHA256="7eb048cafce9ddbf0cb7e2be659fa9016a2d7a24a0454875f418e1571ac934ba"
readonly CANDIDATE_SEAL_SHA256="2e2c05e29a265f5878f703c19db2d5adf0484c06fccfacbc13eed54612f67ed0"
readonly VERIFICATION_INVENTORY_SHA256="ca981a3659c36a5c5802995b84fd168f85edb7b999829b54963d974ca4665733"
readonly VERIFICATION_SEAL_SHA256="6f5d8de817d7c1a286a1372ec841eb7a16682773b4ecb4fea9687590e33b8e8b"
readonly BUILD_INSTRUCTIONS_SHA256="ccf6cba57b00b3bdf1d50b074e4bbe9f13e3579493c22e87682f9d5952048ecd"
readonly AUTHORITY_SHA256="c7172aa2b58539945ce4583f9effb55d9e4a491b6b9533c1e28223119f48c73e"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="2a69ade6adfbb410b5c2a150c7fec8276bfa3bd2fdf0e2b7d434cb0e1ae0f943"
readonly REPO_A_HEAD="22bc5fd5ea1469fb76afef6425964f0b0c7f70bb"
readonly REPO_B_HEAD="f92954597d8bd35372ee181c959be9a5fc637429"
readonly CONFIG_SHA256="c2fcaaeb06fb828058a38f0f0aea3bb26977adcd70610d046a58908cb5f361c0"
readonly SUITE_SCRIPT_SHA256="8957ac1c8d9ad1901ccb707bc7ca270e670de83572ec5b96933291a99b838317"
readonly COLLECTOR_SHA256="2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd"
readonly CODEX_PACKAGE_VERSION="0.144.4"
readonly CODEX_SHA256="134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477"
readonly TASKS_SHA256="33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448"
readonly SOURCE_CI_RUN="30160277511"
readonly ATTESTATION_RUN="30163857880"
readonly ATTESTATION_JOB="89693466274"
readonly COMPANION_CI_RUN="30163853353"
readonly ARTIFACT_ID="8621008768"
readonly ARTIFACT_NAME="level1-v0.1.0-rc.7-attestation"
readonly ARTIFACT_SIZE="70275600"
readonly ARTIFACT_DIGEST="sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356"
readonly REPOSITORY_API="https://api.github.com/repos/ponchione/revolvr"
readonly RC6_SUITE="$CONTROLLER_ROOT/.revolvr/ext20-rc6.LOLauh/suite"
readonly RC6_LAUNCH_RECORD="$CONTROLLER_ROOT/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365"
readonly RC6_TERMINAL_EVIDENCE="$RC6_SUITE/evidence/repo-a/01-successful-source-change-1"
readonly RC6_SUITE_SHA256="d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b"
readonly RC6_LAUNCH_SHA256="2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce"
readonly RC6_TERMINAL_SHA256="e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259"

ACTIVE_LAUNCH_RECORD=""
ACTIVE_CHILD_PID=""
ACTIVE_STATUS_FINAL=false
PREFLIGHT_CONTROLLER_HEAD=""
PREFLIGHT_ORIGIN_HEAD=""

fail() {
	printf 'RC.7 live gate: %s\n' "$*" >&2
	exit 1
}

hash_file() {
	sha256sum "$1" | awk '{print $1}'
}

content_stream_sha256() {
	local root="$1"
	[[ -d "$root" && ! -L "$root" ]] || fail "protected content root is unavailable: $root"
	(
		cd "$root"
		find . -type f -print0 | sort -z | xargs -0 -r sha256sum | sha256sum | awk '{print $1}'
	)
}

authority_value() {
	local key="$1"
	awk -F '\t' -v key="$key" '$1 == key {print $2}' "$RUN_ROOT/authority.tsv"
}

expected_remote_refs() {
	printf '%s\n' \
		$'ed65049fba6bf82852fd406ebc17afa90a953e3f\trefs/heads/level1-v0.1.0-rc.1' \
		$'a1afdd73a7bfb03e9e5ef361616604115f9db5b8\trefs/heads/level1-v0.1.0-rc.1-attestation' \
		$'eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec\trefs/heads/level1-v0.1.0-rc.2' \
		$'7038030d07c9eb1b76e0af2a3fdc84154d9b6fe2\trefs/heads/level1-v0.1.0-rc.2-attestation' \
		$'a16ea1bdc1a4ceff9d6281c7ca5e6b5c0625205c\trefs/heads/level1-v0.1.0-rc.3' \
		$'80441464d55af466bbea15f20448099e2a163684\trefs/heads/level1-v0.1.0-rc.3-attestation' \
		$'2546913e38ec273f64417dece2f91df78fd42fc2\trefs/heads/level1-v0.1.0-rc.4' \
		$'52c2db07a86677e67921bcbfbcbdf26397b47615\trefs/heads/level1-v0.1.0-rc.4-attestation' \
		$'19c1ef4b6a610016487880aa8ad69ec0204bd4f7\trefs/heads/level1-v0.1.0-rc.5' \
		$'109b38cdb309b50c38ab2ef0df33998e92dfd5e6\trefs/heads/level1-v0.1.0-rc.5-attestation' \
		$'73f1f81f1c51d927114f19818a18161d0fcb8541\trefs/heads/level1-v0.1.0-rc.6' \
		$'226276f151ae389d06c0118a931596712fbc7cc1\trefs/heads/level1-v0.1.0-rc.6-attestation' \
		$'f63cbe3989cb281652cf4eec3f92614fec98294d\trefs/heads/level1-v0.1.0-rc.7' \
		$'3cc6d527f889c7b933828fbd832d07b5291aee79\trefs/heads/level1-v0.1.0-rc.7-attestation' \
		| sort -k2
}

expected_source_jobs() {
	printf '%s\n' \
		$'89684369461\tGo 1.22 source floor and tests\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369485\tProduction autonomous strict-fake suite\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369426\tRace tests\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369423\tVet and module verification\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369490\tFake-Codex success smoke\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369458\tFake-Codex verification-failure smoke\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369418\tBuild linux/amd64\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369432\tBuild darwin/amd64\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369443\tBuild freebsd/amd64\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		$'89684369447\tBuild Windows diagnostic stub\tf63cbe3989cb281652cf4eec3f92614fec98294d\tcompleted\tsuccess' \
		| sort -n
}

expected_companion_jobs() {
	printf '%s\n' \
		$'89693455164\tGo 1.22 source floor and tests\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455158\tProduction autonomous strict-fake suite\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455167\tRace tests\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455176\tVet and module verification\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455194\tFake-Codex success smoke\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455193\tFake-Codex verification-failure smoke\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455190\tBuild linux/amd64\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455186\tBuild darwin/amd64\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455183\tBuild freebsd/amd64\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		$'89693455189\tBuild Windows diagnostic stub\t3cc6d527f889c7b933828fbd832d07b5291aee79\tcompleted\tsuccess' \
		| sort -n
}

expected_operation_plan() {
	printf '%s\n' \
		$'01\trepo-a\text20-success-a1\tsuccessful-source-change-1\tcompleted\tnormal' \
		$'02\trepo-a\text20-success-a2\tsuccessful-source-change-2\tcompleted\tnormal' \
		$'03\trepo-a\text20-success-a3\tsuccessful-source-change-3\tcompleted\tnormal' \
		$'04\trepo-a\text20-correction-a\tverification-correction\tcompleted\tnormal' \
		$'05\trepo-a\text20-needs-input-a\tneeds-input\tneeds_input\tnormal' \
		$'06\trepo-a\text20-verification-failure-a\tverification-failure\tunsafe_or_ambiguous\tnormal' \
		$'07\trepo-b\text20-success-b1\tsuccessful-source-change-4\tcompleted\tnormal' \
		$'08\trepo-b\text20-success-b2\tsuccessful-source-change-5\tcompleted\tnormal' \
		$'09\trepo-b\text20-cancel-b\tgraceful-cancellation\toperation_cancelled\tcancel' \
		$'10\trepo-b\text20-cancel-b\tgraceful-restart\tcompleted\tnormal' \
		$'11\trepo-b\text20-safety-b\tsafety-refusal\tsafety_stop\tnormal'
}

expected_tasks() {
	case "$1" in
	repo-a)
		printf '%s\n' ext20-correction-a ext20-needs-input-a ext20-success-a1 ext20-success-a2 ext20-success-a3 ext20-verification-failure-a
		;;
	repo-b)
		printf '%s\n' ext20-cancel-b ext20-safety-b ext20-success-b1 ext20-success-b2
		;;
	*) fail "unknown prepared repository: $1" ;;
	esac
}

api_get() {
	curl --fail --silent --show-error --location \
		-H 'Accept: application/vnd.github+json' \
		-H 'X-GitHub-Api-Version: 2022-11-28' \
		"$1"
}

verify_remote_authority() {
	local source_run attestation_run companion_run attestation_jobs artifacts artifact
	cmp -s \
		<(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.*' | sort -k2) \
		<(expected_remote_refs) || fail "candidate or historical remote refs changed"
	[[ -z "$(git ls-remote --tags origin 'refs/tags/*rc*')" ]] || fail "an RC tag now exists"
	[[ "$(git ls-remote --heads origin "$CANDIDATE_REF" | awk '{print $1}')" == "$CANDIDATE_SOURCE" ]] || fail "candidate ref changed"
	[[ "$(git ls-remote --heads origin "$ATTESTATION_REF" | awk '{print $1}')" == "$ATTESTATION_COMMIT" ]] || fail "attestation ref changed"

	source_run="$(api_get "$REPOSITORY_API/actions/runs/$SOURCE_CI_RUN")" || fail "source CI REST readback failed"
	jq -e '.id == 30160277511 and .event == "push" and .head_branch == "level1-v0.1.0-rc.7" and .head_sha == "f63cbe3989cb281652cf4eec3f92614fec98294d" and .path == ".github/workflows/ci.yml" and .run_attempt == 1 and .status == "completed" and .conclusion == "success"' \
		<<<"$source_run" >/dev/null || fail "source CI authority changed"
	cmp -s \
		<(api_get "$REPOSITORY_API/actions/runs/$SOURCE_CI_RUN/jobs?per_page=100" | jq -r '.jobs[] | [.id,.name,.head_sha,.status,.conclusion] | @tsv' | sort -n) \
		<(expected_source_jobs) || fail "source CI jobs changed"

	attestation_run="$(api_get "$REPOSITORY_API/actions/runs/$ATTESTATION_RUN")" || fail "attestation run REST readback failed"
	jq -e '.id == 30163857880 and .event == "push" and .head_branch == "level1-v0.1.0-rc.7-attestation" and .head_sha == "3cc6d527f889c7b933828fbd832d07b5291aee79" and .path == ".github/workflows/level1-rc7-candidate-attestation.yml" and .run_attempt == 1 and .status == "completed" and .conclusion == "success"' \
		<<<"$attestation_run" >/dev/null || fail "attestation run authority changed"
	attestation_jobs="$(api_get "$REPOSITORY_API/actions/runs/$ATTESTATION_RUN/jobs?per_page=100")" || fail "attestation job REST readback failed"
	jq -e '.total_count == 1 and (.jobs | length) == 1 and .jobs[0].id == 89693466274 and .jobs[0].name == "Rebuild and attest Level 1 RC.7 candidate" and .jobs[0].head_sha == "3cc6d527f889c7b933828fbd832d07b5291aee79" and .jobs[0].status == "completed" and .jobs[0].conclusion == "success"' \
		<<<"$attestation_jobs" >/dev/null || fail "attestation job authority changed"

	companion_run="$(api_get "$REPOSITORY_API/actions/runs/$COMPANION_CI_RUN")" || fail "companion CI REST readback failed"
	jq -e '.id == 30163853353 and .event == "push" and .head_branch == "level1-v0.1.0-rc.7-attestation" and .head_sha == "3cc6d527f889c7b933828fbd832d07b5291aee79" and .path == ".github/workflows/ci.yml" and .run_attempt == 1 and .status == "completed" and .conclusion == "success"' \
		<<<"$companion_run" >/dev/null || fail "companion CI authority changed"
	cmp -s \
		<(api_get "$REPOSITORY_API/actions/runs/$COMPANION_CI_RUN/jobs?per_page=100" | jq -r '.jobs[] | [.id,.name,.head_sha,.status,.conclusion] | @tsv' | sort -n) \
		<(expected_companion_jobs) || fail "companion CI jobs changed"

	artifacts="$(api_get "$REPOSITORY_API/actions/runs/$ATTESTATION_RUN/artifacts?per_page=100")" || fail "attestation artifact list REST readback failed"
	jq -e '.total_count == 1 and (.artifacts | length) == 1 and .artifacts[0].id == 8621008768 and .artifacts[0].name == "level1-v0.1.0-rc.7-attestation" and .artifacts[0].size_in_bytes == 70275600 and .artifacts[0].digest == "sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356" and .artifacts[0].expired == false and .artifacts[0].workflow_run.id == 30163857880' \
		<<<"$artifacts" >/dev/null || fail "attestation artifact list authority changed"
	artifact="$(api_get "$REPOSITORY_API/actions/artifacts/$ARTIFACT_ID")" || fail "attestation artifact REST readback failed"
	jq -e '.id == 8621008768 and .name == "level1-v0.1.0-rc.7-attestation" and .size_in_bytes == 70275600 and .digest == "sha256:ae87472ef86b5d25cca5df333f057f10d77cf74cd7f332f30d6770745bbf6356" and .expired == false and .workflow_run.id == 30163857880' \
		<<<"$artifact" >/dev/null || fail "attestation artifact authority changed"
}

verify_bundle_inventory() {
	local bundle="$1" inventory_sha256="$2" seal_sha256="$3"
	[[ -d "$bundle" && ! -L "$bundle" ]] || fail "sealed bundle is unavailable: $bundle"
	[[ "$(hash_file "$bundle/files.sha256")" == "$inventory_sha256" ]] || fail "bundle inventory changed: $bundle"
	[[ "$(hash_file "$bundle/files.sha256.sha256")" == "$seal_sha256" ]] || fail "bundle seal changed: $bundle"
	[[ -z "$(find "$bundle" -type l -print -quit)" ]] || fail "bundle contains a symlink: $bundle"
	[[ -z "$(find "$bundle" -type f ! -links 1 -print -quit)" ]] || fail "bundle contains aliased evidence: $bundle"
	(cd "$bundle" && sha256sum -c files.sha256.sha256 >/dev/null && sha256sum -c files.sha256 >/dev/null) || fail "bundle inventory verification failed: $bundle"
	cmp -s \
		<(find "$bundle" -type f ! -name files.sha256 ! -name files.sha256.sha256 -printf '%P\n' | sort) \
		<(sed -E 's/^[0-9a-f]{64}  \.\///' "$bundle/files.sha256" | sort) \
		|| fail "bundle regular-file inventory changed: $bundle"
}

verify_bundles_and_scripts() {
	verify_bundle_inventory "$CANDIDATE_BUNDLE" "$CANDIDATE_INVENTORY_SHA256" "$CANDIDATE_SEAL_SHA256"
	verify_bundle_inventory "$VERIFICATION_BUNDLE" "$VERIFICATION_INVENTORY_SHA256" "$VERIFICATION_SEAL_SHA256"
	[[ "$(hash_file "$CANDIDATE_BUNDLE/build-instructions.sh")" == "$BUILD_INSTRUCTIONS_SHA256" ]] || fail "candidate build instructions changed"
	[[ "$(hash_file "$VERIFICATION_BUNDLE/candidate-build-instructions.sh")" == "$BUILD_INSTRUCTIONS_SHA256" ]] || fail "verification-bundle candidate verifier changed"
	"$CANDIDATE_BUNDLE/build-instructions.sh" --verify "$CANDIDATE_BUNDLE" >/dev/null || fail "candidate self-verification failed"
	bash "$VERIFICATION_BUNDLE/candidate-build-instructions.sh" --verify "$CANDIDATE_BUNDLE" >/dev/null || fail "sealed verification-bundle candidate verification failed"
	[[ "$(git rev-parse "$CANDIDATE_SOURCE^{tree}")" == "$CANDIDATE_TREE" ]] || fail "candidate source tree changed"
	git merge-base --is-ancestor "$CANDIDATE_SOURCE" HEAD || fail "candidate source is not in controller ancestry"
	git diff --quiet "$CANDIDATE_SOURCE"..HEAD -- cmd internal go.mod go.sum || fail "product source changed after the candidate"
	bash -n scripts/dogfood-external-level1-suite.sh scripts/dogfood-external-level1.sh
	[[ "$(hash_file scripts/dogfood-external-level1-suite.sh)" == "$SUITE_SCRIPT_SHA256" ]] || fail "guarded suite script changed"
	[[ "$(hash_file scripts/dogfood-external-level1.sh)" == "$COLLECTOR_SHA256" ]] || fail "collector changed"
	scripts/dogfood-external-level1-suite.sh --static >/dev/null || fail "guarded suite static verification failed"
}

verify_controller_authority() {
	local root="$1" remote_main prepared_record
	[[ "$(git branch --show-current)" == main ]] || fail "controller is not on main"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git status --porcelain=v1 --untracked-files=all)" ]] || fail "controller repository is not clean"
	PREFLIGHT_CONTROLLER_HEAD="$(git rev-parse HEAD)"
	PREFLIGHT_ORIGIN_HEAD="$(git rev-parse refs/remotes/origin/main 2>/dev/null)" || fail "fetched origin/main is unavailable"
	[[ "$PREFLIGHT_CONTROLLER_HEAD" == "$PREFLIGHT_ORIGIN_HEAD" ]] || fail "local main does not match origin/main"
	remote_main="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
	[[ -n "$remote_main" && "$remote_main" == "$PREFLIGHT_CONTROLLER_HEAD" ]] || fail "published main does not match local main"
	[[ "$root" == "$CONTROLLER_ROOT" ]] || fail "controller root changed"
	[[ "$(git remote get-url origin)" == "git@github.com:ponchione/revolvr.git" ]] || fail "origin URL changed"
	prepared_record="$(git show -s --format='%H %T %P' "$PREPARED_COMMIT")"
	[[ "$prepared_record" == "$PREPARED_COMMIT $PREPARED_TREE $PREPARED_PARENT" ]] || fail "prepared-suite publication authority changed"
	git merge-base --is-ancestor "$PREPARED_COMMIT" HEAD || fail "prepared-suite publication is not in local ancestry"
	git merge-base --is-ancestor "$PREPARED_COMMIT" refs/remotes/origin/main || fail "prepared-suite publication is not in fetched-main ancestry"
	git merge-base --is-ancestor "$PREPARED_PARENT" HEAD || fail "prepared-suite parent is not in local ancestry"
	git merge-base --is-ancestor "$PREPARED_PARENT" refs/remotes/origin/main || fail "prepared-suite parent is not in fetched-main ancestry"
	[[ "$(hash_file .agent/TASKS.md)" == "$TASKS_SHA256" ]] || fail "task backlog changed"
	grep -Fq -- '- [ ] EXT-20 — Execute the quantitative Level-1 real-Codex dogfood gate.' .agent/TASKS.md || fail "EXT-20 is no longer unchecked"
	[[ -d "$root/.revolvr" && ! -L "$root/.revolvr" ]] || fail "controller runtime root is unavailable"
	[[ "$(cd "$root/.revolvr" && pwd -P)" == "$root/.revolvr" ]] || fail "controller runtime root changed identity"
	[[ ! -e "$LAUNCH_RECORD_ROOT" && ! -L "$LAUNCH_RECORD_ROOT" ]] || fail "RC.7 launch-record root already exists"
	git check-ignore -q -- .revolvr/ext20-rc7-launch-records/probe || fail "RC.7 launch-record path is not ignored runtime state"
}

verify_candidate_and_codex() {
	local codex
	[[ "$(hash_file "$CANDIDATE_BINARY")" == "$CANDIDATE_SHA256" ]] || fail "candidate Linux binary changed"
	[[ "$($CANDIDATE_BINARY --version)" == "revolvr 0.1.0" ]] || fail "candidate version changed"
	go version -m "$CANDIDATE_BINARY" | grep -Fq "vcs.revision=$CANDIDATE_SOURCE" || fail "candidate source metadata changed"
	go version -m "$CANDIDATE_BINARY" | grep -Fq 'vcs.modified=false' || fail "candidate records modified source"
	codex="$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js"
	[[ -f "$codex" && ! -L "$codex" && -x "$codex" ]] || fail "isolated Codex executable is unavailable"
	[[ "$(hash_file "$codex")" == "$CODEX_SHA256" ]] || fail "isolated Codex executable changed"
	[[ "$(node "$codex" --version)" == "codex-cli $CODEX_PACKAGE_VERSION" ]] || fail "isolated Codex version changed"
	[[ "$(sed -n 's/^[[:space:]]*\"version\":[[:space:]]*\"\([^\"]*\)\".*/\1/p' "$RUN_ROOT/codex-package/node_modules/@openai/codex/package.json" | head -n 1)" == "$CODEX_PACKAGE_VERSION" ]] || fail "isolated Codex package version changed"
}

verify_sentinel() {
	local repo="$1" sentinel expected_value_hash
	sentinel="$RUN_ROOT/sentinels/$repo"
	expected_value_hash="a789a0e300ddfa9d62bf2fbb32419cef5aeb1819899a7ece8194a65578e4b9de"
	[[ "$repo" == repo-b ]] && expected_value_hash="4857b4c627d0ea03fb0fab1137aba6c0b5d694b826e7b8ed5b61d9cd4a055268"
	[[ -d "$sentinel" && ! -L "$sentinel" ]] || fail "$repo sentinel root changed"
	[[ -f "$sentinel/value.txt" && ! -L "$sentinel/value.txt" ]] || fail "$repo sentinel value changed"
	[[ "$(hash_file "$sentinel/value.txt")" == "$expected_value_hash" ]] || fail "$repo sentinel value bytes changed"
	[[ "$(stat -c '%a' "$sentinel/value.txt")" == 644 ]] || fail "$repo sentinel value mode changed"
	[[ "$(stat -c '%d:%i:%h' "$sentinel/value.txt")" == "$(stat -c '%d:%i:%h' "$sentinel/value-hardlink.txt")" ]] || fail "$repo sentinel hard link changed"
	[[ "$(stat -c '%h' "$sentinel/value.txt")" == 2 ]] || fail "$repo sentinel link count changed"
	[[ -L "$sentinel/value-link.txt" && "$(readlink "$sentinel/value-link.txt")" == value.txt ]] || fail "$repo sentinel symbolic link changed"
	[[ -f "$sentinel/executable.sh" && ! -L "$sentinel/executable.sh" && -x "$sentinel/executable.sh" ]] || fail "$repo sentinel executable changed"
	[[ "$(stat -c '%a' "$sentinel/executable.sh")" == 755 ]] || fail "$repo sentinel executable mode changed"
	[[ "$(hash_file "$sentinel/executable.sh")" == "306c6ca7407560340797866e077e053627ad409277d1b9da58106fce4cf717cb" ]] || fail "$repo sentinel executable bytes changed"
}

verify_repository() {
	local repo_name="$1" expected_head="$2" repository config_output task state doctor_output
	repository="$RUN_ROOT/repositories/$repo_name"
	[[ -d "$repository" && ! -L "$repository" ]] || fail "$repo_name repository changed identity"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" branch --show-current)" == main ]] || fail "$repo_name branch changed"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" rev-parse HEAD)" == "$expected_head" ]] || fail "$repo_name HEAD changed"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" status --porcelain=v1 --untracked-files=all)" ]] || fail "$repo_name is dirty"
	[[ "$(git -C "$repository" show HEAD:.revolvr-dogfood-disposable-v1)" == "revolvr-external-level1-disposable-v1" ]] || fail "$repo_name disposable marker changed"
	[[ "$(hash_file "$repository/.revolvr/config.yaml")" == "$CONFIG_SHA256" ]] || fail "$repo_name config changed"
	config_output="$(cd "$repository" && "$CANDIDATE_BINARY" config check)" || fail "$repo_name config check failed"
	grep -Fxq 'Codex model: gpt-5.6-sol' <<<"$config_output" || fail "$repo_name model authority changed"
	grep -Fxq 'Codex reasoning effort: xhigh' <<<"$config_output" || fail "$repo_name reasoning authority changed"
	grep -Fxq 'Codex session mode: ephemeral (ephemeral=true)' <<<"$config_output" || fail "$repo_name session authority changed"
	grep -Fxq "Source-writer lock: timeout=32m0s heartbeat_interval=10m40s" <<<"$config_output" || fail "$repo_name source-writer config authority changed"
	grep -Fq "sha256=$CODEX_SHA256" <<<"$config_output" || fail "$repo_name Codex identity projection changed"
	cmp -s \
		<(find "$repository/.agent/tasks" -maxdepth 1 -type f -name '*.md' -printf '%f\n' | sed 's/\.md$//' | sort) \
		<(expected_tasks "$repo_name" | sort) || fail "$repo_name canonical task set changed"
	cmp -s \
		<(find "$repository/.revolvr/autonomous/tasks" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort) \
		<(expected_tasks "$repo_name" | sort) || fail "$repo_name autonomous task set changed"
	while IFS= read -r task; do
		state="$repository/.revolvr/autonomous/tasks/$task/state.json"
		jq -e --arg task "$task" '.schema_version == "autonomous-execution-state-v1" and .task_id == $task and .lifecycle == "pending" and .attempts.total_attempts == 0 and .attempts.consecutive_failures == 0 and .attempts.retry_budget.consumed == 0 and .attempts.elapsed_time_budget.consumed_nanoseconds == 0 and .attempts.token_budget.consumed == 0' "$state" >/dev/null || fail "$repo_name task state changed: $task"
		doctor_output="$(cd "$repository" && "$CANDIDATE_BINARY" doctor --for attended-task --task "$task")" || fail "$repo_name doctor failed: $task"
		grep -Fxq 'OK source-writer lock: timeout=32m0s heartbeat_interval=10m40s required=32m0s' <<<"$doctor_output" || fail "$repo_name source-writer doctor authority changed: $task"
		grep -Fq "task=$task readiness=ready" <<<"$doctor_output" || fail "$repo_name task is not doctor-ready: $task"
		grep -Fxq 'Ready: true' <<<"$doctor_output" || fail "$repo_name doctor refused: $task"
	done < <(expected_tasks "$repo_name")
	[[ -z "$(find "$repository/.revolvr/runs" -mindepth 1 -print -quit)" ]] || fail "$repo_name contains a model run"
	[[ -z "$(find "$repository/.revolvr/receipts" -mindepth 1 -print -quit)" ]] || fail "$repo_name contains a receipt"
	verify_sentinel "$repo_name"
}

verify_prepared_suite() {
	[[ -d "$RUN_ROOT" && ! -L "$RUN_ROOT" ]] || fail "prepared RC.7 suite is unavailable"
	[[ "$(cd "$RUN_ROOT" && pwd -P)" == "$RUN_ROOT" ]] || fail "prepared RC.7 suite changed identity"
	[[ "$(find "$RUN_ROOT" -type f | wc -l | tr -d ' ')" == 268 ]] || fail "prepared RC.7 regular-file count changed"
	(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared authority checksum changed"
	[[ "$(authority_value suite_id)" == "$SUITE_ID" ]] || fail "prepared suite identity changed"
	[[ "$(authority_value candidate_binary)" == "$CANDIDATE_BINARY" ]] || fail "prepared candidate path changed"
	[[ "$(authority_value candidate_sha256)" == "$CANDIDATE_SHA256" ]] || fail "prepared candidate hash authority changed"
	[[ "$(authority_value candidate_source_commit)" == "$CANDIDATE_SOURCE" ]] || fail "prepared candidate source authority changed"
	[[ "$(authority_value candidate_version)" == "revolvr 0.1.0" ]] || fail "prepared candidate version authority changed"
	[[ "$(authority_value codex_binary)" == "$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js" ]] || fail "prepared Codex path changed"
	[[ "$(authority_value codex_sha256)" == "$CODEX_SHA256" ]] || fail "prepared Codex hash authority changed"
	[[ "$(authority_value codex_version)" == "codex-cli $CODEX_PACKAGE_VERSION" ]] || fail "prepared Codex version authority changed"
	[[ "$(authority_value repo_a_config_sha256)" == "$CONFIG_SHA256" ]] || fail "repo-a config authority changed"
	[[ "$(authority_value repo_b_config_sha256)" == "$CONFIG_SHA256" ]] || fail "repo-b config authority changed"
	[[ "$(hash_file "$RUN_ROOT/authority.tsv")" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
	[[ "$(hash_file "$RUN_ROOT/operation-plan.tsv")" == "$PLAN_SHA256" ]] || fail "prepared plan hash changed"
	cmp -s "$RUN_ROOT/operation-plan.tsv" <(expected_operation_plan) || fail "prepared operation plan changed"
	[[ "$(content_stream_sha256 "$RUN_ROOT")" == "$CONTENT_SHA256" ]] || fail "prepared RC.7 content changed"
	verify_candidate_and_codex
	verify_repository repo-a "$REPO_A_HEAD"
	verify_repository repo-b "$REPO_B_HEAD"
	[[ "$(awk -F '\t' '{print $3}' "$RUN_ROOT/operation-plan.tsv" | sort -u | wc -l | tr -d ' ')" == 10 ]] || fail "prepared unique task count changed"
	[[ -z "$(find "$RUN_ROOT/evidence" -type f -print -quit)" ]] || fail "RC.7 collector evidence already exists"
	[[ -z "$(find "$RUN_ROOT" -type f -name operation.tsv -print -quit)" ]] || fail "RC.7 operation evidence already exists"
	[[ -z "$(find "$RUN_ROOT/aggregate" -mindepth 1 -print -quit)" ]] || fail "RC.7 aggregate is not empty"
	cmp -s \
		<(find "$RUN_ROOT/logs" -mindepth 1 -maxdepth 1 -type f -printf '%f\n' | sort) \
		<(printf '%s\n' npm-install.err npm-install.out) || fail "RC.7 suite contains operation logs"
	[[ "$(hash_file "$RUN_ROOT/logs/npm-install.out")" == "65c731cd02e19c79f6f5a3e84a4dd64a49acd6c47d2ee551d7cc9da191e8c96c" ]] || fail "RC.7 npm preparation log changed"
	[[ "$(hash_file "$RUN_ROOT/logs/npm-install.err")" == "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" ]] || fail "RC.7 npm preparation error log changed"
	[[ ! -e "$LAUNCH_RECORD_ROOT" && ! -L "$LAUNCH_RECORD_ROOT" ]] || fail "RC.7 launch-record root already exists"
	[[ "$(content_stream_sha256 "$RUN_ROOT")" == "$CONTENT_SHA256" ]] || fail "RC.7 check commands changed prepared content"
}

verify_rc6_preservation() {
	[[ "$(content_stream_sha256 "$RC6_SUITE")" == "$RC6_SUITE_SHA256" ]] || fail "protected RC.6 suite changed"
	[[ "$(content_stream_sha256 "$RC6_LAUNCH_RECORD")" == "$RC6_LAUNCH_SHA256" ]] || fail "protected RC.6 launch record changed"
	[[ "$(content_stream_sha256 "$RC6_TERMINAL_EVIDENCE")" == "$RC6_TERMINAL_SHA256" ]] || fail "protected RC.6 terminal evidence changed"
}

complete_preflight() {
	local root="$1"
	for command_name in awk cmp curl find git go jq node readlink realpath setsid sha256sum sort stat xargs; do
		command -v "$command_name" >/dev/null 2>&1 || fail "required command is unavailable: $command_name"
	done
	verify_controller_authority "$root"
	[[ -x scripts/dogfood-external-level1-suite.sh ]] || fail "guarded suite is unavailable"
	[[ -x scripts/dogfood-external-level1.sh ]] || fail "collector is unavailable"
	verify_rc6_preservation
	verify_remote_authority
	verify_bundles_and_scripts
	verify_prepared_suite
	verify_rc6_preservation
	[[ "$(hash_file .agent/TASKS.md)" == "$TASKS_SHA256" ]] || fail "task backlog changed during preflight"
	[[ "$(content_stream_sha256 "$RUN_ROOT")" == "$CONTENT_SHA256" ]] || fail "prepared RC.7 content changed during preflight"
	[[ ! -e "$LAUNCH_RECORD_ROOT" && ! -L "$LAUNCH_RECORD_ROOT" ]] || fail "RC.7 launch-record root appeared during preflight"
}

reserve_launch_record() {
	local parent="$1" launch_id="$2" runtime_root record
	[[ "$parent" == /* && "$launch_id" =~ ^[A-Za-z0-9._-]+$ ]] || fail "unsafe launch-record identity"
	runtime_root="$(dirname -- "$parent")"
	[[ -d "$runtime_root" && ! -L "$runtime_root" ]] || fail "launch-record runtime root is unavailable"
	[[ "$(cd "$runtime_root" && pwd -P)" == "$runtime_root" ]] || fail "launch-record runtime root changed identity"
	[[ ! -e "$parent" && ! -L "$parent" ]] || fail "launch-record collision at $parent"
	umask 0077
	mkdir -- "$parent" || fail "launch-record collision at $parent"
	record="$parent/$launch_id"
	mkdir -- "$record" || fail "launch-record collision at $record"
	[[ "$(find "$parent" -mindepth 1 -maxdepth 1 -type d -printf '%f\n')" == "$launch_id" ]] || fail "launch-record root contains foreign entries"
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

write_pre_start_authority() {
	local record="$1" launcher_sha256="$2"
	{
		printf 'schema_version\trevolvr-ext20-live-launch-authority-v1\n'
		printf 'launch_record\t%s\n' "$record"
		printf 'created_at_utc\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		printf 'controller_root\t%s\n' "$CONTROLLER_ROOT"
		printf 'controller_head\t%s\n' "$PREFLIGHT_CONTROLLER_HEAD"
		printf 'origin_main\t%s\n' "$PREFLIGHT_ORIGIN_HEAD"
		printf 'launcher_sha256\t%s\n' "$launcher_sha256"
		printf 'suite_root\t%s\n' "$RUN_ROOT"
		printf 'suite_id\t%s\n' "$SUITE_ID"
		printf 'prepared_commit\t%s\n' "$PREPARED_COMMIT"
		printf 'candidate_ref\t%s\n' "$CANDIDATE_REF"
		printf 'candidate_source\t%s\n' "$CANDIDATE_SOURCE"
		printf 'candidate_sha256\t%s\n' "$CANDIDATE_SHA256"
		printf 'attestation_ref\t%s\n' "$ATTESTATION_REF"
		printf 'attestation_commit\t%s\n' "$ATTESTATION_COMMIT"
		printf 'source_ci_run\t%s\n' "$SOURCE_CI_RUN"
		printf 'attestation_run\t%s\n' "$ATTESTATION_RUN"
		printf 'attestation_job\t%s\n' "$ATTESTATION_JOB"
		printf 'companion_ci_run\t%s\n' "$COMPANION_CI_RUN"
		printf 'artifact_id\t%s\n' "$ARTIFACT_ID"
		printf 'artifact_name\t%s\n' "$ARTIFACT_NAME"
		printf 'artifact_digest\t%s\n' "$ARTIFACT_DIGEST"
		printf 'authority_sha256\t%s\n' "$AUTHORITY_SHA256"
		printf 'plan_sha256\t%s\n' "$PLAN_SHA256"
		printf 'content_sha256\t%s\n' "$CONTENT_SHA256"
		printf 'suite_script_sha256\t%s\n' "$SUITE_SCRIPT_SHA256"
		printf 'collector_sha256\t%s\n' "$COLLECTOR_SHA256"
		printf 'codex_sha256\t%s\n' "$CODEX_SHA256"
		printf 'repo_a_head\t%s\n' "$REPO_A_HEAD"
		printf 'repo_b_head\t%s\n' "$REPO_B_HEAD"
	} >"$record/pre-start-authority.tsv"
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

main() {
	local check_only=false root launcher_sha256 launch_id status
	if [[ "$#" -eq 1 && "$1" == "--check" ]]; then
		check_only=true
	elif [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
		printf 'usage: %s [--check | %s]\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
		exit 64
	fi

	root="$(git rev-parse --show-toplevel 2>/dev/null || pwd -P)"
	cd "$root"
	complete_preflight "$root"
	if [[ "$check_only" == true ]]; then
		printf 'RC.7 live gate: complete preflight passed; no model call or launch record occurred\n'
		exit 0
	fi

	launcher_sha256="$(hash_file "$0")"
	launch_id="$SUITE_ID-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	ACTIVE_LAUNCH_RECORD="$(reserve_launch_record "$LAUNCH_RECORD_ROOT" "$launch_id")"
	umask 0077
	trap 'retain_launcher_exit "$?"' EXIT
	trap 'retain_interruption HUP' HUP
	trap 'retain_interruption INT' INT
	trap 'retain_interruption QUIT' QUIT
	trap 'retain_interruption TERM' TERM
	write_pre_start_authority "$ACTIVE_LAUNCH_RECORD" "$launcher_sha256"
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
	[[ "$status" -eq 0 ]] || fail "guarded suite failed with status $status; terminal diagnostics retained at $ACTIVE_LAUNCH_RECORD"
	printf 'RC.7 live gate: guarded suite completed exactly once; terminal diagnostics retained at %s\n' "$ACTIVE_LAUNCH_RECORD"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
	main "$@"
fi
