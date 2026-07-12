package autonomousfinalization

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/ledger"
	"revolvr/internal/redact"
	"revolvr/internal/taskfile"
)

func TestRenderCapsuleDeterministicGolden(t *testing.T) {
	e := fixtureEvidence(t, "/repo", nil)
	first, err := RenderCapsule(e)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RenderCapsule(e)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("repeated rendering changed bytes")
	}
	sum := sha256.Sum256(first)
	// This digest is the byte-for-byte golden for the canonical section and
	// source ordering above. Change it only with a reviewed capsule contract.
	const golden = "f832ef7d1e6209ad868f5ca4c8f3a34c328ba7fe1034bd00374f57783577032f"
	if got := fmt.Sprintf("%x", sum); got != golden {
		t.Fatalf("golden capsule SHA-256 = %s\n%s", got, first)
	}
	for _, section := range []string{"## Task and Specification", "## Final Source and Workspace", "## Completion Decision", "## Completed Plan", "## Acceptance Matrix", "## Final Verification", "## Independent Audit and Findings", "## Optional Roles", "## Attempts, Runs, and Commits", "## Safety and Reproducibility", "## Waivers, Not Applicable, Omissions, and Warnings", "## Finalization"} {
		if !strings.Contains(string(first), section) {
			t.Fatalf("missing section %q", section)
		}
	}
}

func TestFrozenEvidenceRejectsMissingAndStaleGates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*FrozenEvidence)
		want   string
	}{
		{"fast verification", func(e *FrozenEvidence) { e.Verification.Tiered.Purpose = autonomousverification.PurposeFast }, "final-purpose"},
		{"pending acceptance", func(e *FrozenEvidence) {
			e.State.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusPending
			e.State.AcceptanceCriteria[0].Evidence = nil
		}, "pending"},
		{"stale source", func(e *FrozenEvidence) { e.Verification.SourceRevision = strings.Repeat("9", 64) }, "stale"},
		{"unsafe preflight", func(e *FrozenEvidence) { e.SafetyPreflight.Ready = false }, "unsafe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := fixtureEvidence(t, "/repo", nil)
			tt.mutate(&e)
			identity, identityErr := autonomousstate.StateIdentityFor(e.Task.StatePath, true, e.State)
			if identityErr != nil {
				t.Fatal(identityErr)
			}
			e.StateIdentity = identity
			err := e.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestCapsuleMakesWaiversNotApplicableAndCommitOrderExplicit(t *testing.T) {
	e := fixtureEvidence(t, "/repo", nil)
	e.State.AcceptanceCriteria[0].Status = autonomous.AcceptanceStatusWaived
	e.State.AcceptanceCriteria[0].Rationale = "Operator waived on exact scope evidence."
	e.State.AcceptanceCriteria[0].Evidence = []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, "waiver-one")}
	e.State.AcceptanceCriteria = append(e.State.AcceptanceCriteria, autonomous.AcceptanceCriterion{ID: "criterion-two", Requirement: "Legacy migration is required.", Status: autonomous.AcceptanceStatusNotApplicable, Rationale: "No legacy source exists.", Evidence: []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindRepository, "scope-scan")}})
	finalSHA := strings.Repeat("9", 40)
	e.Workspace.HeadSHA = finalSHA
	e.Workspace.Checkpoint.CommitSHA = finalSHA
	e.State.Workspace = &e.Workspace
	e.SafetyPolicy.Workspace = e.Workspace
	var err error
	e.SafetyPolicy, err = autonomoussafety.FinalizePolicy(e.SafetyPolicy)
	if err != nil {
		t.Fatal(err)
	}
	e.SafetyPreflight.PolicySHA256 = e.SafetyPolicy.PolicySHA256
	e.Commits = []CommitEvidence{{Sequence: 1, SHA: finalSHA, RunID: "worker-run", Action: autonomous.ActionImplement, Outcome: "reconciled", Reconciled: true, CreatedAt: e.AdmittedAt.Add(-time.Minute)}}
	e.Runs = append(e.Runs, RunEvidence{Sequence: int64(len(e.Runs) + 1), RunID: "worker-run", Kind: "worker", Outcome: "completed", Artifact: evidence(autonomous.EvidenceKindReceipt, "worker-run/receipt"), StartedAt: e.AdmittedAt.Add(-2 * time.Minute), CompletedAt: e.AdmittedAt.Add(-time.Minute)})
	e.StateIdentity, err = autonomousstate.StateIdentityFor(e.Task.StatePath, true, e.State)
	if err != nil {
		t.Fatal(err)
	}
	e.Route, err = autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: e.Task.TaskID, Decision: e.Decision, Reference: e.DecisionReference, State: e.State, Source: e.Source, Verification: &e.Verification, Audit: &e.Audit})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := RenderCapsule(e)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"criterion-one` | waived", "criterion-two` | not_applicable", "Commit 1 `" + finalSHA + "", "reconciled"} {
		if !strings.Contains(text, want) {
			t.Fatalf("capsule missing %q", want)
		}
	}
	bad := e
	bad.Commits = append(bad.Commits, bad.Commits[0])
	bad.Commits[1].Sequence = 2
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("duplicate commit error = %v", err)
	}
}

func TestFinalizeCrashRetryAndExactReplay(t *testing.T) {
	root := t.TempDir()
	e := fixtureEvidence(t, root, func(raw []byte) { writeFixture(t, root, raw) })
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	ledgerStore, err := ledger.Open(context.Background(), filepath.Join(root, ".revolvr", "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledgerStore.Close()
	failOnce := true
	revalidate := func(context.Context, FrozenEvidence) error { return nil }
	_, err = Finalize(context.Background(), Config{RepositoryRoot: root, Evidence: e, StateStore: store, Ledger: ledgerStore, RevalidateEvidence: revalidate, FailureInjector: func(point FailurePoint) error {
		if point == FailureAfterCapsuleRename && failOnce {
			failOnce = false
			return fmt.Errorf("crash")
		}
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "crash") {
		t.Fatalf("first error = %v", err)
	}
	snapshot, found, err := store.Load(context.Background(), e.Task.TaskID)
	if err != nil || !found {
		t.Fatal(err)
	}
	if snapshot.State.Lifecycle != autonomous.LifecycleStateFinalizing || snapshot.State.Finalization.Stage != autonomous.FinalizationStageAdmitted {
		t.Fatalf("state after crash = %s/%s", snapshot.State.Lifecycle, snapshot.State.Finalization.Stage)
	}
	_, err = Finalize(context.Background(), Config{RepositoryRoot: root, Evidence: e, StateStore: store, Ledger: ledgerStore, RevalidateEvidence: func(context.Context, FrozenEvidence) error { return fmt.Errorf("source drift") }})
	if err == nil || !strings.Contains(err.Error(), "source drift") {
		t.Fatalf("stale retry error = %v", err)
	}
	result, err := Finalize(context.Background(), Config{RepositoryRoot: root, Evidence: e, StateStore: store, Ledger: ledgerStore, RevalidateEvidence: revalidate})
	if err != nil {
		t.Fatal(err)
	}
	if result.State.State.Lifecycle != autonomous.LifecycleStateCompleted || result.State.State.Finalization.Stage != autonomous.FinalizationStageLedgerCompleted {
		t.Fatalf("final state = %s/%s", result.State.State.Lifecycle, result.State.State.Finalization.Stage)
	}
	if result.Task.Status != taskfile.StatusCompleted || result.LedgerRun.Status != ledger.StatusCompleted {
		t.Fatalf("task/run = %s/%s", result.Task.Status, result.LedgerRun.Status)
	}
	replay, err := Finalize(context.Background(), Config{RepositoryRoot: root, Evidence: e, StateStore: store, Ledger: ledgerStore, RevalidateEvidence: revalidate})
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed {
		t.Fatal("exact retry was not classified as replay")
	}
	history, ok, err := ledgerStore.GetRunWithEvents(context.Background(), e.FinalizationRunID)
	if err != nil || !ok {
		t.Fatal(err)
	}
	counts := map[ledger.EventType]int{}
	for _, event := range history.Events {
		counts[event.Type]++
	}
	for _, kind := range []ledger.EventType{ledger.EventFinalizationPrepared, ledger.EventFinalizationMaterialized, ledger.EventFinalizationStateTerminal, ledger.EventFinalizationCompleted} {
		if counts[kind] != 1 {
			t.Fatalf("event %s count = %d", kind, counts[kind])
		}
	}
}

func TestFinalizeRedactsConfiguredSecretsFromCompletionArtifacts(t *testing.T) {
	root := t.TempDir()
	e := fixtureEvidence(t, root, func(raw []byte) { writeFixture(t, root, raw) })
	const secret = "secret-value"
	var err error
	e.Decision.Rationale = "Completed with " + secret
	e.State.Plan.Steps[0].Description = "Implement " + secret + " and verify the task."
	e.StateIdentity, err = autonomousstate.StateIdentityFor(e.Task.StatePath, true, e.State)
	if err != nil {
		t.Fatal(err)
	}
	stateRaw, err := autonomousstate.MarshalState(e.State)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(e.Task.StatePath)), stateRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	e.SafetyPolicy.Redaction = redact.Policy{SchemaVersion: redact.PolicySchemaVersion, EnvironmentVariables: []string{"TOKEN"}}
	identity, err := e.SafetyPolicy.Redaction.Identity()
	if err != nil {
		t.Fatal(err)
	}
	e.SafetyPolicy.RedactionPolicyHash = identity
	e.SafetyPolicy, err = autonomoussafety.FinalizePolicy(e.SafetyPolicy)
	if err != nil {
		t.Fatal(err)
	}
	e.SafetyPreflight.PolicySHA256 = e.SafetyPolicy.PolicySHA256
	e.Route, err = autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: e.Task.TaskID, Decision: e.Decision, Reference: e.DecisionReference, State: e.State, Source: e.Source, Verification: &e.Verification, Audit: &e.Audit})
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := redact.New(e.SafetyPolicy.Redaction, func(name string) (string, bool) { return secret, name == "TOKEN" })
	if err != nil {
		t.Fatal(err)
	}
	store, err := autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(context.Background(), filepath.Join(root, ".revolvr", "ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	result, err := Finalize(context.Background(), Config{RepositoryRoot: root, Evidence: e, StateStore: store, Ledger: l, Redactor: r, RevalidateEvidence: func(context.Context, FrozenEvidence) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []autonomous.FinalizationArtifact{result.FrozenEvidence, result.Capsule, result.Manifest} {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(artifact.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), secret) {
			t.Fatalf("secret leaked in %s", artifact.Path)
		}
	}
	capsule, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Capsule.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(capsule), redact.Replacement) {
		t.Fatal("capsule did not contain redaction marker")
	}
}

func fixtureEvidence(t *testing.T, root string, persist func([]byte)) FrozenEvidence {
	t.Helper()
	root = filepath.Clean(root)
	taskID := "final-task"
	statePath := filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json"))
	taskPath := filepath.ToSlash(filepath.Join(".agent", "tasks", taskID+".md"))
	taskRaw := []byte(fmt.Sprintf("---\nid: %s\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: %s\n---\n# Final task\n\nFinish it.\n", taskID, statePath))
	taskHash := hash(taskRaw)
	completedTaskRaw := []byte(strings.Replace(string(taskRaw), "status: pending", "status: completed", 1))
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	source := strings.Repeat("b", 64)
	head := strings.Repeat("a", 40)
	tree := strings.Repeat("c", 40)
	w := autonomous.TaskWorkspace{SchemaVersion: autonomous.WorkspaceSchemaVersion, TaskID: taskID, WorkspaceID: "workspace-one", ControlRoot: root, ExecutionRoot: filepath.Join(root, ".revolvr", "autonomous", "worktrees", "workspace-one"), GitCommonDir: filepath.Join(root, ".git"), BranchRef: "refs/heads/revolvr/tasks/final-task-one", OwnerMarker: filepath.Join(root, ".revolvr", "autonomous", "workspaces", "workspace-one.json"), BaselineSHA: head, HeadSHA: head, TreeSHA: tree, SourceRevision: source, Checkpoint: autonomous.WorkspaceCheckpoint{Sequence: 1, CommitSHA: head, TreeSHA: tree, SourceRevision: source, OperationID: "workspace-create", Provenance: "baseline", CreatedAt: now.Add(-time.Hour)}, Status: autonomous.WorkspaceStatusReady, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Minute)}
	ev := evidence(autonomous.EvidenceKindVerification, "verification-run/verification-final")
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, Plan: &autonomous.TaskPlan{TaskID: taskID, ID: "plan-one", Revision: 1, Provenance: []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, taskPath)}, Steps: []autonomous.PlanStep{{ID: "step-one", Description: "Implement and verify the task.", Status: autonomous.PlanStepStatusCompleted, Evidence: []autonomous.EvidenceReference{ev}}}, Completed: true}, AcceptanceCriteria: []autonomous.AcceptanceCriterion{{ID: "criterion-one", Requirement: "The task is verified.", Status: autonomous.AcceptanceStatusSatisfied, Evidence: []autonomous.EvidenceReference{ev}}}, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}, Workspace: &w}
	stateRaw, err := autonomousstate.MarshalState(state)
	if err != nil {
		t.Fatal(err)
	}
	stateID, err := autonomousstate.StateIdentityFor(statePath, true, state)
	if err != nil {
		t.Fatal(err)
	}
	if persist != nil {
		persist(taskRaw)
		abs := filepath.Join(root, filepath.FromSlash(statePath))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, stateRaw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	decision := autonomous.SupervisorDecision{TaskID: taskID, Action: autonomous.ActionComplete, Rationale: "All exact gates are satisfied.", Inputs: []autonomous.EvidenceReference{ev}}
	reference := autonomous.DecisionReference{DecisionID: "decision-complete", RunID: "supervisor-complete", TaskID: taskID, Action: autonomous.ActionComplete, Artifact: evidence(autonomous.EvidenceKindFile, ".revolvr/runs/supervisor-complete/decision.json"), CreatedAt: now.Add(-time.Minute)}
	planID := autonomousverification.PlanIdentity{SchemaVersion: autonomousverification.PlanSchemaVersion, SHA256: strings.Repeat("d", 64), ByteSize: 12}
	gate := autonomousverification.GateEvidence{SchemaVersion: autonomousverification.GateSchemaVersion, Plan: planID, Purpose: autonomousverification.PurposeFinal, RequiredFinalTiers: []string{"full-suite"}, SelectedTiers: []string{"full-suite"}, ExecutedTiers: []string{"full-suite"}, RequiredOutcomes: []autonomousverification.TierGate{{TierID: "full-suite", Outcome: autonomousverification.OutcomePassed}}, OverallOutcome: autonomousverification.OutcomePassed, FinalSatisfied: true}
	verification := autonomouspolicy.VerificationEvidence{Summary: autonomous.VerificationSummary{TaskID: taskID, Status: autonomous.VerificationStatusPassed, Summary: "All final tiers passed.", RunID: "verification-run", OccurrenceID: "verification-final", Evidence: []autonomous.EvidenceReference{ev}}, SourceRevision: source, Tiered: &gate}
	audit := autonomouspolicy.AuditEvidence{Report: autonomous.AuditReport{TaskID: taskID, Disposition: autonomous.AuditDispositionClean, Rationale: "Independent review is clean.", Inputs: []autonomous.EvidenceReference{ev}}, RunID: "audit-run", AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: source, VerificationRunID: "verification-run", VerificationOccurrenceID: "verification-final"}
	sourceEvidence := autonomouspolicy.SourceEvidence{Revision: source, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: &autonomouspolicy.SourceMutation{TaskID: taskID, RunID: "worker-run", DecisionID: "decision-worker", Action: autonomous.ActionImplement, ResultingRevision: source}}
	configHash := strings.Repeat("e", 64)
	policy := autonomoussafety.Policy{SchemaVersion: autonomoussafety.PolicySchemaVersion, TaskID: taskID, Workspace: w, Mode: autonomoussafety.ModeOperatorAttended, Codex: autonomoussafety.CodexPolicy{Sandbox: "danger-full-access", ApprovalPolicy: "never", DangerousBypass: true, Model: "gpt-5.6-sol", ReasoningEffort: "xhigh", Ephemeral: true}, ExternalIsolation: autonomoussafety.ExternalIsolation{Expectation: autonomoussafety.IsolationNone, Enforcement: autonomoussafety.EnforcementNone}, Network: autonomoussafety.NetworkPolicy{Access: autonomoussafety.NetworkUnknown, Enforcement: autonomoussafety.EnforcementNone}, Hooks: autonomoussafety.HookTrust{Policy: autonomoussafety.HooksOperatorAttended}, Environment: autonomoussafety.EnvironmentPolicy{InheritHost: true}, Redaction: redact.Policy{SchemaVersion: redact.PolicySchemaVersion}, RedactionPolicyHash: strings.Repeat("f", 64), ConfigPath: ".revolvr/config.yaml", ConfigSHA256: configHash, WorktreeNotice: "Git worktree isolation is source/Git isolation, not a security sandbox."}
	policy, err = autonomoussafety.FinalizePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	preflight := autonomoussafety.PreflightResult{SchemaVersion: autonomoussafety.PreflightSchemaVersion, TaskID: taskID, WorkspaceID: w.WorkspaceID, SourceRevision: source, PolicySHA256: policy.PolicySHA256, ConfigSHA256: configHash, ObservedAt: now.Add(-30 * time.Second), Ready: true, Checks: []autonomoussafety.Check{{Name: "ready", Status: autonomoussafety.CheckOK, Detail: "exact authority ready"}}}
	e := FrozenEvidence{SchemaVersion: FrozenEvidenceSchemaVersion, OperationID: "finalize-one", FinalizationRunID: "finalization-run", Task: TaskSource{TaskID: taskID, Title: "Final task", Path: taskPath, SHA256: taskHash, ByteSize: len(taskRaw), Workflow: taskfile.WorkflowAutonomousV1, StatePath: statePath, CompletedSHA256: hash(completedTaskRaw), CompletedByteSize: len(completedTaskRaw)}, State: state, StateIdentity: stateID, Decision: decision, DecisionReference: reference, Source: sourceEvidence, Verification: verification, Audit: audit, Workspace: w, SafetyPolicy: policy, SafetyPreflight: preflight, EffectiveConfigSchema: "revolvr-effective-run-config-v2", EffectiveConfigSHA256: configHash, Runs: []RunEvidence{{Sequence: 1, RunID: "supervisor-complete", Kind: "supervisor", Outcome: "completed", Artifact: reference.Artifact, StartedAt: now.Add(-2 * time.Minute), CompletedAt: now.Add(-time.Minute)}, {Sequence: 2, RunID: "verification-run", Kind: "verification", Outcome: "passed", Artifact: ev, StartedAt: now.Add(-time.Minute), CompletedAt: now.Add(-45 * time.Second)}, {Sequence: 3, RunID: "audit-run", Kind: "audit", Outcome: "clean", Artifact: evidence(autonomous.EvidenceKindAudit, "audit-run/report"), StartedAt: now.Add(-40 * time.Second), CompletedAt: now.Add(-30 * time.Second)}}, Provenance: []autonomous.EvidenceReference{evidence(autonomous.EvidenceKindTask, taskPath)}, AdmittedAt: now, TerminalAt: now.Add(time.Second)}
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: taskID, Decision: decision, Reference: reference, State: state, Source: sourceEvidence, Verification: &verification, Audit: &audit})
	if err != nil {
		t.Fatal(err)
	}
	e.Route = route
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	return e
}

func writeFixture(t *testing.T, root string, taskRaw []byte) {
	t.Helper()
	path := filepath.Join(root, ".agent", "tasks", "final-task.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, taskRaw, 0o644); err != nil {
		t.Fatal(err)
	}
}
func evidence(kind autonomous.EvidenceKind, reference string) autonomous.EvidenceReference {
	return autonomous.EvidenceReference{Kind: kind, Reference: reference, Detail: "trusted harness evidence"}
}
