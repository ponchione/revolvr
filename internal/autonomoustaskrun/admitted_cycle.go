package autonomoustaskrun

import (
	"context"
	"errors"
	"time"

	"revolvr/internal/autonomousattempt"
	"revolvr/internal/autonomouscycle"
)

// AdmittedCycleConfig composes one decision-only supervisor cycle with AW-15
// admission at the cycle's validated pre-worker seam. It remains a one-cycle
// operation and never retries.
type AdmittedCycleConfig struct {
	Cycle                 autonomouscycle.Config
	Admission             autonomousattempt.AdmissionConfig
	CompletionOperationID string
	Clock                 func() time.Time
}

type AdmittedCycleResult struct {
	Cycle      autonomouscycle.Result
	Admission  autonomousattempt.Result
	Completion autonomousattempt.Result
}

func RunAdmittedCycle(ctx context.Context, cfg AdmittedCycleConfig) (AdmittedCycleResult, error) {
	if cfg.Clock == nil || cfg.CompletionOperationID == "" {
		return AdmittedCycleResult{}, errors.New("task run admitted cycle: clock and completion operation ID are required")
	}
	if cfg.Cycle.BeforeWorker != nil {
		return AdmittedCycleResult{}, errors.New("task run admitted cycle: worker admission seam is already owned")
	}
	var result AdmittedCycleResult
	var admittedAt time.Time
	var admittedTaskID string
	cfg.Cycle.BeforeWorker = func(admitCtx context.Context, input autonomouscycle.WorkerAdmissionInput) error {
		admission := cfg.Admission
		admission.TaskID = input.TaskID
		admission.Action = input.Decision.Action
		admission.Decision = input.Decision
		admission.Reference = input.Reference
		admission.SourceRevision = input.SourceRevision
		admission.CreatedAt = cfg.Clock().UTC()
		admittedAt = admission.CreatedAt
		admittedTaskID = input.TaskID
		var err error
		result.Admission, err = autonomousattempt.Admit(admitCtx, admission)
		if err != nil {
			return err
		}
		if result.Admission.Disposition == autonomousattempt.DispositionBlocked {
			return errors.New("attempt admission stopped by durable circuit breaker")
		}
		return nil
	}
	cycle, cycleErr := autonomouscycle.Run(ctx, cfg.Cycle)
	result.Cycle = cycle
	if result.Admission.Disposition == "" {
		return result, cycleErr
	}
	finished := cfg.Clock().UTC()
	if finished.Before(admittedAt) {
		return result, errors.New("task run admitted cycle: trusted clock moved backwards")
	}
	observation := autonomousattempt.ObserveCycle(cycle, cycleErr)
	completion, err := autonomousattempt.Complete(context.WithoutCancel(ctx), autonomousattempt.CompletionConfig{TaskID: admittedTaskID, OperationID: cfg.CompletionOperationID, AttemptID: cfg.Admission.AttemptID, Expected: result.Admission.Current.Expected(), RunID: observation.RunID, OccurrenceID: observation.OccurrenceID, SourceAfter: observation.SourceAfter, Outcome: observation.Outcome, Duration: finished.Sub(admittedAt), Tokens: observation.Tokens, Evidence: observation.Evidence, Signatures: observation.Signatures, StopReason: observation.StopReason, CreatedAt: finished, Store: cfg.Admission.Store})
	if err != nil {
		return result, err
	}
	result.Completion = completion
	return result, cycleErr
}
