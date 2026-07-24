#!/usr/bin/env bash
set -euo pipefail

readonly PUBLISH_CONFIRMATION="EXT20_PUBLISH_RC5_RECOVERY"
readonly EXPECTED_HEAD="49dafe186f4c081c49483c46c6487914b1fd9c00"
readonly RUN_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite"
readonly AUTHORITY_SHA256="c4c6cd842aca0861db9c26bc269a6e5d38300d44f37cc44c78aea583564acc7f"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783"

fail() {
	printf 'RC.5 recovery publication gate: %s\n' "$*" >&2
	exit 1
}

if [[ "$#" -ne 1 || "$1" != "$PUBLISH_CONFIRMATION" ]]; then
	printf 'usage: %s %s\n' "${0##*/}" "$PUBLISH_CONFIRMATION" >&2
	exit 64
fi

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

[[ -f .agent/LOOP_PROMPT.md ]] || fail "missing .agent/LOOP_PROMPT.md"
[[ "$(git rev-parse HEAD)" == "$EXPECTED_HEAD" ]] || fail "local main changed"
[[ "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" == "$EXPECTED_HEAD" ]] || fail "origin/main changed"
[[ -z "$(git diff --cached --name-only)" ]] || fail "index is not empty"

expected_tracked=$'.agent/DECISIONS.md\n.agent/HANDOFF.md\n.agent/STATE.md\nagent-ext20-rc5-live-direct.sh'
expected_untracked=$'agent-ext20-rc5-recovery-publish.sh\nagent-ext20-rc5-recovery-review.sh'
[[ "$(git diff --name-only)" == "$expected_tracked" ]] || fail "tracked recovery scope changed"
[[ "$(git ls-files --others --exclude-standard)" == "$expected_untracked" ]] || fail "untracked recovery scope changed"
git diff --check || fail "recovery diff has whitespace errors"

[[ -d "$RUN_ROOT" ]] || fail "prepared suite is unavailable"
(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "prepared checksum changed"
[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "prepared authority changed"
[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "prepared content changed"
[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 recovery publication gate:
- The operator supplied exact confirmation $PUBLISH_CONFIRMATION to this launcher. This grants commit and raw-Git push authority only for the exact reviewed recovery scope below.
- Never use gh. Use raw Git for all Git operations and read-only public REST for GitHub evidence.
- Do not start a Revolvr live operation or any nested Codex/model operation. Never pass the live-suite confirmation token to a command. Do not tag, release, or approve external use.
- Do exactly one bounded task: reverify, publish, and record the independently reviewed RC.5 persistent prepared-suite recovery, then run its no-model direct preflight from clean published main. Keep EXT-20 unchecked.
- Initial local and origin/main authority must both be $EXPECTED_HEAD with an empty index. Before edits, the only tracked diff must be .agent/DECISIONS.md, .agent/HANDOFF.md, .agent/STATE.md, and agent-ext20-rc5-live-direct.sh; the only untracked files must be agent-ext20-rc5-recovery-publish.sh and agent-ext20-rc5-recovery-review.sh.
- Review and publish exactly those six files. No candidate source, workflow, bundle, suite script, collector, Go source, dependency, task status, tag, or candidate/attestation ref may change.
- Reverify candidate ref 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, attestation ref 109b38cdb309b50c38ab2ef0df33998e92dfd5e6, workflow SHA-256 9c650a1fbbad1354cf7e991018bb505aba59698c8fec4bc828260c512b069852, successful runs 29697069305 / 29698647782 / 29698647807, attestation job 88223716039, and artifact 8445792045 with its exact recorded name and digest.
- Reverify both complete sealed RC.5 bundles, shell syntax/static mode, go test ./..., and git diff --check.
- Exact replacement root is $RUN_ROOT. Require suite ID ext20-c871c96647e9, authority SHA-256 $AUTHORITY_SHA256, plan SHA-256 $PLAN_SHA256, content-stream SHA-256 $CONTENT_SHA256, exact candidate/Codex identities, repository heads, ten pending doctor-ready tasks, sentinels, source-lock authority, zero operation/collector manifests, and empty aggregate.
- Update durable state minimally to say the independent recovery review passed and publication is the authorized bounded task. Stage only the exact six-file recovery scope, verify the staged names and diff, commit with a concise RC.5 recovery-publication message, and raw-Git push main with an exact $EXPECTED_HEAD lease. Require exact remote readback of the new commit.
- From that clean published recovery commit, run ./agent-ext20-rc5-live-direct.sh --check and require it to pass without a model call.
- Then record the exact recovery commit and passed preflight in only .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md; commit that controller record separately, raw-Git push main with an exact recovery-commit lease, require exact final local/remote readback, and rerun ./agent-ext20-rc5-live-direct.sh --check from the final clean published tree.
- Stop after the final no-model preflight. Report both commit SHAs and the exact next separately confirmation-gated live command, but do not execute it."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
