package taskfile

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestListRunnableOrdersByPriorityThenFilename(t *testing.T) {
	repo := t.TempDir()
	writeTaskFile(t, repo, "030-later.md", taskMarkdown("later", "pending", "30"))
	writeTaskFile(t, repo, "010-alpha.md", taskMarkdown("alpha", "pending", "10"))
	writeTaskFile(t, repo, "010-beta.md", taskMarkdown("beta", "pending", "10"))
	writeTaskFile(t, repo, "999-unprioritized.md", "# Unprioritized\n")
	writeTaskFile(t, repo, "001-completed.md", taskMarkdown("completed", "completed", "1"))
	writeTaskFile(t, repo, "002-running.md", taskMarkdown("running", "running", "2"))

	runnable, err := ListRunnable(repo)
	if err != nil {
		t.Fatalf("list runnable task files: %v", err)
	}
	got := taskSourcePaths(runnable)
	want := []string{
		filepath.Join(TasksDir, "010-alpha.md"),
		filepath.Join(TasksDir, "010-beta.md"),
		filepath.Join(TasksDir, "030-later.md"),
		filepath.Join(TasksDir, "999-unprioritized.md"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runnable order = %#v, want %#v", got, want)
	}

	next, ok, err := SelectNext(repo)
	if err != nil {
		t.Fatalf("select next task file: %v", err)
	}
	if !ok {
		t.Fatal("select next returned ok=false, want true")
	}
	if got, want := next.SourcePath, filepath.Join(TasksDir, "010-alpha.md"); got != want {
		t.Fatalf("next source path = %q, want %q", got, want)
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
unknown: preserved
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
unknown: preserved
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

func taskMarkdown(title string, status string, priority string) string {
	return fmt.Sprintf(`---
status: %s
priority: %s
---
# %s
`, status, priority, title)
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
