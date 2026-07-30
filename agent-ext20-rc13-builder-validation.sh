#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT="/home/gernsback/source/revolvr"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
SOURCE_REMOTE="git@github.com:ponchione/revolvr.git"
SELF="$ROOT/agent-ext20-rc13-builder-validation.sh"
SELF_REL="agent-ext20-rc13-builder-validation.sh"
FAILURE_REVIEW="$ROOT/agent-ext20-rc12-construction-failure-review.sh"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
RC12_BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.12.sh"
RC13_BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.13.sh"
RC13_FINAL="$RELEASE_ROOT/level1-v0.1.0-rc.13-a24804bcf2a3"

FAILURE_REVIEW_SHA256="43cdfee5154ed70e689f4db7cc9df589f1b3bc6f56cd53a0ac6cd16c78148cd9"
RC12_BUILDER_SHA256="dfa46ae7c21cb238cf2191696de159aee19b4fe46c5a835b77a130acb583d90e"

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

require_absent() {
	local path="$1"
	[[ ! -e "$path" && ! -L "$path" ]] || fail "Prospective RC.13 collision: $path"
}

require_no_glob_matches() {
	local base="$1"
	local pattern="$2"
	if find "$base" -maxdepth 1 -mindepth 1 -name "$pattern" -print -quit | grep -q .; then
		fail "Prospective RC.13 glob collision: $base/$pattern"
	fi
}

require_file_hash() {
	local expected="$1"
	local path="$2"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required terminal-history file is absent or unsafe: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$expected" ]] || fail "Terminal-history file changed: $path"
}

verify_public_controller() {
	local head_commit fetched_main public_main self_stage self_blob
	[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
	[[ "$(stat -c '%u' "$ROOT")" == "$(id -u)" ]] || fail 'Controller root owner changed'
	cd "$ROOT"
	git fetch --no-tags origin main
	[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail 'RC.13 design validation requires a clean published controller'
	[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Controller is not on local main'
	[[ "$(git remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'
	head_commit="$(git rev-parse HEAD)"
	fetched_main="$(git rev-parse refs/remotes/origin/main)"
	public_main="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
	[[ "$head_commit" == "$fetched_main" && "$head_commit" == "$public_main" ]] ||
		fail 'RC.13 design validation requires exact local, fetched, and public main'
	git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
	[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
	git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'

	[[ "$(realpath -e "$SELF")" == "$SELF" ]] || fail 'RC.13 validation launcher resolves unexpectedly'
	[[ "$(stat -c '%a:%u:%h' "$SELF")" == "755:$(id -u):1" ]] || fail 'RC.13 validation launcher identity changed'
	self_stage="$(git ls-files --stage -- "$SELF_REL")"
	[[ "$self_stage" == 100755\ *$'\t'"$SELF_REL" ]] || fail 'RC.13 validation launcher is not tracked executable'
	self_blob="$(git rev-parse "HEAD:$SELF_REL")"
	[[ "$(git hash-object "$SELF")" == "$self_blob" ]] || fail 'RC.13 validation launcher differs from published main'
}

verify_terminal_rc12() {
	require_file_hash "$FAILURE_REVIEW_SHA256" "$FAILURE_REVIEW"
	[[ "$(stat -c '%a:%u:%h:%s' "$FAILURE_REVIEW")" == "755:$(id -u):1:8862" ]] || fail 'RC.12 failure-review identity changed'
	require_file_hash "$RC12_BUILDER_SHA256" "$RC12_BUILDER"
	[[ "$(stat -c '%a:%u:%h:%s' "$RC12_BUILDER")" == "555:$(id -u):1:38528" ]] || fail 'RC.12 terminal builder identity changed'
	"$FAILURE_REVIEW" --preflight-only >/dev/null
}

verify_rc13_absence() {
	require_absent "$RC13_BUILDER"
	require_absent "$RC13_FINAL"
	require_absent "${RC13_FINAL}-verification"
	require_absent "$ROOT/agent-ext20-rc13.sh"
	require_absent "$ROOT/agent-ext20-rc13-builder-validation-review.sh"
	require_absent "$ROOT/agent-ext20-rc13-builder-publication.sh"
	require_absent "$ROOT/agent-ext20-rc13-local-review.sh"
	require_absent "$ROOT/.github/workflows/level1-rc13-candidate-attestation.yml"
	require_absent "$ROOT/.revolvr/ext20-rc13"
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.13-preflight.*'
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.13-stage.*'
	require_no_glob_matches "$RELEASE_ROOT/diagnostics" 'level1-v0.1.0-rc.13-*'
	require_no_glob_matches /tmp 'revolvr-ext20-rc13-build.*'
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc13.*'
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc13-*'
	require_no_glob_matches "$ROOT/.revolvr" 'prospective-builder-validation-v5.*'
	[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.13*' 'refs/remotes/origin/level1-v0.1.0-rc.13*' 'refs/tags/*rc.13*')" ]] ||
		fail 'Local RC.13 ref or tag collision'
	[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.13*')" ]] || fail 'Remote RC.13 ref collision'
	[[ -z "$(git ls-remote --tags origin '*rc.13*')" ]] || fail 'Remote RC.13 tag collision'

	local artifact response releases asset
	for artifact in \
		level1-v0.1.0-rc.13-a24804bcf2a3 \
		level1-v0.1.0-rc.13-a24804bcf2a3-verification \
		level1-v0.1.0-rc.13-attestation
	do
		response="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
			"https://api.github.com/repos/ponchione/revolvr/actions/artifacts?name=$artifact&per_page=1")"
		printf '%s' "$response" | grep -Eq '"total_count"[[:space:]]*:[[:space:]]*0' || fail "Actions artifact collision: $artifact"
	done
	releases="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
		'https://api.github.com/repos/ponchione/revolvr/releases?per_page=100')"
	for asset in \
		level1-v0.1.0-rc.13-a24804bcf2a3 \
		level1-v0.1.0-rc.13-a24804bcf2a3-verification \
		level1-v0.1.0-rc.13-attestation
	do
		if printf '%s' "$releases" | grep -Fq "\"name\": \"$asset\""; then
			fail "Release asset collision: $asset"
		fi
	done
}

verify_public_controller
verify_terminal_rc12
verify_rc13_absence

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Prospective RC.13 builder-validation preflight passed; no design, builder, or candidate was created\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one prospective EXT-20 RC.13 builder-design validation pass:
- Treat RC.12 and every earlier candidate as immutable terminal history. Never execute, copy, patch, transform, derive from, delete, or reuse any historical builder, construction launcher, draft mode, candidate output, or lost-root summary. Do not use old session transcripts as required context.
- Create no RC.13 builder, construction launcher, candidate, preflight/build/stage/diagnostic path, ref, tag, workflow, Actions artifact, release asset, suite, Revolvr/model dogfood operation, external-use decision, or EXT-20 completion. Do not run product tests/builds or full construction.
- Independently author a fresh prospective RC.13 construction design only under one unique persistent ignored root matching $ROOT/.revolvr/prospective-builder-validation-v5.XXXXXX. Do not place authoritative retained evidence only in /tmp. The prospective draft may describe RC.13 paths but must never appear at an RC.13 builder path.
- Re-establish exact source commit $SOURCE_COMMIT and tree $SOURCE_TREE, unchanged product-source boundary, tool identities, all surviving historical identities/manifests, every terminal lost-root absence, and complete RC.12 terminal failure identities and absent outputs. Never treat recorded summaries as replacement bytes.
- The design must preserve two independent shallow source fetches, isolated caches, clean exact Go environments, both Go verification matrices, vet/module/vulnerability evidence, reproducible supported-target builds, embedded metadata and empty build IDs, manifests, post-seal inventories, and terminal mkdir plus cp-a publication with distinct-inode and complete stage/final comparisons.
- Correct the RC.12 status-propagation defect by construction, not by editing RC.12. Every collision-check function and loop must explicitly return success when all required absences pass. Add a neutral regression that proves all no-collision paths return 0 and audit every final false-negative probe or AND-list for unintended function status.
- Before accepting the prospective design, run two complete immutable validation sequences. Each must include bash syntax, successful and induced-failure semantic publication/cleanup probes, full-context role/collision audit, focused static audit, expected status-64 exact-self refusal, forbidden identity/residue audit, available-history preservation, and the new status-propagation regression. Permit at most one neutral repair only between sequences; any later failure stops.
- If and only if both sequences pass with accepted bytes unchanged, seal exactly one complete persistent evidence root with a self-verifying manifest and create one inert tracked agent-ext20-rc13-builder-validation-review.sh for a later read-only review. Update TASKS, HANDOFF, STATE, and DECISIONS accurately; keep EXT-20 unchecked. Do not stage, commit, push, or grant builder publication/construction authority. Stop after this one task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
