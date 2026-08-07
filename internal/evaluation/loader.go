package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadSuite(repositoryRoot string) (Suite, []byte, error) {
	path := filepath.Join(repositoryRoot, "evals", "scenarios.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, nil, err
	}
	var suite Suite
	if err := decodeStrict(raw, &suite); err != nil {
		return Suite{}, nil, err
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, nil, err
	}
	canonical, err := Canonical(suite)
	if err != nil {
		return Suite{}, nil, err
	}
	if string(raw) != string(canonical) {
		return Suite{}, nil, errors.New("evaluation: scenarios.json is not canonical")
	}
	return suite, canonical, nil
}

func FixtureIdentity(repositoryRoot, relative string) (string, error) {
	root := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("evaluation: fixture repository must be a real directory")
	}
	type entry struct {
		Path   string `json:"path"`
		Mode   uint32 `json:"mode"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	}
	var entries []entry
	err = filepath.WalkDir(root, func(path string, value fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if value.IsDir() {
			return nil
		}
		info, err := value.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || strings.HasPrefix(rel, ".git/") || rel == ".git" {
			return errors.New("evaluation: fixture repository contains a non-regular or Git-owned entry")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		entries = append(entries, entry{Path: rel, Mode: uint32(info.Mode().Perm()), SHA256: hex.EncodeToString(sum[:]), Size: info.Size()})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	raw, err := Canonical(entries)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}

func FreezeAuthority(repositoryRoot string, suite Suite, scenario Scenario) (FrozenAuthority, error) {
	fixtureSHA, err := FixtureIdentity(repositoryRoot, suite.FixtureRepository)
	if err != nil {
		return FrozenAuthority{}, err
	}
	policy := suite.Policy
	policy.ProtectedPaths = append([]string(nil), policy.ProtectedPaths...)
	policy.AllowedMutationPaths = append([]string(nil), policy.AllowedMutationPaths...)
	policySHA, err := hashValue(policy)
	if err != nil {
		return FrozenAuthority{}, err
	}
	requirementSHA := hashBytes([]byte(scenario.TaskRequirement))
	criterionSHA := hashBytes([]byte(scenario.AcceptanceRequirement))
	value := FrozenAuthority{
		SchemaVersion: AuthoritySchemaVersion,
		ScenarioID:    scenario.ID,
		Task: TaskAuthority{
			TaskID:         "eval-" + scenario.ID,
			TaskVersionID:  "eval-" + scenario.ID + "-v1",
			Requirement:    scenario.TaskRequirement,
			RequirementSHA: requirementSHA,
		},
		Acceptance: []CriterionAuthority{{
			CriterionID:       "ac-1",
			Requirement:       scenario.AcceptanceRequirement,
			RequirementSHA256: criterionSHA,
		}},
		Policy:       policy,
		PolicySHA256: policySHA,
		Source:       SourceAuthority{FixturePath: suite.FixtureRepository, FixtureSHA256: fixtureSHA},
		Expected:     ExpectedAuthority{Outcome: scenario.ExpectedOutcome, StopReason: scenario.ExpectedStopReason},
	}
	material := value
	material.SHA256 = ""
	value.SHA256, err = hashValue(material)
	return value, err
}
