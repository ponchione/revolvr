package prompt

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"revolvr/internal/codexexec"
)

func TestBuildContextPayloadSnapshot(t *testing.T) {
	got, err := BuildContextPayload(Input{
		RunID:          "run-123",
		PassID:         "pass-123",
		TaskID:         "task-7",
		Task:           "Implement Tasks 7 and 8 only.",
		RunProfile:     testRunProfile(DefaultRunProfileName, defaultImplementerProfileContent(t)),
		RepositoryRoot: "/repo/revolvr",
		ReceiptPath:    ".revolvr/receipts/run-123.md",
		ArtifactPaths: []ArtifactPath{
			{Label: "codex stdout jsonl", Path: ".revolvr/runs/run-123/codex.jsonl"},
			{Label: "codex stderr", Path: ".revolvr/runs/run-123/codex.stderr"},
		},
		RepositoryRules: []string{
			"Preserve existing internal packages.",
			"Do not implement CLI task command wiring.",
		},
		StopCondition: "Stop after updating the receipt.",
	})
	if err != nil {
		t.Fatalf("build context payload: %v", err)
	}

	want := `# Revolvr Codex Pass

You are running one fresh bounded Codex pass controlled by revolvr.

## Run Profile
You are the implementer for this Revolvr pass.

Focus on the selected task, make small reviewable changes, preserve existing repository state, verify the work, and write the required receipt before stopping.

## Selected Task
- Task ID: ` + "`task-7`" + `
- Run ID: ` + "`run-123`" + `
- Pass ID: ` + "`pass-123`" + `
- Repository root: ` + "`/repo/revolvr`" + `
- Receipt path: ` + "`.revolvr/receipts/run-123.md`" + `
- Task text:

` + "```text" + `
Implement Tasks 7 and 8 only.
` + "```" + `

## Repository Rules
- Use one bounded task only: the selected task in this context payload.
- Write or update the receipt before stopping.
- Do not use codex resume.
- Do not launch nested Codex runs.
- Do not push branches or create PRs.
- Preserve existing internal packages.
- Do not implement CLI task command wiring.

## Artifact Paths
- codex stdout jsonl: ` + "`.revolvr/runs/run-123/codex.jsonl`" + `
- codex stderr: ` + "`.revolvr/runs/run-123/codex.stderr`" + `

## Required Receipt Schema
Write the receipt as Markdown with YAML frontmatter at the configured receipt path.
The frontmatter must include these fields:

` + "```yaml" + `
schema_version: revolvr.receipt.v1
run_id: "run-123"
pass_id: "pass-123"
task_id: "task-7"
task: "Implement Tasks 7 and 8 only."
verdict: completed
timestamp: 2026-06-25T00:00:00Z
codex_exit_code: 0
verification_status: not_run
commit_sha: ""
changed_files: []
verification: []
metrics:
  input_tokens: 0
  output_tokens: 0
  duration_seconds: 0
` + "```" + `

Use one of these verdict values: completed, completed_with_concerns, blocked, verification_failed, codex_failed, safety_limit, no_changes.
After the frontmatter, include these body sections exactly once:
- ` + "`## Summary`" + `
- ` + "`## Changed Files`" + `
- ` + "`## Verification`" + `
- ` + "`## Concerns`" + `
- ` + "`## Next Steps`" + `

## Stop Condition
Stop after updating the receipt.
Do not start another task.
`

	if got != want {
		t.Fatalf("context payload snapshot mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	assertSectionOrder(t, got, []string{
		"# Revolvr Codex Pass",
		"## Run Profile",
		"## Selected Task",
		"## Repository Rules",
		"## Artifact Paths",
		"## Required Receipt Schema",
		"## Stop Condition",
	})
}

func TestBuildRequiresLoadedRunProfile(t *testing.T) {
	_, err := BuildContextPayload(Input{
		RunID:       "run-default",
		TaskID:      "task-default",
		Task:        "Do the selected work.",
		ReceiptPath: "receipts/run-default.md",
	})
	if err == nil {
		t.Fatal("build context payload succeeded, want missing run profile error")
	}
	if !strings.Contains(err.Error(), "run profile name is required") {
		t.Fatalf("error = %v, want run profile requirement", err)
	}
}

func TestBuildUsesLoadedRunProfileBody(t *testing.T) {
	got, err := BuildContextPayload(Input{
		RunID:       "run-default",
		TaskID:      "task-default",
		Task:        "Do the selected work.",
		RunProfile:  testRunProfile(DefaultRunProfileName, "Loaded profile body.\n\nUse this exact file-backed content."),
		ReceiptPath: "receipts/run-default.md",
	})
	if err != nil {
		t.Fatalf("build context payload: %v", err)
	}

	required := []string{
		"## Run Profile",
		"Loaded profile body.\n\nUse this exact file-backed content.",
	}
	for _, want := range required {
		if !strings.Contains(got, want) {
			t.Fatalf("context payload missing %q:\n%s", want, got)
		}
	}
}

func TestBuildIncludesDefaultRulesAndRequiredInstructions(t *testing.T) {
	got, err := BuildContextPayload(Input{
		RunID:       "run-default",
		TaskID:      "task-default",
		Task:        "Do the selected work.",
		RunProfile:  testRunProfile(DefaultRunProfileName, defaultImplementerProfileContent(t)),
		ReceiptPath: "receipts/run-default.md",
	})
	if err != nil {
		t.Fatalf("build context payload: %v", err)
	}

	required := []string{
		"Run ID: `run-default`",
		"Pass ID: `run-default`",
		"Task ID: `task-default`",
		"Receipt path: `receipts/run-default.md`",
		"## Repository Rules",
		"Use one bounded task only",
		"Write or update the receipt",
		"Do not use codex resume",
		"Do not launch nested Codex runs",
		"Do not push branches or create PRs",
		"Work only in this repository and keep changes scoped to the selected task.",
		"## Artifact Paths",
		"No additional artifact paths were provided.",
		"schema_version: revolvr.receipt.v1",
		"## Stop Condition",
		DefaultStopCondition,
	}
	for _, want := range required {
		if !strings.Contains(got, want) {
			t.Fatalf("context payload missing %q:\n%s", want, got)
		}
	}
}

func TestBuildContextManifestHashesPayloadAndSources(t *testing.T) {
	generatedAt := time.Date(2026, 7, 9, 15, 30, 0, 0, time.FixedZone("offset", -4*60*60))
	payload := []byte("full context payload\nwith exact bytes\n")
	manifest, err := BuildContextManifest(ContextManifestInput{
		Input: Input{
			RunID:       "run-123",
			TaskID:      "task-7",
			Task:        "  Implement the selected task.\n\nVerify it.  ",
			ReceiptPath: ".revolvr/receipts/run-123.md",
			RunProfile: RunProfile{
				Name:        "reviewer",
				Description: "Read context carefully.\nThen make the change.",
				SourcePath:  RunProfileSourcePath("reviewer"),
			},
		},
		ContextPayload:     payload,
		ContextPayloadPath: ".revolvr/runs/run-123/context.md",
		GeneratedAt:        generatedAt,
		Invocation:         testInvocationProvenance(),
	})
	if err != nil {
		t.Fatalf("build context manifest: %v", err)
	}

	if manifest.RunID != "run-123" || manifest.TaskID != "task-7" || manifest.ProfileName != "reviewer" {
		t.Fatalf("manifest identity = %+v, want run/task/profile", manifest)
	}
	if got, want := manifest.ContextPayloadPath, ".revolvr/runs/run-123/context.md"; got != want {
		t.Fatalf("payload path = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadSHA256, sha256HexTest(payload); got != want {
		t.Fatalf("payload sha256 = %q, want %q", got, want)
	}
	if got, want := manifest.ContextPayloadByteSize, len(payload); got != want {
		t.Fatalf("payload byte size = %d, want %d", got, want)
	}
	if got, want := manifest.GeneratedAt, generatedAt.UTC(); !got.Equal(want) {
		t.Fatalf("generated_at = %s, want %s", got, want)
	}

	selectedTask := sourceByLabel(t, manifest, "selected_task")
	selectedTaskBytes := []byte("Implement the selected task.\n\nVerify it.")
	if got, want := selectedTask.SHA256, sha256HexTest(selectedTaskBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(selectedTaskBytes); got != want {
		t.Fatalf("selected task byte size = %d, want %d", got, want)
	}
	if selectedTask.Path != "" {
		t.Fatalf("selected task path = %q, want empty", selectedTask.Path)
	}

	runProfile := sourceByLabel(t, manifest, "run_profile")
	runProfileBytes := []byte("Read context carefully.\nThen make the change.")
	if got, want := runProfile.SHA256, sha256HexTest(runProfileBytes); got != want {
		t.Fatalf("run profile sha256 = %q, want %q", got, want)
	}
	if got, want := runProfile.ByteSize, len(runProfileBytes); got != want {
		t.Fatalf("run profile byte size = %d, want %d", got, want)
	}
	if got, want := runProfile.Path, filepath.Join(".agent", "profiles", "reviewer.md"); got != want {
		t.Fatalf("run profile path = %q, want %q", got, want)
	}
}

func TestBuildContextManifestUsesSelectedTaskFileSourceBytes(t *testing.T) {
	taskBytes := []byte("---\nid: task-file\npriority: 10\n---\n# File Backed Task\n\n## Goal\nUse exact bytes.\n")
	manifest, err := BuildContextManifest(ContextManifestInput{
		Input: Input{
			RunID:       "run-task-file",
			TaskID:      "task-file",
			Task:        string(taskBytes),
			TaskSource:  SourceContent{Path: filepath.Join(".agent", "tasks", "010-task.md"), Content: taskBytes},
			ReceiptPath: ".revolvr/receipts/run-task-file.md",
			RunProfile:  testRunProfile(DefaultRunProfileName, defaultImplementerProfileContent(t)),
		},
		ContextPayload:     []byte("payload"),
		ContextPayloadPath: ".revolvr/runs/run-task-file/context.md",
		GeneratedAt:        time.Date(2026, 7, 9, 20, 0, 0, 0, time.UTC),
		Invocation:         testInvocationProvenance(),
	})
	if err != nil {
		t.Fatalf("build context manifest: %v", err)
	}

	selectedTask := sourceByLabel(t, manifest, "selected_task")
	if got, want := selectedTask.Path, filepath.Join(".agent", "tasks", "010-task.md"); got != want {
		t.Fatalf("selected task path = %q, want %q", got, want)
	}
	if got, want := selectedTask.SHA256, sha256HexTest(taskBytes); got != want {
		t.Fatalf("selected task sha256 = %q, want %q", got, want)
	}
	if got, want := selectedTask.ByteSize, len(taskBytes); got != want {
		t.Fatalf("selected task byte size = %d, want %d", got, want)
	}
}

func TestMarshalContextManifestWritesStableJSON(t *testing.T) {
	generatedAt := time.Date(2026, 7, 9, 19, 30, 0, 0, time.UTC)
	invocation := testInvocationProvenance()
	manifest, err := BuildContextManifest(ContextManifestInput{
		Input: Input{
			RunID:       "run-json",
			TaskID:      "task-json",
			Task:        "Write JSON.",
			RunProfile:  testRunProfile(DefaultRunProfileName, defaultImplementerProfileContent(t)),
			ReceiptPath: ".revolvr/receipts/run-json.md",
		},
		ContextPayload:     []byte("payload"),
		ContextPayloadPath: ".revolvr/runs/run-json/context.md",
		GeneratedAt:        generatedAt,
		Invocation:         invocation,
	})
	if err != nil {
		t.Fatalf("build context manifest: %v", err)
	}

	raw, err := MarshalContextManifest(manifest)
	if err != nil {
		t.Fatalf("marshal context manifest: %v", err)
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("manifest JSON does not end with newline: %q", raw)
	}
	invocation.Argv[0] = "resume"
	if manifest.Invocation.Argv[0] != "exec" {
		t.Fatalf("caller-owned invocation argv mutated manifest: %#v", manifest.Invocation.Argv)
	}
	repeated, err := MarshalContextManifest(manifest)
	if err != nil {
		t.Fatalf("marshal context manifest again: %v", err)
	}
	if !bytes.Equal(raw, repeated) {
		t.Fatalf("manifest JSON is not deterministic:\n%s\n%s", raw, repeated)
	}
	var reparsed ContextManifest
	if err := json.Unmarshal(raw, &reparsed); err != nil {
		t.Fatalf("unmarshal manifest JSON: %v\n%s", err, raw)
	}
	if got, want := reparsed.ContextPayloadSHA256, sha256HexTest([]byte("payload")); got != want {
		t.Fatalf("reparsed payload sha256 = %q, want %q", got, want)
	}
}

func TestBuildRequiresCoreFields(t *testing.T) {
	_, err := BuildContextPayload(Input{RunID: "run-1", TaskID: "task-1", Task: "x"})
	if err == nil {
		t.Fatal("build context payload succeeded, want missing receipt path error")
	}
	if !strings.Contains(err.Error(), "receipt path is required") {
		t.Fatalf("error = %v, want receipt path requirement", err)
	}
}

func sourceByLabel(t *testing.T, manifest ContextManifest, label string) ContextSource {
	t.Helper()
	for _, source := range manifest.Sources {
		if source.Label == label {
			return source
		}
	}
	t.Fatalf("manifest source %q not found: %+v", label, manifest.Sources)
	return ContextSource{}
}

func sha256HexTest(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func assertSectionOrder(t *testing.T, content string, headings []string) {
	t.Helper()
	lastIndex := -1
	for _, heading := range headings {
		index := strings.Index(content, heading)
		if index < 0 {
			t.Fatalf("context payload missing heading %q:\n%s", heading, content)
		}
		if index <= lastIndex {
			t.Fatalf("heading %q rendered out of order:\n%s", heading, content)
		}
		lastIndex = index
	}
}

func testRunProfile(name string, content string) RunProfile {
	return RunProfile{
		Name:        name,
		Description: strings.TrimSpace(content),
		SourcePath:  RunProfileSourcePath(name),
	}
}

func defaultImplementerProfileContent(t *testing.T) string {
	t.Helper()
	for _, template := range DefaultRunProfileTemplates() {
		if template.Name == DefaultRunProfileName {
			return template.Content
		}
	}
	t.Fatalf("default profile template %q not found", DefaultRunProfileName)
	return ""
}

func testInvocationProvenance() codexexec.InvocationProvenance {
	return codexexec.InvocationProvenance{
		Executable:            "codex-test",
		Version:               "codex-test 1.2.3",
		Model:                 codexexec.DefaultModel,
		ReasoningEffort:       codexexec.DefaultReasoningEffort,
		Ephemeral:             true,
		SessionMode:           codexexec.SessionModeEphemeral,
		EffectiveConfigSchema: "test-effective-config-v1",
		EffectiveConfigSHA256: strings.Repeat("a", 64),
		Argv: []string{
			"exec", "--json", "--model", codexexec.DefaultModel,
			"-c", "model_reasoning_effort=" + codexexec.DefaultReasoningEffort,
			"--ephemeral", "--cd", "/repo", "-",
		},
		WorkingDir: "/repo",
	}
}
