package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"revolvr/internal/app"
	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
	tuiapp "revolvr/internal/tui"
)

const defaultVersion = "dev"

type Options struct {
	Version             string
	Out                 io.Writer
	Err                 io.Writer
	WorkDir             string
	RunOnce             RunOnceFunc
	TUIRunner           TUIRunFunc
	DoctorCommandRunner DoctorCommandRunner
	ExecutableLookPath  ExecutableLookPath
}

type RunOnceFunc = app.RunOnceRunner
type TUIRunFunc func(context.Context, app.StatusResult, tuiapp.RunOptions) error

func NewRootCommand(opts Options) *cobra.Command {
	version := opts.Version
	if version == "" {
		version = defaultVersion
	}

	root := &cobra.Command{
		Use:           "revolvr",
		Short:         "Run bounded Codex harness passes",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.SetVersionTemplate("revolvr {{.Version}}\n")
	root.CompletionOptions.DisableDefaultCmd = true

	if opts.Out != nil {
		root.SetOut(opts.Out)
	}
	if opts.Err != nil {
		root.SetErr(opts.Err)
	}

	root.AddCommand(
		newInitCommand(opts),
		newTaskCommand(opts),
		newConfigCommand(opts),
		newRunCommand(opts),
		newDoctorCommand(opts),
		newStatusCommand(opts),
		newTUICommand(opts),
		newShowCommand(opts),
		newReceiptCommand(opts),
	)

	return root
}

func newInitCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize revolvr state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := resolveStatePaths(opts.WorkDir)
			if err != nil {
				return err
			}
			if err := initializeState(cmd.Context(), paths); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"Initialized revolvr state:\nState: %s\nTasks: %s\nLedger: %s\nRuns: %s\nReceipts: %s\nLocks: %s\n",
				paths.StateDir,
				paths.TaskDBPath,
				paths.LedgerDBPath,
				paths.RunsDir,
				paths.ReceiptsDir,
				paths.LocksDir,
			)
			return err
		},
	}
}

func newTaskCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(
		newTaskAddCommand(opts),
		newTaskListCommand(opts),
		newTaskRetryCommand(opts),
		newTaskUnblockCommand(opts),
	)
	return cmd
}

func newTaskAddCommand(opts Options) *cobra.Command {
	var summary string
	cmd := &cobra.Command{
		Use:   "add <task text>",
		Short: "Add a task",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := app.AddTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, app.AddTaskInput{
				Task:    strings.Join(args, " "),
				Summary: summary,
			})
			if err != nil {
				return err
			}
			if task.Summary != "" {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added task %s: %s (summary: %s)\n", task.ID, task.Task, task.Summary)
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added task %s: %s\n", task.ID, task.Task)
			return err
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "short task summary")
	return cmd
}

func newTaskListCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tasks, err := app.ListTasks(cmd.Context(), app.Config{WorkDir: opts.WorkDir})
			if err != nil {
				return err
			}
			if len(tasks) == 0 {
				_, err = fmt.Fprint(cmd.OutOrStdout(), "No tasks.\n")
				return err
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), "ID\tSTATUS\tTASK\tSUMMARY\n"); err != nil {
				return err
			}
			for _, task := range tasks {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", task.ID, task.Status, oneLine(task.Task), oneLine(task.Summary)); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newTaskUnblockCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "unblock <task-id>",
		Short: "Make a blocked task pending again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return retryBlockedTask(cmd, opts, args[0], "task unblock", "Unblocked")
		},
	}
}

func newTaskRetryCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "retry <task-id>",
		Short: "Retry a blocked task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return retryBlockedTask(cmd, opts, args[0], "task retry", "Retried")
		},
	}
}

func retryBlockedTask(cmd *cobra.Command, opts Options, rawTaskID string, operation string, successVerb string) error {
	var (
		task taskqueue.Task
		err  error
	)
	switch operation {
	case "task retry":
		task, err = app.RetryTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, rawTaskID)
	case "task unblock":
		task, err = app.UnblockTask(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, rawTaskID)
	default:
		err = fmt.Errorf("%s: unsupported task operation", operation)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s task %s.\n", successVerb, task.ID)
	return err
}

func newConfigCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect run configuration",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(newConfigCheckCommand(opts))
	return cmd
}

func newConfigCheckCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Validate and show effective run configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := checkRunConfig(opts.WorkDir)
			if err != nil {
				return err
			}
			return writeConfigCheck(cmd.OutOrStdout(), result)
		},
	}
}

func newRunCommand(opts Options) *cobra.Command {
	runOnce := opts.RunOnce
	if runOnce == nil {
		runOnce = runonce.Run
	}
	var once bool
	var maxPasses int
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one harness pass",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if once {
				return runSinglePass(cmd, opts.WorkDir, runOnce)
			}
			if cmd.Flags().Changed("max-passes") {
				return runBoundedLoop(cmd, opts.WorkDir, runOnce, maxPasses)
			}
			return runPlaceholder(cmd, nil)
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one selected task")
	cmd.Flags().IntVar(&maxPasses, "max-passes", 0, "run up to N fresh passes")
	return cmd
}

func runSinglePass(cmd *cobra.Command, workDir string, runOnce RunOnceFunc) error {
	result, err := app.RunOnce(cmd.Context(), app.Config{WorkDir: workDir}, app.RunOnceInput{
		Runner:   runOnce,
		Progress: runProgress(cmd.OutOrStdout()),
	})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result)); err != nil {
		return err
	}
	return app.RunOnceOutcomeError(result)
}

func runBoundedLoop(cmd *cobra.Command, workDir string, runOnce RunOnceFunc, maxPasses int) error {
	result, err := app.RunLoop(cmd.Context(), app.Config{WorkDir: workDir}, app.RunLoopInput{
		MaxPasses: maxPasses,
		Runner:    runOnce,
		Progress:  runProgress(cmd.OutOrStdout()),
		OnPass: func(result runonce.Result) error {
			_, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result))
			return err
		},
	})
	if strings.TrimSpace(result.Stats.StopReason) != "" {
		if writeErr := writeRunLoopSummary(cmd.OutOrStdout(), result.Stats); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func writeRunLoopSummary(out io.Writer, stats app.RunLoopStats) error {
	if strings.TrimSpace(stats.StopReason) == "" {
		stats.StopReason = "unknown"
	}
	_, err := fmt.Fprintf(out,
		"Loop summary: passes=%d/%d completed=%d failed_or_blocked=%d no_task=%t stop=%s\n",
		stats.Passes,
		stats.MaxPasses,
		stats.Completed,
		stats.FailedOrBlocked,
		stats.NoTask,
		stats.StopReason,
	)
	return err
}

func newStatusCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show harness status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			status, err := app.Status(cmd.Context(), app.Config{WorkDir: opts.WorkDir})
			if err != nil {
				return err
			}
			if !status.Initialized {
				_, err = fmt.Fprint(cmd.OutOrStdout(), "Not initialized. Run `revolvr init` first.\n")
				return err
			}
			return writeStatus(cmd.OutOrStdout(), status.Tasks, status.RecentRuns, status.LatestEvents)
		},
	}
}

func newTUICommand(opts Options) *cobra.Command {
	runner := opts.TUIRunner
	if runner == nil {
		runner = tuiapp.RunStatus
	}
	return &cobra.Command{
		Use:   "tui",
		Short: "Open status TUI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := app.Config{WorkDir: opts.WorkDir}
			ctx := cmd.Context()
			status, err := app.Status(ctx, cfg)
			if err != nil {
				return err
			}
			return runner(cmd.Context(), status, tuiapp.RunOptions{
				Input:  cmd.InOrStdin(),
				Output: cmd.OutOrStdout(),
				RefreshStatus: func() (app.StatusResult, error) {
					return app.Status(ctx, cfg)
				},
				OpenRun: func(runID string) (ledger.RunWithEvents, error) {
					return app.ShowRun(ctx, cfg, runID)
				},
			})
		},
	}
}

func newShowCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show one run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runID := strings.TrimSpace(args[0])
			if runID == "" {
				return fmt.Errorf("show: run id is required")
			}

			history, err := app.ShowRun(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, runID)
			if err != nil {
				return err
			}
			return writeRun(cmd.OutOrStdout(), history)
		},
	}
}

func newReceiptCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "receipt",
		Short: "Inspect and validate receipts",
		Args:  cobra.NoArgs,
		RunE:  runHelp,
	}
	cmd.AddCommand(newReceiptValidateCommand(opts))
	return cmd
}

func newReceiptValidateCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "validate <run-id>",
		Short: "Validate one run receipt against the ledger",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := app.ValidateReceipt(cmd.Context(), app.Config{WorkDir: opts.WorkDir}, args[0])
			if err != nil {
				return err
			}
			if err := writeReceiptValidation(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !result.Passed() {
				return receiptValidationError{RunID: result.RunID, FailureCount: len(result.Failures())}
			}
			return nil
		},
	}
}

func runPlaceholder(cmd *cobra.Command, _ []string) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s is not implemented yet.\n", cmd.CommandPath())
	return err
}

func runHelp(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func runOnceSummary(result runonce.Result) string {
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
		return "No pending runnable tasks.\n"
	}
	switch result.Outcome {
	case runonce.OutcomeCommitted:
		return fmt.Sprintf("Run %s completed task %s; commit %s.\n", result.Run.ID, result.Task.ID, result.Commit.CommitSHA)
	default:
		return fmt.Sprintf("Run %s stopped (%s): %s\n", result.Run.ID, result.Outcome, result.Message)
	}
}

func runProgress(out io.Writer) app.RunProgress {
	if out == nil {
		return nil
	}
	var mu sync.Mutex
	return func(event codexexec.ProgressEvent) {
		event.Source = strings.TrimSpace(event.Source)
		event.Message = strings.TrimSpace(event.Message)
		if event.Message == "" {
			return
		}
		if event.Source == "" {
			event.Source = "codex"
		}
		mu.Lock()
		defer mu.Unlock()
		_, _ = fmt.Fprintf(out, "%s: %s\n", event.Source, event.Message)
	}
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

type taskCounts struct {
	total     int
	pending   int
	blocked   int
	completed int
}

func writeStatus(out io.Writer, tasks []taskqueue.Task, recentRuns []ledger.Run, latestEvents []ledger.Event) error {
	counts := countTasks(tasks)
	if _, err := fmt.Fprintf(out, "Total tasks: %d\n", counts.total); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Pending tasks: %d\n", counts.pending); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Blocked tasks: %d\n", counts.blocked); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Completed tasks: %d\n", counts.completed); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Recent runs: %d\n", len(recentRuns)); err != nil {
		return err
	}
	if len(recentRuns) == 0 {
		_, err := fmt.Fprint(out, "Latest run: none\n")
		return err
	}
	return writeLatestRunStatus(out, recentRuns[0], latestEvents)
}

func writeLatestRunStatus(out io.Writer, run ledger.Run, events []ledger.Event) error {
	if _, err := fmt.Fprintf(out, "Latest run: %s (%s)\n", run.ID, run.Status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest summary: %s\n", optionalStatusValue(run.Summary)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest verification: %s\n", optionalStatusValue(run.VerificationStatus)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Latest commit: %s\n", optionalStatusValue(run.CommitSHA)); err != nil {
		return err
	}
	return writeLatestArtifacts(out, events)
}

func optionalStatusValue(value string) string {
	value = oneLine(value)
	if value == "" {
		return "none"
	}
	return value
}

func writeLatestArtifacts(out io.Writer, events []ledger.Event) error {
	artifacts, found := ledger.RunArtifactsFromEvents(events)
	if !found || artifacts.Empty() {
		_, err := fmt.Fprint(out, "Latest artifacts: none\n")
		return err
	}
	if _, err := fmt.Fprint(out, "Latest artifacts:\n"); err != nil {
		return err
	}
	return writeArtifactPathLines(out, artifacts)
}

func countTasks(tasks []taskqueue.Task) taskCounts {
	counts := taskCounts{total: len(tasks)}
	for _, task := range tasks {
		switch task.Status {
		case taskqueue.StatusPending:
			counts.pending++
		case taskqueue.StatusBlocked:
			counts.blocked++
		case taskqueue.StatusCompleted:
			counts.completed++
		}
	}
	return counts
}

func writeRun(out io.Writer, history ledger.RunWithEvents) error {
	run := history.Run
	if _, err := fmt.Fprintf(out, "Run ID: %s\n", run.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Task ID: %s\n", run.TaskID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Task: %s\n", oneLine(run.Task)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Status: %s\n", run.Status); err != nil {
		return err
	}
	if run.Summary != "" {
		if _, err := fmt.Fprintf(out, "Summary: %s\n", oneLine(run.Summary)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "Started at: %s\n", cliTime(run.StartedAt)); err != nil {
		return err
	}
	if run.CompletedAt != nil {
		if _, err := fmt.Fprintf(out, "Completed at: %s\n", cliTime(*run.CompletedAt)); err != nil {
			return err
		}
	}
	if run.CodexExitCode != nil {
		if _, err := fmt.Fprintf(out, "Codex exit code: %d\n", *run.CodexExitCode); err != nil {
			return err
		}
	}
	if run.VerificationStatus != "" {
		if _, err := fmt.Fprintf(out, "Verification status: %s\n", run.VerificationStatus); err != nil {
			return err
		}
	}
	if run.CommitSHA != "" {
		if _, err := fmt.Fprintf(out, "Commit SHA: %s\n", run.CommitSHA); err != nil {
			return err
		}
	}
	artifacts, artifactEvents := ledger.RunArtifactsFromEvents(history.Events)
	if artifactEvents {
		if err := writeArtifacts(out, artifacts); err != nil {
			return err
		}
	}
	if err := writeDiagnostics(out, diagnosticsFromHistory(history)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Events:\n"); err != nil {
		return err
	}
	if len(history.Events) == 0 {
		_, err := fmt.Fprint(out, "No events.\n")
		return err
	}
	if _, err := fmt.Fprint(out, "ID\tTYPE\tTIMESTAMP\n"); err != nil {
		return err
	}
	for _, event := range history.Events {
		if _, err := fmt.Fprintf(out, "%d\t%s\t%s\n", event.ID, event.Type, cliTime(event.CreatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func writeArtifacts(out io.Writer, artifacts ledger.RunArtifacts) error {
	if _, err := fmt.Fprint(out, "Artifacts:\n"); err != nil {
		return err
	}
	if artifacts.Empty() {
		_, err := fmt.Fprint(out, "none\n")
		return err
	}
	return writeArtifactPathLines(out, artifacts)
}

func writeArtifactPathLines(out io.Writer, artifacts ledger.RunArtifacts) error {
	for _, artifact := range []struct {
		label string
		path  string
	}{
		{label: "prompt", path: artifacts.PromptPath},
		{label: "codex stdout jsonl", path: artifacts.CodexStdoutJSONLPath},
		{label: "codex stderr", path: artifacts.CodexStderrPath},
		{label: "last message", path: artifacts.LastMessagePath},
		{label: "receipt", path: artifacts.ReceiptPath},
	} {
		if strings.TrimSpace(artifact.path) == "" {
			continue
		}
		if _, err := fmt.Fprintf(out, "%s: %s\n", artifact.label, artifact.path); err != nil {
			return err
		}
	}
	return nil
}

func cliTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

type receiptValidationError struct {
	RunID        string
	FailureCount int
}

func (e receiptValidationError) Error() string {
	return fmt.Sprintf("receipt validation failed for run %s (%d failed checks)", e.RunID, e.FailureCount)
}

func writeReceiptValidation(out io.Writer, result receipt.ValidationResult) error {
	status := "passed"
	if !result.Passed() {
		status = "failed"
	}
	if _, err := fmt.Fprintf(out, "Receipt validation: %s\n", status); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Run ID: %s\n", result.RunID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "Receipt: %s\n", result.ReceiptPath); err != nil {
		return err
	}
	if _, err := fmt.Fprint(out, "Checks:\n"); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(out, "%s: %s\n", check.Name, check.Message()); err != nil {
			return err
		}
	}
	return nil
}
