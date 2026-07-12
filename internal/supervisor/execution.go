package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/codexexec"
	"revolvr/internal/gitstate"
	"revolvr/internal/id"
	"revolvr/internal/ledger"
	"revolvr/internal/lock"
	"revolvr/internal/pathguard"
	"revolvr/internal/prompt"
)

const supervisorProvenanceSchemaVersion = "revolvr-supervisor-provenance-v1"

type Ledger interface {
	CreateRun(context.Context, ledger.RunSpec) (ledger.Run, error)
	AppendEvent(context.Context, string, ledger.EventType, any) (ledger.Event, error)
	CompleteRun(context.Context, string, ledger.RunCompletion) (ledger.Run, bool, error)
}

type SourceSnapshotter func(context.Context, gitstate.SourceSnapshotConfig) (gitstate.SourceSnapshot, error)

type Config struct {
	RepositoryRoot      string
	TaskID              string
	Dossier             autonomous.TaskDossier
	Audit               *autonomous.AuditReport
	VerificationFailure *autonomous.VerificationFailureTarget
	Ledger              Ledger

	RunID       string
	DecisionID  string
	IDGenerator func() string
	Clock       func() time.Time

	CodexExecutable                string
	CodexModel                     string
	CodexReasoningEffort           string
	CodexEphemeral                 bool
	CodexSandbox                   string
	CodexApprovalPolicy            string
	CodexBypassApprovalsAndSandbox bool
	CodexVersion                   string
	EffectiveConfigSchema          string
	EffectiveConfigSHA256          string
	CodexTimeout                   time.Duration
	CodexStdoutCap                 int
	CodexStderrCap                 int
	CodexCommandRunner             codexexec.CommandRunner

	GitExecutable     string
	GitTimeout        time.Duration
	GitStdoutCap      int
	GitStderrCap      int
	GitCommandRunner  gitstate.CommandRunner
	SourceSnapshotter SourceSnapshotter

	SourceLockTimeout time.Duration
	SourceLockPID     int
}

type Artifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256,omitempty"`
	ByteSize int    `json:"byte_size,omitempty"`
}

type Artifacts struct {
	Prompt         Artifact `json:"prompt"`
	Schema         Artifact `json:"schema"`
	RawOutput      Artifact `json:"raw_output"`
	Decision       Artifact `json:"decision"`
	Provenance     Artifact `json:"provenance"`
	SourceEvidence Artifact `json:"source_evidence"`
	Diagnostics    Artifact `json:"diagnostics"`
	CodexStdout    Artifact `json:"codex_stdout"`
	CodexStderr    Artifact `json:"codex_stderr"`
}

type DossierProvenance struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	SHA256        string `json:"sha256"`
	ByteSize      int    `json:"byte_size"`
}

type ProfileProvenance struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type Result struct {
	RunID             string
	LedgerRun         ledger.Run
	Decision          *autonomous.SupervisorDecision
	DecisionReference *autonomous.DecisionReference
	Artifacts         Artifacts
	Dossier           DossierProvenance
	Profile           ProfileProvenance
	Invocation        codexexec.InvocationProvenance
	Codex             codexexec.Result
	SourceBefore      *gitstate.SourceSnapshot
	SourceAfter       *gitstate.SourceSnapshot
	SourceDifference  gitstate.SourceDifference
}

type normalizedConfig struct {
	Config
	root       string
	runID      string
	decisionID string
}

type artifactPaths struct {
	prompt         string
	schema         string
	rawOutput      string
	decision       string
	provenance     string
	sourceEvidence string
	diagnostics    string
	codexStdout    string
	codexStderr    string
}

type supervisorProvenance struct {
	SchemaVersion string                         `json:"schema_version"`
	RunID         string                         `json:"run_id"`
	TaskID        string                         `json:"task_id"`
	Dossier       autonomous.TaskDossierManifest `json:"dossier_manifest"`
	Profile       ProfileProvenance              `json:"profile"`
	Invocation    codexexec.InvocationProvenance `json:"invocation"`
	Artifacts     Artifacts                      `json:"artifacts"`
}

type sourceEvidence struct {
	Before        *gitstate.SourceSnapshot  `json:"before,omitempty"`
	After         *gitstate.SourceSnapshot  `json:"after,omitempty"`
	Difference    gitstate.SourceDifference `json:"difference"`
	BaselineError string                    `json:"baseline_error,omitempty"`
	AfterError    string                    `json:"after_error,omitempty"`
	ReleaseError  string                    `json:"source_lock_release_error,omitempty"`
}

type rejectionDiagnostics struct {
	Category  string           `json:"category"`
	Reason    string           `json:"reason"`
	Codex     codexDiagnostics `json:"codex"`
	Source    sourceEvidence   `json:"source"`
	Artifacts Artifacts        `json:"artifacts"`
}

type codexDiagnostics struct {
	ExitCode        int      `json:"exit_code"`
	TimedOut        bool     `json:"timed_out"`
	Error           string   `json:"error,omitempty"`
	ParseError      string   `json:"parse_error,omitempty"`
	ArtifactError   string   `json:"artifact_error,omitempty"`
	LedgerError     string   `json:"ledger_error,omitempty"`
	JSONParseErrors []string `json:"json_parse_errors,omitempty"`
	StdoutTruncated int64    `json:"stdout_truncated_bytes"`
	StderrTruncated int64    `json:"stderr_truncated_bytes"`
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	normalized, err := normalize(cfg)
	if err != nil {
		return Result{}, err
	}
	paths, err := prepareArtifactPaths(normalized.root, normalized.runID)
	if err != nil {
		return Result{RunID: normalized.runID}, err
	}
	result := Result{
		RunID:     normalized.runID,
		Artifacts: artifactsWithPaths(paths),
	}

	run, err := normalized.Ledger.CreateRun(ctx, ledger.RunSpec{
		ID:        normalized.runID,
		TaskID:    normalized.TaskID,
		Task:      "supervisor decision for task " + normalized.TaskID,
		StartedAt: normalized.Clock().UTC(),
	})
	if err != nil {
		return result, fmt.Errorf("create supervisor ledger run: %w", err)
	}
	result.LedgerRun = run
	if _, err := normalized.Ledger.AppendEvent(ctx, run.ID, ledger.EventRunStarted, map[string]any{
		"run_id": run.ID, "task_id": normalized.TaskID, "pass": "supervisor",
	}); err != nil {
		return reject(ctx, normalized, paths, result, "ledger", fmt.Errorf("record supervisor run start: %w", err), sourceEvidence{}, false)
	}

	if err := ValidateDossier(normalized.TaskID, normalized.Dossier); err != nil {
		return reject(ctx, normalized, paths, result, "dossier", err, sourceEvidence{}, false)
	}
	if err := validateAudit(normalized.TaskID, normalized.Audit); err != nil {
		return reject(ctx, normalized, paths, result, "audit_context", err, sourceEvidence{}, false)
	}
	if normalized.VerificationFailure != nil {
		if normalized.Audit != nil {
			return reject(ctx, normalized, paths, result, "correction_context", errors.New("audit and verification-failure correction contexts are mutually exclusive"), sourceEvidence{}, false)
		}
		if err := normalized.VerificationFailure.Validate(); err != nil || normalized.VerificationFailure.TaskID != normalized.TaskID {
			return reject(ctx, normalized, paths, result, "correction_context", errors.New("verification-failure correction context is invalid or belongs to another task"), sourceEvidence{}, false)
		}
	}

	runProfile, err := prompt.LoadRunProfile(normalized.root, SupervisorProfileName)
	if err != nil {
		return reject(ctx, normalized, paths, result, "profile", err, sourceEvidence{}, false)
	}
	profileBytes := []byte(runProfile.Description)
	result.Profile = ProfileProvenance{
		Name: runProfile.Name, Path: runProfile.SourcePath,
		SHA256: sha256Hex(profileBytes), ByteSize: len(profileBytes),
	}
	result.Dossier = DossierProvenance{
		SchemaVersion: normalized.Dossier.Manifest.SchemaVersion,
		TaskID:        normalized.Dossier.Manifest.TaskID,
		SHA256:        normalized.Dossier.Manifest.DossierSHA256,
		ByteSize:      normalized.Dossier.Manifest.DossierByteSize,
	}

	schema, err := DecisionOutputSchema()
	if err != nil {
		return reject(ctx, normalized, paths, result, "schema", err, sourceEvidence{}, false)
	}
	promptBytes, err := BuildPrompt(PromptInput{TaskID: normalized.TaskID, Dossier: normalized.Dossier, Profile: runProfile})
	if err != nil {
		return reject(ctx, normalized, paths, result, "prompt", err, sourceEvidence{}, false)
	}
	result.Artifacts.Prompt, err = writeArtifact(normalized.root, paths.prompt, promptBytes)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, sourceEvidence{}, false)
	}
	result.Artifacts.Schema, err = writeArtifact(normalized.root, paths.schema, schema)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, sourceEvidence{}, false)
	}

	invocation, _, err := codexexec.PrepareInvocation(codexexec.InvocationConfig{
		Executable:             normalized.CodexExecutable,
		WorkingDir:             normalized.root,
		Model:                  normalized.CodexModel,
		ReasoningEffort:        normalized.CodexReasoningEffort,
		Ephemeral:              normalized.CodexEphemeral,
		Sandbox:                normalized.CodexSandbox,
		ApprovalPolicy:         normalized.CodexApprovalPolicy,
		BypassApprovalsSandbox: normalized.CodexBypassApprovalsAndSandbox,
		Artifacts:              codexexec.ArtifactPaths{StdoutJSONL: paths.codexStdout, Stderr: paths.codexStderr, LastMessage: paths.rawOutput},
		OutputSchema:           paths.schema,
		CodexVersion:           normalized.CodexVersion,
		EffectiveConfigSchema:  normalized.EffectiveConfigSchema,
		EffectiveConfigSHA256:  normalized.EffectiveConfigSHA256,
	})
	if err != nil {
		return reject(ctx, normalized, paths, result, "invocation", err, sourceEvidence{}, false)
	}
	if err := invocation.Validate(); err != nil {
		return reject(ctx, normalized, paths, result, "invocation", err, sourceEvidence{}, false)
	}
	result.Invocation = invocation
	provenanceBytes, err := marshalIndented(supervisorProvenance{
		SchemaVersion: supervisorProvenanceSchemaVersion,
		RunID:         normalized.runID,
		TaskID:        normalized.TaskID,
		Dossier:       normalized.Dossier.Manifest,
		Profile:       result.Profile,
		Invocation:    invocation,
		Artifacts:     result.Artifacts,
	})
	if err != nil {
		return reject(ctx, normalized, paths, result, "provenance", err, sourceEvidence{}, false)
	}
	result.Artifacts.Provenance, err = writeArtifact(normalized.root, paths.provenance, provenanceBytes)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, sourceEvidence{}, false)
	}
	if _, err := normalized.Ledger.AppendEvent(ctx, run.ID, ledger.EventSupervisorPrepared, map[string]any{
		"run_id":     run.ID,
		"task_id":    normalized.TaskID,
		"dossier":    result.Dossier,
		"profile":    result.Profile,
		"invocation": result.Invocation,
		"artifacts":  result.Artifacts,
	}); err != nil {
		return reject(ctx, normalized, paths, result, "ledger", fmt.Errorf("record prepared supervisor pass: %w", err), sourceEvidence{}, false)
	}

	sourceLock, err := lock.AcquireSourceWriter(ctx, lock.Config{
		WorkingDir: normalized.root,
		RunID:      normalized.runID,
		PID:        normalized.SourceLockPID,
		Timeout:    normalized.SourceLockTimeout,
		Clock:      normalized.Clock,
	})
	if err != nil {
		return reject(ctx, normalized, paths, result, "source_lock", err, sourceEvidence{}, false)
	}

	snapshotCfg := gitstate.SourceSnapshotConfig{
		WorkingDir:    normalized.root,
		GitExecutable: normalized.GitExecutable,
		Timeout:       normalized.GitTimeout,
		StdoutCap:     normalized.GitStdoutCap,
		StderrCap:     normalized.GitStderrCap,
		CommandRunner: normalized.GitCommandRunner,
	}
	baseline, baselineErr := normalized.SourceSnapshotter(ctx, snapshotCfg)
	if baselineErr == nil {
		baselineErr = baseline.Validate()
	}
	if baselineErr != nil {
		releaseErr := releaseSourceLock(sourceLock)
		evidence := sourceEvidence{BaselineError: baselineErr.Error(), ReleaseError: errorText(releaseErr)}
		var evidenceErr error
		result.Artifacts.SourceEvidence, evidenceErr = writeSourceEvidence(normalized.root, paths.sourceEvidence, evidence)
		return reject(ctx, normalized, paths, result, "source_capture", errors.Join(baselineErr, releaseErr, evidenceErr), evidence, false)
	}
	result.SourceBefore = &baseline

	codexResult, codexRunErr := codexexec.Run(ctx, codexexec.Config{
		Executable:                normalized.CodexExecutable,
		WorkingDir:                normalized.root,
		Prompt:                    string(promptBytes),
		Model:                     normalized.CodexModel,
		ReasoningEffort:           normalized.CodexReasoningEffort,
		Ephemeral:                 boolPointer(normalized.CodexEphemeral),
		Timeout:                   normalized.CodexTimeout,
		StdoutCap:                 normalized.CodexStdoutCap,
		StderrCap:                 normalized.CodexStderrCap,
		Sandbox:                   normalized.CodexSandbox,
		ApprovalPolicy:            normalized.CodexApprovalPolicy,
		BypassApprovalsAndSandbox: normalized.CodexBypassApprovalsAndSandbox,
		Artifacts:                 codexexec.ArtifactPaths{StdoutJSONL: paths.codexStdout, Stderr: paths.codexStderr, LastMessage: paths.rawOutput},
		OutputSchema:              paths.schema,
		RunID:                     normalized.runID,
		Ledger:                    normalized.Ledger,
		CommandRunner:             normalized.CodexCommandRunner,
		Provenance:                invocation,
	})
	result.Codex = codexResult

	postCtx, cancel := context.WithTimeout(context.Background(), normalized.GitTimeout)
	after, afterErr := normalized.SourceSnapshotter(postCtx, snapshotCfg)
	cancel()
	if afterErr == nil {
		afterErr = after.Validate()
	}
	releaseErr := releaseSourceLock(sourceLock)
	evidence := sourceEvidence{Before: &baseline, AfterError: errorText(afterErr), ReleaseError: errorText(releaseErr)}
	if afterErr == nil {
		result.SourceAfter = &after
		result.SourceDifference = gitstate.CompareSourceSnapshots(baseline, after)
		evidence.After = &after
		evidence.Difference = result.SourceDifference
	}
	result.Artifacts.SourceEvidence, err = writeSourceEvidence(normalized.root, paths.sourceEvidence, evidence)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, evidence, true)
	}

	result.Artifacts.CodexStdout, err = referenceArtifact(normalized.root, paths.codexStdout)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, evidence, true)
	}
	result.Artifacts.CodexStderr, err = referenceArtifact(normalized.root, paths.codexStderr)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, evidence, true)
	}
	if rawRef, rawErr := referenceArtifact(normalized.root, paths.rawOutput); rawErr == nil {
		result.Artifacts.RawOutput = rawRef
	}

	if afterErr != nil {
		return reject(ctx, normalized, paths, result, "source_capture", fmt.Errorf("capture post-Codex source snapshot: %w", afterErr), evidence, true)
	}
	if releaseErr != nil {
		return reject(ctx, normalized, paths, result, "source_lock", fmt.Errorf("release source-writer lock: %w", releaseErr), evidence, true)
	}
	if result.SourceDifference.Changed {
		mutationErr := errors.New("supervisor decision pass changed repository source")
		_, ledgerErr := normalized.Ledger.AppendEvent(ctx, run.ID, ledger.EventSupervisorMutation, map[string]any{
			"run_id":     run.ID,
			"task_id":    normalized.TaskID,
			"difference": result.SourceDifference,
			"artifacts":  result.Artifacts,
		})
		return reject(ctx, normalized, paths, result, "source_mutation", errors.Join(mutationErr, ledgerErr), evidence, true)
	}
	if codexErr := codexFailure(codexResult, codexRunErr); codexErr != nil {
		return reject(ctx, normalized, paths, result, "codex", codexErr, evidence, true)
	}

	raw, err := os.ReadFile(filepath.Join(normalized.root, filepath.FromSlash(paths.rawOutput)))
	if err != nil {
		return reject(ctx, normalized, paths, result, "output_artifact", fmt.Errorf("read exact supervisor output: %w", err), evidence, true)
	}
	if len(raw) == 0 {
		return reject(ctx, normalized, paths, result, "output_artifact", errors.New("exact supervisor output is empty"), evidence, true)
	}
	result.Artifacts.RawOutput = artifactForBytes(paths.rawOutput, raw)
	decision, err := ParseDecision(raw, normalized.TaskID, normalized.Audit, normalized.VerificationFailure)
	if err != nil {
		return reject(ctx, normalized, paths, result, "decision", err, evidence, true)
	}
	canonical, err := marshalIndented(decision)
	if err != nil {
		return reject(ctx, normalized, paths, result, "decision", err, evidence, true)
	}
	result.Artifacts.Decision, err = writeArtifact(normalized.root, paths.decision, canonical)
	if err != nil {
		return reject(ctx, normalized, paths, result, "artifact", err, evidence, true)
	}
	decisionReference := autonomous.DecisionReference{
		DecisionID:    normalized.decisionID,
		RunID:         normalized.runID,
		TaskID:        normalized.TaskID,
		Action:        decision.Action,
		WorkerProfile: decision.WorkerProfile,
		Artifact: autonomous.EvidenceReference{
			Kind:      autonomous.EvidenceKindFile,
			Reference: paths.decision,
			Detail:    "Canonical validated SupervisorDecision with SHA-256 " + result.Artifacts.Decision.SHA256 + ".",
		},
		CreatedAt: normalized.Clock().UTC(),
	}
	if err := decisionReference.Validate(); err != nil {
		return reject(ctx, normalized, paths, result, "decision_reference", err, evidence, true)
	}
	if _, err := normalized.Ledger.AppendEvent(ctx, run.ID, ledger.EventSupervisorValidated, map[string]any{
		"run_id":             run.ID,
		"task_id":            normalized.TaskID,
		"action":             decision.Action,
		"worker_profile":     decision.WorkerProfile,
		"decision_reference": decisionReference,
		"dossier":            result.Dossier,
		"profile":            result.Profile,
		"invocation":         result.Invocation,
		"artifacts":          result.Artifacts,
	}); err != nil {
		return reject(ctx, normalized, paths, result, "ledger", fmt.Errorf("record validated supervisor decision: %w", err), evidence, true)
	}

	completedAt := normalized.Clock().UTC()
	exitCode := codexResult.ExitCode
	completed, ok, err := normalized.Ledger.CompleteRun(ctx, run.ID, ledger.RunCompletion{
		Status:             ledger.StatusCompleted,
		Summary:            "supervisor decision accepted: " + string(decision.Action),
		CompletedAt:        completedAt,
		CodexExitCode:      &exitCode,
		VerificationStatus: "not_run",
	})
	if err != nil || !ok {
		if err == nil {
			err = fmt.Errorf("complete supervisor run: run %q not found", run.ID)
		}
		return reject(ctx, normalized, paths, result, "ledger", err, evidence, true)
	}
	result.LedgerRun = completed
	if _, err := normalized.Ledger.AppendEvent(ctx, run.ID, ledger.EventRunCompleted, map[string]any{
		"run_id":              run.ID,
		"task_id":             normalized.TaskID,
		"pass":                "supervisor",
		"outcome":             "decision_accepted",
		"action":              decision.Action,
		"worker_profile":      decision.WorkerProfile,
		"verification_status": "not_run",
		"commit_sha":          "",
		"artifacts":           result.Artifacts,
	}); err != nil {
		return reject(ctx, normalized, paths, result, "ledger", fmt.Errorf("record completed supervisor run: %w", err), evidence, true)
	}
	result.Decision = &decision
	result.DecisionReference = &decisionReference
	return result, nil
}

func normalize(cfg Config) (normalizedConfig, error) {
	root := strings.TrimSpace(cfg.RepositoryRoot)
	if root == "" {
		return normalizedConfig{}, errors.New("run supervisor: repository root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return normalizedConfig{}, fmt.Errorf("run supervisor: resolve repository root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return normalizedConfig{}, fmt.Errorf("run supervisor: repository root is not a readable directory: %w", err)
	}
	if err := validateTaskID(cfg.TaskID); err != nil {
		return normalizedConfig{}, err
	}
	if cfg.Ledger == nil {
		return normalizedConfig{}, errors.New("run supervisor: writable ledger is required")
	}
	if cfg.IDGenerator == nil {
		cfg.IDGenerator = id.New
	}
	runID := strings.TrimSpace(cfg.RunID)
	if runID == "" {
		runID = cfg.IDGenerator()
	}
	if !safeRunID(runID) {
		return normalizedConfig{}, fmt.Errorf("run supervisor: unsafe run id %q", runID)
	}
	decisionID := strings.TrimSpace(cfg.DecisionID)
	if decisionID == "" {
		decisionID = "decision-" + strings.ToLower(runID)
	}
	if !validDecisionID(decisionID) {
		return normalizedConfig{}, fmt.Errorf("run supervisor: invalid decision id %q", decisionID)
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	if !cfg.CodexEphemeral {
		return normalizedConfig{}, errors.New("run supervisor: only fresh ephemeral Codex sessions are supported")
	}
	if cfg.CodexTimeout <= 0 {
		cfg.CodexTimeout = 30 * time.Minute
	}
	if cfg.CodexStdoutCap <= 0 {
		cfg.CodexStdoutCap = 8 * 1024 * 1024
	}
	if cfg.CodexStderrCap <= 0 {
		cfg.CodexStderrCap = 8 * 1024 * 1024
	}
	if cfg.GitTimeout <= 0 {
		cfg.GitTimeout = 30 * time.Second
	}
	if cfg.SourceSnapshotter == nil {
		cfg.SourceSnapshotter = gitstate.CaptureSourceSnapshot
	}
	minimumLock := cfg.CodexTimeout + 2*cfg.GitTimeout + time.Minute
	if cfg.SourceLockTimeout <= 0 {
		cfg.SourceLockTimeout = minimumLock
	} else if cfg.SourceLockTimeout < minimumLock {
		return normalizedConfig{}, fmt.Errorf("run supervisor: source lock timeout %s is shorter than required pass window %s", cfg.SourceLockTimeout, minimumLock)
	}
	return normalizedConfig{Config: cfg, root: abs, runID: runID, decisionID: decisionID}, nil
}

func prepareArtifactPaths(root, runID string) (artifactPaths, error) {
	base := filepath.ToSlash(filepath.Join(".revolvr", "runs", runID))
	paths := artifactPaths{
		prompt:         base + "/supervisor-prompt.md",
		schema:         base + "/supervisor-output-schema.json",
		rawOutput:      base + "/supervisor-output.json",
		decision:       base + "/supervisor-decision.json",
		provenance:     base + "/supervisor-provenance.json",
		sourceEvidence: base + "/supervisor-source.json",
		diagnostics:    base + "/supervisor-diagnostics.json",
		codexStdout:    base + "/codex.jsonl",
		codexStderr:    base + "/codex.stderr",
	}
	for _, path := range []string{paths.prompt, paths.schema, paths.rawOutput, paths.decision, paths.provenance, paths.sourceEvidence, paths.diagnostics, paths.codexStdout, paths.codexStderr} {
		if _, err := pathguard.Resolve(root, path); err != nil {
			return artifactPaths{}, fmt.Errorf("prepare supervisor artifact path %q: %w", path, err)
		}
	}
	return paths, nil
}

func artifactsWithPaths(paths artifactPaths) Artifacts {
	return Artifacts{
		Prompt:         Artifact{Path: paths.prompt},
		Schema:         Artifact{Path: paths.schema},
		RawOutput:      Artifact{Path: paths.rawOutput},
		Decision:       Artifact{Path: paths.decision},
		Provenance:     Artifact{Path: paths.provenance},
		SourceEvidence: Artifact{Path: paths.sourceEvidence},
		Diagnostics:    Artifact{Path: paths.diagnostics},
		CodexStdout:    Artifact{Path: paths.codexStdout},
		CodexStderr:    Artifact{Path: paths.codexStderr},
	}
}

func reject(ctx context.Context, cfg normalizedConfig, paths artifactPaths, result Result, category string, cause error, source sourceEvidence, codexAttempted bool) (Result, error) {
	result.Decision = nil
	result.DecisionReference = nil
	diagnostics := rejectionDiagnostics{
		Category:  category,
		Reason:    errorText(cause),
		Codex:     diagnosticsForCodex(result.Codex),
		Source:    source,
		Artifacts: result.Artifacts,
	}
	raw, marshalErr := marshalIndented(diagnostics)
	if marshalErr == nil {
		result.Artifacts.Diagnostics, marshalErr = writeArtifact(cfg.root, paths.diagnostics, raw)
	}
	payload := map[string]any{
		"run_id":            cfg.runID,
		"task_id":           cfg.TaskID,
		"category":          category,
		"reason":            errorText(cause),
		"artifacts":         result.Artifacts,
		"source_difference": result.SourceDifference,
		"codex":             diagnosticsForCodex(result.Codex),
	}
	_, rejectedErr := cfg.Ledger.AppendEvent(ctx, cfg.runID, ledger.EventSupervisorRejected, payload)
	completedAt := cfg.Clock().UTC()
	completion := ledger.RunCompletion{
		Status:             ledger.StatusFailed,
		Summary:            "supervisor decision rejected: " + category + ": " + errorText(cause),
		CompletedAt:        completedAt,
		VerificationStatus: "not_run",
	}
	if codexAttempted {
		exitCode := result.Codex.ExitCode
		completion.CodexExitCode = &exitCode
	}
	completed, ok, completionErr := cfg.Ledger.CompleteRun(ctx, cfg.runID, completion)
	if completionErr == nil && ok {
		result.LedgerRun = completed
	} else if completionErr == nil {
		completionErr = fmt.Errorf("complete rejected supervisor run: run %q not found", cfg.runID)
	}
	_, terminalErr := cfg.Ledger.AppendEvent(ctx, cfg.runID, ledger.EventRunFailed, map[string]any{
		"run_id":              cfg.runID,
		"task_id":             cfg.TaskID,
		"pass":                "supervisor",
		"outcome":             "decision_rejected",
		"category":            category,
		"reason":              errorText(cause),
		"verification_status": "not_run",
		"commit_sha":          "",
		"artifacts":           result.Artifacts,
	})
	return result, errors.Join(cause, marshalErr, rejectedErr, completionErr, terminalErr)
}

func codexFailure(result codexexec.Result, runErr error) error {
	if runErr != nil {
		return fmt.Errorf("run Codex supervisor invocation: %w", runErr)
	}
	if result.TimedOut {
		return errors.New("Codex supervisor invocation timed out")
	}
	if result.Err != nil {
		return fmt.Errorf("Codex supervisor invocation failed: %w", result.Err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("Codex supervisor invocation exited with code %d", result.ExitCode)
	}
	if result.ArtifactError != nil {
		return fmt.Errorf("Codex supervisor artifact failure: %w", result.ArtifactError)
	}
	if result.LedgerError != nil {
		return fmt.Errorf("Codex supervisor ledger failure: %w", result.LedgerError)
	}
	if result.ParseError != nil {
		return fmt.Errorf("Codex supervisor JSONL parse failure: %w", result.ParseError)
	}
	if len(result.JSONParseErrors) != 0 {
		return fmt.Errorf("Codex supervisor JSONL contains parse errors: %s", strings.Join(result.JSONParseErrors, "; "))
	}
	return nil
}

func writeSourceEvidence(root, path string, evidence sourceEvidence) (Artifact, error) {
	raw, err := marshalIndented(evidence)
	if err != nil {
		return Artifact{Path: path}, err
	}
	return writeArtifact(root, path, raw)
}

func writeArtifact(root, path string, content []byte) (Artifact, error) {
	abs, err := pathguard.Resolve(root, path)
	if err != nil {
		return Artifact{Path: path}, fmt.Errorf("resolve artifact %q: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return Artifact{Path: path}, fmt.Errorf("create artifact directory for %q: %w", path, err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		return Artifact{Path: path}, fmt.Errorf("write artifact %q: %w", path, err)
	}
	return artifactForBytes(path, content), nil
}

func referenceArtifact(root, path string) (Artifact, error) {
	abs, err := pathguard.Resolve(root, path)
	if err != nil {
		return Artifact{Path: path}, err
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return Artifact{Path: path}, fmt.Errorf("read artifact %q: %w", path, err)
	}
	return artifactForBytes(path, raw), nil
}

func artifactForBytes(path string, raw []byte) Artifact {
	return Artifact{Path: path, SHA256: sha256Hex(raw), ByteSize: len(raw)}
}

func marshalIndented(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func diagnosticsForCodex(result codexexec.Result) codexDiagnostics {
	return codexDiagnostics{
		ExitCode:        result.ExitCode,
		TimedOut:        result.TimedOut,
		Error:           errorText(result.Err),
		ParseError:      errorText(result.ParseError),
		ArtifactError:   errorText(result.ArtifactError),
		LedgerError:     errorText(result.LedgerError),
		JSONParseErrors: append([]string(nil), result.JSONParseErrors...),
		StdoutTruncated: result.Stdout.TruncatedBytes,
		StderrTruncated: result.Stderr.TruncatedBytes,
	}
}

func releaseSourceLock(sourceLock *lock.SourceWriter) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return sourceLock.Release(ctx)
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func boolPointer(value bool) *bool {
	return &value
}

func safeRunID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validDecisionID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for i, r := range value {
		switch {
		case i == 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && r == '-' && value[i-1] != '-' && i < len(value)-1:
		default:
			return false
		}
	}
	return true
}
