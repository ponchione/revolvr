package autonomousplanning

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func ParsePlanningOutput(raw []byte) (PlanningOutput, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return PlanningOutput{}, errors.New("parse planning output: output is missing or empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var output PlanningOutput
	if err := decoder.Decode(&output); err != nil {
		return PlanningOutput{}, fmt.Errorf("parse planning output: decode exactly one JSON object: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return PlanningOutput{}, errors.New("parse planning output: output contains more than one JSON value")
		}
		return PlanningOutput{}, fmt.Errorf("parse planning output: non-whitespace content follows the JSON object: %w", err)
	}
	if err := output.Validate(); err != nil {
		return PlanningOutput{}, fmt.Errorf("parse planning output: %w", err)
	}
	return output, nil
}
