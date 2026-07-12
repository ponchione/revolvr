package autonomous

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const CorrectionOutputSchemaVersion = "autonomous-correction-output-v1"

type VerificationFailureTarget struct {
	TaskID         string              `json:"task_id"`
	RunID          string              `json:"run_id"`
	OccurrenceID   string              `json:"occurrence_id"`
	SourceRevision string              `json:"source_revision"`
	Status         VerificationStatus  `json:"status"`
	Evidence       []EvidenceReference `json:"evidence"`
}

func (t VerificationFailureTarget) Validate() error {
	for _, field := range []struct{ name, value string }{{"task_id", t.TaskID}, {"run_id", t.RunID}, {"occurrence_id", t.OccurrenceID}} {
		if strings.TrimSpace(field.value) == "" || field.value != strings.TrimSpace(field.value) || strings.ContainsAny(field.value, "\r\n") {
			return fmt.Errorf("%s is empty or malformed", field.name)
		}
	}
	if len(t.SourceRevision) != 64 {
		return errors.New("source_revision must be a lower-case SHA-256")
	}
	for _, r := range t.SourceRevision {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return errors.New("source_revision must be a lower-case SHA-256")
		}
	}
	if t.Status != VerificationStatusFailed {
		return fmt.Errorf("status must be %q", VerificationStatusFailed)
	}
	return validateEvidenceReferences("verification failure evidence", t.Evidence)
}

type CorrectionOutcome string

const (
	CorrectionOutcomeCorrected CorrectionOutcome = "corrected"
	CorrectionOutcomePartial   CorrectionOutcome = "partial"
	CorrectionOutcomeFailed    CorrectionOutcome = "failed"
)

type CorrectionOutput struct {
	SchemaVersion                string                     `json:"schema_version"`
	TaskID                       string                     `json:"task_id"`
	WorkerRunID                  string                     `json:"worker_run_id"`
	DecisionID                   string                     `json:"decision_id"`
	FindingIDs                   []string                   `json:"finding_ids,omitempty"`
	VerificationFailure          *VerificationFailureTarget `json:"verification_failure,omitempty"`
	Outcome                      CorrectionOutcome          `json:"outcome"`
	ResolvedFindingIDs           []string                   `json:"resolved_finding_ids,omitempty"`
	RemainingFindingIDs          []string                   `json:"remaining_finding_ids,omitempty"`
	VerificationFailureAddressed bool                       `json:"verification_failure_addressed"`
	Evidence                     []EvidenceReference        `json:"evidence"`
}

func (o CorrectionOutput) Validate() error {
	if o.SchemaVersion != CorrectionOutputSchemaVersion {
		return fmt.Errorf("validate correction output: unsupported schema_version %q", o.SchemaVersion)
	}
	if strings.TrimSpace(o.TaskID) == "" || strings.TrimSpace(o.WorkerRunID) == "" || strings.TrimSpace(o.DecisionID) == "" {
		return errors.New("validate correction output: task_id, worker_run_id, and decision_id are required")
	}
	if err := validateEvidenceReferences("validate correction output: evidence", o.Evidence); err != nil {
		return err
	}
	if (len(o.FindingIDs) == 0) == (o.VerificationFailure == nil) {
		return errors.New("validate correction output: exactly one authority kind is required")
	}
	if len(o.FindingIDs) != 0 {
		if err := validateFindingIDs("validate correction output: finding_ids", o.FindingIDs); err != nil {
			return err
		}
		if err := validateFindingIDs("validate correction output: resolved_finding_ids", o.ResolvedFindingIDs); err != nil {
			return err
		}
		if err := validateFindingIDs("validate correction output: remaining_finding_ids", o.RemainingFindingIDs); err != nil {
			return err
		}
		if o.VerificationFailureAddressed {
			return errors.New("validate correction output: audit correction cannot address a verification failure")
		}
		seen := map[string]int{}
		for _, id := range o.ResolvedFindingIDs {
			seen[id]++
		}
		for _, id := range o.RemainingFindingIDs {
			seen[id]++
		}
		for _, id := range o.FindingIDs {
			if seen[id] != 1 {
				return fmt.Errorf("validate correction output: finding %q is not partitioned exactly once", id)
			}
			delete(seen, id)
		}
		if len(seen) != 0 {
			return errors.New("validate correction output: claimed finding is outside correction authority")
		}
	} else {
		if err := o.VerificationFailure.Validate(); err != nil {
			return fmt.Errorf("validate correction output: verification_failure: %w", err)
		}
		if len(o.ResolvedFindingIDs) != 0 || len(o.RemainingFindingIDs) != 0 {
			return errors.New("validate correction output: verification repair cannot claim audit findings")
		}
	}
	switch o.Outcome {
	case CorrectionOutcomeCorrected:
		if len(o.RemainingFindingIDs) != 0 || (o.VerificationFailure != nil && !o.VerificationFailureAddressed) {
			return errors.New("validate correction output: corrected outcome retains unresolved authority")
		}
	case CorrectionOutcomePartial:
		if len(o.FindingIDs) == 0 || len(o.RemainingFindingIDs) == 0 {
			return errors.New("validate correction output: partial outcome requires remaining cited findings")
		}
	case CorrectionOutcomeFailed:
		if len(o.ResolvedFindingIDs) != 0 || o.VerificationFailureAddressed {
			return errors.New("validate correction output: failed outcome cannot claim resolution")
		}
	default:
		return fmt.Errorf("validate correction output: unknown outcome %q", o.Outcome)
	}
	return nil
}

func ParseCorrectionOutput(raw []byte) (CorrectionOutput, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var output CorrectionOutput
	if err := decoder.Decode(&output); err != nil {
		return output, fmt.Errorf("parse correction output: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return output, errors.New("parse correction output: multiple JSON values")
		}
		return output, err
	}
	if err := output.Validate(); err != nil {
		return output, err
	}
	return output, nil
}

func MarshalCorrectionOutput(output CorrectionOutput) ([]byte, error) {
	if err := output.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}

func CorrectionOutputSchema() ([]byte, error) {
	// The Go validator is authoritative; this schema keeps model output strict and map-free at the contract boundary.
	schema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": []string{"schema_version", "task_id", "worker_run_id", "decision_id", "outcome", "verification_failure_addressed", "evidence"},
		"properties": map[string]any{
			"schema_version": map[string]any{"const": CorrectionOutputSchemaVersion}, "task_id": map[string]any{"type": "string", "minLength": 1}, "worker_run_id": map[string]any{"type": "string", "minLength": 1}, "decision_id": map[string]any{"type": "string", "minLength": 1},
			"finding_ids": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}}, "verification_failure": map[string]any{"type": "object"}, "outcome": map[string]any{"enum": []string{string(CorrectionOutcomeCorrected), string(CorrectionOutcomePartial), string(CorrectionOutcomeFailed)}},
			"resolved_finding_ids": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}}, "remaining_finding_ids": map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string"}}, "verification_failure_addressed": map[string]any{"type": "boolean"}, "evidence": map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "object"}},
		}, "oneOf": []any{map[string]any{"required": []string{"finding_ids"}, "not": map[string]any{"required": []string{"verification_failure"}}}, map[string]any{"required": []string{"verification_failure"}, "not": map[string]any{"required": []string{"finding_ids"}}}}}
	raw, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(raw, '\n'), nil
}
