#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 prepared-suite recovery review:
- Never use gh. Git operations must use raw Git; GitHub evidence must use read-only public REST.
- Do not commit or push. Do not start Revolvr live operations or any nested Codex/model operation. Do not pass the live confirmation token to any command.
- Do exactly one bounded task: independently review the current uncommitted RC.5 volatile-root recovery and report whether it is safe for a later explicitly authorized commit/push.
- The published launcher correctly failed closed before any model call because /tmp/revolvr-ext20-rc5.weLZtI/suite was externally removed. Older RC.1-through-RC.4 /tmp roots are also absent; do not recreate or retry them, and do not claim their filesystem copies remain retained.
- Require the tracked diff to be limited to agent-ext20-rc5-live-direct.sh, agent-ext20-rc5-recovery-review.sh, .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md. Preserve all candidate source, workflow, bundle, plan, collector, Go source, dependency, tag, and remote-ref authority.
- Reverify exact candidate ref 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, attestation ref 109b38cdb309b50c38ab2ef0df33998e92dfd5e6, workflow SHA-256 9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852, successful runs 29697069305 / 29698647782 / 29698647807, attestation job 88223716039, and artifact 8445792045 with its recorded exact name and digest.
- Reverify both complete sealed RC.5 bundles, shell syntax/static mode, go test -count=1 ./..., and git diff --check.
- Exact replacement root is /home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite. Require suite ID ext20-c871c96647e9, authority SHA-256 c4c6cd842aca0861db9c26bc269a6e5d38300d44f37cc44c78aea583564acc7f, plan SHA-256 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1, and path-bearing content-stream SHA-256 06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783.
- Require exact candidate and Codex hashes/versions, repo-a HEAD 7ff28f8e19c4d3b57ea0e565b764db35a5207599, repo-b HEAD 697def8f11122af055c69726277e88dd86e63a6c, ten pending doctor-ready tasks across the exact 11-row plan, intact sentinels, 32-minute source-writer authority, zero operation/collector manifests, and an empty aggregate.
- Review agent-ext20-rc5-live-direct.sh line by line. Its --check is expected to stop only at the dirty-controller guard until publication; independently verify all later predicates without weakening that guard.
- If review passes, make no file change and report the exact reviewed diff plus the remaining explicit commit/push authorization gate. If a blocking defect is found, make only the smallest recovery-scope repair, rerun relevant verification, update durable state, and stop. Keep EXT-20 unchecked."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
