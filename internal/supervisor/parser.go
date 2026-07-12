package supervisor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"revolvr/internal/autonomous"
)

func ParseDecision(raw []byte, taskID string, audit *autonomous.AuditReport, failure ...*autonomous.VerificationFailureTarget) (autonomous.SupervisorDecision, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return autonomous.SupervisorDecision{}, errors.New("parse supervisor decision: output is missing or empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var decision autonomous.SupervisorDecision
	if err := decoder.Decode(&decision); err != nil {
		return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: decode exactly one JSON object: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return autonomous.SupervisorDecision{}, errors.New("parse supervisor decision: output contains more than one JSON value")
		}
		return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: non-whitespace content follows the JSON object: %w", err)
	}
	if err := decision.Validate(); err != nil {
		return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: %w", err)
	}
	if decision.TaskID != taskID {
		return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: decision task_id %q does not match requested task_id %q", decision.TaskID, taskID)
	}
	if decision.Action == autonomous.ActionCorrect {
		var target *autonomous.VerificationFailureTarget
		if len(failure) != 0 {
			target = failure[0]
		}
		if decision.VerificationFailure != nil {
			if target == nil {
				return autonomous.SupervisorDecision{}, errors.New("parse supervisor decision: verification correction lacks exact supplied failure authority")
			}
			if err := autonomous.ValidateVerificationCorrectionDecision(decision, *target); err != nil {
				return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: %w", err)
			}
		} else {
			if audit == nil {
				return autonomous.SupervisorDecision{}, errors.New("parse supervisor decision: audit correction requires a current changes_required audit report")
			}
			if err := autonomous.ValidateCorrectionDecision(decision, *audit); err != nil {
				return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: %w", err)
			}
		}
	} else if len(decision.FindingIDs) != 0 || decision.VerificationFailure != nil {
		return autonomous.SupervisorDecision{}, fmt.Errorf("parse supervisor decision: correction authority is only valid for action %q", autonomous.ActionCorrect)
	}
	return decision, nil
}

func validateAudit(taskID string, audit *autonomous.AuditReport) error {
	if audit == nil {
		return nil
	}
	if err := audit.Validate(); err != nil {
		return fmt.Errorf("validate supervisor audit context: %w", err)
	}
	if audit.TaskID != taskID {
		return fmt.Errorf("validate supervisor audit context: audit task_id %q does not match requested task_id %q", audit.TaskID, taskID)
	}
	if strings.TrimSpace(audit.TaskID) != audit.TaskID {
		return errors.New("validate supervisor audit context: audit task_id must be normalized")
	}
	return nil
}
