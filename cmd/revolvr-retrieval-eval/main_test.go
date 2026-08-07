package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"revolvr/internal/index"
)

func TestRealProjectFixtureManifestResolvesExactPathsAndSymbols(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "internal", "retrieval", "testdata", "architecture-021-real-projects.json")
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	data, err := decodeDataset(raw)
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := "/home/gernsback/source"
	if value := os.Getenv("REVOLVR_EVAL_SOURCE_ROOT"); value != "" {
		sourceRoot = value
	}
	if _, err := os.Stat(filepath.Join(sourceRoot, "revolvr", ".git")); err != nil {
		t.Skip("exact real-project repositories are not present")
	}
	paths := map[string]map[string]bool{}
	symbols := map[string]map[string]map[string]bool{}
	for _, project := range data.Projects {
		snapshot, err := index.ReadGitSnapshot(context.Background(), index.DeterministicID("fixture", project.Name), filepath.Join(sourceRoot, project.Repository), project.Revision, project.Tree, index.AdmissionRules{Include: project.Include}, index.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		paths[project.Name] = map[string]bool{}
		symbols[project.Name] = map[string]map[string]bool{}
		for _, file := range snapshot.Files {
			paths[project.Name][file.Path] = true
			parsed, err := index.ParseFile(snapshot.ProjectID, file, index.DefaultLimits())
			if err != nil {
				t.Fatal(err)
			}
			symbols[project.Name][file.Path] = map[string]bool{}
			for _, symbol := range parsed.Symbols {
				symbols[project.Name][file.Path][symbol.Name] = true
			}
		}
	}
	for _, fixture := range data.Fixtures {
		for _, expected := range fixture.Expected {
			if !paths[fixture.Project][expected.Path] {
				t.Errorf("fixture %s expected path %s is not admitted", fixture.ID, expected.Path)
			}
			if expected.Symbol != "" && !symbols[fixture.Project][expected.Path][expected.Symbol] {
				t.Errorf("fixture %s expected symbol %s is not parsed from %s", fixture.ID, expected.Symbol, expected.Path)
			}
		}
	}
}
