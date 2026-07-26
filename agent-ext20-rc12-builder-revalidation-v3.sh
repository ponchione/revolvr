#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

[[ -f .agent/LOOP_PROMPT.md ]] || {
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
}

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'Third builder revalidation requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'Third builder revalidation requires exact local, fetched, and public main\n' >&2
	exit 1
}
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]]
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum

require_sha256() {
	local expected="$1"
	local path="$2"
	[[ -f "$path" && ! -L "$path" ]] || {
		printf 'Required immutable file is absent or unsafe: %s\n' "$path" >&2
		exit 1
	}
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$expected" ]] || {
		printf 'Immutable file hash changed: %s\n' "$path" >&2
		exit 1
	}
}

FIRST_VALIDATION_ROOT="/tmp/revolvr-builder-validation.maYqgv"
[[ -d "$FIRST_VALIDATION_ROOT" && ! -L "$FIRST_VALIDATION_ROOT" ]]
[[ "$(stat -c '%a' "$FIRST_VALIDATION_ROOT")" == 500 ]]
require_sha256 \
	"71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d" \
	"$FIRST_VALIDATION_ROOT/candidate-construction-draft.sh"
require_sha256 \
	"2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85" \
	"$FIRST_VALIDATION_ROOT/evidence-manifest.tsv"

SECOND_VALIDATION_ROOT="/tmp/revolvr-builder-revalidation.CSGs5E"
SECOND_DRAFT="$SECOND_VALIDATION_ROOT/prospective-construction.sh"
SECOND_MANIFEST="$SECOND_VALIDATION_ROOT/evidence-manifest.tsv"
[[ -d "$SECOND_VALIDATION_ROOT" && ! -L "$SECOND_VALIDATION_ROOT" ]]
[[ "$(stat -c '%a' "$SECOND_VALIDATION_ROOT")" == 500 ]]
require_sha256 \
	"ae560b6eebc0c2d77721df426477727c473774cc1661db9dc9ef2194fd120768" \
	"$SECOND_DRAFT"
[[ "$(stat -c '%a:%s' "$SECOND_DRAFT")" == "444:42999" ]]
[[ "$(wc -l <"$SECOND_DRAFT")" == 895 ]]
require_sha256 \
	"0b962f2cfc2095c530d4414676e17c53a06d8ef65223baf7d9f26e6e958453ee" \
	"$SECOND_MANIFEST"
[[ "$(stat -c '%a:%s' "$SECOND_MANIFEST")" == "444:1037" ]]
[[ "$(wc -l <"$SECOND_MANIFEST")" == 11 ]]
while IFS=$'\t' read -r mode size sha name; do
	[[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]
	path="$SECOND_VALIDATION_ROOT/$name"
	[[ -f "$path" && ! -L "$path" ]]
	[[ "$(stat -c '%a:%s' "$path")" == "$mode:$size" ]]
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$sha" ]]
done <"$SECOND_MANIFEST"
[[ "$(find "$SECOND_VALIDATION_ROOT" -maxdepth 1 -type f | wc -l)" == 12 ]]
[[ "$(find "$SECOND_VALIDATION_ROOT" -maxdepth 1 -type f -printf '%s\n' | awk '{total += $1} END {print total}')" == 50756 ]]
[[ ! -e /tmp/revolvr-neutral-publication.Jcoaht && ! -L /tmp/revolvr-neutral-publication.Jcoaht ]]

require_sha256 \
	"9aa31f45fc925e214c180fad8abac262d812f93b795f3a22105ac4d3853820e3" \
	agent-ext20-rc12-builder-validation.sh
require_sha256 \
	"0cdbde37d6c33404c988b68c2da28fd325c50c277333ba35983bad419a235fbb" \
	agent-ext20-rc12-builder-validation-review.sh
require_sha256 \
	"9218d873e24214aa7fff9574e1e35ede1e967629947e101e961ad87eb285b4d7" \
	agent-ext20-rc12-builder-revalidation.sh
bash -n agent-ext20-rc12-builder-validation.sh
bash -n agent-ext20-rc12-builder-validation-review.sh
bash -n agent-ext20-rc12-builder-revalidation.sh

if find /tmp -maxdepth 1 -name 'revolvr-builder-revalidation-v3.*' -print -quit | grep -q .; then
	printf 'Third neutral builder-revalidation root collision\n' >&2
	exit 1
fi
if find .revolvr -maxdepth 6 \( -iname '*rc12*' -o -iname '*rc.12*' \) -print -quit | grep -q .; then
	printf 'Prospective RC.12 runtime identity already exists\n' >&2
	exit 1
fi
for forbidden_path in \
	agent-ext20-rc12.sh \
	agent-ext20-rc12-local-review.sh \
	agent-ext20-rc12-builder-revalidation-review.sh \
	agent-ext20-rc12-builder-revalidation-v3-review.sh \
	.github/workflows/level1-rc12-candidate-attestation.yml
do
	[[ ! -e "$forbidden_path" && ! -L "$forbidden_path" ]]
done
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12-attestation; then
	printf 'Local RC.12 ref collision\n' >&2
	exit 1
fi
[[ -z "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.12 refs/heads/level1-v0.1.0-rc.12-attestation)" ]]
[[ -z "$(git tag --list '*rc.12*')" ]]
[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]]

RELEASE_GO="/usr/local/go/bin/go"
SOURCE_GO="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64/bin/go"
GOVULNCHECK="/home/gernsback/go/bin/govulncheck"
require_sha256 \
	"8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9" \
	"$RELEASE_GO"
require_sha256 \
	"929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec" \
	"$SOURCE_GO"
require_sha256 \
	"f66036976d8995fbed427315bb2d6b525e58ee5867e88f097709e62fe93b412f" \
	"$GOVULNCHECK"
clean_go() {
	env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local "$@"
}
[[ "$(clean_go "$RELEASE_GO" version)" == "go version go1.26.5 linux/amd64" ]]
[[ "$(clean_go "$RELEASE_GO" env GOROOT)" == "/usr/local/go" ]]
[[ "$(clean_go "$RELEASE_GO" env GOTOOLDIR)" == "/usr/local/go/pkg/tool/linux_amd64" ]]
SOURCE_GOROOT="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64"
[[ "$(clean_go "$SOURCE_GO" version)" == "go version go1.22.12 linux/amd64" ]]
[[ "$(clean_go "$SOURCE_GO" env GOROOT)" == "$SOURCE_GOROOT" ]]
[[ "$(clean_go "$SOURCE_GO" env GOTOOLDIR)" == "$SOURCE_GOROOT/pkg/tool/linux_amd64" ]]

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Third neutral builder-revalidation preflight passed\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 prospective RC.12 third neutral-builder revalidation pass:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not create or execute an RC.12 builder, candidate, construction launcher, preflight/build/stage/diagnostic root, artifact, bundle, ref, workflow, tag, suite, launch record, Revolvr task, release, or external-use action.
- Do exactly one bounded task: independently author and validate a third anonymous prospective builder in a unique /tmp/revolvr-builder-revalidation-v3.XXXXXX root. Do not edit, copy, execute, source, derive bytes from, or reuse either sealed prior draft, either sealed validation root, or any historical builder. Requirements and independently verified constants may be used; prior implementation text may not.
- The second sealed revalidation record at $SECOND_VALIDATION_ROOT is authentic terminal failure evidence. Its draft corrected the full-context design, but attempt one used wrong RC.6/RC.7 file counts and consumed the sole repair; attempt two reached copy-publication and then failed because cleanup restored write permission on only source/destination roots while nested directories remained mode 0500. Do not repair that sealed draft. Independently reimplement the design.
- Before starting validation sequence one, remeasure every historical count/hash constant with read-only commands and compare it with durable state. This authoring check is mandatory so a transcription error does not become the validation repair. Preserve exact RC.6/RC.7 stream tuples 461/d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b, 4/2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce, 130/e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259 and 461/ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a, 4/deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c, 130/6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f.
- Reimplement neutral cleanup as a single guarded function that accepts only its exact /tmp/revolvr-neutral-publication.XXXXXX root, proves the root is a real directory and the whole tree contains no symlink, restores owner write permission to every directory in the exact probe with a depth-first find, deletes only that tree with depth-first find deletion, and proves absence. It must clean both the success path and an induced pre-copy failure path with at least two nested sealed directory levels and mode-0400 files. Cleanup failure must never be hidden by an earlier probe status.
- The semantic publication probe must write only under writable parents, seal files and every nested/source directory, create the destination separately, use cp -a source/. destination/., verify byte equality, all modes, single-link distinct inodes, complete tree inventories, and no symlinks, then invoke the guarded cleanup. Never use rm -rf, directory rename, hard-link publication, or symlink publication.
- Preserve the corrected context model. Full mode requires and hashes the exact read-only builder and separately published construction launcher; permits all tracked validation-history launchers; and treats only exact candidate/verification outputs, post-candidate local-review launcher, refs/tags/workflow/remote release and Actions namespaces, construction conflicts, and runtime/preflight/build/stage/diagnostic/suite/launch roots as forbidden collisions. Neutral full-context audit must prove these distinctions before exact self-identity enforcement.
- Full mode must independently snapshot and compare before/after every immutable RC.6-RC.11 history and both prior sealed validation roots; verify RC.6/RC.7 terminal checksum manifests and RC.9 staged manifests; preserve every recorded absence; and include the third validation root only as a full-mode required immutable input after publication, never as a forbidden output.
- Fetch each candidate source with git init plus a shallow exact-commit fetch from a non-local origin. Require detached commit $SOURCE_COMMIT, tree $SOURCE_TREE, clean status, and exclusion of later controller commits and launchers from checkout and object database. Candidate and verification designs must retain complete executable build instructions; source/tree/controller/product-diff and clean tool/environment identity; both Go matrices' test/race/vet/module results; ordinary and verbose vulnerability results; reproducibility hashes; exact GOOS/GOARCH/CGO/VCS/build-ID metadata; sorted complete inventories/hashes; and history preservation evidence.
- Final publication must create each exact final directory with mkdir, set final-path-appeared only after successful creation, copy with cp -a, restore the staged root mode, and verify complete stage/final manifests, inventories, hashes, file counts, modes, no links, and no extra entries. A final-path creation or copy failure is terminal and never falls back to rename/link/symlink.
- The draft must support --neutral-admission and --neutral-full-context-audit before exact self-identity enforcement. Run two complete validation sequences. Each sequence includes bash -n, neutral admission, neutral full-context audit, focused static audit, expected no-argument status-64 identity refusal, forbidden-identity scans, history-preservation checks, and probe-residue absence. If sequence one fails, make at most one reasonable neutral repair and record it; sequence two must pass completely or the third draft is rejected. If sequence one passes, make no repair and still run sequence two.
- Never run product tests/builds or full mode. Seal the root, draft, and evidence mode 0444/0500 with a self-verifying manifest. Record exact identities, both full sequences, cleanup-success and induced-failure evidence, before/after preservation, and the terminal result in .agent/HANDOFF.md, .agent/STATE.md, .agent/DECISIONS.md, and the EXT-20 current-gate text. Keep EXT-20 unchecked.
- Only if both sequences pass, create at most one inert agent-ext20-rc12-builder-revalidation-v3-review.sh for later independent review. Do not create agent-ext20-rc12.sh or any candidate authority. If either terminal sequence fails, create no review launcher. Stop after this third neutral revalidation task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
