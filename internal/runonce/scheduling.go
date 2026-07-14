package runonce

import (
	"context"
	"fmt"
	"os"
	"strings"

	"revolvr/internal/autonomousarchive"
	"revolvr/internal/ledger"
	"revolvr/internal/taskschedule"
	"revolvr/internal/taskscheduler"
)

// ScheduleError preserves the shared typed diagnostics that refused
// execution. Callers can render richer detail without reconstructing policy.
type ScheduleError struct {
	Diagnostics []taskscheduler.Diagnostic
}

func (e ScheduleError) Error() string {
	parts := make([]string, 0, len(e.Diagnostics))
	for _, diagnostic := range e.Diagnostics {
		parts = append(parts, string(diagnostic.Code)+": "+diagnostic.Detail)
	}
	return "run once: invalid task graph: " + strings.Join(parts, "; ")
}

// TerminalDependencyError distinguishes permanently unsatisfied dependency
// edges from an ordinary valid queue that is temporarily waiting or empty.
type TerminalDependencyError struct {
	Tasks []taskscheduler.TaskReadiness
}

func (e TerminalDependencyError) Error() string {
	parts := make([]string, 0, len(e.Tasks))
	for _, task := range e.Tasks {
		parts = append(parts, fmt.Sprintf("%s (%s)", task.TaskID, strings.Join(task.UnmetDependencyIDs, ",")))
	}
	return "run once: terminal dependencies are unsatisfied: " + strings.Join(parts, "; ")
}

func evaluateMixedSchedule(ctx context.Context, cfg Config, workDir string, runs *ledger.Store) (taskschedule.Snapshot, error) {
	archiveRunner := autonomousarchive.CommandRunner(nil)
	if cfg.CommandRunner != nil {
		archiveRunner = autonomousarchive.CommandRunner(cfg.CommandRunner)
	}
	return taskschedule.Load(ctx, taskschedule.Config{
		RepositoryRoot:    workDir,
		SelectionWorkflow: taskscheduler.WorkflowMixedPassV1,
		Ledger:            runs,
		GitExecutable:     cfg.GitExecutable,
		GitTimeout:        cfg.GitTimeout,
		CommandRunner:     archiveRunner,
		ForbiddenValues:   schedulingSecretValues(cfg),
	})
}

func schedulingSecretValues(cfg Config) []string {
	values := make([]string, 0, len(cfg.SafetyDeclaration.Redaction.EnvironmentVariables))
	for _, name := range cfg.SafetyDeclaration.Redaction.EnvironmentVariables {
		if value, ok := os.LookupEnv(strings.TrimSpace(name)); ok && value != "" {
			values = append(values, value)
		}
	}
	return values
}
