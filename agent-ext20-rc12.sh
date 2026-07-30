#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT="/home/gernsback/source/revolvr"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
SOURCE_REMOTE="git@github.com:ponchione/revolvr.git"
EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
DRAFT="$EVIDENCE_ROOT/prospective-builder.sh"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.12.sh"
SELF="$ROOT/agent-ext20-rc12.sh"
SELF_REL="agent-ext20-rc12.sh"
FINAL_CANDIDATE="$RELEASE_ROOT/level1-v0.1.0-rc.12-a24804bcf2a3"
FINAL_VERIFICATION="${FINAL_CANDIDATE}-verification"
POST_CANDIDATE_REVIEW="$ROOT/agent-ext20-rc12-local-review.sh"
WORKFLOW="$ROOT/.github/workflows/level1-rc12-candidate-attestation.yml"

BUILDER_SHA256="dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e"
MANIFEST_SHA256="f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2"
STREAM_SHA256="22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8"

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

require_regular_file() {
	local path="$1"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required regular file is absent or unsafe: $path"
}

require_absent() {
	local path="$1"
	[[ ! -e "$path" && ! -L "$path" ]] || fail "RC.12 collision: $path"
}

require_sha256() {
	local expected="$1"
	local path="$2"
	require_regular_file "$path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$expected" ]] ||
		fail "SHA-256 changed: $path"
}

require_file_identity() {
	local expected_mode="$1"
	local expected_size="$2"
	local expected_lines="$3"
	local expected_hash="$4"
	local path="$5"
	require_sha256 "$expected_hash" "$path"
	[[ "$(stat -c '%a' "$path")" == "$expected_mode" ]] || fail "Mode changed: $path"
	[[ "$(stat -c '%s' "$path")" == "$expected_size" ]] || fail "Size changed: $path"
	[[ "$(wc -l <"$path")" == "$expected_lines" ]] || fail "Line count changed: $path"
}

require_no_glob_matches() {
	local base="$1"
	local pattern="$2"
	if find "$base" -maxdepth 1 -mindepth 1 -name "$pattern" -print -quit | grep -q .; then
		fail "RC.12 glob collision: $base/$pattern"
	fi
}

content_stream_sha256() {
	local root="$1"
	(
		cd "$root"
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	) | sha256sum | awk '{print $1}'
}

verify_controller_anchor() {
	local commit="$1"
	local tree="$2"
	git merge-base --is-ancestor "$commit" HEAD || fail "Controller anchor is not in main history: $commit"
	[[ "$(git rev-parse "$commit^{tree}")" == "$tree" ]] || fail "Controller anchor tree changed: $commit"
}

require_tracked_controller_hash() {
	local expected="$1"
	local relative="$2"
	local stage_record
	require_sha256 "$expected" "$ROOT/$relative"
	stage_record="$(git ls-files --stage -- "$relative")"
	[[ "$stage_record" == 100755\ *$'\t'"$relative" ]] || fail "Historical controller is not tracked executable: $relative"
	[[ "$(git cat-file blob "HEAD:$relative" | sha256sum | awk '{print $1}')" == "$expected" ]] ||
		fail "Committed historical controller hash changed: $relative"
}

verify_controller() {
	local head_commit local_main fetched_main public_main
	[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
	[[ "$(stat -c '%u' "$ROOT")" == "$(id -u)" ]] || fail 'Controller root owner changed'
	cd "$ROOT"
	git fetch --no-tags origin main
	[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] ||
		fail 'RC.12 construction requires a clean published controller'
	[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Controller is not on local main'
	[[ "$(git remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'

	head_commit="$(git rev-parse HEAD)"
	local_main="$(git rev-parse refs/heads/main)"
	fetched_main="$(git rev-parse refs/remotes/origin/main)"
	public_main="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
	[[ "$head_commit" == "$local_main" && "$head_commit" == "$fetched_main" && "$head_commit" == "$public_main" ]] ||
		fail 'RC.12 construction requires exact local, fetched, and public main'

	git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
	[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
	git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum ||
		fail 'Product source changed after the candidate source commit'

	verify_controller_anchor 70aefe61ccdf6c6c6359558c483f6f1d9efac393 0703838e922e677e4b9beb8efbf66f752f0cef60
	verify_controller_anchor ce3045a071bcef873f5c5785fc773be9b13454e1 b290a732421b7e955b43fcb16d48e034dfcb2ba7
	verify_controller_anchor bae8ff6b1e5d7e14a9002cd7fbba1ece101dc005 71f13bb3ff9768a6e00500385d14641dd39f0081
	verify_controller_anchor 82ed582b2cae57b15010ebd102e35eb83cb3010a 3fcd0626a44287f94c8fef52bb2e05b8dee622e9
	verify_controller_anchor 2f21a4399a0a1bc00ceac345e0ebbeac9616d75a c20b692ad1a136b0cb7a1b70f4f941c1e14c8bfc
	verify_controller_anchor ca702ef2931a006843c10b3b899db2b5ca0689dd 5eb8f6888529640693a5ef14bd1e181e0c2bb0d4

	require_tracked_controller_hash 9aa31f45fc925e214c180fad8abac262d812f93b795f3a22105ac4d3853820e3 agent-ext20-rc12-builder-validation.sh
	require_tracked_controller_hash 0cdbde37d6c33404c988b68c2da28fd325c50c277333ba35983bad419a235fbb agent-ext20-rc12-builder-validation-review.sh
	require_tracked_controller_hash 9218d873e24214aa7fff9574e1e35ede1e967629947e101e961ad87eb285b4d7 agent-ext20-rc12-builder-revalidation.sh
	require_tracked_controller_hash 591a79098ced60f9aa0abbd11f66cf722b2adba22b1fd277237e0890aa536ee5 agent-ext20-rc12-builder-revalidation-v3.sh
	require_tracked_controller_hash 3bafb2b55cde7a872e5b159f3fc9e721d39942b208f83718875721d45dca888d agent-ext20-rc12-builder-revalidation-v3-review.sh
	require_tracked_controller_hash 5d9c82acbe9527e93421355a06843d60a2dd55c877dc2fb856c367fd02bc647c agent-ext20-rc12-builder-revalidation-v4.sh
	require_tracked_controller_hash 8def05def6c116b2b4645090a0661bd70146d52076710214b8be1084c3f771ea agent-ext20-rc12-volatile-root-recovery-review.sh
	require_tracked_controller_hash b98e2b84c93d65beb805b96cb2b6b1bc28e69de145b1d7b51fe8fb072ae33a33 agent-ext20-rc12-builder-revalidation-v4-review.sh
	require_tracked_controller_hash 180254489c4fe55b42681fe88726518b6b6acc6a83ae1d3593d8d462dccb16b7 agent-ext20-rc12-builder-publication.sh
}

verify_validation_evidence() {
	local relative mode size expected extra manifest_count total_size path
	declare -A seen=()
	[[ -d "$EVIDENCE_ROOT" && ! -L "$EVIDENCE_ROOT" ]] || fail 'Persistent validation root is absent or unsafe'
	[[ "$(realpath -e "$EVIDENCE_ROOT")" == "$EVIDENCE_ROOT" ]] || fail 'Persistent validation root resolves unexpectedly'
	[[ "$(stat -c '%a:%u' "$EVIDENCE_ROOT")" == "500:$(id -u)" ]] || fail 'Persistent validation root mode or owner changed'
	[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 \( -type l -o ! -type f \) -print -quit)" ]] ||
		fail 'Persistent validation root contains a non-regular entry'
	[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 10 ]] ||
		fail 'Persistent validation root file count changed'
	total_size="$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')"
	[[ "$total_size" == 53626 ]] || fail 'Persistent validation root byte count changed'
	[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f ! -perm 0444 -print -quit)" ]] ||
		fail 'Persistent validation evidence mode changed'

	require_file_identity 444 902 9 "$MANIFEST_SHA256" "$MANIFEST"
	manifest_count=0
	while IFS=$'\t' read -r relative mode size expected extra; do
		[[ -n "$relative" && -z "${extra:-}" && "$relative" != */* && "$relative" != evidence-manifest.tsv ]] ||
			fail 'Unsafe validation-manifest entry'
		[[ -z "${seen[$relative]:-}" ]] || fail "Duplicate validation-manifest entry: $relative"
		seen[$relative]=1
		path="$EVIDENCE_ROOT/$relative"
		require_sha256 "$expected" "$path"
		[[ "$(stat -c '%a:%s:%u:%h' "$path")" == "$mode:$size:$(id -u):1" ]] ||
			fail "Validation evidence identity changed: $relative"
		manifest_count=$((manifest_count + 1))
	done <"$MANIFEST"
	[[ "$manifest_count" == 9 ]] || fail 'Validation-manifest entry count changed'
	while IFS= read -r relative; do
		[[ -n "${seen[$relative]:-}" ]] || fail "Validation evidence is not listed: $relative"
	done < <(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f ! -name evidence-manifest.tsv -printf '%f\n' | LC_ALL=C sort)

	require_file_identity 444 38528 756 "$BUILDER_SHA256" "$DRAFT"
	[[ "$(content_stream_sha256 "$EVIDENCE_ROOT")" == "$STREAM_SHA256" ]] ||
		fail 'Persistent validation content stream changed'
	bash -n "$DRAFT"
}

verify_builder_and_self() {
	local builder_rel release_mode self_blob self_stage builder_count
	[[ -d "$RELEASE_ROOT" && ! -L "$RELEASE_ROOT" ]] || fail 'Release-candidate parent is absent or unsafe'
	[[ "$(realpath -e "$RELEASE_ROOT")" == "$RELEASE_ROOT" ]] || fail 'Release-candidate parent resolves unexpectedly'
	[[ "$(stat -c '%u' "$RELEASE_ROOT")" == "$(id -u)" ]] || fail 'Release-candidate parent owner changed'
	release_mode="$(stat -c '%a' "$RELEASE_ROOT")"
	[[ "$release_mode" == 755 ]] || fail 'Release-candidate parent mode is not preserved at 0755'
	(( (8#$release_mode & 8#022) == 0 )) || fail 'Release-candidate parent is group/other writable'

	require_file_identity 555 38528 756 "$BUILDER_SHA256" "$BUILDER"
	[[ "$(stat -c '%u:%h' "$BUILDER")" == "$(id -u):1" ]] || fail 'Builder owner or link count changed'
	cmp "$DRAFT" "$BUILDER" || fail 'Builder bytes differ from the reviewed draft'
	[[ "$(stat -c '%d:%i' "$DRAFT")" != "$(stat -c '%d:%i' "$BUILDER")" ]] || fail 'Builder reuses the sealed draft inode'
	bash -n "$BUILDER"
	builder_rel="${BUILDER#"$ROOT/"}"
	[[ -z "$(git ls-files -- "$builder_rel")" ]] || fail 'Builder unexpectedly became tracked'
	git check-ignore -q -- "$builder_rel" || fail 'Builder is not ignored runtime state'
	builder_count="$(find "$RELEASE_ROOT" -maxdepth 1 -mindepth 1 -name 'build-level1-v0.1.0-rc.12*' -printf '.\n' | wc -l)"
	[[ "$builder_count" == 1 ]] || fail 'Builder is not the sole RC.12 builder identity'

	require_regular_file "$SELF"
	[[ "$(realpath -e "$SELF")" == "$SELF" ]] || fail 'Construction-launcher path resolves unexpectedly'
	[[ "$(stat -c '%a:%u:%h' "$SELF")" == "755:$(id -u):1" ]] || fail 'Construction-launcher mode, owner, or link count changed'
	self_stage="$(git ls-files --stage -- "$SELF_REL")"
	[[ "$self_stage" == 100755\ *$'\t'"$SELF_REL" ]] || fail 'Construction launcher is not tracked executable'
	self_blob="$(git rev-parse "HEAD:$SELF_REL")"
	[[ "$(git hash-object "$SELF")" == "$self_blob" ]] || fail 'Construction-launcher bytes differ from published main'
}

verify_local_collisions() {
	require_absent "$FINAL_CANDIDATE"
	require_absent "$FINAL_VERIFICATION"
	require_absent "$POST_CANDIDATE_REVIEW"
	require_absent "$WORKFLOW"
	if git cat-file -e "refs/remotes/origin/main:.github/workflows/level1-rc12-candidate-attestation.yml" 2>/dev/null; then
		fail 'Published-main RC.12 workflow collision'
	fi

	require_absent "$ROOT/.revolvr/ext20-rc12"
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc12.*'
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc12-*'
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.12-preflight.*'
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.12-stage.*'
	[[ -d "$RELEASE_ROOT/diagnostics" && ! -L "$RELEASE_ROOT/diagnostics" ]] || fail 'Diagnostics parent is absent or unsafe'
	require_no_glob_matches "$RELEASE_ROOT/diagnostics" 'level1-v0.1.0-rc.12-*'
	require_no_glob_matches /tmp 'revolvr-ext20-rc12-build.*'

	require_no_glob_matches "$ROOT" 'agent-ext20-rc12-attestation*.sh'
	require_no_glob_matches "$ROOT" 'agent-ext20-rc12-remote*.sh'
	require_no_glob_matches "$ROOT" 'agent-ext20-rc12-suite*.sh'
	require_no_glob_matches "$ROOT" 'agent-ext20-rc12-live*.sh'
	[[ -z "$(git for-each-ref --format='%(refname)' \
		'refs/heads/level1-v0.1.0-rc.12*' \
		'refs/remotes/origin/level1-v0.1.0-rc.12*' \
		'refs/tags/*rc.12*')" ]] || fail 'Local RC.12 ref or tag collision'
}

verify_remote_collisions() {
	local artifact response releases asset
	[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.12*')" ]] || fail 'Remote RC.12 ref collision'
	[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]] || fail 'Remote RC.12 tag collision'
	for artifact in \
		level1-v0.1.0-rc.12-a24804bcf2a3 \
		level1-v0.1.0-rc.12-a24804bcf2a3-verification \
		level1-v0.1.0-rc.12-attestation
	do
		response="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
			"https://api.github.com/repos/ponchione/revolvr/actions/artifacts?name=$artifact&per_page=1")"
		printf '%s' "$response" | grep -Eq '"total_count"[[:space:]]*:[[:space:]]*0' ||
			fail "Actions artifact collision: $artifact"
	done
	releases="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
		'https://api.github.com/repos/ponchione/revolvr/releases?per_page=100')"
	for asset in \
		level1-v0.1.0-rc.12-a24804bcf2a3 \
		level1-v0.1.0-rc.12-a24804bcf2a3-verification \
		level1-v0.1.0-rc.12-attestation
	do
		if printf '%s' "$releases" | grep -Fq "\"name\": \"$asset\""; then
			fail "Release asset collision: $asset"
		fi
	done
}

verify_controller
verify_validation_evidence
verify_builder_and_self
verify_local_collisions
verify_remote_collisions

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'RC.12 construction preflight passed; builder was not executed (launcher SHA-256 %s)\n' \
		"$(sha256sum "$SELF" | awk '{print $1}')"
	exit 0
fi

launcher_sha256="$(sha256sum "$SELF" | awk '{print $1}')"
export REVOLVR_PROSPECTIVE_BUILDER_SHA256="$BUILDER_SHA256"
export REVOLVR_CONSTRUCTION_LAUNCHER_SHA256="$launcher_sha256"
export REVOLVR_VALIDATION_MANIFEST_SHA256="$MANIFEST_SHA256"
export REVOLVR_VALIDATION_STREAM_SHA256="$STREAM_SHA256"
exec "$BUILDER"
