package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousfinalization"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func TestProductionAutonomousCorrectionAndReaudit(t *testing.T) {
	const (
		taskID      = "production-correction"
		operationID = "production-correction-operation"
		findingID   = "incorrect-result"
	)
	now := time.Date(2026, 7, 16, 18, 0, 0, 0, time.UTC)
	t.Setenv("GIT_AUTHOR_DATE", now.Format(time.RFC3339))
	t.Setenv("GIT_COMMITTER_DATE", now.Format(time.RFC3339))

	repo := t.TempDir()
	initializeSchedulingGitRepository(t, repo)
	for name, content := range map[string]string{
		"supervisor": "Select exactly one evidence-grounded autonomous action.",
		"corrector":  "Repair only the exact cited blocking audit finding.",
		"auditor":    "Independently audit exact source and verification evidence.",
	} {
		writeProductionHappyFile(t, filepath.Join(repo, ".agent", "profiles", name+".md"), content+"\n")
	}
	writeProductionHappyFile(t, filepath.Join(repo, "docs", "result.md"), "incorrect production result\n")
	task, err := taskfile.ProjectAutonomousTask(repo, taskfile.AutonomousCreateInput{
		ID: taskID, Title: "Production correction and independent re-audit",
		Body: "Correct `docs/result.md` so it contains exactly `corrected production result`, resolve the cited blocking finding, independently re-audit the corrected source, and complete the task.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if task, err = taskfile.PublishAutonomousTask(repo, task); err != nil {
		t.Fatal(err)
	}
	runSchedulingGit(t, repo, "add", ".agent", "docs")
	runSchedulingGit(t, repo, "commit", "-q", "-m", "Seed production correction path")
	baselineHead := runSchedulingGit(t, repo, "rev-parse", "HEAD")
	baselineRevision := productionHappySourceRevision(t, repo)
	seedProductionHappyAudit(t, repo, task, baselineRevision, now)
	for _, directory := range []string{"locks", "receipts", "runs"} {
		if err := os.MkdirAll(filepath.Join(repo, ".revolvr", directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	executable := buildStrictFakeCodex(t)
	executableIdentity, err := codexexec.InspectExecutable(executable, nil)
	if err != nil {
		t.Fatal(err)
	}
	releaseManifest := codexexec.ReleaseManifest{
		SchemaVersion: codexexec.ReleaseManifestSchema,
		Codex:         []codexexec.ReleaseCodexBuild{{Version: strictFakeCodexVersion, SHA256: executableIdentity.SHA256}},
	}
	if err := releaseManifest.Validate(); err != nil {
		t.Fatal(err)
	}
	runCfg := DefaultRunOnceConfig(repo)
	runCfg.CodexExecutable = executable
	runCfg.CodexTimeout = 10 * time.Second
	runCfg.VerificationPlan = &autonomousverification.Plan{
		SchemaVersion: autonomousverification.PlanSchemaVersion,
		Tiers: []autonomousverification.Tier{{
			ID: "task-acceptance", Kind: autonomousverification.TierTaskAcceptance,
			RunForFast: true, RequiredForFinal: true, RunForFinal: true,
			Commands:    []verification.Command{{Name: "sh", Args: []string{"-c", `test "$(cat docs/result.md)" = "corrected production result"`}}},
			RerunPolicy: autonomousverification.RerunNever,
		}},
	}
	effective, err := runonce.EffectiveConfig(runCfg)
	if err != nil {
		t.Fatal(err)
	}
	workspace := productionHappyWorkspaceRoot(t, repo, taskID)
	contract := productionCorrectionCodexContract(t, repo, workspace, executable, effective, taskID, findingID)
	fixture := configureStrictFakeCodex(t, executable, repo, contract)

	ids := 0
	clockTicks := int64(-1)
	result, err := RunTaskUntilTerminal(context.Background(), Config{WorkDir: repo}, TaskRunInput{
		OperationID: operationID,
		TaskID:      taskID,
		MaxCycles:   3,
		Clock: func() time.Time {
			clockTicks++
			return now.Add(time.Duration(clockTicks) * time.Second)
		},
		RunConfig: &runCfg,
		idGenerator: func() string {
			ids++
			return "correction-" + leftPadTwo(ids)
		},
		releaseManifest: &releaseManifest,
	})
	if err != nil {
		t.Fatalf("RunTaskUntilTerminal() error = %v; result=%+v; strict fake=%+v", err, result, fixture.loadState(t))
	}
	wantStatistics := autonomoustaskrun.Statistics{
		SupervisorStarted: 3, SupervisorCompleted: 3, CyclesStarted: 3, CyclesCompleted: 3,
		AttemptsAdmitted: 2, AttemptsCompleted: 2, VerificationRuns: 2, Audits: 2, Corrections: 1,
		SourceCommits: 1, CheckpointAdvances: 1,
		Actions: []autonomoustaskrun.ActionCount{{Action: string(autonomous.ActionAudit), Count: 1}, {Action: string(autonomous.ActionCorrect), Count: 1}},
	}
	if result.StopReason != autonomoustaskrun.StopCompleted || result.OperationID != operationID || result.TaskID != taskID || result.LastAction != string(autonomous.ActionComplete) || result.LastRunID != "correction-23" || !reflect.DeepEqual(result.Statistics, wantStatistics) {
		t.Fatalf("terminal result = %+v, want completed exact statistics %+v", result, wantStatistics)
	}
	if ids != 24 {
		t.Fatalf("production ID calls = %d, want 24", ids)
	}
	wantFakeState := strictFakeCodexState{SchemaVersion: strictFakeCodexStateSchema, VersionInvocations: 1, NextInvocation: 7, OutputSequence: append([]string(nil), contract.OutputSequence...)}
	if got := fixture.loadState(t); !reflect.DeepEqual(got, wantFakeState) {
		t.Fatalf("strict fake state = %+v, want %+v", got, wantFakeState)
	}

	assertStrictFakeArtifact(t, filepath.Join(workspace, "docs", "result.md"), "corrected production result\n")
	assertStrictFakeArtifact(t, filepath.Join(repo, "docs", "result.md"), "incorrect production result\n")
	workspaceHead := runSchedulingGit(t, workspace, "rev-parse", "HEAD")
	if workspaceHead == baselineHead || runSchedulingGit(t, repo, "rev-parse", "HEAD") != baselineHead {
		t.Fatalf("workspace/control HEADs = %s/%s, baseline %s", workspaceHead, runSchedulingGit(t, repo, "rev-parse", "HEAD"), baselineHead)
	}
	if got := runSchedulingGit(t, workspace, "rev-list", "--count", baselineHead+".."+workspaceHead); got != "1" {
		t.Fatalf("run-owned commit count = %q, want 1", got)
	}
	if got := runSchedulingGit(t, workspace, "diff-tree", "--no-commit-id", "--name-status", "-r", workspaceHead); got != "M\tdocs/result.md" {
		t.Fatalf("run-owned correction diff = %q", got)
	}

	stateStore, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: repo})
	if err != nil {
		t.Fatal(err)
	}
	state, found, err := stateStore.Load(context.Background(), taskID)
	if err != nil || !found {
		t.Fatalf("load terminal state found=%v err=%v", found, err)
	}
	if state.State.Lifecycle != autonomous.LifecycleStateCompleted || state.State.Finalization == nil || state.State.Finalization.Stage != autonomous.FinalizationStageLedgerCompleted || state.State.Workspace == nil || state.State.Workspace.HeadSHA != workspaceHead {
		t.Fatalf("terminal state = %+v", state.State)
	}
	if len(state.State.Attempts.Events) != 4 {
		t.Fatalf("attempt events = %+v, want exact audit and correction admission/completion pairs", state.State.Attempts.Events)
	}
	auditAdmitted, auditCompleted := state.State.Attempts.Events[0], state.State.Attempts.Events[1]
	correctionAdmitted, correctionCompleted := state.State.Attempts.Events[2], state.State.Attempts.Events[3]
	if auditAdmitted.Kind != autonomous.AttemptEventAdmitted || auditCompleted.Kind != autonomous.AttemptEventCompleted || auditAdmitted.AttemptID != "attempt-correction-01" || auditCompleted.AttemptID != auditAdmitted.AttemptID || auditAdmitted.Action != autonomous.ActionAudit || auditCompleted.Action != autonomous.ActionAudit || auditCompleted.Outcome != autonomous.AttemptOutcomeSucceeded || auditCompleted.RunID != "correction-05" {
		t.Fatalf("exact finding-audit attempt evidence = %+v / %+v", auditAdmitted, auditCompleted)
	}
	if correctionAdmitted.Kind != autonomous.AttemptEventAdmitted || correctionCompleted.Kind != autonomous.AttemptEventCompleted || correctionAdmitted.AttemptID != "attempt-correction-07" || correctionCompleted.AttemptID != correctionAdmitted.AttemptID || correctionAdmitted.Action != autonomous.ActionCorrect || correctionCompleted.Action != autonomous.ActionCorrect || correctionCompleted.Outcome != autonomous.AttemptOutcomeSucceeded || correctionCompleted.RunID != "correction-18" {
		t.Fatalf("exact correction attempt evidence = %+v / %+v", correctionAdmitted, correctionCompleted)
	}
	if len(state.State.FindingResolutions) != 1 || state.State.FindingResolutions[0].FindingID != findingID || state.State.FindingResolutions[0].Status != autonomous.FindingResolutionStatusResolved || state.State.FindingResolutions[0].Resolution == nil || state.State.FindingResolutions[0].Resolution.RunID != "correction-10" {
		t.Fatalf("finding resolutions = %+v", state.State.FindingResolutions)
	}

	history, err := stateStore.LoadCommittedAuditHistory(context.Background(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	var findingAudit, resolution, cleanReaudit *autonomousstate.AuditHistoryRecord
	for i := range history {
		record := &history[i].Record
		switch {
		case record.Kind == autonomousstate.AuditTransitionRecorded && record.WorkerRunID == "correction-05":
			findingAudit = record
		case record.Kind == autonomousstate.AuditTransitionFindingResolved && record.Resolution != nil && record.Resolution.FindingID == findingID:
			resolution = record
		case record.Kind == autonomousstate.AuditTransitionRecorded && record.WorkerRunID == "correction-18":
			cleanReaudit = record
		}
	}
	if findingAudit == nil || findingAudit.Report.Disposition != autonomous.AuditDispositionChangesRequired || len(findingAudit.NewFindingIDs) != 1 || findingAudit.NewFindingIDs[0] != findingID {
		t.Fatalf("blocking audit record = %+v", findingAudit)
	}
	if resolution == nil || resolution.OperationID != "correction-11-resolution-01" || resolution.Resolution.Resulting.Status != autonomous.FindingResolutionStatusResolved || resolution.Resolution.Resulting.Resolution == nil || resolution.Resolution.Resulting.Resolution.RunID != "correction-10" || !hasEvidenceReference(resolution.Resolution.Resulting.Evidence, "ledger:correction-14:verification:correction-15") {
		t.Fatalf("finding resolution record = %+v", resolution)
	}
	if cleanReaudit == nil || cleanReaudit.Report.Disposition != autonomous.AuditDispositionClean || cleanReaudit.Verification.Summary.RunID != "correction-14" || cleanReaudit.Verification.Summary.OccurrenceID != "correction-15" || cleanReaudit.LatestSourceMutation == nil || cleanReaudit.LatestSourceMutation.RunID != "correction-11" {
		t.Fatalf("clean re-audit record = %+v", cleanReaudit)
	}
	if !(findingAudit.Sequence < resolution.Sequence && resolution.Sequence < cleanReaudit.Sequence) {
		t.Fatalf("audit sequence = finding %d, resolution %d, clean %d", findingAudit.Sequence, resolution.Sequence, cleanReaudit.Sequence)
	}

	receiptEntries, err := os.ReadDir(filepath.Join(repo, ".revolvr", "receipts"))
	if err != nil {
		t.Fatal(err)
	}
	var receiptNames []string
	for _, entry := range receiptEntries {
		receiptNames = append(receiptNames, entry.Name())
	}
	sort.Strings(receiptNames)
	if want := []string{"correction-05.md", "correction-11.md", "correction-18.md"}; !reflect.DeepEqual(receiptNames, want) {
		t.Fatalf("receipt files = %v, want %v", receiptNames, want)
	}
	correctorReceiptRaw, err := os.ReadFile(filepath.Join(repo, ".revolvr", "receipts", "correction-11.md"))
	if err != nil {
		t.Fatal(err)
	}
	correctorReceipt, err := receipt.Parse(correctorReceiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	if correctorReceipt.RunID != "correction-11" || correctorReceipt.Verdict != receipt.VerdictCompleted || correctorReceipt.VerificationStatus != "passed" || correctorReceipt.CommitSHA != workspaceHead || !reflect.DeepEqual(correctorReceipt.ChangedFiles, []string{"docs/result.md"}) {
		t.Fatalf("corrector receipt = %+v", correctorReceipt)
	}

	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(repo, ".revolvr", "ledger.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	runs, err := ledgerStore.ListRecentRunsForTaskWithEvents(context.Background(), taskID, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 10 {
		t.Fatalf("ledger run count = %d, want 10", len(runs))
	}
	byID := make(map[string]ledger.RunWithEvents, len(runs))
	for _, item := range runs {
		byID[item.Run.ID] = item
	}
	for _, runID := range []string{productionTaskRunLedgerID(operationID), "correction-04", "correction-05", "correction-10", "correction-11", "correction-14", "correction-17", "correction-18", "correction-23", "finalization-correction-24"} {
		if item, ok := byID[runID]; !ok || item.Run.Status != ledger.StatusCompleted {
			t.Fatalf("ledger run %s missing or noncompleted: %+v", runID, item.Run)
		}
	}
	assertProductionVerificationOccurrence(t, byID["correction-11"], autonomousverification.PurposeFast, "correction-12")
	assertProductionVerificationOccurrence(t, byID["correction-14"], autonomousverification.PurposeFinal, "correction-15")

	completionBase := filepath.Join(repo, ".revolvr", "autonomous", "tasks", taskID, "completion")
	completionEntries, err := os.ReadDir(completionBase)
	if err != nil {
		t.Fatal(err)
	}
	var completionNames []string
	for _, entry := range completionEntries {
		completionNames = append(completionNames, entry.Name())
	}
	sort.Strings(completionNames)
	if want := []string{"completion-evidence.json", "completion-manifest.json", "completion.md"}; !reflect.DeepEqual(completionNames, want) {
		t.Fatalf("completion artifacts = %v, want %v", completionNames, want)
	}
	frozenRaw, err := os.ReadFile(filepath.Join(completionBase, "completion-evidence.json"))
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := autonomousfinalization.DecodeFrozen(frozenRaw)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.OperationID != operationID+"-finalization" || frozen.FinalizationRunID != "finalization-correction-24" || frozen.Verification.Summary.RunID != "correction-14" || frozen.Verification.Summary.OccurrenceID != "correction-15" || frozen.Audit.RunID != "correction-18" || frozen.DecisionReference.RunID != "correction-23" || len(frozen.Commits) != 1 || frozen.Commits[0].RunID != "correction-11" || frozen.Commits[0].SHA != workspaceHead {
		t.Fatalf("terminal frozen evidence = %+v", frozen)
	}
	operation, found, err := autonomoustaskrun.Inspect(repo, operationID)
	if err != nil || !found || operation.StopReason != autonomoustaskrun.StopCompleted || operation.Stage != "terminal" || operation.CheckpointSHA != workspaceHead {
		t.Fatalf("terminal operation found=%v operation=%+v err=%v", found, operation, err)
	}
}

func productionCorrectionCodexContract(t *testing.T, root, workspace, executable string, cfg runonce.Config, taskID, findingID string) strictFakeCodexContract {
	t.Helper()
	supervisorSchema, err := supervisor.DecisionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	auditSchema, err := autonomousaudit.AuditOutputSchema()
	if err != nil {
		t.Fatal(err)
	}
	correctionSchema, err := autonomous.CorrectionOutputSchema()
	if err != nil {
		t.Fatal(err)
	}

	initialAudit := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor,
		Rationale:       "The current result requires an independent audit against the exact task.",
		SuccessCriteria: []string{"Record every blocking mismatch in docs/result.md."},
		Inputs:          []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact result requiring independent audit.")},
		Strategy:        &autonomous.Strategy{Approach: "Audit the exact result against the canonical task.", Techniques: []string{"inspect source and verification evidence"}, Targets: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact audit target.")}},
	}
	correct := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionCorrect, WorkerProfile: autonomous.WorkerProfileCorrector,
		Rationale:       "The current independent audit identifies one blocking incorrect result.",
		SuccessCriteria: []string{"Resolve only the cited incorrect-result finding."},
		Inputs:          []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact cited repair target.")},
		FindingIDs:      []string{findingID},
		Strategy:        &autonomous.Strategy{Approach: "Replace only the incorrect result with the exact task-required value.", Techniques: []string{"edit only docs/result.md"}, Targets: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exclusive correction target.")}},
	}
	reaudit := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionAudit, WorkerProfile: autonomous.WorkerProfileAuditor,
		Rationale:       "The corrected commit and distinct final verification require a fresh independent re-audit.",
		SuccessCriteria: []string{"Return a clean audit tied to the final verification occurrence."},
		Inputs:          []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact corrected and finally verified result.")},
		Strategy:        &autonomous.Strategy{Approach: "Independently re-audit the corrected source.", Techniques: []string{"inspect corrected source and final verification"}, Targets: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "Exact re-audit target.")}},
	}
	complete := autonomous.SupervisorDecision{
		TaskID: taskID, Action: autonomous.ActionComplete,
		Rationale: "The cited repair, distinct final verification, exact finding resolution, clean independent re-audit, and checkpoint authorize completion.",
		Inputs:    []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindTask, ".agent/tasks/"+taskID+".md", "Exact canonical task.")},
	}
	for _, decision := range []*autonomous.SupervisorDecision{&initialAudit, &correct, &reaudit, &complete} {
		if err := decision.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	decisionJSON := func(value autonomous.SupervisorDecision) string {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw) + "\n"
	}
	events := func(name string, input, output int) []string {
		return []string{
			`{"type":"thread.started","thread_id":"` + name + `-thread"}`,
			`{"type":"turn.completed","final_message":"` + name + ` completed","usage":{"input_tokens":` + decimal(input) + `,"output_tokens":` + decimal(output) + `,"duration_seconds":1}}`,
		}
	}
	initialAuditOutput := `{"schema_version":"autonomous-audit-output-v1","task_id":"` + taskID + `","report":{"task_id":"` + taskID + `","disposition":"changes_required","rationale":"The result contradicts the exact task-required value.","inputs":@@INPUTS@@,"findings":[{"id":"` + findingID + `","significance":"blocking","summary":"docs/result.md contains the incorrect production result.","evidence":[{"kind":"file","reference":"docs/result.md","detail":"The exact committed result is incorrect."}],"required_correction":"Replace only docs/result.md with the exact corrected production result."}]},"provenance":@@PROVENANCE@@}` + "\n"
	correctionOutput := autonomous.CorrectionOutput{
		SchemaVersion: autonomous.CorrectionOutputSchemaVersion, TaskID: taskID, WorkerRunID: "correction-11", DecisionID: "decision-correction-10",
		FindingIDs: []string{findingID}, Outcome: autonomous.CorrectionOutcomeCorrected, ResolvedFindingIDs: []string{findingID},
		Evidence: []autonomous.EvidenceReference{productionEvidence(autonomous.EvidenceKindFile, "docs/result.md", "The cited result now exactly matches the task requirement.")},
	}
	correctionRaw, err := autonomous.MarshalCorrectionOutput(correctionOutput)
	if err != nil {
		t.Fatal(err)
	}
	cleanAuditOutput := `{"schema_version":"autonomous-audit-output-v1","task_id":"` + taskID + `","report":{"task_id":"` + taskID + `","disposition":"clean","rationale":"The cited repair is exact and the distinct final verification passed.","inputs":@@INPUTS@@},"provenance":@@PROVENANCE@@}` + "\n"

	type invocationSpec struct {
		name, runID string
		action      autonomous.Action
		supervisor  bool
		message     string
		writes      []strictFakeCodexMaterial
	}
	specs := []invocationSpec{
		{name: "finding-audit-supervisor", runID: "correction-04", supervisor: true, message: decisionJSON(initialAudit)},
		{name: "finding-audit-worker", runID: "correction-05", action: autonomous.ActionAudit, message: initialAuditOutput},
		{name: "correction-supervisor", runID: "correction-10", supervisor: true, message: decisionJSON(correct)},
		{name: "correction-worker", runID: "correction-11", action: autonomous.ActionCorrect, message: string(correctionRaw), writes: []strictFakeCodexMaterial{{Path: filepath.Join("docs", "result.md"), Content: "corrected production result\n"}}},
		{name: "clean-reaudit-supervisor", runID: "correction-17", supervisor: true, message: decisionJSON(reaudit)},
		{name: "clean-reaudit-worker", runID: "correction-18", action: autonomous.ActionAudit, message: cleanAuditOutput},
		{name: "complete-supervisor", runID: "correction-23", supervisor: true, message: decisionJSON(complete)},
	}
	contract := strictFakeCodexContract{VersionInvocationCount: 1}
	for index, spec := range specs {
		invocation := strictFakeCodexInvocation{Name: spec.name, WorkingDirectory: workspace, LastMessage: spec.message, Writes: spec.writes}
		base := filepath.ToSlash(filepath.Join(".revolvr", "runs", spec.runID))
		if spec.supervisor {
			invocation.PromptPath = filepath.Join(root, filepath.FromSlash(base), "supervisor-prompt.md")
			invocation.OutputSchema = &strictFakeCodexMaterial{Path: filepath.Join(root, filepath.FromSlash(base), "supervisor-output-schema.json"), Content: string(supervisorSchema)}
			writeProductionHappyFile(t, invocation.OutputSchema.Path, invocation.OutputSchema.Content)
			invocation.Argv = productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: base + "/supervisor-output.json"}, base+"/supervisor-output-schema.json")
		} else {
			output, schema := base+"/worker-output.txt", ""
			switch spec.action {
			case autonomous.ActionAudit:
				output, schema = base+"/auditor-output.raw.json", base+"/auditor-output-schema.json"
				invocation.OutputSchema = &strictFakeCodexMaterial{Path: filepath.Join(root, filepath.FromSlash(schema)), Content: string(auditSchema)}
				invocation.Substitutions = []strictFakeSubstitution{{Token: "@@INPUTS@@", Heading: "Exact Audit Output Provenance", JSONPointer: "/verification/summary/evidence"}, {Token: "@@PROVENANCE@@", Heading: "Exact Audit Output Provenance"}}
			case autonomous.ActionCorrect:
				output, schema = base+"/corrector-output.raw.json", base+"/corrector-output-schema.json"
				invocation.OutputSchema = &strictFakeCodexMaterial{Path: filepath.Join(root, filepath.FromSlash(schema)), Content: string(correctionSchema)}
			}
			writeProductionHappyFile(t, invocation.OutputSchema.Path, invocation.OutputSchema.Content)
			invocation.PromptPath = filepath.Join(root, filepath.FromSlash(base), "worker-prompt.md")
			invocation.Argv = productionHappyInvocation(t, cfg, executable, workspace, root, codexexec.ArtifactPaths{StdoutJSONL: base + "/codex.jsonl", Stderr: base + "/codex.stderr", LastMessage: output}, schema)
		}
		invocation.StdoutJSONL = events(spec.name, 20+index, 10+index)
		invocation.OutputEventTypes = []string{"thread.started", "turn.completed"}
		contract.Invocations = append(contract.Invocations, invocation)
		contract.OutputSequence = append(contract.OutputSequence, spec.name+":thread.started", spec.name+":turn.completed")
	}
	return contract
}

func assertProductionVerificationOccurrence(t *testing.T, run ledger.RunWithEvents, purpose autonomousverification.Purpose, occurrenceID string) {
	t.Helper()
	var completed []autonomousverification.CompletedLedgerEvent
	for _, event := range run.Events {
		if event.Type != ledger.EventVerificationCompleted {
			continue
		}
		decoded, err := autonomousverification.DecodeCompletedLedgerEvent(event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		completed = append(completed, decoded)
	}
	if len(completed) != 1 || completed[0].Purpose != purpose || completed[0].OccurrenceID != occurrenceID || completed[0].Outcome != autonomousverification.OutcomePassed {
		t.Fatalf("verification occurrences for %s = %+v", run.Run.ID, completed)
	}
}

func hasEvidenceReference(evidence []autonomous.EvidenceReference, reference string) bool {
	for _, item := range evidence {
		if item.Reference == reference {
			return true
		}
	}
	return false
}
