package autonomousstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
)

const BlockHistorySchemaVersion = "autonomous-block-transition-v1"

// BlockHistoryRecord is the immutable authority for one explicit supervisor
// block transition. Attempt circuit breakers retain their existing history.
type BlockHistoryRecord struct {
	SchemaVersion     string                         `json:"schema_version"`
	TaskID            string                         `json:"task_id"`
	OperationID       string                         `json:"operation_id"`
	ApplicationSHA256 string                         `json:"application_sha256"`
	Decision          autonomous.DecisionReference   `json:"decision"`
	Reason            string                         `json:"reason"`
	Evidence          []autonomous.EvidenceReference `json:"evidence"`
	SourceRevision    string                         `json:"source_revision"`
	CreatedAt         time.Time                      `json:"created_at"`
	PreviousState     StateIdentity                  `json:"previous_state"`
	ResultingState    StateIdentity                  `json:"resulting_state"`
}

func (r BlockHistoryRecord) Validate() error {
	if r.SchemaVersion != BlockHistorySchemaVersion {
		return fmt.Errorf("validate block history: unsupported schema_version %q", r.SchemaVersion)
	}
	if err := validateIdentity("task_id", r.TaskID); err != nil {
		return err
	}
	if err := validateIdentity("operation_id", r.OperationID); err != nil || !validSHA256(r.ApplicationSHA256) || !validSHA256(r.SourceRevision) {
		return errors.New("validate block history: invalid operation, application, or source identity")
	}
	if err := r.Decision.Validate(); err != nil || r.Decision.TaskID != r.TaskID || r.Decision.Action != autonomous.ActionBlock {
		return errors.New("validate block history: exact block decision reference is required")
	}
	if r.Reason == "" || r.Reason != strings.TrimSpace(r.Reason) || r.CreatedAt.IsZero() {
		return errors.New("validate block history: normalized reason and created_at are required")
	}
	if err := (autonomous.TerminalDetail{Reason: r.Reason, Evidence: r.Evidence}).Validate(); err != nil {
		return fmt.Errorf("validate block history: %w", err)
	}
	if err := r.PreviousState.Validate(); err != nil {
		return err
	}
	if err := r.ResultingState.Validate(); err != nil || !r.ResultingState.Persisted {
		return errors.New("validate block history: invalid resulting state identity")
	}
	return nil
}

func MarshalBlockHistory(record BlockHistoryRecord) ([]byte, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func DecodeBlockHistory(raw []byte) (BlockHistoryRecord, error) {
	var record BlockHistoryRecord
	if err := decodeStrict(raw, &record); err != nil {
		return record, fmt.Errorf("decode block history: %w", err)
	}
	if err := record.Validate(); err != nil {
		return record, err
	}
	canonical, _ := MarshalBlockHistory(record)
	if !bytes.Equal(raw, canonical) {
		return record, errors.New("decode block history: bytes are not canonical deterministic JSON")
	}
	return record, nil
}

func sameBlockOperation(existing, requested BlockHistoryRecord) error {
	if existing.OperationID != requested.OperationID || existing.ApplicationSHA256 != requested.ApplicationSHA256 {
		return fmt.Errorf("%w: block operation %q was reused for different material", ErrOperationConflict, requested.OperationID)
	}
	normalized := requested
	normalized.CreatedAt = existing.CreatedAt
	if !reflect.DeepEqual(existing, normalized) {
		return fmt.Errorf("%w: block operation %q history differs", ErrOperationConflict, requested.OperationID)
	}
	return nil
}

type BlockHistorySnapshot struct {
	Record     BlockHistoryRecord
	SHA256     string
	ByteSize   int
	SourcePath string
}
