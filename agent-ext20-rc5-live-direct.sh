#!/usr/bin/env bash
set -euo pipefail

readonly LIVE_CONFIRMATION="EXT20_LIVE_REAL_CODEX_MODEL_CALLS"
readonly RUN_ROOT="/tmp/revolvr-ext20-rc5.weLZtI/suite"
readonly CANDIDATE_SOURCE="19c1ef4b6a610016487880aa8ad69ec0204bd4f7"
readonly CANDIDATE_SHA256="1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043"
readonly ATTESTATION_COMMIT="109b38cdb309b50c38ab2ef0df33998e92dfd5e6"
readonly AUTHORITY_SHA256="6577bd6c433db64178f5406b62c554370b64f030523c28c0486c6f35fc779b7e"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="c945bb72f24c226215565ac868bb2c255d0a9f75be31819a1dafc030cb032009"
readonly SUITE_SCRIPT_SHA256="bd7fcfb15e91db5361b9c4c91471618ad8ac4fe45c98f028e1b439127b0e66f6"
readonly COLLECTOR_SHA256="2aa507930a12f4040fc8e1e359968b67d2be9cfa6e92aa65d9c8ce0577959cdd"
readonly CODEX_SHA256="134063e133f0b4244fa3b251acf973d4fe4b4aeeacbdc135211bf480f59f1477"

fail() {
	printf 'RC.5 live gate: %s\n' "$*" >&2
	exit 1
}

CHECK_ONLY=false
if [[ "$#" -eq 1 && "$1" == "--check" ]]; then
	CHECK_ONLY=true
elif [[ "$#" -ne 1 || "$1" != "$LIVE_CONFIRMATION" ]]; then
	printf 'usage: %s [--check | %s]\n' "${0##*/}" "$LIVE_CONFIRMATION" >&2
	exit 64
fi
readonly CHECK_ONLY

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

[[ -x scripts/dogfood-external-level1-suite.sh ]] || fail "guarded suite is unavailable"
[[ -x scripts/dogfood-external-level1.sh ]] || fail "collector is unavailable"
[[ -d "$RUN_ROOT" ]] || fail "prepared suite is unavailable"
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || fail "controller repository is not clean"
[[ "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" == "$(git rev-parse HEAD)" ]] || fail "origin/main does not match local HEAD"
[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.5 | awk '{print $1}')" == "$CANDIDATE_SOURCE" ]] || fail "candidate ref changed"
[[ "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.5-attestation | awk '{print $1}')" == "$ATTESTATION_COMMIT" ]] || fail "attestation ref changed"

bash -n scripts/dogfood-external-level1-suite.sh scripts/dogfood-external-level1.sh
[[ "$(sha256sum scripts/dogfood-external-level1-suite.sh | awk '{print $1}')" == "$SUITE_SCRIPT_SHA256" ]] || fail "guarded suite script changed"
[[ "$(sha256sum scripts/dogfood-external-level1.sh | awk '{print $1}')" == "$COLLECTOR_SHA256" ]] || fail "collector changed"
.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61/build-instructions.sh \
	--verify .revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61 >/dev/null
(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared checksum changed"
[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "prepared file content changed"

candidate="$ROOT/.revolvr/release-candidates/level1-v0.1.0-rc.5-19c1ef4b6a61/artifacts/revolvr-v0.1.0-linux-amd64"
codex="$RUN_ROOT/codex-package/node_modules/@openai/codex/bin/codex.js"
[[ "$(sha256sum "$candidate" | awk '{print $1}')" == "$CANDIDATE_SHA256" ]] || fail "candidate binary changed"
[[ "$("$candidate" --version)" == "revolvr 0.1.0" ]] || fail "candidate version changed"
[[ "$(sha256sum "$codex" | awk '{print $1}')" == "$CODEX_SHA256" ]] || fail "Codex binary changed"
[[ "$(node "$codex" --version)" == "codex-cli 0.144.4" ]] || fail "Codex version changed"

for repo in repo-a repo-b; do
	repository="$RUN_ROOT/repositories/$repo"
	sentinel="$RUN_ROOT/sentinels/$repo"
	expected_head="7f1a2135c8dc403a612913195068d2ba1db21690"
	[[ "$repo" == repo-b ]] && expected_head="7d8510cd82281776bb6ebe2436db56da84e7802c"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" branch --show-current)" == main ]] || fail "$repo branch changed"
	[[ "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" rev-parse HEAD)" == "$expected_head" ]] || fail "$repo HEAD changed"
	[[ -z "$(GIT_OPTIONAL_LOCKS=0 git -C "$repository" status --porcelain=v1 --untracked-files=all)" ]] || fail "$repo is dirty"
	[[ -f "$sentinel/value.txt" && ! -L "$sentinel/value.txt" ]] || fail "$repo sentinel changed"
	[[ "$(stat -c '%d:%i:%h' "$sentinel/value.txt")" == "$(stat -c '%d:%i:%h' "$sentinel/value-hardlink.txt")" ]] || fail "$repo sentinel hard link changed"
	[[ "$(stat -c '%h' "$sentinel/value.txt")" == 2 ]] || fail "$repo sentinel link count changed"
	[[ -L "$sentinel/value-link.txt" && "$(readlink "$sentinel/value-link.txt")" == value.txt ]] || fail "$repo sentinel symbolic link changed"
done

[[ "$(find "$RUN_ROOT/repositories" -path '*/.agent/tasks/*.md' -type f | wc -l)" -eq 10 ]] || fail "prepared task count changed"
[[ "$(find "$RUN_ROOT/repositories" -path '*/.revolvr/autonomous/tasks/*/state.json' -type f | wc -l)" -eq 10 ]] || fail "prepared task-state count changed"
[[ "$(grep -l '"lifecycle": "pending"' "$RUN_ROOT"/repositories/*/.revolvr/autonomous/tasks/*/state.json | wc -l)" -eq 10 ]] || fail "prepared task lifecycle changed"
[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"

if [[ "$CHECK_ONLY" == true ]]; then
	printf 'RC.5 live gate: preflight passed; no model call occurred\n'
	exit 0
fi

exec scripts/dogfood-external-level1-suite.sh \
	--live \
	--run-root "$RUN_ROOT" \
	--confirm-live-real-codex "$LIVE_CONFIRMATION"
