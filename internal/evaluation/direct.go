package evaluation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DirectExecutor is the sole implemented Architecture 022 worker mode. Its
// model, embedding, sandbox, clock, and effect boundaries are deterministic
// fakes; source identity and original-checkout proofs use local Git fixtures.
type DirectExecutor struct {
	RepositoryRoot string
	WorkRoot       string
}

func (e *DirectExecutor) Execute(ctx context.Context, request ExecutionRequest) (Result, error) {
	if request.Mode != DirectToolsV1 {
		return Result{}, &ModeRefusalError{Mode: request.Mode, Code: "not_implemented_not_admitted"}
	}
	if err := request.Scenario.Validate(); err != nil {
		return Result{}, err
	}
	if request.Authority.SchemaVersion != AuthoritySchemaVersion || request.Authority.ScenarioID != request.Scenario.ID || request.Authority.SHA256 == "" {
		return Result{}, errors.New("evaluation: immutable execution authority is incomplete")
	}
	authorityMaterial := request.Authority
	authorityMaterial.SHA256 = ""
	authoritySHA, err := hashValue(authorityMaterial)
	if err != nil || authoritySHA != request.Authority.SHA256 {
		return Result{}, errors.New("evaluation: immutable execution authority changed")
	}
	source, err := prepareFixtureSource(ctx, e.RepositoryRoot, e.WorkRoot, request)
	if err != nil {
		return Result{}, err
	}
	retrieved, err := retrieveAndCompile(ctx, request, source)
	if err != nil {
		return Result{}, err
	}
	clock := newDeterministicClock()
	model := &deterministicModel{}
	sandboxRuntime := &deterministicSandbox{}
	if workerReached(request.Scenario.Behavior) {
		if err := sandboxRuntime.Start(ctx, request); err != nil {
			return Result{}, err
		}
		if _, err := model.Invoke(ctx, request); err != nil {
			return Result{}, err
		}
	}
	measured, err := measureScriptedExecution(request.Scenario)
	if err != nil {
		return Result{}, err
	}
	candidateCommit, err := source.candidate(ctx, request.Scenario)
	if err != nil {
		return Result{}, err
	}
	sandboxRuntime.Remove()
	clock.Advance(time.Duration(request.Scenario.LogicalWallTimeMS) * time.Millisecond)
	originalAfter, originalUnchanged, err := source.originalAfter(ctx)
	if err != nil {
		return Result{}, err
	}
	actualOutcome, actualStopReason := behaviorOutcome(request.Scenario.Behavior)
	result := Result{
		SchemaVersion:   ResultSchemaVersion,
		ScenarioID:      request.Scenario.ID,
		WorkerMode:      request.Mode,
		AuthoritySHA256: request.Authority.SHA256,
		Outcome:         actualOutcome,
		StopReason:      actualStopReason,
		Workspace: WorkspaceFact{
			BaselineCommit: source.baselineCommit, CandidateCommit: candidateCommit,
			OriginalCheckoutBefore: source.originalBefore, OriginalCheckoutAfter: originalAfter,
			OriginalCheckoutUnchanged: originalUnchanged,
		},
		Retrieval: retrieved.fact,
		Metrics: Metrics{
			SchemaVersion: MetricsSchemaVersion, WorkerExecutionMode: DirectToolsV1,
			ContextBytes:            len(retrieved.dossier),
			DirectToolCount:         measured.directTools,
			RepeatedReadCount:       measured.repeatedReads,
			VerificationExecutions:  measured.verificationRuns,
			VerificationExactReuses: measured.verificationReuses,
			CorrectionCycles:        measured.correctionCycles,
			WallTimeNanoseconds:     clock.Elapsed().Nanoseconds(),
			FinalTypedOutcome:       actualOutcome,
			Omissions:               tokenOmissions(),
		},
		LeaseAcquired: leaseAcquired(request.Scenario.Behavior), LeaseReleased: true,
		Safety: SafetyFact{
			NoLiveModel: true, NoPublicNetwork: true, NoAmbientCredentials: true, NoOperatorHomeData: true,
			NoRuntimeSocket: true, OriginalCheckoutIntact: originalUnchanged,
		},
	}
	if err := applyOutcomeState(&result, request, measured); err != nil {
		return Result{}, err
	}
	if workerReached(request.Scenario.Behavior) && (!sandboxRuntime.removed || model.calls != 1) {
		return Result{}, errors.New("evaluation: deterministic worker fakes did not settle")
	}
	accepted, err := (&deterministicAcceptance{}).Evaluate(request, result)
	if err != nil || accepted != result.Completion.Authorized {
		return Result{}, fmt.Errorf("evaluation: completion authority mismatch: accepted=%t err=%v", accepted, err)
	}
	result.Events = eventFacts(request, eventTrace(request.Scenario.Behavior))
	for _, boundary := range request.Scenario.CrashBoundaries {
		fact, err := exerciseCrashBoundary(boundary, request.Scenario.ID)
		if err != nil {
			return Result{}, err
		}
		result.CrashReplays = append(result.CrashReplays, fact)
	}
	result.Artifacts, err = resultArtifacts(result, retrieved.dossier, retrieved.manifest)
	if err != nil {
		return Result{}, err
	}
	if result.Completion.Authorized {
		capsule, err := Canonical(struct {
			SchemaVersion   string         `json:"schema_version"`
			ScenarioID      string         `json:"scenario_id"`
			AuthoritySHA256 string         `json:"authority_sha256"`
			Outcome         string         `json:"outcome"`
			Artifacts       []ArtifactFact `json:"artifacts"`
		}{"revolvr-evaluation-completion-capsule-v1", result.ScenarioID, result.AuthoritySHA256, result.Outcome, result.Artifacts})
		if err != nil {
			return Result{}, err
		}
		result.Completion.CapsuleSHA256 = hashBytes(capsule)
		result.Artifacts = append(result.Artifacts, ArtifactFact{Kind: "completion_capsule", SHA256: result.Completion.CapsuleSHA256, SizeBytes: len(capsule)})
	}
	if err := validateResult(result, request); err != nil {
		return Result{}, err
	}
	return result, nil
}

func tokenOmissions() []Omission {
	return []Omission{
		{Field: "tokens.cached", Reason: "not_reported_by_deterministic_fake"},
		{Field: "tokens.input", Reason: "not_reported_by_deterministic_fake"},
		{Field: "tokens.output", Reason: "not_reported_by_deterministic_fake"},
		{Field: "tokens.reasoning", Reason: "not_reported_by_deterministic_fake"},
	}
}

func applyOutcomeState(result *Result, request ExecutionRequest, measured measuredExecution) error {
	behavior := request.Scenario.Behavior
	completed := behaviorCompleted(behavior)
	result.Task = stateFact("task", taskStatus(behavior), "applicable", request.Authority.SHA256)
	result.Run = stateFact("run", runStatus(behavior), runApplicability(behavior), request.Authority.SHA256)
	result.Plan = stateFact("plan", planStatus(behavior), planApplicability(behavior), request.Authority.SHA256)
	criterionStatus := "pending"
	if completed {
		criterionStatus = "passed"
	}
	criterion := CriterionFact{CriterionID: "ac-1", Status: criterionStatus}
	if completed {
		criterion.EvidenceSHA256 = hashBytes([]byte(request.Scenario.ID + ":criterion:passed"))
	}
	result.Criteria = []CriterionFact{criterion}
	if behavior == "audit_correction" {
		result.Findings = []FindingFact{{FindingID: "audit-finding-1", Status: "resolved", DefinitionSHA256: hashBytes([]byte(request.Scenario.ID + ":audit-finding-1"))}}
	}
	workspaceApplicable := workspaceApplicability(behavior)
	result.Workspace.State = stateFact("workspace", workspaceStatus(behavior), workspaceApplicable, request.Authority.SHA256)
	result.Workspace.Cleaned = workspaceApplicable == "applicable"
	result.Sandbox = SandboxFact{
		State:   stateFact("sandbox", sandboxStatus(behavior), sandboxApplicability(behavior), request.Authority.SHA256),
		Profile: "strict", Network: "none", AmbientEnv: false, RuntimeSocket: false, OriginalSource: false,
	}
	result.Verification = VerificationFact{
		State:      stateFact("verification", verificationStatus(behavior), verificationApplicability(behavior), request.Authority.SHA256),
		Executions: measured.verificationRuns, ExactReuses: measured.verificationReuses,
		Occurrences: measured.verificationRuns + measured.verificationReuses,
		FreshFinal:  completed,
	}
	auditRuns := 0
	blockingFindings := 0
	if completed {
		auditRuns = 1
	}
	if behavior == "audit_correction" {
		auditRuns = 2
		blockingFindings = 1
	}
	result.Audit = AuditFact{
		State: stateFact("audit", auditStatus(behavior), auditApplicability(behavior), request.Authority.SHA256),
		Runs:  auditRuns, Independent: auditRuns > 0, BlockingFindings: blockingFindings,
	}
	result.Completion = CompletionFact{
		State:      stateFact("completion", completionStatus(behavior), completionApplicability(behavior), request.Authority.SHA256),
		Authorized: completed,
	}
	return nil
}

func stateFact(kind, status, applicability, authority string) StateFact {
	value := StateFact{Status: status, Applicability: applicability}
	value.SHA256 = hashBytes([]byte(kind + "\x00" + status + "\x00" + applicability + "\x00" + authority))
	return value
}

func behaviorCompleted(behavior string) bool {
	switch behavior {
	case "straight_success", "compile_correction", "test_correction", "audit_correction", "crash_state", "crash_external", "stale_index", "missing_embeddings":
		return true
	default:
		return false
	}
}

func behaviorOutcome(behavior string) (string, string) {
	switch behavior {
	case "straight_success", "compile_correction", "test_correction", "audit_correction":
		return "completed", "completed"
	case "ambiguity":
		return "needs_input", "ambiguity_requires_operator"
	case "missing_dependency":
		return "dependency_missing", "dependency_missing"
	case "cyclic_dependency":
		return "dependency_cycle", "dependency_cycle"
	case "scope_violation":
		return "scope_violation", "scope_violation"
	case "protected_path":
		return "protected_path_violation", "protected_path_violation"
	case "repeated_strategy":
		return "repeated_strategy_denied", "no_progress_repeated_strategy"
	case "no_changes":
		return "no_changes", "no_changes"
	case "test_tampering":
		return "test_tampering", "verification_authority_tampered"
	case "mid_run_source_change":
		return "source_revision_changed", "source_revision_changed"
	case "cancellation":
		return "cancelled", "cancelled"
	case "crash_state", "crash_external":
		return "completed_after_recovery", "completed"
	case "stale_index", "missing_embeddings":
		return "completed_degraded_retrieval", "completed"
	case "sandbox_timeout":
		return "sandbox_timeout", "sandbox_timeout"
	case "network_denied_install":
		return "network_denied", "network_denied_dependency_install"
	default:
		return "unsafe_or_ambiguous", "unknown_behavior"
	}
}

func taskStatus(behavior string) string {
	if behaviorCompleted(behavior) {
		return "completed"
	}
	switch behavior {
	case "ambiguity":
		return "needs_input"
	case "missing_dependency", "cyclic_dependency":
		return "pending"
	case "scope_violation", "protected_path", "test_tampering", "mid_run_source_change":
		return "unsafe"
	case "cancellation":
		return "cancelled"
	default:
		return "blocked"
	}
}

func runStatus(behavior string) string {
	if behaviorCompleted(behavior) {
		return "completed"
	}
	if behavior == "missing_dependency" || behavior == "cyclic_dependency" {
		return "not_admitted"
	}
	return "stopped"
}

func runApplicability(behavior string) string {
	if behavior == "missing_dependency" || behavior == "cyclic_dependency" {
		return "not_admitted"
	}
	return "applicable"
}

func leaseAcquired(behavior string) bool {
	return behavior != "missing_dependency" && behavior != "cyclic_dependency"
}

func planStatus(behavior string) string {
	if behavior == "missing_dependency" || behavior == "cyclic_dependency" || behavior == "ambiguity" {
		return "not_created"
	}
	if behaviorCompleted(behavior) {
		return "completed"
	}
	return "accepted"
}

func planApplicability(behavior string) string {
	if planStatus(behavior) == "not_created" {
		return "not_reached"
	}
	return "applicable"
}

func workspaceApplicability(behavior string) string {
	switch behavior {
	case "missing_dependency", "cyclic_dependency", "ambiguity":
		return "not_reached"
	default:
		return "applicable"
	}
}

func workspaceStatus(behavior string) string {
	if workspaceApplicability(behavior) == "not_reached" {
		return "not_created"
	}
	return "cleaned"
}

func sandboxApplicability(behavior string) string {
	switch behavior {
	case "missing_dependency", "cyclic_dependency", "ambiguity":
		return "not_reached"
	default:
		return "applicable"
	}
}

func sandboxStatus(behavior string) string {
	if sandboxApplicability(behavior) == "not_reached" {
		return "not_created"
	}
	return "removed"
}

func verificationStatus(behavior string) string {
	if behaviorCompleted(behavior) {
		return "passed"
	}
	switch behavior {
	case "repeated_strategy":
		return "failed"
	case "test_tampering":
		return "authority_tampered"
	case "cancellation":
		return "cancelled"
	case "sandbox_timeout":
		return "timed_out"
	default:
		return "not_run"
	}
}

func verificationApplicability(behavior string) string {
	if verificationStatus(behavior) == "not_run" {
		return "not_reached"
	}
	return "applicable"
}

func auditStatus(behavior string) string {
	if behaviorCompleted(behavior) {
		return "clean"
	}
	return "not_run"
}

func auditApplicability(behavior string) string {
	if behaviorCompleted(behavior) {
		return "applicable"
	}
	return "not_reached"
}

func completionStatus(behavior string) string {
	if behaviorCompleted(behavior) {
		return "materialized"
	}
	return "not_authorized"
}

func completionApplicability(behavior string) string {
	if behaviorCompleted(behavior) {
		return "applicable"
	}
	return "rejected_or_not_reached"
}

func eventFacts(request ExecutionRequest, eventTypes []string) []EventFact {
	result := make([]EventFact, 0, len(eventTypes))
	for index, eventType := range eventTypes {
		sequence := index + 1
		sha := hashBytes([]byte(fmt.Sprintf("%s\x00%d\x00%s\x00%s", request.Scenario.ID, sequence, eventType, request.Authority.SHA256)))
		result = append(result, EventFact{Sequence: sequence, Type: eventType, SHA256: sha})
	}
	return result
}

func eventTrace(behavior string) []string {
	prefix := []string{"task_admitted", "plan_accepted", "workspace_ready", "sandbox_running", "worker_completed", "candidate_captured"}
	completedTail := []string{"verification_passed", "audit_clean", "completion_materialized", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned"}
	switch behavior {
	case "straight_success":
		return append(prefix, completedTail...)
	case "compile_correction":
		return append(prefix, "compile_verification_failed", "correction_admitted", "corrector_completed", "verification_passed", "audit_clean", "completion_materialized", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "test_correction":
		return append(prefix, "test_verification_failed", "correction_admitted", "corrector_completed", "verification_passed", "audit_clean", "completion_materialized", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "audit_correction":
		return append(prefix, "verification_passed", "audit_changes_required", "finding_opened", "correction_admitted", "corrector_completed", "verification_passed", "audit_clean", "finding_resolved", "completion_materialized", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "ambiguity":
		return []string{"task_admitted", "supervisor_needs_input", "task_needs_input", "lease_released"}
	case "missing_dependency":
		return []string{"dependency_graph_refused_missing", "lease_not_acquired"}
	case "cyclic_dependency":
		return []string{"dependency_graph_refused_cycle", "lease_not_acquired"}
	case "scope_violation":
		return append(prefix[:5], "scope_change_denied", "task_unsafe", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "protected_path":
		return append(prefix[:5], "protected_path_change_denied", "task_unsafe", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "repeated_strategy":
		return append(prefix, "verification_failed", "correction_failed", "repeated_strategy_denied", "task_blocked", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "no_changes":
		return append(prefix[:5], "source_capture_no_changes", "task_blocked", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "test_tampering":
		return append(prefix[:5], "verification_authority_tampered", "task_unsafe", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "mid_run_source_change":
		return append(prefix, "source_revision_changed", "task_unsafe", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "cancellation":
		return []string{"task_admitted", "plan_accepted", "workspace_ready", "sandbox_running", "cancellation_requested", "sandbox_cancelled", "task_cancelled", "lease_released", "sandbox_removed", "workspace_cleaned"}
	case "crash_state":
		return append(prefix, "crash_before_sandbox_recovered", "verification_passed", "audit_clean", "completion_materialized", "crash_after_completion_artifacts_recovered", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "crash_external":
		return append(prefix, "sandbox_effect_reconciled", "worker_output_reconciled", "candidate_commit_reconciled", "external_export_reconciled", "verification_passed", "audit_clean", "completion_materialized", "task_completed", "lease_released", "sandbox_removed", "workspace_cleaned")
	case "stale_index":
		return append([]string{"retrieval_index_stale_exact_lexical_retained"}, append(prefix, completedTail...)...)
	case "missing_embeddings":
		return append([]string{"embedding_service_degraded_exact_lexical_retained"}, append(prefix, completedTail...)...)
	case "sandbox_timeout":
		return []string{"task_admitted", "plan_accepted", "workspace_ready", "sandbox_running", "sandbox_timed_out", "task_blocked", "lease_released", "sandbox_removed", "workspace_cleaned"}
	case "network_denied_install":
		return []string{"task_admitted", "plan_accepted", "workspace_ready", "sandbox_running", "network_dependency_install_denied", "task_blocked", "lease_released", "sandbox_removed", "workspace_cleaned"}
	default:
		return []string{"unknown_behavior_refused", "lease_released"}
	}
}

func resultArtifacts(result Result, dossier, manifest []byte) ([]ArtifactFact, error) {
	events, err := Canonical(result.Events)
	if err != nil {
		return nil, err
	}
	artifacts := []ArtifactFact{
		{Kind: "context_dossier", SHA256: hashBytes(dossier), SizeBytes: len(dossier)},
		{Kind: "context_manifest", SHA256: hashBytes(manifest), SizeBytes: len(manifest)},
		{Kind: "event_timeline", SHA256: hashBytes(events), SizeBytes: len(events)},
	}
	if result.Workspace.CandidateCommit != "" {
		material := []byte(result.Workspace.BaselineCommit + "\n" + result.Workspace.CandidateCommit + "\n")
		artifacts = append(artifacts, ArtifactFact{Kind: "candidate_source", SHA256: hashBytes(material), SizeBytes: len(material)})
	}
	if result.Verification.Occurrences > 0 {
		material, err := Canonical(result.Verification)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, ArtifactFact{Kind: "verification_evidence", SHA256: hashBytes(material), SizeBytes: len(material)})
	}
	if result.Audit.Runs > 0 {
		material, err := Canonical(struct {
			Audit    AuditFact     `json:"audit"`
			Findings []FindingFact `json:"findings"`
		}{result.Audit, result.Findings})
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, ArtifactFact{Kind: "audit_evidence", SHA256: hashBytes(material), SizeBytes: len(material)})
	}
	return artifacts, nil
}

func validateResult(result Result, request ExecutionRequest) error {
	if result.SchemaVersion != ResultSchemaVersion || result.WorkerMode != DirectToolsV1 || result.AuthoritySHA256 != request.Authority.SHA256 {
		return errors.New("evaluation: result identity is invalid")
	}
	if result.Outcome != request.Authority.Expected.Outcome || result.StopReason != request.Authority.Expected.StopReason || result.Metrics.FinalTypedOutcome != result.Outcome {
		return errors.New("evaluation: result differs from frozen expected-outcome authority")
	}
	if !result.LeaseReleased || !result.Workspace.OriginalCheckoutUnchanged || !result.Safety.OriginalCheckoutIntact || !result.Safety.NoLiveModel || !result.Safety.NoPublicNetwork || !result.Safety.NoAmbientCredentials || !result.Safety.NoOperatorHomeData || !result.Safety.NoRuntimeSocket {
		return errors.New("evaluation: safety or cleanup invariant failed")
	}
	if len(result.Criteria) != len(request.Authority.Acceptance) || !result.Retrieval.ExactSourceFirst {
		return errors.New("evaluation: acceptance or exact retrieval authority is incomplete")
	}
	if len(result.Metrics.Omissions) != 4 || result.Metrics.Tokens.Input != nil || result.Metrics.Tokens.Output != nil || result.Metrics.Tokens.Reasoning != nil || result.Metrics.Tokens.Cached != nil {
		return errors.New("evaluation: token omissions are incomplete")
	}
	for index, event := range result.Events {
		if event.Sequence != index+1 || event.Type == "" || event.SHA256 == "" {
			return errors.New("evaluation: event sequence is invalid")
		}
	}
	return nil
}
