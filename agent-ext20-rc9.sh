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
	printf 'RC.9 construction requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.9 construction requires exact local, fetched, and public main\n' >&2
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
	printf 'Product source changed after the pinned RC.9 source commit\n' >&2
	exit 1
}

require_sha256() {
	local expected="$1"
	local path="$2"
	local actual
	[[ -f "$path" && ! -L "$path" ]] || {
		printf 'Required immutable file is absent or not regular: %s\n' "$path" >&2
		exit 1
	}
	actual="$(sha256sum "$path" | awk '{print $1}')"
	[[ "$actual" == "$expected" ]] || {
		printf 'Immutable file hash changed: %s\n' "$path" >&2
		exit 1
	}
}

require_tree_identity() {
	local expected_count="$1"
	local expected_hash="$2"
	local path="$3"
	local actual_count actual_hash
	[[ -d "$path" && ! -L "$path" ]] || {
		printf 'Required immutable tree is absent or not a directory: %s\n' "$path" >&2
		exit 1
	}
	actual_count="$(find "$path" -type f -printf '.' | wc -c)"
	actual_hash="$(cd "$path" && find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum | sha256sum | awk '{print $1}')"
	[[ "$actual_count" == "$expected_count" && "$actual_hash" == "$expected_hash" ]] || {
		printf 'Immutable tree identity changed: %s\n' "$path" >&2
		exit 1
	}
}

require_sha256 \
	"b4b23c2ede3502f666253e8b34e6962df58bbf848fe108c51172b95619d45b6e" \
	.revolvr/release-candidates/build-level1-v0.1.0-rc.8.sh
require_sha256 \
	"5cc3e477270fd051d967bf63de50a4c2ad007ed8b5217a429e8a8525ebcc89c5" \
	.revolvr/release-candidates/diagnostics/level1-v0.1.0-rc.8-20260726T121225Z-227936.txt
require_tree_identity \
	15882 \
	"576db5a2a76ef52013b54c1cb29d5623908e57b1f34a772d7f56e5635cf952e2" \
	/tmp/revolvr-ext20-rc8-build.wnKv7Q
require_tree_identity \
	20 \
	"ea9fdc5cc97e475d13109d8a3b1d7eafb25e4b3cd87e4085c5bef44aa3e2841a" \
	/tmp/revolvr-ext20-rc8-build.wnKv7Q/verification

for path in \
	.revolvr/release-candidates/level1-v0.1.0-rc.8-a24804bcf2a3 \
	.revolvr/release-candidates/level1-v0.1.0-rc.8-a24804bcf2a3-verification
do
	[[ ! -e "$path" && ! -L "$path" ]] || {
		printf 'Failed RC.8 final-path boundary changed: %s\n' "$path" >&2
		exit 1
	}
done

[[ ! -e agent-ext20-rc9-local-review.sh && ! -L agent-ext20-rc9-local-review.sh ]] || {
	printf 'RC.9 local-review launcher already exists\n' >&2
	exit 1
}
[[ ! -e .github/workflows/level1-rc9-candidate-attestation.yml && ! -L .github/workflows/level1-rc9-candidate-attestation.yml ]] || {
	printf 'RC.9 workflow collision\n' >&2
	exit 1
}
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.9 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.9-attestation; then
	printf 'Local RC.9 ref collision\n' >&2
	exit 1
fi
REMOTE_RC9_REFS="$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.9 refs/heads/level1-v0.1.0-rc.9-attestation)"
[[ -z "$REMOTE_RC9_REFS" ]] || {
	printf 'Remote RC.9 ref collision\n' >&2
	exit 1
}
[[ -z "$(git tag --list '*rc.9*')" ]] || {
	printf 'Local RC.9 tag collision\n' >&2
	exit 1
}
REMOTE_RC9_TAGS="$(git ls-remote --tags origin '*rc.9*')"
[[ -z "$REMOTE_RC9_TAGS" ]] || {
	printf 'Remote RC.9 tag collision\n' >&2
	exit 1
}
if find .revolvr -maxdepth 4 \( -iname '*rc9*' -o -iname '*rc.9*' \) -print -quit | grep -q .; then
	printf 'Local RC.9 runtime, diagnostic, or bundle collision\n' >&2
	exit 1
fi
if find /tmp -maxdepth 1 \( -iname 'revolvr-ext20-rc9-*' -o -iname '*level1-v0.1.0-rc.9*' \) -print -quit | grep -q .; then
	printf 'Temporary RC.9 construction-root collision\n' >&2
	exit 1
fi

RELEASE_GO="/usr/local/go/bin/go"
SOURCE_GO="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64/bin/go"
require_sha256 \
	"8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9" \
	"$RELEASE_GO"
require_sha256 \
	"929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec" \
	"$SOURCE_GO"

clean_go() {
	env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local "$@"
}

[[ "$(clean_go "$RELEASE_GO" version)" == "go version go1.26.5 linux/amd64" ]] || {
	printf 'Exact release Go version changed\n' >&2
	exit 1
}
[[ "$(clean_go "$RELEASE_GO" env GOROOT)" == "/usr/local/go" ]] || {
	printf 'Exact release Go resolved the wrong GOROOT\n' >&2
	exit 1
}
[[ "$(clean_go "$RELEASE_GO" env GOTOOLDIR)" == "/usr/local/go/pkg/tool/linux_amd64" ]] || {
	printf 'Exact release Go resolved the wrong GOTOOLDIR\n' >&2
	exit 1
}

SOURCE_GOROOT="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64"
[[ "$(clean_go "$SOURCE_GO" version)" == "go version go1.22.12 linux/amd64" ]] || {
	printf 'Exact source-floor Go version changed\n' >&2
	exit 1
}
[[ "$(clean_go "$SOURCE_GO" env GOROOT)" == "$SOURCE_GOROOT" ]] || {
	printf 'Exact source-floor Go resolved the wrong GOROOT\n' >&2
	exit 1
}
[[ "$(clean_go "$SOURCE_GO" env GOTOOLDIR)" == "$SOURCE_GOROOT/pkg/tool/linux_amd64" ]] || {
	printf 'Exact source-floor Go resolved the wrong GOTOOLDIR\n' >&2
	exit 1
}

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 RC.9 local candidate pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not prepare or launch a live dogfood suite, use any live confirmation token, publish a candidate or attestation ref, add an attestation workflow, tag, release, approve external use, grant recovery or queue authority, or complete EXT-20.
- Do exactly one bounded task: construct and locally verify a fresh collision-free Level-1 candidate named level1-v0.1.0-rc.9 from exact published planner-contract remediation source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba and tree 2c8ee9f6b4283410547a9f99972e25eac06c9e33.
- Require source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba to be published and reachable from origin/main. The later controller commits containing RC.8 failure history and this launcher are not candidate source and must not enter candidate clones or artifacts. Require the product-source diff from the remediation commit through controller HEAD to be empty for .agent/profiles, cmd, internal, go.mod, and go.sum.
- Treat RC.1 through RC.8 and every historical candidate, ref, workflow, artifact, bundle, builder, partial tree, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never retry, resume, repair, reconcile, relabel, delete, mutate, derive candidate material from, or reuse any of it.
- In particular, do not execute or copy the failed RC.8 builder. Preserve its exact builder, diagnostic, 15,882-file partial root, and 20-file partial verification subtree at the hashes admitted by this launcher. Keep the two intended final RC.8 paths absent. Reverify all four retained RC.8 identities and both absences read-only before and after work.
- Preserve byte-for-byte the RC.7 suite /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite, launch record /home/gernsback/source/revolvr/.revolvr/ext20-rc7-launch-records/ext20-14b2bf40212b-20260725T183415Z-1784416, and terminal evidence /home/gernsback/source/revolvr/.revolvr/ext20-rc7.rpIUM5/suite/evidence/repo-a/01-successful-source-change-1 at content-stream SHA-256 ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a, deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c, and 6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f before and after work. Reverify both terminal checksum manifests read-only before and after.
- Also preserve the independently retired RC.6 suite, launch record, and terminal evidence at content-stream SHA-256 d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, and e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259.
- Before construction, fail on any local or remote RC.9 candidate ref, attestation ref, tag, workflow, bundle, verification bundle, build root, suite root, launch record, review launcher, or diagnostic collision. Do not overwrite or reuse partial output; retain any fail-closed diagnostic under a unique suffix and stop. Use a fresh independently authored RC.9 builder and unique fresh roots.
- The parent Codex environment has GOROOT and GOTOOLDIR removed, GOFLAGS removed, GOENV=off, and GOTOOLCHAIN=local. Preserve those controls. For every selected Go executable invocation, explicitly use env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local. Before creating a build root, record and require exact executable path, executable SHA-256, go version, go env GOROOT, and go env GOTOOLDIR. Require GOROOT/GOTOOLDIR to belong to that selected executable's exact toolchain root. Use task-specific independent GOCACHE and GOMODCACHE roots for each clean clone and toolchain pass.
- Reuse the settled EXT-18 reproducible procedure without changing product source or dependencies: Go 1.22.12 source-floor verification; exact Go 1.26.5 release builds; version 0.1.0; module-readonly mode; disabled CGO; amd64; trimpath; explicit clean VCS metadata; empty Go build ID; and Linux, Darwin, and FreeBSD targets. Build twice in independent fresh non-local clean clones and require byte-identical artifacts.
- Rerun the planner lifecycle prompt/schema/revision regressions, Structured Outputs compatibility guard, production autonomous happy path, strict-fake Codex contract, full Go suite, vet, module verification, and vulnerability scan. These are local evidence only and make no live API-acceptance claim.
- Retain a new immutable RC.9 candidate bundle and separate verification bundle with exact source/tree/tool/build/version/target identities, effective clean Go environment identities, build instructions, artifact hashes, embedded metadata, tests, vulnerability result, complete sorted regular-file inventories, and inventory hashes. Verify every bundle from its manifest after construction.
- RC.9 local construction grants no remote-CI, attestation, dogfood, live-model, suite-preparation, tag, release, external-use, recovery, or queue authority. It must not create or reuse a suite.
- Keep EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact files, hashes, bundle paths, verification, RC.6/RC.7/RC.8 preservation evidence, and the next independent local-review gate. Create at most one inert next-pass local-review launcher; do not execute it. Stop after this one task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
