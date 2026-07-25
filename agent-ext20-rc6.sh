#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.6 construction requires a clean controller repository\n' >&2
	exit 1
}
[[ "$(git rev-parse HEAD)" == "$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')" ]] || {
	printf 'RC.6 construction requires exact local/remote main\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.6 local candidate pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not prepare a live suite, use a historical live confirmation token, publish a ref, add an attestation workflow, tag, release, approve external use, or complete EXT-20.
- The operator explicitly directed progress to continue after publication of the attended developer-alpha path. Do exactly one bounded task: construct and locally verify a fresh collision-free Level-1 candidate named level1-v0.1.0-rc.6 from exact published source commit 73f1f81f1c51d927114f19818a18161d0fcb8541 and tree 7c9753461a08b25915f4f53533d91e57d40a20ca.
- Require source commit 73f1f81f1c51d927114f19818a18161d0fcb8541 to be published and reachable from origin/main. Verify remediation commit 010a8939ef6ad889a34590d05ce0326b6df57571 is its ancestor and that the product-source diff from that remediation through the candidate source is empty for cmd, internal, go.mod, and go.sum. The controller commit containing this launcher is not candidate source and must not enter candidate clones or artifacts.
- Treat RC.1 through RC.5, every historical candidate/ref/workflow/artifact/bundle/suite/operation/launch record, and all retained evidence as immutable rejected history. Never retry, resume, reconcile, relabel, mutate, or reuse them. Preserve the RC.5 suite/evidence/launch-record content-stream SHA-256 values 875398913b77aff293ea672ffd78fbcbab14a76fbaa5e00211c9d44f1cc8932c, 9dfee028b56dbed6d30c0952e77e8f1e8de55751914aff97178530fca7e12c76, and f7c69ba137d2f1c58383df71750fc327fc5e22f6c7cf35350935fc5ba8c26ce8 before and after work. Require agent-ext20-rc5-live-direct.sh to remain permanently failed closed.
- Before construction, fail on any local or remote RC.6 candidate-ref, attestation-ref, tag, workflow, bundle, verification-bundle, build-root, suite-root, or diagnostic collision. Do not overwrite or reuse partial output; retain any fail-closed diagnostic under a unique suffix and stop.
- Reuse the settled EXT-18 reproducible procedure without changing product source or dependencies: Go 1.22.12 source-floor verification; exact Go 1.26.5 release builds; version 0.1.0; module-readonly mode; disabled CGO; amd64; trimpath; explicit clean VCS metadata; empty Go build ID; and Linux, Darwin, and FreeBSD targets. Build twice in independent fresh non-local clean clones and require byte-identical artifacts.
- Rerun the focused planner-role dossier and fallback-receipt regressions, Structured Outputs compatibility guard, lifecycle-routing authority tests, production autonomous happy path, strict-fake Codex contract, full Go suite, vet, module verification, and vulnerability scan. These are local evidence only and make no real API-acceptance claim.
- Retain a new immutable RC.6 candidate bundle and separate verification bundle with exact source/tree/tool/build/version/target identities, build instructions, artifact hashes, embedded metadata, tests, vulnerability result, complete sorted regular-file inventories, and inventory hashes. Verify every bundle from its manifest after construction.
- Keep docs/attended-developer-alpha.md explicitly separate from release qualification. RC.6 local construction grants no remote-CI, attestation, dogfood, live-model, tag, release, external-use, or queue authority.
- Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact files, hashes, bundle paths, verification, preservation evidence, and the next independent review gate. Keep .agent/TASKS.md unchanged with EXT-20 unchecked and stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
