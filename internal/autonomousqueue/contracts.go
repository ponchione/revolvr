// Package autonomousqueue owns bounded queue sweeps above the pure scheduler
// and the exact single-task runner.
package autonomousqueue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomoustaskrun"
)

const (
	OperationSchemaVersion       = "autonomous-queue-operation-v2"
	LegacyOperationSchemaVersion = "autonomous-queue-operation-v1"
	ResultSchemaVersion          = "autonomous-queue-result-v2"
	MaximumWorkerLimit           = 4
)

const QueuePolicySchemaVersion = "autonomous-queue-policy-v1"

type Policy struct {
	SchemaVersion  string `json:"schema_version" yaml:"schema_version"`
	MaximumWorkers int    `json:"maximum_workers" yaml:"maximum_workers"`
}

func DefaultPolicy() Policy {
	return Policy{SchemaVersion: QueuePolicySchemaVersion, MaximumWorkers: 1}
}

func (p Policy) Validate() error {
	if p.SchemaVersion != QueuePolicySchemaVersion {
		return fmt.Errorf("autonomous queue: unknown policy schema %q", p.SchemaVersion)
	}
	if p.MaximumWorkers <= 0 || p.MaximumWorkers > MaximumWorkerLimit {
		return fmt.Errorf("autonomous queue: maximum_workers must be between 1 and %d", MaximumWorkerLimit)
	}
	return nil
}

type Mode string

const (
	ModeUntilExhausted Mode = "until_exhausted"
	ModeDaemon         Mode = "daemon"
)

type StopReason string

const (
	StopDrained           StopReason = "drained"
	StopWaitingDependency StopReason = "waiting_dependencies"
	StopWaitingInput      StopReason = "waiting_input"
	StopWaitingBlocked    StopReason = "waiting_blocked"
	StopBudgetExhausted   StopReason = "budget_exhausted"
	StopCancelled         StopReason = "cancelled"
	StopSafety            StopReason = "safety_stop"
	StopUnsafeAmbiguous   StopReason = "unsafe_or_ambiguous"
)

func (r StopReason) Valid() bool {
	switch r {
	case StopDrained, StopWaitingDependency, StopWaitingInput, StopWaitingBlocked, StopBudgetExhausted, StopCancelled, StopSafety, StopUnsafeAmbiguous:
		return true
	default:
		return false
	}
}

type TaskOutcome struct {
	SelectionSequence int64                        `json:"selection_sequence,omitempty"`
	Batch             int64                        `json:"batch,omitempty"`
	Slot              int                          `json:"slot,omitempty"`
	TaskID            string                       `json:"task_id"`
	TaskOperationID   string                       `json:"task_operation_id"`
	StopReason        autonomoustaskrun.StopReason `json:"stop_reason"`
	StopDetail        string                       `json:"stop_detail,omitempty"`
	BeforeFingerprint string                       `json:"before_fingerprint"`
	AfterFingerprint  string                       `json:"after_fingerprint"`
	Authority         string                       `json:"authority"`
	Statistics        autonomoustaskrun.Statistics `json:"statistics"`
	Evidence          []string                     `json:"evidence,omitempty"`
	Replayed          bool                         `json:"replayed,omitempty"`
}

type Exclusion struct {
	TaskID    string `json:"task_id"`
	Authority string `json:"authority"`
}

type OutcomeCount struct {
	Reason autonomoustaskrun.StopReason `json:"reason"`
	Count  int64                        `json:"count"`
}

type Statistics struct {
	Selections          int64          `json:"selections"`
	TasksRun            int64          `json:"tasks_run"`
	Batches             int64          `json:"batches,omitempty"`
	PeakActiveWorkers   int            `json:"peak_active_workers,omitempty"`
	SequentialFallbacks int64          `json:"sequential_fallbacks,omitempty"`
	Outcomes            []OutcomeCount `json:"outcomes,omitempty"`
}

func (s *Statistics) add(reason autonomoustaskrun.StopReason) {
	s.Selections++
	s.TasksRun++
	counts := make(map[autonomoustaskrun.StopReason]int64, len(s.Outcomes)+1)
	for _, item := range s.Outcomes {
		counts[item.Reason] += item.Count
	}
	counts[reason]++
	s.Outcomes = s.Outcomes[:0]
	for key, count := range counts {
		s.Outcomes = append(s.Outcomes, OutcomeCount{Reason: key, Count: count})
	}
	sort.Slice(s.Outcomes, func(i, j int) bool { return s.Outcomes[i].Reason < s.Outcomes[j].Reason })
}

type Selection struct {
	Sequence        int64  `json:"sequence,omitempty"`
	Batch           int64  `json:"batch,omitempty"`
	Slot            int    `json:"slot,omitempty"`
	TaskID          string `json:"task_id"`
	TaskOperationID string `json:"task_operation_id"`
	Fingerprint     string `json:"fingerprint"`
	Authority       string `json:"authority"`
}

type SlotState string

const (
	SlotAdmitted SlotState = "admitted"
	SlotTerminal SlotState = "terminal"
)

type WorkerSlot struct {
	Selection Selection    `json:"selection"`
	State     SlotState    `json:"state"`
	Outcome   *TaskOutcome `json:"outcome,omitempty"`
}

type Operation struct {
	SchemaVersion         string        `json:"schema_version"`
	OperationID           string        `json:"operation_id"`
	Mode                  Mode          `json:"mode"`
	ConfigSchema          string        `json:"config_schema"`
	ConfigSHA256          string        `json:"config_sha256"`
	SafetyIdentity        string        `json:"safety_identity"`
	MaxTasks              int64         `json:"max_tasks"`
	MaximumWorkers        int           `json:"maximum_workers,omitempty"`
	StartedAt             time.Time     `json:"started_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
	CompletedAt           *time.Time    `json:"completed_at,omitempty"`
	Sequence              int64         `json:"sequence"`
	Sweep                 int64         `json:"sweep"`
	DaemonWakeCount       int64         `json:"daemon_wake_count,omitempty"`
	DaemonWakeFingerprint string        `json:"daemon_wake_fingerprint,omitempty"`
	Stage                 string        `json:"stage"`
	LastFingerprint       string        `json:"last_fingerprint,omitempty"`
	InFlight              *Selection    `json:"in_flight,omitempty"`
	Slots                 []WorkerSlot  `json:"slots,omitempty"`
	SequentialFallback    string        `json:"sequential_fallback,omitempty"`
	Outcomes              []TaskOutcome `json:"outcomes,omitempty"`
	Exclusions            []Exclusion   `json:"exclusions,omitempty"`
	Statistics            Statistics    `json:"statistics"`
	RemainingReady        []string      `json:"remaining_ready,omitempty"`
	RemainingWaiting      []string      `json:"remaining_waiting,omitempty"`
	StopReason            StopReason    `json:"stop_reason,omitempty"`
	StopDetail            string        `json:"stop_detail,omitempty"`
}

func (o Operation) Validate() error {
	if (o.SchemaVersion != OperationSchemaVersion && o.SchemaVersion != LegacyOperationSchemaVersion) || !safeID(o.OperationID) {
		return errors.New("autonomous queue: invalid operation schema or identity")
	}
	if o.Mode != ModeUntilExhausted && o.Mode != ModeDaemon {
		return errors.New("autonomous queue: invalid mode")
	}
	if strings.TrimSpace(o.ConfigSchema) == "" || !validHash(o.ConfigSHA256) || strings.TrimSpace(o.SafetyIdentity) == "" || o.MaxTasks <= 0 {
		return errors.New("autonomous queue: invalid configuration, safety, or task bound")
	}
	if o.StartedAt.IsZero() || o.UpdatedAt.IsZero() || o.UpdatedAt.Before(o.StartedAt) || o.Sequence < 0 || o.Sweep <= 0 || stageOrder(o.Stage) < 0 {
		return errors.New("autonomous queue: invalid time, sequence, sweep, or stage")
	}
	if o.DaemonWakeCount < 0 || o.DaemonWakeFingerprint != "" && !validHash(o.DaemonWakeFingerprint) || (o.DaemonWakeCount == 0) != (o.DaemonWakeFingerprint == "") || o.Mode == ModeUntilExhausted && (o.DaemonWakeCount != 0 || o.DaemonWakeFingerprint != "") {
		return errors.New("autonomous queue: invalid daemon wake observation")
	}
	if o.SchemaVersion == LegacyOperationSchemaVersion {
		if o.MaximumWorkers != 0 || len(o.Slots) != 0 || o.SequentialFallback != "" {
			return errors.New("autonomous queue: legacy operation contains parallel fields")
		}
	} else if o.MaximumWorkers <= 0 || o.MaximumWorkers > MaximumWorkerLimit || o.InFlight != nil {
		return errors.New("autonomous queue: invalid worker bound or legacy in-flight field")
	}
	if len(o.Slots) > o.MaximumWorkers {
		return errors.New("autonomous queue: durable worker slots exceed the configured bound")
	}
	if o.SequentialFallback != "" && o.SequentialFallback != "overlap_authority_unavailable" && o.SequentialFallback != "no_additional_safe_candidate" {
		return errors.New("autonomous queue: unknown sequential fallback reason")
	}
	if o.InFlight != nil {
		if !safeID(o.InFlight.TaskID) || !safeID(o.InFlight.TaskOperationID) || !validHash(o.InFlight.Fingerprint) || !validHash(o.InFlight.Authority) {
			return errors.New("autonomous queue: invalid in-flight selection")
		}
		if o.Stage != "selected" {
			return errors.New("autonomous queue: in-flight selection has wrong stage")
		}
	}
	for i, slot := range o.Slots {
		s := slot.Selection
		if !safeID(s.TaskID) || !safeID(s.TaskOperationID) || !validHash(s.Fingerprint) || !validHash(s.Authority) || s.Sequence <= 0 || s.Batch <= 0 || s.Slot != i+1 {
			return fmt.Errorf("autonomous queue: invalid worker slot %d", i)
		}
		if i > 0 && o.Slots[i-1].Selection.Sequence+1 != s.Sequence {
			return errors.New("autonomous queue: worker slot selections are not contiguous")
		}
		switch slot.State {
		case SlotAdmitted:
			if slot.Outcome != nil || o.Stage == "terminal" {
				return errors.New("autonomous queue: admitted worker slot has terminal evidence")
			}
		case SlotTerminal:
			if slot.Outcome == nil || slot.Outcome.TaskID != s.TaskID || slot.Outcome.TaskOperationID != s.TaskOperationID || slot.Outcome.SelectionSequence != s.Sequence || slot.Outcome.Batch != s.Batch || slot.Outcome.Slot != s.Slot {
				return errors.New("autonomous queue: terminal worker slot evidence conflicts")
			}
		default:
			return errors.New("autonomous queue: unknown worker slot state")
		}
	}
	if o.SchemaVersion == OperationSchemaVersion && o.Stage == "selected" && len(o.Slots) == 0 {
		return errors.New("autonomous queue: selected operation has no worker slots")
	}
	if o.StopReason != "" {
		if !o.StopReason.Valid() || o.Stage != "terminal" || o.CompletedAt == nil || o.InFlight != nil {
			return errors.New("autonomous queue: incomplete terminal operation")
		}
	} else if o.CompletedAt != nil || o.Stage == "terminal" {
		return errors.New("autonomous queue: nonterminal operation has terminal evidence")
	}
	for i, outcome := range o.Outcomes {
		if !safeID(outcome.TaskID) || !safeID(outcome.TaskOperationID) || !outcome.StopReason.Valid() || !validHash(outcome.BeforeFingerprint) || !validHash(outcome.AfterFingerprint) || !validHash(outcome.Authority) {
			return fmt.Errorf("autonomous queue: invalid outcome %d", i)
		}
	}
	for i, exclusion := range o.Exclusions {
		if !safeID(exclusion.TaskID) || !validHash(exclusion.Authority) || i > 0 && o.Exclusions[i-1].TaskID >= exclusion.TaskID {
			return errors.New("autonomous queue: exclusions are invalid or not canonical")
		}
	}
	return nil
}

type Result struct {
	SchemaVersion         string        `json:"schema_version"`
	OperationID           string        `json:"operation_id"`
	Mode                  Mode          `json:"mode"`
	StopReason            StopReason    `json:"stop_reason"`
	StopDetail            string        `json:"stop_detail,omitempty"`
	Outcomes              []TaskOutcome `json:"outcomes,omitempty"`
	Statistics            Statistics    `json:"statistics"`
	MaximumWorkers        int           `json:"maximum_workers"`
	RemainingReady        []string      `json:"remaining_ready,omitempty"`
	RemainingWaiting      []string      `json:"remaining_waiting,omitempty"`
	Replayed              bool          `json:"replayed,omitempty"`
	DaemonWakeCount       int64         `json:"daemon_wake_count,omitempty"`
	DaemonWakeFingerprint string        `json:"daemon_wake_fingerprint,omitempty"`
}

func resultOf(op Operation, replayed bool) Result {
	workers := op.MaximumWorkers
	if op.SchemaVersion == LegacyOperationSchemaVersion {
		workers = 1
	}
	return Result{SchemaVersion: ResultSchemaVersion, OperationID: op.OperationID, Mode: op.Mode, StopReason: op.StopReason, StopDetail: op.StopDetail, Outcomes: append([]TaskOutcome(nil), op.Outcomes...), Statistics: op.Statistics, MaximumWorkers: workers, RemainingReady: append([]string(nil), op.RemainingReady...), RemainingWaiting: append([]string(nil), op.RemainingWaiting...), Replayed: replayed, DaemonWakeCount: op.DaemonWakeCount, DaemonWakeFingerprint: op.DaemonWakeFingerprint}
}

func safeID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func validHash(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func canonical(value any) ([]byte, error) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func hashStrings(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func stageOrder(stage string) int {
	switch stage {
	case "admitted":
		return 0
	case "selected":
		return 1
	case "task_stopped":
		return 2
	case "terminal":
		return 3
	default:
		return -1
	}
}
