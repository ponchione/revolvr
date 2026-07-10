package taskfile

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"revolvr/internal/id"
)

const TasksDir = ".agent/tasks"

const (
	StatusPending   = "pending"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusBlocked   = "blocked"
)

const (
	WorkflowMixedPassV1 = "mixed-pass-v1"
	DefaultWorkflow     = WorkflowMixedPassV1
)

const (
	PhaseImplement = "implement"
	PhaseAudit     = "audit"
	PhaseDocument  = "document"
	PhaseSimplify  = "simplify"
	DefaultPhase   = PhaseImplement
)

type Task struct {
	ID          string
	Title       string
	Profile     string
	Status      string
	Workflow    string
	Phase       string
	Priority    int
	HasPriority bool
	ContextBody string
	SourcePath  string
	SourceBytes []byte
}

type CreateInput struct {
	ID    string
	Title string
	Body  string
}

type MetadataUpdate struct {
	Status string
	Phase  string
}

func Create(repositoryRoot string, input CreateInput) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}

	title := taskTitle(input.Title)
	if title == "" {
		return Task{}, errors.New("create task file: title is required")
	}
	body := strings.TrimSpace(normalizeLineEndings(input.Body))
	if body == "" {
		return Task{}, errors.New("create task file: body is required")
	}

	taskID := strings.TrimSpace(input.ID)
	generated := taskID == ""
	for attempts := 0; attempts < 8; attempts++ {
		if taskID == "" {
			taskID = id.New()
		}
		if !validTaskID(taskID) {
			return Task{}, fmt.Errorf("create task file: invalid task id %q", taskID)
		}

		if existing, ok, err := FindByID(root, taskID); err != nil {
			return Task{}, fmt.Errorf("create task file: %w", err)
		} else if ok {
			if generated {
				taskID = ""
				continue
			}
			return Task{}, fmt.Errorf("create task file: task id %q already exists at %s", taskID, existing.SourcePath)
		}

		task, err := writeNewTaskFile(root, taskID, title, body, generated)
		if err != nil {
			if generated && errors.Is(err, os.ErrExist) {
				taskID = ""
				continue
			}
			return Task{}, err
		}
		return task, nil
	}
	return Task{}, errors.New("create task file: generated task id collided repeatedly")
}

func Load(repositoryRoot string, path string) (Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, path)
	if err != nil {
		return Task{}, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, fmt.Errorf("load task file %s: %w", sourcePath, err)
	}
	task, err := parse(raw, sourcePath)
	if err != nil {
		return Task{}, fmt.Errorf("load task file %s: %w", sourcePath, err)
	}
	return task, nil
}

func List(repositoryRoot string) ([]Task, error) {
	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, TasksDir)
	if err := validateResolvedTaskDirectory(root, dir); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list task files: read %s: %w", TasksDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	tasks := make([]Task, 0, len(names))
	tasksByID := make(map[string]Task, len(names))
	for _, name := range names {
		task, err := Load(root, filepath.Join(TasksDir, name))
		if err != nil {
			return nil, err
		}
		if previous, exists := tasksByID[task.ID]; exists {
			return nil, fmt.Errorf("task id %q is duplicated in %s and %s", task.ID, previous.SourcePath, task.SourcePath)
		}
		tasksByID[task.ID] = task
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func ListRunnable(repositoryRoot string) ([]Task, error) {
	tasks, err := List(repositoryRoot)
	if err != nil {
		return nil, err
	}
	runnable := tasks[:0]
	for _, task := range tasks {
		if task.Status == StatusPending {
			runnable = append(runnable, task)
		}
	}
	sort.SliceStable(runnable, func(i, j int) bool {
		left := runnable[i]
		right := runnable[j]
		if left.HasPriority && right.HasPriority && left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.HasPriority != right.HasPriority {
			return left.HasPriority
		}
		return filepath.Base(left.SourcePath) < filepath.Base(right.SourcePath)
	})
	return runnable, nil
}

func SelectNext(repositoryRoot string) (Task, bool, error) {
	tasks, err := ListRunnable(repositoryRoot)
	if err != nil {
		return Task{}, false, err
	}
	if len(tasks) == 0 {
		return Task{}, false, nil
	}
	return tasks[0], true, nil
}

func FindByID(repositoryRoot string, taskID string) (Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, false, errors.New("find task file: task id is required")
	}

	tasks, err := List(repositoryRoot)
	if err != nil {
		return Task{}, false, err
	}
	var found Task
	for _, task := range tasks {
		if task.ID != taskID {
			continue
		}
		if found.ID != "" {
			return Task{}, false, fmt.Errorf("task id %q is duplicated in %s and %s", taskID, found.SourcePath, task.SourcePath)
		}
		found = task
	}
	if found.ID == "" {
		return Task{}, false, nil
	}
	return found, true, nil
}

func UpdateBlockedToPending(repositoryRoot string, taskID string) (Task, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Task{}, false, errors.New("update blocked task file: task id is required")
	}

	task, ok, err := FindByID(repositoryRoot, taskID)
	if err != nil {
		return Task{}, false, err
	}
	if !ok {
		return Task{}, false, nil
	}
	if task.Status != StatusBlocked {
		return task, false, nil
	}
	updated, err := UpdateStatus(repositoryRoot, task.SourcePath, StatusPending)
	if err != nil {
		return Task{}, false, err
	}
	return updated, true, nil
}

func UpdateStatus(repositoryRoot string, path string, status string) (Task, error) {
	return UpdateMetadata(repositoryRoot, path, MetadataUpdate{Status: status})
}

func UpdateMetadata(repositoryRoot string, path string, update MetadataUpdate) (Task, error) {
	update, err := validateMetadataUpdate(update)
	if err != nil {
		return Task{}, err
	}

	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, path)
	if err != nil {
		return Task{}, err
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	return updateMetadataFromBytes(sourcePath, absPath, raw, update)
}

func UpdateMetadataFromSnapshot(repositoryRoot string, snapshot Task, update MetadataUpdate) (Task, error) {
	update, err := validateMetadataUpdate(update)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(snapshot.SourcePath) == "" {
		return Task{}, errors.New("update task metadata from snapshot: source path is required")
	}
	if len(snapshot.SourceBytes) == 0 {
		return Task{}, errors.New("update task metadata from snapshot: source bytes are required")
	}

	root, err := repositoryRootAbs(repositoryRoot)
	if err != nil {
		return Task{}, err
	}
	sourcePath, absPath, err := resolveTaskPath(root, snapshot.SourcePath)
	if err != nil {
		return Task{}, err
	}
	parsed, err := parse(snapshot.SourceBytes, sourcePath)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata from snapshot %s: %w", sourcePath, err)
	}
	if parsed.ID != snapshot.ID {
		return Task{}, fmt.Errorf("update task metadata from snapshot %s: task id changed from %q to %q", sourcePath, snapshot.ID, parsed.ID)
	}
	return updateMetadataFromBytes(sourcePath, absPath, snapshot.SourceBytes, update)
}

func validateMetadataUpdate(update MetadataUpdate) (MetadataUpdate, error) {
	update.Status = strings.TrimSpace(update.Status)
	update.Phase = strings.TrimSpace(update.Phase)
	if update.Status == "" && update.Phase == "" {
		return MetadataUpdate{}, errors.New("update task metadata: no metadata update requested")
	}
	if update.Status != "" && !validStatus(update.Status) {
		return MetadataUpdate{}, fmt.Errorf("invalid status %q", update.Status)
	}
	if update.Phase != "" && !validPhase(update.Phase) {
		return MetadataUpdate{}, fmt.Errorf("invalid phase %q", update.Phase)
	}
	return update, nil
}

func updateMetadataFromBytes(sourcePath string, absPath string, raw []byte, update MetadataUpdate) (Task, error) {
	if _, err := parse(raw, sourcePath); err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}

	updated, err := updateMetadataBytes(raw, update)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	task, err := parse(updated, sourcePath)
	if err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	if err := writeFileAtomically(absPath, updated, 0o644); err != nil {
		return Task{}, fmt.Errorf("update task metadata %s: %w", sourcePath, err)
	}
	return task, nil
}

func writeNewTaskFile(root string, taskID string, title string, body string, generated bool) (Task, error) {
	dir := filepath.Join(root, TasksDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Task{}, fmt.Errorf("create task file: create %s: %w", TasksDir, err)
	}

	sourcePath, absPath, err := resolveTaskPath(root, filepath.Join(TasksDir, taskID+".md"))
	if err != nil {
		return Task{}, fmt.Errorf("create task file: %w", err)
	}
	content := createTaskMarkdown(taskID, title, body)
	file, err := os.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) && generated {
			return Task{}, err
		}
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, err)
	}
	_, writeErr := file.Write(content)
	closeErr := file.Close()
	if writeErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, writeErr)
	}
	if closeErr != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, closeErr)
	}

	task, err := parse(content, sourcePath)
	if err != nil {
		return Task{}, fmt.Errorf("create task file %s: %w", sourcePath, err)
	}
	return task, nil
}

func createTaskMarkdown(taskID string, title string, body string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "---\nid: %s\nstatus: %s\n---\n# %s\n\n%s\n", taskID, StatusPending, title, body)
	return []byte(out.String())
}

func taskTitle(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func (t Task) SourceSHA256() string {
	sum := sha256.Sum256(t.SourceBytes)
	return fmt.Sprintf("%x", sum)
}

func (t Task) SourceByteSize() int {
	return len(t.SourceBytes)
}

func parse(raw []byte, sourcePath string) (Task, error) {
	lines := splitLines(string(raw))
	meta, bodyStart, err := parseFrontmatter(lines)
	if err != nil {
		return Task{}, err
	}
	title, err := findH1Title(lines[bodyStart:])
	if err != nil {
		return Task{}, err
	}

	status, statusSet := meta["status"]
	status = strings.TrimSpace(status)
	if !statusSet {
		status = StatusPending
	}
	if !validStatus(status) {
		return Task{}, fmt.Errorf("invalid status %q", status)
	}

	profile := strings.TrimSpace(meta["profile"])
	if profile != "" && !validProfileName(profile) {
		return Task{}, fmt.Errorf("invalid profile name %q", profile)
	}

	workflow, workflowSet := meta["workflow"]
	workflow = strings.TrimSpace(workflow)
	if !workflowSet {
		workflow = DefaultWorkflow
	}
	if !validWorkflow(workflow) {
		return Task{}, fmt.Errorf("invalid workflow %q", workflow)
	}

	phase, phaseSet := meta["phase"]
	phase = strings.TrimSpace(phase)
	if !phaseSet {
		phase = DefaultPhase
	}
	if !validPhase(phase) {
		return Task{}, fmt.Errorf("invalid phase %q", phase)
	}

	var priority int
	hasPriority := false
	if rawPriority, prioritySet := meta["priority"]; prioritySet {
		rawPriority = strings.TrimSpace(rawPriority)
		parsed, err := strconv.Atoi(rawPriority)
		if err != nil {
			return Task{}, fmt.Errorf("invalid priority %q", rawPriority)
		}
		priority = parsed
		hasPriority = true
	}

	taskID := strings.TrimSpace(meta["id"])
	if taskID == "" {
		base := filepath.Base(sourcePath)
		taskID = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if !validTaskID(taskID) {
		return Task{}, fmt.Errorf("invalid task id %q", taskID)
	}

	return Task{
		ID:          taskID,
		Title:       title,
		Profile:     profile,
		Status:      status,
		Workflow:    workflow,
		Phase:       phase,
		Priority:    priority,
		HasPriority: hasPriority,
		ContextBody: string(raw),
		SourcePath:  sourcePath,
		SourceBytes: append([]byte(nil), raw...),
	}, nil
}

func parseFrontmatter(lines []string) (map[string]string, int, error) {
	meta := map[string]string{}
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return meta, 0, nil
	}
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "---" {
			return meta, i + 1, nil
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, 0, fmt.Errorf("invalid frontmatter line %d: expected key: value", i+1)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = trimScalar(strings.TrimSpace(value))
		switch key {
		case "id", "profile", "status", "priority", "workflow", "phase":
			if _, exists := meta[key]; exists {
				return nil, 0, fmt.Errorf("duplicate frontmatter key %q", key)
			}
			meta[key] = value
		default:
			continue
		}
	}
	return nil, 0, errors.New("unterminated frontmatter")
}

func updateMetadataBytes(raw []byte, update MetadataUpdate) ([]byte, error) {
	markdown := normalizeLineEndings(string(raw))
	lines := strings.Split(markdown, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		return updateMetadataInFrontmatter(lines, update)
	}

	var out strings.Builder
	out.WriteString("---\n")
	writeMetadataUpdate(&out, update)
	out.WriteString("---\n\n")
	out.WriteString(markdown)
	return []byte(out.String()), nil
}

func updateMetadataInFrontmatter(lines []string, update MetadataUpdate) ([]byte, error) {
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return nil, errors.New("unterminated frontmatter")
	}

	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[0])
	replacedStatus := update.Status == ""
	replacedPhase := update.Phase == ""
	for i := 1; i < end; i++ {
		switch frontmatterKey(lines[i]) {
		case "status":
			if update.Status != "" {
				out = append(out, "status: "+update.Status)
				replacedStatus = true
				continue
			}
		case "phase":
			if update.Phase != "" {
				out = append(out, "phase: "+update.Phase)
				replacedPhase = true
				continue
			}
		}
		out = append(out, lines[i])
	}
	if !replacedStatus {
		out = append(out, "status: "+update.Status)
	}
	if !replacedPhase {
		out = append(out, "phase: "+update.Phase)
	}
	out = append(out, lines[end:]...)
	return []byte(strings.Join(out, "\n")), nil
}

func writeMetadataUpdate(out *strings.Builder, update MetadataUpdate) {
	if update.Status != "" {
		fmt.Fprintf(out, "status: %s\n", update.Status)
	}
	if update.Phase != "" {
		fmt.Fprintf(out, "phase: %s\n", update.Phase)
	}
}

func frontmatterKey(line string) string {
	key, _, ok := strings.Cut(strings.TrimSpace(line), ":")
	if !ok {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(key))
}

func normalizeLineEndings(markdown string) string {
	normalized := strings.ReplaceAll(markdown, "\r\n", "\n")
	return strings.ReplaceAll(normalized, "\r", "\n")
}

func trimScalar(value string) string {
	if len(value) < 2 {
		return value
	}
	if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}

func findH1Title(lines []string) (string, error) {
	for _, line := range lines {
		heading, ok := parseHeading(line)
		if !ok || heading.level != 1 {
			continue
		}
		if heading.text == "" {
			return "", errors.New("task file has empty H1 title")
		}
		return heading.text, nil
	}
	return "", errors.New("task file has no H1 title")
}

func splitLines(markdown string) []string {
	return strings.Split(normalizeLineEndings(markdown), "\n")
}

type heading struct {
	level int
	text  string
}

func parseHeading(line string) (heading, bool) {
	leftTrimmed := strings.TrimLeft(line, " ")
	if len(line)-len(leftTrimmed) > 3 || !strings.HasPrefix(leftTrimmed, "#") {
		return heading{}, false
	}

	level := 0
	for level < len(leftTrimmed) && leftTrimmed[level] == '#' {
		level++
	}
	if level > 6 || level == len(leftTrimmed) {
		return heading{}, false
	}
	if leftTrimmed[level] != ' ' && leftTrimmed[level] != '\t' {
		return heading{}, false
	}

	text := strings.TrimSpace(leftTrimmed[level:])
	return heading{level: level, text: stripClosingHashes(text)}, true
}

func stripClosingHashes(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasSuffix(text, "#") {
		return text
	}

	lastNonHash := len(text) - 1
	for lastNonHash >= 0 && text[lastNonHash] == '#' {
		lastNonHash--
	}
	if lastNonHash < 0 || (text[lastNonHash] != ' ' && text[lastNonHash] != '\t') {
		return text
	}
	return strings.TrimSpace(text[:lastNonHash])
}

func validStatus(status string) bool {
	switch status {
	case StatusPending, StatusRunning, StatusCompleted, StatusBlocked:
		return true
	default:
		return false
	}
}

func validWorkflow(workflow string) bool {
	return workflow == WorkflowMixedPassV1
}

func validPhase(phase string) bool {
	switch phase {
	case PhaseImplement, PhaseAudit, PhaseDocument, PhaseSimplify:
		return true
	default:
		return false
	}
}

func validTaskID(taskID string) bool {
	if taskID == "" || taskID == "." || taskID == ".." {
		return false
	}
	for _, r := range taskID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func validProfileName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func repositoryRootAbs(repositoryRoot string) (string, error) {
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		repositoryRoot = "."
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	return root, nil
}

func writeFileAtomically(path string, content []byte, defaultPerm os.FileMode) error {
	perm := defaultPerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(perm); err != nil {
		return err
	}
	if _, err := temp.Write(content); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	return os.Rename(tempPath, path)
}

func resolveTaskPath(root string, path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", errors.New("task file path is required")
	}
	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(root, absPath)
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve task file path: %w", err)
	}

	taskDir := filepath.Join(root, TasksDir)
	rel, err := filepath.Rel(taskDir, absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve task file path relative to %s: %w", TasksDir, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("task file path %s is outside %s", path, TasksDir)
	}
	if err := validateResolvedTaskPath(root, taskDir, absPath, path); err != nil {
		return "", "", err
	}
	return filepath.Join(TasksDir, rel), absPath, nil
}

func validateResolvedTaskPath(root string, taskDir string, absPath string, displayPath string) error {
	resolvedTaskDir, err := validatedResolvedTaskDirectory(root, taskDir)
	if err != nil {
		return err
	}
	if resolvedTaskDir == "" {
		return nil
	}

	if info, err := os.Lstat(absPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("task file path %s is a symbolic link", displayPath)
		}
		resolvedPath, resolveErr := filepath.EvalSymlinks(absPath)
		if resolveErr != nil {
			return fmt.Errorf("resolve task file path %s symlinks: %w", displayPath, resolveErr)
		}
		if !pathWithin(resolvedTaskDir, resolvedPath) {
			return fmt.Errorf("task file path %s resolves outside %s", displayPath, TasksDir)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect task file path %s: %w", displayPath, err)
	}

	resolvedParent, err := filepath.EvalSymlinks(filepath.Dir(absPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("resolve task file parent for %s: %w", displayPath, err)
	}
	if !pathWithin(resolvedTaskDir, resolvedParent) {
		return fmt.Errorf("task file path %s resolves outside %s", displayPath, TasksDir)
	}
	return nil
}

func validateResolvedTaskDirectory(root string, taskDir string) error {
	_, err := validatedResolvedTaskDirectory(root, taskDir)
	return err
}

func validatedResolvedTaskDirectory(root string, taskDir string) (string, error) {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root symlinks: %w", err)
	}
	resolvedTaskDir, err := filepath.EvalSymlinks(taskDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("resolve %s symlinks: %w", TasksDir, err)
		}
		resolvedParent, parentErr := filepath.EvalSymlinks(filepath.Dir(taskDir))
		if parentErr != nil {
			if errors.Is(parentErr, os.ErrNotExist) {
				return "", nil
			}
			return "", fmt.Errorf("resolve %s parent symlinks: %w", TasksDir, parentErr)
		}
		resolvedTaskDir = filepath.Join(resolvedParent, filepath.Base(taskDir))
	}
	if !pathWithin(resolvedRoot, resolvedTaskDir) {
		return "", fmt.Errorf("task directory %s resolves outside repository root", TasksDir)
	}
	return resolvedTaskDir, nil
}

func pathWithin(base string, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
