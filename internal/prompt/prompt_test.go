package prompt

import (
	"strings"
	"testing"
)

func TestBuildSnapshot(t *testing.T) {
	got, err := Build(Input{
		RunID:          "run-123",
		PassID:         "pass-123",
		TaskID:         "task-7",
		Task:           "Implement Tasks 7 and 8 only.",
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
		t.Fatalf("build prompt: %v", err)
	}

	want := `# Revolvr Codex Pass

You are running one fresh bounded Codex pass controlled by revolvr.

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

## Required Operating Rules
- Use one bounded task only: the selected task in this prompt.
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
		t.Fatalf("prompt snapshot mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestBuildIncludesDefaultRulesAndRequiredInstructions(t *testing.T) {
	got, err := Build(Input{
		RunID:       "run-default",
		TaskID:      "task-default",
		Task:        "Do the selected work.",
		ReceiptPath: "receipts/run-default.md",
	})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}

	required := []string{
		"Run ID: `run-default`",
		"Pass ID: `run-default`",
		"Task ID: `task-default`",
		"Receipt path: `receipts/run-default.md`",
		"Use one bounded task only",
		"Write or update the receipt",
		"Do not use codex resume",
		"Do not launch nested Codex runs",
		"Do not push branches or create PRs",
		"schema_version: revolvr.receipt.v1",
		"## Stop Condition",
		DefaultStopCondition,
	}
	for _, want := range required {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRequiresCoreFields(t *testing.T) {
	_, err := Build(Input{RunID: "run-1", TaskID: "task-1", Task: "x"})
	if err == nil {
		t.Fatal("build prompt succeeded, want missing receipt path error")
	}
	if !strings.Contains(err.Error(), "receipt path is required") {
		t.Fatalf("error = %v, want receipt path requirement", err)
	}
}
