package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const defaultVersion = "dev"

type Options struct {
	Version string
	Out     io.Writer
	Err     io.Writer
}

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
		newRunCommand(),
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

func newRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one harness pass",
		Args:  cobra.NoArgs,
		RunE:  runPlaceholder,
	}
	cmd.Flags().Bool("once", false, "run one selected task")
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
