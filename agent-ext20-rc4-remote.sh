#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

if [[ ! -f .agent/LOOP_PROMPT.md ]]; then
	echo "Missing .agent/LOOP_PROMPT.md" >&2
	exit 1
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.4 candidate-ref and remote-CI gate:
- Never use gh. Use raw git for Git operations and read-only GitHub REST responses for Actions evidence.
- Execution of this launcher is explicit authority for exactly one external mutation: collision-safe creation of refs/heads/level1-v0.1.0-rc.4 at exact source commit 2546913e38ec273f64417dece2f91df78fd42fc2. Do not commit or push main; the controller will independently verify and publish durable-state changes afterward.
- Do not start live or nested Codex/model operations in this pass.
- Read the newest RC.4 local-candidate and independent-controller verification state before acting. Preserve RC.1, RC.2, and RC.3 as immutable rejected history, and preserve every historical ref, workflow, bundle, artifact, hash, suite, operation, run, and diagnostic unchanged.
- Verify both sealed RC.4 inventories, candidate ID level1-v0.1.0-rc.4, release 0.1.0, source commit 2546913e38ec273f64417dece2f91df78fd42fc2, source tree 8b0dfb46a9bfd0d22f14a23af810d7a7cd034aa5, candidate inventory 3535d7a2b46a0dbd3101428b4177e4c46baabc29190e5b1c580d90e6ff033f5d, verification inventory 75a2bcaba12d28d42a5012ad70995f4eb10363e250ec8028350e0802b0b8429c, and Linux/Darwin/FreeBSD hashes 98ab93de990d00c9395d2fc7912658d2f36dcb9f9c3f358fa0422cfe2260e7fe, 042563f350b71ec8cd5be1b49fc9d948383caa28087c0a5689bd6eb12f3808ab, and 128b9f8ced3038a51534da63b9d9ffbaa5ea7341e0ab8dd17102fba86084a8e6 before publication.
- Do exactly one bounded task: publish the exact RC.4 candidate ref and require the complete push-triggered EXT-15 CI matrix to finish successfully on that exact source SHA.
- Immediately before publication, fetch origin without tags, prove the exact source is published and reachable, prove refs/heads/level1-v0.1.0-rc.4 is absent remotely, and prove no RC.4 tag or attestation ref collision exists. Fail closed on any collision or identity drift.
- Create only the candidate ref with raw git using an empty-expected force-with-lease: git push --force-with-lease=refs/heads/level1-v0.1.0-rc.4: origin 2546913e38ec273f64417dece2f91df78fd42fc2:refs/heads/level1-v0.1.0-rc.4. Read it back and require the exact SHA. Never force-update, delete, or move any existing ref.
- Locate the new push-triggered GitHub Actions CI run through the public REST API, require event push, head_branch level1-v0.1.0-rc.4, and head_sha 2546913e38ec273f64417dece2f91df78fd42fc2, then poll with a finite bound until completion. Fail if the exact run cannot be identified unambiguously or does not conclude success.
- Require exactly the ten mandatory successful jobs for that run: Go 1.22 source floor and tests; Production autonomous strict-fake suite; Race tests; Vet and module verification; Fake-Codex success smoke; Fake-Codex verification-failure smoke; Build linux/amd64; Build darwin/amd64; Build freebsd/amd64; and Build Windows diagnostic stub. Record exact run and job IDs, URLs, head SHA, status, and conclusions in durable state.
- Stop after remote CI evidence. Do not add or publish an attestation workflow/ref, prepare a suite, run live model work, tag a release, approve external use, or mark EXT-20 complete.
- Update durable state, decisions, and handoff with the exact remote candidate-ref readback, CI run/job evidence, preservation checks, and the next separately bounded RC.4 exact-checkout Go 1.26.5 attestation-workflow step."

codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
