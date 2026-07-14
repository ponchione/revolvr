# Wide-Sweep Code Audit Problems

Audit date: 2026-07-14

## Scope and method

This audit reviewed the CLI, execution lifecycle, filesystem and Git boundaries,
ledger behavior, task and receipt parsing, autonomous-state contracts, and
repository-map construction. Suspected problems were retained only when they
were directly established from the code or reproduced with a focused probe.

The ordinary suite, race suite, `go vet`, formatting, module verification,
shell syntax checks, supported-platform cross-builds, CLI help, and local smoke
tests passed. `govulncheck` found no reachable vulnerability. A shuffled test
run and its focused repetition exposed AP-03 below. No substantiated general
performance regression was found.

## Summary

| ID | Severity | Area | Problem |
| --- | --- | --- | --- |
| AP-01 | High | Process lifecycle | A successful direct child may leave mutation-capable descendants running |
| AP-02 | High | Filesystem safety | Initialization follows repository-controlled symlinks and can write outside the repository |
| AP-03 | Medium | Ledger/read cancellation | A live-reader retry loses the required SQLite busy evidence |
| AP-04 | Medium | Markdown parsing | Structural headings inside fenced code blocks are treated as real task/receipt headings |
| AP-05 | Medium | Git compatibility | SHA-256 repositories are accepted by one boundary and rejected by the dossier path |
| AP-06 | Low | Repository paths | Valid tracked names beginning with `..` are rejected |
| AP-07 | Low | Determinism | Contract validation returns map-order-dependent diagnostics |
| AP-08 | Low | Code size | Confirmed no-caller production wrappers and a no-caller orchestration path remain |

## AP-01 — successful commands can leave descendants running

**Affected code:** `internal/runner/runner.go:97-129` and
`internal/runner/runner.go:135-166`.

`runner.Run` creates a process group and terminates that group when the context
is cancelled. On the normal path, however, `cmd.Wait` only establishes that the
direct child exited. Closing `commandDone` makes the watcher return without
checking whether the process group still contains descendants. The runner can
therefore report exit code 0 while a background process still has authority to
modify the workspace.

This was reproduced with a temporary regression test whose command was
equivalent to:

```sh
/bin/sh -c '(sleep 0.2; printf late > sentinel) >/dev/null 2>&1 &'
```

The shell exited successfully, `runner.Run` returned success, and the
descendant created `sentinel` afterward. Redirecting the descendant's standard
streams is important: it proves this is not merely `os/exec` waiting for an
inherited pipe.

**Impact:** a Codex command, verification command, or other runner consumer can
appear complete while descendants continue writing. Those writes can occur
after verification, commit construction, terminal receipt/ledger mutation, or
source-lease release, undermining the lifecycle guarantees those layers are
intended to provide.

**Required correction:** settle the whole process group on every exit path, not
only cancellation. After the leader exits, inspect the original group and, if
members remain, terminate and join it before returning. A leftover descendant
should not be classified as an ordinary successful command. The implementation
must avoid signalling a reused process-group identity and must preserve the
existing cancellation/deadline result semantics.

**Regression coverage:** run a direct child that redirects all inherited
streams, backgrounds a delayed writer, and exits 0. Assert that the runner
cannot return success while the writer remains and that no delayed mutation is
possible after return. Cover both natural exit and cancellation.

## AP-02 — initialization can write outside the repository through symlinks

**Affected code:** `internal/cli/state.go:52-107`,
`internal/cli/state.go:118-174`, and the create paths in
`internal/taskfile/taskfile.go:774-783`.

Initialization uses `os.MkdirAll`, `os.Stat`, `os.WriteFile`, and the normal
ledger opener on paths below `.revolvr`, `.agent`, and `.git`. Those operations
follow symlinks. A temporary-repository probe created `.revolvr` as a symlink
to an external directory and then ran `revolvr init`. The command exited 0 and
created all of these outside the repository:

```text
runs/
receipts/
locks/
ledger.sqlite
```

The `.agent/profiles` and `.agent/tasks` creation paths have the same class of
problem. `writeNewTaskFile` calls `MkdirAll` before its resolved-path validation,
so an escaping `.agent` symlink can cause an external `tasks` directory to be
created even though the later file operation is rejected.

`gitExcludePath` also trusts the contents of a repository-controlled `.git`
file as a gitdir path, and `ensureExcludePattern` follows intermediate and final
symlinks. A forged pointer or `info/exclude` symlink can therefore direct an
initialization write elsewhere. Legitimate linked worktrees can have an admin
directory outside the worktree, so simply requiring every Git-admin path to be
beneath the worktree would break valid repositories.

There is also a linked-worktree correctness error in this same path. In a real
fixture, the `.git` file named `<main>/.git/worktrees/<name>`, so the current
code selected `<main>/.git/worktrees/<name>/info/exclude`. Git itself reported
`<main>/.git/info/exclude` from `git rev-parse --git-path info/exclude`. Thus the
current initialization can safely return after editing a file Git does not use.

**Impact:** running an ostensibly repository-scoped initialization on an
untrusted or damaged checkout can create or overwrite data at attacker-chosen
locations available to the current user.

**Required correction:** canonicalize the worktree identity once and create
each protected component with no-follow, identity-checked operations (the
existing hardened runtime-path primitives are the natural starting point).
Reject symlink, non-directory, unsafe-permission, and substitution cases before
the first side effect. Resolve Git administrative paths through Git's own
reported admin/common-directory identity, while protecting the opened
`info/exclude` identity against symlink or replacement attacks. Validate task
directory containment before `MkdirAll` and use a no-follow create path.

**Regression coverage:** for `.revolvr`, `.agent`, the final profile/task file,
`.git`, and `info/exclude`, point one component at an external sentinel
directory. Assert a failure and an unchanged external tree. Include a genuine
linked-worktree case to retain supported Git behavior.

## AP-03 — live-reader cancellation drops SQLite busy evidence

**Affected code:** `internal/ledger/store.go:750-780` and
`internal/ledger/read_only_test.go:215`.

`retryLiveRead` joins the context error with a SQLite busy error only when the
context is observed immediately after a busy attempt or while its retry timer
is active. It does not retain busy evidence across attempts. If an earlier
attempt returns `SQLITE_BUSY`, then the context expires during the next
`operation()` and that operation returns only `context deadline exceeded`, the
early return at the top of the loop discards the earlier SQLite cause.

This is not hypothetical. The shuffled suite failed at seed
`1784054248504186763`, and a focused invocation configured for twenty
repetitions of
`TestLiveReadOnlyCancellationInterruptsBusyRollbackReaderAndReopens` also
failed:

```text
busy read error = read ledger snapshot: runs: context deadline exceeded,
want retained SQLite busy evidence
```

That contradicts the established live-read contract and makes the test suite
order/timing sensitive.

**Required correction:** retain the most recent busy/locked error for the
entire retry operation. If a later attempt or context check terminates with
cancellation/deadline, return a zero value and an error joining the context
cause with the retained SQLite evidence. Do not attach busy evidence when no
busy attempt occurred.

**Regression coverage:** add a deterministic retry seam or table test that
returns busy once and context expiration on the next call. Assert both causes
with `errors.Is`/`errors.As`, the zero result, and successful reopening. Repeat
the focused test and the shuffled suite.

## AP-04 — fenced Markdown headings are parsed as document structure

**Affected code:** `internal/taskimport/parser.go:89-139`,
`internal/receipt/parser.go:228-241`, and
`internal/receipt/claims.go:180-196`.

These scanners recognize headings line by line but do not track fenced code
blocks. A temporary CLI fixture containing one task and this example in its
body was reported and persisted as two tasks:

````markdown
## Task: parser
Document this example:

```markdown
## Task: not a real task
example body
```
````

`revolvr task import` printed `Imported 2 task(s)`. The temporary task files
were removed after the probe. The receipt scanners have the same parsing
model, so a level-two heading in a code sample can incorrectly satisfy a
required-section check or switch the section from which claims are extracted.

**Impact:** ordinary documentation examples can split an import and create
bogus persisted tasks. Receipt structure and derived claim warnings can also
be computed from example text rather than actual sections.

**Required correction:** make structural scanning fence-aware. Backtick and
tilde fences, closing-fence length, indentation accepted by the supported
Markdown format, and unclosed fences should be handled consistently. Content
inside a fence must remain byte-preserved but structurally inert.

**Regression coverage:** cover fenced `## Task`, `## Changed Files`, and
`## Verification` headings; both backtick and tilde fences; longer outer
fences; an unclosed fence; and normal headings immediately after a closing
fence. The import regression must assert that exactly one task is produced.

## AP-05 — Git SHA-256 object IDs are only partially supported

**Affected code:** acceptance in `internal/autonomous/workspace.go:157-163`
and `internal/autonomous/state.go:1888-1891`; rejection in
`internal/dossiercache/cache.go:117-120`,
`internal/dossiercache/map.go:45-52`, and
`internal/autonomous/dossier_projection.go:228-235`.

The workspace/state boundaries explicitly accept lowercase 40- or 64-character
Git object IDs. The dossier cache and projection path still require exactly 40
characters, including the object ID parsed from `git ls-tree`. Git 2.43 in the
audit environment successfully created a SHA-256 repository whose commit,
tree, and listed object IDs are all 64 characters.

**Impact:** a valid SHA-256 repository passes initial workspace validation but
fails later while constructing the autonomous dossier. This is a delayed and
internally contradictory compatibility failure; the README does not declare a
SHA-1-only restriction.

**Required correction:** centralize a strict lowercase Git-OID validator that
accepts the supported 40- and 64-character forms, and use it at every boundary,
including `ls-tree` parsing and dossier projection. If SHA-256 is intentionally
unsupported, the alternative is to reject it at the first workspace boundary
and document that restriction; the present mixed contract is the bug.

**Regression coverage:** validate 40- and 64-character IDs, reject invalid
length/case/non-hex values, parse a 64-character `ls-tree -z` record, and run a
repository-map build against a real `git init --object-format=sha256` fixture
when the installed Git supports it.

## AP-06 — valid paths beginning with `..` are over-rejected

**Affected code:** `internal/dossiercache/map.go:63-70` and
`internal/dossiercache/cache.go:122-128`.

Both validators reject `strings.HasPrefix(path, "..")`. That catches traversal,
but also valid repository-relative names such as `..foo` and
`..well-known/file`. Git permits those names. Other packages in this repository
already use the correct component-aware form: reject exact `..` or a cleaned
path beginning with `../` (using the platform separator where appropriate).

**Impact:** one valid tracked filename can prevent autonomous repository-map
and dossier construction.

**Required correction:** replace the textual prefix test with a canonical,
component-aware traversal check. Preserve the existing absolute-path,
normalization, NUL, ordering, and duplicate checks.

**Regression coverage:** accept `..foo` and `..well-known/file`; reject `..`,
`../foo`, `a/../../b`, absolute paths, and noncanonical equivalents.

## AP-07 — validation diagnostics depend on Go map iteration order

**Affected code:** `internal/gitstate/source_snapshot.go:280-291`,
`internal/dossiercache/cache.go:112-116`, and
`internal/autonomousaudit/contracts.go:209-234`.

The first two validators range over map literals and return on the first
invalid field. When multiple fields are bad, which field is reported is not
defined by Go. The audit transition similarly ranges `statusByID` and reports
the first open finding missing from the new report. With multiple missing
findings, the diagnostic ID is nondeterministic.

This does not corrupt state, but it conflicts with the codebase's deterministic
contract style and makes exact diagnostics, fixtures, and operator triage vary
between runs.

**Required correction:** validate fixed fields through an ordered slice. For
finding maps, collect and sort missing IDs before selecting or reporting them.

**Regression coverage:** construct inputs with multiple simultaneous failures
and assert one stable diagnostic over repeated runs.

## AP-08 — confirmed no-caller production code remains

**Affected code:** `internal/app/autonomous_run.go:987`,
`internal/autonomousnotification/store.go:76-78`,
`internal/autonomousnotification/store.go:365-367`, and
`internal/autonomoustaskrun/admitted_cycle.go`.

Repository-wide Go reference searches found no call to the private `bytesSHA`,
`persistTransition`, or `replaceFile` wrappers. Their fault-injecting
counterparts are the functions actually used. The 74-line admitted-cycle file
defines `AdmittedCycleConfig`, `AdmittedCycleResult`, and `RunAdmittedCycle`,
but none has a caller or test anywhere in the repository. Current production
orchestration uses other paths.

This is not a correctness defect. It is a small, evidence-backed LOC and
maintenance reduction opportunity, recorded separately so it is not confused
with the operational findings above.

**Required correction:** remove the three private wrappers. Remove the
no-caller admitted-cycle path if it is not a deliberately reserved internal
API; because it is under `internal/`, it is not a supported external Go API.
Run the full suite to let the compiler prove no current consumer was missed.

**Regression coverage:** no new behavior test is needed for deleted no-op
surface. Existing autonomous lifecycle tests and `go test ./...` must remain
green.
