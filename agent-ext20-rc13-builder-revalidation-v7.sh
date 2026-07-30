#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT="/home/gernsback/source/revolvr"
SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
SOURCE_REMOTE="git@github.com:ponchione/revolvr.git"
SELF="$ROOT/agent-ext20-rc13-builder-revalidation-v7.sh"
SELF_REL="agent-ext20-rc13-builder-revalidation-v7.sh"
V6_REVIEW="$ROOT/agent-ext20-rc13-builder-revalidation-v6-review.sh"
V6_ROOT="$ROOT/.revolvr/prospective-builder-validation-v6.bHfL29"
V6_DESIGN="$V6_ROOT/prospective-construction-design.sh"
RELEASE_ROOT="$ROOT/.revolvr/release-candidates"
RC13_BUILDER="$RELEASE_ROOT/build-level1-v0.1.0-rc.13.sh"
RC13_FINAL="$RELEASE_ROOT/level1-v0.1.0-rc.13-a24804bcf2a3"

V6_REVIEW_SHA256="c0c187322fb4597b62666332ff8595296f81c59b8fb853a0ab24dc799a06f5e2"
V6_DESIGN_SHA256="e457a4f8566f24fe5cd824cc8dc186a96019470838b94b9794069037cd03b8ff"
V6_STREAM_SHA256="1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f"
V6_INVENTORY_SHA256="0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41"

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
	[[ ! -e "$path" && ! -L "$path" ]] || fail "Prospective RC.13 v7 collision: $path"
}

require_no_glob_matches() {
	local base="$1" pattern="$2"
	if find "$base" -maxdepth 1 -mindepth 1 -name "$pattern" -print -quit | grep -q .; then
		fail "Prospective RC.13 v7 glob collision: $base/$pattern"
	fi
}

require_file_identity() {
	local mode="$1" size="$2" lines="$3" hash="$4" path="$5"
	[[ -f "$path" && ! -L "$path" ]] || fail "Required v6 history file is absent or unsafe: $path"
	[[ "$(stat -c '%a:%u:%h:%s' "$path")" == "$mode:$(id -u):1:$size" ]] || fail "V6 history file identity changed: $path"
	[[ "$(wc -l <"$path")" == "$lines" ]] || fail "V6 history file line count changed: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$hash" ]] || fail "V6 history file hash changed: $path"
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
				printf '%s\t%s\t%s\t%s\t%s\n' "$(stat -c '%a' "$file")" "$(stat -c '%h' "$file")" \
					"$(stat -c '%s' "$file")" "$(sha256sum "$file" | awk '{print $1}')" "$file"
			done
	) | sha256sum | awk '{print $1}'
}

verify_public_controller() {
	local head_commit fetched_main public_main self_stage self_blob
	[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
	[[ "$(stat -c '%u' "$ROOT")" == "$(id -u)" ]] || fail 'Controller root owner changed'
	cd "$ROOT"
	git fetch --no-tags origin main
	[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail 'RC.13 v7 validation requires a clean published controller'
	[[ "$(git symbolic-ref --short HEAD)" == main ]] || fail 'Controller is not on local main'
	[[ "$(git remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'
	head_commit="$(git rev-parse HEAD)"
	fetched_main="$(git rev-parse refs/remotes/origin/main)"
	public_main="$(git ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
	[[ "$head_commit" == "$fetched_main" && "$head_commit" == "$public_main" ]] ||
		fail 'RC.13 v7 validation requires exact local, fetched, and public main'
	git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || fail 'Candidate source is not in controller ancestry'
	[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Candidate source tree changed'
	git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'
	[[ "$(realpath -e "$SELF")" == "$SELF" ]] || fail 'V7 launcher resolves unexpectedly'
	[[ "$(stat -c '%a:%u:%h' "$SELF")" == "755:$(id -u):1" ]] || fail 'V7 launcher identity changed'
	self_stage="$(git ls-files --stage -- "$SELF_REL")"
	[[ "$self_stage" == 100755\ *$'\t'"$SELF_REL" ]] || fail 'V7 launcher is not tracked executable'
	self_blob="$(git rev-parse "HEAD:$SELF_REL")"
	[[ "$(git hash-object "$SELF")" == "$self_blob" ]] || fail 'V7 launcher differs from published main'
}

verify_rejected_v6() {
	require_file_identity 755 9215 146 "$V6_REVIEW_SHA256" "$V6_REVIEW"
	"$V6_REVIEW" --preflight-only >/dev/null
	require_file_identity 444 22459 541 "$V6_DESIGN_SHA256" "$V6_DESIGN"
	[[ "$(stat -c '%a:%u:%h' "$V6_ROOT")" == "500:$(id -u):2" ]] || fail 'Sealed v6 root identity changed'
	[[ "$(find "$V6_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 13 ]] || fail 'V6 file count changed'
	[[ "$(find "$V6_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{sum += $1} END {print sum + 0}')" == 45820 ]] ||
		fail 'V6 byte count changed'
	[[ "$(content_stream_sha256 "$V6_ROOT")" == "$V6_STREAM_SHA256" ]] || fail 'V6 content stream changed'
	[[ "$(inventory_sha256 "$V6_ROOT")" == "$V6_INVENTORY_SHA256" ]] || fail 'V6 inventory changed'
	grep -F '"$ROOT/agent-ext20-rc13-builder-revalidation-v6-review.sh"' "$V6_DESIGN" >/dev/null || fail 'V6 role contradiction evidence changed'
	grep -F '"$ROOT/agent-ext20-rc13-builder-publication.sh"' "$V6_DESIGN" >/dev/null || fail 'V6 publication-role contradiction changed'
	[[ -z "$(grep -F 'cmp ' "$V6_DESIGN" | grep -F 'prospective-construction-design.sh' || true)" ]] || fail 'V6 exact-self defect evidence changed'
	[[ -z "$(grep -F 'chmod 0500 "$tree"' "$V6_DESIGN" || true)" ]] || fail 'V6 root-sealing defect evidence changed'
}

verify_rc13_absence() {
	require_absent "$RC13_BUILDER"
	require_absent "$RC13_FINAL"
	require_absent "${RC13_FINAL}-verification"
	require_absent "$ROOT/agent-ext20-rc13.sh"
	require_absent "$ROOT/agent-ext20-rc13-builder-publication.sh"
	require_absent "$ROOT/agent-ext20-rc13-builder-revalidation-v7-review.sh"
	require_absent "$ROOT/agent-ext20-rc13-local-review.sh"
	require_absent "$ROOT/.github/workflows/level1-rc13-candidate-attestation.yml"
	require_absent "$ROOT/.revolvr/ext20-rc13"
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.13-preflight.*'
	require_no_glob_matches "$RELEASE_ROOT" '.level1-v0.1.0-rc.13-stage.*'
	require_no_glob_matches "$RELEASE_ROOT/diagnostics" 'level1-v0.1.0-rc.13-*'
	require_no_glob_matches /tmp 'revolvr-ext20-rc13-build.*'
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc13.*'
	require_no_glob_matches "$ROOT/.revolvr" 'ext20-rc13-*'
	require_no_glob_matches "$ROOT/.revolvr" 'prospective-builder-validation-v7.*'
}

verify_public_controller
verify_rejected_v6
verify_rc13_absence

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Prospective RC.13 v7 revalidation preflight passed; no design, builder, or candidate was created\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for exactly one fresh prospective EXT-20 RC.13 v7 builder-design revalidation pass:
- Treat RC.12, rejected v5, and sealed-but-publication-rejected v6 at $V6_ROOT as immutable terminal design history. Never execute, edit, copy, patch, transform, derive from, delete, unseal, relabel, or reuse their builders, launchers, draft modes, scripts, or output bytes. Use only durable requirements and independently remeasured facts. Do not rely on old transcripts.
- Create no RC.13 builder, construction launcher, candidate, preflight/build/stage/diagnostic path, ref, tag, workflow, Actions artifact, release asset, suite, product test/build, full construction, Revolvr dogfood operation, release, external-use decision, or EXT-20 completion.
- Independently author a fresh design only under one unique persistent ignored root matching $ROOT/.revolvr/prospective-builder-validation-v7.XXXXXX. Preserve v5 and v6 exact before and after every sequence. Retain authoritative evidence persistently, never only in /tmp.
- Correct every v6 review blocker by design. Full-role admission must require the exact builder and tracked construction launcher, permit tracked validation/review/publication history, and reject only actual output/runtime/ref/tag/artifact collisions. A neutral full-context role simulation must prove that the exact future tracked history set does not contradict full admission.
- Full mode must prove exact builder byte equality with the sealed v7 design, sealed-root manifest/stream/inventory authority, builder mode/owner/link count, construction-launcher tracked blob and dynamic self hash, current clean exact local/fetched/public controller commit, unchanged product source through that current commit, and explicit authority exported by the launcher. No path/mode-only builder admission is sufficient.
- Seal stage candidate and verification roots themselves plus all descendants before inventory. Create final roots, copy only from sealed stages, apply the same sealed root modes, and compare complete root-inclusive type/mode/link/size/hash inventories with distinct file inodes. Neutral probes must exercise the exact root-inclusive publication semantics.
- Model terminal final-path behavior explicitly: before any final path, safe exact-root cleanup is allowed; after either final path appears, no build/stage/final evidence cleanup is allowed and the failure report must retain which final paths appeared. Neutral tests must cover success, induced pre-final failure, and induced post-first-final failure with retained evidence under non-RC.13 probe names.
- Retain the complete source/tool/history/build requirements from durable state: two shallow fetches, isolated exact Go environments/caches, both Go test/race/vet/module matrices, ordinary/verbose vulnerability evidence, reproducible supported targets, embedded metadata/empty build IDs, payload manifests, canonical history EOF, explicit no-collision success, and complete post-seal comparisons.
- Run two complete validation sequences. Each must reach and pass syntax, cleanup lifetime, exact publication and post-final-retention probes, neutral full-context role simulation, focused static audit, status-64 self refusal, forbidden residue, canonical history EOF, status propagation, v5/v6 preservation, and accepted-byte preservation. Permit at most one repair only between sequences; none after sequence two begins.
- Only if both sequences pass unchanged, seal one v7 root with a self-verifying manifest and create one inert tracked agent-ext20-rc13-builder-revalidation-v7-review.sh for later read-only review. Update TASKS, HANDOFF, STATE, and DECISIONS; keep EXT-20 unchecked. Do not stage, commit, push, publish a builder, or grant construction authority. Stop after this task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local \
	codex exec --dangerously-bypass-approvals-and-sandbox --cd "$ROOT" "$PROMPT"
