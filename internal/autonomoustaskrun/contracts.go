// Package autonomoustaskrun owns the durable loop for exactly one pinned
// autonomous task. It deliberately delegates every bounded workflow effect to
// an injected step runner.
package autonomoustaskrun

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"revolvr/internal/autonomouspolicy"
)

const (
	OperationSchemaVersion = "autonomous-task-run-operation-v1"
	ResultSchemaVersion    = "autonomous-task-run-result-v1"
)

type StopReason string

const (
	StopCompleted          StopReason = "completed"
	StopBlocked            StopReason = "blocked"
	StopNeedsInput         StopReason = "needs_input"
	StopBudgetExhausted    StopReason = "budget_exhausted"
	StopNoProgress         StopReason = "no_progress"
	StopSafety             StopReason = "safety_stop"
	StopTaskCancelled      StopReason = "task_cancelled"
	StopOperationCancelled StopReason = "operation_cancelled"
	StopMaxCycles          StopReason = "max_cycles"
	StopUnsafeAmbiguous    StopReason = "unsafe_or_ambiguous"
	StopNoTask             StopReason = "no_task"
)

func (r StopReason) Valid() bool {
	switch r {
	case StopCompleted, StopBlocked, StopNeedsInput, StopBudgetExhausted, StopNoProgress, StopSafety, StopTaskCancelled, StopOperationCancelled, StopMaxCycles, StopUnsafeAmbiguous, StopNoTask:
		return true
	default:
		return false
	}
}

type MaxCycles struct {
	Mode  string `json:"mode"`
	Limit int64  `json:"limit,omitempty"`
}

func Limited(n int64) MaxCycles { return MaxCycles{Mode: "limited", Limit: n} }
func Unlimited() MaxCycles      { return MaxCycles{Mode: "unlimited"} }

func (m MaxCycles) Validate() error {
	if m.Mode == "unlimited" && m.Limit == 0 {
		return nil
	}
	if m.Mode == "limited" && m.Limit > 0 {
		return nil
	}
	return errors.New("task run: max cycles must be unlimited or a positive limited value")
}

type Identity struct {
	Path     string `json:"path"`
	SHA256   string `json:"sha256"`
	ByteSize int    `json:"byte_size"`
}

type ActionCount struct {
	Action string `json:"action"`
	Count  int64  `json:"count"`
}

type Statistics struct {
	SupervisorStarted   int64         `json:"supervisor_started"`
	SupervisorCompleted int64         `json:"supervisor_completed"`
	SupervisorReplayed  int64         `json:"supervisor_replayed,omitempty"`
	CyclesStarted       int64         `json:"cycles_started"`
	CyclesCompleted     int64         `json:"cycles_completed"`
	AttemptsAdmitted    int64         `json:"attempts_admitted,omitempty"`
	AttemptsCompleted   int64         `json:"attempts_completed,omitempty"`
	VerificationRuns    int64         `json:"verification_runs,omitempty"`
	Audits              int64         `json:"audits,omitempty"`
	Corrections         int64         `json:"corrections,omitempty"`
	OptionalRoles       int64         `json:"optional_roles,omitempty"`
	SourceCommits       int64         `json:"source_commits,omitempty"`
	CheckpointAdvances  int64         `json:"checkpoint_advances,omitempty"`
	Actions             []ActionCount `json:"actions,omitempty"`
}

func (s *Statistics) Add(d Statistics) {
	s.SupervisorStarted += d.SupervisorStarted
	s.SupervisorCompleted += d.SupervisorCompleted
	s.SupervisorReplayed += d.SupervisorReplayed
	s.CyclesStarted += d.CyclesStarted
	s.CyclesCompleted += d.CyclesCompleted
	s.AttemptsAdmitted += d.AttemptsAdmitted
	s.AttemptsCompleted += d.AttemptsCompleted
	s.VerificationRuns += d.VerificationRuns
	s.Audits += d.Audits
	s.Corrections += d.Corrections
	s.OptionalRoles += d.OptionalRoles
	s.SourceCommits += d.SourceCommits
	s.CheckpointAdvances += d.CheckpointAdvances
	counts := map[string]int64{}
	for _, v := range s.Actions {
		counts[v.Action] += v.Count
	}
	for _, v := range d.Actions {
		counts[v.Action] += v.Count
	}
	s.Actions = s.Actions[:0]
	for action, count := range counts {
		s.Actions = append(s.Actions, ActionCount{Action: action, Count: count})
	}
	sort.Slice(s.Actions, func(i, j int) bool { return s.Actions[i].Action < s.Actions[j].Action })
}

type Operation struct {
	SchemaVersion  string                                 `json:"schema_version"`
	OperationID    string                                 `json:"operation_id"`
	TaskID         string                                 `json:"task_id"`
	Task           Identity                               `json:"task"`
	State          Identity                               `json:"state"`
	WorkspaceID    string                                 `json:"workspace_id,omitempty"`
	CheckpointSHA  string                                 `json:"checkpoint_sha,omitempty"`
	ConfigSHA256   string                                 `json:"config_sha256"`
	MaxCycles      MaxCycles                              `json:"max_cycles"`
	StartedAt      time.Time                              `json:"started_at"`
	UpdatedAt      time.Time                              `json:"updated_at"`
	CompletedAt    *time.Time                             `json:"completed_at,omitempty"`
	Sequence       int64                                  `json:"sequence"`
	Stage          string                                 `json:"stage"`
	InFlight       bool                                   `json:"in_flight"`
	Statistics     Statistics                             `json:"statistics"`
	LastAction     string                                 `json:"last_action,omitempty"`
	LastRunID      string                                 `json:"last_run_id,omitempty"`
	LastDecisionID string                                 `json:"last_decision_id,omitempty"`
	Evidence       []string                               `json:"evidence,omitempty"`
	LatestMutation *autonomouspolicy.SourceMutation       `json:"latest_mutation,omitempty"`
	Verification   *autonomouspolicy.VerificationEvidence `json:"verification,omitempty"`
	Audit          *autonomouspolicy.AuditEvidence        `json:"audit,omitempty"`
	StopReason     StopReason                             `json:"stop_reason,omitempty"`
	StopDetail     string                                 `json:"stop_detail,omitempty"`
}

func (o Operation) Validate() error {
	if o.SchemaVersion != OperationSchemaVersion || !safeID(o.OperationID) || !safeID(o.TaskID) {
		return errors.New("task run: operation schema or identity is invalid")
	}
	if err := validateIdentity("task", o.Task); err != nil {
		return err
	}
	if err := validateIdentity("state", o.State); err != nil {
		return err
	}
	if len(o.ConfigSHA256) != 64 {
		return errors.New("task run: operation effective configuration identity is invalid")
	}
	if err := o.MaxCycles.Validate(); err != nil {
		return err
	}
	if o.StartedAt.IsZero() || o.UpdatedAt.IsZero() || o.UpdatedAt.Before(o.StartedAt) || o.Sequence < 0 || operationStageOrder(o.Stage) < 0 {
		return errors.New("task run: operation time, sequence, or stage is invalid")
	}
	s := o.Statistics
	values := []int64{s.SupervisorStarted, s.SupervisorCompleted, s.SupervisorReplayed, s.CyclesStarted, s.CyclesCompleted, s.AttemptsAdmitted, s.AttemptsCompleted, s.VerificationRuns, s.Audits, s.Corrections, s.OptionalRoles, s.SourceCommits, s.CheckpointAdvances}
	for _, value := range values {
		if value < 0 {
			return errors.New("task run: operation statistics cannot be negative")
		}
	}
	if s.CyclesCompleted > s.CyclesStarted || s.SupervisorCompleted > s.SupervisorStarted || s.AttemptsCompleted > s.AttemptsAdmitted {
		return errors.New("task run: operation completion statistics exceed starts")
	}
	for i, action := range s.Actions {
		if strings.TrimSpace(action.Action) == "" || action.Count <= 0 || i > 0 && s.Actions[i-1].Action >= action.Action {
			return errors.New("task run: action statistics are not positive and canonically ordered")
		}
	}
	if o.StopReason != "" && !o.StopReason.Valid() {
		return errors.New("task run: operation stop reason is invalid")
	}
	if o.StopReason.Valid() {
		if o.Stage != "terminal" || o.CompletedAt == nil || o.CompletedAt.Before(o.StartedAt) || o.InFlight {
			return errors.New("task run: terminal operation is incomplete")
		}
	} else if o.CompletedAt != nil || o.Stage == "terminal" {
		return errors.New("task run: nonterminal operation has terminal evidence")
	}
	return nil
}

type Result struct {
	SchemaVersion  string     `json:"schema_version"`
	OperationID    string     `json:"operation_id,omitempty"`
	TaskID         string     `json:"task_id,omitempty"`
	StopReason     StopReason `json:"stop_reason"`
	StopDetail     string     `json:"stop_detail,omitempty"`
	Statistics     Statistics `json:"statistics"`
	LastAction     string     `json:"last_action,omitempty"`
	LastRunID      string     `json:"last_run_id,omitempty"`
	LastDecisionID string     `json:"last_decision_id,omitempty"`
	Evidence       []string   `json:"evidence,omitempty"`
	Replayed       bool       `json:"replayed,omitempty"`
}

func hashBytes(raw []byte) Identity {
	sum := sha256.Sum256(raw)
	return Identity{SHA256: hex.EncodeToString(sum[:]), ByteSize: len(raw)}
}
func canonical(v any) ([]byte, error) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
func safeID(v string) bool {
	if v == "" || v != strings.TrimSpace(v) {
		return false
	}
	for _, r := range v {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func validateIdentity(label string, v Identity) error {
	if v.Path == "" || len(v.SHA256) != 64 || v.ByteSize < 0 {
		return fmt.Errorf("task run: invalid %s identity", label)
	}
	return nil
}
