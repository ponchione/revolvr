package taskfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestProjectAutonomousMigrationPreservesTaskBytesAndRemovesMixedRouting(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("---\r\nid: exact-task\r\nstatus: pending\r\nworkflow: mixed-pass-v1\r\nphase: implement\r\nprofile: custom-worker\r\npriority: 7\r\ndepends_on: base\r\ntags: api,small\r\nconflicts: shared-db\r\nx-owner: keep exact spacing\r\n---\r\n# Exact Task\r\n\r\nPreserve this body byte-for-byte.\r\n\r\n```text\r\nphase: audit\r\n```\r\nwithout-final-newline")
	path := writeTaskFile(t, repo, "exact-task.md", string(raw))
	snapshot, err := Load(repo, path)
	if err != nil {
		t.Fatal(err)
	}

	projected, err := ProjectAutonomousMigration(repo, snapshot)
	if err != nil {
		t.Fatalf("project migration: %v", err)
	}
	want := []byte("---\r\nid: exact-task\r\nstatus: pending\r\nworkflow: autonomous-v1\r\npriority: 7\r\ndepends_on: base\r\ntags: api,small\r\nconflicts: shared-db\r\nx-owner: keep exact spacing\r\nautonomous_state_path: .revolvr/autonomous/tasks/exact-task/state.json\r\n---\r\n# Exact Task\r\n\r\nPreserve this body byte-for-byte.\r\n\r\n```text\r\nphase: audit\r\n```\r\nwithout-final-newline")
	if !bytes.Equal(projected.SourceBytes, want) {
		t.Fatalf("projected bytes:\n%q\nwant:\n%q", projected.SourceBytes, want)
	}
	if projected.Workflow != WorkflowAutonomousV1 || projected.Phase != "" || projected.Profile != "" || projected.AutonomousStatePath != autonomousStatePath("exact-task") {
		t.Fatalf("projected routing = %+v", projected)
	}
	if projected.ID != snapshot.ID || projected.Title != snapshot.Title || projected.Priority != snapshot.Priority || !reflect.DeepEqual(projected.DependsOn, snapshot.DependsOn) || !reflect.DeepEqual(projected.Tags, snapshot.Tags) || !reflect.DeepEqual(projected.Conflicts, snapshot.Conflicts) {
		t.Fatalf("projected task did not preserve task identity and scheduling metadata: %+v", projected)
	}
	current, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, raw) {
		t.Fatal("projection mutated the canonical task file")
	}
}

func TestProjectAutonomousMigrationAddsFrontmatterWithoutChangingBody(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("# Implicit Task\n\nExact body without a final newline")
	path := writeTaskFile(t, repo, "implicit-task.md", string(raw))
	snapshot, err := Load(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectAutonomousMigration(repo, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []byte("---\nworkflow: autonomous-v1\nautonomous_state_path: .revolvr/autonomous/tasks/implicit-task/state.json\n---\n")
	if !bytes.Equal(projected.SourceBytes, append(wantPrefix, raw...)) {
		t.Fatalf("projected implicit task = %q", projected.SourceBytes)
	}
}

func TestProjectAutonomousMigrationRejectsIneligibleSnapshot(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "child.md", "---\nid: child\nstatus: pending\nworkflow: mixed-pass-v1\nphase: implement\nparent_task_id: parent\nchild_proposal_id: proposal\nchild_decision_id: decision\nchild_run_id: run\nchild_evidence: task:parent\nparent_behavior: independent\n---\n# Child\n")
	task, err := Load(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectAutonomousMigration(repo, task); err == nil || !strings.Contains(err.Error(), "has child lineage") {
		t.Fatalf("projection error = %v", err)
	}
}

func TestPublishAutonomousMigrationIsAtomicReplaySafeAndConflictSafe(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "candidate.md", "---\nid: candidate\nstatus: pending\nworkflow: mixed-pass-v1\nphase: implement\nx-owner: exact\n---\n# Candidate\n\nExact body.\n")
	snapshot, err := Load(repo, path)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ProjectAutonomousMigration(repo, snapshot)
	if err != nil {
		t.Fatal(err)
	}

	updated, changed, err := PublishAutonomousMigration(repo, snapshot, projected)
	if err != nil || !changed || !bytes.Equal(updated.SourceBytes, projected.SourceBytes) {
		t.Fatalf("publish = changed %v task %+v err %v", changed, updated, err)
	}
	updated, changed, err = PublishAutonomousMigration(repo, snapshot, projected)
	if err != nil || changed || !bytes.Equal(updated.SourceBytes, projected.SourceBytes) {
		t.Fatalf("replay = changed %v task %+v err %v", changed, updated, err)
	}

	repo = t.TempDir()
	path = writeTaskFile(t, repo, "candidate.md", string(snapshot.SourceBytes))
	snapshot, _ = Load(repo, path)
	projected, _ = ProjectAutonomousMigration(repo, snapshot)
	sentinel := errors.New("injected atomic failure")
	originalWriter := writeMigrationFileAtomically
	writeMigrationFileAtomically = func(string, []byte, os.FileMode) error { return sentinel }
	t.Cleanup(func() { writeMigrationFileAtomically = originalWriter })
	_, changed, err = PublishAutonomousMigration(repo, snapshot, projected)
	if !errors.Is(err, sentinel) || changed {
		t.Fatalf("failed publication = changed %v err %v", changed, err)
	}
	if got, readErr := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path))); readErr != nil || !bytes.Equal(got, snapshot.SourceBytes) {
		t.Fatalf("failed publication changed task: err=%v", readErr)
	}
	conflict := append(append([]byte(nil), snapshot.SourceBytes...), []byte("user change\n")...)
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(path)), conflict, 0o644); err != nil {
		t.Fatal(err)
	}
	_, changed, err = PublishAutonomousMigration(repo, snapshot, projected)
	if err == nil || changed || !strings.Contains(err.Error(), "changed since planning") {
		t.Fatalf("conflicting publication = changed %v err %v", changed, err)
	}
	if got, readErr := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path))); readErr != nil || !bytes.Equal(got, conflict) {
		t.Fatalf("conflicting publication overwrote task: err=%v", readErr)
	}
}

func TestLoadAutonomousTaskLifecycleMetadata(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "autonomous.md", autonomousTaskMarkdown(
		"task-autonomous",
		StatusPending,
		"4",
		"Autonomous Task",
		"Implement the autonomous task.",
	))

	task, err := Load(repo, path)
	if err != nil {
		t.Fatalf("load autonomous task: %v", err)
	}
	if got, want := task.Workflow, WorkflowAutonomousV1; got != want {
		t.Fatalf("workflow = %q, want %q", got, want)
	}
	if task.Phase != "" {
		t.Fatalf("phase = %q, want no mixed-pass phase", task.Phase)
	}
	if task.Profile != "" {
		t.Fatalf("profile = %q, want no task-level profile", task.Profile)
	}
	if got, want := task.AutonomousStatePath, autonomousStatePath("task-autonomous"); got != want {
		t.Fatalf("autonomous state path = %q, want %q", got, want)
	}
	if !task.HasPriority || task.Priority != 4 || task.Status != StatusPending {
		t.Fatalf("priority/status = %d/%v/%s, want 4/true/pending", task.Priority, task.HasPriority, task.Status)
	}
	if _, err := os.Stat(filepath.Join(repo, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("runtime state stat error = %v, want not exist", err)
	}
}

func TestLoadRejectsInvalidAutonomousLifecycleMetadata(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "duplicate state path",
			content: autonomousFrontmatter("task-duplicate", "") +
				"autonomous_state_path: " + autonomousStatePath("task-duplicate") + "\n---\n# Duplicate\n",
			want: `duplicate frontmatter key "autonomous_state_path"`,
		},
		{
			name: "missing state path",
			content: `---
id: task-missing
workflow: autonomous-v1
---
# Missing
`,
			want: `frontmatter key "autonomous_state_path" is required`,
		},
		{
			name: "empty state path",
			content: `---
id: task-empty
workflow: autonomous-v1
autonomous_state_path:
---
# Empty
`,
			want: `frontmatter key "autonomous_state_path" is required`,
		},
		{
			name: "phase",
			content: `---
id: task-phase
workflow: autonomous-v1
phase: implement
autonomous_state_path: .revolvr/autonomous/tasks/task-phase/state.json
---
# Phase
`,
			want: `frontmatter key "phase" is not allowed`,
		},
		{
			name: "profile",
			content: `---
id: task-profile
workflow: autonomous-v1
profile: implementer
autonomous_state_path: .revolvr/autonomous/tasks/task-profile/state.json
---
# Profile
`,
			want: `frontmatter key "profile" is not allowed`,
		},
		{
			name: "mixed pass state path",
			content: `---
id: task-mixed
workflow: mixed-pass-v1
autonomous_state_path: .revolvr/autonomous/tasks/task-mixed/state.json
---
# Mixed
`,
			want: `frontmatter key "autonomous_state_path" is not allowed`,
		},
		{
			name:    "absolute path",
			content: autonomousTaskMarkdownWithStatePath("task-absolute", "/tmp/state.json"),
			want:    `invalid autonomous_state_path "/tmp/state.json"`,
		},
		{
			name:    "traversal path",
			content: autonomousTaskMarkdownWithStatePath("task-traversal", ".revolvr/autonomous/tasks/task-traversal/../../state.json"),
			want:    "invalid autonomous_state_path",
		},
		{
			name:    "outside runtime state",
			content: autonomousTaskMarkdownWithStatePath("task-outside", ".agent/task-outside/state.json"),
			want:    "invalid autonomous_state_path",
		},
		{
			name:    "other task namespace",
			content: autonomousTaskMarkdownWithStatePath("task-owner", autonomousStatePath("task-other")),
			want:    `invalid autonomous_state_path`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			path := writeTaskFile(t, repo, "invalid.md", tt.content)
			_, err := Load(repo, path)
			if err == nil {
				t.Fatal("load succeeded, want lifecycle metadata error")
			}
			for _, want := range []string{path, tt.want} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want %q", err, want)
				}
			}
		})
	}
}

func TestLoadRejectsAutonomousStatePathSymlinkEscape(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".revolvr"), 0o755); err != nil {
		t.Fatalf("create runtime root: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".revolvr", "autonomous")); err != nil {
		t.Fatalf("create autonomous symlink: %v", err)
	}
	path := writeTaskFile(t, repo, "escaped.md", autonomousTaskMarkdown(
		"task-escaped",
		StatusPending,
		"",
		"Escaped",
		"Do not escape.",
	))

	_, err := Load(repo, path)
	if err == nil || !strings.Contains(err.Error(), "path escapes root") {
		t.Fatalf("load error = %v, want symlink escape rejection", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("outside entries = %v, error = %v; want empty", entries, readErr)
	}
}

func TestLoadRejectsAutonomousStatePathSymlinkIntoAnotherNamespace(t *testing.T) {
	repo := t.TempDir()
	other := filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("create other task namespace: %v", err)
	}
	owner := filepath.Join(repo, ".revolvr", "autonomous", "tasks", "task-owner")
	if err := os.Symlink(other, owner); err != nil {
		t.Fatalf("create task namespace symlink: %v", err)
	}
	path := writeTaskFile(t, repo, "owner.md", autonomousTaskMarkdown(
		"task-owner",
		StatusPending,
		"",
		"Owner",
		"Keep evidence in the owning task namespace.",
	))

	_, err := Load(repo, path)
	if err == nil || !strings.Contains(err.Error(), "is a symbolic link") {
		t.Fatalf("load error = %v, want task namespace symlink rejection", err)
	}
}

func TestListLoadsMixedAndAutonomousWorkflowsInSourceOrder(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "001-autonomous-blocked.md", autonomousTaskMarkdown("auto-blocked", StatusBlocked, "0", "Blocked", "Blocked."))
	writeTaskFile(t, repo, "002-autonomous-completed.md", autonomousTaskMarkdown("auto-completed", StatusCompleted, "0", "Completed", "Completed."))
	writeTaskFile(t, repo, "003-autonomous-running.md", autonomousTaskMarkdown("auto-running", StatusRunning, "0", "Running", "Running."))
	writeTaskFile(t, repo, "010-autonomous-beta.md", autonomousTaskMarkdown("auto-beta", StatusPending, "1", "Beta", "Beta."))
	writeTaskFile(t, repo, "010-autonomous-alpha.md", autonomousTaskMarkdown("auto-alpha", StatusPending, "1", "Alpha", "Alpha."))
	writeTaskFile(t, repo, "001-mixed-priority.md", taskMarkdownWithPhase("mixed-priority", StatusPending, "2", PhaseAudit))
	writeTaskFile(t, repo, "000-mixed-unprioritized.md", "# Mixed Unprioritized\n")

	tasks, err := List(repo)
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if got, want := taskSourcePaths(tasks), []string{
		filepath.Join(TasksDir, "000-mixed-unprioritized.md"),
		filepath.Join(TasksDir, "001-autonomous-blocked.md"),
		filepath.Join(TasksDir, "001-mixed-priority.md"),
		filepath.Join(TasksDir, "002-autonomous-completed.md"),
		filepath.Join(TasksDir, "003-autonomous-running.md"),
		filepath.Join(TasksDir, "010-autonomous-alpha.md"),
		filepath.Join(TasksDir, "010-autonomous-beta.md"),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("task source order = %#v, want %#v", got, want)
	}
}

func TestListRejectsMalformedFilesAndDuplicateIDsAcrossWorkflows(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		repo := t.TempDir()
		writeTaskFile(t, repo, "010-valid.md", autonomousTaskMarkdown("valid", StatusPending, "1", "Valid", "Valid."))
		writeTaskFile(t, repo, "020-malformed.md", "## Missing H1\n")
		_, err := List(repo)
		if err == nil || !strings.Contains(err.Error(), "020-malformed.md") {
			t.Fatalf("selection error = %v, want malformed source", err)
		}
	})

	t.Run("duplicate IDs across workflows", func(t *testing.T) {
		repo := t.TempDir()
		writeTaskFile(t, repo, "010-autonomous.md", autonomousTaskMarkdown("duplicate", StatusPending, "1", "Autonomous", "Autonomous."))
		writeTaskFile(t, repo, "020-mixed.md", `---
id: duplicate
status: pending
workflow: mixed-pass-v1
phase: implement
priority: 2
---
# Mixed
`)
		_, err := List(repo)
		if err == nil || !strings.Contains(err.Error(), `task id "duplicate" is duplicated`) {
			t.Fatalf("selection error = %v, want duplicate task id", err)
		}
	})
}

func TestAutonomousRetryPreservesLifecycleAndSpecificationBytes(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("---\r\nid: task-retry\r\nstatus: blocked\r\nworkflow: autonomous-v1\r\nautonomous_state_path: .revolvr/autonomous/tasks/task-retry/state.json\r\npriority: 7\r\nx-unknown: preserved\r\n---\r\n# Retry Autonomous\r\n\r\nHuman-authored body.\r\nNo final newline.")
	path := filepath.Join(TasksDir, "retry.md")
	writeFile(t, filepath.Join(repo, path), string(raw))

	updated, changed, err := UpdateBlockedToPending(repo, "task-retry")
	if err != nil {
		t.Fatalf("retry autonomous task: %v", err)
	}
	if !changed || updated.Status != StatusPending || updated.Workflow != WorkflowAutonomousV1 || updated.Phase != "" {
		t.Fatalf("updated task = %+v, changed = %v", updated, changed)
	}
	if got, want := updated.AutonomousStatePath, autonomousStatePath("task-retry"); got != want {
		t.Fatalf("state path = %q, want %q", got, want)
	}
	want := []byte(strings.Replace(string(raw), "status: blocked", "status: pending", 1))
	got, readErr := os.ReadFile(filepath.Join(repo, path))
	if readErr != nil {
		t.Fatalf("read retried task: %v", readErr)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("retried bytes = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(repo, filepath.FromSlash(updated.AutonomousStatePath))); !os.IsNotExist(err) {
		t.Fatalf("state file stat error = %v, want not exist", err)
	}

	_, changed, err = UpdateBlockedToPending(repo, "task-retry")
	if err != nil || changed {
		t.Fatalf("repeated retry changed=%v err=%v, want false/nil", changed, err)
	}
	gotAgain, readErr := os.ReadFile(filepath.Join(repo, path))
	if readErr != nil || !reflect.DeepEqual(gotAgain, want) {
		t.Fatalf("repeated retry bytes = %q, error = %v; want unchanged", gotAgain, readErr)
	}
}

func TestAutonomousMetadataUpdatesCannotAddMixedPhaseOrRemoveStateReference(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "protected.md", autonomousTaskMarkdown(
		"task-protected",
		StatusPending,
		"",
		"Protected",
		"Keep autonomous evidence attached.",
	))
	original, err := os.ReadFile(filepath.Join(repo, path))
	if err != nil {
		t.Fatalf("read original task: %v", err)
	}

	_, err = UpdateMetadata(repo, path, MetadataUpdate{Phase: PhaseAudit})
	if err == nil || !strings.Contains(err.Error(), `frontmatter key "phase" is not allowed`) {
		t.Fatalf("phase update error = %v, want autonomous phase rejection", err)
	}
	after, readErr := os.ReadFile(filepath.Join(repo, path))
	if readErr != nil || !reflect.DeepEqual(after, original) {
		t.Fatalf("task after rejected update = %q, error = %v; want original", after, readErr)
	}

	updated, err := UpdateStatus(repo, path, StatusBlocked)
	if err != nil {
		t.Fatalf("update autonomous status: %v", err)
	}
	if updated.Workflow != WorkflowAutonomousV1 || updated.AutonomousStatePath != autonomousStatePath("task-protected") {
		t.Fatalf("updated lifecycle metadata = %+v", updated)
	}
}

func TestAutonomousSnapshotUpdateRetainsSnapshotLifecycleEvidence(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "snapshot.md", autonomousTaskMarkdown(
		"task-snapshot",
		StatusPending,
		"3",
		"Snapshot",
		"Original specification.",
	))
	snapshot, err := Load(repo, path)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	writeRepoFile(t, repo, path, autonomousTaskMarkdown(
		"task-replacement",
		StatusCompleted,
		"99",
		"Replacement",
		"Intervening specification.",
	))

	updated, err := UpdateMetadataFromSnapshot(repo, snapshot, MetadataUpdate{Status: StatusBlocked})
	if err != nil {
		t.Fatalf("update from snapshot: %v", err)
	}
	if updated.ID != "task-snapshot" || updated.Status != StatusBlocked || updated.Workflow != WorkflowAutonomousV1 || updated.AutonomousStatePath != autonomousStatePath("task-snapshot") || updated.Priority != 3 {
		t.Fatalf("updated snapshot task = %+v", updated)
	}
	if !strings.Contains(updated.ContextBody, "Original specification.") || strings.Contains(updated.ContextBody, "Intervening specification.") {
		t.Fatalf("updated snapshot context = %q", updated.ContextBody)
	}
}

func autonomousStatePath(taskID string) string {
	return filepath.ToSlash(filepath.Join(".revolvr", "autonomous", "tasks", taskID, "state.json"))
}

func autonomousTaskMarkdown(id string, status string, priority string, title string, body string) string {
	var out strings.Builder
	out.WriteString(autonomousFrontmatter(id, status))
	if priority != "" {
		fmt.Fprintf(&out, "priority: %s\n", priority)
	}
	out.WriteString("---\n")
	fmt.Fprintf(&out, "# %s\n\n%s\n", title, body)
	return out.String()
}

func autonomousFrontmatter(id string, status string) string {
	var out strings.Builder
	out.WriteString("---\n")
	fmt.Fprintf(&out, "id: %s\n", id)
	if status != "" {
		fmt.Fprintf(&out, "status: %s\n", status)
	}
	fmt.Fprintf(&out, "workflow: %s\n", WorkflowAutonomousV1)
	fmt.Fprintf(&out, "autonomous_state_path: %s\n", autonomousStatePath(id))
	return out.String()
}

func autonomousTaskMarkdownWithStatePath(id string, statePath string) string {
	return fmt.Sprintf(`---
id: %s
workflow: autonomous-v1
autonomous_state_path: %s
---
# Invalid State Path
`, id, statePath)
}
