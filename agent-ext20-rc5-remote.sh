#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.5 candidate-ref and remote-CI gate:
- Never use gh. Use raw git for Git operations and read-only GitHub REST responses for Actions evidence.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.5 at exact source commit 19c1ef4b6a610016487880aa8ad69ec0204bd4f7. Do not commit or push main; the controller will independently verify and publish durable-state changes afterward.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.5 local-candidate and independent-controller verification state before acting. Preserve RC.1, RC.2, RC.3, and RC.4 as immutable rejected history, including terminal RC.4 suite /tmp/revolvr-ext20-rc4.DGg1pW/suite and operation ext20-2bd21aea4f72-01. Preserve every historical ref, workflow, bundle, artifact, hash, suite, operation, run, diagnostic, and sentinel unchanged.
- Independently reverify the sealed RC.5 candidate and verification bundles before publication. Require candidate ID level1-v0.1.0-rc.5, release 0.1.0, source commit 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, source tree 2fb39c93694e72d986e7a8a849a542fc1bf1728d, candidate inventory ba718e4bef733a370cff72570b96e3c2f0db0af4b9ad8eedc77db2c965ca0b88, candidate seal 8bf947efd3d7f6467d500f88278913c0bcf5dd922331e558d483176a777584ab, verification inventory e57353d8b929758b44d234458dfb2c3b4bae0cf347eccc206ba9424312a0e366, verification seal 2cded484b787daa903ebf457f3f96bb9520af122bd48114300d78e543f39ccb8, build-instructions hash 69e0e533258b88b810db465935e66c49fd4e294fb745fc13998115dc8951dcb8, and Linux/Darwin/FreeBSD hashes 1cad902dff8d31e36af0a3d2aa38e71280daf214af79d9b7c748516bb5e16043, a0ba1e05f76d92c1d20577c897a37bc2b4a3252a4e0fb10ef9d736f25b07645d, and f9b6da20be9497c5eb772f7b40945fceedc064ecb6e081809c9510d71462e2d6.
- Do exactly one bounded task: publish the exact RC.5 candidate ref and require the complete push-triggered EXT-15 CI matrix to finish successfully on that exact source SHA.
- Immediately before publication, fetch origin without tags, prove the exact source is published and reachable from origin/main, prove refs/heads/level1-v0.1.0-rc.5 is absent locally and remotely, and prove no RC.5 tag or attestation-ref collision exists. Fail closed on any collision or identity drift.
- Create only the candidate ref with raw git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.5: origin 19c1ef4b6a610016487880aa8ad69ec0204bd4f7:refs/heads/level1-v0.1.0-rc.5. Read it back and require the exact SHA. Never force-update, delete, or move any existing ref.
- Locate the new push-triggered GitHub Actions CI run through the public REST API, require event push, head_branch level1-v0.1.0-rc.5, and head_sha 19c1ef4b6a610016487880aa8ad69ec0204bd4f7, then poll with a finite bound until completion. Fail if the exact run cannot be identified unambiguously or does not conclude success.
- Require exactly the ten mandatory successful jobs for that run: Go 1.22 source floor and tests; Production autonomous strict-fake suite; Race tests; Vet and module verification; Fake-Codex success smoke; Fake-Codex verification-failure smoke; Build linux/amd64; Build darwin/amd64; Build freebsd/amd64; and Build Windows diagnostic stub. Record exact run and job IDs, URLs, head SHA, status, and conclusions in durable state.
- Stop after remote CI evidence. Do not add or publish an attestation workflow/ref, prepare a suite, run live model work, tag a release, approve external use, or mark EXT-20 complete.
- Update durable state, decisions, and handoff with the exact remote candidate-ref readback, CI run/job evidence, preservation checks, and the next separately bounded RC.5 exact-checkout Go 1.26.5 attestation-workflow step."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
