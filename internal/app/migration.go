package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousmigration"
	revolvrlock "revolvr/internal/lock"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskschedule"
)

type MigrationPlanInput struct {
	TargetWorkflow string
	TaskIDs        []string
	All            bool
	DryRun         bool
}

// PlanTaskMigration builds one all-or-nothing read-only migration projection.
// Applying or recovering the projected writes is owned by AM-02.
func PlanTaskMigration(ctx context.Context, cfg Config, input MigrationPlanInput) (autonomousmigration.Plan, error) {
	if err := validateMigrationInput(input); err != nil {
		return autonomousmigration.Plan{}, err
	}

	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return autonomousmigration.Plan{}, err
	}
	runs, closeRuns, err := openSchedulingLedger(ctx, paths)
	if err != nil {
		return autonomousmigration.Plan{}, err
	}
	defer closeRuns()
	snapshot, err := loadTaskSchedule(ctx, paths.WorkDir, runs)
	if err != nil {
		return autonomousmigration.Plan{}, err
	}
	return autonomousmigration.Build(paths.WorkDir, snapshot, autonomousmigration.Request{
		TaskIDs: append([]string(nil), input.TaskIDs...), All: input.All, DryRun: input.DryRun,
	})
}

// ApplyTaskMigration serializes migration against autonomous execution and
// source writers, recovers an existing exact operation before fresh planning,
// and publishes no lifecycle evidence beyond the canonical pending state.
func ApplyTaskMigration(ctx context.Context, cfg Config, input MigrationPlanInput) (result autonomousmigration.ApplyResult, resultErr error) {
	if err := validateMigrationInput(input); err != nil {
		return result, err
	}
	if input.DryRun {
		return result, errors.New("task migrate: dry-run must use the read-only planning path")
	}
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return result, err
	}
	initialized, err := stateInitialized(paths)
	if err != nil {
		return result, err
	}
	if !initialized {
		return result, errors.New("task migrate: state is not initialized; run `revolvr init` first")
	}
	releaseExecution, err := autonomousexec.TryAcquire(paths.WorkDir)
	if err != nil {
		return result, fmt.Errorf("task migrate: acquire autonomous execution lease: %w", err)
	}
	defer releaseExecution()

	now := time.Now().UTC()
	writer, err := revolvrlock.AcquireSourceWriter(ctx, revolvrlock.Config{
		WorkingDir: paths.WorkDir,
		RunID:      fmt.Sprintf("migration-%d-%d", os.Getpid(), now.UnixNano()),
		PID:        os.Getpid(),
	})
	if err != nil {
		return result, fmt.Errorf("task migrate: acquire source writer: %w", err)
	}
	defer func() {
		resultErr = errors.Join(resultErr, writer.Release(context.WithoutCancel(ctx)))
	}()

	runs, closeRuns, err := openSchedulingLedger(ctx, paths)
	if err != nil {
		return result, err
	}
	defer closeRuns()

	request := autonomousmigration.Request{TaskIDs: append([]string(nil), input.TaskIDs...), All: input.All}
	plan, authority, found, err := autonomousmigration.FindPlan(ctx, paths.WorkDir, request)
	if err != nil {
		return result, err
	}
	createdAt := now
	if found {
		createdAt = authority.CreatedAt
	} else {
		snapshot, loadErr := loadTaskSchedule(ctx, paths.WorkDir, runs)
		if loadErr != nil {
			return result, loadErr
		}
		request.AllowExactOrphanState = true
		plan, err = autonomousmigration.Build(paths.WorkDir, snapshot, request)
		if err != nil {
			return result, err
		}
	}
	if _, err := taskschedule.LoadActiveStrict(ctx, paths.WorkDir); err != nil {
		return result, fmt.Errorf("task migrate: pre-publication autonomous state validation: %w", err)
	}
	result, err = autonomousmigration.Apply(ctx, autonomousmigration.ApplyInput{
		RepositoryRoot: paths.WorkDir,
		Plan:           plan,
		CreatedAt:      createdAt,
	})
	if err != nil {
		return result, err
	}
	if _, err := taskschedule.LoadActiveStrict(ctx, paths.WorkDir); err != nil {
		return result, fmt.Errorf("task migrate: post-publication autonomous state validation: %w", err)
	}
	return result, nil
}

func validateMigrationInput(input MigrationPlanInput) error {
	target := strings.TrimSpace(input.TargetWorkflow)
	if target == "" {
		return errors.New("task migrate: --to is required")
	}
	if input.TargetWorkflow != target {
		return errors.New("task migrate: --to must not have surrounding whitespace")
	}
	if target != taskfile.WorkflowAutonomousV1 {
		return errors.New("task migrate: --to must be autonomous-v1")
	}
	if input.All && len(input.TaskIDs) != 0 {
		return errors.New("plan autonomous migration: --all cannot be combined with task IDs")
	}
	if !input.All && len(input.TaskIDs) == 0 {
		return errors.New("plan autonomous migration: provide at least one task ID or --all")
	}
	return nil
}
