package cli

import (
	"bytes"
	"strings"
	"testing"
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

func TestRunOnceFlagIsAcceptedByPlaceholder(t *testing.T) {
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out})
	root.SetArgs([]string{"run", "--once"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --once: %v", err)
	}

	if got, want := out.String(), "revolvr run is not implemented yet.\n"; got != want {
		t.Fatalf("placeholder output = %q, want %q", got, want)
	}
}

func TestShowRequiresRunID(t *testing.T) {
	root := NewRootCommand(Options{Version: "test"})
	root.SetArgs([]string{"show"})

	if err := root.Execute(); err == nil {
		t.Fatal("execute show without run id succeeded, want error")
	}
}
