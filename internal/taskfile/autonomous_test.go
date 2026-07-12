package taskfile

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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

func TestWorkflowAwareSelectionIsDeterministicAndIsolated(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "001-autonomous-blocked.md", autonomousTaskMarkdown("auto-blocked", StatusBlocked, "0", "Blocked", "Blocked."))
	writeTaskFile(t, repo, "002-autonomous-completed.md", autonomousTaskMarkdown("auto-completed", StatusCompleted, "0", "Completed", "Completed."))
	writeTaskFile(t, repo, "003-autonomous-running.md", autonomousTaskMarkdown("auto-running", StatusRunning, "0", "Running", "Running."))
	writeTaskFile(t, repo, "010-autonomous-beta.md", autonomousTaskMarkdown("auto-beta", StatusPending, "1", "Beta", "Beta."))
	writeTaskFile(t, repo, "010-autonomous-alpha.md", autonomousTaskMarkdown("auto-alpha", StatusPending, "1", "Alpha", "Alpha."))
	writeTaskFile(t, repo, "001-mixed-priority.md", taskMarkdownWithPhase("mixed-priority", StatusPending, "2", PhaseAudit))
	writeTaskFile(t, repo, "000-mixed-unprioritized.md", "# Mixed Unprioritized\n")

	autonomous, err := ListRunnableForWorkflow(repo, WorkflowAutonomousV1)
	if err != nil {
		t.Fatalf("list autonomous tasks: %v", err)
	}
	if got, want := taskSourcePaths(autonomous), []string{
		filepath.Join(TasksDir, "010-autonomous-alpha.md"),
		filepath.Join(TasksDir, "010-autonomous-beta.md"),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("autonomous order = %#v, want %#v", got, want)
	}

	mixed, err := ListRunnableForWorkflow(repo, WorkflowMixedPassV1)
	if err != nil {
		t.Fatalf("list mixed-pass tasks: %v", err)
	}
	if got, want := taskSourcePaths(mixed), []string{
		filepath.Join(TasksDir, "001-mixed-priority.md"),
		filepath.Join(TasksDir, "000-mixed-unprioritized.md"),
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed-pass order = %#v, want %#v", got, want)
	}

	for i := 0; i < 3; i++ {
		nextAutonomous, ok, err := SelectNextForWorkflow(repo, WorkflowAutonomousV1)
		if err != nil || !ok || nextAutonomous.ID != "auto-alpha" {
			t.Fatalf("autonomous selection %d = %+v, %v, %v; want auto-alpha", i, nextAutonomous, ok, err)
		}
		nextMixed, ok, err := SelectNext(repo)
		if err != nil || !ok || nextMixed.SourcePath != filepath.Join(TasksDir, "001-mixed-priority.md") {
			t.Fatalf("default selection %d = %+v, %v, %v; want 001-mixed-priority.md", i, nextMixed, ok, err)
		}
	}

	if _, err := os.Stat(filepath.Join(repo, ".revolvr")); !os.IsNotExist(err) {
		t.Fatalf("selection created runtime state: %v", err)
	}
	if _, err := ListRunnableForWorkflow(repo, "future-workflow"); err == nil || !strings.Contains(err.Error(), "invalid workflow") {
		t.Fatalf("unknown workflow error = %v, want invalid workflow", err)
	}
}

func TestWorkflowAwareSelectionRejectsMalformedFilesAndDuplicateIDs(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		repo := t.TempDir()
		writeTaskFile(t, repo, "010-valid.md", autonomousTaskMarkdown("valid", StatusPending, "1", "Valid", "Valid."))
		writeTaskFile(t, repo, "020-malformed.md", "## Missing H1\n")
		_, _, err := SelectNextForWorkflow(repo, WorkflowAutonomousV1)
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
		_, _, err := SelectNextForWorkflow(repo, WorkflowAutonomousV1)
		if err == nil || !strings.Contains(err.Error(), `task id "duplicate" is duplicated`) {
			t.Fatalf("selection error = %v, want duplicate task id", err)
		}
	})
}

func TestAutonomousRetryPreservesLifecycleAndSpecificationBytes(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("---\r\nid: task-retry\r\nstatus: blocked\r\nworkflow: autonomous-v1\r\nautonomous_state_path: .revolvr/autonomous/tasks/task-retry/state.json\r\npriority: 7\r\nunknown: preserved\r\n---\r\n# Retry Autonomous\r\n\r\nHuman-authored body.\r\nNo final newline.")
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
