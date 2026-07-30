#!/usr/bin/env bash
set -euo pipefail

umask 077

ROOT=/home/gernsback/source/revolvr
SELF="$ROOT/agent-ext20-rc13-builder-revalidation-v6-review.sh"
SELF_REL=agent-ext20-rc13-builder-revalidation-v6-review.sh
EVIDENCE_ROOT="$ROOT/.revolvr/prospective-builder-validation-v6.bHfL29"
MANIFEST="$EVIDENCE_ROOT/evidence-manifest.tsv"
SOURCE_COMMIT=a24804bcf2a32ee5434d3686eabad5b72d9f39ba
SOURCE_TREE=2c8ee9f6b4283410547a9f99972e25eac06c9e33
SOURCE_REMOTE=git@github.com:ponchione/revolvr.git
MANIFEST_SHA256=ecdd6f9f5a589038754d1bdb8326d5e19a1ea660eb0bb53a17029fa2aa7734be
STREAM_SHA256=1b0c16fe2d886b60c04ea390b4d364bdfc9431dfde1617c1d34e7da28f8bc56f
INVENTORY_SHA256=0cd5ef032c89cf7be7a6872df665deb8d28481ee3beb80203473909cbdefbf41
V5_ROOT="$ROOT/.revolvr/prospective-builder-validation-v5.tL50Wc"
V5_STREAM_SHA256=6931b60c434205e2ce3130c119aa82750c117f8c947dd7c39f62b5011ddcb7e0
V5_INVENTORY_SHA256=3f7403726cf59e3d02533deeb0c0f975e773adc0423e1f9a470eb30e5cf88cb5

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

content_stream() {
	(
		cd "$1"
		find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum
	) | sha256sum | awk '{print $1}'
}

inventory() {
	(
		cd "$1"
		find . -mindepth 1 -maxdepth 1 -type f -printf '%f\0' | LC_ALL=C sort -z |
			while IFS= read -r -d '' name; do
				printf '%s\t%s\t%s\t%s\t%s\n' "$(stat -c %a "$name")" "$(stat -c %h "$name")" \
					"$(stat -c %s "$name")" "$(sha256sum "$name" | awk '{print $1}')" "$name"
			done
	) | sha256sum | awk '{print $1}'
}

require_absent() {
	[[ ! -e "$1" && ! -L "$1" ]] || fail "RC.13 review collision: $1"
}

verify_manifest() {
	local mode size hash path
	[[ -f "$MANIFEST" && ! -L "$MANIFEST" ]] || fail 'V6 evidence manifest is absent or unsafe'
	[[ "$(sha256sum "$MANIFEST" | awk '{print $1}')" == "$MANIFEST_SHA256" ]] || fail 'V6 evidence manifest hash changed'
	while IFS=$'\t' read -r mode size hash path; do
		[[ -n "$path" && "$path" != */* && "$path" != . && "$path" != .. ]] || fail 'Unsafe V6 manifest path'
		[[ -f "$EVIDENCE_ROOT/$path" && ! -L "$EVIDENCE_ROOT/$path" ]] || fail "V6 evidence entry is absent or unsafe: $path"
		[[ "$(stat -c %a "$EVIDENCE_ROOT/$path")" == "$mode" ]] || fail "V6 evidence mode changed: $path"
		[[ "$(stat -c %s "$EVIDENCE_ROOT/$path")" == "$size" ]] || fail "V6 evidence size changed: $path"
		[[ "$(sha256sum "$EVIDENCE_ROOT/$path" | awk '{print $1}')" == "$hash" ]] || fail "V6 evidence hash changed: $path"
	done <"$MANIFEST"
}

verify_v5() {
	[[ -d "$V5_ROOT" && ! -L "$V5_ROOT" ]] || fail 'Rejected v5 root is absent or unsafe'
	[[ "$(stat -c '%a:%u:%h' "$V5_ROOT")" == "700:$(id -u):2" ]] || fail 'Rejected v5 root identity changed'
	[[ "$(find "$V5_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 11 ]] || fail 'Rejected v5 file count changed'
	[[ "$(find "$V5_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{n += $1} END {print n + 0}')" == 44298 ]] || fail 'Rejected v5 byte count changed'
	[[ "$(content_stream "$V5_ROOT")" == "$V5_STREAM_SHA256" ]] || fail 'Rejected v5 content stream changed'
	[[ "$(inventory "$V5_ROOT")" == "$V5_INVENTORY_SHA256" ]] || fail 'Rejected v5 inventory changed'
}

verify_collision_absence() {
	local path artifact response releases asset
	for path in \
		"$ROOT/.revolvr/release-candidates/build-level1-v0.1.0-rc.13.sh" \
		"$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.13-a24804bcf2a3" \
		"$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.13-a24804bcf2a3-verification" \
		"$ROOT/agent-ext20-rc13.sh" \
		"$ROOT/agent-ext20-rc13-builder-publication.sh" \
		"$ROOT/agent-ext20-rc13-local-review.sh" \
		"$ROOT/.github/workflows/level1-rc13-candidate-attestation.yml" \
		"$ROOT/.revolvr/ext20-rc13"; do
		require_absent "$path"
	done
	[[ -z "$(find "$ROOT/.revolvr/release-candidates" -mindepth 1 -maxdepth 1 \( -name '.level1-v0.1.0-rc.13-preflight.*' -o -name '.level1-v0.1.0-rc.13-stage.*' \) -print -quit)" ]] || fail 'RC.13 stage or preflight collision'
	[[ -z "$(find "$ROOT/.revolvr/release-candidates/diagnostics" -mindepth 1 -maxdepth 1 -name 'level1-v0.1.0-rc.13-*' -print -quit)" ]] || fail 'RC.13 diagnostic collision'
	[[ -z "$(find /tmp -mindepth 1 -maxdepth 1 \( -name 'revolvr-rc13-neutral.*' -o -name 'revolvr-ext20-rc13-build.*' -o -name 'revolvr-v6-history.*' \) -print -quit)" ]] || fail 'RC.13 temporary residue exists'
	[[ -z "$(git -C "$ROOT" for-each-ref --format='%(refname)' 'refs/heads/level1-v0.1.0-rc.13*' 'refs/remotes/origin/level1-v0.1.0-rc.13*' 'refs/tags/*rc.13*')" ]] || fail 'Local RC.13 ref or tag collision'
	[[ -z "$(git -C "$ROOT" ls-remote --heads origin 'refs/heads/level1-v0.1.0-rc.13*')" ]] || fail 'Remote RC.13 ref collision'
	[[ -z "$(git -C "$ROOT" ls-remote --tags origin '*rc.13*')" ]] || fail 'Remote RC.13 tag collision'
	for artifact in level1-v0.1.0-rc.13-a24804bcf2a3 level1-v0.1.0-rc.13-a24804bcf2a3-verification level1-v0.1.0-rc.13-attestation; do
		response="$(curl -fsSL -H 'Accept: application/vnd.github+json' "https://api.github.com/repos/ponchione/revolvr/actions/artifacts?name=$artifact&per_page=1")"
		printf '%s' "$response" | grep -Eq '"total_count"[[:space:]]*:[[:space:]]*0' || fail "Actions artifact collision: $artifact"
	done
	releases="$(curl -fsSL -H 'Accept: application/vnd.github+json' 'https://api.github.com/repos/ponchione/revolvr/releases?per_page=100')"
	for asset in level1-v0.1.0-rc.13-a24804bcf2a3 level1-v0.1.0-rc.13-a24804bcf2a3-verification level1-v0.1.0-rc.13-attestation; do
		if printf '%s' "$releases" | grep -Fq "\"name\": \"$asset\""; then fail "Release asset collision: $asset"; fi
	done
	return 0
}

verify_controller() {
	local head remote_head self_stage self_blob
	[[ "$(realpath -e "$ROOT")" == "$ROOT" ]] || fail 'Controller root resolves unexpectedly'
	[[ "$(git -C "$ROOT" symbolic-ref --short HEAD)" == main ]] || fail 'Controller is not on main'
	[[ "$(git -C "$ROOT" remote get-url origin)" == "$SOURCE_REMOTE" ]] || fail 'Controller origin changed'
	[[ -z "$(git -C "$ROOT" status --porcelain=v1 --untracked-files=all)" ]] || fail 'Review requires a clean published controller'
	head="$(git -C "$ROOT" rev-parse HEAD)"
	remote_head="$(git -C "$ROOT" ls-remote --heads origin refs/heads/main | awk 'NR == 1 {print $1} END {if (NR != 1) exit 1}')"
	[[ "$head" == "$remote_head" ]] || fail 'Local and public main differ'
	[[ "$(git -C "$ROOT" rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || fail 'Source tree changed'
	git -C "$ROOT" diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || fail 'Product source changed'
	self_stage="$(git -C "$ROOT" ls-files --stage -- "$SELF_REL")"
	[[ "$self_stage" == 100755\ *$'\t'"$SELF_REL" ]] || fail 'Review launcher is not tracked executable'
	self_blob="$(git -C "$ROOT" rev-parse "HEAD:$SELF_REL")"
	[[ "$(git -C "$ROOT" hash-object "$SELF")" == "$self_blob" ]] || fail 'Review launcher differs from published main'
}

verify_controller
[[ -d "$EVIDENCE_ROOT" && ! -L "$EVIDENCE_ROOT" ]] || fail 'V6 evidence root is absent or unsafe'
[[ "$(stat -c '%a:%u:%h' "$EVIDENCE_ROOT")" == "500:$(id -u):2" ]] || fail 'V6 evidence root identity changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f | wc -l)" == 13 ]] || fail 'V6 evidence file count changed'
[[ "$(find "$EVIDENCE_ROOT" -mindepth 1 -maxdepth 1 -type f -printf '%s\n' | awk '{n += $1} END {print n + 0}')" == 45820 ]] || fail 'V6 evidence byte count changed'
[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 \( -type l -o ! -type f \) -print -quit)" ]] || fail 'V6 evidence shape changed'
[[ -z "$(find "$EVIDENCE_ROOT" -mindepth 1 -type f \( ! -perm 0444 -o ! -links 1 \) -print -quit)" ]] || fail 'V6 evidence mode or links changed'
verify_manifest
[[ "$(content_stream "$EVIDENCE_ROOT")" == "$STREAM_SHA256" ]] || fail 'V6 evidence content stream changed'
[[ "$(inventory "$EVIDENCE_ROOT")" == "$INVENTORY_SHA256" ]] || fail 'V6 evidence inventory changed'
verify_v5
verify_collision_absence

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'RC.13 v6 read-only review preflight passed; no design mode was executed\n'
	exit 0
fi

PROMPT="Perform one fresh independent read-only review of the sealed prospective RC.13 v6 evidence at $EVIDENCE_ROOT. Read AGENTS.md and durable .agent state first. Do not execute any retained design, validation, builder, launcher, or draft mode. Do not change any file, ref, tag, workflow, artifact, release, or runtime state. Independently verify the manifest, both complete sequences, accepted-byte preservation, canonical EOF policy, trap lifetimes, cleanup proof, status-propagation regression, rejected-v5 preservation, source/tools/history/collision boundary, and the unexecuted construction design's completeness. Return only an evidence-backed accept or reject recommendation. Acceptance grants no builder, construction, candidate, remote, suite, dogfood, release, external-use, or EXT-20 authority."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local \
	codex exec --dangerously-bypass-approvals-and-sandbox --cd "$ROOT" "$PROMPT"
