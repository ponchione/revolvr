package app

import (
	"context"
	"fmt"
	"strings"

	"revolvr/internal/codexexec"
	"revolvr/internal/runonce"
)

const loopFailureGuardrailLimit = 2

type RunOnceRunner func(context.Context, runonce.Config) (runonce.Result, error)
type RunProgress func(codexexec.ProgressEvent)
type RunPassFunc func(runonce.Result) error

type RunOnceInput struct {
	Runner   RunOnceRunner
	Progress RunProgress
}

type RunLoopInput struct {
	MaxPasses int
	Runner    RunOnceRunner
	Progress  RunProgress
	OnPass    RunPassFunc
}

type RunLoopResult struct {
	Stats RunLoopStats
}

type RunLoopStats struct {
	MaxPasses                  int
	Passes                     int
	Completed                  int
	FailedOrBlocked            int
	NoTask                     bool
	StopReason                 string
	ConsecutiveFailedOrBlocked int
}

func RunOnce(ctx context.Context, cfg Config, input RunOnceInput) (runonce.Result, error) {
	runCfg, err := loadConfiguredRunOnce(cfg, input.Progress)
	if err != nil {
		return runonce.Result{}, err
	}
	return runWithConfig(ctx, input.Runner, runCfg)
}

func RunLoop(ctx context.Context, cfg Config, input RunLoopInput) (RunLoopResult, error) {
	if input.MaxPasses <= 0 {
		return RunLoopResult{}, fmt.Errorf("run: --max-passes must be greater than 0")
	}

	stats := RunLoopStats{MaxPasses: input.MaxPasses}
	for pass := 0; pass < input.MaxPasses; pass++ {
		if err := ctx.Err(); err != nil {
			stats.StopReason = "context_cancelled"
			return RunLoopResult{Stats: stats}, err
		}

		runCfg, err := loadConfiguredRunOnce(cfg, input.Progress)
		if err != nil {
			stats.StopReason = "config_error"
			return RunLoopResult{Stats: stats}, err
		}

		result, err := runWithConfig(ctx, input.Runner, runCfg)
		if err != nil {
			stats.Passes++
			stats.FailedOrBlocked++
			stats.ConsecutiveFailedOrBlocked++
			stats.StopReason = "runner_error"
			if ctx.Err() != nil {
				stats.StopReason = "context_cancelled"
			}
			if resultHasRunSummary(result) {
				if passErr := callRunPass(input.OnPass, result); passErr != nil {
					stats.StopReason = ""
					return RunLoopResult{Stats: stats}, passErr
				}
			}
			return RunLoopResult{Stats: stats}, err
		}

		stats.Passes++
		if err := callRunPass(input.OnPass, result); err != nil {
			stats.StopReason = ""
			return RunLoopResult{Stats: stats}, err
		}
		if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
			stats.NoTask = true
			stats.ConsecutiveFailedOrBlocked = 0
			stats.StopReason = "no_task"
			return RunLoopResult{Stats: stats}, runLoopFailureError(stats)
		}

		if err := RunOnceOutcomeError(result); err != nil {
			stats.FailedOrBlocked++
			stats.ConsecutiveFailedOrBlocked++
			if loopFailureRequiresInspection(result) {
				stats.StopReason = "failed_or_blocked"
				return RunLoopResult{Stats: stats}, err
			}
			if stats.ConsecutiveFailedOrBlocked >= loopFailureGuardrailLimit {
				stats.StopReason = "failure_guardrail"
				return RunLoopResult{Stats: stats}, runLoopGuardrailError{ConsecutiveFailedOrBlocked: stats.ConsecutiveFailedOrBlocked}
			}
			continue
		}

		if result.Outcome == runonce.OutcomeCommitted {
			stats.Completed++
		}
		stats.ConsecutiveFailedOrBlocked = 0
	}

	stats.StopReason = "max_passes"
	return RunLoopResult{Stats: stats}, runLoopFailureError(stats)
}

func loadConfiguredRunOnce(cfg Config, progress RunProgress) (runonce.Config, error) {
	runCfg, err := LoadRunOnceConfig(cfg.WorkDir, DefaultRunOnceConfig(cfg.WorkDir))
	if err != nil {
		return runonce.Config{}, err
	}
	if progress != nil {
		runCfg.CodexProgress = progress
	}
	return runCfg, nil
}

func runWithConfig(ctx context.Context, runner RunOnceRunner, runCfg runonce.Config) (runonce.Result, error) {
	if runner == nil {
		runner = runonce.Run
	}
	return runner(ctx, runCfg)
}

func callRunPass(onPass RunPassFunc, result runonce.Result) error {
	if onPass == nil {
		return nil
	}
	return onPass(result)
}

type runOnceError struct {
	RunID   string
	Outcome runonce.Outcome
}

func (e runOnceError) Error() string {
	if e.RunID == "" {
		return fmt.Sprintf("run stopped with outcome %s", e.Outcome)
	}
	return fmt.Sprintf("run %s stopped with outcome %s", e.RunID, e.Outcome)
}

func RunOnceOutcomeError(result runonce.Result) error {
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask || result.Outcome == runonce.OutcomeCommitted || result.Outcome == "" {
		return nil
	}
	return runOnceError{RunID: result.Run.ID, Outcome: result.Outcome}
}

type runLoopError struct {
	FailedOrBlocked int
}

func (e runLoopError) Error() string {
	return fmt.Sprintf("run loop completed with %d failed or blocked %s", e.FailedOrBlocked, pluralPass(e.FailedOrBlocked))
}

type runLoopGuardrailError struct {
	ConsecutiveFailedOrBlocked int
}

func (e runLoopGuardrailError) Error() string {
	return fmt.Sprintf("run loop stopped after %d consecutive failed or blocked %s", e.ConsecutiveFailedOrBlocked, pluralPass(e.ConsecutiveFailedOrBlocked))
}

func runLoopFailureError(stats RunLoopStats) error {
	if stats.FailedOrBlocked == 0 {
		return nil
	}
	return runLoopError{FailedOrBlocked: stats.FailedOrBlocked}
}

func resultHasRunSummary(result runonce.Result) bool {
	return result.NoTask || result.Outcome != "" || result.Run.ID != "" || result.Task.ID != "" || strings.TrimSpace(result.Message) != ""
}

func loopFailureRequiresInspection(result runonce.Result) bool {
	if result.Outcome == runonce.OutcomeBlocked {
		return true
	}
	if strings.TrimSpace(result.PostRunChanged.CaptureError) != "" {
		return true
	}
	return len(result.PostRunChanged.ChangedFiles) > 0 || len(result.PostRunChanged.Paths) > 0
}

func pluralPass(count int) string {
	if count == 1 {
		return "pass"
	}
	return "passes"
}
