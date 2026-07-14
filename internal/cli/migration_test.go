package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"revolvr/internal/app"
	"revolvr/internal/autonomousexec"
	"revolvr/internal/autonomousmigration"
	revolvrlock "revolvr/internal/lock"
	"revolvr/internal/taskfile"
)

func TestTaskMigrateDryRunRendersStableGoldenAndForwardsSelection(t *testing.T) {
	var received app.MigrationPlanInput
	runner := func(_ context.Context, _ app.Config, input app.MigrationPlanInput) (autonomousmigration.Plan, error) {
		received = input
		return autonomousmigration.Plan{
			SchemaVersion: autonomousmigration.PlanSchemaVersion, TargetWorkflow: taskfile.WorkflowAutonomousV1, DryRun: input.DryRun,
			Entries: []autonomousmigration.Entry{
				{TaskID: "alpha", SourcePath: ".agent/tasks/a.md", SourceSHA256: strings.Repeat("1", 64), ProjectedSHA256: strings.Repeat("2", 64), AutonomousStatePath: ".revolvr/autonomous/tasks/alpha/state.json", StateSHA256: strings.Repeat("3", 64)},
				{TaskID: "beta", SourcePath: ".agent/tasks/b.md", SourceSHA256: strings.Repeat("4", 64), ProjectedSHA256: strings.Repeat("5", 64), AutonomousStatePath: ".revolvr/autonomous/tasks/beta/state.json", StateSHA256: strings.Repeat("6", 64)},
			},
		}, nil
	}
	var out bytes.Buffer
	root := NewRootCommand(Options{Version: "test", Out: &out, PlanTaskMigration: runner})
	root.SetArgs([]string{"task", "migrate", "--to", "autonomous-v1", "--dry-run", "beta", "alpha"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	wantInput := app.MigrationPlanInput{TargetWorkflow: "autonomous-v1", TaskIDs: []string{"beta", "alpha"}, DryRun: true}
	if !reflect.DeepEqual(received, wantInput) {
		t.Fatalf("migration input = %+v, want %+v", received, wantInput)
	}
	want := "Autonomous migration dry-run: 2 task(s); no files written.\n" +
		"Schema: autonomous-migration-plan-v1\n" +
		"Target: autonomous-v1\n" +
		"TASK ID\tSOURCE\tSOURCE SHA-256\tPROJECTED SHA-256\tSTATE PATH\tSTATE SHA-256\n" +
		"alpha\t.agent/tasks/a.md\t" + strings.Repeat("1", 64) + "\t" + strings.Repeat("2", 64) + "\t.revolvr/autonomous/tasks/alpha/state.json\t" + strings.Repeat("3", 64) + "\n" +
		"beta\t.agent/tasks/b.md\t" + strings.Repeat("4", 64) + "\t" + strings.Repeat("5", 64) + "\t.revolvr/autonomous/tasks/beta/state.json\t" + strings.Repeat("6", 64) + "\n"
	if out.String() != want {
		t.Fatalf("migration output:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestTaskMigrateDryRunIsDeterministicAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeCLIMigrationTask(t, root, "b.md", "beta", "pending", "implement")
	writeCLIMigrationTask(t, root, "a.md", "alpha", "pending", "implement")
	alphaPath := filepath.Join(root, ".agent", "tasks", "a.md")
	betaPath := filepath.Join(root, ".agent", "tasks", "b.md")
	alphaBefore, err := os.ReadFile(alphaPath)
	if err != nil {
		t.Fatal(err)
	}
	betaBefore, err := os.ReadFile(betaPath)
	if err != nil {
		t.Fatal(err)
	}

	first := executeMigrationCommand(t, root, []string{"task", "migrate", "--to", "autonomous-v1", "--dry-run", "beta", "alpha"})
	second := executeMigrationCommand(t, root, []string{"task", "migrate", "--to", "autonomous-v1", "--dry-run", "alpha", "beta"})
	all := executeMigrationCommand(t, root, []string{"task", "migrate", "--to", "autonomous-v1", "--all", "--dry-run"})
	if first != second || first != all {
		t.Fatalf("dry-run output varies by selection order:\nfirst:\n%s\nsecond:\n%s\nall:\n%s", first, second, all)
	}
	if strings.Index(first, "alpha\t.agent/tasks/a.md") > strings.Index(first, "beta\t.agent/tasks/b.md") {
		t.Fatalf("dry-run order is not canonical:\n%s", first)
	}
	alphaAfter, _ := os.ReadFile(alphaPath)
	betaAfter, _ := os.ReadFile(betaPath)
	if !bytes.Equal(alphaBefore, alphaAfter) || !bytes.Equal(betaBefore, betaAfter) {
		t.Fatal("dry-run changed canonical task bytes")
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created runtime state: %v", err)
	}
}

func TestTaskMigrateRejectsInvalidBatchAndArgumentsWithoutWrites(t *testing.T) {
	root := t.TempDir()
	writeCLIMigrationTask(t, root, "valid.md", "valid", "pending", "implement")
	writeCLIMigrationTask(t, root, "blocked.md", "blocked", "blocked", "implement")
	blockedPath := filepath.Join(root, ".agent", "tasks", "blocked.md")
	before, err := os.ReadFile(blockedPath)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "all or nothing", args: []string{"task", "migrate", "--to", "autonomous-v1", "--all", "--dry-run"}, want: "status_not_pending"},
		{name: "unknown target", args: []string{"task", "migrate", "--to", "other", "valid"}, want: "--to must be autonomous-v1"},
		{name: "missing selection", args: []string{"task", "migrate", "--to", "autonomous-v1"}, want: "provide at least one task ID or --all"},
		{name: "ambiguous selection flags", args: []string{"task", "migrate", "--to", "autonomous-v1", "--all", "valid"}, want: "--all cannot be combined"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := NewRootCommand(Options{Version: "test", WorkDir: root, Out: &out})
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Execute error = %v, want %q", err, test.want)
			}
			if out.Len() != 0 {
				t.Fatalf("failed migration rendered partial plan: %q", out.String())
			}
		})
	}
	after, _ := os.ReadFile(blockedPath)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected migration changed task bytes")
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("rejected migration created runtime state: %v", err)
	}
}

func TestTaskMigrateAppliesAndReplaysWithoutInitConversion(t *testing.T) {
	root := t.TempDir()
	writeCLIMigrationTask(t, root, "candidate.md", "candidate", "pending", "implement")
	taskPath := filepath.Join(root, ".agent", "tasks", "candidate.md")
	before, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executeCLI(t, root, "init"); err != nil {
		t.Fatal(err)
	}
	afterInit, err := os.ReadFile(taskPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, afterInit) {
		t.Fatal("revolvr init converted canonical task bytes")
	}
	statePath := filepath.Join(root, ".revolvr", "autonomous", "tasks", "candidate", "state.json")
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("revolvr init created autonomous state: %v", err)
	}

	first, err := executeCLI(t, root, "task", "migrate", "--to", "autonomous-v1", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "Autonomous migration applied: 1 task(s).") || !strings.Contains(first, "Stage: completed") {
		t.Fatalf("apply output:\n%s", first)
	}
	task, err := taskfile.Load(root, filepath.ToSlash(filepath.Join(taskfile.TasksDir, "candidate.md")))
	if err != nil {
		t.Fatal(err)
	}
	if task.Workflow != taskfile.WorkflowAutonomousV1 || task.Phase != "" || task.Profile != "" {
		t.Fatalf("migrated task = %+v", task)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("migrated state: %v", err)
	}

	second, err := executeCLI(t, root, "task", "migrate", "--to", "autonomous-v1", "candidate")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second, "Autonomous migration replayed: 1 task(s).") {
		t.Fatalf("replay output:\n%s", second)
	}
	if strings.Replace(first, "applied", "replayed", 1) != second {
		t.Fatalf("replay changed operation identity:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestTaskMigrateRespectsExecutionAndSourceWriterLocks(t *testing.T) {
	for _, test := range []struct {
		name    string
		acquire func(*testing.T, string) func()
		want    string
	}{
		{
			name: "autonomous execution",
			acquire: func(t *testing.T, root string) func() {
				release, err := autonomousexec.TryAcquire(root)
				if err != nil {
					t.Fatal(err)
				}
				return release
			},
			want: "another coordinator is active",
		},
		{
			name: "source writer",
			acquire: func(t *testing.T, root string) func() {
				writer, err := revolvrlock.AcquireSourceWriter(context.Background(), revolvrlock.Config{WorkingDir: root, RunID: "existing-writer", PID: 123, Timeout: time.Minute})
				if err != nil {
					t.Fatal(err)
				}
				return func() { _ = writer.Release(context.Background()) }
			},
			want: "source-writer lock is held",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if _, err := executeCLI(t, root, "init"); err != nil {
				t.Fatal(err)
			}
			writeCLIMigrationTask(t, root, "candidate.md", "candidate", "pending", "implement")
			taskPath := filepath.Join(root, ".agent", "tasks", "candidate.md")
			before, err := os.ReadFile(taskPath)
			if err != nil {
				t.Fatal(err)
			}
			release := test.acquire(t, root)
			_, err = executeCLI(t, root, "task", "migrate", "--to", "autonomous-v1", "candidate")
			release()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("migration error = %v, want %q", err, test.want)
			}
			after, readErr := os.ReadFile(taskPath)
			if readErr != nil || !bytes.Equal(before, after) {
				t.Fatalf("contended migration changed task: err=%v", readErr)
			}
		})
	}
}

func TestTaskMigrateAllDoesNotReplayAnOlderBatchWhenNewMixedTasksExist(t *testing.T) {
	root := t.TempDir()
	if _, err := executeCLI(t, root, "init"); err != nil {
		t.Fatal(err)
	}
	writeCLIMigrationTask(t, root, "alpha.md", "alpha", "pending", "implement")
	if _, err := executeCLI(t, root, "task", "migrate", "--to", "autonomous-v1", "--all"); err != nil {
		t.Fatal(err)
	}
	writeCLIMigrationTask(t, root, "beta.md", "beta", "pending", "implement")
	out, err := executeCLI(t, root, "task", "migrate", "--to", "autonomous-v1", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Autonomous migration applied: 1 task(s).") {
		t.Fatalf("second all migration output:\n%s", out)
	}
	beta, err := taskfile.Load(root, filepath.ToSlash(filepath.Join(taskfile.TasksDir, "beta.md")))
	if err != nil || beta.Workflow != taskfile.WorkflowAutonomousV1 {
		t.Fatalf("beta = %+v, err=%v", beta, err)
	}
}

func executeMigrationCommand(t *testing.T, workDir string, args []string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := NewRootCommand(Options{Version: "test", WorkDir: workDir, Out: &out})
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %q: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

func writeCLIMigrationTask(t *testing.T, root, name, id, status, phase string) {
	t.Helper()
	path := filepath.Join(root, ".agent", "tasks", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := "---\nid: " + id + "\nstatus: " + status + "\nworkflow: mixed-pass-v1\nphase: " + phase + "\nx-owner: exact\n---\n# " + id + "\n\nExact body.\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}
