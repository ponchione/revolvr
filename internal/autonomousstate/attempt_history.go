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

const AttemptHistorySchemaVersion = "autonomous-attempt-transition-v1"

type AttemptTransitionKind string

const (
	AttemptTransitionAdmitted  AttemptTransitionKind = "admitted"
	AttemptTransitionCompleted AttemptTransitionKind = "completed"
	AttemptTransitionBreaker   AttemptTransitionKind = "breaker"
)

type AttemptHistoryRecord struct {
	SchemaVersion     string                           `json:"schema_version"`
	TaskID            string                           `json:"task_id"`
	OperationID       string                           `json:"operation_id"`
	ApplicationSHA256 string                           `json:"application_sha256"`
	Sequence          int64                            `json:"sequence"`
	Kind              AttemptTransitionKind            `json:"kind"`
	CreatedAt         time.Time                        `json:"created_at"`
	Event             *autonomous.AttemptEvent         `json:"event,omitempty"`
	Breaker           *autonomous.CircuitBreakerDetail `json:"breaker,omitempty"`
	PreviousState     StateIdentity                    `json:"previous_state"`
	ResultingState    StateIdentity                    `json:"resulting_state"`
}

func (r AttemptHistoryRecord) Validate() error {
	if r.SchemaVersion != AttemptHistorySchemaVersion {
		return fmt.Errorf("validate attempt history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return fmt.Errorf("validate attempt history: %w", err)
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil {
		return fmt.Errorf("validate attempt history: %w", err)
	}
	if !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate attempt history: application_sha256 is invalid")
	}
	if r.Sequence < 1 || r.CreatedAt.IsZero() {
		return errors.New("validate attempt history: positive sequence and created_at are required")
	}
	if err := r.PreviousState.Validate(); err != nil {
		return fmt.Errorf("validate attempt history: previous_state: %w", err)
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate attempt history: resulting_state must be a valid persisted identity")
	}
	switch r.Kind {
	case AttemptTransitionAdmitted, AttemptTransitionCompleted:
		if r.Event == nil || (r.Kind == AttemptTransitionAdmitted && r.Breaker != nil) {
			return errors.New("validate attempt history: event transition has contradictory evidence")
		}
		if err := r.Event.Validate(r.TaskID); err != nil {
			return fmt.Errorf("validate attempt history: event: %w", err)
		}
		want := autonomous.AttemptEventAdmitted
		if r.Kind == AttemptTransitionCompleted {
			want = autonomous.AttemptEventCompleted
		}
		if r.Event.Kind != want || r.Event.Sequence != r.Sequence {
			return errors.New("validate attempt history: event kind or sequence mismatch")
		}
		if r.Breaker != nil {
			if err := r.Breaker.Validate(); err != nil {
				return fmt.Errorf("validate attempt history: breaker: %w", err)
			}
		}
	case AttemptTransitionBreaker:
		if r.Event != nil || r.Breaker == nil {
			return errors.New("validate attempt history: breaker transition requires only breaker evidence")
		}
		if err := r.Breaker.Validate(); err != nil {
			return fmt.Errorf("validate attempt history: breaker: %w", err)
		}
	default:
		return fmt.Errorf("validate attempt history: unknown kind %q", r.Kind)
	}
	return nil
}

func MarshalAttemptHistory(record AttemptHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal attempt history: %w", err)
	}
	return append(raw, '\n'), nil
}

func DecodeAttemptHistory(raw []byte) (AttemptHistoryRecord, error) {
	var record AttemptHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode attempt history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, err := MarshalAttemptHistory(record)
	if err != nil {
		return record, err
	}
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode attempt history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameAttemptOperation(existing, requested AttemptHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: attempt operation %q was reused for materially different input", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: attempt operation %q history content differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}
