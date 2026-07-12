package prompt

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"revolvr/internal/autonomous"
)

func TestLoadRunProfileLoadsMarkdownFile(t *testing.T) {
	repo := t.TempDir()
	content := "Audit this run.\n\nKeep findings concrete.\n"
	writeProfileFile(t, repo, "auditor", content)

	profile, err := LoadRunProfile(repo, "auditor")
	if err != nil {
		t.Fatalf("load run profile: %v", err)
	}

	if profile.Name != "auditor" {
		t.Fatalf("profile name = %q, want auditor", profile.Name)
	}
	if got, want := profile.Description, strings.TrimSpace(content); got != want {
		t.Fatalf("profile description = %q, want %q", got, want)
	}
	if got, want := profile.SourcePath, filepath.Join(".agent", "profiles", "auditor.md"); got != want {
		t.Fatalf("profile source path = %q, want %q", got, want)
	}
}

func TestLoadRunProfileUsesDefaultName(t *testing.T) {
	repo := t.TempDir()
	writeProfileFile(t, repo, DefaultRunProfileName, "Default implementer body.\n")

	profile, err := LoadRunProfile(repo, "")
	if err != nil {
		t.Fatalf("load run profile: %v", err)
	}
	if profile.Name != DefaultRunProfileName {
		t.Fatalf("profile name = %q, want %q", profile.Name, DefaultRunProfileName)
	}
}

func TestLoadRunProfileMissingFile(t *testing.T) {
	_, err := LoadRunProfile(t.TempDir(), "implementer")
	if err == nil {
		t.Fatal("load run profile succeeded, want missing file error")
	}
	if !strings.Contains(err.Error(), "missing "+filepath.Join(".agent", "profiles", "implementer.md")) {
		t.Fatalf("error = %v, want missing profile path", err)
	}
}

func TestLoadRunProfileRejectsEmptyProfile(t *testing.T) {
	repo := t.TempDir()
	writeProfileFile(t, repo, "implementer", " \n\t\n")

	_, err := LoadRunProfile(repo, "implementer")
	if err == nil {
		t.Fatal("load run profile succeeded, want empty profile error")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("error = %v, want empty profile message", err)
	}
}

func TestLoadRunProfileRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{
		"../implementer",
		"nested/implementer",
		"nested\\implementer",
		"implementer.md",
		".hidden",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadRunProfile(t.TempDir(), name)
			if err == nil {
				t.Fatal("load run profile succeeded, want invalid name error")
			}
			if !strings.Contains(err.Error(), "invalid profile name") {
				t.Fatalf("error = %v, want invalid profile name", err)
			}
		})
	}
}

func TestDefaultRunProfileTemplatesIncludesSimplifier(t *testing.T) {
	template := runProfileTemplateByName(t, "simplifier")
	for _, want := range []string{
		"You are the simplifier for this Revolvr pass.",
		"Reduce unnecessary complexity, duplication, and line count only when doing so is meaningful.",
		"Preserve behavior",
		"avoid clever abstractions",
		"create helpers only when they reduce real duplication or complexity",
		"stop cleanly when no simplification is worthwhile",
	} {
		if !strings.Contains(template.Content, want) {
			t.Fatalf("simplifier template missing %q:\n%s", want, template.Content)
		}
	}

	repo := t.TempDir()
	writeProfileFile(t, repo, template.Name, template.Content)
	profile, err := LoadRunProfile(repo, "simplifier")
	if err != nil {
		t.Fatalf("load seeded simplifier profile: %v", err)
	}
	if got, want := profile.Description, strings.TrimSpace(template.Content); got != want {
		t.Fatalf("simplifier description = %q, want %q", got, want)
	}
}

func TestDefaultRunProfileTemplatesNamesAreUniqueAndDeterministic(t *testing.T) {
	templates := DefaultRunProfileTemplates()
	got := make([]string, 0, len(templates))
	seen := make(map[string]struct{}, len(templates))
	for _, template := range templates {
		if _, exists := seen[template.Name]; exists {
			t.Fatalf("duplicate profile template name %q", template.Name)
		}
		seen[template.Name] = struct{}{}
		got = append(got, template.Name)
	}

	want := []string{
		"supervisor",
		"planner",
		DefaultRunProfileName,
		"auditor",
		"corrector",
		"documentor",
		"simplifier",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile template order = %#v, want %#v", got, want)
	}
	if DefaultRunProfileName != "implementer" {
		t.Fatalf("default profile name = %q, want implementer", DefaultRunProfileName)
	}
}

func TestAutonomousWorkerProfileTemplateNamesMatchContracts(t *testing.T) {
	for _, workerProfile := range []autonomous.WorkerProfile{
		autonomous.WorkerProfilePlanner,
		autonomous.WorkerProfileCorrector,
	} {
		t.Run(string(workerProfile), func(t *testing.T) {
			template := runProfileTemplateByName(t, string(workerProfile))
			if template.Name != string(workerProfile) {
				t.Fatalf("profile template name = %q, want worker profile %q", template.Name, workerProfile)
			}
		})
	}
}

func TestAutonomousRunProfileTemplatesDefineRoleContracts(t *testing.T) {
	tests := []struct {
		name     string
		required []string
	}{
		{
			name: "supervisor",
			required: []string{
				"fresh, decision-only, read-only session",
				"complete decision context",
				"Make exactly one next-action recommendation",
				"structured output schema exactly",
				"emit exactly one JSON decision and no surrounding prose",
				"Use only these actions: plan, implement, audit, correct, document, simplify, complete, block, or needs_input",
				"plan -> planner; implement -> implementer; audit -> auditor; correct -> corrector; document -> documentor; simplify -> simplifier",
				"Complete, block, and needs_input must select no worker profile",
				"stable question ID, positive revision, deterministic content SHA-256",
				"Never choose the recommendation",
				"concrete evidence references",
				"cite exact finding IDs",
				"missing evidence from evidence of a negative result",
				"Never claim completion when required verification, audit, acceptance, finding-resolution, or Git evidence is missing or stale",
				"Do not edit product files",
				"Never create commits",
				"execute the selected worker",
				"Never invoke Codex recursively",
				"launch nested Codex runs",
				"resume another session",
			},
		},
		{
			name: "planner",
			required: []string{
				"fresh, planning-only session",
				"Build or revise a durable plan",
				"Produce only the harness-requested structured output",
				"supplied JSON schema exactly",
				"ordered, stable, lower-case kebab-case plan and step IDs",
				"preserve its revision and predecessor relationships",
				"concrete, reviewable steps with observable completion conditions",
				"Cite exact evidence references",
				"silently drop completed steps, acceptance criteria, findings, or prior evidence",
				"Do not implement, edit source or task files",
				"persist or mutate plans or runtime state",
				"route autonomous work",
				"launch nested Codex runs",
				"resume a session",
			},
		},
		{
			name: "corrector",
			required: []string{
				"fresh, narrowly scoped source-changing session",
				"explicit verification failures and/or referenced audit finding IDs",
				"without broadening scope",
				"Prefer root-cause repairs",
				"Run only the relevant configured verification",
				"concrete new evidence",
				"partial correction, remaining failures, uncertainty, and blockers",
				"structured response or receipt schema exactly",
				"Do not perform unrelated cleanup, documentation, simplification, new roadmap work, or broad refactoring",
				"Never route another worker",
				"launch nested Codex runs",
				"decide that the overall task is complete",
				"resume a session",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			template := runProfileTemplateByName(t, tt.name)
			for _, want := range tt.required {
				if !strings.Contains(template.Content, want) {
					t.Fatalf("%s template missing %q:\n%s", tt.name, want, template.Content)
				}
			}
		})
	}
}

func TestCheckedInRunProfilesMatchTemplatesAndLoad(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, name := range []string{"supervisor", "planner", "corrector"} {
		t.Run(name, func(t *testing.T) {
			template := runProfileTemplateByName(t, name)
			path := filepath.Join(repositoryRoot, RunProfileSourcePath(name))
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read checked-in profile %s: %v", path, err)
			}
			want := strings.TrimRight(template.Content, "\n") + "\n"
			if got := string(raw); got != want {
				t.Fatalf("checked-in profile %s does not match template\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
			}

			profile, err := LoadRunProfile(repositoryRoot, name)
			if err != nil {
				t.Fatalf("load checked-in profile %s: %v", name, err)
			}
			if got, want := profile.Description, strings.TrimSpace(template.Content); got != want {
				t.Fatalf("loaded profile description = %q, want %q", got, want)
			}
		})
	}
}

func TestLoadNewRunProfilesMissingFilesReturnActionableErrors(t *testing.T) {
	for _, name := range []string{"supervisor", "planner", "corrector"} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadRunProfile(t.TempDir(), name)
			if err == nil {
				t.Fatal("load run profile succeeded, want missing file error")
			}
			for _, want := range []string{
				"missing " + RunProfileSourcePath(name),
				"run `revolvr init` or create the profile file",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %v, want %q", err, want)
				}
			}
		})
	}
}

func writeProfileFile(t *testing.T, repo string, name string, content string) {
	t.Helper()
	path := filepath.Join(repo, RunProfileSourcePath(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create profile dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write profile file: %v", err)
	}
}

func runProfileTemplateByName(t *testing.T, name string) RunProfileTemplate {
	t.Helper()
	for _, template := range DefaultRunProfileTemplates() {
		if template.Name == name {
			return template
		}
	}
	t.Fatalf("profile template %q not found", name)
	return RunProfileTemplate{}
}
