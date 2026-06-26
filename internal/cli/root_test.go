package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"revolvr/internal/commit"
	"revolvr/internal/ledger"
	"revolvr/internal/runonce"
	"revolvr/internal/taskqueue"
)

func TestNewRootCommandConstructsExpectedCommands(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})

	for _, args := range [][]string{
		{"init"},
		{"task"},
		{"run"},
		{"status"},
		{"show"},
	} {
		cmd, remaining, err := root.Find(args)
		if err != nil {
			t.Fatalf("find %q: %v", strings.Join(args, " "), err)
		}
		if len(remaining) != 0 {
			t.Fatalf("find %q left remaining args %v", strings.Join(args, " "), remaining)
		}
		if cmd == root {
			t.Fatalf("find %q returned root command", strings.Join(args, " "))
		}
	}
}

func TestRootHelpWorks(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}

	help := out.String()
	for _, want := range []string{"Run bounded Codex harness passes", "init", "task", "run", "status", "show"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestVersionOutputWorks(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test-version", Out: &out})
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}

	if got, want := out.String(), "revolvr test-version\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestPlaceholderCommandOutput(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"init"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute init: %v", err)
	}

	if got, want := out.String(), "revolvr init is not implemented yet.\n"; got != want {
		t.Fatalf("placeholder output = %q, want %q", got, want)
	}
}

func TestRunOnceInvokesRunnerAndPrintsSummary(t *testing.T) {
	var out bytes.Buffer
	called := false
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		WorkDir: "/repo",
		RunOnce: func(_ context.Context, cfg runonce.Config) (runonce.Result, error) {
			called = true
			if cfg.WorkingDir != "/repo" {
				t.Fatalf("working dir = %q, want /repo", cfg.WorkingDir)
			}
			return runonce.Result{
				Outcome: runonce.OutcomeCommitted,
				Run:     ledger.Run{ID: "run-1"},
				Task:    taskqueue.Task{ID: "task-1"},
				Commit:  commit.Result{CommitSHA: "abc123"},
			}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}
	if !called {
		t.Fatal("run once runner was not called")
	}

	if got, want := out.String(), "Run run-1 completed task task-1; commit abc123.\n"; got != want {
		t.Fatalf("run once output = %q, want %q", got, want)
	}
}

func TestRunOncePrintsNoTaskSummary(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{
		Version: "test",
		Out:     &out,
		RunOnce: func(context.Context, runonce.Config) (runonce.Result, error) {
			return runonce.Result{Outcome: runonce.OutcomeNoTask, NoTask: true}, nil
		},
	})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}

	if got, want := out.String(), "No pending runnable tasks.\n"; got != want {
		t.Fatalf("run once output = %q, want %q", got, want)
	}
}

func TestShowRequiresRunID(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})
	root.SetArgs([]string{"show"})

	if err := root.Execute(); err == nil {
		t.Fatal("execute show without run id succeeded, want error")
	}
}
