package autonomousstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousaudit"
	"revolvr/internal/autonomouspolicy"
)

const AuditHistorySchemaVersion = "autonomous-audit-transition-v1"

type AuditTransitionKind string

const (
	AuditTransitionRecorded          AuditTransitionKind = "audit_recorded"
	AuditTransitionFindingResolved   AuditTransitionKind = "finding_resolved"
	AuditTransitionFindingWaived     AuditTransitionKind = "finding_waived"
	AuditTransitionFindingSuperseded AuditTransitionKind = "finding_superseded"
	AuditTransitionFindingInvalid    AuditTransitionKind = "finding_invalidated"
)

type ResolutionTransition struct {
	FindingID string                        `json:"finding_id"`
	Previous  autonomous.FindingResolution  `json:"previous"`
	Resulting autonomous.FindingResolution  `json:"resulting"`
	Authority *autonomous.DecisionReference `json:"authority,omitempty"`
}

type AuditHistoryRecord struct {
	SchemaVersion     string              `json:"schema_version"`
	TaskID            string              `json:"task_id"`
	Sequence          int64               `json:"sequence"`
	AuditRevision     int64               `json:"audit_revision"`
	OperationID       string              `json:"operation_id"`
	ApplicationSHA256 string              `json:"application_sha256"`
	Kind              AuditTransitionKind `json:"kind"`
	CreatedAt         time.Time           `json:"created_at"`

	Decision             autonomous.DecisionReference            `json:"audit_decision_reference"`
	SupervisorDecision   ArtifactIdentity                        `json:"supervisor_decision_artifact"`
	WorkerRunID          string                                  `json:"auditor_worker_run_id"`
	Profile              autonomousaudit.ProfileIdentity         `json:"auditor_profile"`
	Dossier              autonomousaudit.DossierIdentity         `json:"dossier"`
	SourceRevision       string                                  `json:"source_revision"`
	Verification         autonomouspolicy.VerificationEvidence   `json:"verification"`
	LatestSourceMutation *autonomousaudit.SourceMutationIdentity `json:"latest_source_mutation,omitempty"`
	TaskSource           ArtifactIdentity                        `json:"task_source"`
	RawOutput            ArtifactIdentity                        `json:"raw_output"`
	CanonicalOutput      ArtifactIdentity                        `json:"canonical_output"`
	Report               autonomous.AuditReport                  `json:"report"`
	PolicyEvidence       autonomouspolicy.AuditEvidence          `json:"policy_evidence"`

	PreviousState        StateIdentity                  `json:"previous_state"`
	ResultingState       StateIdentity                  `json:"resulting_state"`
	PreviousResolutions  []autonomous.FindingResolution `json:"previous_resolutions"`
	ResultingResolutions []autonomous.FindingResolution `json:"resulting_resolutions"`
	NewFindingIDs        []string                       `json:"new_finding_ids,omitempty"`
	Resolution           *ResolutionTransition          `json:"resolution_transition,omitempty"`
}

func (r AuditHistoryRecord) Validate() error {
	if r.SchemaVersion != AuditHistorySchemaVersion {
		return fmt.Errorf("validate audit history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if r.Sequence < 1 || r.AuditRevision < 1 {
		return errors.New("validate audit history: sequence and audit_revision must be positive")
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate audit history: operation/application identity is invalid")
	}
	if r.CreatedAt.IsZero() {
		return errors.New("validate audit history: created_at is required")
	}
	switch r.Kind {
	case AuditTransitionRecorded, AuditTransitionFindingResolved, AuditTransitionFindingWaived, AuditTransitionFindingSuperseded, AuditTransitionFindingInvalid:
	default:
		return fmt.Errorf("validate audit history: unknown kind %q", r.Kind)
	}
	if err := r.Decision.Validate(); err != nil || r.Decision.TaskID != r.TaskID || r.Decision.Action != autonomous.ActionAudit || r.Decision.WorkerProfile != autonomous.WorkerProfileAuditor {
		return errors.New("validate audit history: audit decision reference is invalid")
	}
	if err := r.SupervisorDecision.Validate(); err != nil || r.SupervisorDecision.Path != r.Decision.Artifact.Reference {
		return errors.New("validate audit history: supervisor decision artifact is invalid")
	}
	if err := validateIdentity("auditor_worker_run_id", r.WorkerRunID); err != nil || r.WorkerRunID == r.Decision.RunID {
		return errors.New("validate audit history: independent auditor worker identity is invalid")
	}
	if r.WorkerRunID == r.Verification.Summary.RunID || r.Decision.RunID == r.Verification.Summary.RunID {
		return errors.New("validate audit history: supervisor, auditor, and verification run identities must differ")
	}
	if r.LatestSourceMutation != nil && (r.WorkerRunID == r.LatestSourceMutation.RunID || r.Decision.RunID == r.LatestSourceMutation.RunID) {
		return errors.New("validate audit history: supervisor and auditor must differ from latest source-mutating run")
	}
	if err := r.Profile.Validate(); err != nil {
		return err
	}
	if err := r.Dossier.Validate(); err != nil || r.Dossier.TaskID != r.TaskID {
		return errors.New("validate audit history: dossier identity is invalid")
	}
	for _, artifact := range []ArtifactIdentity{r.TaskSource, r.RawOutput, r.CanonicalOutput} {
		if err := artifact.Validate(); err != nil {
			return fmt.Errorf("validate audit history: artifact: %w", err)
		}
	}
	if err := r.Report.Validate(); err != nil || r.Report.TaskID != r.TaskID {
		return errors.New("validate audit history: report is invalid")
	}
	if err := autonomouspolicy.ValidateEvidence(r.TaskID, autonomouspolicy.SourceEvidence{Revision: r.SourceRevision, Safety: autonomouspolicy.SourceSafetySafe, LatestMutation: r.LatestSourceMutation.Policy()}, &r.Verification, &r.PolicyEvidence); err != nil {
		return fmt.Errorf("validate audit history: policy evidence: %w", err)
	}
	if r.Verification.SourceRevision != r.SourceRevision || r.Verification.Summary.Status != autonomous.VerificationStatusPassed {
		return errors.New("validate audit history: verification is failed or stale for audited source")
	}
	if r.Verification.Tiered != nil && !r.Verification.Tiered.FinalSatisfied {
		return errors.New("validate audit history: tiered verification does not satisfy the final gate")
	}
	if !reflect.DeepEqual(r.PolicyEvidence.Report, r.Report) || r.PolicyEvidence.RunID != r.WorkerRunID || r.PolicyEvidence.AuditorProfile != autonomous.WorkerProfileAuditor || r.PolicyEvidence.SourceRevision != r.SourceRevision || r.PolicyEvidence.VerificationRunID != r.Verification.Summary.RunID || r.PolicyEvidence.VerificationOccurrenceID != r.Verification.Summary.OccurrenceID {
		return errors.New("validate audit history: policy evidence does not exactly project audit provenance")
	}
	if err := r.PreviousState.Validate(); err != nil {
		return err
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate audit history: resulting state identity is invalid")
	}
	if err := validateResolutionSlice(r.TaskID, r.PreviousResolutions); err != nil {
		return err
	}
	if err := validateResolutionSlice(r.TaskID, r.ResultingResolutions); err != nil {
		return err
	}
	if r.Kind == AuditTransitionRecorded {
		if r.Resolution != nil {
			return errors.New("validate audit history: audit record must not contain a resolution transition")
		}
	} else {
		if r.Resolution == nil || r.Resolution.FindingID == "" || r.Resolution.Previous.Status != autonomous.FindingResolutionStatusOpen || r.Resolution.Resulting.FindingID != r.Resolution.FindingID || r.Resolution.Resulting.Status == autonomous.FindingResolutionStatusOpen {
			return errors.New("validate audit history: finding transition is incomplete")
		}
	}
	return nil
}

func MarshalAuditHistory(record AuditHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
func DecodeAuditHistory(raw []byte) (AuditHistoryRecord, error) {
	var record AuditHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode audit history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalAuditHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode audit history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func validateResolutionSlice(taskID string, values []autonomous.FindingResolution) error {
	state := autonomous.ExecutionState{SchemaVersion: autonomous.ExecutionStateSchemaVersion, TaskID: taskID, Lifecycle: autonomous.LifecycleStateReady, FindingResolutions: values, Attempts: autonomous.AttemptState{RetryBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}, ElapsedTimeBudget: autonomous.DurationBudget{Mode: autonomous.BudgetModeUnset}, TokenBudget: autonomous.CountBudget{Mode: autonomous.BudgetModeUnset}}}
	return state.Validate()
}

type AuditHistorySnapshot struct {
	Record     AuditHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}

type AuditSnapshot struct {
	State           Snapshot
	Revision        int64
	Report          autonomous.AuditReport
	PolicyEvidence  autonomouspolicy.AuditEvidence
	CanonicalOutput ArtifactIdentity
	History         AuditHistorySnapshot
}
