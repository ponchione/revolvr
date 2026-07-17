package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/autonomoustaskrun"
	"revolvr/internal/autonomousworkspace"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/repositorypath"
	"revolvr/internal/taskfile"
)

const AutonomousRecoverySchemaVersion = "revolvr-autonomous-task-recovery-v1"

type RecoverAutonomousTaskInput struct {
	TaskID           string
	OperationID      string
	Reconcile        bool
	ConfirmOperation string
	Clock            func() time.Time
}

type RecoveryAuthorityCheck struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type RecoverAutonomousTaskResult struct {
	SchemaVersion     string                       `json:"schema_version"`
	TaskID            string                       `json:"task_id"`
	OperationID       string                       `json:"operation_id"`
	StopReason        autonomoustaskrun.StopReason `json:"stop_reason,omitempty"`
	Checks            []RecoveryAuthorityCheck     `json:"checks"`
	AuthoritySHA256   string                       `json:"authority_sha256"`
	Ready             bool                         `json:"ready"`
	ReconcileEligible bool                         `json:"reconcile_eligible"`
	Reconciled        bool                         `json:"reconciled,omitempty"`
	Replayed          bool                         `json:"replayed,omitempty"`
	NewOperationID    string                       `json:"new_operation_id,omitempty"`
}

type recoveryInspection struct {
	result    RecoverAutonomousTaskResult
	operation autonomoustaskrun.Operation
	task      taskfile.Task
	state     autonomousstate.Snapshot
	workspace *autonomous.TaskWorkspace
}

// RecoverAutonomousTask inspects exact durable task-operation authority. It
// performs no mutation unless Reconcile and the exact old-operation
// confirmation are both supplied.
func RecoverAutonomousTask(ctx context.Context, cfg Config, input RecoverAutonomousTaskInput) (RecoverAutonomousTaskResult, error) {
	if err := validateRecoveryInput(input); err != nil {
		return RecoverAutonomousTaskResult{}, err
	}
	inspection, err := inspectAutonomousRecovery(ctx, cfg, input)
	if err != nil {
		return RecoverAutonomousTaskResult{}, err
	}
	if !input.Reconcile {
		return inspection.result, nil
	}
	if !inspection.result.ReconcileEligible {
		return inspection.result, errors.New("task recovery: reconciliation requires terminal unsafe_or_ambiguous evidence and agreement from every authority check")
	}

	unlock, err := autonomousexec.Acquire(ctx, cfg.WorkDir)
	if err != nil {
		return inspection.result, err
	}
	defer unlock()
	current, err := inspectAutonomousRecovery(ctx, cfg, input)
	if err != nil {
		return inspection.result, err
	}
	if !current.result.ReconcileEligible || current.result.AuthoritySHA256 != inspection.result.AuthoritySHA256 || !reflect.DeepEqual(current.operation, inspection.operation) {
		return current.result, errors.New("task recovery: authority changed while reconciliation was being admitted")
	}

	authoritySHA := current.result.AuthoritySHA256
	newOperationID := "task-recovery-" + recoveryHash("revolvr-task-recovery-operation-v1", input.OperationID, authoritySHA)[:24]
	now := time.Now().UTC()
	if input.Clock != nil {
		now = input.Clock().UTC()
	}
	var bounds *autonomoustaskrun.EffectiveBounds
	if current.operation.EffectiveBounds != nil {
		value := *current.operation.EffectiveBounds
		value.ActionAttempts = append([]autonomoustaskrun.ActionAttemptBound(nil), value.ActionAttempts...)
		bounds = &value
	}
	next := autonomoustaskrun.Operation{
		SchemaVersion:   autonomoustaskrun.OperationSchemaVersion,
		OperationID:     newOperationID,
		TaskID:          current.operation.TaskID,
		Task:            current.operation.Task,
		State:           current.operation.State,
		WorkspaceID:     current.operation.WorkspaceID,
		CheckpointSHA:   current.operation.CheckpointSHA,
		ConfigSHA256:    current.operation.ConfigSHA256,
		EffectiveBounds: bounds,
		MaxCycles:       current.operation.MaxCycles,
		StartedAt:       now,
		UpdatedAt:       now,
		Stage:           "admitted",
		Evidence: []string{
			"reconciled_from_operation:" + current.operation.OperationID,
			"recovery_authority_sha256:" + authoritySHA,
		},
	}
	created, replayed, err := autonomoustaskrun.CreateReconciledOperation(ctx, cfg.WorkDir, current.operation, next)
	if err != nil {
		return current.result, err
	}
	current.result.Reconciled = true
	current.result.Replayed = replayed
	current.result.NewOperationID = created.OperationID
	return current.result, nil
}

func validateRecoveryInput(input RecoverAutonomousTaskInput) error {
	if strings.TrimSpace(input.TaskID) == "" || input.TaskID != strings.TrimSpace(input.TaskID) {
		return errors.New("task recovery: exact task id is required without surrounding whitespace")
	}
	if strings.TrimSpace(input.OperationID) == "" || input.OperationID != strings.TrimSpace(input.OperationID) {
		return errors.New("task recovery: exact --operation-id is required without surrounding whitespace")
	}
	if !input.Reconcile && input.ConfirmOperation != "" {
		return errors.New("task recovery: --confirm-operation requires --reconcile")
	}
	if input.Reconcile && input.ConfirmOperation != input.OperationID {
		return errors.New("task recovery: --confirm-operation must exactly match --operation-id")
	}
	return nil
}

func inspectAutonomousRecovery(ctx context.Context, cfg Config, input RecoverAutonomousTaskInput) (recoveryInspection, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return recoveryInspection{}, err
	}
	repository, err := repositorypath.Inspect(paths.WorkDir, repositorypath.InspectOptions{})
	if err != nil {
		return recoveryInspection{}, err
	}
	if !repository.Initialized() {
		return recoveryInspection{}, errors.New("task recovery: state is not initialized; run `revolvr init` first")
	}
	op, found, err := autonomoustaskrun.Inspect(paths.WorkDir, input.OperationID)
	if err != nil {
		return recoveryInspection{}, fmt.Errorf("task recovery: inspect operation: %w", err)
	}
	if !found {
		return recoveryInspection{}, fmt.Errorf("task recovery: operation %q not found", input.OperationID)
	}
	if op.TaskID != input.TaskID {
		return recoveryInspection{}, fmt.Errorf("task recovery: operation %q belongs to task %q, not %q", input.OperationID, op.TaskID, input.TaskID)
	}

	inspection := recoveryInspection{operation: op}
	result := RecoverAutonomousTaskResult{SchemaVersion: AutonomousRecoverySchemaVersion, TaskID: input.TaskID, OperationID: input.OperationID, StopReason: op.StopReason}
	add := func(name string, passed bool, detail string) {
		result.Checks = append(result.Checks, RecoveryAuthorityCheck{Name: name, Passed: passed, Detail: detail})
	}

	task, taskFound, taskErr := taskfile.FindByID(paths.WorkDir, input.TaskID)
	if taskErr != nil || !taskFound {
		add("task", false, recoveryDetail(taskErr, "canonical task is missing"))
	} else {
		inspection.task = task
		passed := task.SourcePath == op.Task.Path && task.SourceSHA256() == op.Task.SHA256 && task.SourceByteSize() == op.Task.ByteSize
		add("task", passed, fmt.Sprintf("path=%s sha256=%s bytes=%d", task.SourcePath, task.SourceSHA256(), task.SourceByteSize()))
	}

	stateStore, stateErr := autonomousstate.New(autonomousstate.Config{RepositoryRoot: paths.WorkDir})
	var stateFound bool
	if stateErr == nil {
		inspection.state, stateFound, stateErr = stateStore.Load(ctx, input.TaskID)
	}
	if stateErr != nil || !stateFound {
		add("state", false, recoveryDetail(stateErr, "canonical autonomous state is missing"))
	} else {
		passed := inspection.state.SourcePath == op.State.Path && inspection.state.SHA256 == op.State.SHA256 && inspection.state.ByteSize == op.State.ByteSize
		add("state", passed, fmt.Sprintf("path=%s sha256=%s bytes=%d lifecycle=%s", inspection.state.SourcePath, inspection.state.SHA256, inspection.state.ByteSize, inspection.state.State.Lifecycle))
	}

	workspacePassed := false
	if stateFound && stateErr == nil && inspection.state.State.Workspace != nil {
		workspace := *inspection.state.State.Workspace
		inspection.workspace = &workspace
		workspaceErr := workspace.Validate()
		workspacePassed = workspaceErr == nil && workspace.WorkspaceID == op.WorkspaceID && workspace.Checkpoint.CommitSHA == op.CheckpointSHA
		add("workspace", workspacePassed, recoveryDetail(workspaceErr, fmt.Sprintf("id=%s execution=%s head=%s checkpoint=%s", workspace.WorkspaceID, workspace.ExecutionRoot, workspace.HeadSHA, workspace.Checkpoint.CommitSHA)))
	} else {
		add("workspace", false, "durable state has no task workspace authority")
	}

	if workspacePassed {
		runCfg, loadErr := LoadRunOnceConfig(paths.WorkDir, DefaultRunOnceConfig(paths.WorkDir))
		if loadErr != nil {
			add("git", false, loadErr.Error())
		} else {
			observed, inspectErr := autonomousworkspace.Inspect(ctx, autonomousworkspace.Config{
				ControlRoot: paths.WorkDir, TaskID: input.TaskID, OperationID: input.OperationID + "-recovery-inspect",
				BaselineSHA: inspection.workspace.BaselineSHA, GitExecutable: runCfg.GitExecutable,
				Timeout: runCfg.GitTimeout, StdoutCap: runCfg.GitStdoutCap, StderrCap: runCfg.GitStderrCap,
				Clock: func() time.Time { return inspection.workspace.UpdatedAt },
			}, *inspection.workspace)
			passed := inspectErr == nil && recoveryWorkspaceEqual(observed.Workspace, *inspection.workspace)
			detail := fmt.Sprintf("branch=%s expected_head=%s observed_head=%s expected_tree=%s observed_tree=%s expected_source=%s observed_source=%s", inspection.workspace.BranchRef, inspection.workspace.HeadSHA, observed.Workspace.HeadSHA, inspection.workspace.TreeSHA, observed.Workspace.TreeSHA, inspection.workspace.SourceRevision, observed.Workspace.SourceRevision)
			add("git", passed, recoveryDetail(inspectErr, detail))
		}
	} else {
		add("git", false, "workspace authority must agree before Git inspection")
	}

	runs, ledgerErr := openReadOnlyLedger(ctx, paths)
	var referenced ledger.RunWithEvents
	var referencedFound bool
	if ledgerErr == nil {
		defer runs.Close()
	}
	ledgerPassed := false
	ledgerDetail := ""
	if ledgerErr != nil {
		ledgerDetail = ledgerErr.Error()
	} else {
		ledgerPassed, ledgerDetail = inspectRecoveryLedger(ctx, runs, op)
		if op.LastRunID != "" {
			referenced, referencedFound, err = runs.GetRunWithEvents(ctx, op.LastRunID)
			if err != nil {
				referencedFound = false
			}
		}
	}
	add("ledger", ledgerPassed, ledgerDetail)

	if op.LastRunID == "" {
		add("receipt", true, "not applicable: old operation names no worker or supervisor run")
		add("artifacts", true, "not applicable: old operation names no worker or supervisor run")
	} else if !referencedFound {
		add("receipt", false, fmt.Sprintf("referenced run %s is missing from the ledger", op.LastRunID))
		add("artifacts", false, fmt.Sprintf("referenced run %s is missing from the ledger", op.LastRunID))
	} else {
		artifacts, artifactsFound := ledger.RunArtifactsFromEvents(referenced.Events)
		if !artifactsFound || strings.TrimSpace(artifacts.ReceiptPath) == "" {
			add("receipt", true, fmt.Sprintf("not applicable: referenced run %s claims no receipt", op.LastRunID))
		} else {
			receiptRaw, present, protectedErr := repository.ReadFile(artifacts.ReceiptPath, false)
			if protectedErr != nil || !present {
				add("receipt", false, recoveryDetail(protectedErr, fmt.Sprintf("receipt %s is missing", artifacts.ReceiptPath)))
			} else if validation, validationErr := receipt.ValidateRunReceipt(receipt.ValidationInput{WorkDir: paths.WorkDir, History: referenced}); validationErr != nil {
				add("receipt", false, validationErr.Error())
			} else if receiptAfter, stillPresent, rereadErr := repository.ReadFile(artifacts.ReceiptPath, false); rereadErr != nil || !stillPresent || !reflect.DeepEqual(receiptAfter, receiptRaw) {
				add("receipt", false, recoveryDetail(rereadErr, fmt.Sprintf("receipt %s changed during validation", artifacts.ReceiptPath)))
			} else {
				add("receipt", validation.Passed(), fmt.Sprintf("run=%s path=%s checks=%d sha256=%s", op.LastRunID, validation.ReceiptPath, len(validation.Checks), recoveryBytesHash(receiptRaw)))
			}
		}
		passed, detail := inspectRecoveryArtifacts(repository, op.LastRunID, referenced.Events, artifacts, artifactsFound)
		add("artifacts", passed, detail)
	}

	result.Ready = true
	for _, check := range result.Checks {
		if !check.Passed {
			result.Ready = false
			break
		}
	}
	result.ReconcileEligible = result.Ready && op.StopReason == autonomoustaskrun.StopUnsafeAmbiguous && op.Stage == "terminal" && op.CompletedAt != nil && !op.InFlight
	projection := struct {
		Operation autonomoustaskrun.Operation `json:"operation"`
		Checks    []RecoveryAuthorityCheck    `json:"checks"`
	}{Operation: op, Checks: result.Checks}
	raw, err := json.Marshal(projection)
	if err != nil {
		return recoveryInspection{}, err
	}
	sum := sha256.Sum256(raw)
	result.AuthoritySHA256 = hex.EncodeToString(sum[:])
	inspection.result = result
	return inspection, nil
}

func inspectRecoveryLedger(ctx context.Context, runs *ledger.Store, op autonomoustaskrun.Operation) (bool, string) {
	runID := autonomoustaskrun.LedgerRunID(op.OperationID)
	history, found, err := runs.GetRunWithEvents(ctx, runID)
	if err != nil {
		return false, err.Error()
	}
	if !found {
		return false, fmt.Sprintf("task-operation ledger run %s is missing", runID)
	}
	if history.Run.TaskID != op.TaskID || !history.Run.StartedAt.Equal(op.StartedAt) {
		return false, fmt.Sprintf("task-operation ledger run %s conflicts with task or start authority", runID)
	}
	admitted, stopped := false, false
	for _, event := range history.Events {
		if event.Type != ledger.EventTaskRunAdmitted && event.Type != ledger.EventTaskRunCycleStarted && event.Type != ledger.EventTaskRunCycleCompleted && event.Type != ledger.EventTaskRunRestarted && event.Type != ledger.EventTaskRunStopped {
			continue
		}
		decoded, decodeErr := autonomoustaskrun.DecodeLedgerEvent(event.Payload)
		if decodeErr != nil || decoded.OperationID != op.OperationID || decoded.TaskID != op.TaskID {
			return false, recoveryDetail(decodeErr, fmt.Sprintf("task-operation ledger event %d conflicts with durable operation", event.ID))
		}
		if event.Type == ledger.EventTaskRunAdmitted && decoded.Sequence == 0 && decoded.Stage == "admitted" {
			admitted = true
		}
		if event.Type == ledger.EventTaskRunStopped && decoded.Sequence == op.Sequence && decoded.Stage == op.Stage && decoded.StopReason == op.StopReason && decoded.CompletedAt != nil && op.CompletedAt != nil && decoded.CompletedAt.Equal(*op.CompletedAt) && reflect.DeepEqual(decoded.Statistics, op.Statistics) {
			stopped = true
		}
	}
	if !admitted || !stopped || history.Run.Status != ledger.StatusCompleted || history.Run.CompletedAt == nil || op.CompletedAt == nil || !history.Run.CompletedAt.Equal(*op.CompletedAt) {
		return false, fmt.Sprintf("task-operation ledger run %s lacks exact admitted/stopped/completed authority", runID)
	}
	return true, fmt.Sprintf("run=%s events=%d stop=%s sequence=%d", runID, len(history.Events), op.StopReason, op.Sequence)
}

func inspectRecoveryArtifacts(repository repositorypath.Authority, runID string, events []ledger.Event, artifacts ledger.RunArtifacts, found bool) (bool, string) {
	if !found || artifacts.Empty() {
		return true, fmt.Sprintf("not applicable: referenced run %s claims no artifact paths", runID)
	}
	paths := recoveryArtifactPaths(artifacts)
	identities, identityErr := recoveryArtifactIdentities(events)
	if identityErr != nil {
		return false, identityErr.Error()
	}
	details := make([]string, 0, len(paths))
	for _, item := range paths {
		clean := filepath.Clean(filepath.FromSlash(item.path))
		if filepath.IsAbs(clean) || clean == "." || clean != filepath.FromSlash(item.path) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return false, fmt.Sprintf("%s path %q is not a canonical repository-relative path", item.name, item.path)
		}
		raw, present, err := repository.ReadFile(filepath.ToSlash(clean), false)
		if err != nil || !present {
			return false, recoveryDetail(err, fmt.Sprintf("%s artifact %s is missing", item.name, item.path))
		}
		sum := sha256.Sum256(raw)
		digest := hex.EncodeToString(sum[:])
		identity, exact := identities[item.path]
		if exact && (identity.sha256 != digest || identity.byteSize != len(raw)) {
			return false, fmt.Sprintf("%s artifact %s identity disagrees: recorded sha256=%s bytes=%d current sha256=%s bytes=%d", item.name, item.path, identity.sha256, identity.byteSize, digest, len(raw))
		}
		if !exact && item.name != "receipt" {
			return false, fmt.Sprintf("%s artifact %s has no exact recorded SHA-256 and byte-size authority", item.name, item.path)
		}
		details = append(details, fmt.Sprintf("%s=%s:%d@%s", item.name, digest, len(raw), item.path))
	}
	return true, strings.Join(details, " ")
}

type recoveryArtifactIdentity struct {
	sha256   string
	byteSize int
}

func recoveryArtifactIdentities(events []ledger.Event) (map[string]recoveryArtifactIdentity, error) {
	identities := map[string]recoveryArtifactIdentity{}
	for _, event := range events {
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(event.Payload)))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			continue
		}
		var walk func(any) error
		walk = func(current any) error {
			switch typed := current.(type) {
			case []any:
				for _, child := range typed {
					if err := walk(child); err != nil {
						return err
					}
				}
			case map[string]any:
				path, pathOK := recoveryStringField(typed, "path")
				digest, digestOK := recoveryStringField(typed, "sha256")
				size, sizeOK := recoveryIntegerField(typed, "byte_size")
				if pathOK && digestOK && sizeOK && len(digest) == 64 && size >= 0 {
					identity := recoveryArtifactIdentity{sha256: digest, byteSize: size}
					if prior, exists := identities[path]; exists && prior != identity {
						return fmt.Errorf("artifact %s has conflicting recorded identities", path)
					}
					identities[path] = identity
				}
				for _, child := range typed {
					if err := walk(child); err != nil {
						return err
					}
				}
			}
			return nil
		}
		if err := walk(value); err != nil {
			return nil, err
		}
	}
	return identities, nil
}

func recoveryStringField(value map[string]any, name string) (string, bool) {
	for key, raw := range value {
		if strings.EqualFold(key, name) {
			text, ok := raw.(string)
			return text, ok && strings.TrimSpace(text) != ""
		}
	}
	return "", false
}

func recoveryIntegerField(value map[string]any, name string) (int, bool) {
	for key, raw := range value {
		if !strings.EqualFold(key, name) {
			continue
		}
		number, ok := raw.(json.Number)
		if !ok {
			return 0, false
		}
		parsed, err := number.Int64()
		if err != nil || int64(int(parsed)) != parsed {
			return 0, false
		}
		return int(parsed), true
	}
	return 0, false
}

type recoveryArtifactPath struct{ name, path string }

func recoveryArtifactPaths(a ledger.RunArtifacts) []recoveryArtifactPath {
	values := []recoveryArtifactPath{
		{"context", a.ContextPayloadPath}, {"context_manifest", a.ContextManifestPath}, {"codex_jsonl", a.CodexStdoutJSONLPath},
		{"codex_stderr", a.CodexStderrPath}, {"last_message", a.LastMessagePath}, {"receipt", a.ReceiptPath},
		{"dossier", a.DossierPath}, {"dossier_manifest", a.DossierManifestPath}, {"supervisor_dossier", a.SupervisorDossierPath},
		{"supervisor_dossier_manifest", a.SupervisorDossierManifestPath}, {"supervisor_prompt", a.SupervisorPromptPath},
		{"supervisor_schema", a.SupervisorSchemaPath}, {"supervisor_output", a.SupervisorOutputPath},
		{"supervisor_decision", a.SupervisorDecisionPath}, {"supervisor_provenance", a.SupervisorProvenancePath},
		{"supervisor_source", a.SupervisorSourcePath}, {"supervisor_diagnostics", a.SupervisorDiagnosticsPath},
		{"verification", a.VerificationEvidencePath},
	}
	result := values[:0]
	for _, value := range values {
		if strings.TrimSpace(value.path) != "" {
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result
}

func recoveryWorkspaceEqual(observed, expected autonomous.TaskWorkspace) bool {
	observed.UpdatedAt = expected.UpdatedAt
	return reflect.DeepEqual(observed, expected)
}

func recoveryDetail(err error, otherwise string) string {
	if err != nil {
		return err.Error()
	}
	return otherwise
}

func recoveryHash(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func recoveryBytesHash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
