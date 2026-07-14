package taskfile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSchedulingMetadataStrictRoundTripAndStatusPreservation(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("---\r\nid: child\r\nstatus: pending\r\nworkflow: autonomous-v1\r\nautonomous_state_path: .revolvr/autonomous/tasks/child/state.json\r\npriority: 2\r\ndepends_on: base,parent\r\ntags: api,small\r\nconflicts: task-x,shared-db\r\nparent_task_id: parent\r\nchild_proposal_id: proposal-one\r\nchild_decision_id: decision-one\r\nchild_run_id: run-one\r\nchild_evidence: task:parent,plan:plan-one\r\nparent_behavior: depends_on_parent\r\nx-unknown: keep exact\r\n---\r\n# Child\r\n\r\nExact body without final newline")
	path := filepath.Join(repo, TasksDir, "child.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	task, err := Load(repo, filepath.Join(TasksDir, "child.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(task.DependsOn, []string{"base", "parent"}) || !reflect.DeepEqual(task.Tags, []string{"api", "small"}) || !reflect.DeepEqual(task.Conflicts, []string{"task-x", "shared-db"}) {
		t.Fatalf("metadata = %#v", task)
	}
	updated, err := UpdateStatus(repo, task.SourcePath, StatusBlocked)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(raw, []byte("status: pending"), []byte("status: blocked"), 1)
	if !bytes.Equal(updated.SourceBytes, want) {
		t.Fatalf("updated bytes changed unrelated scheduling metadata\n%s", updated.SourceBytes)
	}
}

func TestSchedulingMetadataRejectsInvalidListsAndLineage(t *testing.T) {
	base := "---\nid: child\nstatus: pending\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/child/state.json\n%s\n---\n# Child\n"
	for _, tt := range []struct{ name, metadata, want string }{
		{"self", "depends_on: child", "self dependency"},
		{"duplicate", "depends_on: base,base", "duplicate depends_on"},
		{"whitespace", "tags: one, two", "invalid tags item"},
		{"partial lineage", "parent_task_id: parent", "child lineage requires"},
		{"dependent missing edge", "parent_task_id: parent\nchild_proposal_id: proposal\nchild_decision_id: decision\nchild_run_id: run\nchild_evidence: task:parent\nparent_behavior: depends_on_parent", "must name parent_task_id"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			path := writeTaskFile(t, repo, "child.md", strings.Replace(base, "%s", tt.metadata, 1))
			_, err := Load(repo, path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadAllPreservesDuplicateIDsForSharedGraphValidation(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "a.md", "---\nid: duplicate\nstatus: pending\n---\n# A\n")
	writeTaskFile(t, repo, "b.md", "---\nid: duplicate\nstatus: pending\n---\n# B\n")
	tasks, err := LoadAll(repo)
	if err != nil || len(tasks) != 2 || tasks[0].ID != "duplicate" || tasks[1].ID != "duplicate" {
		t.Fatalf("load all tasks=%#v error=%v", tasks, err)
	}
	if _, err := List(repo); err == nil || !strings.Contains(err.Error(), "duplicated") {
		t.Fatalf("ordinary list error = %v, want compatibility duplicate rejection", err)
	}
}

func TestProjectAndPublishAutonomousChildDeterministically(t *testing.T) {
	repo := t.TempDir()
	input := AutonomousCreateInput{ID: "child-one", Title: "Child One", Body: "Do bounded work.", Priority: 3, HasPriority: true, DependsOn: []string{"parent"}, Tags: []string{"small"}, Conflicts: []string{"shared"}, ParentTaskID: "parent", ChildProposalID: "proposal-one", ChildDecisionID: "decision-one", ChildRunID: "run-one", ChildEvidence: []string{"task:parent"}, ParentBehavior: ParentBehaviorDependent}
	first, err := ProjectAutonomousTask(repo, input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ProjectAutonomousTask(repo, input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.SourceBytes, second.SourceBytes) {
		t.Fatal("projection is not deterministic")
	}
	published, err := PublishAutonomousTask(repo, first)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := PublishAutonomousTask(repo, second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(published.SourceBytes, replayed.SourceBytes) {
		t.Fatal("exact replay changed bytes")
	}
}
