#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT="/home/gernsback/source/revolvr"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
SOURCE_REMOTE="git@github.com:ponchione/revolvr.git"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.12.sh"
CONSTRUCTION_LAUNCHER="$ROOT/agent-ext20-rc12.sh"
EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx"
DRAFT="$EVIDENCE_ROOT/prospective-builder.sh"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
FINAL_CANDIDATE="$RELEASE_ROOT/level1-v0.1.0-rc.12-a24804bcf2a3"
FINAL_VERIFICATION="${FINAL_CANDIDATE}-verification"

BUILDER_SHA256="dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e"
LAUNCHER_SHA256="f2a5f95323cf95334aed2c79c08368d63d0a73646600155a0032e6027bec6572"
MANIFEST_SHA256="f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2"
STREAM_SHA256="22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8"

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

require_file_identity() {
	local mode="$1"
	local size="$2"
	local lines="$3"
	local hash="$4"
	local path="$5"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required failure-record file is absent or unsafe: $path"
	[[ "$(stat -c '%a:%u:%h:%s' "$path")" == "$mode:$(id -u):1:$size" ]] ||
		fail "Failure-record identity changed: $path"
	[[ "$(wc -l <"$path")" == "$lines" ]] || fail "Failure-record line count changed: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$hash" ]] || fail "Failure-record hash changed: $path"
}

require_absent() {
	local path="$1"
	[[ ! -e "$path" && ! -L "$path" ]] || fail "Unexpected RC.12 construction path: $path"
}

require_no_glob_matches() {
	local base="$1"
	local pattern="$2"
	if find "$base" -maxdepth 1 -mindepth 1 -name "$pattern" -print -quit | grep -q .; then
		fail "Unexpected RC.12 construction residue: $base/$pattern"
	fi
}

content_stream_sha256() {
	(
		cd "$1"
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	) | sha256sum | awk '{print $1}'
}

verify_status_bug() {
	local status
	if bash -c '
		set -euo pipefail
		status_bug() {
			local item
			for item in one two; do
				printf "%s" "$item" | grep -Fq absent && return 99
			done
		}
		status_bug
	'; then
		status=0
	else
		status=$?
	fi
	[[ "$status" == 1 ]] || fail 'Bash final-loop status behavior changed'
	grep -Fn 'printf '\''%s'\'' "$response" | grep -Fq "\"name\": \"$asset\"" && fail "release asset collision: $asset"' "$BUILDER" |
		grep -Fx '498:'"$(sed -n '498p' "$BUILDER")" >/dev/null || fail 'Builder status-bug site changed'
}

cd "$ROOT"
[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
[[ "$(stat -c '%u' "$ROOT")" == "$(id -u)" ]] || fail 'Controller root owner changed'
git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail 'Failure review requires a clean published controller'
[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Failure review requires local main'
[[ "$(git remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] ||
	fail 'Failure review requires exact local, fetched, and public main'
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'

[[ "$(stat -c '%a:%u' "$RELEASE_ROOT")" == "755:$(id -u)" ]] || fail 'Release-candidate parent changed'
require_file_identity 555 38528 756 "$BUILDER_SHA256" "$BUILDER"
require_file_identity 755 14242 284 "$LAUNCHER_SHA256" "$CONSTRUCTION_LAUNCHER"
require_file_identity 444 38528 756 "$BUILDER_SHA256" "$DRAFT"
require_file_identity 444 902 9 "$MANIFEST_SHA256" "$MANIFEST"
cmp "$BUILDER" "$DRAFT" || fail 'Exact builder differs from sealed draft'
[[ "$(stat -c '%d:%i' "$BUILDER")" != "$(stat -c '%d:%i' "$DRAFT")" ]] || fail 'Builder reuses sealed draft inode'
bash -n "$BUILDER"
bash -n "$CONSTRUCTION_LAUNCHER"
[[ -d "$EVIDENCE_ROOT" && ! -L "$EVIDENCE_ROOT" && "$(stat -c '%a:%u' "$EVIDENCE_ROOT")" == "500:$(id -u)" ]] ||
	fail 'Persistent validation root changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 10 ]] || fail 'Persistent validation count changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')" == 53626 ]] ||
	fail 'Persistent validation byte count changed'
[[ "$(content_stream_sha256 "$EVIDENCE_ROOT")" == "$STREAM_SHA256" ]] || fail 'Persistent validation stream changed'

require_absent "$FINAL_CANDIDATE"
require_absent "$FINAL_VERIFICATION"
require_absent "$ROOT/agent-ext20-rc12-local-review.sh"
require_absent "$ROOT/.github/workflows/level1-rc12-candidate-attestation.yml"
require_absent "$ROOT/.revolvr/ext20-rc12"
require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.12-preflight.*'
require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.12-stage.*'
require_no_glob_matches "$RELEASE_ROOT/diagnostics" 'level1-v0.1.0-rc.12-*'
require_no_glob_matches /tmp 'revolvr-ext20-rc12-build.*'
require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc12.*'
require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc12-*'
[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.12*' 'refs/remotes/origin/level1-v0.1.0-rc.12*' 'refs/tags/*rc.12*')" ]] ||
	fail 'Local RC.12 ref or tag collision'
[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.12*')" ]] || fail 'Remote RC.12 ref collision'
[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]] || fail 'Remote RC.12 tag collision'

verify_status_bug

for artifact in \
	level1-v0.1.0-rc.12-a24804bcf2a3 \
	level1-v0.1.0-rc.12-a24804bcf2a3-verification \
	level1-v0.1.0-rc.12-attestation
do
	response="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
		"https://api.github.com/repos/ponchione/revolvr/actions/artifacts?name=$artifact&per_page=1")"
	printf '%s' "$response" | grep -Eq '"total_count"[[:space:]]*:[[:space:]]*0' || fail "Actions artifact collision: $artifact"
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

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'RC.12 terminal construction-failure review preflight passed; builder and construction launcher were not executed\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one read-only EXT-20 RC.12 construction-failure review:
- Review only the immutable RC.12 construction failure and its current durable record. Never execute the builder, construction launcher, any draft mode, product test/build, candidate construction, remote workflow, suite, Revolvr operation, release, or external-use action.
- Independently verify exact builder/draft/launcher/sealed-evidence identities; the absence of every RC.12 preflight/build/stage/diagnostic/final/runtime/ref/tag/workflow/artifact/release output; and unchanged source/product history.
- Verify the deterministic root cause in exact builder lines 496-499: when release assets are absent, the final grep-and-fail AND-list returns status 1, the final for loop propagates 1 from verify_remote_collisions, and set -e reaches the generic terminal trap before the first construction root.
- Treat the operator-observed no-argument builder execution as consuming RC.12. Do not retry, repair, delete, relabel, derive from, or reuse the builder or any RC.12 identity. EXT-20 remains unchecked.
- Do not edit, stage, commit, push, or create any artifact, launcher, candidate, or continuation. Return a neutral accept/reject report only. Any fresh candidate identity requires a later separate controller decision and explicit operator authority."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
