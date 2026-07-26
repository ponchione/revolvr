#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

[[ -f .agent/LOOP_PROMPT.md ]] || {
	printf 'Missing .agent/LOOP_PROMPT.md\n' >&2
	exit 1
}

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] || {
	printf 'Builder revalidation requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'Builder revalidation requires exact local, fetched, and public main\n' >&2
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

VALIDATION_ROOT="/tmp/revolvr-builder-validation.maYqgv"
DRAFT="$VALIDATION_ROOT/candidate-construction-draft.sh"
EVIDENCE_MANIFEST="$VALIDATION_ROOT/evidence-manifest.tsv"
[[ -d "$VALIDATION_ROOT" && ! -L "$VALIDATION_ROOT" && "$(stat -c '%a' "$VALIDATION_ROOT")" == 500 ]]
require_sha256 \
	"71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d" \
	"$DRAFT"
[[ "$(stat -c '%a:%s' "$DRAFT")" == "444:24352" && "$(wc -l <"$DRAFT")" == 575 ]]
require_sha256 \
	"2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85" \
	"$EVIDENCE_MANIFEST"
[[ "$(stat -c '%a:%s' "$EVIDENCE_MANIFEST")" == "444:3695" && "$(wc -l <"$EVIDENCE_MANIFEST")" == 38 ]]
while IFS=$'\t' read -r mode size sha name; do
	path="$VALIDATION_ROOT/$name"
	[[ "$name" != */* && -f "$path" && ! -L "$path" ]]
	[[ "$(stat -c '%a:%s' "$path")" == "$mode:$size" ]]
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$sha" ]]
done <"$EVIDENCE_MANIFEST"
[[ "$(find "$VALIDATION_ROOT" -maxdepth 1 -type f | wc -l)" == 40 ]]
cmp "$VALIDATION_ROOT/history-metadata-before.tsv" "$VALIDATION_ROOT/history-metadata-after.tsv"
cmp "$VALIDATION_ROOT/history-content-before.sha256" "$VALIDATION_ROOT/history-content-after.sha256"

require_sha256 \
	"9aa31f45fc925e214c180fad8abac262d812f93b795f3a22105ac4d3853820e3" \
	agent-ext20-rc12-builder-validation.sh
require_sha256 \
	"0cdbde37d6c33404c988b68c2da28fd325c50c277333ba35983bad419a235fbb" \
	agent-ext20-rc12-builder-validation-review.sh
bash -n agent-ext20-rc12-builder-validation.sh
bash -n agent-ext20-rc12-builder-validation-review.sh

RC11_DRAFT="/tmp/revolvr-builder-draft.cZLxf2/builder.sh"
RC11_BUILDER=".revolvr/release-candidates/build-level1-v0.1.0-rc.11.sh"
require_sha256 \
	"c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355" \
	"$RC11_DRAFT"
require_sha256 \
	"c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355" \
	"$RC11_BUILDER"
cmp "$RC11_DRAFT" "$RC11_BUILDER"

if find /tmp -maxdepth 1 -name 'revolvr-builder-revalidation.*' -print -quit | grep -q .; then
	printf 'Neutral builder-revalidation root collision\n' >&2
	exit 1
fi
if find .revolvr -maxdepth 6 \( -iname '*rc12*' -o -iname '*rc.12*' \) -print -quit | grep -q .; then
	printf 'Prospective RC.12 runtime identity already exists\n' >&2
	exit 1
fi
[[ ! -e agent-ext20-rc12.sh && ! -L agent-ext20-rc12.sh ]]
[[ ! -e agent-ext20-rc12-local-review.sh && ! -L agent-ext20-rc12-local-review.sh ]]
[[ ! -e agent-ext20-rc12-builder-revalidation-review.sh && ! -L agent-ext20-rc12-builder-revalidation-review.sh ]]
[[ ! -e .github/workflows/level1-rc12-candidate-attestation.yml && ! -L .github/workflows/level1-rc12-candidate-attestation.yml ]]
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
require_sha256 \
	"8da5fd321795754b994c64e3eb8a5a14ff47bd285559a7e876f3c79abafc67f9" \
	"$RELEASE_GO"
require_sha256 \
	"929407e69c08952cd944a7457ae4eb289078a35473dd5dad2179369db7c5a6ec" \
	"$SOURCE_GO"
clean_go() {
	env -u GOROOT -u GOTOOLDIR -u GOFLAGS GOENV=off GOTOOLCHAIN=local "$@"
}
[[ "$(clean_go "$RELEASE_GO" version)" == "go version go1.26.5 linux/amd64" ]]
[[ "$(clean_go "$RELEASE_GO" env GOROOT)" == "/usr/local/go" ]]
SOURCE_GOROOT="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64"
[[ "$(clean_go "$SOURCE_GO" version)" == "go version go1.22.12 linux/amd64" ]]
[[ "$(clean_go "$SOURCE_GO" env GOROOT)" == "$SOURCE_GOROOT" ]]

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 prospective RC.12 neutral-builder revalidation pass:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not create or execute an RC.12 builder, candidate, construction launcher, preflight/build/stage root, artifact, bundle, ref, workflow, tag, suite, launch record, Revolvr/model operation, release, or external-use action.
- Do exactly one bounded task: independently author and validate a corrected anonymous prospective builder in a unique /tmp/revolvr-builder-revalidation.XXXXXX root. Do not edit, copy, execute, derive bytes from, or reuse the sealed first validation root or any historical builder. Use the independent review findings below as requirements.
- The first sealed draft's validation evidence is authentic but independent review rejects it for construction. Full mode proves its exact builder exists at lines 564-568, then require_collision_free at lines 165-179 requires that same builder absent. It also wrongly requires the necessary construction launcher and existing validation-review launcher absent. Correct the context model: full mode requires and hashes its exact read-only builder and the separately published construction launcher, permits the two tracked validation-history launchers, and requires only candidate outputs, post-candidate local-review launcher, refs/tags/workflow/remote artifacts/runtime roots to be absent. Neutral admission must model and statically prove this distinction without creating an identity.
- Full mode must independently snapshot and compare before/after all immutable RC.6-RC.11 histories, including RC.6/RC.7 terminal checksum manifests, RC.8/RC.9 files and trees, RC.9 staged manifests, RC.10/RC.11 builders/drafts, and both sealed neutral-validation roots. Preserve every recorded absence.
- Fetch each candidate source clone with git init plus a shallow exact-commit fetch from a non-local origin. Require exact detached commit/tree, clean status, and absence of later controller commits and launchers from both checkout and object database. Do not use a full clone containing controller history.
- Candidate and verification bundles must include executable build instructions; exact source/tree/controller/product-diff identity; tool executable hashes, versions, GOROOT/GOTOOLDIR and clean environment; every test/vet/module/vulnerability command and result including verbose unreachable findings; reproducibility hashes; artifact build metadata with exact GOOS, GOARCH, CGO, source revision, clean VCS state and empty build ID; sorted complete inventories and their hashes; and history preservation results.
- Admission must check local/remote refs and tags, workflow, release assets and Actions artifacts, exact final paths, preflight/build/stage/diagnostic roots, suite/launch roots, construction conflicts, and the correct post-candidate local-review launcher. No remote write is authorized.
- Retain the corrected publication contract: neutral probe writes under writable parents, seals file/nested/source, uses mkdir plus cp -a, verifies bytes/modes/link/inode separation, restores parent writes, removes only the probe, and proves absence. Final publication uses no rename/link/symlink and verifies complete stage/final manifests, inventories, content hashes, file counts, and modes. A final-path failure is terminal.
- The new neutral draft must support --neutral-admission and --neutral-full-context-audit before exact self-identity enforcement. Run bash -n, both modes, a focused static audit, and an expected no-argument self-identity refusal. The full-context audit must fail if the expected exact builder or construction launcher is placed in an absence list, if validation-history launchers are rejected, or if post-candidate review/output collisions are not checked. Run a second complete sequence after at most one neutral repair.
- Do not run product tests/builds or full mode. Seal the new root/draft/evidence read-only with a self-verifying manifest. Record exact identities and the independent-review rejection/remediation result in .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md. Keep EXT-20 unchecked.
- Create at most one inert agent-ext20-rc12-builder-revalidation-review.sh for a later independent review. Do not create agent-ext20-rc12.sh or candidate authority. Stop after this neutral revalidation task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
