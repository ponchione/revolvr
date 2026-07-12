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

const OptionalRoleHistorySchemaVersion = "autonomous-optional-role-transition-v1"

type OptionalRoleHistoryRecord struct {
	SchemaVersion     string                            `json:"schema_version"`
	TaskID            string                            `json:"task_id"`
	OperationID       string                            `json:"operation_id"`
	ApplicationSHA256 string                            `json:"application_sha256"`
	Sequence          int64                             `json:"sequence"`
	CreatedAt         time.Time                         `json:"created_at"`
	Occurrence        autonomous.OptionalRoleOccurrence `json:"occurrence"`
	PreviousState     StateIdentity                     `json:"previous_state"`
	ResultingState    StateIdentity                     `json:"resulting_state"`
}

func (r OptionalRoleHistoryRecord) Validate() error {
	if r.SchemaVersion != OptionalRoleHistorySchemaVersion {
		return fmt.Errorf("validate optional-role history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate optional-role history: operation/application identity is invalid")
	}
	if r.Sequence < 1 || r.CreatedAt.IsZero() || r.Occurrence.Sequence != r.Sequence || r.Occurrence.TaskID != r.TaskID {
		return errors.New("validate optional-role history: sequence, time, or occurrence identity is invalid")
	}
	if err := r.Occurrence.Validate(); err != nil {
		return fmt.Errorf("validate optional-role history: occurrence: %w", err)
	}
	if err := r.PreviousState.Validate(); err != nil {
		return err
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate optional-role history: resulting state identity is invalid")
	}
	return nil
}

func MarshalOptionalRoleHistory(record OptionalRoleHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func DecodeOptionalRoleHistory(raw []byte) (OptionalRoleHistoryRecord, error) {
	var record OptionalRoleHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode optional-role history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalOptionalRoleHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode optional-role history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameOptionalRoleOperation(existing, requested OptionalRoleHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: optional-role operation %q was reused for materially different input", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: optional-role operation %q history content differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}

type OptionalRoleHistorySnapshot struct {
	Record     OptionalRoleHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}
