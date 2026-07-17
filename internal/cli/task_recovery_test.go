package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"revolvr/internal/app"
	"revolvr/internal/autonomoustaskrun"
)

func TestTaskRecoveryCommand(t *testing.T) {
	checks := make([]app.RecoveryAuthorityCheck, 0, 7)
	for _, name := range []string{"task", "state", "workspace", "git", "ledger", "receipt", "artifacts"} {
		checks = append(checks, app.RecoveryAuthorityCheck{Name: name, Passed: true, Detail: name + " authority agrees"})
	}
	baseResult := app.RecoverAutonomousTaskResult{
		SchemaVersion: app.AutonomousRecoverySchemaVersion, TaskID: "task-one", OperationID: "operation-one",
		StopReason: autonomoustaskrun.StopUnsafeAmbiguous, Checks: checks, AuthoritySHA256: strings.Repeat("a", 64),
		Ready: true, ReconcileEligible: true,
	}

	t.Run("read only report", func(t *testing.T) {
		var called app.RecoverAutonomousTaskInput
		var out bytes.Buffer
		root := NewRootCommand(Options{Out: &out, RecoverTask: func(_ context.Context, _ app.Config, input app.RecoverAutonomousTaskInput) (app.RecoverAutonomousTaskResult, error) {
			called = input
			return baseResult, nil
		}})
		root.SetArgs([]string{"task", "recover", "task-one", "--operation-id", "operation-one"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if called.TaskID != "task-one" || called.OperationID != "operation-one" || called.Reconcile || called.ConfirmOperation != "" {
			t.Fatalf("recovery input = %+v", called)
		}
		text := out.String()
		for _, want := range []string{"Autonomous task recovery (read-only)", "Operation: operation-one", "Ready: true", "Reconcile eligible: true", "task\tPASS", "state\tPASS", "workspace\tPASS", "git\tPASS", "ledger\tPASS", "receipt\tPASS", "artifacts\tPASS"} {
			if !strings.Contains(text, want) {
				t.Fatalf("output missing %q:\n%s", want, text)
			}
		}
	})

	t.Run("exact reconciliation", func(t *testing.T) {
		var called app.RecoverAutonomousTaskInput
		var out bytes.Buffer
		root := NewRootCommand(Options{Out: &out, RecoverTask: func(_ context.Context, _ app.Config, input app.RecoverAutonomousTaskInput) (app.RecoverAutonomousTaskResult, error) {
			called = input
			result := baseResult
			result.Reconciled = true
			result.NewOperationID = "task-recovery-new"
			return result, nil
		}})
		root.SetArgs([]string{"task", "recover", "task-one", "--operation-id", "operation-one", "--reconcile", "--confirm-operation", "operation-one"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if !called.Reconcile || called.ConfirmOperation != called.OperationID {
			t.Fatalf("reconciliation input = %+v", called)
		}
		if text := out.String(); !strings.Contains(text, "New operation: task-recovery-new (created)") || !strings.Contains(text, "Old operation: operation-one (unchanged)") {
			t.Fatalf("reconciliation output:\n%s", text)
		}
	})

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing confirmation", args: []string{"task", "recover", "task-one", "--operation-id", "operation-one", "--reconcile"}, want: "exactly match"},
		{name: "wrong confirmation", args: []string{"task", "recover", "task-one", "--operation-id", "operation-one", "--reconcile", "--confirm-operation", "other"}, want: "exactly match"},
		{name: "confirmation without reconciliation", args: []string{"task", "recover", "task-one", "--operation-id", "operation-one", "--confirm-operation", "operation-one"}, want: "requires --reconcile"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			root := NewRootCommand(Options{RecoverTask: func(context.Context, app.Config, app.RecoverAutonomousTaskInput) (app.RecoverAutonomousTaskResult, error) {
				calls++
				return baseResult, nil
			}})
			root.SetArgs(test.args)
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if calls != 0 {
				t.Fatalf("invalid reconciliation invoked app %d time(s)", calls)
			}
		})
	}

	t.Run("generic retry cannot invoke recovery", func(t *testing.T) {
		calls := 0
		for _, args := range [][]string{{"task", "retry", "missing"}, {"task", "unblock", "missing"}} {
			root := NewRootCommand(Options{WorkDir: t.TempDir(), RecoverTask: func(context.Context, app.Config, app.RecoverAutonomousTaskInput) (app.RecoverAutonomousTaskResult, error) {
				calls++
				return baseResult, nil
			}})
			root.SetArgs(args)
			_ = root.Execute()
		}
		if calls != 0 {
			t.Fatalf("generic retry invoked recovery %d time(s)", calls)
		}
	})
}
