package autonomous

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomousverification"
)

func TestBuildTaskDossierFullSnapshot(t *testing.T) {
	result, err := BuildTaskDossier(fullDossierInput())
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}

	// These fixed digests and sizes are exact byte snapshots. The content and
	// section-order assertions below make an intentional snapshot update easy
	// to review without storing a second copy of a large dossier fixture.
	if got, want := sha256HexBytes(result.Markdown), "d65fcaec1cd7267f43a02f9f255bd1dadb994776721f983a4115da4a060ef827"; got != want {
		t.Fatalf("Markdown SHA-256 = %q, want %q (byte size %d)", got, want, len(result.Markdown))
	}
	if got, want := len(result.Markdown), 6385; got != want {
		t.Fatalf("Markdown byte size = %d, want %d", got, want)
	}

	manifestJSON, err := MarshalTaskDossierManifest(result.Manifest)
	if err != nil {
		t.Fatalf("MarshalTaskDossierManifest() error = %v", err)
	}
	if got, want := sha256HexBytes(manifestJSON), "36fced1a1694d31c994d4587ddd454bbce19f44330fd7e95e41ff8e08963d92a"; got != want {
		t.Fatalf("manifest JSON SHA-256 = %q, want %q\n%s", got, want, manifestJSON)
	}
	if got, want := len(manifestJSON), 2349; got != want {
		t.Fatalf("manifest JSON byte size = %d, want %d", got, want)
	}
	if manifestJSON[len(manifestJSON)-1] != '\n' {
		t.Fatalf("manifest JSON does not end with newline: %q", manifestJSON)
	}

	assertOrderedSubstrings(t, string(result.Markdown), []string{
		"# Autonomous Task Dossier",
		"## Dossier Identity",
		"## Canonical Task/Spec",
		"## Current Autonomous State",
		"## Current Plan",
		"## Acceptance Criteria",
		"## Verification",
		"## Audit and Finding Resolutions",
		"## Recent Runs",
		"## Git Snapshot",
		"## Repository Guidance",
		"## Omissions and Truncation",
	})
	for _, want := range []string{
		"- Lifecycle: correcting",
		"- Plan progress: 2/3 terminal steps; revision plan-002 (2); completed=false",
		"### Step 1: inspect-contracts",
		"### Criterion 2: criterion-waived",
		"- Audit disposition: changes_required",
		"### Finding 1: finding-001",
		"- Current resolution: resolved",
		"### Run 1: run-newest",
		"Omitted 1 older run(s) due to the history limit.",
		"### Guidance 1: AGENTS.md",
		"### Guidance 2: docs/AGENTS.md",
		"- recent_runs: history limit retained 2 of 3 items and omitted 1.",
	} {
		if !bytes.Contains(result.Markdown, []byte(want)) {
			t.Fatalf("Markdown missing %q:\n%s", want, result.Markdown)
		}
	}
}

func TestBuildTaskDossierRendersTieredFlakyAttempts(t *testing.T) {
	in := minimalDossierInput()
	summary := testVerificationSummary()
	gate := autonomousverification.GateEvidence{
		SchemaVersion: autonomousverification.GateSchemaVersion,
		Plan:          autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: strings.Repeat("a", 64), ByteSize: 12},
		Purpose:       autonomousverification.PurposeFinal, RequiredFinalTiers: []string{"focused"}, SelectedTiers: []string{"focused"}, ExecutedTiers: []string{"focused"},
		RequiredOutcomes: []autonomousverification.TierGate{{TierID: "focused", Outcome: autonomousverification.OutcomeFlaky}}, OverallOutcome: autonomousverification.OutcomeFlaky,
	}
	identity := autonomousverification.CommandIdentity{PlanSHA256: gate.Plan.SHA256, Purpose: gate.Purpose, TierID: "focused", TierKind: autonomousverification.TierFocused, Name: "go", Args: []string{"test", "./internal/autonomous"}}
	identity.SHA256 = autonomousverification.CommandMaterialSHA256(identity)
	tiered := autonomousverification.Result{
		SchemaVersion: autonomousverification.ResultSchemaVersion, TaskID: summary.TaskID, RunID: summary.RunID, OccurrenceID: summary.OccurrenceID,
		SourceRevision: strings.Repeat("c", 64), Plan: gate.Plan, Purpose: gate.Purpose, Outcome: autonomousverification.OutcomeFlaky, Gate: gate,
		Tiers: []autonomousverification.TierResult{{ID: "focused", Kind: autonomousverification.TierFocused, RequiredForFinal: true, Outcome: autonomousverification.OutcomeFlaky,
			Commands: []autonomousverification.CommandResult{{Identity: identity, Outcome: autonomousverification.OutcomeFlaky,
				Attempts: []autonomousverification.Attempt{
					{AttemptID: "attempt-one", Number: 1, Command: identity, Outcome: autonomousverification.OutcomeFailed, ExitCode: 1, Stdout: autonomousverification.Output{TruncatedBytes: 2}},
					{AttemptID: "attempt-two", Number: 2, Command: identity, Outcome: autonomousverification.OutcomePassed, Passed: true, ExitCode: 0, Stderr: autonomousverification.Output{TruncatedBytes: 3}},
				}}}}},
	}
	summary.Status = VerificationStatusFailed
	summary.Tiered = &tiered
	in.Verification = &summary
	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatal(err)
	}
	text := string(result.Markdown)
	for _, want := range []string{"Purpose: final", "Final gate satisfied: false", "Verification Tier: focused", "Outcome: flaky", "Attempt 1 (attempt-one): failed", "Attempt 2 (attempt-two): passed", "stdout_truncated=2", "stderr_truncated=3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dossier missing %q:\n%s", want, text)
		}
	}
}

func TestBuildTaskDossierIsDeterministicAndDoesNotMutateInput(t *testing.T) {
	in := fullDossierInput()
	taskBytes := append([]byte(nil), in.TaskSpec.Content...)
	runs := append([]RecentRunSummary(nil), in.RecentRuns...)
	guidance := append([]GuidanceSource(nil), in.Guidance...)
	guidanceBytes := make([][]byte, len(in.Guidance))
	for i := range in.Guidance {
		guidanceBytes[i] = append([]byte(nil), in.Guidance[i].Content...)
	}
	stateJSON, err := json.Marshal(in.State)
	if err != nil {
		t.Fatalf("json.Marshal(state) error = %v", err)
	}

	first, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("first BuildTaskDossier() error = %v", err)
	}
	second, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("second BuildTaskDossier() error = %v", err)
	}
	if !bytes.Equal(first.Markdown, second.Markdown) {
		t.Fatal("repeated Markdown differs")
	}
	firstManifest, err := MarshalTaskDossierManifest(first.Manifest)
	if err != nil {
		t.Fatalf("first manifest marshal error = %v", err)
	}
	secondManifest, err := MarshalTaskDossierManifest(second.Manifest)
	if err != nil {
		t.Fatalf("second manifest marshal error = %v", err)
	}
	if !bytes.Equal(firstManifest, secondManifest) {
		t.Fatal("repeated manifest JSON differs")
	}
	if got, want := first.Manifest.DossierSHA256, sha256HexBytes(first.Markdown); got != want {
		t.Fatalf("dossier SHA-256 = %q, want %q", got, want)
	}
	if got, want := first.Manifest.DossierByteSize, len(first.Markdown); got != want {
		t.Fatalf("dossier byte size = %d, want %d", got, want)
	}

	if !bytes.Equal(in.TaskSpec.Content, taskBytes) {
		t.Fatalf("task bytes mutated: %q, want %q", in.TaskSpec.Content, taskBytes)
	}
	if !reflect.DeepEqual(in.RecentRuns, runs) {
		t.Fatalf("recent runs mutated:\ngot  %#v\nwant %#v", in.RecentRuns, runs)
	}
	if !reflect.DeepEqual(in.Guidance, guidance) {
		t.Fatalf("guidance slice mutated:\ngot  %#v\nwant %#v", in.Guidance, guidance)
	}
	for i := range in.Guidance {
		if !bytes.Equal(in.Guidance[i].Content, guidanceBytes[i]) {
			t.Fatalf("guidance[%d] bytes mutated", i)
		}
	}
	afterStateJSON, err := json.Marshal(in.State)
	if err != nil {
		t.Fatalf("second json.Marshal(state) error = %v", err)
	}
	if !bytes.Equal(afterStateJSON, stateJSON) {
		t.Fatalf("execution state mutated:\ngot  %s\nwant %s", afterStateJSON, stateJSON)
	}

	markdown := string(first.Markdown)
	assertOrderedSubstrings(t, markdown, []string{"### Run 1: run-newest", "### Run 2: run-middle"})
	if strings.Contains(markdown, "### Run 3: run-oldest") {
		t.Fatalf("truncated run was rendered:\n%s", markdown)
	}
	assertOrderedSubstrings(t, markdown, []string{"### Guidance 1: AGENTS.md", "### Guidance 2: docs/AGENTS.md"})
}

func TestTaskDossierSourceProvenance(t *testing.T) {
	in := fullDossierInput()
	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}

	task := dossierSource(t, result.Manifest, DossierSourceKindTaskSpec, "task-spec")
	assertRawSourceRecord(t, task, in.TaskSpec.Content)
	if got, want := task.Path, ".agent/tasks/task-1.md"; got != want {
		t.Fatalf("task path = %q, want %q", got, want)
	}

	state := dossierSource(t, result.Manifest, DossierSourceKindExecutionState, "execution-state")
	stateJSON, err := json.Marshal(in.State)
	if err != nil {
		t.Fatalf("json.Marshal(state) error = %v", err)
	}
	assertTypedSourceRecord(t, state, stateJSON)

	verificationJSON, err := json.Marshal(*in.Verification)
	if err != nil {
		t.Fatalf("json.Marshal(verification) error = %v", err)
	}
	assertTypedSourceRecord(t, dossierSource(t, result.Manifest, DossierSourceKindVerification, "current-verification"), verificationJSON)

	auditJSON, err := json.Marshal(*in.Audit)
	if err != nil {
		t.Fatalf("json.Marshal(audit) error = %v", err)
	}
	assertTypedSourceRecord(t, dossierSource(t, result.Manifest, DossierSourceKindAudit, "latest-audit"), auditJSON)

	sortedRuns, err := validateAndSortRecentRuns(in.RecentRuns, in.TaskID)
	if err != nil {
		t.Fatalf("validateAndSortRecentRuns() error = %v", err)
	}
	runsJSON, err := json.Marshal(sortedRuns)
	if err != nil {
		t.Fatalf("json.Marshal(runs) error = %v", err)
	}
	runs := dossierSource(t, result.Manifest, DossierSourceKindRecentRuns, "recent-runs")
	assertTypedSourceRecord(t, runs, runsJSON)
	if runs.Items == nil || *runs.Items != (DossierItemCounts{Total: 3, Included: 2, Omitted: 1}) {
		t.Fatalf("recent run counts = %#v, want 3/2/1", runs.Items)
	}
	if !runs.Truncated {
		t.Fatal("recent run source truncated = false, want true")
	}

	gitJSON, err := json.Marshal(*in.Git)
	if err != nil {
		t.Fatalf("json.Marshal(git) error = %v", err)
	}
	assertTypedSourceRecord(t, dossierSource(t, result.Manifest, DossierSourceKindGitSnapshot, "git-snapshot"), gitJSON)

	for _, source := range in.Guidance {
		record := dossierSource(t, result.Manifest, DossierSourceKindRepositoryGuidance, source.ID)
		assertRawSourceRecord(t, record, source.Content)
	}
	if got, want := result.Manifest.DossierSHA256, sha256HexBytes(result.Markdown); got != want {
		t.Fatalf("dossier SHA-256 = %q, want %q", got, want)
	}
	if got, want := result.Manifest.DossierByteSize, len(result.Markdown); got != want {
		t.Fatalf("dossier byte size = %d, want %d", got, want)
	}
}

func TestTaskDossierRawSourceHashesAndBoundaryNewlines(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{name: "without trailing newline", content: []byte("first\nsecond")},
		{name: "with trailing newline", content: []byte("first\nsecond\n")},
		{name: "CRLF", content: []byte("first\r\nsecond\r\n")},
	}

	hashes := make(map[string]string)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := sparseDossierInput()
			in.TaskSpec.Content = append([]byte(nil), tt.content...)
			result, err := BuildTaskDossier(in)
			if err != nil {
				t.Fatalf("BuildTaskDossier() error = %v", err)
			}
			record := dossierSource(t, result.Manifest, DossierSourceKindTaskSpec, "task-spec")
			assertRawSourceRecord(t, record, tt.content)
			hashes[tt.name] = record.SHA256

			wantBoundary := append([]byte(nil), tt.content...)
			if !bytes.HasSuffix(wantBoundary, []byte("\n")) {
				wantBoundary = append(wantBoundary, '\n')
			}
			wantBoundary = append(wantBoundary, []byte("\n## Current Autonomous State")...)
			if !bytes.Contains(result.Markdown, wantBoundary) {
				t.Fatalf("Markdown boundary does not preserve source bytes:\n%q\nwant fragment %q", result.Markdown, wantBoundary)
			}
		})
	}
	if hashes["without trailing newline"] == hashes["with trailing newline"] {
		t.Fatal("trailing newline did not change exact source hash")
	}
	if hashes["with trailing newline"] == hashes["CRLF"] {
		t.Fatal("line ending difference did not change exact source hash")
	}
}

func TestBuildTaskDossierSparseInputIsExplicit(t *testing.T) {
	result, err := BuildTaskDossier(sparseDossierInput())
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}
	markdown := string(result.Markdown)
	for _, want := range []string{
		"No current plan is present in the execution state.",
		"No acceptance criteria are present in the execution state.",
		"No verification evidence supplied.",
		"No audit report supplied.",
		"No finding resolutions are present in the execution state.",
		"No recent run summaries supplied.",
		"No Git snapshot supplied.",
		"No repository guidance sources supplied.",
		"- current_plan: not present in execution state.",
		"- repository_guidance: not supplied.",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("sparse Markdown missing %q:\n%s", want, markdown)
		}
	}
	wantFacts := []DossierProjectionFact{
		{Section: "current_plan", Reason: "not_present_in_execution_state"},
		{Section: "verification", Reason: "not_supplied"},
		{Section: "audit", Reason: "not_supplied"},
		{Section: "recent_runs", Reason: "not_supplied"},
		{Section: "git_snapshot", Reason: "not_supplied"},
		{Section: "repository_guidance", Reason: "not_supplied"},
	}
	if !reflect.DeepEqual(result.Manifest.ProjectionFacts, wantFacts) {
		t.Fatalf("projection facts = %#v, want %#v", result.Manifest.ProjectionFacts, wantFacts)
	}
	runs := dossierSource(t, result.Manifest, DossierSourceKindRecentRuns, "recent-runs")
	if runs.Items == nil || *runs.Items != (DossierItemCounts{}) {
		t.Fatalf("empty recent run counts = %#v, want zeros", runs.Items)
	}
}

func TestBuildTaskDossierRendersOptionalRoleOccurrences(t *testing.T) {
	in := sparseDossierInput()
	occurrence := optionalOccurrence(OptionalRoleOutcomeNoChange, WorkerProfileDocumentor)
	occurrence.TaskID = in.TaskID
	occurrence.Decision.TaskID = in.TaskID
	in.State.OptionalRoles = []OptionalRoleOccurrence{occurrence}
	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"### Optional Role Dispositions", "role=documentor outcome=no_change", "attempt=attempt-one worker=worker-run", "audit=audit-worker/1"} {
		if !strings.Contains(string(result.Markdown), want) {
			t.Fatalf("dossier missing %q:\n%s", want, result.Markdown)
		}
	}
}

func TestTaskDossierDistinguishesCleanAuditAndTrackedPriorResolutions(t *testing.T) {
	in := sparseDossierInput()
	report := validAuditReport(AuditDispositionClean)
	in.Audit = &report
	in.State.FindingResolutions = []FindingResolution{
		validFindingResolution("finding-prior", FindingResolutionStatusWaived),
	}
	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}
	markdown := string(result.Markdown)
	for _, want := range []string{
		"- Audit disposition: clean",
		"The latest audit is clean and contains no findings.",
		"### Other Tracked Finding Resolutions",
		"- Finding ID: finding-prior",
		"- Current resolution: waived",
		"- Resolution rationale: The operator accepted the documented non-blocking risk.",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("clean-audit Markdown missing %q:\n%s", want, markdown)
		}
	}
}

func TestBuildTaskDossierRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*TaskDossierInput)
		wantErr string
	}{
		{name: "missing task id", mutate: func(in *TaskDossierInput) { in.TaskID = " " }, wantErr: "build task dossier: task_id is required"},
		{name: "missing task source id", mutate: func(in *TaskDossierInput) { in.TaskSpec.ID = "" }, wantErr: "task_spec.id is required"},
		{name: "missing task content", mutate: func(in *TaskDossierInput) { in.TaskSpec.Content = nil }, wantErr: "task_spec.content is required"},
		{name: "invalid task UTF-8", mutate: func(in *TaskDossierInput) { in.TaskSpec.Content = []byte{0xff} }, wantErr: "task_spec.content is not valid UTF-8"},
		{name: "absolute task path", mutate: func(in *TaskDossierInput) { in.TaskSpec.Path = "/tmp/task.md" }, wantErr: "task_spec.path \"/tmp/task.md\" must be repository-relative"},
		{name: "invalid execution state", mutate: func(in *TaskDossierInput) { in.State.SchemaVersion = "state-v2" }, wantErr: "execution_state: validate execution state: unsupported schema_version"},
		{name: "state task mismatch", mutate: func(in *TaskDossierInput) { in.State.TaskID = "task-2" }, wantErr: `task_id "task-1" does not match execution_state task_id "task-2"`},
		{
			name: "audit task mismatch",
			mutate: func(in *TaskDossierInput) {
				report := validAuditReport(AuditDispositionClean)
				report.TaskID = "task-2"
				in.Audit = &report
			},
			wantErr: `audit task_id "task-2" does not match dossier task_id "task-1"`,
		},
		{
			name: "malformed audit",
			mutate: func(in *TaskDossierInput) {
				report := validAuditReport(AuditDispositionClean)
				report.Inputs = nil
				in.Audit = &report
			},
			wantErr: "audit: validate audit report: inputs requires at least one evidence reference",
		},
		{name: "negative history limit", mutate: func(in *TaskDossierInput) { in.RecentRunLimit = -1 }, wantErr: "recent_run_limit cannot be negative"},
		{
			name: "missing run identity",
			mutate: func(in *TaskDossierInput) {
				in.RecentRuns = []RecentRunSummary{testRecentRun("", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))}
			},
			wantErr: "recent_runs[0].run_id is required",
		},
		{
			name: "run task mismatch",
			mutate: func(in *TaskDossierInput) {
				run := testRecentRun("run-1", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
				run.TaskID = "task-2"
				in.RecentRuns = []RecentRunSummary{run}
			},
			wantErr: `recent_runs[0].task_id "task-2" does not match dossier task_id "task-1"`,
		},
		{
			name: "duplicate run identity",
			mutate: func(in *TaskDossierInput) {
				run := testRecentRun("run-1", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
				in.RecentRuns = []RecentRunSummary{run, run}
			},
			wantErr: `recent_runs[1].run_id duplicates run identity "run-1"`,
		},
		{
			name: "unknown run action",
			mutate: func(in *TaskDossierInput) {
				run := testRecentRun("run-1", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
				run.Action = "review"
				in.RecentRuns = []RecentRunSummary{run}
			},
			wantErr: `recent_runs[0].action has unknown value "review"`,
		},
		{
			name: "unknown run profile",
			mutate: func(in *TaskDossierInput) {
				run := testRecentRun("run-1", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
				run.Profile = "reviewer"
				in.RecentRuns = []RecentRunSummary{run}
			},
			wantErr: `recent_runs[0].profile has unknown value "reviewer"`,
		},
		{
			name: "invalid run evidence",
			mutate: func(in *TaskDossierInput) {
				run := testRecentRun("run-1", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC))
				run.Evidence = []EvidenceReference{{Kind: EvidenceKindLedger}}
				in.RecentRuns = []RecentRunSummary{run}
			},
			wantErr: "recent_runs[0].evidence[0]: reference is required",
		},
		{
			name: "missing verification evidence",
			mutate: func(in *TaskDossierInput) {
				verification := testVerificationSummary()
				verification.Evidence = nil
				in.Verification = &verification
			},
			wantErr: "verification.evidence requires at least one evidence reference",
		},
		{
			name: "unknown verification status",
			mutate: func(in *TaskDossierInput) {
				verification := testVerificationSummary()
				verification.Status = "partial"
				in.Verification = &verification
			},
			wantErr: `verification.status has unknown value "partial"`,
		},
		{
			name: "duplicate guidance identity",
			mutate: func(in *TaskDossierInput) {
				in.Guidance = []GuidanceSource{
					{ID: "rules", Path: "AGENTS.md", Content: []byte("one")},
					{ID: "rules", Path: "docs/AGENTS.md", Content: []byte("two")},
				}
			},
			wantErr: `guidance[1].id duplicates guidance identity "rules"`,
		},
		{name: "missing guidance identity", mutate: func(in *TaskDossierInput) {
			in.Guidance = []GuidanceSource{{Path: "AGENTS.md", Content: []byte("one")}}
		}, wantErr: "guidance[0].id is required"},
		{
			name: "duplicate guidance path",
			mutate: func(in *TaskDossierInput) {
				in.Guidance = []GuidanceSource{
					{ID: "rules-1", Path: "AGENTS.md", Content: []byte("one")},
					{ID: "rules-2", Path: "AGENTS.md", Content: []byte("two")},
				}
			},
			wantErr: `guidance[1].path duplicates guidance path "AGENTS.md"`,
		},
		{name: "missing guidance path", mutate: func(in *TaskDossierInput) { in.Guidance = []GuidanceSource{{ID: "rules", Content: []byte("one")}} }, wantErr: "guidance[0].path is required"},
		{name: "invalid guidance UTF-8", mutate: func(in *TaskDossierInput) {
			in.Guidance = []GuidanceSource{{ID: "rules", Path: "AGENTS.md", Content: []byte{0xff}}}
		}, wantErr: "guidance[0].content is not valid UTF-8"},
		{
			name: "invalid git evidence",
			mutate: func(in *TaskDossierInput) {
				evidence := EvidenceReference{Kind: EvidenceKindGit}
				in.Git = &GitSnapshot{Head: "abc", WorktreeStatus: "clean", DiffSummary: "none", Evidence: &evidence}
			},
			wantErr: "git.evidence[0]: reference is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := sparseDossierInput()
			tt.mutate(&in)
			_, err := BuildTaskDossier(in)
			if err == nil {
				t.Fatal("BuildTaskDossier() error = nil")
			}
			if got := err.Error(); !strings.Contains(got, tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", got, tt.wantErr)
			}
		})
	}
}

func TestTaskDossierRecentRunTruncation(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		included  int
		omitted   int
		truncated bool
	}{
		{name: "below limit", limit: 4, included: 3},
		{name: "exactly at limit", limit: 3, included: 3},
		{name: "above limit", limit: 2, included: 2, omitted: 1, truncated: true},
		{name: "zero limit", limit: 0, included: 0, omitted: 3, truncated: true},
	}

	var fullSourceHash string
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := sparseDossierInput()
			in.RecentRuns = testRecentRuns()
			in.RecentRunLimit = tt.limit
			result, err := BuildTaskDossier(in)
			if err != nil {
				t.Fatalf("BuildTaskDossier() error = %v", err)
			}
			record := dossierSource(t, result.Manifest, DossierSourceKindRecentRuns, "recent-runs")
			wantCounts := DossierItemCounts{Total: 3, Included: tt.included, Omitted: tt.omitted}
			if record.Items == nil || *record.Items != wantCounts {
				t.Fatalf("item counts = %#v, want %#v", record.Items, wantCounts)
			}
			if record.Truncated != tt.truncated {
				t.Fatalf("truncated = %t, want %t", record.Truncated, tt.truncated)
			}
			if fullSourceHash == "" {
				fullSourceHash = record.SHA256
			} else if record.SHA256 != fullSourceHash {
				t.Fatalf("full history hash = %q, want %q", record.SHA256, fullSourceHash)
			}

			markdown := string(result.Markdown)
			if tt.included >= 1 && !strings.Contains(markdown, "### Run 1: run-newest") {
				t.Fatalf("newest run not retained:\n%s", markdown)
			}
			if tt.included >= 2 {
				assertOrderedSubstrings(t, markdown, []string{"### Run 1: run-newest", "### Run 2: run-middle"})
			}
			if tt.included == 0 && !strings.Contains(markdown, "history limit is zero") {
				t.Fatalf("zero-limit notice missing:\n%s", markdown)
			}
			if tt.omitted > 0 {
				want := "Omitted " + string(rune('0'+tt.omitted)) + " older run(s) due to the history limit."
				if !strings.Contains(markdown, want) {
					t.Fatalf("omission notice %q missing:\n%s", want, markdown)
				}
			} else if strings.Contains(markdown, "older run(s)") {
				t.Fatalf("unexpected omission notice:\n%s", markdown)
			}
		})
	}
}

func TestTaskDossierRecentRunOrderingUsesRunIDTieBreaker(t *testing.T) {
	startedAt := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	in := sparseDossierInput()
	in.RecentRuns = []RecentRunSummary{
		testRecentRun("run-z", startedAt),
		testRecentRun("run-a", startedAt),
	}
	in.RecentRunLimit = 2
	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}
	assertOrderedSubstrings(t, string(result.Markdown), []string{"### Run 1: run-a", "### Run 2: run-z"})
}

func TestTaskDossierReceiptAndBoundedWindowProvenance(t *testing.T) {
	in := sparseDossierInput()
	in.RecentRuns = testRecentRuns()
	in.RecentRunLimit = 1
	in.RecentRunWindow = &DossierSourceWindow{Limit: 3, HasOlderItems: true}
	in.Receipts = []ReceiptSource{
		{ID: "receipt:run-newest", Path: ".revolvr/receipts/run-newest.md", Content: []byte("exact receipt bytes\r\n")},
	}

	result, err := BuildTaskDossier(in)
	if err != nil {
		t.Fatalf("BuildTaskDossier() error = %v", err)
	}
	receiptRecord := dossierSource(t, result.Manifest, DossierSourceKindReceipt, "receipt:run-newest")
	assertRawSourceRecord(t, receiptRecord, in.Receipts[0].Content)
	if got, want := receiptRecord.Path, ".revolvr/receipts/run-newest.md"; got != want {
		t.Fatalf("receipt path = %q, want %q", got, want)
	}
	runs := dossierSource(t, result.Manifest, DossierSourceKindRecentRuns, "recent-runs")
	if runs.SourceWindow == nil || *runs.SourceWindow != (DossierSourceWindow{Limit: 3, HasOlderItems: true}) {
		t.Fatalf("source window = %#v, want bounded older history", runs.SourceWindow)
	}
	if !strings.Contains(string(result.Markdown), "older selected-task items exist beyond the bounded source window") {
		t.Fatalf("bounded-window omission missing:\n%s", result.Markdown)
	}
}

func TestTaskDossierBoundedSourceHashDoesNotDependOnRenderLimit(t *testing.T) {
	in := sparseDossierInput()
	in.RecentRuns = testRecentRuns()
	in.RecentRunWindow = &DossierSourceWindow{Limit: 3}

	var sourceHash string
	for _, limit := range []int{0, 1, 3} {
		in.RecentRunLimit = limit
		result, err := BuildTaskDossier(in)
		if err != nil {
			t.Fatalf("BuildTaskDossier(render limit %d) error = %v", limit, err)
		}
		record := dossierSource(t, result.Manifest, DossierSourceKindRecentRuns, "recent-runs")
		if sourceHash == "" {
			sourceHash = record.SHA256
		} else if record.SHA256 != sourceHash {
			t.Fatalf("render limit %d source hash = %q, want %q", limit, record.SHA256, sourceHash)
		}
	}
}

func fullDossierInput() TaskDossierInput {
	state := validExecutionState(LifecycleStateCorrecting)
	state.Plan.ID = "plan-002"
	state.Plan.Revision = 2
	state.Plan.SupersedesPlanID = "plan-001"
	state.Plan.Steps = []PlanStep{
		{
			ID:          "inspect-contracts",
			Description: "Inspect the existing autonomous contracts.",
			Status:      PlanStepStatusCompleted,
			Evidence:    []EvidenceReference{testEvidence(EvidenceKindRepository, "internal/autonomous/state.go")},
		},
		{
			ID:          "render-dossier",
			Description: "Render the pure deterministic dossier.",
			Status:      PlanStepStatusInProgress,
		},
		{
			ID:          "skip-runtime",
			Description: "Avoid runtime assembly in this task.",
			Status:      PlanStepStatusSkipped,
			Rationale:   "Repository-backed assembly belongs to AW-04.",
		},
	}
	state.AcceptanceCriteria = []AcceptanceCriterion{
		validAcceptanceCriterion("criterion-satisfied", AcceptanceStatusSatisfied),
		validAcceptanceCriterion("criterion-waived", AcceptanceStatusWaived),
		validAcceptanceCriterion("criterion-na", AcceptanceStatusNotApplicable),
	}
	state.FindingResolutions = []FindingResolution{
		validFindingResolution("finding-001", FindingResolutionStatusResolved),
		validFindingResolution("finding-002", FindingResolutionStatusSuperseded),
	}
	decision := validDecisionReference(ActionCorrect)
	decision.DecisionID = "decision-current"
	decision.RunID = "run-supervisor"
	state.LatestDecision = &decision
	state.Attempts = validAttemptState()

	verification := testVerificationSummary()
	report := validAuditReport(
		AuditDispositionChangesRequired,
		validFinding("finding-001"),
		validFinding("finding-002"),
	)
	gitEvidence := testEvidence(EvidenceKindGit, "git-snapshot-001")
	return TaskDossierInput{
		TaskID: "task-1",
		TaskSpec: TaskSpecSource{
			ID:      "task-spec",
			Path:    ".agent/tasks/task-1.md",
			Label:   "AW-03 dossier task",
			Content: []byte("# AW-03\n\nBuild a deterministic dossier."),
		},
		State:          state,
		Verification:   &verification,
		Audit:          &report,
		RecentRuns:     testRecentRuns(),
		RecentRunLimit: 2,
		Git: &GitSnapshot{
			Head:           "0123456789abcdef",
			WorktreeStatus: "modified autonomous baseline files",
			DiffSummary:    "2 files changed, 40 insertions(+)",
			Evidence:       &gitEvidence,
		},
		Guidance: []GuidanceSource{
			{ID: "nested-guidance", Path: "docs/AGENTS.md", Label: "Nested guidance", Content: []byte("# Nested Rules\n\nKeep output stable.")},
			{ID: "root-guidance", Path: "AGENTS.md", Label: "Root guidance", Content: []byte("# Rules\n\nWork in small changes.\n")},
		},
	}
}

func sparseDossierInput() TaskDossierInput {
	return TaskDossierInput{
		TaskID: "task-1",
		TaskSpec: TaskSpecSource{
			ID:      "task-spec",
			Path:    ".agent/tasks/task-1.md",
			Content: []byte("# Sparse Task\n"),
		},
		State: ExecutionState{
			SchemaVersion: ExecutionStateSchemaVersion,
			TaskID:        "task-1",
			Lifecycle:     LifecycleStatePending,
			Attempts:      zeroAttemptState(),
		},
		RecentRunLimit: 0,
	}
}

func minimalDossierInput() TaskDossierInput { return sparseDossierInput() }

func testVerificationSummary() VerificationSummary {
	return VerificationSummary{
		TaskID:       "task-1",
		Status:       VerificationStatusPassed,
		Command:      "focused: go test ./internal/autonomous",
		Summary:      "The focused autonomous package tests passed.",
		RunID:        "run-verification",
		OccurrenceID: "verification-001",
		Evidence:     []EvidenceReference{testEvidence(EvidenceKindVerification, "run-verification:focused")},
	}
}

func testRecentRuns() []RecentRunSummary {
	return []RecentRunSummary{
		testRecentRun("run-middle", time.Date(2026, 7, 10, 13, 0, 0, 0, time.UTC)),
		testRecentRun("run-oldest", time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)),
		testRecentRun("run-newest", time.Date(2026, 7, 10, 14, 0, 0, 0, time.UTC)),
	}
}

func testRecentRun(id string, startedAt time.Time) RecentRunSummary {
	completedAt := startedAt.Add(10 * time.Minute)
	return RecentRunSummary{
		RunID:       id,
		TaskID:      "task-1",
		Action:      ActionImplement,
		Profile:     WorkerProfileImplementer,
		Outcome:     "completed with durable evidence",
		StartedAt:   startedAt,
		CompletedAt: &completedAt,
		Evidence:    []EvidenceReference{testEvidence(EvidenceKindLedger, id+":completed")},
	}
}

func dossierSource(t *testing.T, manifest TaskDossierManifest, kind DossierSourceKind, id string) DossierSourceRecord {
	t.Helper()
	for _, source := range manifest.Sources {
		if source.Kind == kind && source.ID == id {
			return source
		}
	}
	t.Fatalf("source kind=%q id=%q not found: %#v", kind, id, manifest.Sources)
	return DossierSourceRecord{}
}

func assertRawSourceRecord(t *testing.T, record DossierSourceRecord, content []byte) {
	t.Helper()
	if got, want := record.SHA256, sha256HexBytes(content); got != want {
		t.Fatalf("source SHA-256 = %q, want %q", got, want)
	}
	if got, want := record.ByteSize, len(content); got != want {
		t.Fatalf("source byte size = %d, want %d", got, want)
	}
	if record.IncludedByteSize == nil || *record.IncludedByteSize != len(content) {
		t.Fatalf("included byte size = %#v, want %d", record.IncludedByteSize, len(content))
	}
	if record.Truncated {
		t.Fatal("raw source unexpectedly marked truncated")
	}
}

func assertTypedSourceRecord(t *testing.T, record DossierSourceRecord, canonicalJSON []byte) {
	t.Helper()
	if got, want := record.SHA256, sha256HexBytes(canonicalJSON); got != want {
		t.Fatalf("typed source SHA-256 = %q, want %q\ncanonical JSON: %s", got, want, canonicalJSON)
	}
	if got, want := record.ByteSize, len(canonicalJSON); got != want {
		t.Fatalf("typed source byte size = %d, want %d", got, want)
	}
	if record.IncludedByteSize != nil {
		t.Fatalf("typed source included byte size = %#v, want nil", record.IncludedByteSize)
	}
}

func assertOrderedSubstrings(t *testing.T, content string, values []string) {
	t.Helper()
	position := 0
	for _, value := range values {
		index := strings.Index(content[position:], value)
		if index < 0 {
			t.Fatalf("%q not found after byte %d:\n%s", value, position, content)
		}
		position += index + len(value)
	}
}
