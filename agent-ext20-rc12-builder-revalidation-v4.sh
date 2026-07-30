#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$ROOT"

fail() {
	printf '%s\n' "$1" >&2
	exit 1
}

PREFLIGHT_ONLY=0
if [[ "$#" == 1 && "$1" == --preflight-only ]]; then
	PREFLIGHT_ONLY=1
elif [[ "$#" != 0 ]]; then
	printf 'Usage: %s [--preflight-only]\n' "$0" >&2
	exit 64
fi

[[ -f .agent/LOOP_PROMPT.md ]] || fail 'Missing .agent/LOOP_PROMPT.md'

git fetch --no-tags origin main
[[ -z "$(git status --porcelain=v1 --untracked-files=all)" ]] ||
	fail 'Fourth builder revalidation requires a clean published controller repository'

SOURCE_COMMIT="a24804bcf2a32ee5434d3686eabad5b72d9f39ba"
SOURCE_TREE="2c8ee9f6b4283410547a9f99972e25eac06c9e33"
HEAD_COMMIT="$(git rev-parse HEAD)"
FETCHED_MAIN="$(git rev-parse refs/remotes/origin/main)"
PUBLIC_MAIN="$(git ls-remote --heads origin refs/heads/main | awk '{print $1}')"
[[ "$HEAD_COMMIT" == "$FETCHED_MAIN" && "$HEAD_COMMIT" == "$PUBLIC_MAIN" ]] ||
	fail 'Fourth builder revalidation requires exact local, fetched, and public main'
git merge-base --is-ancestor "$SOURCE_COMMIT" HEAD ||
	fail 'Candidate source is not an ancestor of controller HEAD'
[[ "$(git rev-parse "$SOURCE_COMMIT^{tree}")" == "$SOURCE_TREE" ]] ||
	fail 'Candidate source tree identity changed'
git diff --quiet "$SOURCE_COMMIT"..HEAD -- .agent/profiles cmd internal go.mod go.sum ||
	fail 'Product source changed after the candidate source commit'

require_sha256() {
	local expected="$1"
	local path="$2"
	[[ -f "$path" && ! -L "$path" ]] ||
		fail "Required immutable file is absent or unsafe: $path"
	[[ "$(sha256sum "$path" | awk '{print $1}')" == "$expected" ]] ||
		fail "Immutable file hash changed: $path"
}

require_sha256 \
	"591a79098ced60f9aa0abbd11f66cf722b2adba22b1fd277237e0890aa536ee5" \
	agent-ext20-rc12-builder-revalidation-v3.sh
require_sha256 \
	"3bafb2b55cde7a872e5b159f3fc9e721d39942b208f83718875721d45dca888d" \
	agent-ext20-rc12-builder-revalidation-v3-review.sh
bash -n agent-ext20-rc12-builder-revalidation-v3.sh
bash -n agent-ext20-rc12-builder-revalidation-v3-review.sh

LOST_VALIDATION_ROOT="/tmp/revolvr-builder-revalidation-v3.PKfbRl"
[[ ! -e "$LOST_VALIDATION_ROOT" && ! -L "$LOST_VALIDATION_ROOT" ]] ||
	fail 'The terminally lost third validation path unexpectedly reappeared; refuse ambiguous recovery'

[[ -d .revolvr && ! -L .revolvr ]] ||
	fail 'Persistent .revolvr parent is absent or unsafe'
[[ "$(realpath -e .revolvr)" == "$ROOT/.revolvr" ]] ||
	fail 'Persistent .revolvr parent resolves outside the controller repository'
[[ "$(stat -c '%u' .revolvr)" == "$(id -u)" ]] ||
	fail 'Persistent .revolvr parent is not owned by the current user'
REVOLVR_MODE="$(stat -c '%a' .revolvr)"
(( (8#$REVOLVR_MODE & 0022) == 0 )) ||
	fail 'Persistent .revolvr parent is group- or other-writable'
if find .revolvr -maxdepth 2 -name 'prospective-builder-revalidation-v4.*' -print -quit | grep -q .; then
	fail 'Fourth persistent neutral builder-revalidation root collision'
fi
if find .revolvr -maxdepth 6 \( -iname '*rc12*' -o -iname '*rc.12*' \) -print -quit | grep -q .; then
	fail 'Prospective RC.12 runtime identity already exists'
fi
for forbidden_path in \
	agent-ext20-rc12.sh \
	agent-ext20-rc12-local-review.sh \
	agent-ext20-rc12-builder-revalidation-v4-review.sh \
	.github/workflows/level1-rc12-candidate-attestation.yml
do
	[[ ! -e "$forbidden_path" && ! -L "$forbidden_path" ]] ||
		fail "Prospective RC.12 path collision: $forbidden_path"
done
if git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12 ||
	git show-ref --verify --quiet refs/heads/level1-v0.1.0-rc.12-attestation; then
	fail 'Local RC.12 ref collision'
fi
[[ -z "$(git ls-remote --heads origin refs/heads/level1-v0.1.0-rc.12 refs/heads/level1-v0.1.0-rc.12-attestation)" ]] ||
	fail 'Remote RC.12 ref collision'
[[ -z "$(git tag --list '*rc.12*')" ]] || fail 'Local RC.12 tag collision'
[[ -z "$(git ls-remote --tags origin '*rc.12*')" ]] || fail 'Remote RC.12 tag collision'

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
[[ "$(clean_go "$RELEASE_GO" version)" == "go version go1.26.5 linux/amd64" ]] ||
	fail 'Release Go version changed'
[[ "$(clean_go "$SOURCE_GO" version)" == "go version go1.22.12 linux/amd64" ]] ||
	fail 'Source Go version changed'

if [[ "$PREFLIGHT_ONLY" == 1 ]]; then
	printf 'Fourth persistent neutral builder-revalidation preflight passed\n'
	exit 0
fi

PROMPT="$(cat .agent/LOOP_PROMPT.md)

Additional operator direction for one bounded EXT-20 prospective RC.12 volatile-evidence recovery pass:
- Never use gh. Use raw Git for reads. Do not commit or push. Do not create or execute an RC.12 builder, candidate, construction launcher, preflight/build/stage/diagnostic root, artifact, bundle, ref, workflow, tag, suite, launch record, Revolvr/model operation, release, or external-use action.
- Do exactly one task: independently author and neutrally validate a fourth anonymous prospective builder design. The third sealed root /tmp/revolvr-builder-revalidation-v3.PKfbRl was externally removed before review. Treat that loss and every older missing /tmp validation root as terminal. Do not recreate their paths, reconstruct their bytes, read old Codex transcripts, or claim their filesystem evidence survives. Durable summaries are historical claims and requirements, not a substitute for the lost bytes.
- Create the new unique evidence root only beneath $ROOT/.revolvr using the template prospective-builder-revalidation-v4.XXXXXX. This root is persistent ignored runtime state, contains no RC.12 identity, and must be the sole retained source for later review. Never retain the authoritative draft or evidence only in /tmp. Use /tmp only for fully cleaned semantic probes.
- Independently reimplement the design from requirements; do not copy or derive implementation bytes from historical builders or surviving launcher prompt text. Before validation sequence one, remeasure every available historical count/hash/absence constant read-only against durable tracked and persistent evidence. Explicitly record which former /tmp evidence cannot be remeasured and preserve its absence rather than inventing authority.
- Preserve the corrected role model: full mode requires and hashes the exact read-only builder, separately tracked construction launcher, and this fourth persistent validation root; permits tracked validation-history and recovery launchers; and rejects actual candidate/verification, post-candidate review, remote/local publication, construction, runtime, suite, and launch collisions. Lost prior /tmp roots are recorded absences, never required inputs.
- Correct all four controller concerns before sequence one: capture the verification inventory only after final sealing (or compare a rigorously normalized invariant that cannot differ because of sealing); check exact .revolvr/ext20-rc12 as well as dotted/glob descendants; preserve every recorded RC.6-RC.11 runtime/root/ref/tag/glob absence still applicable; and check every candidate, verification, and attestation Actions artifact name from the prior contract.
- Neutral cleanup must accept only its active exact /tmp/revolvr-neutral-publication.XXXXXX root, reject symlinks, restore owner write permission to every directory depth-first, delete only that tree depth-first, prove absence, and never hide cleanup failure. Exercise both successful cp -a publication and an induced pre-copy failure through at least two sealed nested levels containing mode-0400 files.
- The semantic publication probe must write only beneath writable parents, seal all files/directories, create the destination separately, use only cp -a source/. destination/., and prove bytes, complete inventories, modes, single links, distinct inodes, no symlinks, and cleanup. Never use rm -rf, rename publication, hard links, or symlink publication.
- Retain two non-local shallow exact-source fetches at commit $SOURCE_COMMIT and tree $SOURCE_TREE, excluding later controller objects and launchers. Retain executable build instructions; clean source/controller/tool/environment identity; both Go test/race/vet/module matrices; ordinary and verbose vulnerability results; reproducibility; exact target/CGO/VCS/build-ID metadata; complete manifests/inventories/hashes; and available-history evidence.
- Final construction publication remains unexecuted design. It must use exact mkdir, set appeared state only after mkdir succeeds, copy with cp -a, restore sealed stage/root modes, and compare complete post-seal stage/final manifests, inventories, counts, hashes, modes, links, distinct inodes, and extra entries. Any failure after appearance is terminal, with no rename/link/symlink fallback and no final-path cleanup.
- The anonymous draft must expose --neutral-admission and --neutral-full-context-audit before exact self-identity enforcement. Run two complete validation sequences: bash -n, neutral admission, neutral full-context audit, focused static audit, expected no-argument status-64 refusal, forbidden-identity/residue scans, and available-history preservation. Permit at most one neutral repair after sequence one; sequence two must pass unchanged or reject the draft.
- Never run product tests/builds or full mode. Seal files mode 0444 and the persistent evidence root mode 0500 with a self-verifying manifest. Record exact root, draft, manifest, stream, modes, counts, both sequence results, volatile-loss boundary, and terminal result in .agent/HANDOFF.md, .agent/STATE.md, .agent/DECISIONS.md, and EXT-20 current-gate text. Keep EXT-20 unchecked.
- Only if both sequences pass, create one inert agent-ext20-rc12-builder-revalidation-v4-review.sh with an explicit --preflight-only mode and named diagnostics for every guard. It must review the persistent root without executing the draft or creating continuation authority. Stop after this fourth neutral revalidation task."

env -u GOROOT -u GOTOOLDIR -u GOFLAGS \
	GOENV=off \
	GOTOOLCHAIN=local \
	codex exec \
	--dangerously-bypass-approvals-and-sandbox \
	--cd "$ROOT" \
	"$PROMPT"
