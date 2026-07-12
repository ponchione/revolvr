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

const InputHistorySchemaVersion = "autonomous-input-transition-v1"

type InputTransitionKind string

const (
	InputTransitionQuestion InputTransitionKind = "question_recorded"
	InputTransitionAnswer   InputTransitionKind = "answer_recorded"
	InputTransitionResume   InputTransitionKind = "resumed"
)

type InputHistoryRecord struct {
	SchemaVersion     string                          `json:"schema_version"`
	TaskID            string                          `json:"task_id"`
	OperationID       string                          `json:"operation_id"`
	ApplicationSHA256 string                          `json:"application_sha256"`
	Sequence          int64                           `json:"sequence"`
	Kind              InputTransitionKind             `json:"kind"`
	CreatedAt         time.Time                       `json:"created_at"`
	Question          *autonomous.InputQuestionRecord `json:"question,omitempty"`
	Answer            *autonomous.InputAnswerRecord   `json:"answer,omitempty"`
	Resume            *autonomous.InputResumeRecord   `json:"resume,omitempty"`
	PreviousState     StateIdentity                   `json:"previous_state"`
	ResultingState    StateIdentity                   `json:"resulting_state"`
}

func (r InputHistoryRecord) Validate() error {
	if r.SchemaVersion != InputHistorySchemaVersion {
		return fmt.Errorf("validate input history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) {
		return errors.New("validate input history: invalid operation/application identity")
	}
	if r.Sequence < 1 || r.CreatedAt.IsZero() {
		return errors.New("validate input history: positive sequence and created_at are required")
	}
	if err := r.PreviousState.Validate(); err != nil {
		return err
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate input history: invalid resulting state identity")
	}
	count := 0
	if r.Question != nil {
		count++
	}
	if r.Answer != nil {
		count++
	}
	if r.Resume != nil {
		count++
	}
	if count != 1 {
		return errors.New("validate input history: exactly one typed transition payload is required")
	}
	switch r.Kind {
	case InputTransitionQuestion:
		if r.Question == nil || r.Question.Sequence != r.Sequence {
			return errors.New("validate input history: question payload mismatch")
		}
		if err := r.Question.Validate(r.TaskID); err != nil {
			return err
		}
		if !r.CreatedAt.Equal(r.Question.RecordedAt) {
			return errors.New("validate input history: question timestamp mismatch")
		}
	case InputTransitionAnswer:
		if r.Answer == nil || r.Answer.Sequence != r.Sequence {
			return errors.New("validate input history: answer payload mismatch")
		}
		if err := r.Answer.Validate(r.TaskID); err != nil {
			return err
		}
		if !r.CreatedAt.Equal(r.Answer.AnsweredAt) {
			return errors.New("validate input history: answer timestamp mismatch")
		}
	case InputTransitionResume:
		if r.Resume == nil || r.Resume.Sequence != r.Sequence {
			return errors.New("validate input history: resume payload mismatch")
		}
		if err := r.Resume.Validate(r.TaskID); err != nil {
			return err
		}
		if !r.CreatedAt.Equal(r.Resume.ResumedAt) {
			return errors.New("validate input history: resume timestamp mismatch")
		}
	default:
		return fmt.Errorf("validate input history: unknown kind %q", r.Kind)
	}
	return nil
}

func MarshalInputHistory(record InputHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func DecodeInputHistory(raw []byte) (InputHistoryRecord, error) {
	var record InputHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode input history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalInputHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode input history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameInputOperation(existing, requested InputHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: input operation %q was reused for materially different input", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: input operation %q history content differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}

type InputHistorySnapshot struct {
	Record     InputHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}
