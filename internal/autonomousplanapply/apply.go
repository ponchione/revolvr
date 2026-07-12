// Package autonomousplanapply is the narrow AW-11 coordinator that consumes
// one successful AW-10 planner result and persists one validated AW-02 state
// transition through autonomousstate.
package autonomousplanapply

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomousplanning"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/commit"
	"revolvr/internal/pathguard"
	"revolvr/internal/supervisor"
	"revolvr/internal/taskfile"
)

type Config struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	InitialState   *autonomous.ExecutionState
	Cycle          autonomouscycle.Result
	CreatedAt      time.Time
	Store          *autonomousstate.Store
}

type Disposition string

const (
	DispositionCreated  Disposition = "created"
	DispositionUpdated  Disposition = "updated"
	DispositionReplayed Disposition = "replayed"
)

type PlannerIdentity struct {
	SupervisorRunID string
	DecisionID      string
	WorkerRunID     string
	Profile         autonomousplanning.ProfileIdentity
	RawOutput       autonomousstate.ArtifactIdentity
	CanonicalOutput autonomousstate.ArtifactIdentity
	Dossier         autonomousplanning.DossierIdentity
	SourceRevision  string
}

type Failure struct {
	Stage  string
	Reason string
}

type Result struct {
	TaskID       string
	OperationID  string
	Disposition  Disposition
	StatePath    string
	Previous     autonomousstate.StateIdentity
	Current      autonomousstate.Snapshot
	PreviousPlan *autonomousstate.PlanIdentity
	CurrentPlan  autonomousstate.PlanIdentity
	Acceptance   autonomousstate.AcceptanceCounts
	History      autonomousstate.HistorySnapshot
	Planner      PlannerIdentity
	State        autonomous.ExecutionState
	Failure      *Failure
}

func ApplyPlanningResult(ctx context.Context, cfg Config) (Result, error) {
	result := Result{TaskID: cfg.TaskID, OperationID: cfg.OperationID}
	root, err := repositoryRoot(cfg.RepositoryRoot)
	if err != nil {
		return reject(result, "configuration", err)
	}
	if err := stableIdentity("task_id", cfg.TaskID); err != nil {
		return reject(result, "configuration", err)
	}
	if err := stableIdentity("operation_id", cfg.OperationID); err != nil {
		return reject(result, "configuration", err)
	}
	if cfg.CreatedAt.IsZero() {
		return reject(result, "configuration", errors.New("created_at is required"))
	}
	if err := cfg.Expected.Validate(); err != nil {
		return reject(result, "configuration", fmt.Errorf("expected state: %w", err))
	}
	task, found, err := taskfile.FindByID(root, cfg.TaskID)
	if err != nil {
		return reject(result, "task", err)
	}
	if !found {
		return reject(result, "task", fmt.Errorf("%w: %q", autonomousstate.ErrTaskMissing, cfg.TaskID))
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 || task.Status != taskfile.StatusPending || task.AutonomousStatePath == "" {
		return reject(result, "task", fmt.Errorf("canonical task %q is not a pending autonomous-v1 task", cfg.TaskID))
	}
	result.StatePath = task.AutonomousStatePath

	store := cfg.Store
	if store == nil {
		store, err = autonomousstate.New(autonomousstate.Config{RepositoryRoot: root})
		if err != nil {
			return reject(result, "state_store", err)
		}
	}
	if err := validateCycle(root, task, cfg.Cycle); err != nil {
		return reject(result, "cycle_evidence", err)
	}
	rawOutput, rawIdentity, err := readCycleArtifact(root, cfg.Cycle.Worker.Artifacts.Output)
	if err != nil {
		return reject(result, "planner_raw_output", err)
	}
	if !bytes.Equal(rawOutput, cfg.Cycle.Worker.RawOutput) {
		return reject(result, "planner_raw_output", errors.New("cycle raw output bytes do not match the exact output artifact"))
	}
	output, err := autonomousplanning.ParsePlanningOutput(rawOutput)
	if err != nil {
		return reject(result, "planner_output", err)
	}
	if err := validateOutputProvenance(cfg.Cycle, output); err != nil {
		return reject(result, "planner_output_identity", err)
	}
	canonicalOutput, err := autonomousplanning.MarshalPlanningOutput(output)
	if err != nil {
		return reject(result, "planner_output", err)
	}
	canonicalPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", cfg.Cycle.Worker.RunID, "planner-output.canonical.json"))
	canonicalIdentity := identityFor(canonicalPath, canonicalOutput)
	applicationSHA, err := applicationSHA256(cfg, rawIdentity, canonicalIdentity)
	if err != nil {
		return reject(result, "application_identity", err)
	}

	existingOperation, operationFound, err := store.LoadPlanningOperation(ctx, cfg.TaskID, cfg.OperationID)
	if err != nil {
		return reject(result, "history_load", err)
	}
	current, currentFound, err := store.Load(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "state_load", err)
	}
	if operationFound {
		if existingOperation.Record.ApplicationSHA256 != applicationSHA {
			return reject(result, "operation_conflict", fmt.Errorf("%w: operation %q has different planner evidence", autonomousstate.ErrOperationConflict, cfg.OperationID))
		}
		if currentFound && current.SHA256 == existingOperation.Record.ResultingState.SHA256 && current.ByteSize == existingOperation.Record.ResultingState.ByteSize {
			replayed, found, err := store.ReplayPlanning(ctx, cfg.TaskID, cfg.OperationID, applicationSHA)
			if err != nil {
				return reject(result, "history_evidence", err)
			}
			if !found {
				return reject(result, "state_compare", fmt.Errorf("%w: committed operation changed during replay", autonomousstate.ErrStaleWrite))
			}
			return committedResult(result, replayed, replayed.History.Record), nil
		}
	}
	if err := compareExpected(cfg.Expected, current, currentFound); err != nil {
		return reject(result, "state_compare", err)
	}

	var previous autonomous.ExecutionState
	if currentFound {
		previous = current.State
		if cfg.InitialState != nil {
			return reject(result, "state_compare", errors.New("initial_state is only valid when canonical state is expected absent"))
		}
	} else {
		if cfg.InitialState == nil {
			return reject(result, "state_compare", errors.New("initial_state is required when creating absent canonical state"))
		}
		previous, err = cloneState(*cfg.InitialState)
		if err != nil {
			return reject(result, "state_compare", err)
		}
	}
	if err := validateDossierStateAndTask(task, previous, cfg.Cycle.DossierManifest); err != nil {
		return reject(result, "dossier_identity", err)
	}
	taskOrigin := autonomousplanning.CanonicalTaskOrigin(task.SourcePath, task.SourceSHA256())
	next, change, err := autonomousplanning.ApplyProposal(previous, output, *cfg.Cycle.Supervisor.Decision, taskOrigin, task.SourceBytes)
	if err != nil {
		stage := "plan_revision"
		if previous.Plan == nil {
			stage = "initial_plan"
		}
		if strings.Contains(err.Error(), "acceptance matrix") {
			stage = "acceptance_matrix"
		}
		return reject(result, stage, err)
	}
	previousBytes, err := autonomousstate.MarshalState(previous)
	if err != nil {
		return reject(result, "state_encoding", err)
	}
	nextBytes, err := autonomousstate.MarshalState(next)
	if err != nil {
		return reject(result, "state_encoding", err)
	}
	previousIdentity := stateIdentity(task.AutonomousStatePath, cfg.Expected.Exists, previousBytes)
	resultingIdentity := stateIdentity(task.AutonomousStatePath, true, nextBytes)
	_, supervisorArtifact, err := readSupervisorArtifact(root, cfg.Cycle.Supervisor.Artifacts.Decision)
	if err != nil {
		return reject(result, "cycle_evidence", fmt.Errorf("reopen supervisor decision artifact: %w", err))
	}
	historyChange := autonomousstate.PlanningChangeCreated
	if change == autonomousplanning.ChangeKindRevision {
		historyChange = autonomousstate.PlanningChangeRevised
	}
	previousPlan := clonePlan(previous.Plan)
	record := autonomousstate.PlanningHistoryRecord{
		SchemaVersion: autonomousstate.PlanningHistorySchemaVersion,
		TaskID:        cfg.TaskID, OperationID: cfg.OperationID, ApplicationSHA256: applicationSHA,
		Change: historyChange, CreatedAt: cfg.CreatedAt.UTC(),
		Decision:           *cfg.Cycle.Supervisor.DecisionReference,
		SupervisorDecision: supervisorArtifact,
		WorkerRunID:        cfg.Cycle.Worker.RunID,
		Profile:            output.Provenance.Profile, Dossier: output.Provenance.Dossier,
		SourceRevision: output.Provenance.SourceRevision,
		TaskSource:     identityFor(task.SourcePath, task.SourceBytes),
		RawOutput:      rawIdentity, CanonicalOutput: canonicalIdentity,
		PreviousState: previousIdentity, ResultingState: resultingIdentity,
		PreviousPlan: previousPlan, ResultingPlan: *next.Plan,
		PreviousAcceptance:    cloneAcceptance(previous.AcceptanceCriteria),
		ResultingAcceptance:   cloneAcceptance(next.AcceptanceCriteria),
		PreviousPlanIdentity:  autonomousstate.PlanIdentityFor(previous.Plan),
		ResultingPlanIdentity: *autonomousstate.PlanIdentityFor(next.Plan),
		Acceptance:            autonomousstate.CountAcceptance(next.AcceptanceCriteria),
	}
	commitResult, err := store.CommitPlanning(ctx, autonomousstate.CommitRequest{
		TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: previous,
		NextState: next, History: record, CanonicalOutput: canonicalOutput,
	})
	if err != nil {
		stage := "state_persistence"
		if strings.Contains(err.Error(), "history") {
			stage = "history_persistence"
		}
		return reject(result, stage, err)
	}
	return committedResult(result, commitResult, record), nil
}

func validateCycle(root string, task taskfile.Task, result autonomouscycle.Result) error {
	if result.Failure != nil || result.Outcome != autonomouscycle.OutcomeReadOnlyCompleted {
		return fmt.Errorf("planner cycle outcome is %q with failure %+v", result.Outcome, result.Failure)
	}
	if result.TaskID != task.ID || result.Route == nil || result.Route.Kind != autonomouspolicy.RouteKindWorker || result.Route.TaskID != task.ID || result.Route.Action != autonomous.ActionPlan || result.Route.WorkerProfile != autonomous.WorkerProfilePlanner {
		return errors.New("cycle route is not the exact task plan -> planner worker authorization")
	}
	if result.Supervisor.Decision == nil || result.Supervisor.DecisionReference == nil {
		return errors.New("cycle has no validated supervisor decision/reference")
	}
	decision := result.Supervisor.Decision
	reference := result.Supervisor.DecisionReference
	if err := decision.Validate(); err != nil {
		return err
	}
	if err := reference.Validate(); err != nil {
		return err
	}
	if decision.TaskID != task.ID || decision.Action != autonomous.ActionPlan || decision.WorkerProfile != autonomous.WorkerProfilePlanner || reference.TaskID != task.ID || reference.Action != decision.Action || reference.WorkerProfile != decision.WorkerProfile || reference.DecisionID != result.Route.DecisionID {
		return errors.New("supervisor decision/reference does not match the planner route")
	}
	if result.Supervisor.RunID == "" || result.Worker.RunID == "" || result.Supervisor.RunID == result.Worker.RunID || reference.RunID != result.Supervisor.RunID {
		return errors.New("supervisor and worker run identities are missing, equal, or inconsistent")
	}
	if !result.Worker.Started || result.Worker.Run.ID != result.Worker.RunID || result.Worker.Run.TaskID != task.ID || result.Worker.Run.Status != "completed" {
		return errors.New("planner worker did not start and complete as valid evidence")
	}
	if result.Worker.Action != autonomous.ActionPlan || result.Worker.Profile.Name != string(autonomous.WorkerProfilePlanner) || result.Worker.Profile.Path != filepath.Join(".agent", "profiles", "planner.md") {
		return errors.New("planner worker action/profile evidence is inconsistent")
	}
	if err := result.Worker.Invocation.Validate(); err != nil {
		return fmt.Errorf("planner invocation: %w", err)
	}
	if result.Worker.Artifacts.OutputSchema == nil {
		return errors.New("planner-only output schema artifact is missing")
	}
	schemaRaw, _, err := readCycleArtifact(root, *result.Worker.Artifacts.OutputSchema)
	if err != nil {
		return fmt.Errorf("planner output schema artifact: %w", err)
	}
	wantSchema, err := autonomousplanning.PlanningOutputSchema()
	if err != nil || !bytes.Equal(schemaRaw, wantSchema) {
		return errors.New("planner output schema artifact does not match the canonical AW-11 schema")
	}
	schemaAbs, err := pathguard.Resolve(root, filepath.FromSlash(result.Worker.Artifacts.OutputSchema.Path))
	if err != nil || !containsPair(result.Worker.Invocation.Argv, "--output-schema", schemaAbs) {
		return errors.New("planner invocation did not use the exact planner-only output schema")
	}
	if result.Worker.Codex.Err != nil || result.Worker.Codex.TimedOut || result.Worker.Codex.ExitCode != 0 || result.Worker.Codex.ArtifactError != nil || len(result.Worker.RawOutput) == 0 {
		return errors.New("planner Codex execution did not complete with exact output")
	}
	if result.Source.Admission == nil || result.Source.WorkerAfter == nil || result.Source.WorkerDifference.Changed || len(result.Source.ChangedFiles) != 0 {
		return errors.New("planner read-only source evidence is missing or records mutation")
	}
	if err := result.Source.Admission.Validate(); err != nil {
		return fmt.Errorf("planner admission source: %w", err)
	}
	if err := result.Source.WorkerAfter.Validate(); err != nil {
		return fmt.Errorf("planner final source: %w", err)
	}
	if result.Source.AdmissionRevision == "" || result.Source.WorkerRevision != result.Source.AdmissionRevision || result.Route.SourceRevision != result.Source.AdmissionRevision {
		return errors.New("planner admitted/worker/route source identities differ")
	}
	if result.Worker.Verification.OccurrenceID != "" || result.Worker.Verification.SourceRevision != "" || result.Worker.Verification.Policy != nil || len(result.Worker.Verification.Result.Commands) != 0 {
		return errors.New("plan route synthesized verification evidence")
	}
	if result.Worker.Commit.Status != commit.Status("") || result.Worker.Commit.CommitSHA != "" {
		return errors.New("plan route synthesized commit evidence")
	}
	if result.DossierManifest.TaskID != task.ID || result.DossierManifest.SchemaVersion != autonomous.DossierManifestSchemaVersion || result.DossierManifest.DossierSHA256 == "" || result.DossierManifest.DossierByteSize <= 0 {
		return errors.New("cycle dossier identity is missing or malformed")
	}
	if result.Supervisor.Dossier.TaskID != task.ID || result.Supervisor.Dossier.SchemaVersion != result.DossierManifest.SchemaVersion || result.Supervisor.Dossier.SHA256 != result.DossierManifest.DossierSHA256 || result.Supervisor.Dossier.ByteSize != result.DossierManifest.DossierByteSize {
		return errors.New("supervisor did not consume the exact cycle dossier")
	}
	if _, _, err := readSupervisorArtifact(root, result.Supervisor.Artifacts.Decision); err != nil {
		return fmt.Errorf("supervisor decision artifact: %w", err)
	}
	profileRaw, err := readPathNoSymlinks(root, filepath.ToSlash(result.Worker.Profile.Path))
	if err != nil {
		return fmt.Errorf("planner profile artifact: %w", err)
	}
	if hashBytes(profileRaw) != result.Worker.Profile.SHA256 || len(profileRaw) != result.Worker.Profile.ByteSize {
		return errors.New("planner profile artifact hash or size mismatch")
	}
	return nil
}

func validateOutputProvenance(cycle autonomouscycle.Result, output autonomousplanning.PlanningOutput) error {
	if output.TaskID != cycle.TaskID || output.Provenance.Action != autonomous.ActionPlan || output.Provenance.WorkerProfile != autonomous.WorkerProfilePlanner || output.Provenance.WorkerRunID != cycle.Worker.RunID {
		return errors.New("planning output route/task/worker identity mismatch")
	}
	if !reflect.DeepEqual(output.Provenance.Decision, *cycle.Supervisor.DecisionReference) {
		return errors.New("planning output decision reference does not exactly match supervisor evidence")
	}
	wantDossier := autonomousplanning.DossierIdentity{SchemaVersion: cycle.DossierManifest.SchemaVersion, TaskID: cycle.TaskID, SHA256: cycle.DossierManifest.DossierSHA256, ByteSize: cycle.DossierManifest.DossierByteSize}
	if output.Provenance.Dossier != wantDossier {
		return errors.New("planning output dossier identity mismatch")
	}
	wantProfile := autonomousplanning.ProfileIdentity{Name: autonomous.WorkerProfilePlanner, Path: filepath.ToSlash(cycle.Worker.Profile.Path), SHA256: cycle.Worker.Profile.SHA256, ByteSize: cycle.Worker.Profile.ByteSize}
	if output.Provenance.Profile != wantProfile {
		return errors.New("planning output profile identity mismatch")
	}
	if output.Provenance.RawOutputPath != cycle.Worker.Artifacts.Output.Path || output.Provenance.SourceRevision != cycle.Source.AdmissionRevision {
		return errors.New("planning output artifact or source identity mismatch")
	}
	return nil
}

func validateDossierStateAndTask(task taskfile.Task, state autonomous.ExecutionState, manifest autonomous.TaskDossierManifest) error {
	stateRaw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	foundTask := false
	foundState := false
	for _, source := range manifest.Sources {
		switch source.Kind {
		case autonomous.DossierSourceKindTaskSpec:
			if source.Path == task.SourcePath && source.SHA256 == task.SourceSHA256() && source.ByteSize == task.SourceByteSize() {
				foundTask = true
			}
		case autonomous.DossierSourceKindExecutionState:
			if source.SHA256 == hashBytes(stateRaw) && source.ByteSize == len(stateRaw) {
				foundState = true
			}
		}
	}
	if !foundTask || !foundState {
		return fmt.Errorf("dossier sources do not contain exact task/state identities (task=%t state=%t)", foundTask, foundState)
	}
	return nil
}

func applicationSHA256(cfg Config, raw, canonical autonomousstate.ArtifactIdentity) (string, error) {
	initialHash := ""
	if cfg.InitialState != nil {
		initialRaw, err := autonomousstate.MarshalState(*cfg.InitialState)
		if err != nil {
			return "", err
		}
		initialHash = hashBytes(initialRaw)
	}
	projection := struct {
		TaskID         string
		OperationID    string
		Expected       autonomousstate.ExpectedState
		InitialHash    string
		SupervisorRun  string
		DecisionID     string
		WorkerRun      string
		DossierHash    string
		SourceRevision string
		Raw            autonomousstate.ArtifactIdentity
		Canonical      autonomousstate.ArtifactIdentity
	}{
		cfg.TaskID, cfg.OperationID, cfg.Expected, initialHash,
		cfg.Cycle.Supervisor.RunID, cfg.Cycle.Supervisor.DecisionReference.DecisionID,
		cfg.Cycle.Worker.RunID, cfg.Cycle.DossierManifest.DossierSHA256,
		cfg.Cycle.Source.AdmissionRevision, raw, canonical,
	}
	rawProjection, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}
	return hashBytes(rawProjection), nil
}

func committedResult(result Result, committed autonomousstate.CommitResult, record autonomousstate.PlanningHistoryRecord) Result {
	result.Disposition = DispositionUpdated
	if committed.Disposition == autonomousstate.CommitCreated {
		result.Disposition = DispositionCreated
	} else if committed.Disposition == autonomousstate.CommitReplayed {
		result.Disposition = DispositionReplayed
	}
	result.Previous = committed.Previous
	result.Current = committed.Current
	result.PreviousPlan = record.PreviousPlanIdentity
	result.CurrentPlan = record.ResultingPlanIdentity
	result.Acceptance = record.Acceptance
	result.History = committed.History
	result.State = committed.Current.State
	result.Planner = plannerIdentity(record)
	return result
}

func plannerIdentity(record autonomousstate.PlanningHistoryRecord) PlannerIdentity {
	return PlannerIdentity{
		SupervisorRunID: record.Decision.RunID, DecisionID: record.Decision.DecisionID,
		WorkerRunID: record.WorkerRunID, Profile: record.Profile,
		RawOutput: record.RawOutput, CanonicalOutput: record.CanonicalOutput,
		Dossier: record.Dossier, SourceRevision: record.SourceRevision,
	}
}

func compareExpected(expected autonomousstate.ExpectedState, current autonomousstate.Snapshot, found bool) error {
	if expected.Exists && !found {
		return fmt.Errorf("%w: caller expected existing state", autonomousstate.ErrStateMissing)
	}
	if !expected.Exists && found {
		return fmt.Errorf("%w: caller expected absent state", autonomousstate.ErrStateExists)
	}
	if expected.Exists && (expected.SHA256 != current.SHA256 || expected.ByteSize != current.ByteSize) {
		return fmt.Errorf("%w: expected %s/%d, observed %s/%d", autonomousstate.ErrStaleWrite, expected.SHA256, expected.ByteSize, current.SHA256, current.ByteSize)
	}
	return nil
}

func readCycleArtifact(root string, artifact autonomouscycle.Artifact) ([]byte, autonomousstate.ArtifactIdentity, error) {
	if artifact.Path == "" || artifact.SHA256 == "" || artifact.ByteSize < 0 {
		return nil, autonomousstate.ArtifactIdentity{}, errors.New("artifact identity is incomplete")
	}
	raw, err := readPathNoSymlinks(root, artifact.Path)
	if err != nil {
		return nil, autonomousstate.ArtifactIdentity{}, err
	}
	identity := identityFor(artifact.Path, raw)
	if identity.SHA256 != artifact.SHA256 || identity.ByteSize != artifact.ByteSize {
		return nil, autonomousstate.ArtifactIdentity{}, errors.New("artifact hash or byte size mismatch")
	}
	return raw, identity, nil
}

func readSupervisorArtifact(root string, artifact supervisor.Artifact) ([]byte, autonomousstate.ArtifactIdentity, error) {
	raw, err := readPathNoSymlinks(root, artifact.Path)
	if err != nil {
		return nil, autonomousstate.ArtifactIdentity{}, err
	}
	identity := identityFor(artifact.Path, raw)
	if identity.SHA256 != artifact.SHA256 || identity.ByteSize != artifact.ByteSize {
		return nil, autonomousstate.ArtifactIdentity{}, errors.New("artifact hash or byte size mismatch")
	}
	return raw, identity, nil
}

func readPathNoSymlinks(root, rel string) ([]byte, error) {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	current := root
	for _, component := range strings.Split(filepath.Clean(filepath.FromSlash(rel)), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("artifact path component %q is a symbolic link", component)
		}
	}
	return os.ReadFile(abs)
}

func repositoryRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("repository root is required")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func stableIdentity(label, value string) error {
	if value == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s %q is empty or malformed", label, value)
	}
	return nil
}

func identityFor(path string, raw []byte) autonomousstate.ArtifactIdentity {
	return autonomousstate.ArtifactIdentity{Path: path, SHA256: hashBytes(raw), ByteSize: len(raw)}
}

func stateIdentity(path string, persisted bool, raw []byte) autonomousstate.StateIdentity {
	return autonomousstate.StateIdentity{Path: path, Persisted: persisted, SHA256: hashBytes(raw), ByteSize: len(raw)}
}

func hashBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}

func containsPair(values []string, key, value string) bool {
	for i := 0; i+1 < len(values); i++ {
		if values[i] == key && values[i+1] == value {
			return true
		}
	}
	return false
}

func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var cloned autonomous.ExecutionState
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return autonomous.ExecutionState{}, err
	}
	return cloned, nil
}

func clonePlan(plan *autonomous.TaskPlan) *autonomous.TaskPlan {
	if plan == nil {
		return nil
	}
	raw, _ := json.Marshal(plan)
	var cloned autonomous.TaskPlan
	_ = json.Unmarshal(raw, &cloned)
	return &cloned
}

func cloneAcceptance(criteria []autonomous.AcceptanceCriterion) []autonomous.AcceptanceCriterion {
	raw, _ := json.Marshal(criteria)
	var cloned []autonomous.AcceptanceCriterion
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func reject(result Result, stage string, err error) (Result, error) {
	result.Failure = &Failure{Stage: stage, Reason: err.Error()}
	return result, fmt.Errorf("apply planning result for task %q at %s: %w", result.TaskID, stage, err)
}
