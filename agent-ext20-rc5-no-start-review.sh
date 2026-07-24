#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 no-start replacement review:
- Never use gh. Use raw Git for Git reads and read-only public REST for GitHub evidence.
- Do not commit or push. Do not start any Revolvr live operation or nested Codex/model operation. Never pass the live confirmation token to a command.
- Do exactly one bounded task: independently review the current uncommitted persistent replacement suite and diagnostic-retaining direct live gate, then report whether the exact scope is safe for later separately authorized publication. Keep EXT-20 unchecked.
- Initial local and origin/main authority must remain exact at 239008cfbe819e65c1b2f886a9ba81323e5643e9 with an empty index. The only tracked changes may be .agent/DECISIONS.md, .agent/HANDOFF.md, .agent/STATE.md, and agent-ext20-rc5-live-direct.sh; the only untracked files may be agent-ext20-rc5-no-start-review.sh and scripts/check-ext20-rc5-live-direct.sh.
- Retired root /home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite must remain byte-for-byte exact at content-stream SHA-256 06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783 with no operation/collector manifests or aggregate. Never run or modify it.
- Replacement root is /home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite. Require suite ID ext20-9b7cc650ea0c, authority SHA-256 3af0ccb8a7dd7fa5c2205882ce63c03dcb59935cb297fd4804a6a544a4827289, plan SHA-256 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1, content-stream SHA-256 44a0bacaf8c9ba0c52e3724553652dedda677a4118947e34b44fefa38671cee7, repo-a HEAD 41b6aa970b209aaad4291d35b04554464c4ba93c, and repo-b HEAD 6ae0cd3ff68995b93b9c22162ca395ed03fca157.
- Require exact candidate/Codex identities, clean repositories, ten pending doctor-ready tasks across the exact 11-row plan, sentinels and links, 32-minute source-writer authority, zero operation/collector manifests, empty aggregate, and no launch-record directory.
- Reverify candidate ref 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, attestation ref 109b38cdb309b50c38ab2ef0df33998e92dfd5e6, workflow SHA-256 9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852, successful runs 29697069305 / 29698647782 / 29698647807, attestation job 88223716039, artifact 8445792045, and both complete sealed RC.5 bundles.
- Review agent-ext20-rc5-live-direct.sh and scripts/check-ext20-rc5-live-direct.sh line by line. Require --check and missing/wrong confirmation to create no record; exact clean-main and suite preflight before record reservation; ignored collision-free record authority outside the suite; pre-start authority plus stdout/stderr/status before child admission; process-group signal forwarding; atomic retained terminal/interruption status; and refusal on collision or nonzero suite exit. Do not weaken any gate.
- Run shell syntax, suite --static, scripts/check-ext20-rc5-live-direct.sh, go test ./..., and git diff --check. Confirm all checks leave both roots and launch-record authority unchanged and never exercise a live path.
- If review passes, make no file change and report the exact accepted six-file scope plus the remaining separately authorized publication gate. If a blocking defect exists, make only the smallest scope-local repair, rerun relevant checks, update durable state, and stop. No live call, tag, release, external-use approval, or EXT-20 completion is authorized."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
