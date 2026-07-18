#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.2 no-model suite preparation:
- Never use gh. Git operations must use raw git.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass. Do not pass --live or the live confirmation value.
- Read the newest RC.2 local, exact-candidate remote-CI, and remote-attestation state before acting.
- Preserve RC.1, both retired failed live roots, every RC.1/RC.2 ref, bundle, workflow, hash, artifact, and retained diagnostic unchanged.
- Do exactly one bounded task: update the guarded external Level-1 suite from rejected RC.1 to immutable RC.2 authority, verify it without model calls, and prepare one new collision-free no-model suite root for the separately confirmed live pass.
- In scripts/dogfood-external-level1-suite.sh bind exact source eeaaf50b52fd82038c6d58c7947d63ddf26eb0ec, Linux SHA-256 06c1258a947def8c53e03bfd79944bb002351358fc8dfecd35682ab7532b5010, and bundle .revolvr/release-candidates/level1-v0.1.0-rc.2-eeaaf50b52fd. Preserve release output revolvr 0.1.0 and exact Codex 0.144.4 authority. Make no unrelated plan, schema, scenario, threshold, or confirmation changes.
- Verify shell syntax, the candidate bundle, suite --static, and collector fixture/manifest behavior. Run relevant repository checks for the tracked shell-only change.
- Create a new parent with mktemp -d using an RC.2-specific /tmp/revolvr-ext20-rc2.XXXXXX template, then run --prepare --run-root <parent>/suite --install-codex. Preparation may install the exact package but must start no model.
- Independently inspect the prepared authority, candidate and Codex identities, zero operation manifests, empty aggregate, both clean disposable repositories, exact task readiness, and refusal of live mode without confirmation without mutating the prepared suite.
- Retain and report the exact prepared suite path and the exact confirmation-gated live command, but do not execute it.
- Keep EXT-20 unchecked. Update durable state with files, verification, prepared-root evidence, and the remaining separately confirmed live step."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
