#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 authorization status requires a clean controller repository\n' >&2
	exit 1
}

HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.7 authorization status requires exact local, fetched, and public main\n' >&2
	exit 1
}
[[ "$(git show -s --format='%H %T %P' 3c621ddfb8d11905150498c9cd3cc173323ac816)" == "3c621ddfb8d11905150498c9cd3cc173323ac816 d5a313ed6a86d73bedb9ad6b1997f2e8e32e5b2e 133fb6ddb2f94873d572dd72b9c91ef337cb87b3" ]] || {
	printf 'RC.7 authorization-boundary publication changed\n' >&2
	exit 1
}
git merge-base --is-ancestor 3c621ddfb8d11905150498c9cd3cc173323ac816 HEAD || {
	printf 'RC.7 authorization boundary is not in main ancestry\n' >&2
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
	printf 'RC.7 launch-record root already exists\n' >&2
	exit 1
}

./agent-ext20-rc7-live-direct.sh --check

printf '%s\n' \
	'RC.7 technical admission remains ready, but live authorization is absent.' \
	'No live command, confirmation value, launch record, Revolvr process, or model call was activated.' \
	'Proceeding requires a new explicit human instruction authorizing exactly one RC.7 live suite launch:' \
	'eleven real-Codex operations over ten tasks in two disposable repositories, with approval/sandbox bypass,' \
	'inherited host environment and network access, real model cost/time, deliberate cancellation, commits,' \
	'and immutable retention of launch, runtime, model, ledger, receipt, and partial failure/interruption evidence.'
