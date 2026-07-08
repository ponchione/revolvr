package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"revolvr/internal/app"
)

type DoctorCommandRunner = app.PreflightCommandRunner
type ExecutableLookPath = app.ExecutableLookPath

type doctorFailedError struct{}

func (doctorFailedError) Error() string {
	return "doctor: preflight failed"
}

func newDoctorCommand(opts Options) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check readiness for dogfooding",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := runDoctor(cmd.Context(), opts.WorkDir, app.PreflightInput{
				CommandRunner: opts.DoctorCommandRunner,
				LookPath:      opts.ExecutableLookPath,
			})
			if err != nil {
				return err
			}
			if err := writeDoctor(cmd.OutOrStdout(), result); err != nil {
				return err
			}
			if !result.Ready {
				return doctorFailedError{}
			}
			return nil
		},
	}
}

func runDoctor(ctx context.Context, workDir string, input app.PreflightInput) (app.PreflightResult, error) {
	return app.Preflight(ctx, app.Config{WorkDir: workDir}, input)
}

func writeDoctor(out io.Writer, result app.PreflightResult) error {
	if _, err := fmt.Fprintln(out, "Dogfood preflight:"); err != nil {
		return err
	}
	for _, check := range result.Checks {
		if _, err := fmt.Fprintf(out, "%s %s: %s\n", check.Status, check.Name, check.Detail); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(out, "Ready: %t\n", result.Ready)
	return err
}
