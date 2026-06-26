package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
)

const defaultVersion = "dev"

type Options struct {
	Version string
	Out     io.Writer
	Err     io.Writer
	WorkDir string
	RunOnce RunOnceFunc
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
		newRunCommand(opts),
		newStatusCommand(opts),
		newShowCommand(opts),
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
		RunE:  runPlaceholder,
	}
	cmd.AddCommand(
		newTaskAddCommand(opts),
		newTaskListCommand(opts),
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

func newRunCommand(opts Options) *cobra.Command {
	runOnce := opts.RunOnce
	if runOnce == nil {
		runOnce = runonce.Run
	}
	var once bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one harness pass",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !once {
				return runPlaceholder(cmd, nil)
			}
			result, err := runOnce(cmd.Context(), runonce.Config{WorkingDir: opts.WorkDir})
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), runOnceSummary(result))
			return err
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "run one selected task")
	return cmd
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
			return writeStatus(cmd.OutOrStdout(), taskList, recentRuns)
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

func runPlaceholder(cmd *cobra.Command, _ []string) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s is not implemented yet.\n", cmd.CommandPath())
	return err
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

func oneLine(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

type taskCounts struct {
	total     int
	pending   int
	blocked   int
	completed int
}

func writeStatus(out io.Writer, tasks []taskqueue.Task, recentRuns []ledger.Run) error {
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
	_, err := fmt.Fprintf(out, "Latest run: %s (%s)\n", recentRuns[0].ID, recentRuns[0].Status)
	return err
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

func cliTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
