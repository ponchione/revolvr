#!/usr/bin/env bash
set -euo pipefail

ROOT="/home/gernsback/source/revolvr"
EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx"
DRAFT="$EVIDENCE_ROOT/prospective-builder.sh"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.12.sh"
CONSTRUCTION_LAUNCHER="$ROOT/agent-ext20-rc12.sh"
FINAL_CANDIDATE="$RELEASE_ROOT/level1-v0.1.0-rc.12-a24804bcf2a3"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
CONTROLLER_COMMIT="ca702ef2931a006843c10b3b899db2b5ca0689dd"
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

stream_hash() {
	(
		cd "$1"
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	) | sha256sum | awk '{print $1}'
}

require_file_identity() {
	local mode="$1"
	local size="$2"
	local lines="$3"
	local hash="$4"
	local path="$5"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required publication file is absent or unsafe: $path"
	[[ "$(stat -c '%a:%u:%h:%s' "$path")" == "$mode:$(id -u):1:$size" ]] || fail "Publication identity changed: $path"
	[[ "$(wc -l <"$path")" == "$lines" ]] || fail "Publication line count changed: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$hash" ]] || fail "Publication hash changed: $path"
}

cd "$ROOT"
[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
git fetch --no-tags origin main
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
[[ "$HEAD_COMMIT" == "$CONTROLLER_COMMIT" && "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] ||
	fail 'Publication review requires the exact published controller'
[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Publication review requires local main'
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'
git diff --cached --quiet || fail 'Publication review refuses staged changes'

expected_status="$(cat <<'EOF'
 M .agent/DECISIONS.md
 M .agent/HANDOFF.md
 M .agent/STATE.md
 M .agent/TASKS.md
?? agent-ext20-rc12-builder-publication-review.sh
?? agent-ext20-rc12.sh
EOF
)"
[[ "$(git status --porcelain=v1 --untracked-files=all)" == "$expected_status" ]] ||
	fail 'Publication review scope changed'
git diff --check

[[ -d "$EVIDENCE_ROOT" && ! -L "$EVIDENCE_ROOT" ]] || fail 'Persistent validation root is absent or unsafe'
[[ "$(realpath -e "$EVIDENCE_ROOT")" == "$EVIDENCE_ROOT" ]] || fail 'Persistent validation root resolves unexpectedly'
[[ "$(stat -c '%a:%u' "$EVIDENCE_ROOT")" == "500:$(id -u)" ]] || fail 'Persistent validation root mode or owner changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 10 ]] || fail 'Persistent validation file count changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')" == 53626 ]] ||
	fail 'Persistent validation byte count changed'
[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 \( -type l -o ! -type f \) -print -quit)" ]] ||
	fail 'Persistent validation root contains a non-regular entry'
[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f ! -perm 0444 -print -quit)" ]] ||
	fail 'Persistent validation evidence mode changed'
[[ "$(sha256sum "$MANIFEST" | awk '{print $1}')" == "$MANIFEST_SHA256" ]] || fail 'Validation manifest changed'
[[ "$(stream_hash "$EVIDENCE_ROOT")" == "$STREAM_SHA256" ]] || fail 'Validation stream changed'

[[ "$(realpath -e "$RELEASE_ROOT")" == "$RELEASE_ROOT" ]] || fail 'Release-candidate parent resolves unexpectedly'
[[ "$(stat -c '%a:%u' "$RELEASE_ROOT")" == "755:$(id -u)" ]] || fail 'Release-candidate parent mode or owner changed'
require_file_identity 444 38528 756 "$BUILDER_SHA256" "$DRAFT"
require_file_identity 555 38528 756 "$BUILDER_SHA256" "$BUILDER"
cmp "$DRAFT" "$BUILDER" || fail 'Published builder differs from the reviewed draft'
[[ "$(stat -c '%d:%i' "$DRAFT")" != "$(stat -c '%d:%i' "$BUILDER")" ]] || fail 'Published builder reuses the sealed draft inode'
bash -n "$BUILDER"
[[ -z "$(git ls-files -- .revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh)" ]] || fail 'Published builder is tracked'
git check-ignore -q -- .revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh || fail 'Published builder is not ignored'

require_file_identity 755 14242 284 "$LAUNCHER_SHA256" "$CONSTRUCTION_LAUNCHER"
bash -n "$CONSTRUCTION_LAUNCHER"
[[ -z "$(git ls-files -- agent-ext20-rc12.sh agent-ext20-rc12-builder-publication-review.sh)" ]] ||
	fail 'Unreviewed publication launcher became tracked'
[[ "$(rg -c '^[[:space:]]*exec "\$BUILDER"$' "$CONSTRUCTION_LAUNCHER")" == 1 ]] ||
	fail 'Construction launcher execution boundary changed'
grep -F 'export REVOLVR_PROSPECTIVE_BUILDER_SHA256="$BUILDER_SHA256"' "$CONSTRUCTION_LAUNCHER" >/dev/null
grep -F 'export REVOLVR_CONSTRUCTION_LAUNCHER_SHA256="$launcher_sha256"' "$CONSTRUCTION_LAUNCHER" >/dev/null
grep -F 'export REVOLVR_VALIDATION_MANIFEST_SHA256="$MANIFEST_SHA256"' "$CONSTRUCTION_LAUNCHER" >/dev/null
grep -F 'export REVOLVR_VALIDATION_STREAM_SHA256="$STREAM_SHA256"' "$CONSTRUCTION_LAUNCHER" >/dev/null

for path in \
	"$FINAL_CANDIDATE" \
	"${FINAL_CANDIDATE}-verification" \
	"$ROOT/agent-ext20-rc12-local-review.sh" \
	"$ROOT/.github/workflows/level1-rc12-candidate-attestation.yml" \
	"$ROOT/.revolvr/ext20-rc12"
do
	[[ ! -e "$path" && ! -L "$path" ]] || fail "RC.12 publication collision: $path"
done
[[ -z "$(find "$ROOT/.revolvr" -maxdepth 1 -mindepth 1 \( -name 'ext20-rc12.*' -o -name 'ext20-rc12-*' \) -print -quit)" ]] ||
	fail 'RC.12 runtime descendant collision'
[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.12*' 'refs/remotes/origin/level1-v0.1.0-rc.12*' 'refs/tags/*rc.12*')" ]] ||
	fail 'Local RC.12 ref or tag collision'
[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.12*')" ]] || fail 'Remote RC.12 ref collision'
[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]] || fail 'Remote RC.12 tag collision'

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'RC.12 builder-publication review preflight passed; builder and construction launcher were not executed\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one read-only EXT-20 RC.12 builder-publication review:
- Review only the exact publication prepared in the current dirty scope. Never execute the builder, construction launcher, prospective draft mode, product test/build, construction, candidate, remote workflow, suite, Revolvr operation, release, or external-use action.
- Independently verify sealed persistent evidence, exact builder byte/mode/syntax identity, protected-parent mode 0755, construction-launcher byte/mode/syntax and static preflight/exec/export boundaries, historical controller hashes, source commit/tree and empty product diff, and complete RC.12 local/remote output, ref, tag, workflow, runtime, Actions-artifact, and release-asset collision absence.
- Verify that durable state keeps EXT-20 unchecked, identifies RC.12 as unconsumed, and names only later controller publication/review. Do not edit, stage, commit, push, or create any artifact, launcher, or continuation.
- Return a neutral accept/reject report only. Favorable review is not construction authority and cannot create further continuation."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
