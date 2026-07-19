#!/usr/bin/env bash
set -euo pipefail

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"
readonly RUN_ROOT="/tmp/revolvr-ext20-rc4.DGg1pW/suite"
readonly CANDIDATE_SOURCE="2546913e38ec273f64417dece2f91df78fd42fc2"
readonly CANDIDATE_SHA256="98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe"
readonly ATTESTATION_COMMIT="52c2db07a86677e67921bcbfbcbdf26397b47615"
readonly AUTHORITY_SHA256="4f9b653c9e62e5fc5932b219952bbe61fccd79d331ac2bd7fcf2c570035eacb7"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="5e988363634a5aa4739c3b4bfccce865d2cf6e2c7ddb634aaa4eb25750641305"
readonly CODEX_SHA256="134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477"

fail() {
	printf 'RC.4 live gate: %s\n' "$*" >&2
	exit 1
}

if [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
	printf 'usage: %s %s\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
	exit 64
fi

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

[[ -x scripts/dogfood-external-level1-suite.sh ]] || fail "guarded suite is unavailable"
[[ -d "$RUN_ROOT" ]] || fail "prepared suite is unavailable"
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail "controller repository is not clean"
[[ "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" == "$(git rev-parse HEAD)" ]] || fail "origin/main does not match local HEAD"
[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.4 | awk '{print $1}')" == "$CANDIDATE_SOURCE" ]] || fail "candidate ref changed"
[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.4-attestation | awk '{print $1}')" == "$ATTESTATION_COMMIT" ]] || fail "attestation ref changed"

bash -n scripts/dogfood-external-level1-suite.sh scripts/dogfood-external-level1.sh
.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/build-instructions.sh \
	--verify .revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec >/dev/null
(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared checksum changed"
[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "prepared file content changed"

candidate="$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.4-2546913e38ec/artifacts/revolvr-v0.1.0-linux-amd64"
codex="$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js"
[[ "$(sha256sum "$candidate" | awk '{print $1}')" == "$CANDIDATE_SHA256" ]] || fail "candidate binary changed"
[[ "$("$candidate" --version)" == "revolvr 0.1.0" ]] || fail "candidate version changed"
[[ "$(sha256sum "$codex" | awk '{print $1}')" == "$CODEX_SHA256" ]] || fail "Codex binary changed"
[[ "$(node "$codex" --version)" == "codex-cli 0.144.4" ]] || fail "Codex version changed"

for repo in repo-a repo-b; do
	repository="$RUN_ROOT/repositories/$repo"
	sentinel="$RUN_ROOT/sentinels/$repo"
	expected_head="a75d4f059721ec7c9320650bd49d6d4cef9526cf"
	[[ "$repo" == repo-b ]] && expected_head="11eb46ae242cf2a3cb5ce32cf94e0df3aab2ab0b"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" branch --show-current)" == main ]] || fail "$repo branch changed"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" rev-parse HEAD)" == "$expected_head" ]] || fail "$repo HEAD changed"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" status --porcelain=v1 --untracked-files=all)" ]] || fail "$repo is dirty"
	[[ -f "$sentinel/value.txt" && ! -L "$sentinel/value.txt" ]] || fail "$repo sentinel changed"
	[[ "$(stat -c '%d:%i:%h' "$sentinel/value.txt")" == "$(stat -c '%d:%i:%h' "$sentinel/value-hardlink.txt")" ]] || fail "$repo sentinel hard link changed"
	[[ "$(stat -c '%h' "$sentinel/value.txt")" == 2 ]] || fail "$repo sentinel link count changed"
	[[ -L "$sentinel/value-link.txt" && "$(readlink "$sentinel/value-link.txt")" == value.txt ]] || fail "$repo sentinel symbolic link changed"
done

[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"

exec scripts/dogfood-external-level1-suite.sh \
	--live \
	--run-root "$RUN_ROOT" \
	--confirm-live-real-codex "$LIVE_CONFIRMATION"
