package prompt

import (
	"errors"
	"fmt"
	"strings"

	"revolvr/internal/receipt"
)

const DefaultStopCondition = "Stop after this one bounded task is complete or clearly blocked, after the receipt has been written or updated."

var DefaultRepositoryRules = []string{
	"Work only in this repository and keep changes scoped to the selected task.",
	"Preserve existing user changes and avoid unrelated refactors.",
	"Let the harness decide verification and commits; do not commit, push branches, or create pull requests.",
}

type ArtifactPath struct {
	Label string
	Path  string
}

type Input struct {
	RunID           string
	PassID          string
	TaskID          string
	Task            string
	RepositoryRoot  string
	ReceiptPath     string
	ArtifactPaths   []ArtifactPath
	RepositoryRules []string
	StopCondition   string
}

func Build(in Input) (string, error) {
	normalized, err := normalize(in)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString("# Revolvr Codex Pass\n\n")
	out.WriteString("You are running one fresh bounded Codex pass controlled by revolvr.\n\n")

	out.WriteString("## Selected Task\n")
	fmt.Fprintf(&out, "- Task ID: `%s`\n", normalized.TaskID)
	fmt.Fprintf(&out, "- Run ID: `%s`\n", normalized.RunID)
	fmt.Fprintf(&out, "- Pass ID: `%s`\n", normalized.PassID)
	fmt.Fprintf(&out, "- Repository root: `%s`\n", normalized.RepositoryRoot)
	fmt.Fprintf(&out, "- Receipt path: `%s`\n", normalized.ReceiptPath)
	out.WriteString("- Task text:\n\n")
	out.WriteString("```text\n")
	out.WriteString(normalized.Task)
	out.WriteString("\n```\n\n")

	out.WriteString("## Required Operating Rules\n")
	out.WriteString("- Use one bounded task only: the selected task in this prompt.\n")
	out.WriteString("- Write or update the receipt before stopping.\n")
	out.WriteString("- Do not use codex resume.\n")
	out.WriteString("- Do not launch nested Codex runs.\n")
	out.WriteString("- Do not push branches or create PRs.\n")
	for _, rule := range normalized.RepositoryRules {
		fmt.Fprintf(&out, "- %s\n", rule)
	}
	out.WriteString("\n")

	out.WriteString("## Artifact Paths\n")
	if len(normalized.ArtifactPaths) == 0 {
		out.WriteString("- No additional artifact paths were provided.\n")
	} else {
		for _, artifact := range normalized.ArtifactPaths {
			fmt.Fprintf(&out, "- %s: `%s`\n", artifact.Label, artifact.Path)
		}
	}
	out.WriteString("\n")

	out.WriteString("## Required Receipt Schema\n")
	out.WriteString("Write the receipt as Markdown with YAML frontmatter at the configured receipt path.\n")
	out.WriteString("The frontmatter must include these fields:\n\n")
	out.WriteString("```yaml\n")
	fmt.Fprintf(&out, "schema_version: %s\n", receipt.SchemaVersion)
	fmt.Fprintf(&out, "run_id: %q\n", normalized.RunID)
	fmt.Fprintf(&out, "pass_id: %q\n", normalized.PassID)
	fmt.Fprintf(&out, "task_id: %q\n", normalized.TaskID)
	fmt.Fprintf(&out, "task: %q\n", normalized.Task)
	out.WriteString("verdict: completed\n")
	out.WriteString("timestamp: 2026-06-25T00:00:00Z\n")
	out.WriteString("codex_exit_code: 0\n")
	out.WriteString("verification_status: not_run\n")
	out.WriteString("commit_sha: \"\"\n")
	out.WriteString("changed_files: []\n")
	out.WriteString("verification: []\n")
	out.WriteString("metrics:\n")
	out.WriteString("  input_tokens: 0\n")
	out.WriteString("  output_tokens: 0\n")
	out.WriteString("  duration_seconds: 0\n")
	out.WriteString("```\n\n")
	out.WriteString("Use one of these verdict values: completed, completed_with_concerns, blocked, verification_failed, codex_failed, safety_limit, no_changes.\n")
	out.WriteString("After the frontmatter, include these body sections exactly once:\n")
	for _, section := range receipt.RequiredSections {
		fmt.Fprintf(&out, "- `## %s`\n", section)
	}
	out.WriteString("\n")

	out.WriteString("## Stop Condition\n")
	out.WriteString(normalized.StopCondition)
	out.WriteString("\n")
	out.WriteString("Do not start another task.\n")

	return out.String(), nil
}

func normalize(in Input) (Input, error) {
	in.RunID = strings.TrimSpace(in.RunID)
	in.PassID = strings.TrimSpace(in.PassID)
	in.TaskID = strings.TrimSpace(in.TaskID)
	in.Task = strings.TrimSpace(in.Task)
	in.RepositoryRoot = strings.TrimSpace(in.RepositoryRoot)
	in.ReceiptPath = strings.TrimSpace(in.ReceiptPath)
	in.StopCondition = strings.TrimSpace(in.StopCondition)

	switch {
	case in.RunID == "":
		return Input{}, errors.New("build prompt: run id is required")
	case in.TaskID == "":
		return Input{}, errors.New("build prompt: task id is required")
	case in.Task == "":
		return Input{}, errors.New("build prompt: task text is required")
	case in.ReceiptPath == "":
		return Input{}, errors.New("build prompt: receipt path is required")
	}

	if in.PassID == "" {
		in.PassID = in.RunID
	}
	if in.RepositoryRoot == "" {
		in.RepositoryRoot = "."
	}
	if in.StopCondition == "" {
		in.StopCondition = DefaultStopCondition
	}
	in.RepositoryRules = compactRules(in.RepositoryRules)
	if len(in.RepositoryRules) == 0 {
		in.RepositoryRules = append([]string(nil), DefaultRepositoryRules...)
	}
	in.ArtifactPaths = compactArtifacts(in.ArtifactPaths)
	return in, nil
}

func compactRules(rules []string) []string {
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule != "" {
			out = append(out, rule)
		}
	}
	return out
}

func compactArtifacts(paths []ArtifactPath) []ArtifactPath {
	out := make([]ArtifactPath, 0, len(paths))
	for _, path := range paths {
		label := strings.TrimSpace(path.Label)
		value := strings.TrimSpace(path.Path)
		if label == "" || value == "" {
			continue
		}
		out = append(out, ArtifactPath{Label: label, Path: value})
	}
	return out
}
