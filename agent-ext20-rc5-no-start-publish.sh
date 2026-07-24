#!/usr/bin/env bash
set -euo pipefail

readonly PUBLISH_CONFIRMATION="EXT20_PUBLISH_RC5_NO_START_REPLACEMENT"
readonly EXPECTED_HEAD="239008cfbe819e65c1b2f886a9ba81323e5643e9"
readonly RETIRED_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc5-recovery.yOb0un/suite"
readonly RETIRED_CONTENT_SHA256="06724d26a212ef4743a1f68ccc31dc59d5f2561ff07f4dc5eff6dda4ba7ac783"
readonly RUN_ROOT="/home/gernsback/source/revolvr/.revolvr/ext20-rc5-no-start-replacement.PNKJ20/suite"
readonly AUTHORITY_SHA256="3af0ccb8a7dd7fa5c2205882ce63c03dcb59935cb297fd4804a6a544a4827289"
readonly PLAN_SHA256="5fad4050bd1e49b556819534c6025ddf048ac5325315e6dae59e40b09644eeb1"
readonly CONTENT_SHA256="44a0bacaf8c9ba0c52e3724553652dedda677a4118947e34b44fefa38671cee7"

fail() {
	printf 'RC.5 no-start replacement publication gate: %s\n' "$*" >&2
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
expected_untracked=$'agent-ext20-rc5-no-start-publish.sh\nagent-ext20-rc5-no-start-review.sh\nscripts/check-ext20-rc5-live-direct.sh'
[[ "$(git diff --name-only)" == "$expected_tracked" ]] || fail "tracked replacement scope changed"
[[ "$(git ls-files --others --exclude-standard)" == "$expected_untracked" ]] || fail "untracked replacement scope changed"
git diff --check || fail "replacement diff has whitespace errors"

[[ "$(cd "$RETIRED_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$RETIRED_CONTENT_SHA256" ]] || fail "retired suite changed"
[[ -d "$RUN_ROOT" ]] || fail "replacement suite is unavailable"
(cd "$RUN_ROOT" && sha256sum -c prepared.sha256 >/dev/null) || fail "replacement checksum changed"
[[ "$(sha256sum "$RUN_ROOT/authority.tsv" | awk '{print $1}')" == "$AUTHORITY_SHA256" ]] || fail "replacement authority changed"
[[ "$(sha256sum "$RUN_ROOT/operation-plan.tsv" | awk '{print $1}')" == "$PLAN_SHA256" ]] || fail "operation plan changed"
[[ "$(cd "$RUN_ROOT" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum | sha256sum | awk '{print $1}')" == "$CONTENT_SHA256" ]] || fail "replacement content changed"
[[ "$(find "$RUN_ROOT" -type f -name operation.tsv | wc -l)" -eq 0 ]] || fail "operation evidence already exists"
[[ "$(find "$RUN_ROOT/evidence" -type f -name manifest.tsv | wc -l)" -eq 0 ]] || fail "collector evidence already exists"
[[ "$(find "$RUN_ROOT/aggregate" -mindepth 1 | wc -l)" -eq 0 ]] || fail "aggregate is not empty"
[[ ! -e .revolvr/ext20-rc5-launch-records ]] || fail "launch record already exists"

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 no-start replacement publication:
- The operator supplied exact confirmation $PUBLISH_CONFIRMATION. This grants commit and raw-Git push authority only for the exact independently reviewed replacement scope and its controller record.
- Never use gh. Use raw Git for Git operations and read-only public REST for GitHub evidence.
- Do not start a Revolvr live operation or nested Codex/model operation. Never pass the live confirmation token to a command. Do not tag, release, approve external use, or complete EXT-20.
- Do exactly one bounded task: reverify, publish, and record the reviewed diagnostic-retaining replacement, then run its check-only preflight from clean published main.
- Initial local and origin/main must equal $EXPECTED_HEAD with an empty index. Before durable publication edits, the only tracked diff may be .agent/DECISIONS.md, .agent/HANDOFF.md, .agent/STATE.md, and agent-ext20-rc5-live-direct.sh; the only untracked files may be agent-ext20-rc5-no-start-publish.sh, agent-ext20-rc5-no-start-review.sh, and scripts/check-ext20-rc5-live-direct.sh.
- Retired root $RETIRED_ROOT must remain exact at content-stream SHA-256 $RETIRED_CONTENT_SHA256 and must never be run or modified.
- Replacement root is $RUN_ROOT. Require suite ID ext20-9b7cc650ea0c, authority SHA-256 $AUTHORITY_SHA256, plan SHA-256 $PLAN_SHA256, content-stream SHA-256 $CONTENT_SHA256, repo-a HEAD 41b6aa970b209aaad4291d35b04554464c4ba93c, repo-b HEAD 6ae0cd3ff68995b93b9c22162ca395ed03fca157, exact candidate/Codex identities, ten pending doctor-ready tasks, intact sentinels, 32-minute source-writer authority, zero operation/collector manifests, empty aggregate, and no launch-record directory.
- Reverify exact candidate and attestation refs, workflow hash, successful recorded runs/job/artifact, both complete sealed RC.5 bundles, shell syntax, suite --static, scripts/check-ext20-rc5-live-direct.sh, go test ./..., and git diff --check. Review the diagnostic boundary again without exercising a live path.
- Update durable state minimally to record the passed independent review and this authorized publication task. Stage exactly these seven files: .agent/DECISIONS.md, .agent/HANDOFF.md, .agent/STATE.md, agent-ext20-rc5-live-direct.sh, agent-ext20-rc5-no-start-publish.sh, agent-ext20-rc5-no-start-review.sh, and scripts/check-ext20-rc5-live-direct.sh. Verify staged names and diff, commit with a concise replacement-publication message, and raw-Git push main with an exact $EXPECTED_HEAD lease. Require exact remote readback.
- From that clean published replacement commit run ./agent-ext20-rc5-live-direct.sh --check and require the exact no-model/no-launch-record success. Require no launch-record directory before and after.
- Then record the exact replacement commit and passed clean preflight in only .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md; commit that controller record separately, raw-Git push main with an exact replacement-commit lease, require exact final local/remote readback, and rerun ./agent-ext20-rc5-live-direct.sh --check from the final clean published tree.
- Stop after the final no-model preflight. Report both commit SHAs and the exact next separately confirmation-gated live command, but do not execute it."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
