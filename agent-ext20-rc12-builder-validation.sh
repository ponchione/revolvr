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
	printf 'Builder validation requires a clean controller repository\n' >&2
	exit 1
}

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] || {
	printf 'Builder validation requires exact local, fetched, and public main\n' >&2
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
	printf 'Product source changed after the pinned source commit\n' >&2
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

require_mode_size_lines() {
	local expected_mode="$1"
	local expected_size="$2"
	local expected_lines="$3"
	local path="$4"
	[[ "$(stat -c '%a' "$path")" == "$expected_mode" ]] || {
		printf 'Immutable mode changed: %s\n' "$path" >&2
		exit 1
	}
	[[ "$(stat -c '%s' "$path")" == "$expected_size" ]] || {
		printf 'Immutable size changed: %s\n' "$path" >&2
		exit 1
	}
	[[ "$(wc -l <"$path")" == "$expected_lines" ]] || {
		printf 'Immutable line count changed: %s\n' "$path" >&2
		exit 1
	}
}

RC11_DRAFT="/tmp/revolvr-builder-draft.cZLxf2/builder.sh"
RC11_BUILDER=".revolvr/release-candidates/build-level1-v0.1.0-rc.11.sh"
RC11_SHA="c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355"
require_sha256 "$RC11_SHA" "$RC11_DRAFT"
require_sha256 "$RC11_SHA" "$RC11_BUILDER"
require_mode_size_lines 444 35599 634 "$RC11_DRAFT"
require_mode_size_lines 555 35599 634 "$RC11_BUILDER"
cmp "$RC11_DRAFT" "$RC11_BUILDER"
bash -n "$RC11_DRAFT"
bash -n "$RC11_BUILDER"

mapfile -t RC11_RUNTIME_PATHS < <(find .revolvr -maxdepth 6 \( -iname '*rc11*' -o -iname '*rc.11*' \) -print | LC_ALL=C sort)
[[ "${#RC11_RUNTIME_PATHS[@]}" == "1" && "${RC11_RUNTIME_PATHS[0]}" == "$RC11_BUILDER" ]] || {
	printf 'Failed RC.11 runtime boundary changed\n' >&2
	exit 1
}
for path in \
	.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3 \
	.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3-verification
do
	[[ ! -e "$path" && ! -L "$path" ]] || {
		printf 'Failed RC.11 final-path boundary changed: %s\n' "$path" >&2
		exit 1
	}
done

RC10_BUILDER=".revolvr/release-candidates/build-level1-v0.1.0-rc.10.sh"
require_sha256 \
	"229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be" \
	"$RC10_BUILDER"
require_mode_size_lines 664 28987 474 "$RC10_BUILDER"

if find /tmp -maxdepth 1 -name 'revolvr-builder-validation.*' -print -quit | grep -q .; then
	printf 'Neutral builder-validation root collision\n' >&2
	exit 1
fi
if find .revolvr -maxdepth 6 \( -iname '*rc12*' -o -iname '*rc.12*' \) -print -quit | grep -q .; then
	printf 'Prospective RC.12 runtime identity already exists\n' >&2
	exit 1
fi
[[ ! -e agent-ext20-rc12-builder-validation-review.sh && ! -L agent-ext20-rc12-builder-validation-review.sh ]] || {
	printf 'Builder-validation review launcher already exists\n' >&2
	exit 1
}
[[ ! -e agent-ext20-rc12.sh && ! -L agent-ext20-rc12.sh ]] || {
	printf 'RC.12 construction launcher already exists\n' >&2
	exit 1
}
[[ ! -e .github/workflows/level1-rc12-candidate-attestation.yml && ! -L .github/workflows/level1-rc12-candidate-attestation.yml ]] || {
	printf 'RC.12 workflow collision\n' >&2
	exit 1
}
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12-attestation; then
	printf 'Local RC.12 ref collision\n' >&2
	exit 1
fi
REMOTE_RC12_REFS="$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.12 refs/heads/level1-v0.1.0-rc.12-attestation)"
[[ -z "$REMOTE_RC12_REFS" ]] || {
	printf 'Remote RC.12 ref collision\n' >&2
	exit 1
}
[[ -z "$(git tag --list '*rc.12*')" ]] || {
	printf 'Local RC.12 tag collision\n' >&2
	exit 1
}
REMOTE_RC12_TAGS="$(git ls-remote --tags origin '*rc.12*')"
[[ -z "$REMOTE_RC12_TAGS" ]] || {
	printf 'Remote RC.12 tag collision\n' >&2
	exit 1
}

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
[[ "$(clean_go "$RELEASE_GO" env GOTOOLDIR)" == "/usr/local/go/pkg/tool/linux_amd64" ]]
SOURCE_GOROOT="/home/gernsback/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.22.12.linux-amd64"
[[ "$(clean_go "$SOURCE_GO" version)" == "go version go1.22.12 linux/amd64" ]]
[[ "$(clean_go "$SOURCE_GO" env GOROOT)" == "$SOURCE_GOROOT" ]]
[[ "$(clean_go "$SOURCE_GO" env GOTOOLDIR)" == "$SOURCE_GOROOT/pkg/tool/linux_amd64" ]]

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for the EXT-20 prospective RC.12 neutral-builder validation pass:
- Never use gh. Use raw Git for Git reads. Do not commit or push.
- Do not create an RC.12 builder, candidate identity, preflight/build/stage root, artifact, bundle, ref, workflow, tag, suite, launch record, or construction launcher. Do not start Revolvr, a nested Codex/model operation, a live task, or a release/external-use action. This pass validates a neutral draft only.
- Do exactly one bounded task: independently author and validate a prospective Level-1 candidate-construction builder in a unique neutral root matching /tmp/revolvr-builder-validation.XXXXXX. The root path, draft filename, and draft bytes must not contain the literal strings rc12, rc.12, or level1-v0.1.0-rc.12; derive prospective digits only by separated runtime concatenation.
- Treat RC.1 through RC.11 and all historical builders, drafts, artifacts, bundles, trees, suites, operations, records, diagnostics, and evidence as immutable. Never execute, copy, repair, delete, mutate, derive builder text from, or reuse them. Preserve every recorded identity and absence before and after.
- The neutral draft must support an explicit --neutral-admission mode dispatched after function definitions but before exact-builder/self-identity enforcement. That mode may perform only read-only source/history/tool/collision checks and fresh neutral publication probes. It must never create a prospective candidate identity or run tests/builds.
- Correctly order the semantic copy probe: create source/nested writable, write its value file, make the file mode 0400, then seal nested and source to mode 0500. Only afterward create/copy into the destination with mkdir plus cp -a source/. destination/. Verify byte equality and exact source/destination root, nested-directory, and file modes. Make destination and source parents writable before removing only the unique neutral probe, and prove no probe residue remains. Never use directory rename, hard links, or symlinks.
- First run bash -n on the draft. Then execute its --neutral-admission mode and retain stdout, stderr, exit status, path/mode/content checks, and before/after forbidden-identity scans as validation evidence in the neutral root. If syntax or neutral admission fails, make at most one reasonable repair while the draft remains neutral, rerun the complete syntax/admission checks, record both attempts, and stop if the second fails.
- After neutral admission passes, audit every quoting-heavy command substitution, inventory exclusion, cleanup target, stage/final identity check, failure trap, and publication line. Run bash -n again. Invoke the draft once without --neutral-admission and require it to refuse its neutral path at the self-identity boundary before any candidate mutation; record that expected refusal and reprove all prospective RC.12 runtime paths absent.
- The full-mode design must preserve exact published source commit a24804bcf2a32ee5434d3686eabad5b72d9f39ba and tree 2c8ee9f6b4283410547a9f99972e25eac06c9e33, clean Go invocation, independent source clones/caches, both Go-version test matrices, vet/module/vulnerability verification, reproducible Linux/Darwin/FreeBSD amd64 artifacts, staged manifests, and mkdir/cp-a final publication with post-copy stage/final identity checks. Do not execute full mode in this pass.
- Seal the successful neutral draft mode 0444 and validation evidence read-only. Record exact root, draft/evidence counts, sizes, modes, SHA-256 values, syntax/admission commands, expected full-mode refusal, and all historical preservation checks in .agent/HANDOFF.md, .agent/STATE.md, and .agent/DECISIONS.md. Keep EXT-20 unchecked.
- Create at most one inert next-pass launcher agent-ext20-rc12-builder-validation-review.sh for independent review of the neutral draft/evidence; do not create agent-ext20-rc12.sh and do not execute the review launcher. Stop after this neutral validation task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
