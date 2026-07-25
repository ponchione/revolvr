#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.7 construction requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.7 construction requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.7 local candidate pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not prepare or launch a live suite, use any live confirmation token, publish a ref, add an attestation workflow, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Do exactly one bounded task: construct and locally verify a fresh collision-free Level-1 candidate named level1-v0.1.0-rc.7 from exact published remediation source commit f63cbe3989cb281652cf4eec3f92614fec98294d and tree 43fc099d966cd6c06a74f00272c945fe3ca0a0f9.
- Require remediation source f63cbe3989cb281652cf4eec3f92614fec98294d to be published and reachable from origin/main. The later controller commit containing this launcher is not candidate source and must not enter candidate clones or artifacts. Require the product-source diff from the remediation commit through controller HEAD to be empty for cmd, internal, go.mod, and go.sum.
- Treat RC.1 through RC.6, every historical candidate/ref/workflow/artifact/bundle/suite/operation/launch record, and all retained evidence as immutable rejected or failed history. Never retry, resume, repair, reconcile, relabel, delete, mutate, or reuse any of it. In particular preserve byte-for-byte the RC.6 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc6-launch-records/ext20-7b4a5932090f-20260725T115426Z-657365, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc6.LOLauh/suite/evidence/repo-a/01-successful-source-change-1. Verify the retained terminal manifest before and after work and compare pre/post content-stream hashes for all three protected roots.
- Before construction, fail on any local or remote RC.7 candidate ref, attestation ref, tag, workflow, bundle, verification bundle, build root, suite root, launch record, or diagnostic collision. Do not overwrite or reuse partial output; retain any fail-closed diagnostic under a unique suffix and stop.
- Reuse the settled EXT-18 reproducible procedure without changing product source or dependencies: Go 1.22.12 source-floor verification; exact Go 1.26.5 release builds; version 0.1.0; module-readonly mode; disabled CGO; amd64; trimpath; explicit clean VCS metadata; empty Go build ID; and Linux, Darwin, and FreeBSD targets. Build twice in independent fresh non-local clean clones and require byte-identical artifacts.
- Rerun the focused normalized-planner-profile regression and adjacent prompt/cycle/audit/planner tests, Structured Outputs compatibility guard, lifecycle-routing authority tests, production autonomous happy path, strict-fake Codex contract, full Go suite, vet, module verification, and vulnerability scan. These are local evidence only and make no real API-acceptance claim.
- Retain a new immutable RC.7 candidate bundle and separate verification bundle with exact source/tree/tool/build/version/target identities, build instructions, artifact hashes, embedded metadata, tests, vulnerability result, complete sorted regular-file inventories, and inventory hashes. Verify every bundle from its manifest after construction.
- RC.7 local construction grants no remote-CI, attestation, dogfood, live-model, suite-preparation, tag, release, external-use, or queue authority. It must not create or reuse a suite.
- Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact files, hashes, bundle paths, verification, RC.6 preservation evidence, and the next independent review gate. Keep .agent/TASKS.md unchanged with EXT-20 unchecked and stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
