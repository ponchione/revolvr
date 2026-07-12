package autonomousstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"revolvr/internal/autonomous"
)

const WorkspaceHistorySchemaVersion = "autonomous-workspace-transition-v1"

type WorkspaceTransitionKind string

const (
	WorkspaceTransitionCreated    WorkspaceTransitionKind = "created"
	WorkspaceTransitionReopened   WorkspaceTransitionKind = "reopened"
	WorkspaceTransitionCheckpoint WorkspaceTransitionKind = "checkpoint_advanced"
	WorkspaceTransitionRestored   WorkspaceTransitionKind = "restored"
	WorkspaceTransitionAmbiguous  WorkspaceTransitionKind = "ambiguity_retained"
	WorkspaceTransitionCleaned    WorkspaceTransitionKind = "cleaned"
)

type WorkspaceHistoryRecord struct {
	SchemaVersion      string                    `json:"schema_version"`
	TaskID             string                    `json:"task_id"`
	OperationID        string                    `json:"operation_id"`
	ApplicationSHA256  string                    `json:"application_sha256"`
	Kind               WorkspaceTransitionKind   `json:"kind"`
	CreatedAt          time.Time                 `json:"created_at"`
	PreviousWorkspace  *autonomous.TaskWorkspace `json:"previous_workspace,omitempty"`
	ResultingWorkspace autonomous.TaskWorkspace  `json:"resulting_workspace"`
	PreviousState      StateIdentity             `json:"previous_state"`
	ResultingState     StateIdentity             `json:"resulting_state"`
}

type WorkspaceHistorySnapshot struct {
	Record     WorkspaceHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}

func (r WorkspaceHistoryRecord) Validate() error {
	if r.SchemaVersion != WorkspaceHistorySchemaVersion {
		return fmt.Errorf("validate workspace history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate workspace history: invalid operation/application identity")
	}
	switch r.Kind {
	case WorkspaceTransitionCreated, WorkspaceTransitionReopened, WorkspaceTransitionCheckpoint, WorkspaceTransitionRestored, WorkspaceTransitionAmbiguous, WorkspaceTransitionCleaned:
	default:
		return fmt.Errorf("validate workspace history: unknown kind %q", r.Kind)
	}
	if r.CreatedAt.IsZero() {
		return errors.New("validate workspace history: created_at is required")
	}
	if r.PreviousWorkspace != nil {
		if err := r.PreviousWorkspace.Validate(); err != nil {
			return err
		}
		if r.PreviousWorkspace.TaskID != r.TaskID {
			return errors.New("validate workspace history: previous workspace task mismatch")
		}
	} else if r.Kind != WorkspaceTransitionCreated {
		return errors.New("validate workspace history: only creation may omit previous workspace")
	}
	if err := r.ResultingWorkspace.Validate(); err != nil {
		return err
	}
	if r.ResultingWorkspace.TaskID != r.TaskID {
		return errors.New("validate workspace history: resulting workspace task mismatch")
	}
	if err := r.PreviousState.Validate(); err != nil {
		return err
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate workspace history: invalid resulting state identity")
	}
	return nil
}

func MarshalWorkspaceHistory(record WorkspaceHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func DecodeWorkspaceHistory(raw []byte) (WorkspaceHistoryRecord, error) {
	var record WorkspaceHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode workspace history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalWorkspaceHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode workspace history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameWorkspaceOperation(existing, requested WorkspaceHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: workspace operation %q was reused for different material", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: workspace operation %q history differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}
