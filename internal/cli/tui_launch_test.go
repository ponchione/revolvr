package cli

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"testing"

	tuiapp "revolvr/internal/tui"
)

const acceptedRootHelp = `Run bounded Codex harness passes

Usage:
  revolvr [flags]
  revolvr [command]

Available Commands:
  archive      Archive, inspect, verify, and reopen terminal tasks
  artifact     Plan, apply, and inspect artifact retention
  checkpoint   Manage pre-authored operator checkpoints
  config       Inspect run configuration
  doctor       Check readiness for dogfooding
  help         Help about any command
  init         Initialize revolvr state
  ledger       Export and validate immutable ledger history
  metrics      Project autonomous-loop metrics from ledger evidence
  notification Inspect durable external notification deliveries
  queue        Manage manually started bounded sequential queues
  receipt      Inspect and validate receipts
  run          Run one harness pass
  show         Show one run
  status       Show harness status
  task         Manage tasks
  tui          Open the Revolvr TUI

Flags:
  -h, --help      help for revolvr
  -v, --version   version for revolvr

Use "revolvr [command] --help" for more information about a command.
`

const acceptedTUIHelp = `Open the Revolvr TUI.

Bare revolvr and revolvr tui are equivalent when stdin and stdout are
terminals. Use an existing subcommand for non-interactive work.

Usage:
  revolvr tui [flags]

Flags:
  -h, --help   help for tui
`

func TestTUILaunchRedirectedIOMatrix(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		stdinTTY      bool
		stdoutTTY     bool
		wantStdout    string
		wantStderr    string
		wantExit      int
		wantChecks    []string
		wantBootstrap int
	}{
		{name: "bare interactive", stdinTTY: true, stdoutTTY: true, wantStdout: "managed tui\n", wantChecks: []string{"stdin", "stdout"}, wantBootstrap: 1},
		{name: "explicit interactive", args: []string{"tui"}, stdinTTY: true, stdoutTTY: true, wantStdout: "managed tui\n", wantChecks: []string{"stdin", "stdout"}, wantBootstrap: 1},
		{name: "bare stdin redirected", stdoutTTY: true, wantStderr: "stdin is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin"}},
		{name: "explicit stdin redirected", args: []string{"tui"}, stdoutTTY: true, wantStderr: "stdin is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin"}},
		{name: "bare stdout redirected", stdinTTY: true, wantStderr: "stdout is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin", "stdout"}},
		{name: "explicit stdout redirected", args: []string{"tui"}, stdinTTY: true, wantStderr: "stdout is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin", "stdout"}},
		{name: "bare both redirected", wantStderr: "stdin is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin"}},
		{name: "explicit both redirected", args: []string{"tui"}, wantStderr: "stdin is not a terminal\n", wantExit: 1, wantChecks: []string{"stdin"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := bytes.NewBuffer(nil)
			var stdout, stderr bytes.Buffer
			var checks []string
			bootstrapCalls := 0
			runnerCalls := 0
			root := NewRootCommand(Options{
				Version: "test",
				Out:     &stdout,
				Err:     &stderr,
				WorkDir: t.TempDir(),
				IsTerminal: func(stream any) bool {
					switch stream {
					case input:
						checks = append(checks, "stdin")
						return tt.stdinTTY
					case &stdout:
						checks = append(checks, "stdout")
						return tt.stdoutTTY
					default:
						t.Fatalf("checked unexpected stream %T", stream)
						return false
					}
				},
				TUIRunner: func(_ context.Context, opts tuiapp.RunOptions) error {
					runnerCalls++
					if opts.Input != input || opts.Output != &stdout {
						t.Fatalf("runner streams = (%T, %T), want effective command streams", opts.Input, opts.Output)
					}
					if opts.BootstrapStatus == nil {
						t.Fatal("bootstrap callback is nil")
					}
					bootstrapCalls++
					if _, err := opts.BootstrapStatus(); err != nil {
						return err
					}
					_, err := fmt.Fprintln(opts.Output, "managed tui")
					return err
				},
			})
			root.SetIn(input)
			root.SetArgs(tt.args)

			err := root.Execute()
			exit := 0
			if err != nil {
				exit = 1
				_, _ = fmt.Fprintln(&stderr, err)
			}

			if got := stdout.String(); got != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", got, tt.wantStdout)
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Fatalf("stderr = %q, want %q", got, tt.wantStderr)
			}
			if exit != tt.wantExit {
				t.Fatalf("exit = %d, want %d", exit, tt.wantExit)
			}
			if !reflect.DeepEqual(checks, tt.wantChecks) {
				t.Fatalf("TTY checks = %#v, want %#v", checks, tt.wantChecks)
			}
			if bootstrapCalls != tt.wantBootstrap {
				t.Fatalf("bootstrap calls = %d, want %d", bootstrapCalls, tt.wantBootstrap)
			}
			if want := tt.wantBootstrap; runnerCalls != want {
				t.Fatalf("runner calls = %d, want %d", runnerCalls, want)
			}
		})
	}
}

func TestTUIHelpMatchesAcceptedContractAndBypassesLaunch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "root short", args: []string{"-h"}, want: acceptedRootHelp},
		{name: "root long", args: []string{"--help"}, want: acceptedRootHelp},
		{name: "root command", args: []string{"help"}, want: acceptedRootHelp},
		{name: "tui short", args: []string{"tui", "-h"}, want: acceptedTUIHelp},
		{name: "tui long", args: []string{"tui", "--help"}, want: acceptedTUIHelp},
		{name: "tui command", args: []string{"help", "tui"}, want: acceptedTUIHelp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			gateCalls := 0
			runnerCalls := 0
			root := NewRootCommand(Options{
				Version: "test",
				Out:     &out,
				IsTerminal: func(any) bool {
					gateCalls++
					return false
				},
				TUIRunner: func(context.Context, tuiapp.RunOptions) error {
					runnerCalls++
					return nil
				},
			})
			root.SetArgs(tt.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute help: %v", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("help output:\n%s\nwant:\n%s", got, tt.want)
			}
			if gateCalls != 0 || runnerCalls != 0 {
				t.Fatalf("help entered launch: gate=%d runner=%d", gateCalls, runnerCalls)
			}
		})
	}
}

func TestTUIParseAndVersionRoutesBypassLaunch(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStdout string
		wantErr    bool
	}{
		{name: "version", args: []string{"--version"}, wantStdout: "revolvr test-version\n"},
		{name: "version is not a subcommand", args: []string{"version"}, wantErr: true},
		{name: "unknown command", args: []string{"not-a-command"}, wantErr: true},
		{name: "root malformed flag", args: []string{"--not-a-flag"}, wantErr: true},
		{name: "explicit positional", args: []string{"tui", "extra"}, wantErr: true},
		{name: "explicit malformed flag", args: []string{"tui", "--not-a-flag"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			gateCalls := 0
			runnerCalls := 0
			root := NewRootCommand(Options{
				Version: "test-version",
				Out:     &stdout,
				IsTerminal: func(any) bool {
					gateCalls++
					return true
				},
				TUIRunner: func(context.Context, tuiapp.RunOptions) error {
					runnerCalls++
					return nil
				},
			})
			root.SetArgs(tt.args)
			err := root.Execute()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %t", err, tt.wantErr)
			}
			if got := stdout.String(); got != tt.wantStdout {
				t.Fatalf("stdout = %q, want %q", got, tt.wantStdout)
			}
			if gateCalls != 0 || runnerCalls != 0 {
				t.Fatalf("parse/version route entered launch: gate=%d runner=%d", gateCalls, runnerCalls)
			}
		})
	}
}

func TestExplicitNonTUICommandBypassesTUILaunch(t *testing.T) {
	var out bytes.Buffer
	gateCalls := 0
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: t.TempDir(),
		IsTerminal: func(any) bool {
			gateCalls++
			return false
		},
		TUIRunner: func(context.Context, tuiapp.RunOptions) error {
			t.Fatal("status entered TUI runner")
			return nil
		},
	})
	root.SetArgs([]string{"status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute status: %v", err)
	}
	if got, want := out.String(), "Not initialized. Run `revolvr init` first.\n"; got != want {
		t.Fatalf("status output = %q, want %q", got, want)
	}
	if gateCalls != 0 {
		t.Fatalf("status performed %d TTY checks", gateCalls)
	}
}

func alwaysTerminal(any) bool {
	return true
}
