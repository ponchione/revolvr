package autonomous

import (
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

	"revolvr/internal/gitoid"
)

const ExecutionStateSchemaVersion = "autonomous-execution-state-v1"

const FinalizationDetailSchemaVersion = "autonomous-finalization-state-v1"

type PlanStepStatus string

const (
	PlanStepStatusPending    PlanStepStatus = "pending"
	PlanStepStatusInProgress PlanStepStatus = "in_progress"
	PlanStepStatusCompleted  PlanStepStatus = "completed"
	PlanStepStatusSkipped    PlanStepStatus = "skipped"
)

type AcceptanceStatus string

const (
	AcceptanceStatusPending       AcceptanceStatus = "pending"
	AcceptanceStatusSatisfied     AcceptanceStatus = "satisfied"
	AcceptanceStatusWaived        AcceptanceStatus = "waived"
	AcceptanceStatusNotApplicable AcceptanceStatus = "not_applicable"
)

type FindingResolutionStatus string

const (
	FindingResolutionStatusOpen       FindingResolutionStatus = "open"
	FindingResolutionStatusResolved   FindingResolutionStatus = "resolved"
	FindingResolutionStatusWaived     FindingResolutionStatus = "waived"
	FindingResolutionStatusSuperseded FindingResolutionStatus = "superseded"
	FindingResolutionStatusInvalid    FindingResolutionStatus = "invalid"
)

type BudgetMode string

const (
	BudgetModeUnset     BudgetMode = "unset"
	BudgetModeLimited   BudgetMode = "limited"
	BudgetModeUnlimited BudgetMode = "unlimited"
)

type LifecycleState string

const (
	LifecycleStatePending    LifecycleState = "pending"
	LifecycleStateReady      LifecycleState = "ready"
	LifecycleStatePlanning   LifecycleState = "planning"
	LifecycleStateWorking    LifecycleState = "working"
	LifecycleStateVerifying  LifecycleState = "verifying"
	LifecycleStateAuditing   LifecycleState = "auditing"
	LifecycleStateCorrecting LifecycleState = "correcting"
	LifecycleStateNeedsInput LifecycleState = "needs_input"
	LifecycleStateFinalizing LifecycleState = "finalizing"
	LifecycleStateCompleted  LifecycleState = "completed"
	LifecycleStateBlocked    LifecycleState = "blocked"
	LifecycleStateCancelled  LifecycleState = "cancelled"
	LifecycleStateSuperseded LifecycleState = "superseded"
	LifecycleStateAbandoned  LifecycleState = "abandoned"
)

// TaskPlan identifies one immutable plan revision. A later revision has a new
// ID and names the plan revision it supersedes.
type TaskPlan struct {
	TaskID           string              `json:"task_id"`
	ID               string              `json:"id"`
	Revision         int64               `json:"revision"`
	SupersedesPlanID string              `json:"supersedes_plan_id,omitempty"`
	Provenance       []EvidenceReference `json:"provenance"`
	Steps            []PlanStep          `json:"steps"`
	Completed        bool                `json:"completed"`
}

// PlanStep order is the order in the containing TaskPlan.Steps slice.
type PlanStep struct {
	ID          string              `json:"id"`
	Description string              `json:"description"`
	Status      PlanStepStatus      `json:"status"`
	Evidence    []EvidenceReference `json:"evidence,omitempty"`
	Rationale   string              `json:"rationale,omitempty"`
}

type AcceptanceCriterion struct {
	ID          string              `json:"id"`
	Requirement string              `json:"requirement"`
	Status      AcceptanceStatus    `json:"status"`
	Evidence    []EvidenceReference `json:"evidence,omitempty"`
	Rationale   string              `json:"rationale,omitempty"`
	Source      *EvidenceReference  `json:"source,omitempty"`
}

type FindingResolution struct {
	FindingID            string                  `json:"finding_id"`
	Status               FindingResolutionStatus `json:"status"`
	Evidence             []EvidenceReference     `json:"evidence,omitempty"`
	Rationale            string                  `json:"rationale,omitempty"`
	SupersedingFindingID string                  `json:"superseding_finding_id,omitempty"`
	Resolution           *DecisionReference      `json:"resolution,omitempty"`
}

// DecisionReference retains the identity and provenance needed to locate a
// SupervisorDecision without embedding the full run or decision payload.
type DecisionReference struct {
	DecisionID    string            `json:"decision_id"`
	RunID         string            `json:"run_id"`
	TaskID        string            `json:"task_id"`
	Action        Action            `json:"action"`
	WorkerProfile WorkerProfile     `json:"worker_profile,omitempty"`
	Artifact      EvidenceReference `json:"artifact"`
	CreatedAt     time.Time         `json:"created_at"`
}

type ActionAttempt struct {
	Action   Action `json:"action"`
	Attempts int64  `json:"attempts"`
}

type ActionBudget struct {
	Action Action      `json:"action"`
	Budget CountBudget `json:"budget"`
}

type AttemptEventKind string

const (
	AttemptEventAdmitted  AttemptEventKind = "admitted"
	AttemptEventCompleted AttemptEventKind = "completed"
)

type AttemptOutcome string

const (
	AttemptOutcomeSucceeded     AttemptOutcome = "succeeded"
	AttemptOutcomeFailed        AttemptOutcome = "failed"
	AttemptOutcomeNoProgress    AttemptOutcome = "no_progress"
	AttemptOutcomeCancelled     AttemptOutcome = "cancelled"
	AttemptOutcomeSafetyStopped AttemptOutcome = "safety_stopped"
)

type SignatureKind string

const (
	SignatureKindDecision            SignatureKind = "decision"
	SignatureKindVerificationFailure SignatureKind = "verification_failure"
	SignatureKindOpenFindings        SignatureKind = "open_findings"
	SignatureKindOperationFailure    SignatureKind = "operation_failure"
)

type CanonicalSignature struct {
	Kind     SignatureKind       `json:"kind"`
	SHA256   string              `json:"sha256"`
	Evidence []EvidenceReference `json:"evidence,omitempty"`
}

// AttemptEvent is append-only. Admission and completion are distinct events so
// completion never rewrites the evidence that authorized and charged a run.
type AttemptEvent struct {
	Sequence         int64                `json:"sequence"`
	Kind             AttemptEventKind     `json:"kind"`
	AttemptID        string               `json:"attempt_id"`
	Action           Action               `json:"action"`
	Decision         DecisionReference    `json:"decision_reference"`
	StrategySHA256   string               `json:"strategy_sha256"`
	RunID            string               `json:"run_id,omitempty"`
	OccurrenceID     string               `json:"occurrence_id,omitempty"`
	SourceBefore     string               `json:"source_before"`
	SourceAfter      string               `json:"source_after,omitempty"`
	SourceAfterKnown bool                 `json:"source_after_known,omitempty"`
	Outcome          AttemptOutcome       `json:"outcome,omitempty"`
	Duration         time.Duration        `json:"duration_nanoseconds,omitempty"`
	Tokens           *int64               `json:"tokens,omitempty"`
	Evidence         []EvidenceReference  `json:"evidence,omitempty"`
	Signatures       []CanonicalSignature `json:"signatures,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

type BreakerReason string

const (
	BreakerTaskAttemptsExhausted   BreakerReason = "task_attempts_exhausted"
	BreakerActionAttemptsExhausted BreakerReason = "action_attempts_exhausted"
	BreakerElapsedExhausted        BreakerReason = "elapsed_exhausted"
	BreakerTokenExhausted          BreakerReason = "token_exhausted"
	BreakerRepeatedSignature       BreakerReason = "repeated_signature"
	BreakerUnchangedSource         BreakerReason = "unchanged_source"
	BreakerIdenticalStrategy       BreakerReason = "identical_strategy"
	BreakerMissingTrustedMetrics   BreakerReason = "missing_trusted_metrics"
	BreakerStaleEvidence           BreakerReason = "stale_state_or_evidence"
	BreakerCancellation            BreakerReason = "cancellation"
	BreakerUnsafeSource            BreakerReason = "unsafe_source_state"
	BreakerAccountingSafety        BreakerReason = "accounting_safety_stop"
)

type BudgetSnapshot struct {
	TaskAttempts CountBudget    `json:"task_attempts"`
	Action       Action         `json:"action"`
	ActionBudget CountBudget    `json:"action_budget"`
	Elapsed      DurationBudget `json:"elapsed"`
	Tokens       CountBudget    `json:"tokens"`
}

type CircuitBreakerDetail struct {
	Reason            BreakerReason       `json:"reason"`
	TriggerAttemptIDs []string            `json:"trigger_attempt_ids,omitempty"`
	TriggerSignature  *CanonicalSignature `json:"trigger_signature,omitempty"`
	RequiredStrategy  string              `json:"required_strategy_change_from,omitempty"`
	Budget            BudgetSnapshot      `json:"budget"`
	Evidence          []EvidenceReference `json:"evidence"`
}

// CountBudget is explicit about unset and unlimited limits. Consumed may be
// greater than a limited value so a durable snapshot can represent the exact
// over-limit state that authoritative attempt accounting must stop on replay.
type CountBudget struct {
	Mode     BudgetMode `json:"mode"`
	Limit    int64      `json:"limit"`
	Consumed int64      `json:"consumed"`
}

// DurationBudget serializes time.Duration values as integer nanoseconds.
type DurationBudget struct {
	Mode     BudgetMode    `json:"mode"`
	Limit    time.Duration `json:"limit_nanoseconds"`
	Consumed time.Duration `json:"consumed_nanoseconds"`
}

type AttemptState struct {
	TotalAttempts              int64                  `json:"total_attempts"`
	ActionAttempts             []ActionAttempt        `json:"action_attempts,omitempty"`
	ActionBudgets              []ActionBudget         `json:"action_budgets,omitempty"`
	ConsecutiveFailures        int64                  `json:"consecutive_failures"`
	RetryBudget                CountBudget            `json:"retry_budget"`
	ElapsedTimeBudget          DurationBudget         `json:"elapsed_time_budget"`
	TokenBudget                CountBudget            `json:"token_budget"`
	RepeatedSignatureLimit     int64                  `json:"repeated_signature_limit,omitempty"`
	RequiredStrategyChangeFrom string                 `json:"required_strategy_change_from,omitempty"`
	TransitionSequence         int64                  `json:"transition_sequence,omitempty"`
	Events                     []AttemptEvent         `json:"events,omitempty"`
	ActionStops                []CircuitBreakerDetail `json:"action_stops,omitempty"`
	LastFailure                *EvidenceReference     `json:"last_failure,omitempty"`
}

type QuestionIdentity struct {
	QuestionID    string `json:"question_id"`
	Revision      int64  `json:"revision"`
	ContentSHA256 string `json:"content_sha256"`
}

type InputQuestionRecord struct {
	Sequence        int64              `json:"sequence"`
	TaskID          string             `json:"task_id"`
	Question        NeedsInputQuestion `json:"question"`
	Decision        DecisionReference  `json:"decision_reference"`
	SourceRevision  string             `json:"source_revision"`
	ResumeLifecycle LifecycleState     `json:"resume_lifecycle"`
	Supersedes      *QuestionIdentity  `json:"supersedes,omitempty"`
	RecordedAt      time.Time          `json:"recorded_at"`
}

type AnswerProvenanceKind string

const AnswerProvenanceOperator AnswerProvenanceKind = "operator"

type AnswerProvenance struct {
	Kind     AnswerProvenanceKind `json:"kind"`
	Actor    string               `json:"actor"`
	Evidence []EvidenceReference  `json:"evidence,omitempty"`
}

type InputAnswerRecord struct {
	Sequence   int64            `json:"sequence"`
	AnswerID   string           `json:"answer_id"`
	TaskID     string           `json:"task_id"`
	Question   QuestionIdentity `json:"question"`
	OptionID   string           `json:"option_id"`
	Provenance AnswerProvenance `json:"provenance"`
	AnsweredAt time.Time        `json:"answered_at"`
}

type InputResumeRecord struct {
	Sequence  int64            `json:"sequence"`
	ResumeID  string           `json:"resume_id"`
	TaskID    string           `json:"task_id"`
	Question  QuestionIdentity `json:"question"`
	AnswerID  string           `json:"answer_id"`
	ResumedAt time.Time        `json:"resumed_at"`
}

type InputState struct {
	TransitionSequence int64                 `json:"transition_sequence,omitempty"`
	Questions          []InputQuestionRecord `json:"questions,omitempty"`
	Answers            []InputAnswerRecord   `json:"answers,omitempty"`
	Resumes            []InputResumeRecord   `json:"resumes,omitempty"`
}

// NeedsInputDetail retains a narrow legacy reason reader. Typed authority is
// present only when CurrentQuestion is non-nil and exactly matches InputState.
type NeedsInputDetail struct {
	Reason          string            `json:"reason,omitempty"`
	CurrentQuestion *QuestionIdentity `json:"current_question,omitempty"`
}

type TerminalDetail struct {
	Reason   string              `json:"reason"`
	Evidence []EvidenceReference `json:"evidence,omitempty"`
}

const ReopenLineageSchemaVersion = "autonomous-reopen-lineage-v1"
const ChildLineageSchemaVersion = "autonomous-child-lineage-v1"

type ChildLineage struct {
	SchemaVersion   string              `json:"schema_version"`
	OperationID     string              `json:"operation_id"`
	ParentTaskID    string              `json:"parent_task_id"`
	ProposalID      string              `json:"proposal_id"`
	ProposalKey     string              `json:"proposal_key"`
	DecisionID      string              `json:"decision_id"`
	SupervisorRunID string              `json:"supervisor_run_id"`
	ParentBehavior  ChildParentBehavior `json:"parent_behavior"`
	Evidence        []EvidenceReference `json:"evidence"`
	CreatedAt       time.Time           `json:"created_at"`
}

func (l ChildLineage) Validate() error {
	if l.SchemaVersion != ChildLineageSchemaVersion || !validTaskIdentity(l.OperationID) || !validTaskIdentity(l.ParentTaskID) || !validFindingID(l.ProposalID) || !validFindingID(l.ProposalKey) || !validTaskIdentity(l.DecisionID) || !validTaskIdentity(l.SupervisorRunID) || l.CreatedAt.IsZero() {
		return errors.New("validate child lineage: schema, identity, or creation time is malformed")
	}
	if l.ParentBehavior != ChildDependsOnParent && l.ParentBehavior != ChildIndependent {
		return errors.New("validate child lineage: parent behavior is invalid")
	}
	if err := validateEvidenceReferences("validate child lineage: evidence", l.Evidence); err != nil {
		return err
	}
	return nil
}

// ReopenLineage binds a new active lifecycle to one immutable verified
// archive. Reopen creates a new task identity; it never changes the archived
// terminal state or weakens terminal transition validation.
type ReopenLineage struct {
	SchemaVersion      string    `json:"schema_version"`
	OperationID        string    `json:"operation_id"`
	ArchiveID          string    `json:"archive_id"`
	ArchivedTaskID     string    `json:"archived_task_id"`
	ArchivedTaskSHA256 string    `json:"archived_task_sha256"`
	ArchivedTaskSize   int       `json:"archived_task_byte_size"`
	Disposition        string    `json:"disposition"`
	ArchiveCommitSHA   string    `json:"archive_commit_sha"`
	Authority          string    `json:"authority"`
	Reason             string    `json:"reason"`
	ReopenedAt         time.Time `json:"reopened_at"`
}

func (r ReopenLineage) Validate() error {
	if r.SchemaVersion != ReopenLineageSchemaVersion {
		return fmt.Errorf("validate reopen lineage: unsupported schema_version %q", r.SchemaVersion)
	}
	for _, field := range []struct{ label, value string }{{"operation_id", r.OperationID}, {"archive_id", r.ArchiveID}, {"archived_task_id", r.ArchivedTaskID}, {"disposition", r.Disposition}, {"authority", r.Authority}, {"reason", r.Reason}} {
		if strings.TrimSpace(field.value) == "" || field.value != strings.TrimSpace(field.value) || strings.ContainsAny(field.value, "\r\n") {
			return fmt.Errorf("validate reopen lineage: %s is empty or malformed", field.label)
		}
	}
	if !validStateSHA256(r.ArchivedTaskSHA256) || r.ArchivedTaskSize <= 0 {
		return errors.New("validate reopen lineage: archived task identity is malformed")
	}
	if !validGitObjectID(r.ArchiveCommitSHA) {
		return errors.New("validate reopen lineage: archive commit SHA is malformed")
	}
	if r.ReopenedAt.IsZero() {
		return errors.New("validate reopen lineage: reopened_at is required")
	}
	return nil
}

// FinalizationStage is monotonic durable progress for the terminal
// transaction. Each value names the last effect that has been strictly
// read back; it is not a collection of independently mutable flags.
type FinalizationStage string

const (
	FinalizationStageAdmitted        FinalizationStage = "admitted"
	FinalizationStageMaterialized    FinalizationStage = "capsule_materialized"
	FinalizationStageTaskCompleted   FinalizationStage = "task_completed"
	FinalizationStageStateCompleted  FinalizationStage = "state_completed"
	FinalizationStageLedgerCompleted FinalizationStage = "ledger_completed"
)

type FinalizationArtifact struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type FinalizationDetail struct {
	SchemaVersion       string                `json:"schema_version"`
	OperationID         string                `json:"operation_id"`
	RunID               string                `json:"run_id"`
	Stage               FinalizationStage     `json:"stage"`
	FrozenEvidence      FinalizationArtifact  `json:"frozen_evidence"`
	Capsule             *FinalizationArtifact `json:"capsule,omitempty"`
	Manifest            *FinalizationArtifact `json:"manifest,omitempty"`
	OriginalTaskSHA256  string                `json:"original_task_sha256"`
	CompletedTaskSHA256 string                `json:"completed_task_sha256,omitempty"`
	AdmittedAt          time.Time             `json:"admitted_at"`
	MaterializedAt      *time.Time            `json:"materialized_at,omitempty"`
	TaskCompletedAt     *time.Time            `json:"task_completed_at,omitempty"`
	StateCompletedAt    *time.Time            `json:"state_completed_at,omitempty"`
	LedgerCompletedAt   *time.Time            `json:"ledger_completed_at,omitempty"`
}

type ExecutionState struct {
	SchemaVersion      string                   `json:"schema_version"`
	TaskID             string                   `json:"task_id"`
	Lifecycle          LifecycleState           `json:"lifecycle"`
	Plan               *TaskPlan                `json:"plan,omitempty"`
	AcceptanceCriteria []AcceptanceCriterion    `json:"acceptance_criteria,omitempty"`
	FindingResolutions []FindingResolution      `json:"finding_resolutions,omitempty"`
	OptionalRoles      []OptionalRoleOccurrence `json:"optional_role_occurrences,omitempty"`
	LatestDecision     *DecisionReference       `json:"latest_decision,omitempty"`
	Attempts           AttemptState             `json:"attempts"`
	Input              InputState               `json:"input,omitempty"`
	Workspace          *TaskWorkspace           `json:"workspace,omitempty"`
	NeedsInput         *NeedsInputDetail        `json:"needs_input,omitempty"`
	Terminal           *TerminalDetail          `json:"terminal,omitempty"`
	CircuitBreaker     *CircuitBreakerDetail    `json:"circuit_breaker,omitempty"`
	Finalization       *FinalizationDetail      `json:"finalization,omitempty"`
	ReopenedFrom       *ReopenLineage           `json:"reopened_from,omitempty"`
	ChildOf            *ChildLineage            `json:"child_of,omitempty"`
}

func (s ExecutionState) MarshalJSON() ([]byte, error) {
	type wire struct {
		SchemaVersion      string                   `json:"schema_version"`
		TaskID             string                   `json:"task_id"`
		Lifecycle          LifecycleState           `json:"lifecycle"`
		Plan               *TaskPlan                `json:"plan,omitempty"`
		AcceptanceCriteria []AcceptanceCriterion    `json:"acceptance_criteria,omitempty"`
		FindingResolutions []FindingResolution      `json:"finding_resolutions,omitempty"`
		OptionalRoles      []OptionalRoleOccurrence `json:"optional_role_occurrences,omitempty"`
		LatestDecision     *DecisionReference       `json:"latest_decision,omitempty"`
		Attempts           AttemptState             `json:"attempts"`
		Input              *InputState              `json:"input,omitempty"`
		Workspace          *TaskWorkspace           `json:"workspace,omitempty"`
		NeedsInput         *NeedsInputDetail        `json:"needs_input,omitempty"`
		Terminal           *TerminalDetail          `json:"terminal,omitempty"`
		CircuitBreaker     *CircuitBreakerDetail    `json:"circuit_breaker,omitempty"`
		Finalization       *FinalizationDetail      `json:"finalization,omitempty"`
		ReopenedFrom       *ReopenLineage           `json:"reopened_from,omitempty"`
		ChildOf            *ChildLineage            `json:"child_of,omitempty"`
	}
	var input *InputState
	if s.Input.TransitionSequence != 0 || len(s.Input.Questions) != 0 || len(s.Input.Answers) != 0 || len(s.Input.Resumes) != 0 {
		value := s.Input
		input = &value
	}
	return json.Marshal(wire{SchemaVersion: s.SchemaVersion, TaskID: s.TaskID, Lifecycle: s.Lifecycle, Plan: s.Plan, AcceptanceCriteria: s.AcceptanceCriteria, FindingResolutions: s.FindingResolutions, OptionalRoles: s.OptionalRoles, LatestDecision: s.LatestDecision, Attempts: s.Attempts, Input: input, Workspace: s.Workspace, NeedsInput: s.NeedsInput, Terminal: s.Terminal, CircuitBreaker: s.CircuitBreaker, Finalization: s.Finalization, ReopenedFrom: s.ReopenedFrom, ChildOf: s.ChildOf})
}

func (p TaskPlan) Validate() error {
	if strings.TrimSpace(p.TaskID) == "" {
		return errors.New("validate task plan: task_id is required")
	}
	if !validFindingID(p.ID) {
		return fmt.Errorf("validate task plan: invalid id %q (want lower-case kebab-case beginning with a letter)", p.ID)
	}
	if p.Revision < 1 {
		return fmt.Errorf("validate task plan: revision must be at least 1 (got %d)", p.Revision)
	}
	if p.Revision == 1 && strings.TrimSpace(p.SupersedesPlanID) != "" {
		return errors.New("validate task plan: revision 1 must not set supersedes_plan_id")
	}
	if p.Revision > 1 {
		if !validFindingID(p.SupersedesPlanID) {
			return fmt.Errorf("validate task plan: invalid supersedes_plan_id %q (want lower-case kebab-case beginning with a letter)", p.SupersedesPlanID)
		}
		if p.SupersedesPlanID == p.ID {
			return errors.New("validate task plan: supersedes_plan_id must differ from id")
		}
	}
	if err := validateEvidenceReferences("validate task plan: provenance", p.Provenance); err != nil {
		return err
	}
	if len(p.Steps) == 0 {
		return errors.New("validate task plan: steps requires at least one plan step")
	}

	seen := make(map[string]struct{}, len(p.Steps))
	inProgress := 0
	allTerminal := true
	for i, step := range p.Steps {
		if err := step.Validate(); err != nil {
			return fmt.Errorf("validate task plan: steps[%d]: %w", i, err)
		}
		if _, exists := seen[step.ID]; exists {
			return fmt.Errorf("validate task plan: steps[%d]: duplicate step id %q", i, step.ID)
		}
		seen[step.ID] = struct{}{}
		if step.Status == PlanStepStatusInProgress {
			inProgress++
		}
		if step.Status != PlanStepStatusCompleted && step.Status != PlanStepStatusSkipped {
			allTerminal = false
		}
	}
	if inProgress > 1 {
		return fmt.Errorf("validate task plan: at most one step may be in_progress (got %d)", inProgress)
	}
	if p.Completed && !allTerminal {
		return errors.New("validate task plan: completed plan contains unfinished steps")
	}
	return nil
}

func (s PlanStep) Validate() error {
	if !validFindingID(s.ID) {
		return fmt.Errorf("invalid step id %q (want lower-case kebab-case beginning with a letter)", s.ID)
	}
	if strings.TrimSpace(s.Description) == "" {
		return errors.New("description is required")
	}
	if !validPlanStepStatus(s.Status) {
		return fmt.Errorf("unknown status %q", s.Status)
	}

	switch s.Status {
	case PlanStepStatusPending, PlanStepStatusInProgress:
		if len(s.Evidence) != 0 {
			return fmt.Errorf("status %q must not include terminal evidence", s.Status)
		}
		if strings.TrimSpace(s.Rationale) != "" {
			return fmt.Errorf("status %q must not include skip rationale", s.Status)
		}
	case PlanStepStatusCompleted:
		if err := validateEvidenceReferences("completed step evidence", s.Evidence); err != nil {
			return err
		}
		if strings.TrimSpace(s.Rationale) != "" {
			return errors.New("completed step must not include skip rationale")
		}
	case PlanStepStatusSkipped:
		if strings.TrimSpace(s.Rationale) == "" {
			return errors.New("skipped step requires rationale")
		}
		if err := validateOptionalEvidenceReferences("skipped step evidence", s.Evidence); err != nil {
			return err
		}
	}
	return nil
}

func (c AcceptanceCriterion) Validate() error {
	if !validFindingID(c.ID) {
		return fmt.Errorf("validate acceptance criterion: invalid id %q (want lower-case kebab-case beginning with a letter)", c.ID)
	}
	if strings.TrimSpace(c.Requirement) == "" {
		return errors.New("validate acceptance criterion: requirement is required")
	}
	if !validAcceptanceStatus(c.Status) {
		return fmt.Errorf("validate acceptance criterion: unknown status %q", c.Status)
	}
	if c.Source != nil {
		if err := validateEvidenceReferences("validate acceptance criterion: source", []EvidenceReference{*c.Source}); err != nil {
			return err
		}
	}

	switch c.Status {
	case AcceptanceStatusPending:
		if len(c.Evidence) != 0 {
			return errors.New("validate acceptance criterion: pending status must not include disposition evidence")
		}
		if strings.TrimSpace(c.Rationale) != "" {
			return errors.New("validate acceptance criterion: pending status must not include disposition rationale")
		}
	case AcceptanceStatusSatisfied:
		if err := validateEvidenceReferences("validate acceptance criterion: satisfied evidence", c.Evidence); err != nil {
			return err
		}
	case AcceptanceStatusWaived:
		if strings.TrimSpace(c.Rationale) == "" {
			return errors.New("validate acceptance criterion: waived status requires rationale")
		}
		if err := validateOptionalEvidenceReferences("validate acceptance criterion: waived evidence", c.Evidence); err != nil {
			return err
		}
	case AcceptanceStatusNotApplicable:
		if strings.TrimSpace(c.Rationale) == "" {
			return errors.New("validate acceptance criterion: not_applicable status requires rationale")
		}
		if err := validateOptionalEvidenceReferences("validate acceptance criterion: not_applicable evidence", c.Evidence); err != nil {
			return err
		}
	}
	return nil
}

func (r FindingResolution) Validate() error {
	if !validFindingID(r.FindingID) {
		return fmt.Errorf("validate finding resolution: invalid finding_id %q (want lower-case kebab-case beginning with a letter)", r.FindingID)
	}
	if !validFindingResolutionStatus(r.Status) {
		return fmt.Errorf("validate finding resolution: unknown status %q", r.Status)
	}

	switch r.Status {
	case FindingResolutionStatusOpen:
		if len(r.Evidence) != 0 {
			return errors.New("validate finding resolution: open status must not include resolution evidence")
		}
		if strings.TrimSpace(r.Rationale) != "" {
			return errors.New("validate finding resolution: open status must not include resolution rationale")
		}
		if strings.TrimSpace(r.SupersedingFindingID) != "" {
			return errors.New("validate finding resolution: open status must not set superseding_finding_id")
		}
		if r.Resolution != nil {
			return errors.New("validate finding resolution: open status must not include resolution reference")
		}
	case FindingResolutionStatusResolved:
		if err := validateEvidenceReferences("validate finding resolution: resolved evidence", r.Evidence); err != nil {
			return err
		}
		if strings.TrimSpace(r.SupersedingFindingID) != "" {
			return errors.New("validate finding resolution: resolved status must not set superseding_finding_id")
		}
	case FindingResolutionStatusWaived:
		if strings.TrimSpace(r.Rationale) == "" {
			return errors.New("validate finding resolution: waived status requires rationale")
		}
		if strings.TrimSpace(r.SupersedingFindingID) != "" {
			return errors.New("validate finding resolution: waived status must not set superseding_finding_id")
		}
		if err := validateOptionalEvidenceReferences("validate finding resolution: waived evidence", r.Evidence); err != nil {
			return err
		}
	case FindingResolutionStatusSuperseded:
		if !validFindingID(r.SupersedingFindingID) {
			return fmt.Errorf("validate finding resolution: invalid superseding_finding_id %q (want lower-case kebab-case beginning with a letter)", r.SupersedingFindingID)
		}
		if r.SupersedingFindingID == r.FindingID {
			return errors.New("validate finding resolution: finding cannot supersede itself")
		}
		if err := validateOptionalEvidenceReferences("validate finding resolution: superseded evidence", r.Evidence); err != nil {
			return err
		}
	case FindingResolutionStatusInvalid:
		if strings.TrimSpace(r.Rationale) == "" {
			return errors.New("validate finding resolution: invalid status requires rationale")
		}
		if strings.TrimSpace(r.SupersedingFindingID) != "" {
			return errors.New("validate finding resolution: invalid status must not set superseding_finding_id")
		}
		if err := validateOptionalEvidenceReferences("validate finding resolution: invalid evidence", r.Evidence); err != nil {
			return err
		}
	}

	if r.Resolution != nil {
		if err := r.Resolution.Validate(); err != nil {
			return fmt.Errorf("validate finding resolution: resolution: %w", err)
		}
	}
	return nil
}

func (r DecisionReference) Validate() error {
	if !validFindingID(r.DecisionID) {
		return fmt.Errorf("validate decision reference: invalid decision_id %q (want lower-case kebab-case beginning with a letter)", r.DecisionID)
	}
	if strings.TrimSpace(r.RunID) == "" {
		return errors.New("validate decision reference: run_id is required")
	}
	if strings.TrimSpace(r.TaskID) == "" {
		return errors.New("validate decision reference: task_id is required")
	}
	if !validAction(r.Action) {
		return fmt.Errorf("validate decision reference: unknown action %q", r.Action)
	}

	expectedProfile, workerAction := workerProfileForAction(r.Action)
	if workerAction {
		if WorkerProfile(strings.TrimSpace(string(r.WorkerProfile))) != expectedProfile {
			return fmt.Errorf("validate decision reference: action %q requires compatible worker_profile %q (got %q)", r.Action, expectedProfile, r.WorkerProfile)
		}
	} else if strings.TrimSpace(string(r.WorkerProfile)) != "" {
		return fmt.Errorf("validate decision reference: terminal action %q must not select worker_profile %q", r.Action, r.WorkerProfile)
	}
	if err := validateEvidenceReferences("validate decision reference: artifact", []EvidenceReference{r.Artifact}); err != nil {
		return err
	}
	if r.CreatedAt.IsZero() {
		return errors.New("validate decision reference: created_at is required")
	}
	return nil
}

func (s CanonicalSignature) Validate() error {
	if !validSignatureKind(s.Kind) {
		return fmt.Errorf("unknown signature kind %q", s.Kind)
	}
	if !validStateSHA256(s.SHA256) {
		return errors.New("signature SHA-256 is invalid")
	}
	if err := validateOptionalEvidenceReferences("signature evidence", s.Evidence); err != nil {
		return err
	}
	return nil
}

func (e AttemptEvent) Validate(taskID string) error {
	if e.Sequence < 1 {
		return fmt.Errorf("sequence must be at least 1 (got %d)", e.Sequence)
	}
	if strings.TrimSpace(e.AttemptID) == "" || e.AttemptID != strings.TrimSpace(e.AttemptID) || strings.ContainsAny(e.AttemptID, "\r\n") {
		return fmt.Errorf("attempt_id %q is empty or malformed", e.AttemptID)
	}
	if !validAction(e.Action) {
		return fmt.Errorf("unknown action %q", e.Action)
	}
	if err := e.Decision.Validate(); err != nil {
		return fmt.Errorf("decision_reference: %w", err)
	}
	if (taskID != "" && e.Decision.TaskID != taskID) || e.Decision.Action != e.Action {
		return errors.New("decision reference does not match event task and action")
	}
	if !validStateSHA256(e.StrategySHA256) || !validStateSHA256(e.SourceBefore) {
		return errors.New("strategy_sha256 and source_before must be valid SHA-256 values")
	}
	if e.CreatedAt.IsZero() {
		return errors.New("created_at is required")
	}
	if err := validateOptionalEvidenceReferences("attempt event evidence", e.Evidence); err != nil {
		return err
	}
	seenSignatures := make(map[SignatureKind]struct{}, len(e.Signatures))
	for i, signature := range e.Signatures {
		if err := signature.Validate(); err != nil {
			return fmt.Errorf("signatures[%d]: %w", i, err)
		}
		if _, ok := seenSignatures[signature.Kind]; ok {
			return fmt.Errorf("signatures[%d]: duplicate kind %q", i, signature.Kind)
		}
		seenSignatures[signature.Kind] = struct{}{}
	}
	switch e.Kind {
	case AttemptEventAdmitted:
		if e.RunID != "" || e.OccurrenceID != "" || e.SourceAfter != "" || e.SourceAfterKnown || e.Outcome != "" || e.Duration != 0 || e.Tokens != nil || len(e.Signatures) != 0 {
			return errors.New("admitted event contains completion-only evidence")
		}
	case AttemptEventCompleted:
		if !validAttemptOutcome(e.Outcome) {
			return fmt.Errorf("unknown completion outcome %q", e.Outcome)
		}
		if strings.TrimSpace(e.RunID) == "" {
			return errors.New("completed event requires run_id")
		}
		if e.SourceAfterKnown {
			if !validStateSHA256(e.SourceAfter) {
				return errors.New("completed event with known source requires valid source_after")
			}
		} else if e.SourceAfter != "" || (e.Outcome != AttemptOutcomeCancelled && e.Outcome != AttemptOutcomeSafetyStopped) {
			return errors.New("completed event requires known source_after unless cancelled or safety-stopped")
		}
		if e.Duration < 0 {
			return fmt.Errorf("duration cannot be negative (got %s)", e.Duration)
		}
		if e.Tokens != nil && *e.Tokens < 0 {
			return fmt.Errorf("tokens cannot be negative (got %d)", *e.Tokens)
		}
	default:
		return fmt.Errorf("unknown event kind %q", e.Kind)
	}
	return nil
}

func (d CircuitBreakerDetail) Validate() error {
	if !validBreakerReason(d.Reason) {
		return fmt.Errorf("unknown reason %q", d.Reason)
	}
	seen := make(map[string]struct{}, len(d.TriggerAttemptIDs))
	for i, id := range d.TriggerAttemptIDs {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return fmt.Errorf("trigger_attempt_ids[%d] is empty or malformed", i)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("trigger_attempt_ids[%d] duplicates %q", i, id)
		}
		seen[id] = struct{}{}
	}
	if d.TriggerSignature != nil {
		if err := d.TriggerSignature.Validate(); err != nil {
			return fmt.Errorf("trigger_signature: %w", err)
		}
	}
	if d.RequiredStrategy != "" && !validStateSHA256(d.RequiredStrategy) {
		return errors.New("required_strategy_change_from is invalid")
	}
	if err := d.Budget.TaskAttempts.Validate(); err != nil {
		return fmt.Errorf("budget task_attempts: %w", err)
	}
	if !validAction(d.Budget.Action) {
		return fmt.Errorf("budget action %q is invalid", d.Budget.Action)
	}
	if err := d.Budget.ActionBudget.Validate(); err != nil {
		return fmt.Errorf("budget action_budget: %w", err)
	}
	if err := d.Budget.Elapsed.Validate(); err != nil {
		return fmt.Errorf("budget elapsed: %w", err)
	}
	if err := d.Budget.Tokens.Validate(); err != nil {
		return fmt.Errorf("budget tokens: %w", err)
	}
	if err := validateEvidenceReferences("circuit breaker evidence", d.Evidence); err != nil {
		return err
	}
	return nil
}

func (b CountBudget) Validate() error {
	if !validBudgetMode(b.Mode) {
		return fmt.Errorf("unknown mode %q", b.Mode)
	}
	if b.Limit < 0 {
		return fmt.Errorf("limit cannot be negative (got %d)", b.Limit)
	}
	if b.Consumed < 0 {
		return fmt.Errorf("consumed cannot be negative (got %d)", b.Consumed)
	}
	if b.Mode != BudgetModeLimited && b.Limit != 0 {
		return fmt.Errorf("mode %q must use a zero limit (got %d)", b.Mode, b.Limit)
	}
	return nil
}

func (b DurationBudget) Validate() error {
	if !validBudgetMode(b.Mode) {
		return fmt.Errorf("unknown mode %q", b.Mode)
	}
	if b.Limit < 0 {
		return fmt.Errorf("limit cannot be negative (got %s)", b.Limit)
	}
	if b.Consumed < 0 {
		return fmt.Errorf("consumed cannot be negative (got %s)", b.Consumed)
	}
	if b.Mode != BudgetModeLimited && b.Limit != 0 {
		return fmt.Errorf("mode %q must use a zero limit (got %s)", b.Mode, b.Limit)
	}
	return nil
}

func (a AttemptState) Validate() error {
	if a.TotalAttempts < 0 {
		return fmt.Errorf("validate attempt state: total_attempts cannot be negative (got %d)", a.TotalAttempts)
	}
	if a.ConsecutiveFailures < 0 {
		return fmt.Errorf("validate attempt state: consecutive_failures cannot be negative (got %d)", a.ConsecutiveFailures)
	}
	if a.ConsecutiveFailures > a.TotalAttempts {
		return fmt.Errorf("validate attempt state: consecutive_failures %d exceeds total_attempts %d", a.ConsecutiveFailures, a.TotalAttempts)
	}

	seen := make(map[Action]struct{}, len(a.ActionAttempts))
	var actionTotal int64
	for i, counter := range a.ActionAttempts {
		if !validAction(counter.Action) {
			return fmt.Errorf("validate attempt state: action_attempts[%d]: unknown action %q", i, counter.Action)
		}
		if counter.Attempts < 0 {
			return fmt.Errorf("validate attempt state: action_attempts[%d]: attempts cannot be negative (got %d)", i, counter.Attempts)
		}
		if _, exists := seen[counter.Action]; exists {
			return fmt.Errorf("validate attempt state: action_attempts[%d]: duplicate action %q", i, counter.Action)
		}
		seen[counter.Action] = struct{}{}
		if counter.Attempts > a.TotalAttempts-actionTotal {
			return fmt.Errorf("validate attempt state: action attempt total exceeds total_attempts %d", a.TotalAttempts)
		}
		actionTotal += counter.Attempts
	}
	if err := a.RetryBudget.Validate(); err != nil {
		return fmt.Errorf("validate attempt state: retry_budget: %w", err)
	}
	if a.RetryBudget.Consumed > a.TotalAttempts {
		return fmt.Errorf("validate attempt state: retry_budget consumed %d exceeds total_attempts %d", a.RetryBudget.Consumed, a.TotalAttempts)
	}
	if err := a.ElapsedTimeBudget.Validate(); err != nil {
		return fmt.Errorf("validate attempt state: elapsed_time_budget: %w", err)
	}
	if err := a.TokenBudget.Validate(); err != nil {
		return fmt.Errorf("validate attempt state: token_budget: %w", err)
	}
	if a.RepeatedSignatureLimit < 0 {
		return fmt.Errorf("validate attempt state: repeated_signature_limit cannot be negative (got %d)", a.RepeatedSignatureLimit)
	}
	if a.RequiredStrategyChangeFrom != "" && !validStateSHA256(a.RequiredStrategyChangeFrom) {
		return errors.New("validate attempt state: required_strategy_change_from is invalid")
	}
	if a.TransitionSequence < 0 {
		return fmt.Errorf("validate attempt state: transition_sequence cannot be negative (got %d)", a.TransitionSequence)
	}
	budgetSeen := make(map[Action]struct{}, len(a.ActionBudgets))
	for i, actionBudget := range a.ActionBudgets {
		if !validAction(actionBudget.Action) {
			return fmt.Errorf("validate attempt state: action_budgets[%d]: unknown action %q", i, actionBudget.Action)
		}
		if _, ok := budgetSeen[actionBudget.Action]; ok {
			return fmt.Errorf("validate attempt state: action_budgets[%d]: duplicate action %q", i, actionBudget.Action)
		}
		budgetSeen[actionBudget.Action] = struct{}{}
		if err := actionBudget.Budget.Validate(); err != nil {
			return fmt.Errorf("validate attempt state: action_budgets[%d]: %w", i, err)
		}
	}
	admitted := make(map[string]AttemptEvent, len(a.Events))
	completed := make(map[string]struct{}, len(a.Events))
	for i, event := range a.Events {
		if event.Sequence > a.TransitionSequence || (i > 0 && event.Sequence <= a.Events[i-1].Sequence) {
			return fmt.Errorf("validate attempt state: events[%d] sequence %d is not strictly ordered within transition sequence %d", i, event.Sequence, a.TransitionSequence)
		}
		if err := event.Validate(""); err != nil {
			return fmt.Errorf("validate attempt state: events[%d]: %w", i, err)
		}
		switch event.Kind {
		case AttemptEventAdmitted:
			if _, ok := admitted[event.AttemptID]; ok {
				return fmt.Errorf("validate attempt state: events[%d]: duplicate admission for %q", i, event.AttemptID)
			}
			admitted[event.AttemptID] = event
		case AttemptEventCompleted:
			prior, ok := admitted[event.AttemptID]
			if !ok || prior.Action != event.Action || prior.Decision != event.Decision || prior.StrategySHA256 != event.StrategySHA256 || prior.SourceBefore != event.SourceBefore {
				return fmt.Errorf("validate attempt state: events[%d]: completion does not match admission %q", i, event.AttemptID)
			}
			if _, ok := completed[event.AttemptID]; ok {
				return fmt.Errorf("validate attempt state: events[%d]: duplicate completion for %q", i, event.AttemptID)
			}
			completed[event.AttemptID] = struct{}{}
		}
	}
	stopActions := make(map[Action]struct{}, len(a.ActionStops))
	for i, stop := range a.ActionStops {
		if err := stop.Validate(); err != nil {
			return fmt.Errorf("validate attempt state: action_stops[%d]: %w", i, err)
		}
		if stop.Reason != BreakerActionAttemptsExhausted {
			return fmt.Errorf("validate attempt state: action_stops[%d] has non-action reason %q", i, stop.Reason)
		}
		if _, ok := stopActions[stop.Budget.Action]; ok {
			return fmt.Errorf("validate attempt state: action_stops[%d] duplicates action %q", i, stop.Budget.Action)
		}
		stopActions[stop.Budget.Action] = struct{}{}
	}
	if a.LastFailure != nil {
		if err := validateEvidenceReferences("validate attempt state: last_failure", []EvidenceReference{*a.LastFailure}); err != nil {
			return err
		}
	}
	return nil
}

func (i QuestionIdentity) Validate() error {
	if !validFindingID(i.QuestionID) || i.Revision < 1 || !validStateSHA256(i.ContentSHA256) {
		return errors.New("question identity requires a stable ID, positive revision, and valid content SHA-256")
	}
	return nil
}

func (r InputQuestionRecord) Validate(taskID string) error {
	if r.Sequence < 1 || r.TaskID != taskID || r.RecordedAt.IsZero() || !validStateSHA256(r.SourceRevision) {
		return errors.New("question record has invalid sequence, task, time, or source identity")
	}
	if err := r.Question.Validate(); err != nil {
		return err
	}
	if r.Question.TaskID != taskID {
		return errors.New("question record has wrong task identity")
	}
	if err := r.Decision.Validate(); err != nil {
		return err
	}
	if r.Decision.TaskID != taskID || r.Decision.Action != ActionNeedsInput || r.Decision.WorkerProfile != "" {
		return errors.New("question record decision is not an exact needs_input decision")
	}
	if r.ResumeLifecycle != LifecycleStatePending && r.ResumeLifecycle != LifecycleStateReady {
		return errors.New("question record resume_lifecycle must be pending or ready")
	}
	if r.Supersedes != nil {
		return r.Supersedes.Validate()
	}
	return nil
}

func (p AnswerProvenance) Validate() error {
	if p.Kind != AnswerProvenanceOperator || strings.TrimSpace(p.Actor) == "" || p.Actor != strings.TrimSpace(p.Actor) {
		return errors.New("answer provenance requires normalized operator actor")
	}
	return validateOptionalEvidenceReferences("answer provenance evidence", p.Evidence)
}

func (r InputAnswerRecord) Validate(taskID string) error {
	if r.Sequence < 1 || !validFindingID(r.AnswerID) || r.TaskID != taskID || !validFindingID(r.OptionID) || r.AnsweredAt.IsZero() {
		return errors.New("answer record has invalid sequence, answer, task, option, or time")
	}
	if err := r.Question.Validate(); err != nil {
		return err
	}
	return r.Provenance.Validate()
}

func (r InputResumeRecord) Validate(taskID string) error {
	if r.Sequence < 1 || !validFindingID(r.ResumeID) || r.TaskID != taskID || !validFindingID(r.AnswerID) || r.ResumedAt.IsZero() {
		return errors.New("resume record has invalid sequence, resume, task, answer, or time")
	}
	return r.Question.Validate()
}

func (s InputState) Validate(taskID string) error {
	if s.TransitionSequence < 0 {
		return errors.New("input transition_sequence cannot be negative")
	}
	sequences := make(map[int64]struct{}, s.TransitionSequence)
	addSequence := func(sequence int64) error {
		if sequence < 1 || sequence > s.TransitionSequence {
			return errors.New("input evidence sequence is outside transition_sequence")
		}
		if _, ok := sequences[sequence]; ok {
			return errors.New("input evidence sequence is duplicated")
		}
		sequences[sequence] = struct{}{}
		return nil
	}
	questions := make(map[QuestionIdentity]InputQuestionRecord, len(s.Questions))
	questionRevisions := make(map[string]struct{}, len(s.Questions))
	latestRevision := make(map[string]int64, len(s.Questions))
	questionDecisions := make(map[string]DecisionReference, len(s.Questions))
	for i, record := range s.Questions {
		if err := record.Validate(taskID); err != nil {
			return fmt.Errorf("questions[%d]: %w", i, err)
		}
		if err := addSequence(record.Sequence); err != nil {
			return fmt.Errorf("questions[%d]: %w", i, err)
		}
		if i > 0 && record.Sequence <= s.Questions[i-1].Sequence {
			return fmt.Errorf("questions[%d] sequence is not increasing", i)
		}
		identity := record.Question.Identity()
		if _, ok := questions[identity]; ok {
			return fmt.Errorf("questions[%d] duplicates question identity", i)
		}
		if i == 0 && record.Supersedes != nil {
			return errors.New("first question must not supersede another question")
		}
		if i > 0 {
			previous := s.Questions[i-1].Question.Identity()
			if record.Supersedes == nil || *record.Supersedes != previous {
				return fmt.Errorf("questions[%d] must supersede the immediately previous question", i)
			}
		}
		questions[identity] = record
		revisionKey := fmt.Sprintf("%s/%d", identity.QuestionID, identity.Revision)
		if _, ok := questionRevisions[revisionKey]; ok {
			return fmt.Errorf("questions[%d] reuses question_id/revision with changed content", i)
		}
		questionRevisions[revisionKey] = struct{}{}
		priorRevision := latestRevision[identity.QuestionID]
		if priorRevision != 0 && identity.Revision != priorRevision+1 {
			return fmt.Errorf("questions[%d] revision for question_id %q is not consecutive", i, identity.QuestionID)
		}
		latestRevision[identity.QuestionID] = identity.Revision
		if prior, ok := questionDecisions[record.Decision.DecisionID]; ok && prior != record.Decision {
			return fmt.Errorf("questions[%d] reuses decision_id with changed provenance", i)
		}
		if _, ok := questionDecisions[record.Decision.DecisionID]; ok {
			return fmt.Errorf("questions[%d] replays decision_id %q", i, record.Decision.DecisionID)
		}
		questionDecisions[record.Decision.DecisionID] = record.Decision
	}
	answers := make(map[QuestionIdentity]InputAnswerRecord, len(s.Answers))
	answerIDs := make(map[string]struct{}, len(s.Answers))
	for i, answer := range s.Answers {
		if err := answer.Validate(taskID); err != nil {
			return fmt.Errorf("answers[%d]: %w", i, err)
		}
		if err := addSequence(answer.Sequence); err != nil {
			return fmt.Errorf("answers[%d]: %w", i, err)
		}
		if i > 0 && answer.Sequence <= s.Answers[i-1].Sequence {
			return fmt.Errorf("answers[%d] sequence is not increasing", i)
		}
		if _, ok := questions[answer.Question]; !ok {
			return fmt.Errorf("answers[%d] references unknown question", i)
		}
		if _, ok := answers[answer.Question]; ok {
			return fmt.Errorf("answers[%d] contradicts an existing answer", i)
		}
		if _, ok := answerIDs[answer.AnswerID]; ok {
			return fmt.Errorf("answers[%d] duplicates answer_id", i)
		}
		questionRecord := questions[answer.Question]
		if answer.Sequence <= questionRecord.Sequence {
			return fmt.Errorf("answers[%d] precedes its question", i)
		}
		for _, later := range s.Questions {
			if later.Sequence > questionRecord.Sequence && answer.Sequence > later.Sequence {
				return fmt.Errorf("answers[%d] is stale after a newer question", i)
			}
		}
		question := questionRecord.Question
		found := false
		for _, option := range question.Options {
			if option.ID == answer.OptionID {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("answers[%d] selects unknown option", i)
		}
		answers[answer.Question] = answer
		answerIDs[answer.AnswerID] = struct{}{}
	}
	resumed := make(map[QuestionIdentity]struct{}, len(s.Resumes))
	for i, resume := range s.Resumes {
		if err := resume.Validate(taskID); err != nil {
			return fmt.Errorf("resumes[%d]: %w", i, err)
		}
		if err := addSequence(resume.Sequence); err != nil {
			return fmt.Errorf("resumes[%d]: %w", i, err)
		}
		if i > 0 && resume.Sequence <= s.Resumes[i-1].Sequence {
			return fmt.Errorf("resumes[%d] sequence is not increasing", i)
		}
		answer, ok := answers[resume.Question]
		if !ok || answer.AnswerID != resume.AnswerID {
			return fmt.Errorf("resumes[%d] does not reference the exact answer", i)
		}
		if resume.Sequence <= answer.Sequence {
			return fmt.Errorf("resumes[%d] precedes its answer", i)
		}
		if _, ok := resumed[resume.Question]; ok {
			return fmt.Errorf("resumes[%d] duplicates a resume", i)
		}
		resumed[resume.Question] = struct{}{}
	}
	if s.TransitionSequence != int64(len(sequences)) {
		return errors.New("input transition_sequence does not match append-only lifecycle evidence")
	}
	return nil
}

func (d NeedsInputDetail) Validate() error {
	legacy := strings.TrimSpace(d.Reason) != ""
	typed := d.CurrentQuestion != nil
	if !legacy && !typed {
		return errors.New("validate needs_input detail: reason is required for legacy state or typed current_question is required")
	}
	if legacy && typed {
		return errors.New("validate needs_input detail: legacy reason and typed current_question are mutually exclusive")
	}
	if legacy && d.Reason != strings.TrimSpace(d.Reason) {
		return errors.New("validate needs_input detail: legacy reason must be normalized")
	}
	if typed {
		return d.CurrentQuestion.Validate()
	}
	return nil
}

func (q NeedsInputQuestion) Identity() QuestionIdentity {
	return QuestionIdentity{QuestionID: q.QuestionID, Revision: q.Revision, ContentSHA256: q.ContentSHA256}
}

func (d TerminalDetail) Validate() error {
	if strings.TrimSpace(d.Reason) == "" {
		return errors.New("validate terminal detail: reason is required")
	}
	if err := validateOptionalEvidenceReferences("validate terminal detail: evidence", d.Evidence); err != nil {
		return err
	}
	return nil
}

func (a FinalizationArtifact) Validate() error {
	if strings.TrimSpace(a.Path) == "" || filepath.IsAbs(a.Path) || filepath.Clean(a.Path) != a.Path || a.Path == ".." || strings.HasPrefix(a.Path, ".."+string(filepath.Separator)) {
		return errors.New("finalization artifact path must be canonical and repository-relative")
	}
	if !validStateSHA256(a.SHA256) || a.ByteSize <= 0 {
		return errors.New("finalization artifact requires a valid SHA-256 and positive byte size")
	}
	return nil
}

func (d FinalizationDetail) Validate() error {
	if d.SchemaVersion != FinalizationDetailSchemaVersion || strings.TrimSpace(d.OperationID) == "" || strings.TrimSpace(d.RunID) == "" {
		return errors.New("validate finalization detail: schema, operation_id, and run_id are required")
	}
	if _, ok := finalizationStageOrder(d.Stage); !ok {
		return fmt.Errorf("validate finalization detail: unknown stage %q", d.Stage)
	}
	if err := d.FrozenEvidence.Validate(); err != nil {
		return fmt.Errorf("validate finalization detail: frozen evidence: %w", err)
	}
	if !validStateSHA256(d.OriginalTaskSHA256) || d.AdmittedAt.IsZero() {
		return errors.New("validate finalization detail: original task identity and admitted_at are required")
	}
	stage, _ := finalizationStageOrder(d.Stage)
	materialized, _ := finalizationStageOrder(FinalizationStageMaterialized)
	taskCompleted, _ := finalizationStageOrder(FinalizationStageTaskCompleted)
	stateCompleted, _ := finalizationStageOrder(FinalizationStageStateCompleted)
	ledgerCompleted, _ := finalizationStageOrder(FinalizationStageLedgerCompleted)
	if stage >= materialized {
		if d.Capsule == nil || d.Manifest == nil || d.MaterializedAt == nil {
			return errors.New("validate finalization detail: materialized stage requires capsule, manifest, and time")
		}
		if err := d.Capsule.Validate(); err != nil {
			return err
		}
		if err := d.Manifest.Validate(); err != nil {
			return err
		}
	} else if d.Capsule != nil || d.Manifest != nil || d.MaterializedAt != nil {
		return errors.New("validate finalization detail: admitted stage cannot claim materialized artifacts")
	}
	if stage >= taskCompleted {
		if !validStateSHA256(d.CompletedTaskSHA256) || d.TaskCompletedAt == nil {
			return errors.New("validate finalization detail: task-completed stage requires completed task identity and time")
		}
	} else if d.CompletedTaskSHA256 != "" || d.TaskCompletedAt != nil {
		return errors.New("validate finalization detail: task completion evidence precedes its stage")
	}
	if stage >= stateCompleted {
		if d.StateCompletedAt == nil {
			return errors.New("validate finalization detail: state-completed stage requires time")
		}
	} else if d.StateCompletedAt != nil {
		return errors.New("validate finalization detail: state completion time precedes its stage")
	}
	if stage >= ledgerCompleted {
		if d.LedgerCompletedAt == nil {
			return errors.New("validate finalization detail: ledger-completed stage requires time")
		}
	} else if d.LedgerCompletedAt != nil {
		return errors.New("validate finalization detail: ledger completion time precedes its stage")
	}
	times := []*time.Time{&d.AdmittedAt, d.MaterializedAt, d.TaskCompletedAt, d.StateCompletedAt, d.LedgerCompletedAt}
	var prior time.Time
	for _, current := range times {
		if current == nil {
			continue
		}
		if !prior.IsZero() && current.Before(prior) {
			return errors.New("validate finalization detail: stage times are not monotonic")
		}
		prior = *current
	}
	return nil
}

func (s ExecutionState) Validate() error {
	if s.SchemaVersion != ExecutionStateSchemaVersion {
		return fmt.Errorf("validate execution state: unsupported schema_version %q (want %q)", s.SchemaVersion, ExecutionStateSchemaVersion)
	}
	if strings.TrimSpace(s.TaskID) == "" {
		return errors.New("validate execution state: task_id is required")
	}
	if !validLifecycleState(s.Lifecycle) {
		return fmt.Errorf("validate execution state: unknown lifecycle %q", s.Lifecycle)
	}
	if s.ChildOf != nil {
		if err := s.ChildOf.Validate(); err != nil {
			return fmt.Errorf("validate execution state: child_of: %w", err)
		}
		if s.ChildOf.ParentTaskID == s.TaskID {
			return errors.New("validate execution state: child parent must differ from task_id")
		}
	}
	if s.Plan != nil {
		if err := s.Plan.Validate(); err != nil {
			return fmt.Errorf("validate execution state: plan: %w", err)
		}
		if strings.TrimSpace(s.Plan.TaskID) != strings.TrimSpace(s.TaskID) {
			return fmt.Errorf("validate execution state: plan task_id %q does not match state task_id %q", s.Plan.TaskID, s.TaskID)
		}
	}

	criterionIDs := make(map[string]struct{}, len(s.AcceptanceCriteria))
	for i, criterion := range s.AcceptanceCriteria {
		if err := criterion.Validate(); err != nil {
			return fmt.Errorf("validate execution state: acceptance_criteria[%d]: %w", i, err)
		}
		if _, exists := criterionIDs[criterion.ID]; exists {
			return fmt.Errorf("validate execution state: acceptance_criteria[%d]: duplicate criterion id %q", i, criterion.ID)
		}
		criterionIDs[criterion.ID] = struct{}{}
	}

	decisionIDs := make(map[string]DecisionReference, len(s.FindingResolutions)+1)
	findingIDs := make(map[string]struct{}, len(s.FindingResolutions))
	for i, resolution := range s.FindingResolutions {
		if err := resolution.Validate(); err != nil {
			return fmt.Errorf("validate execution state: finding_resolutions[%d]: %w", i, err)
		}
		if _, exists := findingIDs[resolution.FindingID]; exists {
			return fmt.Errorf("validate execution state: finding_resolutions[%d]: duplicate finding_id %q", i, resolution.FindingID)
		}
		findingIDs[resolution.FindingID] = struct{}{}
		if resolution.Resolution != nil && strings.TrimSpace(resolution.Resolution.TaskID) != strings.TrimSpace(s.TaskID) {
			return fmt.Errorf("validate execution state: finding_resolutions[%d] resolution task_id %q does not match state task_id %q", i, resolution.Resolution.TaskID, s.TaskID)
		}
		if resolution.Resolution != nil {
			if prior, exists := decisionIDs[resolution.Resolution.DecisionID]; exists && prior != *resolution.Resolution {
				return fmt.Errorf("validate execution state: finding_resolutions[%d] decision_id %q is reused for a materially different reference", i, resolution.Resolution.DecisionID)
			}
			decisionIDs[resolution.Resolution.DecisionID] = *resolution.Resolution
		}
	}
	for i, occurrence := range s.OptionalRoles {
		if err := occurrence.Validate(); err != nil {
			return fmt.Errorf("validate execution state: optional_role_occurrences[%d]: %w", i, err)
		}
		if occurrence.TaskID != s.TaskID {
			return fmt.Errorf("validate execution state: optional_role_occurrences[%d] has wrong task_id %q", i, occurrence.TaskID)
		}
		if occurrence.Sequence != int64(i+1) {
			return fmt.Errorf("validate execution state: optional_role_occurrences[%d] sequence %d is not canonical", i, occurrence.Sequence)
		}
	}

	if s.LatestDecision != nil {
		if err := s.LatestDecision.Validate(); err != nil {
			return fmt.Errorf("validate execution state: latest_decision: %w", err)
		}
		if strings.TrimSpace(s.LatestDecision.TaskID) != strings.TrimSpace(s.TaskID) {
			return fmt.Errorf("validate execution state: latest_decision task_id %q does not match state task_id %q", s.LatestDecision.TaskID, s.TaskID)
		}
		if prior, exists := decisionIDs[s.LatestDecision.DecisionID]; exists && prior != *s.LatestDecision {
			return fmt.Errorf("validate execution state: latest_decision decision_id %q is reused for a materially different reference", s.LatestDecision.DecisionID)
		}
	}
	if err := s.Attempts.Validate(); err != nil {
		return fmt.Errorf("validate execution state: attempts: %w", err)
	}
	for i, event := range s.Attempts.Events {
		if err := event.Validate(s.TaskID); err != nil {
			return fmt.Errorf("validate execution state: attempts events[%d]: %w", i, err)
		}
	}
	if err := s.Input.Validate(s.TaskID); err != nil {
		return fmt.Errorf("validate execution state: input: %w", err)
	}
	if s.Workspace != nil {
		if err := s.Workspace.Validate(); err != nil {
			return fmt.Errorf("validate execution state: workspace: %w", err)
		}
		if s.Workspace.TaskID != s.TaskID {
			return errors.New("validate execution state: workspace task identity mismatch")
		}
	}
	if s.ReopenedFrom != nil {
		if err := s.ReopenedFrom.Validate(); err != nil {
			return fmt.Errorf("validate execution state: reopened_from: %w", err)
		}
		if s.ReopenedFrom.ArchivedTaskID == s.TaskID {
			return errors.New("validate execution state: reopen must create a new task identity")
		}
	}
	if s.Finalization != nil {
		if err := s.Finalization.Validate(); err != nil {
			return fmt.Errorf("validate execution state: finalization: %w", err)
		}
		if s.Lifecycle != LifecycleStateFinalizing && s.Lifecycle != LifecycleStateCompleted {
			return fmt.Errorf("validate execution state: lifecycle %q cannot include finalization detail", s.Lifecycle)
		}
	} else if s.Lifecycle == LifecycleStateFinalizing {
		return errors.New("validate execution state: finalizing lifecycle requires finalization detail")
	}

	if s.Lifecycle == LifecycleStateNeedsInput {
		if s.NeedsInput == nil {
			return errors.New("validate execution state: needs_input lifecycle requires needs_input detail")
		}
		if err := s.NeedsInput.Validate(); err != nil {
			return fmt.Errorf("validate execution state: needs_input: %w", err)
		}
		if s.NeedsInput.CurrentQuestion != nil {
			if len(s.Input.Questions) == 0 || s.Input.Questions[len(s.Input.Questions)-1].Question.Identity() != *s.NeedsInput.CurrentQuestion {
				return errors.New("validate execution state: current question does not match latest durable question")
			}
			if s.LatestDecision == nil || *s.LatestDecision != s.Input.Questions[len(s.Input.Questions)-1].Decision {
				return errors.New("validate execution state: current question provenance does not match latest_decision")
			}
			for _, answer := range s.Input.Answers {
				if answer.Question == *s.NeedsInput.CurrentQuestion {
					for _, resume := range s.Input.Resumes {
						if resume.Question == answer.Question {
							return errors.New("validate execution state: resumed question cannot remain current")
						}
					}
				}
			}
		} else if len(s.Input.Questions) != 0 {
			return errors.New("validate execution state: legacy needs_input detail cannot carry typed question authority")
		}
	} else if s.NeedsInput != nil {
		return fmt.Errorf("validate execution state: lifecycle %q must not include needs_input detail", s.Lifecycle)
	}
	if s.CircuitBreaker != nil {
		if err := s.CircuitBreaker.Validate(); err != nil {
			return fmt.Errorf("validate execution state: circuit_breaker: %w", err)
		}
		if s.Lifecycle != LifecycleStateBlocked && s.Lifecycle != LifecycleStateNeedsInput {
			return fmt.Errorf("validate execution state: circuit_breaker requires blocked or needs_input lifecycle (got %q)", s.Lifecycle)
		}
	} else if s.Lifecycle == LifecycleStateBlocked {
		// Legacy blocked snapshots remain compatible; AW-15-created blocks always
		// carry typed circuit-breaker evidence.
	}

	if terminalLifecycleState(s.Lifecycle) {
		if s.Terminal == nil {
			return fmt.Errorf("validate execution state: terminal lifecycle %q requires terminal detail", s.Lifecycle)
		}
		if err := s.Terminal.Validate(); err != nil {
			return fmt.Errorf("validate execution state: terminal: %w", err)
		}
	} else if s.Terminal != nil {
		return fmt.Errorf("validate execution state: nonterminal lifecycle %q must not include terminal detail", s.Lifecycle)
	}

	if s.Lifecycle == LifecycleStateCompleted {
		if s.Finalization != nil {
			stage, _ := finalizationStageOrder(s.Finalization.Stage)
			required, _ := finalizationStageOrder(FinalizationStageStateCompleted)
			if stage < required {
				return errors.New("validate execution state: completed lifecycle requires state-completed finalization stage")
			}
		}
		if s.Plan == nil {
			return errors.New("validate execution state: completed lifecycle requires a plan")
		}
		if !s.Plan.Completed {
			return errors.New("validate execution state: completed lifecycle requires a completed plan")
		}
		if len(s.AcceptanceCriteria) == 0 {
			return errors.New("validate execution state: completed lifecycle requires acceptance criteria")
		}
		for i, criterion := range s.AcceptanceCriteria {
			if criterion.Status == AcceptanceStatusPending {
				return fmt.Errorf("validate execution state: completed lifecycle has pending acceptance_criteria[%d] %q", i, criterion.ID)
			}
		}
		for i, resolution := range s.FindingResolutions {
			if resolution.Status == FindingResolutionStatusOpen {
				return fmt.Errorf("validate execution state: completed lifecycle has open finding_resolutions[%d] %q", i, resolution.FindingID)
			}
		}
		if s.LatestDecision == nil || s.LatestDecision.Action != ActionComplete {
			return errors.New("validate execution state: completed lifecycle requires latest_decision action complete")
		}
	}
	return nil
}

// ValidateExecutionStateTransition enforces only identity preservation and
// irreversible item/terminal states. Legal action ordering, completion gates,
// evidence freshness, and reopen policy belong to AW-09 and later runtime work.
func ValidateExecutionStateTransition(previous, next ExecutionState) error {
	if err := previous.Validate(); err != nil {
		return fmt.Errorf("validate execution state transition: previous: %w", err)
	}
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate execution state transition: next: %w", err)
	}
	if strings.TrimSpace(previous.TaskID) != strings.TrimSpace(next.TaskID) {
		return fmt.Errorf("validate execution state transition: task_id changed from %q to %q", previous.TaskID, next.TaskID)
	}
	if previous.SchemaVersion != next.SchemaVersion {
		return fmt.Errorf("validate execution state transition: schema_version changed from %q to %q", previous.SchemaVersion, next.SchemaVersion)
	}
	if terminalLifecycleState(previous.Lifecycle) && next.Lifecycle != previous.Lifecycle {
		return fmt.Errorf("validate execution state transition: terminal lifecycle %q cannot transition to %q without an explicit reopen operation", previous.Lifecycle, next.Lifecycle)
	}
	if err := validatePlanTransition(previous.Plan, next.Plan); err != nil {
		return fmt.Errorf("validate execution state transition: plan: %w", err)
	}
	if err := validateAcceptanceTransitions(previous.AcceptanceCriteria, next.AcceptanceCriteria); err != nil {
		return fmt.Errorf("validate execution state transition: acceptance_criteria: %w", err)
	}
	if err := validateFindingResolutionTransitions(previous.FindingResolutions, next.FindingResolutions); err != nil {
		return fmt.Errorf("validate execution state transition: finding_resolutions: %w", err)
	}
	if len(next.OptionalRoles) < len(previous.OptionalRoles) {
		return errors.New("validate execution state transition: optional-role occurrences must not disappear")
	}
	for i := range previous.OptionalRoles {
		if !reflect.DeepEqual(previous.OptionalRoles[i], next.OptionalRoles[i]) {
			return fmt.Errorf("validate execution state transition: optional-role occurrence %d was rewritten", i+1)
		}
	}
	if previous.LatestDecision != nil {
		if next.LatestDecision == nil {
			return errors.New("validate execution state transition: latest_decision must not disappear")
		}
		if previous.LatestDecision.DecisionID == next.LatestDecision.DecisionID && *previous.LatestDecision != *next.LatestDecision {
			return fmt.Errorf("validate execution state transition: decision_id %q was reused for a materially different reference", previous.LatestDecision.DecisionID)
		}
	}
	if err := validateAttemptTransition(previous.Attempts, next.Attempts); err != nil {
		return fmt.Errorf("validate execution state transition: attempts: %w", err)
	}
	if err := validateInputTransition(previous.Input, next.Input); err != nil {
		return fmt.Errorf("validate execution state transition: input: %w", err)
	}
	if previous.Workspace != nil && next.Workspace == nil {
		return errors.New("validate execution state transition: workspace evidence must not disappear")
	}
	if previous.Workspace != nil && next.Workspace != nil {
		if previous.Workspace.WorkspaceID != next.Workspace.WorkspaceID || previous.Workspace.ControlRoot != next.Workspace.ControlRoot || previous.Workspace.ExecutionRoot != next.Workspace.ExecutionRoot || previous.Workspace.GitCommonDir != next.Workspace.GitCommonDir || previous.Workspace.BranchRef != next.Workspace.BranchRef || previous.Workspace.OwnerMarker != next.Workspace.OwnerMarker || previous.Workspace.BaselineSHA != next.Workspace.BaselineSHA {
			return errors.New("validate execution state transition: immutable workspace ownership changed")
		}
		if next.Workspace.Checkpoint.Sequence < previous.Workspace.Checkpoint.Sequence || len(next.Workspace.RetainedRefs) < len(previous.Workspace.RetainedRefs) {
			return errors.New("validate execution state transition: workspace checkpoint or retained evidence regressed")
		}
		for i := range previous.Workspace.RetainedRefs {
			if previous.Workspace.RetainedRefs[i] != next.Workspace.RetainedRefs[i] {
				return errors.New("validate execution state transition: retained workspace evidence was rewritten")
			}
		}
	}
	if previous.CircuitBreaker != nil && next.CircuitBreaker == nil {
		return errors.New("validate execution state transition: circuit_breaker must not disappear")
	}
	if err := validateFinalizationTransition(previous.Finalization, next.Finalization); err != nil {
		return fmt.Errorf("validate execution state transition: finalization: %w", err)
	}
	if previous.ReopenedFrom != nil && !reflect.DeepEqual(previous.ReopenedFrom, next.ReopenedFrom) {
		return errors.New("validate execution state transition: reopen lineage was rewritten or removed")
	}
	if previous.ChildOf != nil && !reflect.DeepEqual(previous.ChildOf, next.ChildOf) {
		return errors.New("validate execution state transition: child lineage was rewritten or removed")
	}
	if previous.ChildOf == nil && next.ChildOf != nil {
		return errors.New("validate execution state transition: child lineage cannot be attached after initial state creation")
	}
	return nil
}

func validateFinalizationTransition(previous, next *FinalizationDetail) error {
	if previous == nil {
		return nil
	}
	if next == nil {
		return errors.New("finalization detail must not disappear")
	}
	if previous.SchemaVersion != next.SchemaVersion || previous.OperationID != next.OperationID || previous.RunID != next.RunID || previous.FrozenEvidence != next.FrozenEvidence || previous.OriginalTaskSHA256 != next.OriginalTaskSHA256 || !previous.AdmittedAt.Equal(next.AdmittedAt) {
		return errors.New("immutable finalization authority changed")
	}
	before, _ := finalizationStageOrder(previous.Stage)
	after, _ := finalizationStageOrder(next.Stage)
	if after < before {
		return errors.New("finalization stage regressed")
	}
	if previous.Capsule != nil && !reflect.DeepEqual(previous.Capsule, next.Capsule) {
		return errors.New("capsule identity was rewritten")
	}
	if previous.Manifest != nil && !reflect.DeepEqual(previous.Manifest, next.Manifest) {
		return errors.New("manifest identity was rewritten")
	}
	if previous.CompletedTaskSHA256 != "" && previous.CompletedTaskSHA256 != next.CompletedTaskSHA256 {
		return errors.New("completed task identity was rewritten")
	}
	for _, pair := range [][2]*time.Time{{previous.MaterializedAt, next.MaterializedAt}, {previous.TaskCompletedAt, next.TaskCompletedAt}, {previous.StateCompletedAt, next.StateCompletedAt}, {previous.LedgerCompletedAt, next.LedgerCompletedAt}} {
		if pair[0] != nil && (pair[1] == nil || !pair[0].Equal(*pair[1])) {
			return errors.New("finalization stage timestamp was rewritten")
		}
	}
	return nil
}

func finalizationStageOrder(stage FinalizationStage) (int, bool) {
	switch stage {
	case FinalizationStageAdmitted:
		return 1, true
	case FinalizationStageMaterialized:
		return 2, true
	case FinalizationStageTaskCompleted:
		return 3, true
	case FinalizationStageStateCompleted:
		return 4, true
	case FinalizationStageLedgerCompleted:
		return 5, true
	default:
		return 0, false
	}
}

func validateInputTransition(previous, next InputState) error {
	if next.TransitionSequence < previous.TransitionSequence || len(next.Questions) < len(previous.Questions) || len(next.Answers) < len(previous.Answers) || len(next.Resumes) < len(previous.Resumes) {
		return errors.New("input lifecycle evidence must not disappear")
	}
	for i := range previous.Questions {
		if !reflect.DeepEqual(previous.Questions[i], next.Questions[i]) {
			return fmt.Errorf("question %d was rewritten", i+1)
		}
	}
	for i := range previous.Answers {
		if !reflect.DeepEqual(previous.Answers[i], next.Answers[i]) {
			return fmt.Errorf("answer %d was rewritten", i+1)
		}
	}
	for i := range previous.Resumes {
		if !reflect.DeepEqual(previous.Resumes[i], next.Resumes[i]) {
			return fmt.Errorf("resume %d was rewritten", i+1)
		}
	}
	return nil
}

func validateAttemptTransition(previous, next AttemptState) error {
	if next.TotalAttempts < previous.TotalAttempts || next.ConsecutiveFailures < 0 {
		return errors.New("attempt counters must not decrease")
	}
	if next.TransitionSequence < previous.TransitionSequence {
		return errors.New("attempt transition sequence must not decrease")
	}
	if next.RetryBudget.Consumed < previous.RetryBudget.Consumed || next.ElapsedTimeBudget.Consumed < previous.ElapsedTimeBudget.Consumed || next.TokenBudget.Consumed < previous.TokenBudget.Consumed {
		return errors.New("budget consumption must not decrease")
	}
	if previous.TotalAttempts > 0 || len(previous.Events) > 0 {
		if previous.RetryBudget.Mode != next.RetryBudget.Mode || previous.RetryBudget.Limit != next.RetryBudget.Limit || previous.ElapsedTimeBudget.Mode != next.ElapsedTimeBudget.Mode || previous.ElapsedTimeBudget.Limit != next.ElapsedTimeBudget.Limit || previous.TokenBudget.Mode != next.TokenBudget.Mode || previous.TokenBudget.Limit != next.TokenBudget.Limit || previous.RepeatedSignatureLimit != next.RepeatedSignatureLimit {
			return errors.New("authoritative attempt limits must not change after admission")
		}
	}
	previousActions := make(map[Action]int64, len(previous.ActionAttempts))
	for _, counter := range previous.ActionAttempts {
		previousActions[counter.Action] = counter.Attempts
	}
	for _, counter := range next.ActionAttempts {
		if counter.Attempts < previousActions[counter.Action] {
			return fmt.Errorf("action %q attempt counter decreased", counter.Action)
		}
	}
	previousBudgets := append([]ActionBudget(nil), previous.ActionBudgets...)
	sort.Slice(previousBudgets, func(i, j int) bool {
		return previousBudgets[i].Action < previousBudgets[j].Action
	})
	for _, previousBudget := range previousBudgets {
		action, prior := previousBudget.Action, previousBudget.Budget
		found := false
		for _, current := range next.ActionBudgets {
			if current.Action != action {
				continue
			}
			found = true
			if prior.Mode != current.Budget.Mode || prior.Limit != current.Budget.Limit || current.Budget.Consumed < prior.Consumed {
				return fmt.Errorf("action %q budget authority changed or consumption decreased", action)
			}
		}
		if !found {
			return fmt.Errorf("action %q budget disappeared", action)
		}
	}
	if len(next.Events) < len(previous.Events) {
		return errors.New("attempt events must not disappear")
	}
	for i := range previous.Events {
		if !reflect.DeepEqual(previous.Events[i], next.Events[i]) {
			return fmt.Errorf("attempt event %d was rewritten", i+1)
		}
	}
	if len(next.ActionStops) < len(previous.ActionStops) {
		return errors.New("action stops must not disappear")
	}
	for i := range previous.ActionStops {
		if !reflect.DeepEqual(previous.ActionStops[i], next.ActionStops[i]) {
			return fmt.Errorf("action stop %d was rewritten", i+1)
		}
	}
	if previous.LastFailure != nil && next.LastFailure == nil {
		return errors.New("last_failure must not disappear")
	}
	return nil
}

func validatePlanTransition(previous, next *TaskPlan) error {
	if previous == nil {
		return nil
	}
	if next == nil {
		return errors.New("current plan must not disappear")
	}
	if previous.ID == next.ID {
		if previous.Revision != next.Revision {
			return fmt.Errorf("plan id %q was reused with revision %d instead of %d", previous.ID, next.Revision, previous.Revision)
		}
		if previous.SupersedesPlanID != next.SupersedesPlanID || !equalEvidenceReferences(previous.Provenance, next.Provenance) {
			return fmt.Errorf("plan id %q was reused with different revision provenance", previous.ID)
		}
		if len(previous.Steps) != len(next.Steps) {
			return fmt.Errorf("plan revision %q changed its step collection without a new revision id", previous.ID)
		}
		if previous.Completed && !next.Completed {
			return fmt.Errorf("completed plan revision %q cannot become incomplete", previous.ID)
		}
		for i, priorStep := range previous.Steps {
			nextStep := next.Steps[i]
			if priorStep.ID != nextStep.ID {
				return fmt.Errorf("plan revision %q changed step ordering at index %d from %q to %q", previous.ID, i, priorStep.ID, nextStep.ID)
			}
			if priorStep.Description != nextStep.Description {
				return fmt.Errorf("step id %q was reused with a different description", priorStep.ID)
			}
			if terminalPlanStepStatus(priorStep.Status) && nextStep.Status != priorStep.Status {
				return fmt.Errorf("terminal step %q changed status from %q to %q", priorStep.ID, priorStep.Status, nextStep.Status)
			}
		}
		return nil
	}

	if next.Revision != previous.Revision+1 {
		return fmt.Errorf("new plan revision %q has revision %d (want %d)", next.ID, next.Revision, previous.Revision+1)
	}
	if next.SupersedesPlanID != previous.ID {
		return fmt.Errorf("new plan revision %q supersedes %q (want %q)", next.ID, next.SupersedesPlanID, previous.ID)
	}
	nextSteps := make(map[string]PlanStep, len(next.Steps))
	for _, step := range next.Steps {
		nextSteps[step.ID] = step
	}
	for _, priorStep := range previous.Steps {
		nextStep, exists := nextSteps[priorStep.ID]
		if !exists {
			if terminalPlanStepStatus(priorStep.Status) {
				return fmt.Errorf("new plan revision dropped terminal step %q", priorStep.ID)
			}
			continue
		}
		if priorStep.Description != nextStep.Description {
			return fmt.Errorf("step id %q was reused with a different description", priorStep.ID)
		}
		if terminalPlanStepStatus(priorStep.Status) && nextStep.Status != priorStep.Status {
			return fmt.Errorf("terminal step %q changed status from %q to %q", priorStep.ID, priorStep.Status, nextStep.Status)
		}
	}
	return nil
}

func validateAcceptanceTransitions(previous, next []AcceptanceCriterion) error {
	nextByID := make(map[string]AcceptanceCriterion, len(next))
	for _, criterion := range next {
		nextByID[criterion.ID] = criterion
	}
	for _, priorCriterion := range previous {
		nextCriterion, exists := nextByID[priorCriterion.ID]
		if !exists {
			return fmt.Errorf("criterion %q must not disappear", priorCriterion.ID)
		}
		if priorCriterion.Requirement != nextCriterion.Requirement || !equalEvidenceReferencePointers(priorCriterion.Source, nextCriterion.Source) {
			return fmt.Errorf("criterion id %q was reused for a materially different requirement", priorCriterion.ID)
		}
		if priorCriterion.Status != AcceptanceStatusPending && nextCriterion.Status == AcceptanceStatusPending {
			return fmt.Errorf("criterion %q cannot return from %q to pending", priorCriterion.ID, priorCriterion.Status)
		}
	}
	return nil
}

func validateFindingResolutionTransitions(previous, next []FindingResolution) error {
	nextByID := make(map[string]FindingResolution, len(next))
	for _, resolution := range next {
		nextByID[resolution.FindingID] = resolution
	}
	for _, priorResolution := range previous {
		nextResolution, exists := nextByID[priorResolution.FindingID]
		if !exists {
			return fmt.Errorf("finding resolution %q must not disappear", priorResolution.FindingID)
		}
		if priorResolution.Status != FindingResolutionStatusOpen && nextResolution.Status != priorResolution.Status {
			return fmt.Errorf("finding %q cannot change terminal resolution from %q to %q", priorResolution.FindingID, priorResolution.Status, nextResolution.Status)
		}
		if priorResolution.Resolution != nil && nextResolution.Resolution != nil && priorResolution.Resolution.DecisionID == nextResolution.Resolution.DecisionID && *priorResolution.Resolution != *nextResolution.Resolution {
			return fmt.Errorf("finding %q reused decision_id %q for a materially different resolution reference", priorResolution.FindingID, priorResolution.Resolution.DecisionID)
		}
	}
	return nil
}

func validPlanStepStatus(status PlanStepStatus) bool {
	switch status {
	case PlanStepStatusPending, PlanStepStatusInProgress, PlanStepStatusCompleted, PlanStepStatusSkipped:
		return true
	default:
		return false
	}
}

func terminalPlanStepStatus(status PlanStepStatus) bool {
	return status == PlanStepStatusCompleted || status == PlanStepStatusSkipped
}

func validAcceptanceStatus(status AcceptanceStatus) bool {
	switch status {
	case AcceptanceStatusPending, AcceptanceStatusSatisfied, AcceptanceStatusWaived, AcceptanceStatusNotApplicable:
		return true
	default:
		return false
	}
}

func validFindingResolutionStatus(status FindingResolutionStatus) bool {
	switch status {
	case FindingResolutionStatusOpen, FindingResolutionStatusResolved, FindingResolutionStatusWaived, FindingResolutionStatusSuperseded, FindingResolutionStatusInvalid:
		return true
	default:
		return false
	}
}

func validBudgetMode(mode BudgetMode) bool {
	switch mode {
	case BudgetModeUnset, BudgetModeLimited, BudgetModeUnlimited:
		return true
	default:
		return false
	}
}

func validSignatureKind(kind SignatureKind) bool {
	switch kind {
	case SignatureKindDecision, SignatureKindVerificationFailure, SignatureKindOpenFindings, SignatureKindOperationFailure:
		return true
	default:
		return false
	}
}

func validAttemptOutcome(outcome AttemptOutcome) bool {
	switch outcome {
	case AttemptOutcomeSucceeded, AttemptOutcomeFailed, AttemptOutcomeNoProgress, AttemptOutcomeCancelled, AttemptOutcomeSafetyStopped:
		return true
	default:
		return false
	}
}

func validBreakerReason(reason BreakerReason) bool {
	switch reason {
	case BreakerTaskAttemptsExhausted, BreakerActionAttemptsExhausted, BreakerElapsedExhausted, BreakerTokenExhausted, BreakerRepeatedSignature, BreakerUnchangedSource, BreakerIdenticalStrategy, BreakerMissingTrustedMetrics, BreakerStaleEvidence, BreakerCancellation, BreakerUnsafeSource, BreakerAccountingSafety:
		return true
	default:
		return false
	}
}

func validStateSHA256(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func validLifecycleState(state LifecycleState) bool {
	switch state {
	case LifecycleStatePending, LifecycleStateReady, LifecycleStatePlanning, LifecycleStateWorking, LifecycleStateVerifying, LifecycleStateAuditing, LifecycleStateCorrecting, LifecycleStateNeedsInput, LifecycleStateFinalizing, LifecycleStateCompleted, LifecycleStateBlocked, LifecycleStateCancelled, LifecycleStateSuperseded, LifecycleStateAbandoned:
		return true
	default:
		return false
	}
}

func terminalLifecycleState(state LifecycleState) bool {
	switch state {
	case LifecycleStateCompleted, LifecycleStateBlocked, LifecycleStateCancelled, LifecycleStateSuperseded, LifecycleStateAbandoned:
		return true
	default:
		return false
	}
}

func validGitObjectID(value string) bool {
	return gitoid.Valid(value)
}

func validateOptionalEvidenceReferences(prefix string, references []EvidenceReference) error {
	if len(references) == 0 {
		return nil
	}
	return validateEvidenceReferences(prefix, references)
}

func equalEvidenceReferences(left, right []EvidenceReference) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func equalEvidenceReferencePointers(left, right *EvidenceReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
