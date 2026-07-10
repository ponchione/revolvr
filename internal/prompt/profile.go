package prompt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultRunProfileName = "implementer"

type RunProfile struct {
	Name        string
	Description string
	SourcePath  string
}

type RunProfileTemplate struct {
	Name    string
	Content string
}

func LoadRunProfile(repositoryRoot string, name string) (RunProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultRunProfileName
	}
	if !validRunProfileName(name) {
		return RunProfile{}, fmt.Errorf("load run profile: invalid profile name %q", name)
	}

	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if repositoryRoot == "" {
		repositoryRoot = "."
	}
	sourcePath := RunProfileSourcePath(name)
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, sourcePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RunProfile{}, fmt.Errorf("load run profile %q: missing %s; run `revolvr init` or create the profile file", name, sourcePath)
		}
		return RunProfile{}, fmt.Errorf("load run profile %q from %s: %w", name, sourcePath, err)
	}

	content := strings.TrimSpace(string(raw))
	if content == "" {
		return RunProfile{}, fmt.Errorf("load run profile %q: %s is empty", name, sourcePath)
	}
	return RunProfile{
		Name:        name,
		Description: content,
		SourcePath:  sourcePath,
	}, nil
}

func RunProfileSourcePath(name string) string {
	return filepath.Join(".agent", "profiles", name+".md")
}

func DefaultRunProfileTemplates() []RunProfileTemplate {
	return []RunProfileTemplate{
		{
			Name: DefaultRunProfileName,
			Content: "You are the implementer for this Revolvr pass.\n\n" +
				"Focus on the selected task, make small reviewable changes, preserve existing repository state, verify the work, and write the required receipt before stopping.",
		},
		{
			Name: "auditor",
			Content: "You are the auditor for this Revolvr pass.\n\n" +
				"Review the selected task and repository state for correctness, regressions, missing verification, and unclear risks. Prefer concrete findings with file and command evidence.",
		},
		{
			Name: "documentor",
			Content: "You are the documentor for this Revolvr pass.\n\n" +
				"Update operator-facing documentation for the selected change. Keep wording concise, accurate, and aligned with the current CLI behavior.",
		},
		{
			Name: "simplifier",
			Content: "You are the simplifier for this Revolvr pass.\n\n" +
				"Reduce unnecessary complexity, duplication, and line count only when doing so is meaningful. Preserve behavior, avoid clever abstractions, create helpers only when they reduce real duplication or complexity, and stop cleanly when no simplification is worthwhile.",
		},
	}
}

func validRunProfileName(name string) bool {
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
