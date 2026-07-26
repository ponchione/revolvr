# Agent Handoff

Updated: 2026-07-26

## Resume Point

The prospective candidate-construction procedure passed its anonymous neutral
validation gate without creating a candidate identity. Sealed root
`/tmp/revolvr-builder-validation.maYqgv` is mode `0500` and contains exactly
40 regular files: one mode-`0444` neutral draft plus 39 mode-`0444` evidence
files.

Neutral draft
`/tmp/revolvr-builder-validation.maYqgv/candidate-construction-draft.sh` is
24,352 bytes, 575 lines, and SHA-256
`71b196d77b6eb89157492609b89e51d0c56a4e418b10b0f28ed43c94d5a4210d`.
Its path, filename, and bytes contain none of the forbidden prospective
identity literals. The 39 evidence files total 37,123,439 bytes. Manifest
`evidence-manifest.tsv` is mode `0444`, 3,695 bytes, 38 lines, and SHA-256
`2ae2f598dffd37f333c672f492438a81fb346c896964c0107d063423a515ae85`;
it records and verifies every other evidence file's exact name, mode, size,
and SHA-256. Its own exact identity is recorded here.

Initial `bash -n` and `--neutral-admission` both exited `0`. A focused audit
then made the sole neutral repair: all Go metadata invocations now use the
clean environment, the binary check uses `--version`, the second build cache
is explicit, and ERR/signal propagation is complete. The entire syntax and
neutral-admission sequence passed again. A final `bash -n` exited `0`; the
draft's one no-argument invocation exited expected status `64` at its exact
self-identity boundary before any candidate mutation.

The semantic probe wrote under mode-`0700` source parents, changed the value
file to `0400`, sealed source/nested to `0500`, used only `mkdir` plus `cp -a
source/. destination/.`, made the destination root `0500`, proved exact bytes
and all six modes, proved distinct single-link inodes and no symlinks, made
both parents writable, removed only its unique neutral probe, and proved no
residue. The draft full-mode design retains exact source commit
`a24804bcf2a32ee5434d3686eabad5b72d9f39ba` and tree
`2c8ee9f6b4283410547a9f99972e25eac06c9e33`, clean exact Go tools,
independent clones/caches, both Go-version ordinary/race matrices, vet/module/
vulnerability checks, reproducible Linux/Darwin/FreeBSD amd64 artifacts,
staged manifests, and `mkdir`/`cp -a` final publication with stage/final
identity checks. Full mode was not run.

Before/after preservation inventories are byte-identical: 57,135 metadata
entries at SHA-256
`b4ea4ce169e5ec575c52d579aff6d868dbe8069245c556adb6a93ab301537360`
and 52,870 file-content entries at SHA-256
`15f89741ef58e4e6b7f1e590a6692bf60669faa91088aca1965228b04a8d6c59`.
They cover complete `.revolvr` history, the recorded RC.8/RC.9 temporary
roots, the RC.11 neutral draft, all pre-existing EXT-20 launchers, and every
Level-1 workflow. RC.10/RC.11 exact builder/draft identities and all recorded
prospective absences also passed. The only prospective-name paths now present
are the pre-existing validation-pass launcher and the new review launcher;
there is no construction launcher, builder, candidate path, preflight/build/
stage root, artifact, bundle, ref, workflow, tag, suite, or launch record.

Executable `agent-ext20-rc12-builder-validation-review.sh` is mode `0755`,
4,583 bytes, 104 lines, and SHA-256
`0cdbde37d6c33404c988b68c2da28fd325c50c277333ba35983bad419a235fbb`.
It has passed `bash -n` but has not been executed.

## Independent Review Result

Controller review verified every sealed evidence-manifest entry, both exact
before/after history comparisons, draft syntax, recorded neutral admissions,
semantic probe evidence, and expected status-64 self-identity refusal. The
limited validation record is published as commit
`7585c40f1a0048d3d7d29267403e0810ca6e352f`.

The draft is rejected for construction. Full mode first requires the exact
builder to exist at lines 564-568, but `require_collision_free` at lines
165-179 then requires that same builder absent. It also rejects the required
construction launcher and the existing validation-review launcher. The
neutral modes never exercise that contradictory context.

Review also found that full mode checks only RC.10/RC.11 identities rather
than complete RC.6-RC.11 before/after preservation; uses a full clone that
contains later controller history instead of an exact shallow fetch; and
omits required bundle build instructions, complete tool/environment identity,
verbose vulnerability evidence, remote artifact/release collision checks,
and full target/CGO embedded-metadata verification. The sealed root remains
valid review evidence but cannot be copied, repaired, or used for RC.12.

## Next Gate

From `/home/gernsback/source/revolvr`, run exactly:

```bash
./agent-ext20-rc12-builder-revalidation.sh
```

The executable launcher SHA-256 is
`9218d873e24214aa7fff9574e1e35ede1e967629947e101e961ad87eb285b4d7`.
It starts one fresh Codex pass to author and validate a second independent
anonymous draft satisfying the review findings. It consumes no RC.12 identity
and cannot run full construction.

This command grants no builder/candidate construction, publication, remote
CI, attestation, suite, live-model, dogfood, tag, release, external-use,
recovery, queue, or `EXT-20` completion authority. `EXT-20` remains unchecked.

## Previous Resume Point

The sole authorized `level1-v0.1.0-rc.11` local-construction attempt failed
closed during its exact builder's one execution, before RC.11 preflight or
build-root creation.

Neutral draft `/tmp/revolvr-builder-draft.cZLxf2/builder.sh` is mode `0444`,
35,599 bytes, 634 lines, and SHA-256
`c92b7611028cf54abe37735c44fb116826193abd97673c1d69f8747f1b6f7355`.
Exact ignored builder
`.revolvr/release-candidates/build-level1-v0.1.0-rc.11.sh` is byte-identical,
mode `0555`, and passed `bash -n` before its sole execution.

At line 294 the builder created neutral probe directory `source/nested` with
mode `0500`; line 295 then received `Permission denied` while creating
`source/nested/value.txt`. The builder made the probe parents writable,
removed only the neutral probe, left no probe path, and exited with
`candidate construction failed: copy-publication probe failed`.

Independent controller review reproduced the draft/builder byte identity,
syntax success, exact write denial, and absent probe residue. The procedure
sealed the nested directory one command too early; the file must be written
while the directory is writable and only then sealed. The accepted failure
record is published as commit
`0bb21ad157d79a2583f58ce385746cfc24713e12`.

The exact builder must remain unchanged and unexecuted. RC.11 is exhausted
and cannot be repaired, retried, completed, relabeled, deleted, mutated,
derived from, or reused. No RC.11 preflight, build/stage root, clone, cache,
diagnostic, artifact, bundle, suite, launch record, workflow, ref, tag, or
local-review launcher exists. Both intended final bundle paths remain absent:

- `.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3`
- `.revolvr/release-candidates/level1-v0.1.0-rc.11-a24804bcf2a3-verification`

## Preserved Authority

Local, fetched, and public `main` were exact at
`846e76199a9ad6c7c286fd3371d670881e5ab2d8`. Candidate source remains only
published commit `a24804bcf2a32ee5434d3686eabad5b72d9f39ba`, tree
`2c8ee9f6b4283410547a9f99972e25eac06c9e33`; its product-source diff through
controller HEAD is empty.

RC.6 / RC.7 suite, launch-record, and terminal-evidence stream hashes remain
respectively
`d619c58b88f9c32380981ab5688e13b0079ff6afc42c77963531cfccd116470b` /
`2de8901e2f2f037627fb400a241a69542d9a6dab2a74acb87595ae70e1d4f2ce` /
`e12757ebbcb10322d020c3347e5cc7ef4fedfb7cd93079efd4b861f577fa3259`
and
`ef031fa8aa3f7849b50551824a9f7c4b8d72e42f19ad5906f32e4aa0d9a1fb3a` /
`deb55229c31197830721f5fc7cff368281451139da0ad52560f29246b91f2e1c` /
`6bce7d6a7edd992ee23e138713bb6e0923d3be9d3c1ffebd0fd2c94ea47fbd9f`.
Both terminal checksum manifest pairs passed before and after. RC.8 and RC.9
files, trees, staged inventories/manifests, and absences remain exact. RC.10
remains exactly its sole 474-line mode-`0664` builder at SHA-256
`229d000616812af01bf001b979b97313d3fb89d18243edb900ab0c4d6f14e8be`.

## Previous Next Gate

From `/home/gernsback/source/revolvr`, run exactly:

```bash
./agent-ext20-rc12-builder-validation.sh
```

The executable launcher SHA-256 is
`9aa31f45fc925e214c180fad8abac262d812f93b795f3a22105ac4d3853820e3`.
It starts one fresh Codex pass to author and validate only an anonymous
prospective RC.12 builder. The draft must pass syntax, a complete semantic
`--neutral-admission` copy probe, static review, and an expected safe refusal
in full mode before any RC.12 identity may be considered.

This command does not create an RC.12 builder or candidate, run product tests
or builds, publish refs/workflows, run a suite or live model operation, commit,
push, tag, release, approve external use, grant recovery/queue authority, or
complete `EXT-20`. RC.6 through RC.11 remain immutable, and `EXT-20` remains
unchecked.
