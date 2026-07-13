// Package autonomousauditapply is the narrow AW-12 coordinator that admits a
// successful AW-10 auditor result and persists audit/finding transitions.
package autonomousauditapply

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
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomouscycle"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/commit"
	"revolvr/internal/pathguard"
	"revolvr/internal/taskfile"
)

type ApplyConfig struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	Cycle          autonomouscycle.Result
	Verification   autonomouspolicy.VerificationEvidence
	LatestMutation *autonomouspolicy.SourceMutation
	CreatedAt      time.Time
	Store          *autonomousstate.Store
}

type ResolutionConfig struct {
	RepositoryRoot string
	TaskID         string
	OperationID    string
	Expected       autonomousstate.ExpectedState
	AuditRevision  int64
	Request        autonomousaudit.ResolutionRequest
	CreatedAt      time.Time
	Store          *autonomousstate.Store
}

type Disposition string

const (
	DispositionUpdated  Disposition = "updated"
	DispositionReplayed Disposition = "replayed"
)

type FindingCounts struct{ Total, Blocking, NonBlocking int }
type ResolutionCounts struct{ Total, Open, Resolved, Waived, Superseded, Invalid int }
type Failure struct {
	Stage  string
	Reason string
}

type Result struct {
	TaskID          string
	OperationID     string
	Kind            autonomousstate.AuditTransitionKind
	Disposition     Disposition
	StatePath       string
	Previous        autonomousstate.StateIdentity
	Current         autonomousstate.Snapshot
	AuditRevision   int64
	Report          autonomous.AuditReport
	PolicyEvidence  autonomouspolicy.AuditEvidence
	RawOutput       autonomousstate.ArtifactIdentity
	CanonicalOutput autonomousstate.ArtifactIdentity
	History         autonomousstate.AuditHistorySnapshot
	Findings        FindingCounts
	Resolutions     ResolutionCounts
	NewFindingIDs   []string
	State           autonomous.ExecutionState
	Failure         *Failure
}

func ApplyAuditResult(ctx context.Context, cfg ApplyConfig) (Result, error) {
	result := Result{TaskID: cfg.TaskID, OperationID: cfg.OperationID, Kind: autonomousstate.AuditTransitionRecorded}
	root, task, store, err := prepare(cfg.RepositoryRoot, cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.CreatedAt, cfg.Store)
	if err != nil {
		return reject(result, "configuration", err)
	}
	result.StatePath = task.AutonomousStatePath
	if err := validateCycle(root, task, cfg); err != nil {
		return reject(result, "cycle_evidence", err)
	}
	raw, rawIdentity, err := readArtifact(root, cfg.Cycle.Worker.Artifacts.Output.Path, cfg.Cycle.Worker.Artifacts.Output.SHA256, cfg.Cycle.Worker.Artifacts.Output.ByteSize)
	if err != nil {
		return reject(result, "auditor_raw_output", err)
	}
	if !bytes.Equal(raw, cfg.Cycle.Worker.RawOutput) {
		return reject(result, "auditor_raw_output", errors.New("cycle raw output differs from exact artifact"))
	}
	output, err := autonomousaudit.ParseAuditOutput(raw)
	if err != nil {
		return reject(result, "auditor_output", err)
	}
	if err := validateOutput(cfg, output); err != nil {
		return reject(result, "auditor_output_identity", err)
	}
	canonical, err := autonomousaudit.MarshalAuditOutput(output)
	if err != nil {
		return reject(result, "auditor_output", err)
	}
	canonicalPath := filepath.ToSlash(filepath.Join(".revolvr", "runs", cfg.Cycle.Worker.RunID, "auditor-output.canonical.json"))
	canonicalIdentity := artifactIdentity(canonicalPath, canonical)
	application := applicationHash(struct {
		TaskID, OperationID                                   string
		Expected                                              autonomousstate.ExpectedState
		SupervisorRun, DecisionID, WorkerRun, Dossier, Source string
		Raw, Canonical                                        autonomousstate.ArtifactIdentity
	}{cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.Cycle.Supervisor.RunID, cfg.Cycle.Supervisor.DecisionReference.DecisionID, cfg.Cycle.Worker.RunID, cfg.Cycle.DossierManifest.DossierSHA256, cfg.Cycle.Source.AdmissionRevision, rawIdentity, canonicalIdentity})
	if replay, found, err := store.ReplayAudit(ctx, cfg.TaskID, cfg.OperationID, application); err != nil {
		return reject(result, replayStage(err), err)
	} else if found {
		return committed(result, replay), nil
	}
	current, found, err := store.Load(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "state_load", err)
	}
	if !found {
		return reject(result, "state_load", autonomousstate.ErrStateMissing)
	}
	if err := compareExpected(cfg.Expected, current, found); err != nil {
		return reject(result, "state_compare", err)
	}
	if err := validateDossier(task, current.State, cfg.Cycle.DossierManifest); err != nil {
		return reject(result, "dossier_identity", err)
	}
	history, err := store.LoadAuditHistory(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "history_load", err)
	}
	committedHistory, err := store.LoadCommittedAuditHistory(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "history_load", err)
	}
	priorReports := make([]autonomous.AuditReport, 0, len(committedHistory))
	for _, h := range committedHistory {
		if h.Record.Kind == autonomousstate.AuditTransitionRecorded {
			priorReports = append(priorReports, h.Record.Report)
		}
	}
	maxSequence, maxRevision := int64(0), int64(0)
	sequence, auditRevision := int64(0), int64(0)
	for _, h := range history {
		if h.Record.Sequence > maxSequence {
			maxSequence = h.Record.Sequence
		}
		if h.Record.AuditRevision > maxRevision {
			maxRevision = h.Record.AuditRevision
		}
		if h.Record.OperationID == cfg.OperationID {
			sequence, auditRevision = h.Record.Sequence, h.Record.AuditRevision
		}
	}
	if sequence == 0 {
		sequence, auditRevision = maxSequence+1, maxRevision+1
	}
	change, err := autonomousaudit.ApplyReport(current.State, output, *cfg.Cycle.Supervisor.Decision, priorReports)
	if err != nil {
		return reject(result, "finding_identity", err)
	}
	previousBytes, _ := autonomousstate.MarshalState(current.State)
	nextBytes, _ := autonomousstate.MarshalState(change.State)
	decisionArtifactRaw, decisionArtifact, err := readArtifact(root, cfg.Cycle.Supervisor.Artifacts.Decision.Path, cfg.Cycle.Supervisor.Artifacts.Decision.SHA256, cfg.Cycle.Supervisor.Artifacts.Decision.ByteSize)
	if err != nil || len(decisionArtifactRaw) == 0 {
		return reject(result, "supervisor_artifact", err)
	}
	policyEvidence := autonomouspolicy.AuditEvidence{Report: output.Report, RunID: cfg.Cycle.Worker.RunID, AuditorProfile: autonomous.WorkerProfileAuditor, SourceRevision: output.Provenance.SourceRevision, VerificationRunID: cfg.Verification.Summary.RunID, VerificationOccurrenceID: cfg.Verification.Summary.OccurrenceID}
	record := autonomousstate.AuditHistoryRecord{
		SchemaVersion: autonomousstate.AuditHistorySchemaVersion, TaskID: cfg.TaskID, Sequence: sequence, AuditRevision: auditRevision, OperationID: cfg.OperationID, ApplicationSHA256: application, Kind: autonomousstate.AuditTransitionRecorded, CreatedAt: cfg.CreatedAt.UTC(),
		Decision: *cfg.Cycle.Supervisor.DecisionReference, SupervisorDecision: decisionArtifact, WorkerRunID: cfg.Cycle.Worker.RunID, Profile: output.Provenance.Profile, Dossier: output.Provenance.Dossier, SourceRevision: output.Provenance.SourceRevision, Verification: cfg.Verification, LatestSourceMutation: autonomousaudit.SourceMutationFromPolicy(cfg.LatestMutation),
		TaskSource: artifactIdentity(task.SourcePath, task.SourceBytes), RawOutput: rawIdentity, CanonicalOutput: canonicalIdentity, Report: output.Report, PolicyEvidence: policyEvidence,
		PreviousState: stateIdentity(task.AutonomousStatePath, true, previousBytes), ResultingState: stateIdentity(task.AutonomousStatePath, true, nextBytes), PreviousResolutions: cloneResolutions(current.State.FindingResolutions), ResultingResolutions: cloneResolutions(change.State.FindingResolutions), NewFindingIDs: append([]string(nil), change.NewFindingIDs...),
	}
	persisted, err := store.CommitAudit(ctx, autonomousstate.AuditCommitRequest{TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: current.State, NextState: change.State, History: record, CanonicalOutput: canonical})
	if err != nil {
		return reject(result, persistenceStage(err), err)
	}
	result = committed(result, persisted)
	result.RawOutput = rawIdentity
	return result, nil
}

func ApplyFindingResolution(ctx context.Context, cfg ResolutionConfig) (Result, error) {
	result := Result{TaskID: cfg.TaskID, OperationID: cfg.OperationID}
	root, task, store, err := prepare(cfg.RepositoryRoot, cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.CreatedAt, cfg.Store)
	if err != nil {
		return reject(result, "configuration", err)
	}
	result.StatePath = task.AutonomousStatePath
	currentAudit, found, err := store.LoadCurrentAudit(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "audit_reopen", err)
	}
	if !found {
		return reject(result, "audit_reopen", errors.New("no current committed audit evidence matches canonical state"))
	}
	if cfg.AuditRevision != currentAudit.Revision {
		return reject(result, "audit_reopen", fmt.Errorf("audit revision %d is stale (current %d)", cfg.AuditRevision, currentAudit.Revision))
	}
	application := applicationHash(struct {
		TaskID, OperationID string
		Expected            autonomousstate.ExpectedState
		AuditRevision       int64
		Request             autonomousaudit.ResolutionRequest
	}{cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.AuditRevision, cfg.Request})
	if replay, found, err := store.ReplayAudit(ctx, cfg.TaskID, cfg.OperationID, application); err != nil {
		return reject(result, replayStage(err), err)
	} else if found {
		return committed(result, replay), nil
	}
	if err := compareExpected(cfg.Expected, currentAudit.State, true); err != nil {
		return reject(result, "state_compare", err)
	}
	history, err := store.LoadAuditHistory(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "history_load", err)
	}
	maxSequence := int64(0)
	sequence := int64(0)
	committedHistory, err := store.LoadCommittedAuditHistory(ctx, cfg.TaskID)
	if err != nil {
		return reject(result, "history_load", err)
	}
	findings := []autonomous.AuditFinding{}
	for _, h := range committedHistory {
		findings = append(findings, h.Record.Report.Findings...)
	}
	for _, h := range history {
		if h.Record.Sequence > maxSequence {
			maxSequence = h.Record.Sequence
		}
		if h.Record.OperationID == cfg.OperationID {
			sequence = h.Record.Sequence
		}
	}
	if sequence == 0 {
		sequence = maxSequence + 1
	}
	next, resolution, err := autonomousaudit.ApplyResolution(currentAudit.State.State, cfg.Request, findings)
	if err != nil {
		return reject(result, "resolution_transition", err)
	}
	kind := kindForResolution(resolution.Status)
	result.Kind = kind
	canonicalRaw, canonicalIdentityRead, err := readArtifact(root, currentAudit.CanonicalOutput.Path, currentAudit.CanonicalOutput.SHA256, currentAudit.CanonicalOutput.ByteSize)
	if err != nil {
		return reject(result, "audit_reopen", err)
	}
	previousBytes, _ := autonomousstate.MarshalState(currentAudit.State.State)
	nextBytes, _ := autonomousstate.MarshalState(next)
	prior := currentAudit.State.State.FindingResolutions[resolutionIndex(currentAudit.State.State, cfg.Request.FindingID)]
	record := currentAudit.History.Record
	record.Sequence = sequence
	record.OperationID = cfg.OperationID
	record.ApplicationSHA256 = application
	record.Kind = kind
	record.CreatedAt = cfg.CreatedAt.UTC()
	record.PreviousState = stateIdentity(task.AutonomousStatePath, true, previousBytes)
	record.ResultingState = stateIdentity(task.AutonomousStatePath, true, nextBytes)
	record.PreviousResolutions = cloneResolutions(currentAudit.State.State.FindingResolutions)
	record.ResultingResolutions = cloneResolutions(next.FindingResolutions)
	record.NewFindingIDs = nil
	record.Resolution = &autonomousstate.ResolutionTransition{FindingID: cfg.Request.FindingID, Previous: prior, Resulting: resolution, Authority: cfg.Request.DecisionReference}
	record.CanonicalOutput = canonicalIdentityRead
	persisted, err := store.CommitAudit(ctx, autonomousstate.AuditCommitRequest{TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: currentAudit.State.State, NextState: next, History: record, CanonicalOutput: canonicalRaw})
	if err != nil {
		return reject(result, persistenceStage(err), err)
	}
	return committed(result, persisted), nil
}

func prepare(root, taskID, operationID string, expected autonomousstate.ExpectedState, created time.Time, store *autonomousstate.Store) (string, taskfile.Task, *autonomousstate.Store, error) {
	abs, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil || strings.TrimSpace(root) == "" {
		return "", taskfile.Task{}, nil, errors.New("repository root is required")
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(operationID) == "" || created.IsZero() {
		return "", taskfile.Task{}, nil, errors.New("task_id, operation_id, and created_at are required")
	}
	if err := expected.Validate(); err != nil || !expected.Exists {
		return "", taskfile.Task{}, nil, errors.New("exact existing expected state is required")
	}
	task, found, err := taskfile.FindByID(abs, taskID)
	if err != nil || !found {
		return "", taskfile.Task{}, nil, errors.Join(err, autonomousstate.ErrTaskMissing)
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 || task.Status != taskfile.StatusPending {
		return "", taskfile.Task{}, nil, errors.New("canonical task is not pending autonomous-v1")
	}
	if store == nil {
		store, err = autonomousstate.New(autonomousstate.Config{RepositoryRoot: abs})
	}
	return abs, task, store, err
}

func validateCycle(root string, task taskfile.Task, cfg ApplyConfig) error {
	r := cfg.Cycle
	if r.Failure != nil || r.Outcome != autonomouscycle.OutcomeReadOnlyCompleted {
		return errors.New("auditor cycle did not complete read-only")
	}
	if r.TaskID != task.ID || r.Route == nil || r.Route.Kind != autonomouspolicy.RouteKindWorker || r.Route.Action != autonomous.ActionAudit || r.Route.WorkerProfile != autonomous.WorkerProfileAuditor {
		return errors.New("cycle route is not audit -> auditor")
	}
	if r.Supervisor.Decision == nil || r.Supervisor.DecisionReference == nil || r.Supervisor.RunID == "" || r.Worker.RunID == "" || r.Supervisor.RunID == r.Worker.RunID {
		return errors.New("supervisor/auditor identities are missing or equal")
	}
	if err := r.Supervisor.Decision.Validate(); err != nil {
		return err
	}
	if err := r.Supervisor.DecisionReference.Validate(); err != nil {
		return err
	}
	if r.Supervisor.Decision.TaskID != task.ID || r.Supervisor.Decision.Action != autonomous.ActionAudit || r.Supervisor.Decision.WorkerProfile != autonomous.WorkerProfileAuditor || r.Supervisor.DecisionReference.TaskID != task.ID || r.Supervisor.DecisionReference.Action != autonomous.ActionAudit || r.Supervisor.DecisionReference.WorkerProfile != autonomous.WorkerProfileAuditor || r.Supervisor.DecisionReference.RunID != r.Supervisor.RunID || r.Supervisor.DecisionReference.DecisionID != r.Route.DecisionID {
		return errors.New("supervisor decision/reference does not exactly match the audit route")
	}
	if r.Worker.RunID == cfg.Verification.Summary.RunID || r.Supervisor.RunID == cfg.Verification.Summary.RunID {
		return errors.New("supervisor, auditor, and verification runs must be distinct")
	}
	if cfg.LatestMutation != nil && (r.Worker.RunID == cfg.LatestMutation.RunID || r.Supervisor.RunID == cfg.LatestMutation.RunID) {
		return errors.New("auditor/supervisor must differ from latest source-mutating run")
	}
	if !r.Worker.Started || r.Worker.Run.ID != r.Worker.RunID || r.Worker.Run.TaskID != task.ID || r.Worker.Run.Status != "completed" || r.Worker.Action != autonomous.ActionAudit || r.Worker.Profile.Name != "auditor" || filepath.ToSlash(r.Worker.Profile.Path) != ".agent/profiles/auditor.md" {
		return errors.New("auditor worker did not start and ledger-complete successfully")
	}
	if err := r.Worker.Invocation.Validate(); err != nil {
		return err
	}
	if r.Worker.Artifacts.OutputSchema == nil {
		return errors.New("auditor output schema is missing")
	}
	if r.Worker.Artifacts.Output.Path != filepath.ToSlash(filepath.Join(".revolvr", "runs", r.Worker.RunID, "auditor-output.raw.json")) {
		return errors.New("auditor raw output path is not canonical for the worker run")
	}
	schema, _, err := readArtifact(root, r.Worker.Artifacts.OutputSchema.Path, r.Worker.Artifacts.OutputSchema.SHA256, r.Worker.Artifacts.OutputSchema.ByteSize)
	if err != nil {
		return err
	}
	want, _ := autonomousaudit.AuditOutputSchema()
	if !bytes.Equal(schema, want) {
		return errors.New("auditor schema artifact is not canonical")
	}
	schemaAbs, _ := pathguard.Resolve(root, filepath.FromSlash(r.Worker.Artifacts.OutputSchema.Path))
	if !containsPair(r.Worker.Invocation.Argv, "--output-schema", schemaAbs) {
		return errors.New("auditor invocation did not use exact schema")
	}
	if r.Worker.Codex.Err != nil || r.Worker.Codex.TimedOut || r.Worker.Codex.ExitCode != 0 || r.Worker.Codex.ArtifactError != nil {
		return errors.New("auditor Codex execution failed")
	}
	if r.Source.Admission == nil || r.Source.WorkerAfter == nil || r.Source.WorkerDifference.Changed || len(r.Source.ChangedFiles) != 0 {
		return errors.New("auditor mutated source")
	}
	if err := r.Source.Admission.Validate(); err != nil {
		return fmt.Errorf("auditor admission source: %w", err)
	}
	if err := r.Source.WorkerAfter.Validate(); err != nil {
		return fmt.Errorf("auditor final source: %w", err)
	}
	if r.Source.AdmissionRevision != r.Source.WorkerRevision || r.Source.AdmissionRevision != r.Route.SourceRevision {
		return errors.New("auditor source identities differ")
	}
	if r.Worker.Verification.OccurrenceID != "" || r.Worker.Verification.Policy != nil || len(r.Worker.Verification.Result.Commands) != 0 {
		return errors.New("auditor synthesized verification")
	}
	if r.Worker.Commit.Status != commit.Status("") || r.Worker.Commit.CommitSHA != "" {
		return errors.New("auditor synthesized commit")
	}
	if r.DossierManifest.TaskID != task.ID || r.DossierManifest.SchemaVersion != autonomous.DossierManifestSchemaVersion || r.DossierManifest.DossierSHA256 == "" || r.DossierManifest.DossierByteSize <= 0 {
		return errors.New("cycle dossier identity is missing or malformed")
	}
	if r.Supervisor.Dossier.TaskID != task.ID || r.Supervisor.Dossier.SchemaVersion != r.DossierManifest.SchemaVersion || r.Supervisor.Dossier.SHA256 != r.DossierManifest.DossierSHA256 || r.Supervisor.Dossier.ByteSize != r.DossierManifest.DossierByteSize {
		return errors.New("supervisor did not consume the exact audit dossier")
	}
	if err := autonomouspolicy.ValidateEvidence(task.ID, autonomouspolicy.SourceEvidence{Revision: r.Source.AdmissionRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: cfg.LatestMutation}, &cfg.Verification, nil); err != nil {
		return err
	}
	if cfg.Verification.Summary.Status != autonomous.VerificationStatusPassed || cfg.Verification.SourceRevision != r.Source.AdmissionRevision {
		return errors.New("verification is failed or stale")
	}
	if cfg.Verification.Tiered != nil && !cfg.Verification.Tiered.FinalSatisfied {
		return errors.New("tiered verification does not satisfy the final gate")
	}
	profileRaw, _, err := readArtifact(root, filepath.ToSlash(r.Worker.Profile.Path), r.Worker.Profile.SHA256, r.Worker.Profile.ByteSize)
	if err != nil || len(profileRaw) == 0 {
		return errors.New("auditor profile artifact mismatch")
	}
	return nil
}

func validateOutput(cfg ApplyConfig, o autonomousaudit.AuditOutput) error {
	c := cfg.Cycle
	if o.TaskID != cfg.TaskID || o.Provenance.WorkerRunID != c.Worker.RunID || !reflect.DeepEqual(o.Provenance.Decision, *c.Supervisor.DecisionReference) {
		return errors.New("task/worker/decision provenance mismatch")
	}
	wantD := autonomousaudit.DossierIdentity{SchemaVersion: c.DossierManifest.SchemaVersion, TaskID: cfg.TaskID, SHA256: c.DossierManifest.DossierSHA256, ByteSize: c.DossierManifest.DossierByteSize}
	if o.Provenance.Dossier != wantD {
		return errors.New("dossier provenance mismatch")
	}
	wantP := autonomousaudit.ProfileIdentity{Name: autonomous.WorkerProfileAuditor, Path: filepath.ToSlash(c.Worker.Profile.Path), SHA256: c.Worker.Profile.SHA256, ByteSize: c.Worker.Profile.ByteSize}
	if o.Provenance.Profile != wantP {
		return errors.New("profile provenance mismatch")
	}
	if o.Provenance.RawOutputPath != c.Worker.Artifacts.Output.Path || o.Provenance.SourceRevision != c.Source.AdmissionRevision || !reflect.DeepEqual(o.Provenance.Verification, cfg.Verification) || !reflect.DeepEqual(o.Provenance.LatestSourceMutation, autonomousaudit.SourceMutationFromPolicy(cfg.LatestMutation)) {
		return errors.New("raw/source/verification/mutation provenance mismatch")
	}
	return nil
}

func validateDossier(task taskfile.Task, state autonomous.ExecutionState, m autonomous.TaskDossierManifest) error {
	stateRaw, _ := json.Marshal(state)
	taskOK, stateOK := false, false
	for _, s := range m.Sources {
		if s.Kind == autonomous.DossierSourceKindTaskSpec && s.Path == task.SourcePath && s.SHA256 == task.SourceSHA256() && s.ByteSize == task.SourceByteSize() {
			taskOK = true
		}
		if s.Kind == autonomous.DossierSourceKindExecutionState && s.SHA256 == hashBytes(stateRaw) && s.ByteSize == len(stateRaw) {
			stateOK = true
		}
	}
	if !taskOK || !stateOK {
		return errors.New("dossier lacks exact task/current-state identities")
	}
	return nil
}
func readArtifact(root, path, sha string, size int) ([]byte, autonomousstate.ArtifactIdentity, error) {
	abs, err := pathguard.Resolve(root, filepath.FromSlash(path))
	if err != nil {
		return nil, autonomousstate.ArtifactIdentity{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, autonomousstate.ArtifactIdentity{}, errors.New("artifact is missing or unsafe")
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, autonomousstate.ArtifactIdentity{}, err
	}
	identity := artifactIdentity(path, raw)
	if identity.SHA256 != sha || identity.ByteSize != size {
		return nil, identity, errors.New("artifact hash or size mismatch")
	}
	return raw, identity, nil
}
func artifactIdentity(path string, raw []byte) autonomousstate.ArtifactIdentity {
	return autonomousstate.ArtifactIdentity{Path: path, SHA256: hashBytes(raw), ByteSize: len(raw)}
}
func stateIdentity(path string, persisted bool, raw []byte) autonomousstate.StateIdentity {
	return autonomousstate.StateIdentity{Path: path, Persisted: persisted, SHA256: hashBytes(raw), ByteSize: len(raw)}
}
func hashBytes(raw []byte) string  { sum := sha256.Sum256(raw); return fmt.Sprintf("%x", sum) }
func applicationHash(v any) string { raw, _ := json.Marshal(v); return hashBytes(raw) }
func compareExpected(e autonomousstate.ExpectedState, s autonomousstate.Snapshot, found bool) error {
	if !found {
		return autonomousstate.ErrStateMissing
	}
	if e.SHA256 != s.SHA256 || e.ByteSize != s.ByteSize {
		return fmt.Errorf("%w: expected %s/%d observed %s/%d", autonomousstate.ErrStaleWrite, e.SHA256, e.ByteSize, s.SHA256, s.ByteSize)
	}
	return nil
}
func containsPair(argv []string, key, value string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == key && argv[i+1] == value {
			return true
		}
	}
	return false
}
func cloneResolutions(v []autonomous.FindingResolution) []autonomous.FindingResolution {
	raw, _ := json.Marshal(v)
	var out []autonomous.FindingResolution
	_ = json.Unmarshal(raw, &out)
	return out
}
func resolutionIndex(state autonomous.ExecutionState, id string) int {
	for i, v := range state.FindingResolutions {
		if v.FindingID == id {
			return i
		}
	}
	return -1
}
func kindForResolution(s autonomous.FindingResolutionStatus) autonomousstate.AuditTransitionKind {
	switch s {
	case autonomous.FindingResolutionStatusResolved:
		return autonomousstate.AuditTransitionFindingResolved
	case autonomous.FindingResolutionStatusWaived:
		return autonomousstate.AuditTransitionFindingWaived
	case autonomous.FindingResolutionStatusSuperseded:
		return autonomousstate.AuditTransitionFindingSuperseded
	default:
		return autonomousstate.AuditTransitionFindingInvalid
	}
}
func counts(s autonomous.ExecutionState) ResolutionCounts {
	c := ResolutionCounts{Total: len(s.FindingResolutions)}
	for _, v := range s.FindingResolutions {
		switch v.Status {
		case autonomous.FindingResolutionStatusOpen:
			c.Open++
		case autonomous.FindingResolutionStatusResolved:
			c.Resolved++
		case autonomous.FindingResolutionStatusWaived:
			c.Waived++
		case autonomous.FindingResolutionStatusSuperseded:
			c.Superseded++
		case autonomous.FindingResolutionStatusInvalid:
			c.Invalid++
		}
	}
	return c
}
func findingCounts(r autonomous.AuditReport) FindingCounts {
	c := FindingCounts{Total: len(r.Findings)}
	for _, f := range r.Findings {
		if f.Significance == autonomous.FindingSignificanceBlocking {
			c.Blocking++
		} else {
			c.NonBlocking++
		}
	}
	return c
}
func committed(r Result, c autonomousstate.AuditCommitResult) Result {
	r.Disposition = DispositionUpdated
	if c.Disposition == autonomousstate.CommitReplayed {
		r.Disposition = DispositionReplayed
	}
	r.Previous = c.Previous
	r.Current = c.Current
	r.History = c.History
	r.AuditRevision = c.History.Record.AuditRevision
	r.Report = c.History.Record.Report
	r.PolicyEvidence = c.History.Record.PolicyEvidence
	r.CanonicalOutput = c.History.Record.CanonicalOutput
	r.RawOutput = c.History.Record.RawOutput
	r.NewFindingIDs = append([]string(nil), c.History.Record.NewFindingIDs...)
	r.State = c.Current.State
	r.Findings = findingCounts(r.Report)
	r.Resolutions = counts(r.State)
	return r
}
func persistenceStage(err error) string {
	if strings.Contains(err.Error(), "history") {
		return "history_persistence"
	}
	if errors.Is(err, autonomousstate.ErrStaleWrite) {
		return "state_compare"
	}
	return "state_persistence"
}
func replayStage(err error) string {
	if errors.Is(err, autonomousstate.ErrOperationConflict) {
		return "operation_conflict"
	}
	return "history_replay"
}
func reject(r Result, stage string, err error) (Result, error) {
	if err == nil {
		err = errors.New("unknown failure")
	}
	r.Failure = &Failure{Stage: stage, Reason: err.Error()}
	return r, fmt.Errorf("apply autonomous audit %s: %w", stage, err)
}
