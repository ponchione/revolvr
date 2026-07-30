#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

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

[[ -f .agent/LOOP_PROMPT.md ]] || fail 'Missing .agent/LOOP_PROMPT.md'

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] ||
	fail 'RC.12 builder publication requires a clean published controller repository'

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] ||
	fail 'RC.12 builder publication requires exact local, fetched, and public main'
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum ||
	fail 'Product source changed after the candidate source commit'

require_sha256() {
	local expected="$1"
	local path="$2"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required immutable file is absent or unsafe: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$expected" ]] || fail "Immutable file hash changed: $path"
}

EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx"
DRAFT="$EVIDENCE_ROOT/prospective-builder.sh"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
REVIEW_LAUNCHER="agent-ext20-rc12-builder-revalidation-v4-review.sh"
BUILDER="$ROOT/.revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh"
CONSTRUCTION_LAUNCHER="$ROOT/agent-ext20-rc12.sh"

require_sha256 \
	"b98e2b84c93d65beb805b96cb2b6b1bc28e69de145b1d7b51fe8fb072ae33a33" \
	"$REVIEW_LAUNCHER"
require_sha256 \
	"dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e" \
	"$DRAFT"
require_sha256 \
	"f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2" \
	"$MANIFEST"
[[ "$(stat -c '%a:%s' "$DRAFT")" == 444:38528 ]] || fail 'Reviewed draft mode or size changed'
[[ "$(wc -l <"$DRAFT")" == 756 ]] || fail 'Reviewed draft line count changed'
[[ "$(stat -c '%a' "$EVIDENCE_ROOT")" == 500 ]] || fail 'Persistent validation root is not sealed'
[[ "$(cd "$EVIDENCE_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum | sha256sum | awk '{print $1}')" == \
	"22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8" ]] ||
	fail 'Persistent validation root stream changed'
bash -n "$DRAFT"
"$ROOT/$REVIEW_LAUNCHER" --preflight-only

RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
[[ -d "$RELEASE_ROOT" && ! -L "$RELEASE_ROOT" ]] || fail 'Release-candidate parent is absent or unsafe'
[[ "$(realpath -e "$RELEASE_ROOT")" == "$RELEASE_ROOT" ]] || fail 'Release-candidate parent resolves unexpectedly'
[[ "$(stat -c '%u' "$RELEASE_ROOT")" == "$(id -u)" ]] || fail 'Release-candidate parent owner changed'

for forbidden_path in \
	"$BUILDER" \
	"$CONSTRUCTION_LAUNCHER" \
	"$ROOT/agent-ext20-rc12-builder-publication-review.sh" \
	"$ROOT/agent-ext20-rc12-local-review.sh" \
	"$ROOT/.github/workflows/level1-rc12-candidate-attestation.yml" \
	"$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.12-a24804bcf2a3" \
	"$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.12-a24804bcf2a3-verification"
do
	[[ ! -e "$forbidden_path" && ! -L "$forbidden_path" ]] || fail "RC.12 publication collision: $forbidden_path"
done
[[ ! -e "$ROOT/.revolvr/ext20-rc12" && ! -L "$ROOT/.revolvr/ext20-rc12" ]] || fail 'Exact RC.12 runtime root collision'
if find "$ROOT/.revolvr" -maxdepth 1 \( -name 'ext20-rc12.*' -o -name 'ext20-rc12-*' \) -print -quit | grep -q .; then
	fail 'RC.12 runtime descendant collision'
fi
[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.12*' 'refs/tags/*rc.12*')" ]] ||
	fail 'Local RC.12 ref or tag collision'
[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.12*')" ]] || fail 'Remote RC.12 ref collision'
[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]] || fail 'Remote RC.12 tag collision'

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'RC.12 builder-publication preflight passed; no identity was created\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one bounded EXT-20 RC.12 builder-publication pass:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not execute the prospective builder, any draft mode, product tests/builds, full construction, a candidate, remote workflow, suite, Revolvr/model operation, release, external-use action, or EXT-20 completion.
- The fourth persistent design review is accepted. Reverify sealed root $EVIDENCE_ROOT, draft SHA-256 dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e, manifest SHA-256 f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2, stream 22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8, syntax, and published review-launcher identity. Never edit the sealed evidence.
- Do exactly one task: publish exact reviewed draft bytes as the sole ignored read-only builder $BUILDER and create one inert tracked construction launcher $CONSTRUCTION_LAUNCHER. Do not run either. RC.12 remains unconsumed until a separately authorized builder execution creates construction identity.
- Before either named path appears, author the construction launcher only in a unique anonymous temporary directory, run bash -n and focused source review, and make all bytes final. It must have an explicit --preflight-only mode and accept no other arguments. No repair is allowed after either exact publication path appears.
- The construction launcher must fetch raw Git, require a clean controller with exact local/fetched/public main, preserve source commit $SOURCE_COMMIT and tree $SOURCE_TREE plus an empty product-source diff, and verify the exact persistent root, manifest, stream, builder bytes/mode/syntax, its own tracked mode/identity, historical controller hashes, and complete RC.12 output/ref/tag/workflow/runtime/Actions/release-asset collision absence.
- Its --preflight-only mode must stop before builder execution. Its no-argument path must dynamically hash its own final tracked bytes, export REVOLVR_PROSPECTIVE_BUILDER_SHA256=dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e, REVOLVR_CONSTRUCTION_LAUNCHER_SHA256 to that exact self hash, REVOLVR_VALIDATION_MANIFEST_SHA256=f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2, and REVOLVR_VALIDATION_STREAM_SHA256=22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8, then exec only $BUILDER with no argument.
- Treat $RELEASE_ROOT as a protected parent: require its exact real path and owner, remove group/other write permission before builder publication, and preserve that safer mode. Copy the sealed draft to the absent exact builder once, set mode 0555, and prove byte equality, size, line count, SHA-256, and bash syntax. Once the builder path appears, it is immutable and any failure is terminal without deletion, repair, or retry.
- Copy the already validated launcher bytes to the absent exact tracked launcher once, set mode 0755, and prove byte/hash/syntax identity. Do not stage, commit, or execute it. Any failure after appearance is terminal without deletion or repair.
- Update .agent/HANDOFF.md, .agent/STATE.md, .agent/DECISIONS.md, and the EXT-20 current gate with exact builder, launcher, parent-mode, and preservation results. Keep EXT-20 unchecked. Only if both exact publications pass, create one inert agent-ext20-rc12-builder-publication-review.sh for a later independent read-only review. It cannot execute construction or create further continuation. Stop after publication preparation."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
