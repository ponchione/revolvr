#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'Attended-alpha readiness requires a clean repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'Attended-alpha readiness requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the attended developer-alpha readiness pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation, and do not pass any historical live confirmation token. Do not construct or name RC.6, create a candidate ref, tag, release, approve external use, or complete EXT-20.
- Product source authority is exact published remediation commit 010a8939ef6ad889a34590d05ce0326b6df57571. The clean local/origin main launch HEAD is its controller-only child; require its diff from the source commit to contain only .agent/DECISIONS.md, .agent/HANDOFF.md, .agent/STATE.md, and this launcher. Verify that agent-ext20-rc5-live-direct.sh remains permanently retired and preserve all RC.1-through-RC.5 runtime evidence unchanged.
- Do exactly one bounded task: establish the simplest honest attended developer-alpha path for using the fixed harness from current main, without continuing release-candidate ceremony. This is a developer-use path for disposable or recoverable repositories, not a Level-1 release qualification or external-use approval.
- Inspect existing README/docs/scripts and CLI conventions before editing. Prefer a concise documented path that reuses existing commands. Add at most one small build/run helper only if it materially reduces mistakes; do not change autonomous product behavior, add a dependency, or create a parallel architecture.
- The resulting instructions must cover exact build invocation and binary location, revolvr init, configuration validation, doctor/readiness checks, one attended single-task invocation, status/evidence inspection, safe stop/recovery expectations, and the known limitation that EXT-20 real-Codex qualification remains incomplete. Clearly require clean Git state, backups or disposable repositories, finite bounds, attended operation, and review of generated commits before integrating them.
- Exercise the documented path without a real model call using the repository's strict fake Codex and a collision-free disposable Git repository. Run relevant CLI checks such as --help, config check, doctor, status, and the focused attended path that can be proven safely with the strict fake. Do not claim more than the commands prove.
- Run shell syntax for any changed shell file, go test -count=1 ./..., focused CLI checks, and git diff --check. Preserve exact RC.5 suite/evidence/launch-record content-stream hashes 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8 before and after work.
- Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with the exact developer-alpha entry command, files, verification, limitations, and next review gate. Keep EXT-20 unchecked and stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
