#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT="/home/gernsback/source/revolvr"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
SOURCE_REMOTE="git@github.com:ponchione/revolvr.git"
SELF="$ROOT/agent-ext20-rc13-builder-revalidation-v6.sh"
SELF_REL="agent-ext20-rc13-builder-revalidation-v6.sh"
RC12_FAILURE_REVIEW="$ROOT/agent-ext20-rc12-construction-failure-review.sh"
V5_LAUNCHER="$ROOT/agent-ext20-rc13-builder-validation.sh"
V5_ROOT="$ROOT/.revolvr/prospective-builder-validation-v5.tL50Wc"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
RC13_BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.13.sh"
RC13_FINAL="$RELEASE_ROOT/level1-v0.1.0-rc.13-a24804bcf2a3"

RC12_FAILURE_REVIEW_SHA256="43cdfee5154ed70e689f4db7cc9df589f1b3bc6f56cd53a0ac6cd16c78148cd9"
V5_LAUNCHER_SHA256="052e97aa653f57bb380ccc130c1f1aa0181f8517f7e65332ab80727a6fcecb2c"
V5_STREAM_SHA256="6931b60c434205e2ce3130c119aa82750c117f8c947dd7c39f62b5011ddcb7e0"
V5_INVENTORY_SHA256="3f7403726cf59e3d02533deeb0c0f975e773adc0423e1f9a470eb30e5cf88cb5"

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
	[[ ! -e "$path" && ! -L "$path" ]] || fail "Prospective RC.13 v6 collision: $path"
}

require_no_glob_matches() {
	local base="$1" pattern="$2"
	if find "$base" -maxdepth 1 -mindepth 1 -name "$pattern" -print -quit | grep -q .; then
		fail "Prospective RC.13 v6 glob collision: $base/$pattern"
	fi
}

require_file_identity() {
	local mode="$1" size="$2" lines="$3" hash="$4" path="$5"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required history file is absent or unsafe: $path"
	[[ "$(stat -c '%a:%u:%h:%s' "$path")" == "$mode:$(id -u):1:$size" ]] || fail "History file identity changed: $path"
	[[ "$(wc -l <"$path")" == "$lines" ]] || fail "History file line count changed: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$hash" ]] || fail "History file hash changed: $path"
}

content_stream_sha256() {
	(
		cd "$1"
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	) | sha256sum | awk '{print $1}'
}

inventory_sha256() {
	(
		cd "$1"
		find . -mindepth 1 -maxdepth 1 -type f -printf '%f\0' | LC_ALL=C sort -z |
			while IFS= read -r -d '' file; do
				printf '%s\t%s\t%s\t%s\t%s\n' \
					"$(stat -c '%a' "$file")" "$(stat -c '%h' "$file")" "$(stat -c '%s' "$file")" \
					"$(sha256sum "$file" | awk '{print $1}')" "$file"
			done
	) | sha256sum | awk '{print $1}'
}

verify_public_controller() {
	local head_commit fetched_main public_main self_stage self_blob
	[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
	[[ "$(stat -c '%u' "$ROOT")" == "$(id -u)" ]] || fail 'Controller root owner changed'
	cd "$ROOT"
	git fetch --no-tags origin main
	[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail 'RC.13 v6 validation requires a clean published controller'
	[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Controller is not on local main'
	[[ "$(git remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'
	head_commit="$(git rev-parse HEAD)"
	fetched_main="$(git rev-parse refs/remotes/origin/main)"
	public_main="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
	[[ "$head_commit" == "$fetched_main" && "$head_commit" == "$public_main" ]] ||
		fail 'RC.13 v6 validation requires exact local, fetched, and public main'
	git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
	[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
	git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'
	[[ "$(realpath -e "$SELF")" == "$SELF" ]] || fail 'V6 launcher resolves unexpectedly'
	[[ "$(stat -c '%a:%u:%h' "$SELF")" == "755:$(id -u):1" ]] || fail 'V6 launcher identity changed'
	self_stage="$(git ls-files --stage -- "$SELF_REL")"
	[[ "$self_stage" == 100755\ *$'\t'"$SELF_REL" ]] || fail 'V6 launcher is not tracked executable'
	self_blob="$(git rev-parse "HEAD:$SELF_REL")"
	[[ "$(git hash-object "$SELF")" == "$self_blob" ]] || fail 'V6 launcher differs from published main'
}

verify_terminal_history() {
	require_file_identity 755 8862 182 "$RC12_FAILURE_REVIEW_SHA256" "$RC12_FAILURE_REVIEW"
	"$RC12_FAILURE_REVIEW" --preflight-only >/dev/null
	require_file_identity 755 9701 160 "$V5_LAUNCHER_SHA256" "$V5_LAUNCHER"
	[[ -d "$V5_ROOT" && ! -L "$V5_ROOT" ]] || fail 'Rejected v5 root is absent or unsafe'
	[[ "$(stat -c '%a:%u:%h' "$V5_ROOT")" == "700:$(id -u):2" ]] || fail 'Rejected v5 root identity changed'
	[[ "$(find "$V5_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 11 ]] || fail 'Rejected v5 file count changed'
	[[ "$(find "$V5_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')" == 44298 ]] ||
		fail 'Rejected v5 byte count changed'
	[[ -z "$(find "$V5_ROOT" -mindepth 1 \( -type l -o ! -type f \) -print -quit)" ]] || fail 'Rejected v5 root shape changed'
	[[ -z "$(find "$V5_ROOT" -mindepth 1 -type f \( ! -perm 0600 -o ! -links 1 \) -print -quit)" ]] ||
		fail 'Rejected v5 file mode or link count changed'
	[[ "$(content_stream_sha256 "$V5_ROOT")" == "$V5_STREAM_SHA256" ]] || fail 'Rejected v5 content stream changed'
	[[ "$(inventory_sha256 "$V5_ROOT")" == "$V5_INVENTORY_SHA256" ]] || fail 'Rejected v5 inventory changed'
	grep -Fx 'Terminal result: REJECTED; NO REVIEW OR CONSTRUCTION AUTHORITY.' "$V5_ROOT/validation-summary.txt" >/dev/null ||
		fail 'Rejected v5 terminal result changed'
	grep -F $'result\tFAIL' "$V5_ROOT/validation-sequence-1.tsv" >/dev/null || fail 'V5 sequence one result changed'
	grep -F $'result\tFAIL' "$V5_ROOT/validation-sequence-2.tsv" >/dev/null || fail 'V5 sequence two result changed'
}

verify_rc13_absence() {
	require_absent "$RC13_BUILDER"
	require_absent "$RC13_FINAL"
	require_absent "${RC13_FINAL}-verification"
	require_absent "$ROOT/agent-ext20-rc13.sh"
	require_absent "$ROOT/agent-ext20-rc13-builder-revalidation-v6-review.sh"
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
	require_no_glob_matches "$ROOT/.revolvr" 'prospective-builder-validation-v6.*'
	[[ -z "$(git for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.13*' 'refs/remotes/origin/level1-v0.1.0-rc.13*' 'refs/tags/*rc.13*')" ]] ||
		fail 'Local RC.13 ref or tag collision'
	[[ -z "$(git ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.13*')" ]] || fail 'Remote RC.13 ref collision'
	[[ -z "$(git ls-remote --tags origin '*rc.13*')" ]] || fail 'Remote RC.13 tag collision'

	local artifact response releases asset
	for artifact in level1-v0.1.0-rc.13-a24804bcf2a3 level1-v0.1.0-rc.13-a24804bcf2a3-verification level1-v0.1.0-rc.13-attestation; do
		response="$(curl -fsSL -H 'Accept: application/vnd.github+json' \
			"https://api.github.com/repos/ponchione/revolvr/actions/artifacts?name=$artifact&per_page=1")"
		printf '%s' "$response" | grep -Eq '"total_count"[[:space:]]*:[[:space:]]*0' || fail "Actions artifact collision: $artifact"
	done
	releases="$(curl -fsSL -H 'Accept: application/vnd.github+json' 'https://api.github.com/repos/ponchione/revolvr/releases?per_page=100')"
	for asset in level1-v0.1.0-rc.13-a24804bcf2a3 level1-v0.1.0-rc.13-a24804bcf2a3-verification level1-v0.1.0-rc.13-attestation; do
		if printf '%s' "$releases" | grep -Fq "\"name\": \"$asset\""; then fail "Release asset collision: $asset"; fi
	done
}

verify_public_controller
verify_terminal_history
verify_rc13_absence

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Prospective RC.13 v6 revalidation preflight passed; no design, builder, or candidate was created\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one fresh prospective EXT-20 RC.13 v6 builder-design revalidation pass:
- Treat RC.12, rejected v5 root $V5_ROOT, and every earlier candidate/design as immutable terminal history. Never execute, edit, copy, patch, transform, derive from, delete, seal, relabel, or reuse their builders, launchers, draft modes, scripts, or output bytes. Read their durable failure facts only as requirements. Do not rely on old session transcripts.
- Create no RC.13 builder, construction launcher, candidate, preflight/build/stage/diagnostic path, ref, tag, workflow, Actions artifact, release asset, suite, product test/build, full construction, Revolvr dogfood operation, release, external-use decision, or EXT-20 completion.
- Independently author a fresh design only under one unique persistent ignored root matching $ROOT/.revolvr/prospective-builder-validation-v6.XXXXXX. Retain authoritative evidence there, never only in /tmp. Preserve the rejected v5 root exactly at 11 files, 44,298 bytes, content stream $V5_STREAM_SHA256, and inventory $V5_INVENTORY_SHA256 before and after each sequence.
- Re-establish source $SOURCE_COMMIT and tree $SOURCE_TREE, unchanged product source, exact tools, surviving history, terminal lost-root absences, RC.12 terminal evidence, and all RC.13 collision absences. Recorded summaries never replace missing bytes.
- Independently implement the complete prospective construction requirements: two shallow source fetches, isolated exact Go environments/caches, both Go test/race/vet/module matrices, ordinary and verbose vulnerability evidence, reproducible supported-target builds, embedded metadata and empty build IDs, manifests, post-seal inventories, and terminal mkdir plus cp-a publication with distinct-inode and complete stage/final comparisons.
- Before sequence one, statically audit every trap for variable lifetime and prove successful, induced-failure, status-regression, and early-return cleanup with stable identities and no residue. Construct the available-history baseline from the exact canonical measurement bytes without command substitution newline loss or an added blank line; record and verify exact byte count, line count, SHA-256, and final-byte policy against a fresh independent measurement.
- Each of two complete sequences must reach and pass syntax, all cleanup variants, full-context role/collision audit, focused static audit, status-64 exact-self refusal, forbidden identity/residue audit, canonical history preservation including EOF identity, the no-collision status-propagation regression, rejected-v5 preservation, and accepted-byte preservation. A sequence that stops early fails. Permit at most one repair only between sequences; no repair after sequence two begins.
- If and only if both complete sequences pass with accepted bytes unchanged, seal exactly one persistent v6 root with a self-verifying manifest and create one inert tracked agent-ext20-rc13-builder-revalidation-v6-review.sh for later read-only review. Update TASKS, HANDOFF, STATE, and DECISIONS accurately; keep EXT-20 unchecked. Do not stage, commit, push, publish a builder, or grant construction authority. Stop after this task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
