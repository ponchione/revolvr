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
