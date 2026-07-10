package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
