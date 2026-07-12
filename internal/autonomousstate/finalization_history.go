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

const FinalizationHistorySchemaVersion = "autonomous-finalization-transition-v1"

type FinalizationHistoryRecord struct {
	SchemaVersion     string                        `json:"schema_version"`
	TaskID            string                        `json:"task_id"`
	OperationID       string                        `json:"operation_id"`
	ApplicationSHA256 string                        `json:"application_sha256"`
	Stage             autonomous.FinalizationStage  `json:"stage"`
	CreatedAt         time.Time                     `json:"created_at"`
	Finalization      autonomous.FinalizationDetail `json:"finalization"`
	PreviousState     StateIdentity                 `json:"previous_state"`
	ResultingState    StateIdentity                 `json:"resulting_state"`
}

type FinalizationHistorySnapshot struct {
	Record     FinalizationHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}

func (r FinalizationHistoryRecord) Validate() error {
	if r.SchemaVersion != FinalizationHistorySchemaVersion {
		return fmt.Errorf("validate finalization history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate finalization history: invalid operation/application identity")
	}
	if r.CreatedAt.IsZero() || r.Stage != r.Finalization.Stage || r.OperationID != r.Finalization.OperationID {
		return errors.New("validate finalization history: stage, operation, or time mismatch")
	}
	if err := r.Finalization.Validate(); err != nil {
		return err
	}
	if err := r.PreviousState.Validate(); err != nil || !r.PreviousState.Persisted {
		return errors.New("validate finalization history: invalid previous state")
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate finalization history: invalid resulting state")
	}
	return nil
}

func MarshalFinalizationHistory(record FinalizationHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func DecodeFinalizationHistory(raw []byte) (FinalizationHistoryRecord, error) {
	var record FinalizationHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, err
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalFinalizationHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode finalization history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameFinalizationOperation(existing, requested FinalizationHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.Stage != requested.Stage || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: finalization operation %q stage %q was reused for different material", ErrOperationConflict, requested.OperationID, requested.Stage)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: finalization operation %q stage %q differs", ErrOperationConflict, requested.OperationID, requested.Stage)
	}
	return nil
}
