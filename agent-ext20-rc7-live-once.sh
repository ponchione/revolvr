#!/usr/bin/env bash
set -euo pipefail

readonly EXECUTE_CONFIRMATION="--execute-authorized-rc7-live-suite-once"
ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ "$#" -ne 1 || "$1" != "$EXECUTE_CONFIRMATION" ]]; then
	printf 'usage: %s %s\n' "${0##*/}" "$EXECUTE_CONFIRMATION" >&2
	exit 64
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 authorized live launch requires a clean controller repository\n' >&2
	exit 1
}
git fetch --no-tags origin main
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.7 authorized live launch requires exact local, fetched, and public main\n' >&2
	exit 1
}
[[ "$(git show -s --format='%H %T %P' 92d9c38ec9e4bbc01b0b5cf2c75ff3819f36658d)" == "92d9c38ec9e4bbc01b0b5cf2c75ff3819f36658d 24e43c5eaf002c272aa27bca578345415427bc7e 6a786e3ce6fa436ff66aa75fd663e20fafd425a1" ]] || {
	printf 'RC.7 human-authorization publication changed\n' >&2
	exit 1
}
git merge-base --is-ancestor 92d9c38ec9e4bbc01b0b5cf2c75ff3819f36658d HEAD || {
	printf 'RC.7 human authorization is not in main ancestry\n' >&2
	exit 1
}
[[ "$(sha256sum agent-ext20-rc7-live-direct.sh | awk '{print $1}')" == 9cfe73e11f69a4e9ad138de6749da04ea5f7bd3d0508ef6858279d557125df45 ]] || {
	printf 'RC.7 direct launcher changed\n' >&2
	exit 1
}
[[ "$(sha256sum .agent/TASKS.md | awk '{print $1}')" == 33d1ead280d00a0246528bf091e526c5010c8e40ebe41cbe35f37e50d652d448 ]] || {
	printf 'RC.7 task backlog changed\n' >&2
	exit 1
}
[[ ! -e .revolvr/ext20-rc7-launch-records && ! -L .revolvr/ext20-rc7-launch-records ]] || {
	printf 'RC.7 launch-record root already exists; one-time authority is exhausted\n' >&2
	exit 1
}

source "$ROOT/agent-ext20-rc7-live-direct.sh"
exec "$ROOT/agent-ext20-rc7-live-direct.sh" "$LIVE_CONFIRMATION"
