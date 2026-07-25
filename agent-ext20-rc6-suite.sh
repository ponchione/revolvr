#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.6 no-model suite preparation:
- Never use gh. Git operations must use raw git; GitHub evidence must use read-only public REST.
- Do not commit or push; the controller will independently verify, commit, and push.
- Do not start live or nested Codex/model operations in this pass. Do not pass --live or the live confirmation value.
- Read the newest RC.6 local-candidate, exact-candidate remote-CI, remote artifact-attestation, and controller-publication state before acting. Require published controller record c2c8c41e51bb4ebb6c65c93778b88ebad4a4eaa7 to be an ancestor of clean local and remote main.
- Preserve RC.1 through RC.5 as immutable rejected history. Never retry, reuse, recreate, reconcile, relabel, or mutate any historical suite, operation, evidence, workflow, ref, bundle, artifact, hash, run, diagnostic, or sentinel. In particular, preserve RC.5 suite .revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite, operation ext20-9b7cc650ea0c-01, evidence evidence/repo-a/01-successful-source-change-1, and launch record .revolvr/ext20-rc5-launch-records/ext20-9b7cc650ea0c-20260724T230449Z-468858 byte-for-byte at content-stream hashes 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8. Its retired launcher check must pass without mutation.
- Before editing, reverify complete RC.6 candidate and verification bundles and exact inventory/seals 30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1 / d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88 and 9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf / f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f.
- Reverify exact candidate ref refs/heads/level1-v0.1.0-rc.6 at 73f1f81f1c51d927114f19818a18161d0fcb8541; exact attestation ref refs/heads/level1-v0.1.0-rc.6-attestation and workflow commit 226276f151ae389d06c0118a931596712fbc7cc1; workflow SHA-256 708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf; successful candidate CI run 30153462797; successful attestation run 30155142491 and job 89671812731; artifact 8618790256 named level1-v0.1.0-rc.6-attestation with digest sha256:1e8d9b6161efb8ff04000eaba24e202eeddff625e443fc15728cf98cbaba95fa; and successful companion ten-job CI run 30155142490.
- Do exactly one bounded task: update the guarded external Level-1 suite from immutable rejected RC.5 authority to immutable RC.6 authority, verify it without model calls, and prepare one new collision-free no-model suite root for a separately reviewed and confirmed live pass.
- In scripts/dogfood-external-level1-suite.sh change only the exact candidate authority constants: source 73f1f81f1c51d927114f19818a18161d0fcb8541, Linux SHA-256 f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d, and bundle .revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51. Preserve release output revolvr 0.1.0, exact Codex 0.144.4 authority, model, reasoning effort, plan, schemas, scenarios, thresholds, configuration, live confirmation, and collector behavior. Make no unrelated change.
- Verify shell syntax, the complete sealed RC.6 candidate and verification bundles, suite --static, and collector fixture/manifest behavior. Run go test -count=1 ./... and git diff --check for the tracked shell-only change.
- Create exactly one persistent parent beneath ignored repository runtime state with mkdir -p \"$ROOT/.revolvr\" and mktemp -d \"$ROOT/.revolvr/ext20-rc6.XXXXXX\", then run --prepare --run-root <parent>/suite --install-codex. Preparation may install the exact package but must start no model. Do not use /tmp for this suite authority.
- Independently inspect the prepared authority, candidate and Codex identities, effective 32-minute source-lock authority, zero operation and collector manifests, empty aggregate, both clean disposable repositories, exact task readiness, intact sentinels, and refusal of live mode without exact confirmation without mutating the prepared suite. Require no RC.6 launch record.
- Retain and report the exact prepared suite path, suite ID, authority/plan/content hashes, repository heads, and exact confirmation-gated live command, but do not execute it and do not add a live launcher in this pass.
- Reverify all RC.6 remote authority and all three RC.5 immutable content streams after preparation. Keep EXT-20 unchecked and .agent/TASKS.md byte-for-byte unchanged. Update durable state, decisions, and handoff with files, verification, persistent prepared-root evidence, and the remaining independent review/publication plus separately confirmed live steps. This pass grants no live-model call, tag, release, external-use approval, queue authority, or EXT-20 completion."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
