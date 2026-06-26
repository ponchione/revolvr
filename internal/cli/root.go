package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"revolvr/internal/runonce"
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
		newInitCommand(),
		newTaskCommand(),
		newRunCommand(opts),
		newStatusCommand(),
		newShowCommand(),
	)

	return root
}

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize revolvr state",
		Args:  cobra.NoArgs,
		RunE:  runPlaceholder,
	}
}

func newTaskCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
		Args:  cobra.NoArgs,
		RunE:  runPlaceholder,
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

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show harness status",
		Args:  cobra.NoArgs,
		RunE:  runPlaceholder,
	}
}

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <run-id>",
		Short: "Show one run",
		Args:  cobra.ExactArgs(1),
		RunE:  runPlaceholder,
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
