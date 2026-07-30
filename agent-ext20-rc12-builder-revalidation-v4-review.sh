#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"
EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx"
DRAFT="$EVIDENCE_ROOT/prospective-builder.sh"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
STATIC_AUDIT="$EVIDENCE_ROOT/focused-static-audit.sh"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

diagnostic() {
	local name="$1"
	shift
	if "$@"; then printf 'PASS\t%s\n' "$name"; else printf 'FAIL\t%s\n' "$name" >&2; exit 1; fi
}

stream_hash() {
	(cd "$1" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum) |
		sha256sum | awk '{print $1}'
}

root_identity() {
	[[ "$ROOT" == /home/gernsback/source/revolvr ]]
	[[ -d "$EVIDENCE_ROOT" && ! -L "$EVIDENCE_ROOT" ]]
	[[ "$(realpath -e "$EVIDENCE_ROOT")" == "$ROOT/.revolvr/prospective-builder-revalidation-v4.5pWwTx" ]]
	[[ "$(stat -c '%u:%a' "$EVIDENCE_ROOT")" == "$(id -u):500" ]]
}

root_shape() {
	[[ "$(find "$EVIDENCE_ROOT" -type f | wc -l)" == 10 ]]
	[[ "$(find "$EVIDENCE_ROOT" -type f -printf '%s\n' | awk '{s += $1} END {print s + 0}')" == 53626 ]]
	[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 \( -type l -o ! -type f \) -print -quit)" ]]
	[[ -z "$(find "$EVIDENCE_ROOT" -type f ! -perm 0444 -print -quit)" ]]
	[[ -z "$(find "$EVIDENCE_ROOT" -type f ! -links 1 -print -quit)" ]]
}

manifest_identity() {
	[[ "$(stat -c '%a:%s' "$MANIFEST")" == 444:902 ]]
	[[ "$(wc -l <"$MANIFEST")" == 9 ]]
	[[ "$(sha256sum "$MANIFEST" | awk '{print $1}')" == f4cbe051d3b6cb13cb111b7374fb3d17c99e6f93979cb31853bbcd1db3da91c2 ]]
}

manifest_entries() {
	local name mode size expected
	while IFS=$'\t' read -r name mode size expected; do
		[[ -n "$name" && "$name" != */* && "$name" != evidence-manifest.tsv ]]
		[[ -f "$EVIDENCE_ROOT/$name" && ! -L "$EVIDENCE_ROOT/$name" ]]
		[[ "$(stat -c '%a:%s' "$EVIDENCE_ROOT/$name")" == "$mode:$size" ]]
		[[ "$(sha256sum "$EVIDENCE_ROOT/$name" | awk '{print $1}')" == "$expected" ]]
	done <"$MANIFEST"
}

draft_identity() {
	[[ "$(stat -c '%a:%s' "$DRAFT")" == 444:38528 ]]
	[[ "$(wc -l <"$DRAFT")" == 756 ]]
	[[ "$(sha256sum "$DRAFT" | awk '{print $1}')" == dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e ]]
	bash -n "$DRAFT"
}

sequence_results() {
	local file tuple
	for file in "$EVIDENCE_ROOT/validation-sequence-1.tsv" "$EVIDENCE_ROOT/validation-sequence-2.tsv"; do
		tuple="$(awk -F '\t' 'NR > 1 && $1 != "sequence" { printf "%s,", $2 }' "$file")"
		[[ "$tuple" == '0,0,0,0,64,0,0,' ]]
		grep -F $'sequence\t0\tpass' "$file" >/dev/null
	done
	grep -F 'one post-sequence repair authorized' "$EVIDENCE_ROOT/validation-sequence-1.tsv" >/dev/null
	grep -F 'dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e unchanged' "$EVIDENCE_ROOT/validation-sequence-2.tsv" >/dev/null
}

loss_and_residue() {
	local path
	for path in /tmp/revolvr-ext20-rc8-build.wnKv7Q /tmp/revolvr-ext20-rc9-build.CRYAYI \
		/tmp/revolvr-builder-draft.cZLxf2 /tmp/revolvr-builder-validation.maYqgv \
		/tmp/revolvr-builder-revalidation.CSGs5E /tmp/revolvr-builder-revalidation-v3.PKfbRl; do
		[[ ! -e "$path" && ! -L "$path" ]]
	done
	! find /tmp -maxdepth 1 -name 'revolvr-neutral-publication.*' -print -quit | grep -q .
}

source_and_tools() {
	[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]]
	git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD
	git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum
	[[ "$(sha256sum /usr/local/go/bin/go | awk '{print $1}')" == 8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9 ]]
	[[ "$(sha256sum /home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64/bin/go | awk '{print $1}')" == 929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec ]]
	[[ "$(sha256sum /home/gernsback/go/bin/govulncheck | awk '{print $1}')" == f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f ]]
}

controller_history() {
	[[ "$(sha256sum agent-ext20-rc12-builder-revalidation-v3.sh | awk '{print $1}')" == 591a79098ced60f9aa0abbd11f66cf722b2adba22b1fd277237e0890aa536ee5 ]]
	[[ "$(sha256sum agent-ext20-rc12-builder-revalidation-v3-review.sh | awk '{print $1}')" == 3bafb2b55cde7a872e5b159f3fc9e721d39942b208f83718875721d45dca888d ]]
	[[ "$(sha256sum agent-ext20-rc12-builder-revalidation-v4.sh | awk '{print $1}')" == 5d9c82acbe9527e93421355a06843d60a2dd55c877dc2fb856c367fd02bc647c ]]
	[[ "$(sha256sum agent-ext20-rc12-volatile-root-recovery-review.sh | awk '{print $1}')" == 8def05def6c116b2b4645090a0661bd70146d52076710214b8be1084c3f771ea ]]
}

role_and_design_guards() {
	bash "$STATIC_AUDIT" "$DRAFT" >/dev/null
	for literal in 'BUILDER_PATH=' 'CONSTRUCTION_LAUNCHER=' 'VALIDATION_ROOT=' \
		'require_absent "$CONTROLLER_ROOT/.revolvr/ext20-rc12"' \
		'level1-v0.1.0-rc.12-a24804bcf2a3' \
		'level1-v0.1.0-rc.12-a24804bcf2a3-verification' \
		'level1-v0.1.0-rc.12-attestation' \
		'find "$root" -type f -perm /0111 -exec chmod 0500 -- {} +' \
		'verify_staged_manifest_pair "$FINAL_CANDIDATE"' \
		'verify_staged_manifest_pair "$FINAL_VERIFICATION"'; do
		grep -F "$literal" "$DRAFT" >/dev/null
	done
}

collision_absence() {
	[[ ! -e .revolvr/ext20-rc12 && ! -L .revolvr/ext20-rc12 ]]
	! find .revolvr -maxdepth 1 \( -name 'ext20-rc12.*' -o -name 'ext20-rc12-*' \) -print -quit | grep -q .
	for path in .revolvr/release-candidates/build-level1-v0.1.0-rc.12.sh agent-ext20-rc12.sh \
		agent-ext20-rc12-local-review.sh .github/workflows/level1-rc12-candidate-attestation.yml; do
		[[ ! -e "$path" && ! -L "$path" ]]
	done
	[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.12*' 'refs/tags/*rc.12*')" ]]
}

diagnostic persistent-root-identity root_identity
diagnostic evidence-count-bytes-modes-links root_shape
diagnostic manifest-identity manifest_identity
diagnostic manifest-self-verification manifest_entries
diagnostic complete-root-stream test "$(stream_hash "$EVIDENCE_ROOT")" = 22e50f2dfc7ce0f0e935b080f567a0527b7e6a943234241161977c78fdaa9cd8
diagnostic draft-identity-and-syntax draft_identity
diagnostic two-sequences-and-single-repair sequence_results
diagnostic volatile-loss-and-probe-residue loss_and_residue
diagnostic source-product-tool-boundary source_and_tools
diagnostic validation-recovery-history controller_history
diagnostic role-model-four-concerns-executable-payloads role_and_design_guards
diagnostic prospective-output-ref-tag-absence collision_absence

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Fourth persistent builder-revalidation review preflight passed; draft was not executed\n'
	exit 0
fi

git fetch --no-tags origin main
diagnostic published-controller-clean test -z "$(git status --porcelain=v1 --untracked-files=all)"
diagnostic review-launcher-tracked git ls-files --error-unmatch agent-ext20-rc12-builder-revalidation-v4-review.sh
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
diagnostic local-fetched-public-main-exact test "$HEAD_COMMIT" = "$FETCHED_MAIN"
diagnostic fetched-public-main-exact test "$FETCHED_MAIN" = "$PUBLIC_MAIN"

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one read-only EXT-20 fourth-design review:
- Review only sealed persistent root $EVIDENCE_ROOT. Never execute the draft or any mode; bash -n and static inspection are permitted.
- Verify the manifest, two accepted sequences and single repair, loss boundary, cleanup design, corrected role model and four controller concerns, executable-payload preservation, two shallow fetches, verification evidence design, and terminal mkdir/cp-a publication.
- Do not edit files, run product tests/builds or full mode, create any identity/artifact/launcher, or grant continuation, construction, candidate, remote, suite, release, external-use, or EXT-20 authority.
- Return a neutral accept/reject report only; favorable review is not continuation authority."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local \
	codex exec --dangerously-bypass-approvals-and-sandbox --cd "$ROOT" "$PROMPT"
