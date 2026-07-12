package autonomousaudit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func ParseAuditOutput(raw []byte) (AuditOutput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return AuditOutput{}, errors.New("parse audit output: output is missing or empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var output AuditOutput
	if err := decoder.Decode(&output); err != nil {
		return AuditOutput{}, fmt.Errorf("parse audit output: decode exactly one JSON object: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return AuditOutput{}, errors.New("parse audit output: output contains more than one JSON value")
		}
		return AuditOutput{}, fmt.Errorf("parse audit output: non-whitespace content follows the JSON object: %w", err)
	}
	if err := output.Validate(); err != nil {
		return AuditOutput{}, fmt.Errorf("parse audit output: %w", err)
	}
	return output, nil
}

func MarshalAuditOutput(output AuditOutput) ([]byte, error) {
	if err := output.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal audit output: %w", err)
	}
	return append(raw, '\n'), nil
}
