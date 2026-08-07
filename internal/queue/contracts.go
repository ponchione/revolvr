// Package queue owns the manually started canonical sequential queue.
package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"revolvr/internal/scheduler"
)

const (
	OperationSchemaVersion = "revolvr-sequential-queue-operation-v1"
	ConfigSchemaVersion    = "revolvr-sequential-queue-config-v1"
	ResultSchemaVersion    = "revolvr-sequential-queue-result-v1"
	WorkerModeDirectTools  = "direct_tools_v1"
	MaximumWorkers         = 1
)

type FailurePoint string

const (
	FailureBeforeSelection    FailurePoint = "before_task_selection"
	FailureAfterSelection     FailurePoint = "after_task_selection"
	FailureBeforeWorkerEffect FailurePoint = "before_worker_effect"
	FailureAfterWorkerEffect  FailurePoint = "after_worker_effect"
	FailureBeforeCompletion   FailurePoint = "before_task_completion"
	FailureAfterCompletion    FailurePoint = "after_task_completion"
	FailureBeforeCheckpoint   FailurePoint = "before_queue_checkpoint"
	FailureAfterCheckpoint    FailurePoint = "after_queue_checkpoint"
)

type FailureInjector func(FailurePoint) error

type QualityGateStatus string

const (
	QualityGateDeterministicOnly QualityGateStatus = "deterministic_evaluation_only"
)

type Limits struct {
	MaximumTasks          int64         `json:"maximum_tasks"`
	MaximumCyclesPerTask  int64         `json:"maximum_cycles_per_task"`
	MaximumTotalCycles    int64         `json:"maximum_total_cycles"`
	MaximumRemoteTokens   int64         `json:"maximum_remote_tokens"`
	MaximumCostMicrousd   int64         `json:"maximum_cost_microusd"`
	MaximumDuration       time.Duration `json:"-"`
	MaximumDurationMillis int64         `json:"maximum_duration_milliseconds"`
}

func (l Limits) normalized() (Limits, error) {
	if l.MaximumDurationMillis == 0 && l.MaximumDuration > 0 {
		l.MaximumDurationMillis = l.MaximumDuration.Milliseconds()
	}
	if l.MaximumDuration == 0 && l.MaximumDurationMillis > 0 {
		l.MaximumDuration = time.Duration(l.MaximumDurationMillis) * time.Millisecond
	}
	if l.MaximumTasks <= 0 || l.MaximumCyclesPerTask <= 0 || l.MaximumTotalCycles <= 0 ||
		l.MaximumRemoteTokens <= 0 || l.MaximumCostMicrousd <= 0 ||
		l.MaximumDurationMillis <= 0 || l.MaximumDuration <= 0 {
		return Limits{}, errors.New("sequential queue: every task, cycle, token, cost, and duration limit must be finite and positive")
	}
	if l.MaximumCyclesPerTask > l.MaximumTotalCycles {
		return Limits{}, errors.New("sequential queue: per-task cycle limit exceeds the total cycle limit")
	}
	return l, nil
}

type PinnedConfiguration struct {
	SchemaVersion     string            `json:"schema_version"`
	WorkerMode        string            `json:"worker_mode"`
	MaximumWorkers    int               `json:"maximum_workers"`
	QualityGateStatus QualityGateStatus `json:"quality_gate_status"`
	Limits            Limits            `json:"limits"`
}

func NewPinnedConfiguration(limits Limits, gate QualityGateStatus) (PinnedConfiguration, string, []byte, error) {
	limits, err := limits.normalized()
	if err != nil {
		return PinnedConfiguration{}, "", nil, err
	}
	if gate != QualityGateDeterministicOnly {
		return PinnedConfiguration{}, "", nil, errors.New("sequential queue: invalid quality-gate status")
	}
	configuration := PinnedConfiguration{
		SchemaVersion: ConfigSchemaVersion, WorkerMode: WorkerModeDirectTools,
		MaximumWorkers: MaximumWorkers, QualityGateStatus: gate, Limits: limits,
	}
	raw, err := json.Marshal(configuration)
	if err != nil {
		return PinnedConfiguration{}, "", nil, err
	}
	return configuration, hashBytes(raw), raw, nil
}

type StopReason string

const (
	StopDrained             StopReason = "drained"
	StopWaitingDependencies StopReason = "waiting_on_dependencies"
	StopWaitingInput        StopReason = "waiting_on_input"
	StopAllRemainingBlocked StopReason = "all_remaining_blocked"
	StopBudgetExhausted     StopReason = "budget_exhausted"
	StopCancelled           StopReason = "cancelled"
	StopUnsafe              StopReason = "unsafe"
	StopSystemFailure       StopReason = "system_failure"
)

func (r StopReason) valid() bool {
	switch r {
	case StopDrained, StopWaitingDependencies, StopWaitingInput,
		StopAllRemainingBlocked, StopBudgetExhausted, StopCancelled,
		StopUnsafe, StopSystemFailure:
		return true
	default:
		return false
	}
}

type TaskOutcome string

const (
	OutcomeCompleted           TaskOutcome = "completed"
	OutcomeBlocked             TaskOutcome = "blocked"
	OutcomeNeedsInput          TaskOutcome = "needs_input"
	OutcomeDependencyWaiting   TaskOutcome = "dependency_waiting"
	OutcomeTaskBudgetExhausted TaskOutcome = "task_budget_exhausted"
	OutcomeCancelled           TaskOutcome = "cancelled"
	OutcomeUnsafe              TaskOutcome = "unsafe"
	OutcomeSystemFailure       TaskOutcome = "system_failure"
)

func (o TaskOutcome) valid() bool {
	switch o {
	case OutcomeCompleted, OutcomeBlocked, OutcomeNeedsInput,
		OutcomeDependencyWaiting, OutcomeTaskBudgetExhausted,
		OutcomeCancelled, OutcomeUnsafe, OutcomeSystemFailure:
		return true
	default:
		return false
	}
}

func (o TaskOutcome) queueStop() StopReason {
	switch o {
	case OutcomeCancelled:
		return StopCancelled
	case OutcomeUnsafe:
		return StopUnsafe
	case OutcomeSystemFailure:
		return StopSystemFailure
	default:
		return ""
	}
}

type EffectKind string

const (
	EffectSupervisor   EffectKind = "supervisor"
	EffectWorker       EffectKind = "worker"
	EffectVerification EffectKind = "verification"
	EffectAudit        EffectKind = "audit"
	EffectCorrection   EffectKind = "correction"
	EffectCompletion   EffectKind = "completion"
)

func (k EffectKind) valid() bool {
	switch k {
	case EffectSupervisor, EffectWorker, EffectVerification, EffectAudit,
		EffectCorrection, EffectCompletion:
		return true
	default:
		return false
	}
}

type EffectAdmission struct {
	Sequence  int64
	Completed bool
	Replayed  bool
	Evidence  string
}

type EffectRecorder interface {
	PersistIntent(ctx context.Context, kind EffectKind, identity, materialSHA256 string) (EffectAdmission, error)
	PersistCompletion(ctx context.Context, kind EffectKind, identity, materialSHA256, evidenceSHA256 string) (EffectAdmission, error)
}

type TaskRequest struct {
	QueueOperationID      string
	OccurrenceID          string
	OccurrenceSequence    int64
	SchedulerRunID        string
	CoordinatorIdentity   string
	Candidate             scheduler.Candidate
	MaximumCycles         int64
	RemainingCycles       int64
	RemainingTokens       int64
	RemainingCostMicrousd int64
	Effects               EffectRecorder
}

type Reconciliation struct {
	Workspace bool `json:"workspace"`
	Evidence  bool `json:"evidence"`
}

type TaskResult struct {
	Outcome              TaskOutcome    `json:"outcome"`
	Detail               string         `json:"detail,omitempty"`
	CyclesConsumed       int64          `json:"cycles_consumed"`
	RemoteTokensConsumed int64          `json:"remote_tokens_consumed"`
	CostMicrousdConsumed int64          `json:"cost_microusd_consumed"`
	Reconciliation       Reconciliation `json:"reconciliation"`
	Replayed             bool           `json:"replayed,omitempty"`
}

type TaskExecutor func(ctx context.Context, request TaskRequest) (TaskResult, error)

type Outcome struct {
	OccurrenceID         string         `json:"occurrence_id"`
	OccurrenceSequence   int64          `json:"occurrence_sequence"`
	TaskID               string         `json:"task_id"`
	ExternalTaskID       string         `json:"external_task_id"`
	SchedulerRunID       string         `json:"scheduler_run_id"`
	Outcome              TaskOutcome    `json:"outcome"`
	Detail               string         `json:"detail,omitempty"`
	CyclesConsumed       int64          `json:"cycles_consumed"`
	RemoteTokensConsumed int64          `json:"remote_tokens_consumed"`
	CostMicrousdConsumed int64          `json:"cost_microusd_consumed"`
	Reconciliation       Reconciliation `json:"reconciliation"`
	LeaseReconciled      bool           `json:"lease_reconciled"`
	ResultSHA256         string         `json:"result_sha256"`
	Replayed             bool           `json:"replayed,omitempty"`
}

type Result struct {
	SchemaVersion             string            `json:"schema_version"`
	OperationID               string            `json:"operation_id"`
	ConfigSHA256              string            `json:"config_sha256"`
	QualityGateStatus         QualityGateStatus `json:"quality_gate_status"`
	StopReason                StopReason        `json:"stop_reason"`
	StopDetail                string            `json:"stop_detail,omitempty"`
	TerminalMarkerSHA256      string            `json:"terminal_marker_sha256"`
	TasksStarted              int64             `json:"tasks_started"`
	CyclesConsumed            int64             `json:"cycles_consumed"`
	RemoteTokensConsumed      int64             `json:"remote_tokens_consumed"`
	CostMicrousdConsumed      int64             `json:"cost_microusd_consumed"`
	PeakSourceMutatingWorkers int               `json:"peak_source_mutating_workers"`
	Outcomes                  []Outcome         `json:"outcomes"`
	Replayed                  bool              `json:"replayed,omitempty"`
}

type Status struct {
	OperationID               string
	Status                    string
	Configuration             PinnedConfiguration
	ConfigSHA256              string
	StartedAt                 time.Time
	DeadlineAt                time.Time
	UpdatedAt                 time.Time
	TerminalAt                *time.Time
	CancelRequestedAt         *time.Time
	StopReason                StopReason
	StopDetail                string
	TerminalMarkerSHA256      string
	TasksStarted              int64
	CyclesConsumed            int64
	RemoteTokensConsumed      int64
	CostMicrousdConsumed      int64
	PeakSourceMutatingWorkers int
	Outcomes                  []Outcome
}

func validateOperationID(value string) (uuid.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Version() != 7 {
		return uuid.Nil, errors.New("sequential queue: operation ID must be UUIDv7")
	}
	return parsed, nil
}

func validateResult(result TaskResult, limits Limits) error {
	if !result.Outcome.valid() {
		return errors.New("sequential queue: task executor returned an invalid outcome")
	}
	if len(result.Detail) > 8192 {
		return errors.New("sequential queue: task outcome detail exceeds 8192 bytes")
	}
	if result.CyclesConsumed < 0 || result.CyclesConsumed > limits.MaximumCyclesPerTask ||
		result.RemoteTokensConsumed < 0 || result.CostMicrousdConsumed < 0 {
		return errors.New("sequential queue: task executor returned invalid or over-limit usage")
	}
	if !result.Reconciliation.Workspace || !result.Reconciliation.Evidence {
		return errors.New("sequential queue: task executor returned unreconciled workspace or evidence")
	}
	return nil
}

func canonicalJSON(value any) ([]byte, string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return raw, hashBytes(raw), nil
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("sequential queue: JSON contains trailing data")
	}
	return nil
}

func hashBytes(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func validateEffectIdentity(identity, material string) error {
	if strings.TrimSpace(identity) == "" || len(identity) > 1024 {
		return errors.New("sequential queue: effect identity is empty or too large")
	}
	if !validHash(material) {
		return fmt.Errorf("sequential queue: effect %q has an invalid material SHA-256", identity)
	}
	return nil
}
