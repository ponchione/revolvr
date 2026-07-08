package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"revolvr/internal/codexexec"
	"revolvr/internal/ledger"
	"revolvr/internal/receipt"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
)

const defaultVersion = "dev"

type Options struct {
	Version             string
	Out                 io.Writer
	Err                 io.Writer
	WorkDir             string
	RunOnce             RunOnceFunc
	DoctorCommandRunner DoctorCommandRunner
	ExecutableLookPath  ExecutableLookPath
}

type RunOnceFunc func(context.Context, runonce.Config) (runonce.Result, error)

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
			taskText := strings.TrimSpace(strings.Join(args, " "))
			if taskText == "" {
				return fmt.Errorf("task add: task text is required")
			}

			store, closeStore, err := openTaskStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeStore()

			task, err := store.AddTask(cmd.Context(), taskqueue.TaskSpec{
				Task:    taskText,
				Summary: strings.TrimSpace(summary),
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
			store, closeStore, err := openTaskStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeStore()

			tasks, err := store.ListTasks(cmd.Context())
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
	taskID := strings.TrimSpace(rawTaskID)
	if taskID == "" {
		return fmt.Errorf("%s: task id is required", operation)
	}

	store, closeStore, err := openTaskStore(cmd.Context(), opts)
	if err != nil {
		return err
	}
	defer closeStore()

	task, changed, err := store.UnblockTask(cmd.Context(), taskID)
	if err != nil {
		return err
	}
	if !changed {
		if task.ID == "" {
			return fmt.Errorf("task %q not found", taskID)
		}
		return fmt.Errorf("task %q is not blocked (status: %s)", taskID, task.Status)
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
	runCfg, err := loadRunOnceConfig(workDir, defaultRunOnceConfig(workDir))
	if err != nil {
		return err
	}
	runCfg = withRunProgress(runCfg, cmd.OutOrStdout())
	result, err := runOnce(cmd.Context(), runCfg)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result)); err != nil {
		return err
	}
	return runOnceOutcomeError(result)
}

func runBoundedLoop(cmd *cobra.Command, workDir string, runOnce RunOnceFunc, maxPasses int) error {
	if maxPasses <= 0 {
		return fmt.Errorf("run: --max-passes must be greater than 0")
	}
	for pass := 0; pass < maxPasses; pass++ {
		if err := cmd.Context().Err(); err != nil {
			return err
		}
		runCfg, err := loadRunOnceConfig(workDir, defaultRunOnceConfig(workDir))
		if err != nil {
			return err
		}
		runCfg = withRunProgress(runCfg, cmd.OutOrStdout())
		result, err := runOnce(cmd.Context(), runCfg)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result)); err != nil {
			return err
		}
		if err := runOnceOutcomeError(result); err != nil {
			return err
		}
		if result.NoTask || result.Outcome == runonce.OutcomeNoTask {
			return nil
		}
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "Reached max passes (%d).\n", maxPasses)
	return err
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

func runOnceOutcomeError(result runonce.Result) error {
	if result.NoTask || result.Outcome == runonce.OutcomeNoTask || result.Outcome == runonce.OutcomeCommitted || result.Outcome == "" {
		return nil
	}
	return runOnceError{RunID: result.Run.ID, Outcome: result.Outcome}
}

func newStatusCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show harness status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := resolveStatePaths(opts.WorkDir)
			if err != nil {
				return err
			}
			initialized, err := stateInitialized(paths)
			if err != nil {
				return err
			}
			if !initialized {
				_, err = fmt.Fprint(cmd.OutOrStdout(), "Not initialized. Run `revolvr init` first.\n")
				return err
			}

			tasks, closeTasks, err := openTaskStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeTasks()

			runs, closeRuns, err := openLedgerStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeRuns()

			taskList, err := tasks.ListTasks(cmd.Context())
			if err != nil {
				return err
			}
			recentRuns, err := runs.ListRecentRuns(cmd.Context(), 20)
			if err != nil {
				return err
			}
			var latestEvents []ledger.Event
			if len(recentRuns) > 0 {
				latestHistory, ok, err := runs.GetRunWithEvents(cmd.Context(), recentRuns[0].ID)
				if err != nil {
					return err
				}
				if ok {
					latestEvents = latestHistory.Events
				}
			}
			return writeStatus(cmd.OutOrStdout(), taskList, recentRuns, latestEvents)
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

			paths, err := resolveStatePaths(opts.WorkDir)
			if err != nil {
				return err
			}
			initialized, err := ledgerInitialized(paths)
			if err != nil {
				return err
			}
			if !initialized {
				return fmt.Errorf("state is not initialized; run `revolvr init` first")
			}

			runs, closeRuns, err := openLedgerStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeRuns()

			history, ok, err := runs.GetRunWithEvents(cmd.Context(), runID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("run %q not found", runID)
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
			runID := strings.TrimSpace(args[0])
			if runID == "" {
				return fmt.Errorf("receipt validate: run id is required")
			}

			paths, err := resolveStatePaths(opts.WorkDir)
			if err != nil {
				return err
			}
			initialized, err := ledgerInitialized(paths)
			if err != nil {
				return err
			}
			if !initialized {
				return fmt.Errorf("state is not initialized; run `revolvr init` first")
			}

			runs, closeRuns, err := openLedgerStore(cmd.Context(), opts)
			if err != nil {
				return err
			}
			defer closeRuns()

			history, ok, err := runs.GetRunWithEvents(cmd.Context(), runID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("run %q not found", runID)
			}

			result, err := receipt.ValidateRunReceipt(receipt.ValidationInput{
				WorkDir: paths.WorkDir,
				History: history,
			})
			if err != nil {
				return err
			}
			if err := writeReceiptValidation(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !result.Passed() {
				return receiptValidationError{RunID: runID, FailureCount: len(result.Failures())}
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
	if result.NoTask {
		return "No pending runnable tasks.\n"
	}
	switch result.Outcome {
	case runonce.OutcomeCommitted:
		return fmt.Sprintf("Run %s completed task %s; commit %s.\n", result.Run.ID, result.Task.ID, result.Commit.CommitSHA)
	default:
		return fmt.Sprintf("Run %s stopped (%s): %s\n", result.Run.ID, result.Outcome, result.Message)
	}
}

func withRunProgress(cfg runonce.Config, out io.Writer) runonce.Config {
	if out == nil {
		return cfg
	}
	var mu sync.Mutex
	cfg.CodexProgress = func(event codexexec.ProgressEvent) {
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
	return cfg
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
