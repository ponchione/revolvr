package taskfile

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/runtimepath"
)

func TestLoadValidTaskFile(t *testing.T) {
	repo := t.TempDir()
	raw := `---
id: task-loader
profile: implementer
status: pending
priority: 3
---
# Add task file loader

## Goal
Load Markdown task files from the repository.

## Verification
- go test ./internal/taskfile
`
	path := writeTaskFile(t, repo, "003-loader.md", raw)

	task, err := Load(repo, path)
	if err != nil {
		t.Fatalf("load task file: %v", err)
	}

	if task.ID != "task-loader" {
		t.Fatalf("task id = %q, want task-loader", task.ID)
	}
	if task.Title != "Add task file loader" {
		t.Fatalf("task title = %q, want Add task file loader", task.Title)
	}
	if task.Profile != "implementer" {
		t.Fatalf("task profile = %q, want implementer", task.Profile)
	}
	if task.Status != StatusPending {
		t.Fatalf("task status = %q, want pending", task.Status)
	}
	if !task.HasPriority || task.Priority != 3 {
		t.Fatalf("task priority = %d/%v, want 3/true", task.Priority, task.HasPriority)
	}
	if got, want := task.ContextBody, raw; got != want {
		t.Fatalf("context body = %q, want exact markdown %q", got, want)
	}
	if got, want := task.SourcePath, filepath.Join(TasksDir, "003-loader.md"); got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}
	if got, want := task.SourceSHA256(), sha256HexTest([]byte(raw)); got != want {
		t.Fatalf("source sha256 = %q, want %q", got, want)
	}
	if got, want := task.SourceByteSize(), len([]byte(raw)); got != want {
		t.Fatalf("source byte size = %d, want %d", got, want)
	}
}

func TestLoadDefaultsIDAndStatus(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "plain-task.md", "# Plain Task\n\n## Goal\nDo it.\n")

	task, err := Load(repo, filepath.Join(TasksDir, "plain-task.md"))
	if err != nil {
		t.Fatalf("load task file: %v", err)
	}

	if got, want := task.ID, "plain-task"; got != want {
		t.Fatalf("task id = %q, want %q", got, want)
	}
	if got, want := task.Status, StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Workflow, DefaultWorkflow; got != want {
		t.Fatalf("task workflow = %q, want %q", got, want)
	}
	if got, want := task.Phase, DefaultPhase; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}
}

func TestLoadExplicitWorkflowAndPhase(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "explicit-workflow.md", `---
workflow: mixed-pass-v1
phase: audit
---
# Explicit Workflow
`)

	task, err := Load(repo, path)
	if err != nil {
		t.Fatalf("load task file: %v", err)
	}

	if got, want := task.Workflow, WorkflowMixedPassV1; got != want {
		t.Fatalf("task workflow = %q, want %q", got, want)
	}
	if got, want := task.Phase, PhaseAudit; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}
}

func TestLoadAcceptsEveryPhase(t *testing.T) {
	repo := t.TempDir()
	phases := []string{PhaseImplement, PhaseAudit, PhaseDocument, PhaseSimplify}

	for _, phase := range phases {
		t.Run(phase, func(t *testing.T) {
			path := writeTaskFile(t, repo, phase+".md", fmt.Sprintf(`---
workflow: mixed-pass-v1
phase: %s
---
# %s Phase
`, phase, strings.Title(phase)))

			task, err := Load(repo, path)
			if err != nil {
				t.Fatalf("load task file: %v", err)
			}
			if got := task.Phase; got != phase {
				t.Fatalf("task phase = %q, want %q", got, phase)
			}
		})
	}
}

func TestLoadRejectsMissingH1(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "missing-title.md", "## Goal\nDo the work.\n")

	_, err := Load(repo, path)
	if err == nil {
		t.Fatal("load task file succeeded, want missing H1 error")
	}
	if !strings.Contains(err.Error(), "no H1 title") {
		t.Fatalf("error = %v, want missing H1 title", err)
	}
}

func TestLoadRejectsInvalidStatus(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "bad-status.md", `---
status: ready
---
# Bad Status
`)

	_, err := Load(repo, path)
	if err == nil {
		t.Fatal("load task file succeeded, want invalid status error")
	}
	if !strings.Contains(err.Error(), `invalid status "ready"`) {
		t.Fatalf("error = %v, want invalid status", err)
	}
}

func TestLoadRejectsInvalidWorkflow(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "bad-workflow.md", `---
workflow: single-pass-v1
---
# Bad Workflow
`)

	_, err := Load(repo, path)
	if err == nil {
		t.Fatal("load task file succeeded, want invalid workflow error")
	}
	if !strings.Contains(err.Error(), `invalid workflow "single-pass-v1"`) {
		t.Fatalf("error = %v, want invalid workflow", err)
	}
}

func TestLoadRejectsInvalidPhase(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "bad-phase.md", `---
phase: review
---
# Bad Phase
`)

	_, err := Load(repo, path)
	if err == nil {
		t.Fatal("load task file succeeded, want invalid phase error")
	}
	if !strings.Contains(err.Error(), `invalid phase "review"`) {
		t.Fatalf("error = %v, want invalid phase", err)
	}
}

func TestLoadRejectsDuplicateWorkflowAndPhase(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "workflow",
			content: `---
workflow: mixed-pass-v1
workflow: mixed-pass-v1
---
# Duplicate Workflow
`,
			want: `duplicate frontmatter key "workflow"`,
		},
		{
			name: "phase",
			content: `---
phase: implement
phase: audit
---
# Duplicate Phase
`,
			want: `duplicate frontmatter key "phase"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			path := writeTaskFile(t, repo, tt.name+".md", tt.content)

			_, err := Load(repo, path)
			if err == nil {
				t.Fatal("load task file succeeded, want duplicate frontmatter key error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsUnsupportedFrontmatterKeysWithExactLocation(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{
			name: "dependency typo",
			key:  "depend_on",
			want: `unsupported frontmatter key "depend_on" at .agent/tasks/dependency-typo.md:4`,
		},
		{
			name: "preserve exact key",
			key:  "Depend_On",
			want: `unsupported frontmatter key "Depend_On" at .agent/tasks/preserve-exact-key.md:4`,
		},
		{
			name: "empty extension namespace",
			key:  "x-",
			want: `unsupported frontmatter key "x-" at .agent/tasks/empty-extension-namespace.md:4`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			name := strings.ReplaceAll(tt.name, " ", "-") + ".md"
			path := writeTaskFile(t, repo, name, fmt.Sprintf("---\nid: strict-task\nstatus: pending\n%s: ignored\n---\n# Strict Task\n", tt.key))

			_, err := Load(repo, path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("load error = %v, want %q", err, tt.want)
			}
			_, err = List(repo)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("list error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsDuplicateExtensionKey(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "duplicate-extension.md", `---
x-owner: first
X-OWNER: second
---
# Duplicate Extension
`)

	_, err := Load(repo, path)
	if err == nil || !strings.Contains(err.Error(), `duplicate frontmatter key "x-owner"`) {
		t.Fatalf("error = %v, want duplicate extension key", err)
	}
}

func TestExtensionMetadataIsInertAndPreservedByUpdate(t *testing.T) {
	repo := t.TempDir()
	raw := []byte("---\r\nid: extension-task\r\nstatus: pending\r\nx-status: completed\r\nx-depends_on: missing-task\r\nx-owner:  Preserve exact spacing  \r\n---\r\n# Extension Task\r\n\r\nBody without final newline")
	path := filepath.Join(repo, TasksDir, "extension.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create task directory: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}

	task, err := Load(repo, filepath.Join(TasksDir, "extension.md"))
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != StatusPending || len(task.DependsOn) != 0 {
		t.Fatalf("extension metadata changed control fields: status=%q depends_on=%v", task.Status, task.DependsOn)
	}
	updated, err := UpdateStatus(repo, task.SourcePath, StatusBlocked)
	if err != nil {
		t.Fatalf("update status: %v", err)
	}
	want := bytes.Replace(raw, []byte("status: pending"), []byte("status: blocked"), 1)
	if !bytes.Equal(updated.SourceBytes, want) {
		t.Fatalf("updated source changed extension bytes\ngot:  %q\nwant: %q", updated.SourceBytes, want)
	}
}

func TestLoadRejectsUnsafeProfile(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "bad-profile.md", `---
profile: ../implementer
---
# Bad Profile
`)

	_, err := Load(repo, path)
	if err == nil {
		t.Fatal("load task file succeeded, want invalid profile error")
	}
	if !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("error = %v, want invalid profile name", err)
	}
}

func TestLoadRejectsUnsafeTaskID(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "bad-id.md", "---\nid: task with spaces\n---\n# Bad ID\n")

	_, err := Load(repo, path)
	if err == nil || !strings.Contains(err.Error(), `invalid task id "task with spaces"`) {
		t.Fatalf("error = %v, want invalid task id", err)
	}
}

func TestLoadRejectsPathOutsideTasksDir(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(repo, ".agent", "outside.md")
	if err := os.MkdirAll(filepath.Dir(outside), 0o755); err != nil {
		t.Fatalf("create outside parent: %v", err)
	}
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o644); err != nil {
		t.Fatalf("write outside task: %v", err)
	}

	_, err := Load(repo, filepath.Join(".agent", "outside.md"))
	if err == nil {
		t.Fatal("load task file succeeded, want outside tasks dir error")
	}
	if !strings.Contains(err.Error(), "outside "+TasksDir) {
		t.Fatalf("error = %v, want outside tasks dir", err)
	}
}

func TestLoadAndUpdateRejectSymlinkedTaskFile(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	outsideContent := "# Outside\n"
	writeFile(t, outside, outsideContent)
	taskDir := filepath.Join(repo, TasksDir)
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("create task dir: %v", err)
	}
	linkPath := filepath.Join(taskDir, "linked.md")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Fatalf("create task symlink: %v", err)
	}

	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{name: "load", run: func() error {
			_, err := Load(repo, filepath.Join(TasksDir, "linked.md"))
			return err
		}},
		{name: "update", run: func() error {
			_, err := UpdateStatus(repo, filepath.Join(TasksDir, "linked.md"), StatusBlocked)
			return err
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			if err == nil || !strings.Contains(err.Error(), "is a symbolic link") {
				t.Fatalf("error = %v, want symbolic link rejection", err)
			}
		})
	}
	content, err := os.ReadFile(outside)
	if err != nil {
		t.Fatalf("read outside file: %v", err)
	}
	if got := string(content); got != outsideContent {
		t.Fatalf("outside content = %q, want unchanged %q", got, outsideContent)
	}
}

func TestCreateRejectsTasksDirectorySymlinkOutsideRepository(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agent"), 0o755); err != nil {
		t.Fatalf("create agent dir: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, TasksDir)); err != nil {
		t.Fatalf("create tasks directory symlink: %v", err)
	}

	_, err := Create(repo, CreateInput{ID: "outside-task", Title: "Outside Task", Body: "Do not write outside."})
	if err == nil || !strings.Contains(err.Error(), "resolves outside repository root") {
		t.Fatalf("create error = %v, want outside repository rejection", err)
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("outside directory entries = %v err=%v, want empty", entries, readErr)
	}
}

func TestWriteNewTaskFileRejectsAgentSymlinkBeforeCreatingTasksDirectory(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside-authority\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(repo, ".agent")); err != nil {
		t.Fatal(err)
	}

	_, err := writeNewTaskFile(repo, "outside-task", "Outside Task", "Do not write outside.", nil, nil, nil, false)
	if !errors.Is(err, runtimepath.ErrUnsafe) {
		t.Fatalf("create error = %v, want unsafe .agent symlink rejection", err)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("outside entries = %v err=%v, want unchanged sentinel only", entries, readErr)
	}
}

func TestWriteNewTaskFileRejectsFinalSymlinkWithoutOutsideMutation(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "sentinel")
	want := []byte("outside-authority\n")
	if err := os.WriteFile(outside, want, 0o600); err != nil {
		t.Fatal(err)
	}
	tasksDir := filepath.Join(repo, TasksDir)
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(tasksDir, "outside-task.md")); err != nil {
		t.Fatal(err)
	}

	_, err := writeNewTaskFile(repo, "outside-task", "Outside Task", "Do not write outside.", nil, nil, nil, false)
	if err == nil || !strings.Contains(err.Error(), "is a symbolic link") {
		t.Fatalf("create error = %v, want final symlink rejection", err)
	}
	if got, readErr := os.ReadFile(outside); readErr != nil || !bytes.Equal(got, want) {
		t.Fatalf("outside bytes = %q err=%v, want %q", got, readErr, want)
	}
}

func TestListLoadsOnlyDirectMarkdownFiles(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "direct.md", "# Direct\n")
	writeRepoFile(t, repo, filepath.Join(TasksDir, "notes.txt"), "# Notes\n")
	writeRepoFile(t, repo, filepath.Join(TasksDir, "nested", "nested.md"), "# Nested\n")

	tasks, err := List(repo)
	if err != nil {
		t.Fatalf("list task files: %v", err)
	}
	got := taskSourcePaths(tasks)
	want := []string{filepath.Join(TasksDir, "direct.md")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listed task files = %#v, want %#v", got, want)
	}
}

func TestTaskDiscoveryReservesAGENTSForGuidance(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "AGENTS.md", "Instructions that are not a task document.\n")
	writeTaskFile(t, repo, "existing.md", taskMarkdownWithPhase("Existing", StatusPending, "1", PhaseAudit))

	tasks, err := List(repo)
	if err != nil {
		t.Fatalf("list task files: %v", err)
	}
	if got, want := taskSourcePaths(tasks), []string{filepath.Join(TasksDir, "existing.md")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listed task files = %#v, want %#v", got, want)
	}

	found, ok, err := FindByID(repo, "existing")
	if err != nil || !ok {
		t.Fatalf("find existing task = %+v, %v, %v", found, ok, err)
	}
	if got, want := found.Phase, PhaseAudit; got != want {
		t.Fatalf("existing task phase = %q, want %q", got, want)
	}

	created, err := Create(repo, CreateInput{ID: "created", Title: "Created", Body: "Create beside nested guidance."})
	if err != nil {
		t.Fatalf("create task beside guidance: %v", err)
	}
	if got, want := created.ID, "created"; got != want {
		t.Fatalf("created task id = %q, want %q", got, want)
	}
}

func TestTaskDiscoveryStillRejectsMalformedTaskDocuments(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "AGENTS.md", "Instructions that are not a task document.\n")
	writeTaskFile(t, repo, "malformed.md", "## Missing H1\n")

	_, err := List(repo)
	if err == nil {
		t.Fatal("list malformed task files succeeded, want error")
	}
	for _, want := range []string{"malformed.md", "no H1 title"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("list error = %v, want %q", err, want)
		}
	}
}

func TestCreateWritesCanonicalPendingTaskFile(t *testing.T) {
	repo := t.TempDir()

	task, err := Create(repo, CreateInput{
		ID:    "task-created",
		Title: "  Created Task  ",
		Body:  "  Do the created work.\r\n\r\n### Verification\r\n- go test ./internal/taskfile  ",
	})
	if err != nil {
		t.Fatalf("create task file: %v", err)
	}
	if got, want := task.ID, "task-created"; got != want {
		t.Fatalf("task id = %q, want %q", got, want)
	}
	if got, want := task.Status, StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Title, "Created Task"; got != want {
		t.Fatalf("task title = %q, want %q", got, want)
	}
	if got, want := task.SourcePath, filepath.Join(TasksDir, "task-created.md"); got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}

	wantContent := `---
id: task-created
status: pending
---
# Created Task

Do the created work.

### Verification
- go test ./internal/taskfile
`
	if got := readRepoFile(t, repo, task.SourcePath); got != wantContent {
		t.Fatalf("created content = %q, want %q", got, wantContent)
	}
	if got, want := task.ContextBody, wantContent; got != want {
		t.Fatalf("context body = %q, want %q", got, want)
	}
}

func TestCreateGeneratesTaskIDAndRejectsDuplicateExplicitID(t *testing.T) {
	repo := t.TempDir()

	first, err := Create(repo, CreateInput{
		Title: "Generated ID Task",
		Body:  "Do the work.",
	})
	if err != nil {
		t.Fatalf("create generated task file: %v", err)
	}
	if first.ID == "" {
		t.Fatal("generated task id is empty")
	}
	if got, want := first.SourcePath, filepath.Join(TasksDir, first.ID+".md"); got != want {
		t.Fatalf("source path = %q, want %q", got, want)
	}

	_, err = Create(repo, CreateInput{
		ID:    first.ID,
		Title: "Duplicate",
		Body:  "Do duplicate work.",
	})
	if err == nil {
		t.Fatal("create duplicate explicit task id succeeded, want error")
	}
	if !strings.Contains(err.Error(), "already exists") || !strings.Contains(err.Error(), first.SourcePath) {
		t.Fatalf("duplicate error = %v, want existing source path", err)
	}
}

func TestFindByIDFindsFrontmatterAndFilenameIDs(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "frontmatter.md", `---
id: task-frontmatter
status: pending
---
# Frontmatter ID
`)
	writeTaskFile(t, repo, "filename-id.md", "# Filename ID\n")

	task, ok, err := FindByID(repo, " task-frontmatter ")
	if err != nil {
		t.Fatalf("find frontmatter id: %v", err)
	}
	if !ok {
		t.Fatal("find frontmatter id ok=false, want true")
	}
	if got, want := task.SourcePath, filepath.Join(TasksDir, "frontmatter.md"); got != want {
		t.Fatalf("frontmatter task path = %q, want %q", got, want)
	}

	task, ok, err = FindByID(repo, "filename-id")
	if err != nil {
		t.Fatalf("find filename id: %v", err)
	}
	if !ok {
		t.Fatal("find filename id ok=false, want true")
	}
	if got, want := task.SourcePath, filepath.Join(TasksDir, "filename-id.md"); got != want {
		t.Fatalf("filename task path = %q, want %q", got, want)
	}

	if _, ok, err := FindByID(repo, "missing-task"); err != nil || ok {
		t.Fatalf("find missing = ok %v err %v, want ok=false nil error", ok, err)
	}
}

func TestFindByIDRejectsDuplicateIDs(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "one.md", `---
id: duplicated
---
# One
`)
	writeTaskFile(t, repo, "two.md", `---
id: duplicated
---
# Two
`)

	_, ok, err := FindByID(repo, "duplicated")
	if err == nil {
		t.Fatal("find duplicate id succeeded, want error")
	}
	if ok {
		t.Fatal("find duplicate id ok=true, want false")
	}
	if !strings.Contains(err.Error(), "duplicated") || !strings.Contains(err.Error(), "one.md") || !strings.Contains(err.Error(), "two.md") {
		t.Fatalf("duplicate id error = %v, want both paths", err)
	}
}

func TestListRejectsDuplicateTaskIDs(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "010-one.md", "---\nid: duplicated\nstatus: pending\n---\n# One\n")
	writeTaskFile(t, repo, "020-two.md", "---\nid: duplicated\nstatus: pending\n---\n# Two\n")

	_, err := List(repo)
	if err == nil {
		t.Fatal("list duplicate id succeeded, want error")
	}
	for _, want := range []string{"duplicated", "010-one.md", "020-two.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("duplicate selection error = %v, want %q", err, want)
		}
	}
}

func TestUpdateBlockedToPendingUpdatesOnlyBlockedTasks(t *testing.T) {
	repo := t.TempDir()
	blockedPath := writeTaskFile(t, repo, "blocked.md", taskMarkdown("blocked", "blocked", "1"))
	writeTaskFile(t, repo, "completed.md", taskMarkdown("completed", "completed", "2"))

	updated, changed, err := UpdateBlockedToPending(repo, "blocked")
	if err != nil {
		t.Fatalf("update blocked to pending: %v", err)
	}
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if got, want := updated.Status, StatusPending; got != want {
		t.Fatalf("updated status = %q, want %q", got, want)
	}
	if !strings.Contains(readRepoFile(t, repo, blockedPath), "status: pending") {
		t.Fatalf("blocked task file was not updated:\n%s", readRepoFile(t, repo, blockedPath))
	}

	notChanged, changed, err := UpdateBlockedToPending(repo, "completed")
	if err != nil {
		t.Fatalf("update completed to pending: %v", err)
	}
	if changed {
		t.Fatal("completed task changed = true, want false")
	}
	if got, want := notChanged.Status, StatusCompleted; got != want {
		t.Fatalf("not changed status = %q, want %q", got, want)
	}

	missing, changed, err := UpdateBlockedToPending(repo, "missing")
	if err != nil {
		t.Fatalf("update missing to pending: %v", err)
	}
	if changed || missing.ID != "" {
		t.Fatalf("missing result = %+v changed=%v, want zero false", missing, changed)
	}
}

func TestUpdateStatusReplacesExistingStatus(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "replace.md", `---
id: replace-status
profile: implementer
status: pending
priority: 7
x-unknown: preserved
---
# Replace Status

Body stays put.
`)

	task, err := UpdateStatus(repo, path, StatusCompleted)
	if err != nil {
		t.Fatalf("update task status: %v", err)
	}
	if got, want := task.Status, StatusCompleted; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
id: replace-status
profile: implementer
status: completed
priority: 7
x-unknown: preserved
---
# Replace Status

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateStatusInsertsIntoExistingFrontmatter(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "insert-frontmatter.md", `---
id: insert-status
priority: 2
---
# Insert Status

Body stays put.
`)

	task, err := UpdateStatus(repo, path, StatusBlocked)
	if err != nil {
		t.Fatalf("update task status: %v", err)
	}
	if got, want := task.Status, StatusBlocked; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
id: insert-status
priority: 2
status: blocked
---
# Insert Status

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateStatusAddsFrontmatterWhenMissing(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "plain.md", "# Plain\n\nBody stays put.\n")

	task, err := UpdateStatus(repo, path, StatusCompleted)
	if err != nil {
		t.Fatalf("update task status: %v", err)
	}
	if got, want := task.Status, StatusCompleted; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
status: completed
---

# Plain

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateStatusPreservesWorkflowAndPhase(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "workflow-phase.md", `---
id: workflow-phase
workflow: mixed-pass-v1
phase: document
status: pending
---
# Workflow Phase

Body stays put.
`)

	task, err := UpdateStatus(repo, path, StatusBlocked)
	if err != nil {
		t.Fatalf("update task status: %v", err)
	}
	if got, want := task.Workflow, WorkflowMixedPassV1; got != want {
		t.Fatalf("task workflow = %q, want %q", got, want)
	}
	if got, want := task.Phase, PhaseDocument; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
id: workflow-phase
workflow: mixed-pass-v1
phase: document
status: blocked
---
# Workflow Phase

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateMetadataReplacesStatusAndPhase(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "advance.md", `---
id: advance-task
workflow: mixed-pass-v1
phase: audit
status: pending
priority: 4
x-unknown: preserved
---
# Advance Task

Body stays put.
`)

	task, err := UpdateMetadata(repo, path, MetadataUpdate{
		Status: StatusPending,
		Phase:  PhaseDocument,
	})
	if err != nil {
		t.Fatalf("update task metadata: %v", err)
	}
	if got, want := task.Status, StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Phase, PhaseDocument; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
id: advance-task
workflow: mixed-pass-v1
phase: document
status: pending
priority: 4
x-unknown: preserved
---
# Advance Task

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateMetadataInsertsMissingPhase(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "missing-phase.md", `---
id: missing-phase
status: pending
---
# Missing Phase

Body stays put.
`)

	task, err := UpdateMetadata(repo, path, MetadataUpdate{
		Status: StatusPending,
		Phase:  PhaseAudit,
	})
	if err != nil {
		t.Fatalf("update task metadata: %v", err)
	}
	if got, want := task.Phase, PhaseAudit; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
id: missing-phase
status: pending
phase: audit
---
# Missing Phase

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateMetadataAddsFrontmatterWhenMissing(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "plain-metadata.md", "# Plain Metadata\n\nBody stays put.\n")

	task, err := UpdateMetadata(repo, path, MetadataUpdate{
		Status: StatusPending,
		Phase:  PhaseAudit,
	})
	if err != nil {
		t.Fatalf("update task metadata: %v", err)
	}
	if got, want := task.Status, StatusPending; got != want {
		t.Fatalf("task status = %q, want %q", got, want)
	}
	if got, want := task.Phase, PhaseAudit; got != want {
		t.Fatalf("task phase = %q, want %q", got, want)
	}

	content := readRepoFile(t, repo, path)
	want := `---
status: pending
phase: audit
---

# Plain Metadata

Body stays put.
`
	if content != want {
		t.Fatalf("updated content = %q, want %q", content, want)
	}
}

func TestUpdateMetadataFromSnapshotDiscardsInterveningTaskMutation(t *testing.T) {
	repo := t.TempDir()
	path := writeTaskFile(t, repo, "snapshot.md", `---
id: stable-task
status: pending
workflow: mixed-pass-v1
phase: audit
priority: 4
---
# Stable Task

Original body.
`)
	snapshot, err := Load(repo, path)
	if err != nil {
		t.Fatalf("load task snapshot: %v", err)
	}
	writeRepoFile(t, repo, path, `---
id: replaced-task
status: completed
workflow: mixed-pass-v1
phase: simplify
priority: 99
---
# Replaced Task

Mutated body.
`)

	updated, err := UpdateMetadataFromSnapshot(repo, snapshot, MetadataUpdate{Status: StatusBlocked})
	if err != nil {
		t.Fatalf("update from snapshot: %v", err)
	}
	if updated.ID != "stable-task" || updated.Status != StatusBlocked || updated.Phase != PhaseAudit || updated.Priority != 4 {
		t.Fatalf("updated task = %+v, want original identity/metadata with blocked status", updated)
	}
	want := `---
id: stable-task
status: blocked
workflow: mixed-pass-v1
phase: audit
priority: 4
---
# Stable Task

Original body.
`
	if got := readRepoFile(t, repo, path); got != want {
		t.Fatalf("restored task = %q, want %q", got, want)
	}
}

func taskMarkdown(title string, status string, priority string) string {
	return fmt.Sprintf(`---
status: %s
priority: %s
---
# %s
`, status, priority, title)
}

func taskMarkdownWithPhase(title string, status string, priority string, phase string) string {
	return fmt.Sprintf(`---
status: %s
priority: %s
workflow: mixed-pass-v1
phase: %s
---
# %s
`, status, priority, phase, title)
}

func taskSourcePaths(tasks []Task) []string {
	paths := make([]string, 0, len(tasks))
	for _, task := range tasks {
		paths = append(paths, task.SourcePath)
	}
	return paths
}

func writeTaskFile(t *testing.T, repo string, name string, content string) string {
	t.Helper()
	path := filepath.Join(repo, TasksDir, name)
	writeFile(t, path, content)
	return filepath.Join(TasksDir, name)
}

func writeRepoFile(t *testing.T, repo string, path string, content string) {
	t.Helper()
	writeFile(t, filepath.Join(repo, path), content)
}

func readRepoFile(t *testing.T, repo string, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repo, path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func sha256HexTest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}
