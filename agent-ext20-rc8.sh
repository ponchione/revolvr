#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
fi

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'RC.8 construction requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.8 construction requires exact local, fetched, and public main\n' >&2
	exit 1
}
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD || {
	printf 'Published planner-contract remediation is not in main ancestry\n' >&2
	exit 1
}
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] || {
	printf 'Published planner-contract remediation tree changed\n' >&2
	exit 1
}
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum || {
	printf 'Product source changed after the pinned RC.8 source commit\n' >&2
	exit 1
}

[[ ! -e agent-ext20-rc8-local-review.sh && ! -L agent-ext20-rc8-local-review.sh ]] || {
	printf 'RC.8 local-review launcher already exists\n' >&2
	exit 1
}
[[ ! -e .github/workflows/level1-rc8-candidate-attestation.yml && ! -L .github/workflows/level1-rc8-candidate-attestation.yml ]] || {
	printf 'RC.8 workflow collision\n' >&2
	exit 1
}
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.8 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.8-attestation; then
	printf 'Local RC.8 ref collision\n' >&2
	exit 1
fi
REMOTE_RC8_REFS="$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.8 refs/heads/level1-v0.1.0-rc.8-attestation)"
[[ -z "$REMOTE_RC8_REFS" ]] || {
	printf 'Remote RC.8 ref collision\n' >&2
	exit 1
}
[[ -z "$(git tag --list '*rc.8*')" ]] || {
	printf 'Local RC.8 tag collision\n' >&2
	exit 1
}
REMOTE_RC8_TAGS="$(git ls-remote --tags origin '*rc.8*')"
[[ -z "$REMOTE_RC8_TAGS" ]] || {
	printf 'Remote RC.8 tag collision\n' >&2
	exit 1
}
if find .revolvr -maxdepth 2 \( -iname '*rc8*' -o -iname '*rc.8*' \) -print -quit | grep -q .; then
	printf 'Local RC.8 runtime or bundle collision\n' >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.8 local candidate pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not prepare or launch a live dogfood suite, use any live confirmation token, publish a candidate or attestation ref, add an attestation workflow, tag, release, approve external use, grant queue authority, or complete EXT-20.
- Do exactly one bounded task: construct and locally verify a fresh collision-free Level-1 candidate named level1-v0.1.0-rc.8 from exact published planner-contract remediation source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba and tree 2c8ee9f6b4283410547a9f99972e25eac06c9e33.
- Require source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba to be published and reachable from origin/main. The later controller commit containing this launcher is not candidate source and must not enter candidate clones or artifacts. Require the product-source diff from the remediation commit through controller HEAD to be empty for .agent/profiles, cmd, internal, go.mod, and go.sum.
- Treat RC.1 through RC.7 and every historical candidate, ref, workflow, artifact, bundle, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never retry, resume, repair, reconcile, relabel, delete, mutate, derive from, or reuse any of it.
- In particular preserve byte-for-byte the RC.7 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc7-launch-records/ext20-14b2bf40212b-20260725T183415Z-1784416, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a, deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c, and 6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f before and after work. Reverify both terminal checksum manifests read-only before and after.
- Also preserve the independently retired RC.6 suite, launch record, and terminal evidence at content-stream SHA-256 d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259.
- Before construction, fail on any local or remote RC.8 candidate ref, attestation ref, tag, workflow, bundle, verification bundle, build root, suite root, launch record, review launcher, or diagnostic collision. Do not overwrite or reuse partial output; retain any fail-closed diagnostic under a unique suffix and stop.
- Reuse the settled EXT-18 reproducible procedure without changing product source or dependencies: Go 1.22.12 source-floor verification; exact Go 1.26.5 release builds; version 0.1.0; module-readonly mode; disabled CGO; amd64; trimpath; explicit clean VCS metadata; empty Go build ID; and Linux, Darwin, and FreeBSD targets. Build twice in independent fresh non-local clean clones and require byte-identical artifacts.
- Rerun the planner lifecycle prompt/schema/revision regressions, Structured Outputs compatibility guard, production autonomous happy path, strict-fake Codex contract, full Go suite, vet, module verification, and vulnerability scan. These are local evidence only and make no live API-acceptance claim.
- Retain a new immutable RC.8 candidate bundle and separate verification bundle with exact source/tree/tool/build/version/target identities, build instructions, artifact hashes, embedded metadata, tests, vulnerability result, complete sorted regular-file inventories, and inventory hashes. Verify every bundle from its manifest after construction.
- RC.8 local construction grants no remote-CI, attestation, dogfood, live-model, suite-preparation, tag, release, external-use, recovery, or queue authority. It must not create or reuse a suite.
- Keep .agent/TASKS.md unchanged with EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact files, hashes, bundle paths, verification, RC.6/RC.7 preservation evidence, and the next independent local-review gate. Create at most one inert next-pass local-review launcher; do not execute it. Stop after this one task."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
