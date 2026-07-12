// Package autonomousoptional owns conditional documentor/simplifier
// assessment, bounded execution composition, and occurrence persistence.
package autonomousoptional

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"revolvr/internal/autonomous"
	"revolvr/internal/autonomousstate"
)

type ApplyConfig struct {
	TaskID      string
	OperationID string
	Expected    autonomousstate.ExpectedState
	Assessment  autonomous.OptionalRoleAssessment
	Occurrence  autonomous.OptionalRoleOccurrence
	CreatedAt   time.Time
	Store       *autonomousstate.Store
}

type ApplyResult struct {
	Disposition autonomousstate.CommitDisposition
	Current     autonomousstate.Snapshot
	History     autonomousstate.OptionalRoleHistorySnapshot
}

func Apply(ctx context.Context, cfg ApplyConfig) (ApplyResult, error) {
	if cfg.Store == nil || strings.TrimSpace(cfg.TaskID) == "" || strings.TrimSpace(cfg.OperationID) == "" || cfg.CreatedAt.IsZero() {
		return ApplyResult{}, errors.New("apply optional-role occurrence: store, identities, and time are required")
	}
	if err := cfg.Expected.Validate(); err != nil || !cfg.Expected.Exists {
		return ApplyResult{}, errors.New("apply optional-role occurrence: exact existing state expectation is required")
	}
	if err := cfg.Assessment.Validate(); err != nil {
		return ApplyResult{}, err
	}
	assessmentSHA, _ := cfg.Assessment.Identity()
	if err := cfg.Occurrence.Validate(); err != nil {
		return ApplyResult{}, err
	}
	if cfg.Assessment.TaskID != cfg.TaskID || cfg.Occurrence.TaskID != cfg.TaskID || cfg.Occurrence.Role != cfg.Assessment.Role || cfg.Occurrence.Decision != cfg.Assessment.DecisionReference || cfg.Occurrence.AssessmentSHA256 != assessmentSHA || cfg.Occurrence.SourceBefore != cfg.Assessment.SourceRevision {
		return ApplyResult{}, errors.New("apply optional-role occurrence: assessment and occurrence identities differ")
	}
	if cfg.Assessment.Disposition == autonomous.OptionalRoleDispositionNotApplicable && cfg.Occurrence.Outcome != autonomous.OptionalRoleOutcomeNotApplicable || cfg.Assessment.Disposition == autonomous.OptionalRoleDispositionRun && cfg.Occurrence.Outcome == autonomous.OptionalRoleOutcomeNotApplicable {
		return ApplyResult{}, errors.New("apply optional-role occurrence: assessment disposition and outcome conflict")
	}
	application := hashValue(struct {
		TaskID, OperationID string
		Expected            autonomousstate.ExpectedState
		Assessment          autonomous.OptionalRoleAssessment
		Occurrence          autonomous.OptionalRoleOccurrence
	}{cfg.TaskID, cfg.OperationID, cfg.Expected, cfg.Assessment, cfg.Occurrence})
	if history, found, err := cfg.Store.LoadOptionalRoleOperation(ctx, cfg.TaskID, cfg.OperationID); err != nil {
		return ApplyResult{}, err
	} else if found {
		if history.Record.ApplicationSHA256 != application {
			return ApplyResult{}, fmt.Errorf("%w: optional-role application differs", autonomousstate.ErrOperationConflict)
		}
		current, stateFound, err := cfg.Store.Load(ctx, cfg.TaskID)
		if err != nil || !stateFound {
			return ApplyResult{}, errors.Join(err, autonomousstate.ErrStateMissing)
		}
		if !occurrencePresent(current.State, cfg.Occurrence) {
			return ApplyResult{}, autonomousstate.ErrStaleWrite
		}
		return ApplyResult{Disposition: autonomousstate.CommitReplayed, Current: current, History: history}, nil
	}
	current, found, err := cfg.Store.Load(ctx, cfg.TaskID)
	if err != nil || !found {
		return ApplyResult{}, errors.Join(err, autonomousstate.ErrStateMissing)
	}
	if current.SHA256 != cfg.Expected.SHA256 || current.ByteSize != cfg.Expected.ByteSize {
		return ApplyResult{}, autonomousstate.ErrStaleWrite
	}
	if cfg.Occurrence.Sequence != int64(len(current.State.OptionalRoles)+1) {
		return ApplyResult{}, errors.New("apply optional-role occurrence: sequence does not append to current state")
	}
	if err := validateAttemptEvidence(current.State, cfg.Occurrence); err != nil {
		return ApplyResult{}, err
	}
	next, err := cloneState(current.State)
	if err != nil {
		return ApplyResult{}, err
	}
	next.OptionalRoles = append(next.OptionalRoles, cfg.Occurrence)
	previousIdentity, err := autonomousstate.StateIdentityFor(current.SourcePath, true, current.State)
	if err != nil {
		return ApplyResult{}, err
	}
	nextIdentity, err := autonomousstate.StateIdentityFor(current.SourcePath, true, next)
	if err != nil {
		return ApplyResult{}, err
	}
	record := autonomousstate.OptionalRoleHistoryRecord{
		SchemaVersion: autonomousstate.OptionalRoleHistorySchemaVersion, TaskID: cfg.TaskID,
		OperationID: cfg.OperationID, ApplicationSHA256: application, Sequence: cfg.Occurrence.Sequence,
		CreatedAt: cfg.CreatedAt.UTC(), Occurrence: cfg.Occurrence,
		PreviousState: previousIdentity, ResultingState: nextIdentity,
	}
	committed, err := cfg.Store.CommitOptionalRole(ctx, autonomousstate.OptionalRoleCommitRequest{TaskID: cfg.TaskID, Expected: cfg.Expected, PreviousState: current.State, NextState: next, History: record})
	if err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Disposition: committed.Disposition, Current: committed.Current, History: committed.History}, nil
}

func validateAttemptEvidence(state autonomous.ExecutionState, occurrence autonomous.OptionalRoleOccurrence) error {
	if occurrence.Outcome == autonomous.OptionalRoleOutcomeNotApplicable {
		return nil
	}
	if occurrence.Worker == nil {
		return errors.New("apply optional-role occurrence: worker evidence is missing")
	}
	var admitted, completed *autonomous.AttemptEvent
	for i := range state.Attempts.Events {
		event := &state.Attempts.Events[i]
		if event.AttemptID != occurrence.Worker.AttemptID {
			continue
		}
		if event.Kind == autonomous.AttemptEventAdmitted {
			admitted = event
		} else if event.Kind == autonomous.AttemptEventCompleted {
			completed = event
		}
	}
	if admitted == nil || completed == nil || admitted.Decision != occurrence.Decision || admitted.SourceBefore != occurrence.SourceBefore || completed.RunID != occurrence.Worker.RunID || completed.SourceAfter != occurrence.SourceAfter {
		return errors.New("apply optional-role occurrence: exact admitted/completed attempt evidence is missing")
	}
	want := autonomous.AttemptOutcomeNoProgress
	if occurrence.Outcome == autonomous.OptionalRoleOutcomeSourceChanged {
		want = autonomous.AttemptOutcomeSucceeded
	}
	if completed.Outcome != want {
		return fmt.Errorf("apply optional-role occurrence: attempt outcome %q does not match %q", completed.Outcome, occurrence.Outcome)
	}
	return nil
}

func occurrencePresent(state autonomous.ExecutionState, occurrence autonomous.OptionalRoleOccurrence) bool {
	index := occurrence.Sequence - 1
	return index >= 0 && index < int64(len(state.OptionalRoles)) && reflect.DeepEqual(state.OptionalRoles[index], occurrence)
}

func cloneState(state autonomous.ExecutionState) (autonomous.ExecutionState, error) {
	raw, err := json.Marshal(state)
	if err != nil {
		return autonomous.ExecutionState{}, err
	}
	var result autonomous.ExecutionState
	if err := json.Unmarshal(raw, &result); err != nil {
		return autonomous.ExecutionState{}, err
	}
	return result, nil
}

func hashValue(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum)
}
