// Package autonomousfinalization owns the bounded, idempotent transaction
// that turns one already-authorized complete decision into terminal evidence.
package autonomousfinalization

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomouspolicy"
	"revolvr/internal/autonomoussafety"
	"revolvr/internal/autonomousstate"
	"revolvr/internal/gitoid"
)

const (
	FrozenEvidenceSchemaVersion = "autonomous-completion-frozen-evidence-v1"
	ManifestSchemaVersion       = "autonomous-completion-capsule-manifest-v1"
)

type TaskSource struct {
	TaskID            string `json:"task_id"`
	Title             string `json:"title"`
	Path              string `json:"path"`
	SHA256            string `json:"sha256"`
	ByteSize          int    `json:"byte_size"`
	Workflow          string `json:"workflow"`
	StatePath         string `json:"state_path"`
	CompletedSHA256   string `json:"completed_sha256"`
	CompletedByteSize int    `json:"completed_byte_size"`
}

type CommitEvidence struct {
	Sequence   int64             `json:"sequence"`
	SHA        string            `json:"sha"`
	ParentSHA  string            `json:"parent_sha,omitempty"`
	RunID      string            `json:"run_id"`
	Action     autonomous.Action `json:"action"`
	Outcome    string            `json:"outcome"`
	Reconciled bool              `json:"reconciled,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

type RunEvidence struct {
	Sequence    int64                        `json:"sequence"`
	RunID       string                       `json:"run_id"`
	Kind        string                       `json:"kind"`
	Outcome     string                       `json:"outcome"`
	Artifact    autonomous.EvidenceReference `json:"artifact"`
	StartedAt   time.Time                    `json:"started_at"`
	CompletedAt time.Time                    `json:"completed_at"`
}

type FrozenEvidence struct {
	SchemaVersion         string                                `json:"schema_version"`
	OperationID           string                                `json:"operation_id"`
	FinalizationRunID     string                                `json:"finalization_run_id"`
	Task                  TaskSource                            `json:"task"`
	State                 autonomous.ExecutionState             `json:"state"`
	StateIdentity         autonomousstate.StateIdentity         `json:"state_identity"`
	Decision              autonomous.SupervisorDecision         `json:"decision"`
	DecisionReference     autonomous.DecisionReference          `json:"decision_reference"`
	Route                 autonomouspolicy.Route                `json:"route"`
	Source                autonomouspolicy.SourceEvidence       `json:"source"`
	Verification          autonomouspolicy.VerificationEvidence `json:"verification"`
	Audit                 autonomouspolicy.AuditEvidence        `json:"audit"`
	Workspace             autonomous.TaskWorkspace              `json:"workspace"`
	SafetyPolicy          autonomoussafety.Policy               `json:"safety_policy"`
	SafetyPreflight       autonomoussafety.PreflightResult      `json:"safety_preflight"`
	EffectiveConfigSchema string                                `json:"effective_config_schema"`
	EffectiveConfigSHA256 string                                `json:"effective_config_sha256"`
	Commits               []CommitEvidence                      `json:"commits"`
	Runs                  []RunEvidence                         `json:"runs"`
	Provenance            []autonomous.EvidenceReference        `json:"provenance"`
	AdmittedAt            time.Time                             `json:"admitted_at"`
	TerminalAt            time.Time                             `json:"terminal_at"`
}

type SourceRecord struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	SHA256    string `json:"sha256"`
	ByteSize  int    `json:"byte_size"`
}

type Omission struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type Manifest struct {
	SchemaVersion  string                          `json:"schema_version"`
	TaskID         string                          `json:"task_id"`
	OperationID    string                          `json:"operation_id"`
	FrozenEvidence autonomous.FinalizationArtifact `json:"frozen_evidence"`
	Capsule        autonomous.FinalizationArtifact `json:"capsule"`
	Sources        []SourceRecord                  `json:"sources"`
	Omissions      []Omission                      `json:"omissions"`
}

func MarshalFrozen(e FrozenEvidence) ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

// DecodeFrozen strictly decodes canonical frozen completion evidence without
// performing any repository or runtime mutation.
func DecodeFrozen(raw []byte) (FrozenEvidence, error) {
	var evidence FrozenEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return evidence, fmt.Errorf("decode frozen evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return evidence, errors.New("decode frozen evidence: expected exactly one JSON value")
	}
	if err := evidence.Validate(); err != nil {
		return evidence, err
	}
	canonical, err := MarshalFrozen(evidence)
	if err != nil {
		return evidence, err
	}
	if !bytes.Equal(raw, canonical) {
		return evidence, errors.New("decode frozen evidence: bytes are not canonical deterministic JSON")
	}
	return evidence, nil
}

func MarshalManifest(m Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func (e FrozenEvidence) Validate() error {
	if e.SchemaVersion != FrozenEvidenceSchemaVersion || strings.TrimSpace(e.OperationID) == "" || strings.TrimSpace(e.FinalizationRunID) == "" {
		return errors.New("frozen evidence schema, operation, and run identities are required")
	}
	if e.Task.TaskID == "" || e.Task.TaskID != e.State.TaskID || e.Task.Title == "" || e.Task.Path == "" || !validHash(e.Task.SHA256) || e.Task.ByteSize <= 0 || !validHash(e.Task.CompletedSHA256) || e.Task.CompletedByteSize <= 0 || e.Task.Workflow != "autonomous-v1" {
		return errors.New("frozen evidence task source is malformed")
	}
	if err := e.State.Validate(); err != nil {
		return err
	}
	if e.State.Lifecycle != autonomous.LifecycleStateReady {
		return errors.New("frozen evidence admission state must be ready")
	}
	if err := e.StateIdentity.Validate(); err != nil || !e.StateIdentity.Persisted {
		return errors.New("frozen evidence state identity is malformed")
	}
	calculatedState, err := autonomousstate.StateIdentityFor(e.StateIdentity.Path, true, e.State)
	if err != nil || calculatedState != e.StateIdentity || e.Task.StatePath != e.StateIdentity.Path {
		return errors.New("frozen evidence state identity does not match exact canonical state bytes")
	}
	if e.State.Workspace == nil || !reflect.DeepEqual(*e.State.Workspace, e.Workspace) {
		return errors.New("frozen evidence workspace does not match canonical state")
	}
	if e.Verification.Tiered == nil {
		return errors.New("frozen evidence requires final-purpose tier verification")
	}
	if err := e.Verification.Tiered.Validate(); err != nil || e.Verification.Tiered.Purpose != "final" || !e.Verification.Tiered.FinalSatisfied || e.Verification.Tiered.OverallOutcome != "passed" {
		return errors.New("frozen evidence final-purpose verification gate is not ordinarily satisfied")
	}
	if err := e.Decision.Validate(); err != nil {
		return err
	}
	if err := e.DecisionReference.Validate(); err != nil {
		return err
	}
	if e.Decision.Action != autonomous.ActionComplete || e.Route.Kind != autonomouspolicy.RouteKindComplete || e.Route.TaskID != e.Task.TaskID || e.Route.DecisionID != e.DecisionReference.DecisionID || e.Route.SourceRevision != e.Source.Revision {
		return errors.New("frozen evidence does not contain one exact complete authorization")
	}
	if err := e.Workspace.Validate(); err != nil {
		return err
	}
	if e.Workspace.TaskID != e.Task.TaskID || e.Workspace.SourceRevision != e.Source.Revision || e.Workspace.HeadSHA != e.Workspace.Checkpoint.CommitSHA || e.Workspace.TreeSHA != e.Workspace.Checkpoint.TreeSHA || e.Workspace.SourceRevision != e.Workspace.Checkpoint.SourceRevision {
		return errors.New("frozen evidence workspace is not an exact clean checkpoint")
	}
	if e.Workspace.Status != autonomous.WorkspaceStatusReady && e.Workspace.Status != autonomous.WorkspaceStatusRestored {
		return errors.New("frozen evidence workspace is not ready or restored")
	}
	if err := e.SafetyPolicy.Validate(); err != nil {
		return err
	}
	if !reflect.DeepEqual(e.SafetyPolicy.Workspace, e.Workspace) {
		return errors.New("frozen evidence safety policy workspace is stale")
	}
	if !e.SafetyPreflight.Ready || e.SafetyPreflight.SchemaVersion != autonomoussafety.PreflightSchemaVersion || e.SafetyPreflight.TaskID != e.Task.TaskID || e.SafetyPreflight.WorkspaceID != e.Workspace.WorkspaceID || e.SafetyPreflight.SourceRevision != e.Source.Revision || e.SafetyPreflight.PolicySHA256 != e.SafetyPolicy.PolicySHA256 || e.SafetyPreflight.ConfigSHA256 != e.EffectiveConfigSHA256 || e.SafetyPolicy.ConfigSHA256 != e.EffectiveConfigSHA256 {
		return errors.New("frozen evidence safety preflight is stale, unsafe, or mismatched")
	}
	checks := map[string]bool{}
	for _, check := range e.SafetyPreflight.Checks {
		if strings.TrimSpace(check.Name) == "" || strings.TrimSpace(check.Detail) == "" || check.Status != autonomoussafety.CheckOK || checks[check.Name] {
			return errors.New("frozen evidence safety preflight contains a failed, malformed, or duplicate check")
		}
		checks[check.Name] = true
	}
	if e.Audit.RunID == e.DecisionReference.RunID || e.Audit.RunID == e.Verification.Summary.RunID {
		return errors.New("frozen evidence audit is not independent from completion supervision and verification")
	}
	if strings.TrimSpace(e.EffectiveConfigSchema) == "" || !validHash(e.EffectiveConfigSHA256) || e.AdmittedAt.IsZero() || e.TerminalAt.Before(e.AdmittedAt) {
		return errors.New("frozen evidence config or harness times are malformed")
	}
	if err := validateAttemptsComplete(e.State.Attempts); err != nil {
		return err
	}
	if err := validateRuns(e.Runs); err != nil {
		return err
	}
	if err := validateCommits(e.Commits, e.Runs, e.Workspace); err != nil {
		return err
	}
	if len(e.Provenance) == 0 {
		return errors.New("frozen evidence requires ordered provenance")
	}
	for _, item := range e.Provenance {
		if err := validateEvidence(item); err != nil {
			return err
		}
	}
	route, err := autonomouspolicy.Evaluate(autonomouspolicy.Input{TaskID: e.Task.TaskID, Decision: e.Decision, Reference: e.DecisionReference, State: e.State, Source: e.Source, Verification: &e.Verification, Audit: &e.Audit})
	if err != nil {
		return err
	}
	if route != e.Route {
		return errors.New("frozen evidence route does not match recomputed completion authorization")
	}
	return nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != ManifestSchemaVersion || m.TaskID == "" || m.OperationID == "" {
		return errors.New("completion manifest identity is malformed")
	}
	if err := m.FrozenEvidence.Validate(); err != nil {
		return err
	}
	if err := m.Capsule.Validate(); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, source := range m.Sources {
		if source.Kind == "" || source.Reference == "" || !validHash(source.SHA256) || source.ByteSize <= 0 || seen[source.Kind] {
			return errors.New("completion manifest sources are malformed or duplicated")
		}
		seen[source.Kind] = true
	}
	for _, omission := range m.Omissions {
		if omission.Kind == "" || omission.Reason == "" {
			return errors.New("completion manifest omission is malformed")
		}
	}
	return nil
}

func validateAttemptsComplete(attempts autonomous.AttemptState) error {
	open := map[string]bool{}
	for _, event := range attempts.Events {
		if event.Kind == autonomous.AttemptEventAdmitted {
			open[event.AttemptID] = true
		} else if event.Kind == autonomous.AttemptEventCompleted {
			delete(open, event.AttemptID)
		}
	}
	if len(open) != 0 {
		return errors.New("finalization gate: an attempt remains in flight")
	}
	return nil
}

func validateCommits(commits []CommitEvidence, runs []RunEvidence, workspace autonomous.TaskWorkspace) error {
	seen := map[string]bool{}
	runIDs := map[string]bool{}
	for _, run := range runs {
		runIDs[run.RunID] = true
	}
	for i, commit := range commits {
		actionOK := commit.Action == autonomous.ActionImplement || commit.Action == autonomous.ActionCorrect || commit.Action == autonomous.ActionDocument || commit.Action == autonomous.ActionSimplify
		if commit.Sequence != int64(i+1) || !validOID(commit.SHA) || !runIDs[commit.RunID] || !actionOK || commit.CreatedAt.IsZero() || (commit.Outcome != "created" && commit.Outcome != "reconciled") || seen[commit.SHA] {
			return fmt.Errorf("commit evidence %d is malformed, duplicated, or noncanonical", i+1)
		}
		if i > 0 && commit.ParentSHA != commits[i-1].SHA {
			return fmt.Errorf("commit evidence %d does not follow the prior commit", i+1)
		}
		if i > 0 && commit.CreatedAt.Before(commits[i-1].CreatedAt) {
			return fmt.Errorf("commit evidence %d precedes the prior commit time", i+1)
		}
		seen[commit.SHA] = true
	}
	if len(commits) == 0 {
		if workspace.HeadSHA != workspace.BaselineSHA {
			return errors.New("commit evidence is missing for an advanced final HEAD")
		}
		return nil
	}
	if commits[len(commits)-1].SHA != workspace.HeadSHA {
		return errors.New("final commit evidence does not match workspace HEAD")
	}
	return nil
}

func validateRuns(runs []RunEvidence) error {
	seen := map[string]bool{}
	for i, run := range runs {
		if run.Sequence != int64(i+1) || run.RunID == "" || run.Kind == "" || run.Outcome == "" || run.StartedAt.IsZero() || run.CompletedAt.Before(run.StartedAt) || seen[run.RunID] {
			return fmt.Errorf("run evidence %d is malformed or duplicated", i+1)
		}
		if err := validateEvidence(run.Artifact); err != nil {
			return err
		}
		seen[run.RunID] = true
	}
	return nil
}

func validateEvidence(item autonomous.EvidenceReference) error {
	switch item.Kind {
	case autonomous.EvidenceKindTask, autonomous.EvidenceKindPlan, autonomous.EvidenceKindLedger, autonomous.EvidenceKindReceipt, autonomous.EvidenceKindVerification, autonomous.EvidenceKindGit, autonomous.EvidenceKindAudit, autonomous.EvidenceKindRepository, autonomous.EvidenceKindFile:
	default:
		return errors.New("evidence reference kind is malformed")
	}
	if strings.TrimSpace(item.Reference) == "" || strings.TrimSpace(item.Detail) == "" {
		return errors.New("evidence reference is incomplete")
	}
	return nil
}

func hash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func validHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && value == strings.ToLower(value)
}
func validOID(value string) bool {
	return gitoid.Valid(value)
}
