package autonomouscycle

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousverification"
	"revolvr/internal/codexexec"
	"revolvr/internal/commit"
	"revolvr/internal/gitstate"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
	"revolvr/internal/verification"
)

func runWorker(
	ctx context.Context,
	n normalizedConfig,
	task taskfile.Task,
	dossier autonomous.TaskDossier,
	supervisorResult supervisor.Result,
	route autonomouspolicy.Route,
	admission gitstate.SourceSnapshot,
	result Result,
) (Result, error) {
	profile, err := n.ProfileLoader(n.root, string(route.WorkerProfile))
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_profile", err)
	}
	if profile.Name != string(route.WorkerProfile) || strings.TrimSpace(profile.SourcePath) == "" || strings.TrimSpace(profile.Description) == "" {
		return failed(result, OutcomeWorkerFailed, "worker_profile", fmt.Errorf("loaded profile does not exactly match policy-selected profile %q", route.WorkerProfile))
	}
	role, err := autonomous.RoleForAction(route.Action, route.WorkerProfile)
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_dossier_role", err)
	}
	dossier, err = autonomous.ReprojectTaskDossier(dossier, role)
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_dossier_projection", err)
	}
	profileRaw := []byte(profile.Description)
	result.Worker.Action = route.Action
	result.Worker.Profile = ProfileEvidence{
		Name: profile.Name, Path: profile.SourcePath,
		SHA256: sha256Hex(profileRaw), ByteSize: len(profileRaw),
	}

	preRunDirty, err := n.DirtyCapture(ctx, gitConfig(n))
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "pre_worker_dirty_capture", err)
	}
	if preRunDirty.CaptureError != "" {
		return failed(result, OutcomeWorkerFailed, "pre_worker_dirty_capture", errors.New(preRunDirty.CaptureError))
	}
	if sourceChangingAction(route.Action) && !n.AllowPreExistingDirty && captureHasPaths(preRunDirty) {
		return failed(result, OutcomeWorkerFailed, "pre_worker_source_safety", fmt.Errorf("pre-existing dirty files are present: %q", capturePaths(preRunDirty)))
	}

	stateBefore, err := captureOptionalFile(n.root, task.AutonomousStatePath)
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "state_evidence_before", err)
	}
	workerRunID := strings.TrimSpace(n.IDGenerator())
	if !safePathID(workerRunID) {
		return failed(result, OutcomeWorkerFailed, "worker_identity", fmt.Errorf("generated unsafe worker run ID %q", workerRunID))
	}
	if workerRunID == supervisorResult.RunID {
		return failed(result, OutcomeWorkerFailed, "worker_identity", errors.New("worker run ID must differ from supervisor run ID"))
	}
	result.Worker.RunID = workerRunID
	paths, err := prepareWorkerPaths(n.root, workerRunID, route.Action)
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_artifacts", err)
	}
	result.Worker.Artifacts = workerArtifactsWithPaths(paths)
	if route.Action == autonomous.ActionPlan || route.Action == autonomous.ActionAudit || route.Action == autonomous.ActionCorrect {
		var schema []byte
		var schemaErr error
		stage := "planner_output_schema"
		if route.Action == autonomous.ActionPlan {
			schema, schemaErr = autonomousplanning.PlanningOutputSchema()
		} else if route.Action == autonomous.ActionAudit {
			stage = "auditor_output_schema"
			schema, schemaErr = autonomousaudit.AuditOutputSchema()
		} else {
			stage = "corrector_output_schema"
			schema, schemaErr = autonomous.CorrectionOutputSchema()
		}
		if schemaErr != nil {
			return failed(result, OutcomeWorkerFailed, stage, schemaErr)
		}
		artifact, schemaErr := writeArtifact(n.root, paths.outputSchema, schema)
		if schemaErr != nil {
			return failed(result, OutcomeWorkerFailed, stage, schemaErr)
		}
		result.Worker.Artifacts.OutputSchema = &artifact
	}

	invocation, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{
		Executable:             n.CodexExecutable,
		WorkingDir:             n.executionRoot,
		ArtifactRoot:           n.root,
		Model:                  n.CodexModel,
		ReasoningEffort:        n.CodexReasoningEffort,
		Ephemeral:              n.CodexEphemeral,
		Sandbox:                n.CodexSandbox,
		ApprovalPolicy:         n.CodexApprovalPolicy,
		BypassApprovalsSandbox: n.CodexBypassApprovalsAndSandbox,
		Artifacts: codexexec.ArtifactPaths{
			StdoutJSONL: paths.codexStdout,
			Stderr:      paths.codexStderr,
			LastMessage: paths.output,
		},
		OutputSchema:          paths.outputSchema,
		CodexVersion:          n.CodexVersion,
		EffectiveConfigSchema: n.EffectiveConfigSchema,
		EffectiveConfigSHA256: n.EffectiveConfigSHA256,
		SafetyPolicySHA256:    n.safetyPolicySHA256,
	})
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_invocation", err)
	}
	if err := invocation.Validate(); err != nil || !sameCodexIntent(n, invocation) {
		if err == nil {
			err = errors.New("worker invocation does not match explicit Codex configuration")
		}
		return failed(result, OutcomeWorkerFailed, "worker_invocation", err)
	}
	result.Worker.Invocation = invocation

	promptBytes, err := buildWorkerPrompt(workerPromptInput{
		Task:              task,
		Dossier:           dossier,
		Decision:          *supervisorResult.Decision,
		Reference:         *supervisorResult.DecisionReference,
		Route:             route,
		Profile:           profile,
		RunID:             workerRunID,
		ReceiptPath:       paths.receipt,
		OutputPath:        paths.output,
		SourceRevision:    result.Source.AdmissionRevision,
		Verification:      n.Verification,
		Audit:             n.Audit,
		LatestMutation:    n.LatestMutation,
		CorrectionFailure: n.CorrectionFailure,
	})
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_prompt", err)
	}
	if n.redactor != nil {
		promptBytes = []byte(n.redactor.String(string(promptBytes)))
	}
	run, err := n.Ledger.CreateRun(ctx, ledger.RunSpec{
		ID:        workerRunID,
		TaskID:    n.TaskID,
		Task:      task.ContextBody,
		StartedAt: n.Clock().UTC(),
	})
	if err != nil {
		return failed(result, OutcomeWorkerFailed, "worker_ledger", err)
	}
	result.Worker.Run = run
	appendWorkerEvent(ctx, n, &result, ledger.EventRunStarted, workerEventIdentity(result, route))
	appendWorkerEvent(ctx, n, &result, ledger.EventTaskSelected, workerTaskSelectedEvent(task, route, supervisorResult.DecisionReference.DecisionID))
	appendWorkerEvent(ctx, n, &result, ledger.EventRunArtifacts, ledger.RunArtifacts{
		ContextPayloadPath:   paths.prompt,
		ContextManifestPath:  paths.provenance,
		DossierPath:          paths.dossier,
		DossierManifestPath:  paths.dossierManifest,
		CodexStdoutJSONLPath: paths.codexStdout,
		CodexStderrPath:      paths.codexStderr,
		LastMessagePath:      paths.output,
		ReceiptPath:          paths.receipt,
	})
	if result.Worker.LedgerError != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_ledger", result.Worker.LedgerError, receipt.VerdictBlocked, "not_run", "")
	}

	manifestBytes, err := autonomous.MarshalTaskDossierManifest(dossier.Manifest)
	if err == nil {
		result.Worker.Artifacts.Dossier, err = writeArtifact(n.root, paths.dossier, dossier.Markdown)
	}
	if err == nil {
		result.Worker.Artifacts.DossierManifest, err = writeArtifact(n.root, paths.dossierManifest, manifestBytes)
	}
	if err == nil {
		result.Worker.Artifacts.Prompt, err = writeArtifact(n.root, paths.prompt, promptBytes)
	}
	var provenanceBytes []byte
	if err == nil {
		provenanceBytes, err = marshalIndented(workerProvenance{
			SchemaVersion:           workerProvenanceSchemaVersion,
			RunID:                   workerRunID,
			TaskID:                  n.TaskID,
			Dossier:                 dossier.Manifest,
			Decision:                *supervisorResult.Decision,
			Reference:               *supervisorResult.DecisionReference,
			Route:                   route,
			Profile:                 result.Worker.Profile,
			Invocation:              invocation,
			Artifacts:               result.Worker.Artifacts,
			PromptByteSize:          len(promptBytes),
			PromptTokenEstimator:    autonomous.DossierTokenEstimatorSchema,
			PromptTokenEstimate:     (len(promptBytes) + 3) / 4,
			AdmissionSourceRevision: result.Source.AdmissionRevision,
			SafetyPolicy:            result.SafetyPolicy,
			SafetyPreflight:         result.SafetyPreflight,
		})
	}
	if err == nil {
		result.Worker.Artifacts.Provenance, err = writeArtifact(n.root, paths.provenance, provenanceBytes)
	}
	if err != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_artifact", err, receipt.VerdictBlocked, "not_run", "")
	}
	appendWorkerEvent(ctx, n, &result, ledger.EventContextBuilt, map[string]any{
		"context_payload_path":      paths.prompt,
		"context_manifest_path":     paths.provenance,
		"context_payload_sha256":    result.Worker.Artifacts.Prompt.SHA256,
		"context_payload_byte_size": result.Worker.Artifacts.Prompt.ByteSize,
		"dossier_path":              paths.dossier,
		"dossier_manifest_path":     paths.dossierManifest,
		"dossier_sha256":            result.Worker.Artifacts.Dossier.SHA256,
		"dossier_byte_size":         result.Worker.Artifacts.Dossier.ByteSize,
		"receipt_path":              paths.receipt,
		"action":                    route.Action,
		"profile_name":              route.WorkerProfile,
		"decision_id":               route.DecisionID,
		"source_revision":           route.SourceRevision,
		"invocation":                invocation,
	})
	if result.Worker.LedgerError != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_ledger", result.Worker.LedgerError, receipt.VerdictBlocked, "not_run", "")
	}

	result.Worker.Started = true
	codexResult, codexErr := n.CodexRunner(ctx, codexexec.Config{
		Executable:                n.CodexExecutable,
		WorkingDir:                n.executionRoot,
		ArtifactRoot:              n.root,
		Prompt:                    string(promptBytes),
		Model:                     n.CodexModel,
		ReasoningEffort:           n.CodexReasoningEffort,
		Ephemeral:                 boolPointer(true),
		Timeout:                   n.CodexTimeout,
		StdoutCap:                 n.CodexStdoutCap,
		StderrCap:                 n.CodexStderrCap,
		Sandbox:                   n.CodexSandbox,
		ApprovalPolicy:            n.CodexApprovalPolicy,
		BypassApprovalsAndSandbox: n.CodexBypassApprovalsAndSandbox,
		Artifacts: codexexec.ArtifactPaths{
			StdoutJSONL: paths.codexStdout,
			Stderr:      paths.codexStderr,
			LastMessage: paths.output,
		},
		OutputSchema:  paths.outputSchema,
		RunID:         workerRunID,
		Ledger:        n.Ledger,
		CommandRunner: codexexec.CommandRunner(n.CommandRunner),
		Provenance:    invocation,
		Redactor:      n.redactor,
	})
	if codexErr != nil {
		if codexResult.ExitCode == 0 {
			codexResult.ExitCode = -1
		}
		codexResult.Err = codexErr
	}
	result.Worker.Codex = codexResult
	if codexResult.LedgerError != nil {
		setWorkerLedgerError(&result, codexResult.LedgerError)
	}
	if err := preserveWorkerOutput(n.root, paths.output, &result.Worker); err != nil && result.Worker.Codex.ArtifactError == nil {
		result.Worker.Codex.ArtifactError = err
	}
	result.Worker.Artifacts.CodexStdout, _ = referenceArtifact(n.root, paths.codexStdout)
	result.Worker.Artifacts.CodexStderr, _ = referenceArtifact(n.root, paths.codexStderr)

	workerAfter, snapshotErr := captureSource(ctx, n)
	if snapshotErr == nil {
		result.Source.WorkerAfter = &workerAfter
		result.Source.WorkerDifference = gitstate.CompareSourceSnapshots(admission, workerAfter)
		result.Source.WorkerRevision, snapshotErr = gitstate.PolicySourceRevision(workerAfter)
	}
	changedCapture, changedErr := n.ChangedCapture(ctx, gitConfig(n))
	if changedErr != nil {
		changedCapture = gitstate.Capture{Kind: gitstate.CaptureKindChanged, CaptureError: changedErr.Error()}
	}
	appendWorkerEvent(ctx, n, &result, ledger.EventChangedFilesCaptured, map[string]any{
		"pre_run_dirty_files": capturePaths(preRunDirty),
		"changed_files":       capturePaths(changedCapture),
		"capture_error":       changedCapture.CaptureError,
	})
	if snapshotErr != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_source_after", snapshotErr, receipt.VerdictSafetyLimit, "not_run", "")
	}
	result.Source.ChangedFiles = differencePaths(result.Source.WorkerDifference)
	if result.SafetyPolicy == nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "protected_paths", errors.New("validated safety policy is missing"), receipt.VerdictSafetyLimit, "not_run", "")
	}
	if err := result.SafetyPolicy.AuthorizeModelChanges(result.Source.ChangedFiles); err != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "protected_paths", err, receipt.VerdictSafetyLimit, "not_run", "")
	}

	if err := validateTaskAndStateUnchanged(n, task, stateBefore); err != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "immutable_task_state", err, receipt.VerdictSafetyLimit, "not_run", "")
	}
	if readOnlyAction(route.Action) && result.Source.WorkerDifference.Changed {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeReadOnlyMutation, "worker_mutation_authority", sourceChangedError("read-only worker changed repository source", result.Source.WorkerDifference), receipt.VerdictSafetyLimit, "not_run", "")
	}
	if !codexSucceeded(result.Worker.Codex) {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_codex", codexFailure(result.Worker.Codex), receipt.VerdictCodexFailed, "not_run", "")
	}
	if len(result.Worker.RawOutput) == 0 {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_output", errors.New("worker produced no exact final output"), receipt.VerdictBlocked, "not_run", "")
	}
	if changedCapture.CaptureError != "" {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeChangedCaptureFailed, "changed_files_capture", errors.New(changedCapture.CaptureError), receipt.VerdictSafetyLimit, "not_run", "")
	}
	if result.Source.WorkerDifference.HeadChanged {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_head_mutation", errors.New("worker changed HEAD or created a commit without harness authority"), receipt.VerdictSafetyLimit, "not_run", "")
	}
	if readOnlyAction(route.Action) {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeReadOnlyCompleted, "", nil, receipt.VerdictCompleted, "not_run", "")
	}
	if !result.Source.WorkerDifference.Changed {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerNoChanges, "", nil, receipt.VerdictNoChanges, "not_run", "")
	}
	if result.Source.WorkerRevision == result.Source.AdmissionRevision {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeWorkerFailed, "worker_administrative_mutation", errors.New("worker changed Git administrative state without changing source content"), receipt.VerdictSafetyLimit, "not_run", "")
	}
	if err := validateChangedCapture(changedCapture, result.Source.ChangedFiles); err != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeChangedCaptureFailed, "changed_files_capture", err, receipt.VerdictSafetyLimit, "not_run", "")
	}

	occurrenceID := strings.TrimSpace(n.IDGenerator())
	if occurrenceID == "" || strings.ContainsAny(occurrenceID, "\r\n") {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerificationFailed, "verification_identity", fmt.Errorf("generated malformed verification occurrence ID %q", occurrenceID), receipt.VerdictVerificationFailed, "failed", "")
	}
	var verificationResult verification.Result
	var verificationErr error
	if n.VerificationPlan == nil {
		verificationResult, verificationErr = n.VerificationRunner(ctx, verification.Config{
			WorkingDir:            n.executionRoot,
			Commands:              cloneVerificationCommands(n.VerificationCommands),
			MissingCommandsPolicy: n.MissingVerificationPolicy,
			Timeout:               n.VerificationTimeout,
			StdoutCap:             n.VerificationStdoutCap,
			StderrCap:             n.VerificationStderrCap,
			RunID:                 workerRunID,
			Ledger:                n.Ledger,
			CommandRunner:         verification.CommandRunner(n.CommandRunner),
		})
		result.Worker.Verification = buildVerificationEvidence(n.TaskID, workerRunID, occurrenceID, result.Source.WorkerRevision, verificationResult, verificationErr)
	} else {
		purpose := verificationPurpose(route.Action)
		tiered, tieredErr := n.TieredVerificationRunner(ctx, autonomousverification.Config{
			RepositoryRoot: n.executionRoot,
			ArtifactRoot:   n.root,
			TaskID:         n.TaskID, RunID: workerRunID, OccurrenceID: occurrenceID,
			SourceRevision: result.Source.WorkerRevision,
			Plan:           *n.VerificationPlan, Purpose: purpose,
			Timeout: n.VerificationTimeout, StdoutCap: n.VerificationStdoutCap, StderrCap: n.VerificationStderrCap,
			Clock: n.Clock, AttemptID: n.IDGenerator,
			CommandRunner: autonomousverification.CommandRunner(n.CommandRunner), Ledger: n.Ledger,
			ArtifactPath:   paths.verification,
			ArtifactWriter: n.VerificationArtifactWriter,
		})
		verificationResult, verificationErr = tiered.Aggregate, tieredErr
		result.Worker.Verification = buildTieredVerificationEvidence(n.TaskID, workerRunID, occurrenceID, result.Source.WorkerRevision, tiered, tieredErr)
		if tiered.Artifact != nil {
			artifact := Artifact{Path: tiered.Artifact.Path, SHA256: tiered.Artifact.SHA256, ByteSize: tiered.Artifact.ByteSize}
			result.Worker.Artifacts.Verification = &artifact
		}
	}
	if verificationResult.LedgerError != nil {
		setWorkerLedgerError(&result, verificationResult.LedgerError)
	}
	verificationAfter, afterVerificationErr := captureSource(ctx, n)
	if afterVerificationErr == nil {
		result.Source.VerificationAfter = &verificationAfter
		result.Source.VerificationDifference = gitstate.CompareSourceSnapshots(workerAfter, verificationAfter)
	}
	if verificationErr != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerificationFailed, "verification", verificationErr, receipt.VerdictVerificationFailed, "failed", "")
	}
	if afterVerificationErr != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerificationFailed, "verification_source_after", afterVerificationErr, receipt.VerdictVerificationFailed, "failed", "")
	}
	if result.Source.VerificationDifference.Changed {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerificationFailed, "verification_source_mutation", sourceChangedError("verification changed source after testing", result.Source.VerificationDifference), receipt.VerdictVerificationFailed, "failed", "")
	}
	if verificationResult.MissingCommands || !verificationResult.Passed || verificationResult.Status != verification.StatusPassed {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerificationFailed, "verification", verificationFailure(verificationResult), receipt.VerdictVerificationFailed, verificationStatus(verificationResult), "")
	}

	workerChanges := captureForWorkerChanges(changedCapture, result.Source.ChangedFiles)
	commitResult, commitErr := n.CommitRunner(ctx, commit.Config{
		WorkingDir:               n.executionRoot,
		RunID:                    workerRunID,
		TaskID:                   n.TaskID,
		TaskSummary:              task.Title,
		CodexResult:              &result.Worker.Codex,
		VerificationResult:       &verificationResult,
		PreRunDirty:              &preRunDirty,
		PostRunChanged:           &workerChanges,
		AllowPreExistingDirty:    n.AllowPreExistingDirty,
		AllowMissingVerification: false,
		GitExecutable:            n.GitExecutable,
		Timeout:                  n.CommitTimeout,
		StdoutCap:                n.CommitStdoutCap,
		StderrCap:                n.CommitStderrCap,
		Ledger:                   n.Ledger,
		CommitRecorder:           n.Ledger,
		CommandRunner:            commit.CommandRunner(n.CommandRunner),
	})
	result.Worker.Commit = commitResult
	if commitResult.LedgerError != nil {
		setWorkerLedgerError(&result, commitResult.LedgerError)
	}
	finalSnapshot, finalErr := captureSource(ctx, n)
	if finalErr == nil {
		result.Source.Final = &finalSnapshot
		result.Source.FinalRevision, finalErr = gitstate.PolicySourceRevision(finalSnapshot)
	}
	if commitErr != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeCommitFailed, "commit", commitErr, receipt.VerdictBlocked, "passed", "")
	}
	if finalErr != nil {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeCommitFailed, "final_source", finalErr, receipt.VerdictBlocked, "passed", commitResult.CommitSHA)
	}
	if commitResult.Status != commit.StatusCommitted || strings.TrimSpace(commitResult.CommitSHA) == "" {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeCommitFailed, "commit", commitFailure(commitResult), receipt.VerdictBlocked, "passed", "")
	}
	if result.Source.FinalRevision != result.Source.WorkerRevision {
		return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeCommitFailed, "commit_freshness", fmt.Errorf("final source revision %q differs from verified revision %q", result.Source.FinalRevision, result.Source.WorkerRevision), receipt.VerdictBlocked, "passed", commitResult.CommitSHA)
	}
	return finishWorker(ctx, n, task, route, preRunDirty, &result, OutcomeVerifiedChangesCommitted, "", nil, receipt.VerdictCompleted, "passed", commitResult.CommitSHA)
}

func finishWorker(ctx context.Context, n normalizedConfig, task taskfile.Task, route autonomouspolicy.Route, preRunDirty gitstate.Capture, result *Result, outcome Outcome, stage string, cause error, verdict receipt.Verdict, verificationStatus string, commitSHA string) (Result, error) {
	if result.Source.Final == nil {
		if result.Source.VerificationAfter != nil {
			final := *result.Source.VerificationAfter
			result.Source.Final = &final
			result.Source.FinalRevision, _ = gitstate.PolicySourceRevision(final)
		} else if result.Source.WorkerAfter != nil {
			final := *result.Source.WorkerAfter
			result.Source.Final = &final
			result.Source.FinalRevision = result.Source.WorkerRevision
		}
	}
	completedAt := n.Clock().UTC()
	finalizeWorkerReceipt(ctx, n, task, route, result, verdict, verificationStatus, commitSHA, completedAt, cause)
	if result.Source.WorkerAfter != nil {
		if artifact, err := writeWorkerSourceEvidence(n.root, result.Worker.Artifacts.SourceEvidence.Path, result.Source); err == nil {
			result.Worker.Artifacts.SourceEvidence = artifact
		} else if cause == nil {
			cause = err
			stage = "worker_source_artifact"
			outcome = OutcomeWorkerFailed
		}
	}
	status := ledger.StatusFailed
	eventType := ledger.EventRunFailed
	if cause == nil {
		status = ledger.StatusCompleted
		eventType = ledger.EventRunCompleted
	}
	exitCode := result.Worker.Codex.ExitCode
	updated, ok, err := n.Ledger.CompleteRun(ctx, result.Worker.RunID, ledger.RunCompletion{
		Status:             status,
		Summary:            workerSummary(outcome, cause),
		CompletedAt:        completedAt,
		CodexExitCode:      &exitCode,
		VerificationStatus: verificationStatus,
		CommitSHA:          commitSHA,
	})
	if err != nil {
		setWorkerLedgerError(result, err)
	} else if !ok {
		setWorkerLedgerError(result, fmt.Errorf("complete worker run %q: not found", result.Worker.RunID))
	} else {
		result.Worker.Run = updated
	}
	appendWorkerEvent(ctx, n, result, eventType, map[string]any{
		"run_id":                     result.Worker.RunID,
		"task_id":                    n.TaskID,
		"pass":                       "worker",
		"outcome":                    outcome,
		"action":                     route.Action,
		"worker_profile":             route.WorkerProfile,
		"decision_id":                route.DecisionID,
		"source_revision_before":     result.Source.AdmissionRevision,
		"source_revision_after":      result.Source.WorkerRevision,
		"source_revision_final":      result.Source.FinalRevision,
		"changed_files":              append([]string(nil), result.Source.ChangedFiles...),
		"verification_occurrence_id": result.Worker.Verification.OccurrenceID,
		"verification_status":        verificationStatus,
		"commit_status":              result.Worker.Commit.Status,
		"commit_sha":                 commitSHA,
		"receipt_path":               result.Worker.Receipt.Path,
		"receipt_synthesized":        result.Worker.Receipt.Synthesized,
		"reason":                     errorText(cause),
	})
	result.Outcome = outcome
	if result.Worker.LedgerError != nil {
		cause = errors.Join(cause, result.Worker.LedgerError)
		if stage == "" {
			stage = "worker_ledger"
		}
	}
	if cause != nil {
		return failed(*result, outcome, stage, cause)
	}
	return *result, nil
}

func buildVerificationEvidence(taskID, runID, occurrenceID, sourceRevision string, result verification.Result, runErr error) VerificationEvidence {
	status := autonomous.VerificationStatusFailed
	if runErr == nil && !result.MissingCommands && result.Passed && result.Status == verification.StatusPassed {
		status = autonomous.VerificationStatusPassed
	}
	summary := verificationSummary(result, runErr)
	policy := &autonomouspolicy.VerificationEvidence{
		Summary: autonomous.VerificationSummary{
			TaskID:       taskID,
			Status:       status,
			Command:      verificationCommandSummary(result),
			Summary:      summary,
			RunID:        runID,
			OccurrenceID: occurrenceID,
			Evidence: []autonomous.EvidenceReference{{
				Kind:      autonomous.EvidenceKindVerification,
				Reference: "ledger:" + runID + ":verification:" + occurrenceID,
				Detail:    "Harness-observed verification result for exact source revision " + sourceRevision + ".",
			}},
		},
		SourceRevision: sourceRevision,
	}
	return VerificationEvidence{OccurrenceID: occurrenceID, SourceRevision: sourceRevision, Result: result, Policy: policy}
}

func buildTieredVerificationEvidence(taskID, runID, occurrenceID, sourceRevision string, result autonomousverification.Result, runErr error) VerificationEvidence {
	evidence := buildVerificationEvidence(taskID, runID, occurrenceID, sourceRevision, result.Aggregate, runErr)
	tiered := result
	evidence.Tiered = &tiered
	if evidence.Policy != nil {
		gate := result.Gate
		evidence.Policy.Tiered = &gate
		if result.Outcome != autonomousverification.OutcomePassed || (result.Purpose == autonomousverification.PurposeFinal && !result.Gate.FinalSatisfied) {
			evidence.Policy.Summary.Status = autonomous.VerificationStatusFailed
		}
		evidence.Policy.Summary.Command = tieredCommandSummary(result)
		evidence.Policy.Summary.Summary = fmt.Sprintf("Tiered %s verification classified %s; final gate satisfied=%t.", result.Purpose, result.Outcome, result.Gate.FinalSatisfied)
		evidence.Policy.Summary.Tiered = &tiered
		if result.Artifact != nil {
			evidence.Policy.Summary.Evidence = append(evidence.Policy.Summary.Evidence, autonomous.EvidenceReference{Kind: autonomous.EvidenceKindVerification, Reference: result.Artifact.Path, Detail: fmt.Sprintf("Tiered verification artifact SHA-256 %s (%d bytes).", result.Artifact.SHA256, result.Artifact.ByteSize)})
		}
	}
	return evidence
}

func verificationPurpose(action autonomous.Action) autonomousverification.Purpose {
	if action == autonomous.ActionCorrect {
		return autonomousverification.PurposeFast
	}
	return autonomousverification.PurposeFinal
}

func tieredCommandSummary(result autonomousverification.Result) string {
	commands := make([]string, 0)
	for _, tier := range result.Tiers {
		for _, command := range tier.Commands {
			commands = append(commands, command.Identity.Name+" "+strings.Join(command.Identity.Args, " "))
		}
	}
	return strings.TrimSpace(strings.Join(commands, "; "))
}

func validateTaskAndStateUnchanged(n normalizedConfig, task taskfile.Task, before optionalFile) error {
	current, found, err := n.TaskLoader(n.root, n.TaskID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("canonical task %q disappeared", n.TaskID)
	}
	if filepath.Clean(current.SourcePath) != filepath.Clean(task.SourcePath) || !equalBytes(current.SourceBytes, task.SourceBytes) {
		return fmt.Errorf("canonical task %q changed during worker execution", n.TaskID)
	}
	after, err := captureOptionalFile(n.root, task.AutonomousStatePath)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("autonomous execution state path %q changed during worker execution", task.AutonomousStatePath)
	}
	return nil
}

func validateChangedCapture(capture gitstate.Capture, observed []string) error {
	captured := capturePaths(capture)
	want := append([]string(nil), observed...)
	sort.Strings(captured)
	sort.Strings(want)
	for _, path := range want {
		if !containsString(captured, path) {
			return fmt.Errorf("changed-files capture omitted observed worker path %q (captured %q)", path, captured)
		}
	}
	return nil
}

func captureForWorkerChanges(capture gitstate.Capture, paths []string) gitstate.Capture {
	result := capture
	result.Paths = append([]string(nil), paths...)
	result.ChangedFiles = append([]string(nil), paths...)
	result.DirtyFiles = nil
	return result
}

func differencePaths(difference gitstate.SourceDifference) []string {
	paths := make([]string, 0, len(difference.PathChanges))
	seen := make(map[string]struct{}, len(difference.PathChanges))
	for _, change := range difference.PathChanges {
		if _, ok := seen[change.Path]; ok {
			continue
		}
		seen[change.Path] = struct{}{}
		paths = append(paths, change.Path)
	}
	sort.Strings(paths)
	return paths
}

func readOnlyAction(action autonomous.Action) bool {
	return action == autonomous.ActionPlan || action == autonomous.ActionAudit
}

func sourceChangingAction(action autonomous.Action) bool {
	switch action {
	case autonomous.ActionImplement, autonomous.ActionCorrect, autonomous.ActionDocument, autonomous.ActionSimplify:
		return true
	default:
		return false
	}
}

func codexSucceeded(result codexexec.Result) bool {
	return result.Err == nil && !result.TimedOut && result.ExitCode == 0 && result.ArtifactError == nil
}

func codexFailure(result codexexec.Result) error {
	switch {
	case result.TimedOut:
		return errors.New("worker Codex timed out")
	case result.Err != nil:
		return fmt.Errorf("worker Codex failed: %w", result.Err)
	case result.ExitCode != 0:
		return fmt.Errorf("worker Codex exited with code %d", result.ExitCode)
	case result.ArtifactError != nil:
		return fmt.Errorf("worker Codex artifact failed: %w", result.ArtifactError)
	default:
		return errors.New("worker Codex did not complete successfully")
	}
}

func verificationFailure(result verification.Result) error {
	if result.MissingCommands {
		return errors.New("verification commands are missing")
	}
	if strings.TrimSpace(result.Message) != "" {
		return errors.New(result.Message)
	}
	return errors.New("verification failed")
}

func verificationStatus(result verification.Result) string {
	if result.MissingCommands {
		return "missing"
	}
	if result.Passed && result.Status == verification.StatusPassed {
		return "passed"
	}
	return "failed"
}

func commitFailure(result commit.Result) error {
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "commit did not complete"
	}
	return fmt.Errorf("%s: status=%q refusal=%q pre_head=%q post_head=%q", message, result.Status, result.RefusalReason, result.PreCommitSHA, result.PostCommitSHA)
}

func workerSummary(outcome Outcome, cause error) string {
	if cause == nil {
		return string(outcome)
	}
	return string(outcome) + ": " + cause.Error()
}

func verificationSummary(result verification.Result, runErr error) string {
	if runErr != nil {
		return "verification runner failed: " + runErr.Error()
	}
	if result.MissingCommands {
		return "verification commands are missing"
	}
	if strings.TrimSpace(result.Message) != "" {
		return result.Message
	}
	if result.Passed && result.Status == verification.StatusPassed {
		return "configured verification passed"
	}
	return "configured verification failed"
}

func verificationCommandSummary(result verification.Result) string {
	commands := make([]string, 0, len(result.Commands))
	for _, command := range result.Commands {
		if strings.TrimSpace(command.Command) != "" {
			commands = append(commands, command.Command)
		}
	}
	return strings.Join(commands, "; ")
}

func workerEventIdentity(result Result, route autonomouspolicy.Route) map[string]any {
	return map[string]any{
		"run_id":       result.Worker.RunID,
		"task_id":      result.TaskID,
		"pass":         "worker",
		"action":       route.Action,
		"profile_name": route.WorkerProfile,
		"decision_id":  route.DecisionID,
	}
}

func workerTaskSelectedEvent(task taskfile.Task, route autonomouspolicy.Route, decisionID string) map[string]any {
	return map[string]any{
		"task_id":         task.ID,
		"task":            task.ContextBody,
		"summary":         task.Title,
		"workflow":        task.Workflow,
		"action":          route.Action,
		"profile_name":    route.WorkerProfile,
		"decision_id":     decisionID,
		"source_revision": route.SourceRevision,
	}
}

func appendWorkerEvent(ctx context.Context, n normalizedConfig, result *Result, eventType ledger.EventType, payload any) {
	if result.Worker.RunID == "" {
		return
	}
	_, err := n.Ledger.AppendEvent(ctx, result.Worker.RunID, eventType, payload)
	setWorkerLedgerError(result, err)
}

func setWorkerLedgerError(result *Result, err error) {
	if err != nil && result.Worker.LedgerError == nil {
		result.Worker.LedgerError = err
	}
}

func gitConfig(n normalizedConfig) gitstate.Config {
	return gitstate.Config{
		WorkingDir:    n.executionRoot,
		GitExecutable: n.GitExecutable,
		Timeout:       n.GitTimeout,
		StdoutCap:     n.GitStdoutCap,
		StderrCap:     n.GitStderrCap,
		CommandRunner: gitstate.CommandRunner(n.CommandRunner),
	}
}

func capturePaths(capture gitstate.Capture) []string {
	values := capture.Paths
	if len(capture.ChangedFiles) > 0 {
		values = capture.ChangedFiles
	} else if len(capture.DirtyFiles) > 0 {
		values = capture.DirtyFiles
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func captureHasPaths(capture gitstate.Capture) bool { return len(capturePaths(capture)) > 0 }
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func boolPointer(value bool) *bool { return &value }
