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
	printf 'RC.11 construction requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'RC.11 construction requires exact local, fetched, and public main\n' >&2
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
	printf 'Product source changed after the pinned RC.11 source commit\n' >&2
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

require_mode() {
	local expected="$1"
	local path="$2"
	[[ "$(stat -c '%a' "$path")" == "$expected" ]] || {
		printf 'Immutable file mode changed: %s\n' "$path" >&2
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

RC10_BUILDER=".revolvr/release-candidates/build-level1-v0.1.0-rc.10.sh"
require_sha256 \
	"229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be" \
	"$RC10_BUILDER"
require_mode 664 "$RC10_BUILDER"
[[ "$(wc -l <"$RC10_BUILDER")" == "474" ]] || {
	printf 'Immutable RC.10 builder line count changed\n' >&2
	exit 1
}

mapfile -t RC10_RUNTIME_PATHS < <(find .revolvr -maxdepth 6 \( -iname '*rc10*' -o -iname '*rc.10*' \) -print | LC_ALL=C sort)
[[ "${#RC10_RUNTIME_PATHS[@]}" == "1" && "${RC10_RUNTIME_PATHS[0]}" == "$RC10_BUILDER" ]] || {
	printf 'Failed RC.10 runtime boundary changed\n' >&2
	exit 1
}
if find /tmp -maxdepth 1 \( -iname '*rc10*' -o -iname '*rc.10*' \) -print -quit | grep -q .; then
	printf 'Failed RC.10 temporary-path boundary changed\n' >&2
	exit 1
fi
for path in \
	.revolvr/release-candidates/level1-v0.1.0-rc.10-a24804bcf2a3 \
	.revolvr/release-candidates/level1-v0.1.0-rc.10-a24804bcf2a3-verification
do
	[[ ! -e "$path" && ! -L "$path" ]] || {
		printf 'Failed RC.10 final-path boundary changed: %s\n' "$path" >&2
		exit 1
	}
done

RC9_BUILDER=".revolvr/release-candidates/build-level1-v0.1.0-rc.9.sh"
RC9_DIAGNOSTIC=".revolvr/release-candidates/diagnostics/level1-v0.1.0-rc.9-20260726T130454Z-370868.qiRJxA.txt"
RC9_PREFLIGHT=".revolvr/release-candidates/.level1-v0.1.0-rc.9-preflight.XQ9PIf"
RC9_BUILD="/tmp/revolvr-ext20-rc9-build.CRYAYI"
RC9_STAGE=".revolvr/release-candidates/.level1-v0.1.0-rc.9-stage.Znq9cp"
require_sha256 \
	"0b4dcba0a68aa9d801d657a085fa1c8b7a81fd503bea773b0670ec394f456ab4" \
	"$RC9_BUILDER"
require_sha256 \
	"b61fc1cf82d7777b58337766c0fe941b901101b6e14fd1b9e1cd9fb7a1774160" \
	"$RC9_DIAGNOSTIC"
require_tree_identity 6 \
	"a52b2f624e03276d2079a45c73e3c172a5387608c341f7376bd7f6cb54959547" \
	"$RC9_PREFLIGHT"
require_tree_identity 34435 \
	"2f2d3392a20afffc6b676cd72b4b71e1f8283f2567dac623b506880c273237e6" \
	"$RC9_BUILD"
require_tree_identity 63 \
	"382e16afb4efbbf25330572ab5ee186001a06bc21112cd18b41414e790519a46" \
	"$RC9_STAGE"

[[ ! -e agent-ext20-rc11-local-review.sh && ! -L agent-ext20-rc11-local-review.sh ]] || {
	printf 'RC.11 local-review launcher already exists\n' >&2
	exit 1
}
[[ ! -e .github/workflows/level1-rc11-candidate-attestation.yml && ! -L .github/workflows/level1-rc11-candidate-attestation.yml ]] || {
	printf 'RC.11 workflow collision\n' >&2
	exit 1
}
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.11 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.11-attestation; then
	printf 'Local RC.11 ref collision\n' >&2
	exit 1
fi
REMOTE_RC11_REFS="$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.11 refs/heads/level1-v0.1.0-rc.11-attestation)"
[[ -z "$REMOTE_RC11_REFS" ]] || {
	printf 'Remote RC.11 ref collision\n' >&2
	exit 1
}
[[ -z "$(git tag --list '*rc.11*')" ]] || {
	printf 'Local RC.11 tag collision\n' >&2
	exit 1
}
REMOTE_RC11_TAGS="$(git ls-remote --tags origin '*rc.11*')"
[[ -z "$REMOTE_RC11_TAGS" ]] || {
	printf 'Remote RC.11 tag collision\n' >&2
	exit 1
}
if find .revolvr -maxdepth 5 \( -iname '*rc11*' -o -iname '*rc.11*' \) -print -quit | grep -q .; then
	printf 'Local RC.11 runtime, diagnostic, or bundle collision\n' >&2
	exit 1
fi
if find /tmp -maxdepth 1 \( -iname 'revolvr-ext20-rc11-*' -o -iname '*level1-v0.1.0-rc.11*' \) -print -quit | grep -q .; then
	printf 'Temporary RC.11 construction-root collision\n' >&2
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

Additional operator direction for the EXT-20 RC.11 local candidate pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push; the controller will independently review and publish later.
- Do not start a Revolvr live operation or nested Codex/model operation. Do not prepare or launch a live dogfood suite, use a live confirmation token, publish a candidate or attestation ref, add an attestation workflow, tag, release, approve external use, grant recovery or queue authority, or complete EXT-20.
- Do exactly one bounded task: construct and locally verify a fresh collision-free Level-1 candidate named level1-v0.1.0-rc.11 from exact published source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba and tree 2c8ee9f6b4283410547a9f99972e25eac06c9e33.
- Require the source commit to be published and reachable from origin/main. Later controller commits and launchers are not candidate source and must not enter candidate clones or artifacts. Require the product-source diff from the source commit through controller HEAD to be empty for .agent/profiles, cmd, internal, go.mod, and go.sum.
- Treat RC.1 through RC.10 and every historical candidate, ref, workflow, artifact, bundle, builder, partial or staged tree, suite, operation, launch record, diagnostic, and evidence root as immutable rejected or failed history. Never retry, execute, resume, repair, reconcile, relabel, delete, mutate, derive candidate material from, or reuse any of it.
- Preserve the exact RC.10 malformed builder as its sole runtime identity, with 474 lines, mode 0664, and SHA-256 229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be. Keep every other RC.10 preflight/build/stage/diagnostic/artifact/bundle/review path absent. Its syntax defect is the missing inner closing quote in the final candidate-inventory file_hash command substitution at line 441; do not edit it.
- Preserve RC.9's exact builder, diagnostic, preflight/build/stage trees, staged inventories/manifests, and absent final paths. Preserve RC.8's exact builder, diagnostic, partial trees, and absent final paths. Preserve the RC.6 and RC.7 suite, launch-record, and terminal-evidence content-stream hashes and reverify both terminal checksum manifest pairs before and after.
- Before any RC.11-named runtime path is created, author the proposed builder only as a neutral draft under a unique /tmp/revolvr-builder-draft.XXXXXX directory whose path and contents do not contain rc11, rc.11, or level1-v0.1.0-rc.11. Run bash -n on the neutral draft. If it fails, inspect it and make at most one reasonable syntax repair to the neutral draft, rerun bash -n, retain a neutral diagnostic, and stop if it still fails. A neutral draft failure does not authorize creating an RC.11 identity.
- After the neutral draft passes bash -n, review every quoting-heavy command-substitution and inventory-summary line, then copy its exact bytes to the sole exact ignored builder .revolvr/release-candidates/build-level1-v0.1.0-rc.11.sh. Set mode 0555, require byte-identical SHA-256 with the parsed draft, run bash -n again on the exact builder, and only then execute it once. Never edit the exact builder after it exists. Any failure after the exact builder appears exhausts RC.11 and all resulting output must remain unchanged.
- Before construction, fail on any local or remote RC.11 candidate ref, attestation ref, tag, workflow, remote artifact, bundle, verification bundle, build/stage/preflight root, suite root, launch record, review launcher, or diagnostic collision.
- Preserve the clean Go boundary: every selected Go invocation explicitly uses env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local. Before creating an RC.11 build root, record and require exact executable path, SHA-256, version, GOROOT, and GOTOOLDIR for each selected toolchain. Use independent task/clone/toolchain GOCACHE and GOMODCACHE roots.
- Use only the proven final publication method: preflight fresh neutral mkdir plus cp -a source/. destination/ paths, verify copied structure/modes, make copied parents writable before removing only the neutral probe, and leave no probe path. Never use directory rename, hard links, or symlinks. Stage and seal RC.11 in its unique fresh build root; then create each absent final directory, copy its stage contents with cp -a STAGE/. FINAL/, and verify manifests, inventory hashes, counts, content-stream hashes, modes, and stage/final equality. Once either final path exists, any failure exhausts RC.11 and no cleanup, completion, or repair is authorized.
- Reuse the settled EXT-18 reproducible procedure without changing product source or dependencies: Go 1.22.12 source-floor verification; exact Go 1.26.5 release builds; version 0.1.0; module-readonly mode; disabled CGO; amd64; trimpath; clean VCS metadata; empty Go build ID; and Linux, Darwin, and FreeBSD targets. Build twice in independent fresh non-local clean clones and require byte-identical artifacts.
- Rerun the planner lifecycle prompt/schema/revision regressions, Structured Outputs compatibility guard, production autonomous happy path, strict-fake Codex contract, full Go suite, vet, module verification, and vulnerability scan. These are local evidence only and make no live API-acceptance claim.
- Retain a new immutable RC.11 candidate bundle and separate verification bundle with exact source/tree/tool/build/version/target/environment identities, build instructions, artifact hashes, embedded metadata, tests, vulnerability result, complete sorted regular-file inventories, and inventory hashes. Verify every bundle from its manifest after construction.
- RC.11 local construction grants no remote-CI, attestation, dogfood, live-model, suite-preparation, tag, release, external-use, recovery, or queue authority. It must not create or reuse a suite.
- Keep EXT-20 unchecked. Update .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md with exact draft/builder identity, bundle paths and verification, RC.6 through RC.10 preservation evidence, and the next independent local-review gate. Create at most one inert next-pass local-review launcher; do not execute it. Stop after this one task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
