#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.6 remote artifact-attestation gate:
- Never use gh. Use raw git for Git operations and read-only public GitHub REST responses for Actions evidence.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.6-attestation at exact reviewed workflow commit 226276f151ae389d06c0118a931596712fbc7cc1. Do not commit or push main; the controller will independently verify and publish durable-state changes afterward.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.6 local candidate, exact-candidate remote-CI, local attestation-workflow, and controller-review state before acting. Preserve RC.1 through RC.5 as immutable rejected history. Never retry, reuse, recreate, reconcile, relabel, or mutate any historical suite, operation, evidence, workflow, ref, bundle, artifact, hash, run, diagnostic, or sentinel. RC.5's retired launchers must remain failed closed.
- Require a clean worktree and index with local main exactly equal to freshly fetched origin/main. Require reviewed workflow commit 226276f151ae389d06c0118a931596712fbc7cc1, tree eac5c8d4696cec6f5383d9fd19f3d482045028c8, parent f282f263d817ff4ab32e04fe86e3c42612d18ca9, and workflow SHA-256 708f2f35d2c9a71f803fc136f33a5bd4bbd65624de50af84caa98cd3a3395fdf to be published ancestors of origin/main.
- Before publication, independently reverify the complete RC.6 candidate and verification bundles at .revolvr/release-candidates/level1-v0.1.0-rc.6-73f1f81f1c51 and its -verification sibling. Require candidate inventory/seal 30353ecc7c828952d3afbff126223a5ff7c5cc3fd30d546774d850001a316ac1 / d1707466e4f3a8bf562fcbb4a5d32392df988e423aaadad75fca5ff0f5c05e88, verification inventory/seal 9ee4be200b5d71275dce0c5cb4fdbeb0428a00af17d30c9ae4ef426dd0daadcf / f70f9cd944456c4b9e939973a297cd7f7169fb42790c86461d038cff2b7a822f, source commit 73f1f81f1c51d927114f19818a18161d0fcb8541, source tree 7c9753461a08b25915f4f53533d91e57d40a20ca, build-instructions hash 94d291ec80db7427bddc1db57cac147c5d061ca3dbdbdd038259e1da3505a906, and Linux/Darwin/FreeBSD hashes f3800b164c83728869a949d7b2240a1558ce2649668c0a05480cf8798304c22d, 596a17a21b5509cfa868762e8675a66251136cf483cdbb40cc0fa51a28f284f7, and 60c4052e2ff717b5f9d09db73d00073c4d182ed2b584328eaae4bd6d7f2b4344.
- Reverify exact candidate ref refs/heads/level1-v0.1.0-rc.6 at 73f1f81f1c51d927114f19818a18161d0fcb8541 and successful candidate CI run 30153462797 with exactly its ten recorded successful jobs. Reverify the reviewed workflow structure and extracted embedded-shell syntax, the retained 29-file local result /tmp/revolvr-ext20-rc6-attestation.9dtfzU, and the independent controller replay /tmp/revolvr-ext20-rc6-attestation-review.OkDN5p. Require .agent/TASKS.md SHA-256 77c8b220a8ed49cdd5c937f295a9add45c9b1d20fc977182298ec9cbd5073917 with EXT-20 unchecked.
- Immediately before publication, prove refs/heads/level1-v0.1.0-rc.6-attestation is absent locally and remotely, all *rc.6* tags are absent, the only existing RC.6 ref is the exact candidate ref, and the public artifact query has zero artifacts named level1-v0.1.0-rc.6-attestation. Fail closed on any collision or identity drift.
- Create only the attestation ref with raw git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.6-attestation: origin 226276f151ae389d06c0118a931596712fbc7cc1:refs/heads/level1-v0.1.0-rc.6-attestation. Read it back and require the exact SHA. Never force-update, delete, or move any existing ref.
- Locate the new push-triggered GitHub Actions run named Level 1 RC.6 candidate attestation through the public REST API. Require event push, head_branch level1-v0.1.0-rc.6-attestation, head_sha 226276f151ae389d06c0118a931596712fbc7cc1, exact workflow path, run attempt 1, and unambiguous identity. Poll with a finite bound until completion and require success.
- Require exactly one successful job named Rebuild and attest Level 1 RC.6 candidate at the exact head SHA. Require exactly one unexpired run artifact, named level1-v0.1.0-rc.6-attestation, with a nonempty ID, positive size, and sha256 digest. If an already configured read-only token permits archive download, verify the 29-file archive and all retained checksum, manifest, metadata, build-ID, version, and reproducibility authority; otherwise record the unauthenticated archive endpoint result and make no controller-side archive-byte claim.
- Also locate the companion ci.yml push run at the exact attestation head and require its complete ten-job matrix to finish successfully. Record exact run, job, artifact IDs, URLs, timestamps, size, digest, and conclusions in durable state.
- Stop after remote ref/run/job/artifact and companion-CI evidence. Do not change candidate source/ref, prepare a suite, start model work, tag a release, approve external use, grant queue authority, or mark EXT-20 complete.
- Update .agent/STATE.md, .agent/DECISIONS.md, and .agent/HANDOFF.md with exact remote readback, complete evidence, preservation checks, and the next separately bounded no-model RC.6 suite-preparation step. This pass must leave .agent/TASKS.md unchanged."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
