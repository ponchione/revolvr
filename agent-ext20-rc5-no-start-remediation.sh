#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.5 no-start remediation requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.5 no-start remediation requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 unproven-live-no-start remediation:
- Never use gh. Use raw Git for Git reads and read-only public REST for GitHub evidence.
- Do not commit or push; the controller will independently review and publish later. Do not start any Revolvr live operation or nested Codex/model operation. Never pass the live confirmation token to a command.
- Do exactly one bounded task: preserve and retire the pristine RC.5 suite whose reported live invocation retained no outcome, prepare one collision-free no-model replacement suite, and create a diagnostic-retaining direct live gate for a later separately confirmed pass. Keep EXT-20 unchecked.
- Retired root is /home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite, suite ID ext20-c871c96647e9, authority SHA-256 c4c6cd842aca0861db9c26bc269a6e5d38300d44f37cc44c78aea583564acc7f, plan SHA-256 5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1, and content-stream SHA-256 06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783. Preserve it byte-for-byte and never execute its live path again.
- The operator reported completing the exact live command, but post-command inspection found no running process, operation log, operation manifest, collector manifest, aggregate, task transition, repository change, or content-hash change. Classify this only as an unproven no-start with no model/API acceptance or rejection; do not invent an exit status or retry the old suite.
- Reverify exact RC.5 candidate ref 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, attestation ref 109b38cdb309b50c38ab2ef0df33998e92dfd5e6, workflow SHA-256 9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852, successful runs 29697069305 / 29698647782 / 29698647807, attestation job 88223716039, and artifact 8445792045 with its exact recorded name and digest.
- Reverify both complete sealed RC.5 bundles, shell syntax, suite --static, go test ./..., git diff --check, and exact retired-root preservation before and after all work.
- Create exactly one new collision-free parent beneath /home/gernsback/source/revolvr/.revolvr with an RC.5 no-start-replacement-specific mktemp template. Prepare only <parent>/suite with scripts/dogfood-external-level1-suite.sh --prepare --run-root <parent>/suite --install-codex. Preparation may install exact Codex 0.144.4 but must make no model call.
- Independently inspect exact candidate/Codex identities, prepared checksum and hashes, clean repository heads, ten pending doctor-ready tasks across the exact 11-row plan, sentinels, 32-minute source-writer authority, zero operation/collector manifests, and empty aggregate.
- Rebind agent-ext20-rc5-live-direct.sh to only the new root and its exact hashes/heads. Replace its retired fail-closed line with a diagnostic-retaining live boundary: --check must remain read-only and create nothing; the token-bearing path must create one collision-free launch-record directory outside the suite root under ignored .revolvr runtime state, retain pre-start authority plus stdout/stderr and exit/interruption status, and then execute the guarded suite only once. Any collision, failure, or interruption must fail closed and preserve diagnostics. Do not weaken any existing preflight.
- Add focused shell checks proving check-only creates no diagnostic, missing/wrong confirmation refuses before a diagnostic, an existing launch-record collision refuses, and no live path is exercised by tests. Do not pass the live confirmation token during verification.
- Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with the unproven no-start, exact retired preservation, new prepared authority, files/verification, and the remaining independent review/publication gates. Keep the live pass, tag, release, external-use approval, and EXT-20 completion separate."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
