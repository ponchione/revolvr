package autonomousmigration

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/autonomous"
	"revolvr/internal/taskfile"
	"revolvr/internal/taskschedule"
	"revolvr/internal/taskscheduler"
)

func TestBuildProducesDeterministicByteExactMigrationPlan(t *testing.T) {
	root := t.TempDir()
	alphaRaw := []byte("---\r\nid: alpha\r\nstatus: pending\r\nworkflow: mixed-pass-v1\r\nphase: implement\r\nprofile: implementer\r\npriority: 2\r\ndepends_on: base\r\ntags: api\r\nconflicts: shared\r\nx-owner: retain me\r\n---\r\n# Alpha\r\n\r\nExact alpha body without final newline")
	writeMigrationTask(t, root, "alpha.md", alphaRaw)
	writeMigrationTask(t, root, "base.md", []byte("---\nid: base\nstatus: pending\nphase: implement\nx-note: exact\n---\n# Base\n\nBase body.\n"))
	snapshot := loadMigrationSchedule(t, root)

	first, err := Build(root, snapshot, Request{TaskIDs: []string{"base", "alpha"}, DryRun: true})
	if err != nil {
		t.Fatalf("build first plan: %v", err)
	}
	second, err := Build(root, snapshot, Request{TaskIDs: []string{"alpha", "base"}, DryRun: true})
	if err != nil {
		t.Fatalf("build second plan: %v", err)
	}
	all, err := Build(root, snapshot, Request{All: true, DryRun: true})
	if err != nil {
		t.Fatalf("build all plan: %v", err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(first, all) {
		t.Fatalf("plans vary by selection order:\nfirst=%+v\nsecond=%+v\nall=%+v", first, second, all)
	}
	if first.SchemaVersion != PlanSchemaVersion || first.TargetWorkflow != taskfile.WorkflowAutonomousV1 || !first.DryRun || len(first.Entries) != 2 {
		t.Fatalf("plan header = %+v", first)
	}
	if first.Entries[0].TaskID != "alpha" || first.Entries[1].TaskID != "base" {
		t.Fatalf("entry order = %q, %q", first.Entries[0].TaskID, first.Entries[1].TaskID)
	}

	alpha := first.Entries[0]
	wantTask := []byte("---\r\nid: alpha\r\nstatus: pending\r\nworkflow: autonomous-v1\r\npriority: 2\r\ndepends_on: base\r\ntags: api\r\nconflicts: shared\r\nx-owner: retain me\r\nautonomous_state_path: .revolvr/autonomous/tasks/alpha/state.json\r\n---\r\n# Alpha\r\n\r\nExact alpha body without final newline")
	if !bytes.Equal(alpha.ProjectedTask.SourceBytes, wantTask) {
		t.Fatalf("projected alpha task = %q, want %q", alpha.ProjectedTask.SourceBytes, wantTask)
	}
	wantState := "{\n  \"schema_version\": \"autonomous-execution-state-v1\",\n  \"task_id\": \"alpha\",\n  \"lifecycle\": \"pending\",\n  \"attempts\": {\n    \"total_attempts\": 0,\n    \"consecutive_failures\": 0,\n    \"retry_budget\": {\n      \"mode\": \"unset\",\n      \"limit\": 0,\n      \"consumed\": 0\n    },\n    \"elapsed_time_budget\": {\n      \"mode\": \"unset\",\n      \"limit_nanoseconds\": 0,\n      \"consumed_nanoseconds\": 0\n    },\n    \"token_budget\": {\n      \"mode\": \"unset\",\n      \"limit\": 0,\n      \"consumed\": 0\n    }\n  }\n}\n"
	if string(alpha.StateBytes) != wantState {
		t.Fatalf("initial state:\n%s\nwant:\n%s", alpha.StateBytes, wantState)
	}
	if alpha.AutonomousState.Plan != nil || len(alpha.AutonomousState.AcceptanceCriteria) != 0 || alpha.AutonomousState.LatestDecision != nil || alpha.AutonomousState.ReopenedFrom != nil || alpha.AutonomousState.ChildOf != nil {
		t.Fatalf("migration fabricated lifecycle evidence: %+v", alpha.AutonomousState)
	}
	if alpha.AutonomousState.Attempts.RetryBudget.Mode != autonomous.BudgetModeUnset || alpha.AutonomousState.Attempts.ElapsedTimeBudget.Mode != autonomous.BudgetModeUnset || alpha.AutonomousState.Attempts.TokenBudget.Mode != autonomous.BudgetModeUnset {
		t.Fatalf("migration state budgets = %+v", alpha.AutonomousState.Attempts)
	}
	current, err := os.ReadFile(filepath.Join(root, ".agent", "tasks", "alpha.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, alphaRaw) {
		t.Fatal("planning changed canonical task bytes")
	}
	if _, err := os.Stat(filepath.Join(root, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("planning created runtime state: %v", err)
	}
}

func TestBuildRejectsEveryEligibilityClass(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		prepare func(*testing.T, string)
		want    string
	}{
		{name: "audit phase", raw: mixedMigrationTask("candidate", "pending", "audit", ""), want: "phase_not_implement"},
		{name: "document phase", raw: mixedMigrationTask("candidate", "pending", "document", ""), want: "phase_not_implement"},
		{name: "simplify phase", raw: mixedMigrationTask("candidate", "pending", "simplify", ""), want: "phase_not_implement"},
		{name: "running", raw: mixedMigrationTask("candidate", "running", "implement", ""), want: "status_not_pending"},
		{name: "blocked", raw: mixedMigrationTask("candidate", "blocked", "implement", ""), want: "status_not_pending"},
		{name: "terminal", raw: mixedMigrationTask("candidate", "completed", "implement", ""), want: "status_not_pending"},
		{name: "autonomous workflow", raw: "---\nid: candidate\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/candidate/state.json\n---\n# Candidate\n", want: "workflow_not_mixed_pass"},
		{name: "child lineage", raw: mixedMigrationTask("candidate", "pending", "implement", "parent_task_id: parent\nchild_proposal_id: proposal\nchild_decision_id: decision\nchild_run_id: run\nchild_evidence: task:parent\nparent_behavior: independent\n"), want: "child_lineage_present"},
		{name: "existing state", raw: mixedMigrationTask("candidate", "pending", "implement", ""), prepare: func(t *testing.T, root string) {
			writeMigrationFile(t, filepath.Join(root, ".revolvr", "autonomous", "tasks", "candidate", "state.json"), []byte("user evidence\n"))
		}, want: "autonomous_state_exists"},
		{name: "existing namespace", raw: mixedMigrationTask("candidate", "pending", "implement", ""), prepare: func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, ".revolvr", "autonomous", "tasks", "candidate"), 0o755); err != nil {
				t.Fatal(err)
			}
		}, want: "autonomous_namespace_exists"},
		{name: "symlinked state ancestor", raw: mixedMigrationTask("candidate", "pending", "implement", ""), prepare: func(t *testing.T, root string) {
			target := filepath.Join(root, "runtime-target")
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(root, ".revolvr")); err != nil {
				t.Fatal(err)
			}
		}, want: "autonomous_state_path_unsafe"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeMigrationTask(t, root, "candidate.md", []byte(test.raw))
			if test.prepare != nil {
				test.prepare(t, root)
			}
			snapshot := loadMigrationSchedule(t, root)
			_, err := Build(root, snapshot, Request{TaskIDs: []string{"candidate"}, DryRun: true})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildIsAllOrNothingForSelectionAndGraphFailures(t *testing.T) {
	t.Run("one ineligible task rejects the batch without writes", func(t *testing.T) {
		root := t.TempDir()
		validPath := writeMigrationTask(t, root, "valid.md", []byte(mixedMigrationTask("valid", "pending", "implement", "")))
		blockedPath := writeMigrationTask(t, root, "blocked.md", []byte(mixedMigrationTask("blocked", "blocked", "implement", "")))
		beforeValid := mustReadMigrationFile(t, validPath)
		beforeBlocked := mustReadMigrationFile(t, blockedPath)
		_, err := Build(root, loadMigrationSchedule(t, root), Request{All: true, DryRun: true})
		if err == nil || !strings.Contains(err.Error(), "status_not_pending") {
			t.Fatalf("Build error = %v", err)
		}
		if !bytes.Equal(mustReadMigrationFile(t, validPath), beforeValid) || !bytes.Equal(mustReadMigrationFile(t, blockedPath), beforeBlocked) {
			t.Fatal("failed batch changed task bytes")
		}
		if _, err := os.Stat(filepath.Join(root, ".revolvr")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed batch created runtime state: %v", err)
		}
	})

	t.Run("invalid shared graph rejects before projection", func(t *testing.T) {
		root := t.TempDir()
		writeMigrationTask(t, root, "invalid.md", []byte(mixedMigrationTask("invalid", "pending", "implement", "depends_on: missing\n")))
		snapshot := loadMigrationSchedule(t, root)
		if snapshot.Result.Valid() {
			t.Fatal("fixture graph unexpectedly valid")
		}
		_, err := Build(root, snapshot, Request{TaskIDs: []string{"invalid"}, DryRun: true})
		if err == nil || !strings.Contains(err.Error(), "invalid_graph") || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("Build error = %v", err)
		}
	})

	t.Run("missing and duplicate explicit IDs are deterministic", func(t *testing.T) {
		root := t.TempDir()
		writeMigrationTask(t, root, "valid.md", []byte(mixedMigrationTask("valid", "pending", "implement", "")))
		snapshot := loadMigrationSchedule(t, root)
		_, first := Build(root, snapshot, Request{TaskIDs: []string{"missing", "valid", "valid"}, DryRun: true})
		_, second := Build(root, snapshot, Request{TaskIDs: []string{"valid", "missing", "valid"}, DryRun: true})
		if first == nil || second == nil || first.Error() != second.Error() || !strings.Contains(first.Error(), "duplicate_request") || !strings.Contains(first.Error(), "task_not_found") {
			t.Fatalf("selection errors:\nfirst=%v\nsecond=%v", first, second)
		}
	})
}

func loadMigrationSchedule(t *testing.T, root string) taskschedule.Snapshot {
	t.Helper()
	snapshot, err := taskschedule.Load(context.Background(), taskschedule.Config{
		RepositoryRoot: root, SelectionWorkflow: taskscheduler.WorkflowMixedPassV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func mixedMigrationTask(id, status, phase, extra string) string {
	return "---\nid: " + id + "\nstatus: " + status + "\nworkflow: mixed-pass-v1\nphase: " + phase + "\n" + extra + "---\n# " + id + "\n\nExact body.\n"
}

func writeMigrationTask(t *testing.T, root, name string, raw []byte) string {
	t.Helper()
	path := filepath.Join(root, ".agent", "tasks", name)
	writeMigrationFile(t, path, raw)
	return path
}

func writeMigrationFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustReadMigrationFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
